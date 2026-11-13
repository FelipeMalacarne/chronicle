package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import com.google.inject.Provides;
import java.time.Clock;
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
import net.runelite.api.ScriptEvent;
import net.runelite.api.events.ChatMessage;
import net.runelite.api.events.GameStateChanged;
import net.runelite.api.events.GameTick;
import net.runelite.api.ScriptID;
import net.runelite.api.events.ItemContainerChanged;
import net.runelite.api.events.MenuOptionClicked;
import net.runelite.api.events.ScriptPostFired;
import net.runelite.api.events.ScriptPreFired;
import net.runelite.api.events.StatChanged;
import net.runelite.api.events.VarbitChanged;
import net.runelite.api.events.WidgetClosed;
import net.runelite.api.events.WidgetLoaded;
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
import net.runelite.client.util.LinkBrowser;
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
 *
 * <p>Managed workers (1.11.0): {@code CHRONICLE_LINK_SERVER_URL} and {@code CHRONICLE_LINK_TOKEN}
 * in the environment replace the config's URL and credential ({@link LinkEnvironment}); nothing
 * of them is written to the profile, and the panel says the credential comes from there.
 *
 * <p>Collection log (1.12.0): when the manifest offers {@code collection_log}, each page the
 * player opens in game is read from the interface ({@link CollectionLogReader}; never another
 * player's log from an adventure log) and sent in {@code data.collectionLog}. The control bridge
 * also answers {@code /bridge/version} and {@code /bridge/break} (the Break Handler's state).
 *
 * <p>Demonstration recording (1.13.0, planner docs/10 section 11): with "Record demonstration"
 * on and the manifest's {@code recording} flag, the owner's own interactions (NPCs and objects
 * clicked, dialogue options, shop purchases, bank changes, items used, a coarse position) are
 * recorded as typed events by {@link DemonstrationRecorder} and sent in {@code data.recording}
 * until a push carrying them is acknowledged. It only records; nothing acts in game.
 *
 * <p>Assisted mode (1.14.0, planner docs/10 phase 2): the panel shows the current step's typed
 * tasks as a checklist, with what the planner sees done; still read-only.
 *
 * <p>Task executor (1.15.0, planner docs/10 phase 3): in the dev harness client only, the control
 * bridge starts one typed task (travel, bank or buy) the planner's agent sends, and a local state
 * machine ({@link TaskMachine}) executes it over Microbot's primitives on its own thread. Nothing
 * starts it but the agent through the bridge; tracking stays as it was. When the machine ends the
 * plugin asks for a capture, so the ingest (position and item containers) proves the task's
 * termination soon: the machine's own "done" never does.
 */
@PluginDescriptor(
	name = ChronicleLinkPlugin.TAG + "Chronicle Link",
	description = "Syncs skills, quests, diary varbits, the collection log pages you open and opted-in items to your Chronicle (Ironman Planner) server and shows its route in a side panel",
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
	static final String VERSION = "1.15.0";
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
	/** The task executor the bridge drives (dev harness client only). */
	private TaskExecutor tasks;

	/** Client thread only. */
	private StateCollector collector;

	/** The demonstration: fed on the client thread, drained by the sync thread (it locks). */
	private final DemonstrationRecorder recorder = new DemonstrationRecorder();
	/** Client thread only; null while the plugin is off. */
	private DemonstrationSource demonstration;

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

	/** URL and token from the environment (managed workers), or null: then the config decides. */
	private volatile LinkEnvironment linkEnv;

	/** This installation's id, generated once into the config. */
	private volatile String installationId;

	/** The login of Credential Login (planner item 15); used from the sync and panel threads. */
	private volatile LoginSession login;

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
			// The task's end asks for a capture: the planner proves the termination from it.
			tasks = new TaskExecutor(new MicrobotTaskGame(), Clock.systemUTC(),
				m -> trigger.onEvent(CaptureTrigger.Strength.HARD, Instant.now()));
			bridge = new AgentBridge(gson, configManager, tasks);
			bridge.start();
		}
		linkEnv = LinkEnvironment.read(System::getenv);
		if (linkEnv != null)
		{
			// token mode needs no installation id, and a worker's profile keeps nothing of ours
			log.info("Chronicle Link: server URL and token from the environment");
			installationId = config.installationId();
		}
		else
		{
			installationId = InstallationId.ensure(config.installationId(),
				id -> configManager.setConfiguration(ChronicleLinkConfig.GROUP, InstallationId.CONFIG_KEY, id),
				UUID::randomUUID);
		}
		login = new LoginSession(new DeviceLogin(okHttpClient, gson), config::loginRefreshToken,
			token -> configManager.setConfiguration(ChronicleLinkConfig.GROUP, ChronicleLinkConfig.LOGIN_REFRESH_TOKEN, token));
		collector = new StateCollector(client);
		demonstration = new DemonstrationSource(client, recorder);
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
			public void login()
			{
				onPanel(() ->
				{
					login.start(PlannerHttpClient.parseServer(serverUrl()), Instant.now());
					publishLogin();
				});
			}

			@Override
			public void cancelLogin()
			{
				onPanel(() ->
				{
					login.cancel();
					publishLogin();
				});
			}

			@Override
			public void logout()
			{
				onPanel(() ->
				{
					login.signOut();
					publishLogin();
					panelController.refresh(Instant.now());
				});
			}

			@Override
			public void open(String url)
			{
				LinkBrowser.browse(url);
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
		boolean fromEnvironment = linkEnv != null;
		RoutePanel.onSwing(() ->
		{
			routePanel = new RoutePanel(actions, config.itemsExpanded());
			routePanel.setFromEnvironment(fromEnvironment);
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
			if (login.poll(now))
			{
				// confirmed, refused or expired in the browser
				publishLogin();
				panelController.refresh(now);
			}
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
		onPanel(() ->
		{
			publishLogin();
			panelController.refresh(Instant.now());
		});
	}

	/** Hands the login state to the panel; null outside Credential Login. Panel thread. */
	private void publishLogin()
	{
		LoginSession.View view = credentialMode() == ChronicleLinkConfig.CredentialMode.LOGIN ? login.view() : null;
		RoutePanel.onSwing(() ->
		{
			if (routePanel != null)
			{
				routePanel.updateLogin(view);
			}
		});
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
		HttpUrl base = PlannerHttpClient.parseServer(serverUrl());
		PlannerCredential credential = credential();
		return base == null || !credential.ready() ? null : new RoutePanelController.Endpoint(base, credential);
	}

	/** The server URL in effect: the environment's, else the config's. */
	private String serverUrl()
	{
		return LinkEnvironment.serverUrl(linkEnv, config.serverUrl());
	}

	/** The credential mode in effect: token with the environment, else the config's. */
	private ChronicleLinkConfig.CredentialMode credentialMode()
	{
		return LinkEnvironment.mode(linkEnv, config.credentialMode());
	}

	private PlannerCredential credential()
	{
		LinkEnvironment env = linkEnv;
		if (env != null)
		{
			return env.credential();
		}
		if (credentialMode() == ChronicleLinkConfig.CredentialMode.LOGIN)
		{
			LoginSession session = login;
			String access = session == null ? null : session.accessToken(PlannerHttpClient.parseServer(serverUrl()), Instant.now());
			return new LoginCredential(access, installationId, config.account());
		}
		return PlannerCredential.of(config.credentialMode(), config.ingestToken(), installationId, config.account());
	}

	/** A 401 from the planner: a login renews its access token on the next request. */
	private void onStatus(int status)
	{
		LoginSession session = login;
		if (status == 401 && session != null)
		{
			session.dropAccess();
		}
	}

	@Override
	protected void shutDown()
	{
		configSchema = null;
		if (tasks != null)
		{
			tasks.stop();
			tasks = null;
		}
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
		login = null;
		panelVisible = false;
		linkEnv = null;
		overlayManager.remove(overlay);
		engine = null;
		engineKey = null;
		lastSnapshot = null;
		logoutSnapshot = null;
		eventFilter = null;
		trigger.clear();
		demonstration = null;
		recorder.stop();
	}

	SyncStatus getStatus()
	{
		return status;
	}

	/** Whether the demonstration is being recorded, and how many events wait for the planner. */
	boolean isRecording()
	{
		return recorder.recording();
	}

	int recordingPending()
	{
		return recorder.pendingCount();
	}

	/**
	 * Matches the recorder to the owner's toggle, the manifest's flag and the login, starting or
	 * stopping a recording; true while recording. Client thread.
	 */
	private boolean recordingNow()
	{
		DemonstrationSource d = demonstration;
		EventFilter filter = eventFilter;
		boolean wanted = filter != null && filter.recording() && config.recordDemonstration()
			&& collector != null && collector.readyForEvents();
		return d != null && d.sync(wanted);
	}

	/** A demonstration event was recorded: it goes out with the next capture. */
	private void recorded(Instant now)
	{
		EventFilter filter = eventFilter;
		if (filter != null)
		{
			trigger.onEvent(filter.recorded(), now);
		}
	}

	/** Every clicked menu entry; the demonstration records the owner's own, never acting on any. */
	@Subscribe
	public void onMenuOptionClicked(MenuOptionClicked event)
	{
		if (recordingNow() && demonstration.onMenu(event, Instant.now()))
		{
			recorded(Instant.now());
		}
	}

	@Subscribe
	public void onWidgetClosed(WidgetClosed event)
	{
		DemonstrationSource d = demonstration;
		if (d != null)
		{
			d.onWidgetClosed(event.getGroupId());
		}
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
			// the next login is a new recording; what is pending still goes out
			recorder.stop();
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
		// every tick also starts or stops the recording as the toggle and the login say
		if (recordingNow() && demonstration.onTick(Instant.now()))
		{
			recorded(Instant.now());
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
		if (recordingNow() && demonstration.onContainer(event.getContainerId(), now))
		{
			recorded(now);
		}
	}

	/**
	 * The collection log drew a page: read it (this character's log only) and, when the page is
	 * new or changed and the manifest takes pages, ask for a capture.
	 */
	@Subscribe
	public void onScriptPreFired(ScriptPreFired event)
	{
		// a number key on the dialogue options: the demonstration records the choice (scripts run
		// all the time, so the cheap test of the source widget goes first)
		ScriptEvent se = event.getScriptEvent();
		if (se != null && se.getSource() != null && DemonstrationMenu.dialogueWidget(se.getSource().getId())
			&& recordingNow() && demonstration.onScript(event, Instant.now()))
		{
			recorded(Instant.now());
		}
	}

	@Subscribe
	public void onScriptPostFired(ScriptPostFired event)
	{
		if (event.getScriptId() != ScriptID.COLLECTION_DRAW_LIST || collector == null)
		{
			return;
		}
		Instant now = Instant.now();
		EventFilter filter = eventFilter;
		if (collector.onCollectionLogDrawn(now) && filter != null && collector.readyForEvents())
		{
			trigger.onEvent(filter.collectionLog(), now);
		}
	}

	/** An adventure log can open another player's collection log: the collector tracks whose. */
	@Subscribe
	public void onWidgetLoaded(WidgetLoaded event)
	{
		if (collector != null)
		{
			collector.onWidgetLoaded(event.getGroupId());
		}
		// the bank, a shop or new dialogue options: the demonstration's context
		if (recordingNow() && demonstration.onWidgetLoaded(event.getGroupId(), Instant.now()))
		{
			recorded(Instant.now());
		}
	}

	@Subscribe
	public void onConfigChanged(ConfigChanged event)
	{
		if (!ChronicleLinkConfig.GROUP.equals(event.getGroup()))
		{
			return;
		}
		if (ChronicleLinkConfig.LOGIN_REFRESH_TOKEN.equals(event.getKey()))
		{
			// a renewal rotates it; nothing to resend
			return;
		}
		fullRequested = true;
		if ("credentialMode".equals(event.getKey()) || "serverUrl".equals(event.getKey()))
		{
			onPanel(() ->
			{
				publishLogin();
				panelController.refresh(Instant.now());
			});
		}
	}

	private void tick()
	{
		try
		{
			HttpUrl base = PlannerHttpClient.parseServer(serverUrl());
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
			onStatus(result.status);
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
				onStatus(result.status);
			}
			engine.onPushResult(result.status, result.body, Instant.now());
			IngestContract.Response body = result.body;
			if (result.status == 200 && body != null && body.ok && body.acceptedSeq == push.clientSeq)
			{
				// stored on the planner: the recorder drops what this push carried
				recorder.acknowledge(push.data.recording);
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
				PlayerSnapshot snapshot = collector.capture(manifest, config, Instant.now());
				if (snapshot != null && manifest.recordingEnabled())
				{
					// the events of this character still waiting for the planner, whatever the toggle now
					snapshot = snapshot.withRecording(recorder.pending(snapshot.username, DemonstrationRecorder.MAX_PER_PUSH));
				}
				future.complete(snapshot);
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
