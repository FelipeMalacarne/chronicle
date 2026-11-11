package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import com.google.inject.Provides;
import java.time.Instant;
import java.util.UUID;
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
import net.runelite.api.events.ChatMessage;
import net.runelite.api.events.GameStateChanged;
import net.runelite.api.events.GameTick;
import net.runelite.api.events.ItemContainerChanged;
import net.runelite.api.events.StatChanged;
import net.runelite.api.events.VarbitChanged;
import net.runelite.api.gameval.InventoryID;
import net.runelite.client.callback.ClientThread;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.events.ConfigChanged;
import net.runelite.client.events.ExternalPluginsChanged;
import net.runelite.client.events.PluginChanged;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.ui.ClientToolbar;
import net.runelite.client.ui.NavigationButton;
import net.runelite.client.ui.overlay.OverlayManager;
import okhttp3.HttpUrl;
import okhttp3.OkHttpClient;

/**
 * Chronicle Link: Chronicle's plugin (the Ironman Planner webapp). Its tracking is read-only
 * (ingest contract v1): it reads skills, the
 * manifest's varbits/varps, quest states (when the manifest offers {@code quests}) and, when
 * opted in twice (toggle + token scope), the bank mirror, inventory and equipment from
 * Microbot's caches, then sends a full baseline followed by
 * acknowledged deltas. It never performs a game action. Reads happen on the client thread;
 * HTTP and the protocol state machine run on the plugin's own single thread.
 *
 * <p>Cadence: game events that can change what is sent (a collected container, a listed var,
 * quest vars, stats) ask {@link CaptureTrigger} for a capture about a second later; the
 * manifest's {@code interval_seconds} capture stays as a safety net and is pushed back by every
 * capture.
 *
 * <p>Route panel (1.2.0, readable layout 1.2.1): a side panel shows the planner's window of the active route and
 * writes the owner's marks (Feito, Pular, Desfazer) through the planner's mark API. It refetches
 * after an acknowledged push that changed the route, after each mark, on login and every 30 s
 * while visible; the last window stays on screen, marked outdated, while the planner is down.
 * Its HTTP runs on a third thread ({@code chronicle-link-panel}), its Swing work on the EDT.
 */
@PluginDescriptor(
	name = ChronicleLinkPlugin.TAG + "Chronicle Link",
	description = "Syncs skills, quests, diary varbits and opted-in items to your Chronicle (Ironman Planner) server and shows its route in a side panel",
	tags = {"chronicle", "ironman", "tracker", "planner", "guide", "panel"},
	authors = {"FelipeMalacarne"},
	version = ChronicleLinkPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class ChronicleLinkPlugin extends Plugin
{
	static final String VERSION = "1.7.0";
	/** The dev harness client (bin/mb-client): the only one that serves Chronicle's control bridge. */
	static final String HARNESS_PROPERTY = "microbot.dev.harness";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";
	private static final long CAPTURE_TIMEOUT_MS = 2_000;
	/** How often the sync thread looks at the engine: well under {@link CaptureTrigger#DEBOUNCE}. */
	private static final long TICK_PERIOD_MS = 250;

	@Inject
	private Client client;

	@Inject
	private ClientThread clientThread;

	@Inject
	private ChronicleLinkConfig config;

	@Inject
	private OverlayManager overlayManager;

	@Inject
	private PlannerStatusOverlay overlay;

	@Inject
	private OkHttpClient okHttpClient;

	@Inject
	private Gson gson;

	@Inject
	private ClientToolbar clientToolbar;

	@Inject
	private ConfigManager configManager;

	/** Chronicle's control part, dev harness client only: config options and the bridge. */
	private ConfigSchemaExporter configSchema;
	private volatile AgentBridge bridge;

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

	/** Events from the client thread, drained by the sync thread. */
	private final CaptureTrigger trigger = new CaptureTrigger();
	/** What the current manifest makes worth a capture; null ignores every event. */
	private volatile EventFilter eventFilter;

	/** This installation's id, generated once into the config. */
	private volatile String installationId;

	// route panel: controller only on panelExecutor, panel only on the EDT
	private ScheduledExecutorService panelExecutor;
	private RoutePanelController panelController;
	private RoutePanel routePanel;
	private NavigationButton navigationButton;
	private volatile boolean panelVisible;

	@Provides
	ChronicleLinkConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(ChronicleLinkConfig.class);
	}

	@Override
	protected void startUp()
	{
		log.info("Chronicle Link {} starting", VERSION);
		if (Boolean.getBoolean(HARNESS_PROPERTY))
		{
			// Tracking stays read-only; only the dev harness client also serves the control bridge
			// (behind the Agent Server's token) and the config options for Chronicle's form.
			configSchema = new ConfigSchemaExporter(Microbot.getPluginManager(), configManager, gson);
			configSchema.export();
			bridge = new AgentBridge(gson, configManager);
			bridge.start();
		}
		installationId = InstallationId.ensure(config.installationId(),
			id -> configManager.setConfiguration(ChronicleLinkConfig.GROUP, InstallationId.CONFIG_KEY, id),
			UUID::randomUUID);
		collector = new StateCollector(client);
		status = notConfigured();
		trigger.clear();
		eventFilter = null;
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
			Thread t = new Thread(r, "chronicle-link-sync");
			t.setDaemon(true);
			return t;
		});
		executor.scheduleWithFixedDelay(this::tick, TICK_PERIOD_MS, TICK_PERIOD_MS, TimeUnit.MILLISECONDS);
		startPanel();
	}

	private void startPanel()
	{
		panelExecutor = Executors.newSingleThreadScheduledExecutor(r ->
		{
			Thread t = new Thread(r, "chronicle-link-panel");
			t.setDaemon(true);
			return t;
		});
		PlannerHttpClient panelHttp = new PlannerHttpClient(okHttpClient, gson);
		panelController = new RoutePanelController(panelHttp, this::panelEndpoint,
			state -> RoutePanel.onSwing(() ->
			{
				if (routePanel != null)
				{
					routePanel.update(state);
				}
			}));
		RoutePanel.Actions actions = new RoutePanel.Actions()
		{
			@Override
			public void mark(String stepId, String kind)
			{
				onPanel(() -> panelController.mark(stepId, kind, Instant.now()));
			}

			@Override
			public void undo()
			{
				onPanel(() -> panelController.undo(Instant.now()));
			}

			@Override
			public void refresh()
			{
				onPanel(() -> panelController.refresh(Instant.now()));
			}

			@Override
			public void itemsExpanded(boolean expanded)
			{
				configManager.setConfiguration(ChronicleLinkConfig.GROUP, ChronicleLinkConfig.ITEMS_EXPANDED, expanded);
			}

			@Override
			public void visible(boolean visible)
			{
				panelVisible = visible;
				if (visible)
				{
					onPanel(() ->
					{
						if (panelController.pollDue(Instant.now(), true))
						{
							panelController.refresh(Instant.now());
						}
					});
				}
			}
		};
		RoutePanel.onSwing(() ->
		{
			routePanel = new RoutePanel(actions, config.itemsExpanded());
			navigationButton = NavigationButton.builder()
				.tooltip("Chronicle")
				.icon(RoutePanel.navigationIcon())
				.priority(8)
				.panel(routePanel)
				.build();
			clientToolbar.addNavigation(navigationButton);
		});
		panelExecutor.scheduleWithFixedDelay(() ->
		{
			Instant now = Instant.now();
			if (panelController.pollDue(now, panelVisible))
			{
				panelController.refresh(now);
			}
			else if (panelVisible)
			{
				// keeps the "h\u00e1 N s" line current
				RoutePanel.onSwing(() ->
				{
					if (routePanel != null)
					{
						routePanel.tickClock();
					}
				});
			}
		}, 5, 5, TimeUnit.SECONDS);
		// a first window for the panel, even before login
		onPanel(() -> panelController.refresh(Instant.now()));
	}

	/** Runs on the panel's own thread; never on the client or Swing thread. */
	private void onPanel(Runnable r)
	{
		ScheduledExecutorService ex = panelExecutor;
		if (ex == null)
		{
			return;
		}
		try
		{
			ex.execute(() ->
			{
				try
				{
					r.run();
				}
				catch (RuntimeException e)
				{
					// the class name only: messages can carry request details
					log.debug("Chronicle Link panel task failed: {}", e.getClass().getSimpleName());
				}
			});
		}
		catch (java.util.concurrent.RejectedExecutionException ignored)
		{
			// shutting down
		}
	}

	/** The panel's planner endpoint, or null while not configured. */
	private RoutePanelController.Endpoint panelEndpoint()
	{
		HttpUrl base = PlannerHttpClient.parseServer(config.serverUrl());
		PlannerCredential credential = credential();
		return base == null || !credential.ready() ? null : new RoutePanelController.Endpoint(base, credential);
	}

	private PlannerCredential credential()
	{
		return PlannerCredential.of(config.credentialMode(), config.ingestToken(), installationId, config.account());
	}

	@Override
	protected void shutDown()
	{
		configSchema = null;
		if (bridge != null)
		{
			bridge.stop();
			bridge = null;
		}
		if (executor != null)
		{
			executor.shutdownNow();
			executor = null;
		}
		if (panelExecutor != null)
		{
			panelExecutor.shutdownNow();
			panelExecutor = null;
		}
		NavigationButton button = navigationButton;
		RoutePanel.onSwing(() ->
		{
			if (button != null)
			{
				clientToolbar.removeNavigation(button);
			}
			routePanel = null;
		});
		navigationButton = null;
		panelController = null;
		panelVisible = false;
		overlayManager.remove(overlay);
		engine = null;
		engineKey = null;
		lastSnapshot = null;
		logoutSnapshot = null;
		eventFilter = null;
		trigger.clear();
	}

	SyncStatus getStatus()
	{
		return status;
	}

	/** A plugin loaded, unloaded, installed or removed: its config options come or go. */
	@Subscribe
	public void onPluginChanged(PluginChanged event)
	{
		if (configSchema != null)
		{
			configSchema.export();
		}
	}

	@Subscribe
	public void onExternalPluginsChanged(ExternalPluginsChanged event)
	{
		if (configSchema != null)
		{
			configSchema.export();
		}
	}

	/** Chat messages feed the bridge's game events (dev harness client only). */
	@Subscribe
	public void onChatMessage(ChatMessage event)
	{
		AgentBridge b = bridge;
		if (b != null)
		{
			b.onChat(event.getType(), event.getMessage());
		}
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
			trigger.clear();
			fullRequested = true;
			onPanel(() -> panelController.refresh(Instant.now()));
		}
		else if (event.getGameState() == GameState.LOGIN_SCREEN && collector.inEpoch())
		{
			collector.endEpoch();
			trigger.clear();
			logoutRequested = true;
		}
	}

	@Subscribe
	public void onGameTick(GameTick event)
	{
		if (collector != null && collector.onGameTick())
		{
			// the post-login wait just ended: the login's full can be read now, not at the retry
			trigger.onEvent(CaptureTrigger.Strength.HARD, Instant.now());
		}
	}

	/**
	 * Quest states derive from vars, so any change marks the cached quests dirty; they are
	 * re-read at the next capture. Whether the change also asks for one is {@link EventFilter#var}.
	 */
	@Subscribe
	public void onVarbitChanged(VarbitChanged event)
	{
		if (collector == null)
		{
			return;
		}
		collector.onVarsChanged();
		EventFilter filter = eventFilter;
		if (filter != null && collector.readyForEvents())
		{
			trigger.onEvent(filter.var(event.getVarpId(), event.getVarbitId()), Instant.now());
		}
	}

	@Subscribe
	public void onStatChanged(StatChanged event)
	{
		if (collector == null)
		{
			return;
		}
		// tracked from the first ticks on, so the first level-up after login is recognised
		boolean levelUp = collector.onStatChanged(event.getSkill(), event.getLevel());
		EventFilter filter = eventFilter;
		if (filter != null && collector.readyForEvents())
		{
			trigger.onEvent(filter.stat(levelUp), Instant.now());
		}
	}

	@Subscribe
	public void onItemContainerChanged(ItemContainerChanged event)
	{
		if (collector == null)
		{
			return;
		}
		Instant now = Instant.now();
		if (event.getContainerId() == InventoryID.BANK)
		{
			collector.onBankContainerChanged(now);
		}
		EventFilter filter = eventFilter;
		if (filter != null && collector.readyForEvents())
		{
			trigger.onEvent(filter.container(event.getContainerId(), config), now);
		}
	}

	@Subscribe
	public void onConfigChanged(ConfigChanged event)
	{
		if (ChronicleLinkConfig.GROUP.equals(event.getGroup()))
		{
			fullRequested = true;
		}
	}

	private void tick()
	{
		try
		{
			HttpUrl base = PlannerHttpClient.parseServer(config.serverUrl());
			PlannerCredential token = credential();
			if (base == null || !token.ready())
			{
				engine = null;
				engineKey = null;
				eventFilter = null;
				status = notConfigured();
				return;
			}
			String key = base + "\n" + token.fingerprint();
			if (!key.equals(engineKey))
			{
				engine = new SyncEngine();
				http = new PlannerHttpClient(okHttpClient, gson);
				engineKey = key;
				lastSnapshot = null;
				logoutSnapshot = null;
				eventFilter = null;
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

			switch (trigger.next(engine, now))
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
			log.debug("Chronicle Link sync tick failed: {}", ex.getClass().getSimpleName());
		}
	}

	private void fetchManifest(HttpUrl base, PlannerCredential token)
	{
		PlannerHttpClient.Result<IngestContract.Manifest> result = http.fetchManifest(base, token);
		if (result.status == 200 && result.body != null)
		{
			engine.onManifest(result.body, Instant.now());
			eventFilter = EventFilter.of(engine.manifest());
		}
		else
		{
			log.debug("Chronicle Link manifest failed: HTTP {}", result.status);
			engine.onManifestError(result.status, Instant.now());
		}
	}

	private void captureAndPush(HttpUrl base, PlannerCredential token) throws InterruptedException
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
			log.debug("Chronicle Link push #{} timed out", push.clientSeq);
			engine.onPushError(Instant.now());
		}
		else
		{
			if (result.status != 200)
			{
				log.debug("Chronicle Link push #{} answered HTTP {}", push.clientSeq, result.status);
			}
			engine.onPushResult(result.status, result.body, Instant.now());
			IngestContract.Response body = result.body;
			if (result.status == 200 && body != null && body.ok && body.acceptedSeq == push.clientSeq)
			{
				// the panel decides from the response's route whether to refetch
				onPanel(() -> panelController.onPushAck(body, Instant.now()));
			}
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
