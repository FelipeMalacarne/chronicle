package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static net.runelite.client.plugins.microbot.fm.chroniclelink.CaptureTrigger.Strength.HARD;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.CaptureTrigger.Strength.NONE;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.CaptureTrigger.Strength.SOFT;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.ack;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.manifest;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.simple;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.lang.reflect.Proxy;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import net.runelite.api.Client;
import net.runelite.api.GameState;
import net.runelite.api.Skill;
import net.runelite.api.gameval.InventoryID;
import net.runelite.api.gameval.VarPlayerID;
import org.junit.jupiter.api.Test;

class CaptureTriggerTest
{
	private static final String WRITE = IngestContract.SCOPE_STATE_WRITE;
	/** The plugin's sync-thread period. */
	private static final long STEP_MS = 250;

	/** The plugin's sync loop on a fake clock: trigger, engine, capture, push, answer. */
	private static final class Sim
	{
		final SyncEngine engine = new SyncEngine();
		final CaptureTrigger trigger = new CaptureTrigger();
		final List<Long> captures = new ArrayList<>();
		final List<IngestContract.Push> pushes = new ArrayList<>();
		long ms;
		long level = 40;
		boolean autoAck = true;
		IngestContract.Push held;

		Sim()
		{
			engine.onManifest(manifest(7, WRITE), T0);
			step(); // the baseline full, acknowledged: next periodic check at 10000
			captures.clear();
			pushes.clear();
		}

		Instant now()
		{
			return T0.plusMillis(ms);
		}

		void event(CaptureTrigger.Strength strength)
		{
			trigger.onEvent(strength, now());
		}

		/** One sync tick at the current time; every capture finds something new to send. */
		void step()
		{
			if (trigger.next(engine, now()) != SyncEngine.Action.CAPTURE)
			{
				return;
			}
			captures.add(ms);
			IngestContract.Push push = engine.prepare(simple(7, ++level), now());
			assertNotNull(push);
			pushes.add(push);
			if (autoAck)
			{
				engine.onPushResult(200, ack(push.clientSeq, 7), now());
			}
			else
			{
				held = push;
			}
		}

		/** Ticks until {@code untilMs}, firing {@code strength} every {@code everyMs} (0: none). */
		void run(long untilMs, long everyMs, CaptureTrigger.Strength strength)
		{
			while (ms < untilMs)
			{
				ms += STEP_MS;
				if (everyMs > 0 && ms % everyMs == 0)
				{
					event(strength);
				}
				step();
			}
		}

		void runQuiet(long untilMs)
		{
			run(untilMs, 0, NONE);
		}
	}

	@Test
	void timingsAreTheOwnersDecision()
	{
		assertEquals(1_000, CaptureTrigger.DEBOUNCE.toMillis());
		assertEquals(3_000, CaptureTrigger.MAX_WAIT.toMillis());
	}

	@Test
	void aBurstOfEventsIsOneCaptureADebounceAfterTheLast()
	{
		Sim sim = new Sim();
		sim.runQuiet(2_000);
		for (int i = 0; i < 5; i++)
		{
			sim.event(HARD);
			sim.run(sim.ms + 250, 0, NONE);
		}
		// events at 2000..3000, so the capture is one debounce after the last one
		sim.runQuiet(9_750);
		assertEquals(Arrays.asList(4_000L), sim.captures);
		assertEquals("delta", sim.pushes.get(0).kind);
	}

	@Test
	void aContinuousStreamCapturesAtTheMaxWaitCap()
	{
		Sim sim = new Sim();
		sim.runQuiet(1_000);
		sim.run(9_000, 500, HARD);
		// first event at 1500 -> cap at 4500; next stream starts at 5000 -> 8000
		assertEquals(Arrays.asList(4_500L, 8_000L), sim.captures);
	}

	@Test
	void aSoftStreamWaitsForThePeriodicCaptureButASettledOneGoesOut()
	{
		Sim sim = new Sim();
		sim.run(25_000, 500, SOFT);
		assertEquals(Arrays.asList(10_000L, 20_000L), sim.captures, "churning vars keep the periodic cadence");

		sim.captures.clear();
		sim.runQuiet(27_000);
		assertEquals(Arrays.asList(26_000L), sim.captures, "the stream stopped at 25000");
	}

	@Test
	void aHardEventIsCappedEvenWhenSoftEventsKeepComing()
	{
		Sim sim = new Sim();
		sim.runQuiet(1_000);
		sim.event(HARD);
		sim.run(5_000, 500, SOFT);
		assertEquals(Arrays.asList(4_000L), sim.captures);
	}

	@Test
	void eventsWhileAPushIsInFlightBecomeExactlyOneFollowUpAfterTheAck()
	{
		Sim sim = new Sim();
		sim.autoAck = false;
		sim.runQuiet(10_000);
		assertEquals(Arrays.asList(10_000L), sim.captures, "periodic capture, its push still unanswered");

		sim.run(14_000, 250, HARD);
		assertEquals(1, sim.captures.size(), "nothing else while the push is in flight");
		assertTrue(sim.trigger.pending(), "the requests are kept, not dropped");

		sim.engine.onPushResult(200, ack(sim.held.clientSeq, 7), sim.now());
		sim.autoAck = true;
		sim.runQuiet(23_000);
		assertEquals(Arrays.asList(10_000L, 14_250L), sim.captures, "one follow-up, then the periodic one");
		assertEquals(3, sim.pushes.get(1).clientSeq);
		assertFalse(sim.trigger.pending());
	}

	@Test
	void eventsDoNotCutABackoffShortAndTheRetryKeepsItsSeq()
	{
		Sim sim = new Sim();
		sim.autoAck = false;
		sim.runQuiet(10_000);
		long seq = sim.held.clientSeq;
		sim.engine.onPushError(sim.now());

		sim.autoAck = true;
		sim.run(14_750, 250, HARD);
		assertEquals(1, sim.captures.size(), "backoff holds the requested capture back");
		sim.runQuiet(15_000);
		assertEquals(Arrays.asList(10_000L, 15_000L), sim.captures);
		assertEquals(seq, sim.pushes.get(1).clientSeq);
	}

	@Test
	void thePeriodicCaptureIsSkippedAfterARecentEventCapture()
	{
		Sim sim = new Sim();
		sim.runQuiet(3_000);
		sim.event(HARD);
		sim.runQuiet(13_750);
		assertEquals(Arrays.asList(4_000L), sim.captures, "no capture at the old 10000 check");
		sim.runQuiet(14_000);
		assertEquals(Arrays.asList(4_000L, 14_000L), sim.captures, "one interval after the event capture");
	}

	@Test
	void anIgnoredEventNeverCaptures()
	{
		Sim sim = new Sim();
		sim.run(9_750, 250, NONE);
		sim.event(NONE);
		assertFalse(sim.trigger.pending());
		assertTrue(sim.captures.isEmpty());
	}

	@Test
	void onlyCollectedContainersTrigger()
	{
		Toggles on = new Toggles(true, true, true);
		EventFilter scoped = EventFilter.of(manifest(7, WRITE, IngestContract.SCOPE_INVENTORY));
		assertEquals(HARD, scoped.container(InventoryID.INV, on));
		assertEquals(NONE, scoped.container(InventoryID.BANK, on), "no items:bank scope");
		assertEquals(NONE, scoped.container(InventoryID.WORN, on), "no items:equipment scope");
		assertEquals(NONE, scoped.container(InventoryID.INV, new Toggles(true, false, true)), "toggle off");
		assertEquals(NONE, scoped.container(InventoryID.BANK + 1000, on), "not a container the plugin reads");

		EventFilter all = EventFilter.of(manifest(7, WRITE, IngestContract.SCOPE_BANK,
			IngestContract.SCOPE_INVENTORY, IngestContract.SCOPE_EQUIPMENT));
		assertEquals(HARD, all.container(InventoryID.BANK, on));
		assertEquals(HARD, all.container(InventoryID.WORN, on));
		assertEquals(NONE, all.container(InventoryID.WORN, new Toggles(true, true, false)));

		IngestContract.Manifest flagOff = manifest(7, WRITE, IngestContract.SCOPE_INVENTORY);
		flagOff.items.inventory = false;
		assertEquals(NONE, EventFilter.of(flagOff).container(InventoryID.INV, on), "manifest flag off");
		assertNull(EventFilter.of(null), "no manifest: every event is ignored");
	}

	@Test
	void varsAreHardWhenListedOrQuestPointsAndSoftOnlyForQuests()
	{
		IngestContract.Manifest m = manifest(7, WRITE);
		m.varps = Arrays.asList(1234);
		EventFilter noQuests = EventFilter.of(m);
		assertEquals(HARD, noQuests.var(4000, 1777), "listed varbit");
		assertEquals(HARD, noQuests.var(1234, -1), "listed varp");
		assertEquals(NONE, noQuests.var(4000, 9999));
		assertEquals(NONE, noQuests.var(4000, -1));
		assertEquals(NONE, noQuests.var(VarPlayerID.QP, -1), "quest points matter only with quests");

		m.quests = true;
		EventFilter quests = EventFilter.of(m);
		assertEquals(HARD, quests.var(VarPlayerID.QP, -1), "a finished quest");
		assertEquals(SOFT, quests.var(4000, 9999), "a quest's own progress var");
		assertEquals(SOFT, quests.var(4000, -1));
		assertEquals(HARD, quests.var(4000, 1777));
	}

	@Test
	void levelUpsAreHardAndOtherStatChangesSoft()
	{
		EventFilter skills = EventFilter.of(manifest(7, WRITE));
		assertEquals(HARD, skills.stat(true));
		assertEquals(SOFT, skills.stat(false));
		IngestContract.Manifest off = manifest(7, WRITE);
		off.skills = false;
		assertEquals(NONE, EventFilter.of(off).stat(true));

		StateCollector collector = new StateCollector(client(GameState.LOGGED_IN));
		collector.startEpoch();
		assertFalse(collector.onStatChanged(Skill.ATTACK, 40), "the first value is not a level-up");
		assertFalse(collector.onStatChanged(Skill.ATTACK, 40), "experience only");
		assertTrue(collector.onStatChanged(Skill.ATTACK, 41));
		collector.startEpoch();
		assertFalse(collector.onStatChanged(Skill.ATTACK, 50), "a new login forgets the levels");
	}

	@Test
	void eventsWaitForThePostLoginTicks()
	{
		StateCollector collector = new StateCollector(client(GameState.LOGGED_IN));
		assertFalse(collector.readyForEvents(), "logged out");
		collector.startEpoch();
		for (int tick = 1; tick < StateCollector.SETTLE_TICKS; tick++)
		{
			assertFalse(collector.onGameTick());
			assertFalse(collector.readyForEvents(), "tick " + tick);
		}
		assertTrue(collector.onGameTick(), "the wait ends: one capture request");
		assertTrue(collector.readyForEvents());
		assertFalse(collector.onGameTick(), "only once");

		collector.onGameStateChanged(GameState.LOADING);
		assertTrue(collector.readyForEvents(), "a region load keeps the vars");
		collector.onGameStateChanged(GameState.HOPPING);
		assertFalse(collector.readyForEvents(), "a hop waits again");
		for (int tick = 1; tick <= StateCollector.SETTLE_TICKS; tick++)
		{
			collector.onGameTick();
		}
		assertTrue(collector.readyForEvents());
		collector.endEpoch();
		assertFalse(collector.readyForEvents());
		assertFalse(collector.onGameTick(), "no settle request while logged out");
	}

	@Test
	void eventsAreReadyOnlyWhileLoggedIn()
	{
		StateCollector collector = new StateCollector(client(GameState.LOADING));
		collector.startEpoch();
		for (int tick = 1; tick <= StateCollector.SETTLE_TICKS; tick++)
		{
			collector.onGameTick();
		}
		assertFalse(collector.readyForEvents());
	}

	@Test
	void loginOrLogoutDropsWhatWasPending()
	{
		CaptureTrigger trigger = new CaptureTrigger();
		trigger.onEvent(HARD, T0);
		trigger.clear();
		assertFalse(trigger.due(T0.plusSeconds(5)));
	}

	/** A Client whose only answer is the game state. */
	private static Client client(GameState state)
	{
		return (Client) Proxy.newProxyInstance(Client.class.getClassLoader(), new Class<?>[]{Client.class},
			(proxy, method, args) ->
			{
				if ("getGameState".equals(method.getName()))
				{
					return state;
				}
				throw new UnsupportedOperationException(method.getName());
			});
	}

	private static final class Toggles implements ChronicleLinkConfig
	{
		private final boolean bank;
		private final boolean inventory;
		private final boolean equipment;

		Toggles(boolean bank, boolean inventory, boolean equipment)
		{
			this.bank = bank;
			this.inventory = inventory;
			this.equipment = equipment;
		}

		@Override
		public boolean sendBank()
		{
			return bank;
		}

		@Override
		public boolean sendInventory()
		{
			return inventory;
		}

		@Override
		public boolean sendEquipment()
		{
			return equipment;
		}
	}
}
