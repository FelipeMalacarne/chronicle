package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Quest;
import net.runelite.api.QuestState;
import net.runelite.api.Skill;
import net.runelite.api.gameval.VarPlayerID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.Script;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.globval.enums.InterfaceTab;
import net.runelite.client.plugins.microbot.testing.TestResult;
import net.runelite.client.plugins.microbot.testing.TestResultWriter;
import net.runelite.client.plugins.microbot.util.combat.Rs2Combat;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.tabs.Rs2Tab;

/**
 * Runs the {@link Step}s in priority order every 50 ms; the first that acts owns the tick.
 * Safety comes first: the ladder (or the unarmed reset) ends a fight, wine keeps HP up, and the
 * run only stops once nothing is hitting us. HP and hits arrive through {@link GameEvents}.
 */
@Slf4j
public class BlackjackerScript extends Script
{
	// Short: the first pick has to be clicked within the tick of the knock-out message.
	private static final long LOOP_MS = 50;

	private final GameEvents events = new GameEvents();
	private BlackjackerConfig config;
	// Read by the overlay on the client thread.
	private volatile Session session;
	private List<Step> steps;
	private TestResult testResult;
	private int startXp = -1;
	private long lastReport;
	private long retaliateCheckedAt;

	GameEvents events()
	{
		return events;
	}

	/** The run in progress, or null before it starts and after it stops. */
	Session session()
	{
		return session;
	}

	/** Thieving XP gained this run. */
	int xpGained()
	{
		return startXp < 0 || events.thievingXp() == 0 ? 0 : events.thievingXp() - startXp;
	}

	public boolean run(BlackjackerConfig config)
	{
		this.config = config;
		session = null;
		events.reset();
		startXp = -1;
		testResult = Boolean.getBoolean("microbot.test.mode") ? new TestResult("Blackjacker") : null;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				if (!Microbot.isLoggedIn()) return;
				if (!super.run()) return;
				loop();
				report(false);
			}
			catch (Exception ex)
			{
				log.error("Blackjacker loop failed", ex);
			}
		}, 0, LOOP_MS, TimeUnit.MILLISECONDS);
		return true;
	}

	@Override
	public void shutdown()
	{
		super.shutdown();
		session = null;
	}

	private void loop()
	{
		if (session == null && !start())
		{
			return;
		}
		Session s = session;
		if (!s.begin())
		{
			return;
		}
		checkLimits(s);
		if (s.stopReason != null && (s.room.upstairs(s.me) || !s.attacked))
		{
			stop(s.stopReason);
			return;
		}
		for (Step step : steps)
		{
			if (step.run(s))
			{
				return;
			}
		}
	}

	/** Picks the target for the Thieving level and builds the steps; false when it cannot run. */
	private boolean start()
	{
		int thieving = Rs2Player.getRealSkillLevel(Skill.THIEVING);
		if (thieving <= 0)
		{
			// Right after login the stats are not in yet.
			return false;
		}
		boolean feudDone = Rs2Player.getQuestState(Quest.THE_FEUD) == QuestState.FINISHED;
		BlackjackTarget target = config.target() == BlackjackTarget.AUTO ? BlackjackTarget.best(thieving, feudDone) : config.target();
		if (thieving < target.thievingLevel)
		{
			stop(target + " needs " + target.thievingLevel + " Thieving (have " + thieving + ")");
			return false;
		}
		if (target == BlackjackTarget.MENAPHITE_THUG && !feudDone)
		{
			stop("Menaphite Thugs need The Feud completed");
			return false;
		}
		log.info("Blackjacker target: {} (Thieving {})", target, thieving);
		events.onHitpoints(Rs2Player.getBoostedSkillLevel(Skill.HITPOINTS), Rs2Player.getRealSkillLevel(Skill.HITPOINTS));
		events.onThieving(thieving, thievingXp());
		startXp = events.thievingXp();
		session = new Session(config, events, target);

		Healing healing = new Healing();
		LadderEscape ladder = new LadderEscape(healing);
		Blackjacking blackjacking = new Blackjacking();
		steps = Arrays.asList(
			// Out of a fight first.
			ladder,
			// A knock-out in flight or a target down: straight to the picks, ahead of healing. A
			// downed target is not hitting us; the drink comes once it is up, before the next
			// knock-out.
			blackjacking::burst,
			healing,
			new UnarmedReset(ladder),
			// A drink is still landing, or the client is too slow to fight safely: wait.
			s -> s.lowHp() && Healing.hasWine(),
			BlackjackerScript::paused,
			// Upkeep.
			this::autoRetaliateOff,
			blackjacking::wield,
			new Restock(),
			blackjacking::openPouches,
			// Nobody else in the room, a target in it, then the thieving.
			new Witness(),
			new BackCurtain(),
			new Lure(),
			new EnterRoom(),
			blackjacking::strike);
		return true;
	}

	/** The configured stop conditions; like every stop, it waits until nothing is hitting us. */
	private void checkLimits(Session s)
	{
		int level = config.stopAtLevel();
		if (level > 0 && events.thievingLevel() >= level)
		{
			s.stop("Reached Thieving " + events.thievingLevel());
		}
		int minutes = config.stopAfterMinutes();
		if (minutes > 0 && s.now - s.startedAt >= minutes * 60_000L)
		{
			s.stop("Ran for " + minutes + " minutes");
		}
	}

	/**
	 * Step: auto retaliate off. On, the hit after a glance makes us attack the target back: a
	 * real fight, Attack clicks on it while it is down, and knock-outs refused "during combat".
	 * Checked every few seconds; the setting is a varp read.
	 */
	private boolean autoRetaliateOff(Session s)
	{
		if (s.now - retaliateCheckedAt < 5000)
		{
			return false;
		}
		retaliateCheckedAt = s.now;
		if (Microbot.getVarbitPlayerValue(VarPlayerID.OPTION_NODEF) == 1)
		{
			return false;
		}
		Microbot.status = "Blackjacker: turning auto retaliate off";
		log.info("Blackjacker: auto retaliate is on, turning it off");
		if (!Rs2Combat.setAutoRetaliate(false))
		{
			log.warn("Blackjacker: could not turn auto retaliate off");
		}
		Rs2Tab.switchTo(InterfaceTab.INVENTORY);
		return true;
	}

	private static boolean paused(Session s)
	{
		if (s.throttled)
		{
			Microbot.status = "Blackjacker: paused, the client runs at " + Microbot.getClient().getFPS() + " FPS (window hidden or minimized?)";
		}
		return s.throttled;
	}

	// ----- stopping and reporting ---------------------------------------------------------------

	private void stop(String reason)
	{
		Microbot.status = "Blackjacker stopped: " + reason;
		log.info("Blackjacker stopped: {}", reason);
		report(true, reason);
		if (testResult != null)
		{
			finishTest(reason);
			return;
		}
		shutdown();
	}

	private void report(boolean force)
	{
		report(force, session == null ? null : session.stopReason);
	}

	private void report(boolean force, String stopReason)
	{
		long now = System.currentTimeMillis();
		if (!force && now - lastReport < 5000)
		{
			return;
		}
		lastReport = now;
		Session s = session;
		Map<String, Object> progress = new LinkedHashMap<>();
		progress.put("target", s == null ? null : s.target.name());
		progress.put("thievingXpGained", xpGained());
		if (s != null)
		{
			progress.putAll(s.counters());
			progress.put("wines", Healing.wines());
			progress.put("hp", s.hp());
			progress.put("drinkAt", s.drinkAt());
		}
		progress.put("stopReason", stopReason);
		ScriptResultStore.submit(BlackjackerPlugin.class.getName(), progress);

		if (testResult != null && s != null && s.stopReason == null && s.knockouts >= config.testCycles())
		{
			s.stop("Test: " + s.knockouts + " knock-outs");
		}
	}

	// Test mode protocol (docs/AGENTIC_TESTING_LOOP.md): record checks, write result.json, exit.
	private void finishTest(String reason)
	{
		Session s = session;
		int knockouts = s == null ? 0 : s.knockouts;
		int picks = s == null ? 0 : s.picks;
		int xp = xpGained();
		check("knock-outs: " + knockouts + "/" + config.testCycles(), knockouts >= config.testCycles(), reason);
		check("successful pickpockets: " + picks, picks > 0, "no successful pickpocket");
		check("thieving xp gained: " + xp, xp > 0, "no Thieving XP gained");
		testResult.complete("completed");
		TestResultWriter.write(testResult);
		log.info("Blackjacker test finished: {}/{} checks passed", testResult.passedCount(), testResult.checks.size());
		System.exit(testResult.exitCode);
	}

	private void check(String name, boolean passed, String failure)
	{
		testResult.addCheck(name, passed, passed ? null : failure);
	}

	private static int thievingXp()
	{
		return Microbot.getClientThread()
			.runOnClientThreadOptional(() -> Microbot.getClient().getSkillExperience(Skill.THIEVING))
			.orElse(0);
	}
}
