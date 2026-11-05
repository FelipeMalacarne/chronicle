package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import com.google.gson.Gson;
import com.google.inject.Provides;
import java.time.Instant;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Client;
import net.runelite.api.GameState;
import net.runelite.api.events.GameStateChanged;
import net.runelite.api.events.GameTick;
import net.runelite.api.events.ItemContainerChanged;
import net.runelite.api.events.VarbitChanged;
import net.runelite.api.gameval.InventoryID;
import net.runelite.client.callback.ClientThread;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.events.ConfigChanged;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.ui.overlay.OverlayManager;
import okhttp3.HttpUrl;
import okhttp3.OkHttpClient;

/**
 * Read-only tracker for the Ironman Planner (ingest contract v1). It reads skills, the
 * manifest's varbits/varps, quest states (when the manifest offers {@code quests}) and, when
 * opted in twice (toggle + token scope), the bank mirror, inventory and equipment from
 * Microbot's caches, then sends a full baseline followed by
 * acknowledged deltas. It never performs a game action. Reads happen on the client thread;
 * HTTP and the protocol state machine run on the plugin's own single thread.
 */
@PluginDescriptor(
	name = PluginDescriptor.Mocrosoft + "Ironman Planner",
	description = "Read-only: syncs skills, quests, diary varbits and opted-in items to your Ironman Planner server",
	tags = {"ironman", "tracker", "planner"},
	authors = {"FelipeMalacarne"},
	version = IronmanPlannerPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class IronmanPlannerPlugin extends Plugin
{
	static final String VERSION = "1.0.0";
	private static final long CAPTURE_TIMEOUT_MS = 2_000;

	@Inject
	private Client client;

	@Inject
	private ClientThread clientThread;

	@Inject
	private IronmanPlannerConfig config;

	@Inject
	private OverlayManager overlayManager;

	@Inject
	private PlannerStatusOverlay overlay;

	@Inject
	private OkHttpClient okHttpClient;

	@Inject
	private Gson gson;

	/** Client thread only. */
	private StateCollector collector;

	private ScheduledExecutorService executor;

	// executor thread only
	private SyncEngine engine;
	private PlannerHttpClient http;
	private String engineKey;
	private PlayerSnapshot logoutSnapshot;
	private PlayerSnapshot lastSnapshot;

	// handed from the client thread to the executor
	private volatile boolean fullRequested;
	private volatile boolean logoutRequested;

	private volatile SyncStatus status = notConfigured();

	@Provides
	IronmanPlannerConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(IronmanPlannerConfig.class);
	}

	@Override
	protected void startUp()
	{
		log.info("Ironman Planner {} starting", VERSION);
		collector = new StateCollector(client);
		status = notConfigured();
		overlayManager.add(overlay);
		clientThread.invoke(() ->
		{
			if (client.getGameState() == GameState.LOGGED_IN && !collector.inEpoch())
			{
				collector.startEpoch();
				fullRequested = true;
			}
		});
		executor = Executors.newSingleThreadScheduledExecutor(r ->
		{
			Thread t = new Thread(r, "ironman-planner-sync");
			t.setDaemon(true);
			return t;
		});
		executor.scheduleWithFixedDelay(this::tick, 1, 1, TimeUnit.SECONDS);
	}

	@Override
	protected void shutDown()
	{
		if (executor != null)
		{
			executor.shutdownNow();
			executor = null;
		}
		overlayManager.remove(overlay);
		engine = null;
		engineKey = null;
		lastSnapshot = null;
		logoutSnapshot = null;
	}

	SyncStatus getStatus()
	{
		return status;
	}

	@Subscribe
	public void onGameStateChanged(GameStateChanged event)
	{
		if (collector == null)
		{
			return;
		}
		collector.onGameStateChanged(event.getGameState());
		// world hops pass through HOPPING/LOADING, not LOGIN_SCREEN, and keep the live epoch
		if (event.getGameState() == GameState.LOGGED_IN && !collector.inEpoch())
		{
			collector.startEpoch();
			fullRequested = true;
		}
		else if (event.getGameState() == GameState.LOGIN_SCREEN && collector.inEpoch())
		{
			collector.endEpoch();
			logoutRequested = true;
		}
	}

	@Subscribe
	public void onGameTick(GameTick event)
	{
		if (collector != null)
		{
			collector.onGameTick();
		}
	}

	/**
	 * Quest states derive from vars; this only marks the cached quests dirty. They are re-read
	 * at the next capture, so at most once per interval however many vars change.
	 */
	@Subscribe
	public void onVarbitChanged(VarbitChanged event)
	{
		if (collector != null)
		{
			collector.onVarsChanged();
		}
	}

	@Subscribe
	public void onItemContainerChanged(ItemContainerChanged event)
	{
		if (collector != null && event.getContainerId() == InventoryID.BANK)
		{
			collector.onBankContainerChanged(Instant.now());
		}
	}

	@Subscribe
	public void onConfigChanged(ConfigChanged event)
	{
		if (IronmanPlannerConfig.GROUP.equals(event.getGroup()))
		{
			fullRequested = true;
		}
	}

	private void tick()
	{
		try
		{
			HttpUrl base = PlannerHttpClient.parseServer(config.serverUrl());
			String token = config.ingestToken() == null ? "" : config.ingestToken().trim();
			if (base == null || token.isEmpty())
			{
				engine = null;
				engineKey = null;
				status = notConfigured();
				return;
			}
			String key = base + "\n" + token;
			if (!key.equals(engineKey))
			{
				engine = new SyncEngine();
				http = new PlannerHttpClient(okHttpClient, gson);
				engineKey = key;
				lastSnapshot = null;
				logoutSnapshot = null;
			}

			Instant now = Instant.now();
			if (logoutRequested)
			{
				logoutRequested = false;
				// the last known state goes out as one more full
				logoutSnapshot = lastSnapshot;
				lastSnapshot = null;
				engine.requestFull(now);
			}
			if (fullRequested)
			{
				fullRequested = false;
				engine.requestFull(now);
			}

			switch (engine.next(now))
			{
				case FETCH_MANIFEST:
					fetchManifest(base, token);
					break;
				case CAPTURE:
					captureAndPush(base, token);
					break;
				default:
					break;
			}
			status = engine.status();
		}
		catch (InterruptedException ex)
		{
			// shutDown() stops the executor mid-capture
			Thread.currentThread().interrupt();
		}
		catch (Exception ex)
		{
			// the class name only: messages can carry request details
			log.debug("Ironman Planner sync tick failed: {}", ex.getClass().getSimpleName());
		}
	}

	private void fetchManifest(HttpUrl base, String token)
	{
		PlannerHttpClient.Result<IngestContract.Manifest> result = http.fetchManifest(base, token);
		if (result.status == 200 && result.body != null)
		{
			engine.onManifest(result.body, Instant.now());
		}
		else
		{
			log.debug("Ironman Planner manifest failed: HTTP {}", result.status);
			engine.onManifestError(result.status, Instant.now());
		}
	}

	private void captureAndPush(HttpUrl base, String token) throws InterruptedException
	{
		IngestContract.Manifest manifest = engine.manifest();
		PlayerSnapshot snapshot = capture(manifest);
		if (snapshot != null)
		{
			lastSnapshot = snapshot;
			logoutSnapshot = null;
		}
		else if (logoutSnapshot != null && logoutSnapshot.manifestVersion == manifest.manifestVersion)
		{
			snapshot = logoutSnapshot;
		}

		IngestContract.Push push = engine.prepare(snapshot, Instant.now());
		if (push == null)
		{
			return;
		}
		PlannerHttpClient.Result<IngestContract.Response> result = http.push(base, token, push);
		if (result.status == 0)
		{
			log.debug("Ironman Planner push #{} timed out", push.clientSeq);
			engine.onPushError(Instant.now());
		}
		else
		{
			if (result.status != 200)
			{
				log.debug("Ironman Planner push #{} answered HTTP {}", push.clientSeq, result.status);
			}
			engine.onPushResult(result.status, result.body, Instant.now());
		}
		if (engine.isAcknowledged(logoutSnapshot))
		{
			logoutSnapshot = null;
		}
	}

	/** Copies state out on the client thread; the executor waits, the client thread never does. */
	private PlayerSnapshot capture(IngestContract.Manifest manifest) throws InterruptedException
	{
		CompletableFuture<PlayerSnapshot> future = new CompletableFuture<>();
		clientThread.invoke(() ->
		{
			try
			{
				future.complete(collector.capture(manifest, config, Instant.now()));
			}
			catch (RuntimeException ex)
			{
				future.completeExceptionally(ex);
			}
		});
		try
		{
			return future.get(CAPTURE_TIMEOUT_MS, TimeUnit.MILLISECONDS);
		}
		catch (TimeoutException | ExecutionException ex)
		{
			future.cancel(false);
			return null;
		}
	}

	private static SyncStatus notConfigured()
	{
		return new SyncStatus(SyncStatus.Kind.NOT_CONFIGURED, null, 0, null);
	}
}
