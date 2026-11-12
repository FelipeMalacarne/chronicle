package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.allUnavailable;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.vars;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.time.Instant;
import java.util.Map;
import java.util.TreeMap;
import net.runelite.client.plugins.timetracking.SummaryState;
import org.junit.jupiter.api.Test;

/** data.timers and data.position (planner docs/06, adendo 2026-11-10). */
class TimersAndPositionTest
{
	private static final Gson GSON = new Gson();
	private static final Instant SENT = T0.plusSeconds(5);

	private static PlayerSnapshot at(long attack, Map<String, IngestContract.Timer> timers, IngestContract.Position pos)
	{
		return new PlayerSnapshot("Iron Test", "STANDARD", "IRONMAN", 7, vars("1777", 1), vars(),
			vars("attack", attack), vars("attack", attack), vars("attack", attack), null, timers, pos, allUnavailable(T0));
	}

	private static Map<String, IngestContract.Timer> clocks(SummaryState herb, long herbReady)
	{
		Map<String, IngestContract.Timer> m = new TreeMap<>();
		m.put("HERB", IngestContract.Timer.of("Herbs", herb, herbReady));
		m.put("BIRD_HOUSE", IngestContract.Timer.of("Bird Houses", SummaryState.EMPTY, -1));
		return m;
	}

	@Test
	void timerKeepsReadyAtOnlyWhileGrowingOrDone()
	{
		assertEquals(Long.valueOf(1762720000L), IngestContract.Timer.of("Herbs", SummaryState.IN_PROGRESS, 1762720000L).readyAt);
		assertEquals(Long.valueOf(1762720000L), IngestContract.Timer.of("Herbs", SummaryState.COMPLETED, 1762720000L).readyAt);
		assertNull(IngestContract.Timer.of("Herbs", SummaryState.EMPTY, 1762720000L).readyAt);
		assertNull(IngestContract.Timer.of("Herbs", SummaryState.IN_PROGRESS, 0).readyAt);
		assertEquals("UNKNOWN", IngestContract.Timer.of("Herbs", null, -1).state);
	}

	@Test
	void fullCarriesTheClocksAndThePosition()
	{
		PlayerSnapshot s = at(40, clocks(SummaryState.IN_PROGRESS, 1762720000L), new IngestContract.Position(330, 3222, 3218, 0));
		String json = GSON.toJson(PushBuilder.full(s, 1, SENT).data);
		assertTrue(json.contains("\"timers\":{\"BIRD_HOUSE\":{\"state\":\"EMPTY\",\"label\":\"Bird Houses\"},\"HERB\":{\"state\":\"IN_PROGRESS\",\"readyAt\":1762720000,\"label\":\"Herbs\"}}"), json);
		assertTrue(json.contains("\"position\":{\"world\":330,\"x\":3222,\"y\":3218,\"plane\":0}"), json);
		// Neither is sent when not collected.
		String bare = GSON.toJson(PushBuilder.full(at(40, null, null), 1, SENT).data);
		assertFalse(bare.contains("timers") || bare.contains("position"), bare);
	}

	@Test
	void deltaCarriesTheClocksThatChanged()
	{
		PlayerSnapshot acked = at(40, clocks(SummaryState.IN_PROGRESS, 1762720000L), null);
		assertNull(PushBuilder.delta(acked, at(40, clocks(SummaryState.IN_PROGRESS, 1762720000L), null), 2, SENT));
		IngestContract.Push push = PushBuilder.delta(acked, at(40, clocks(SummaryState.COMPLETED, 1762720000L), null), 2, SENT);
		assertEquals(1, push.data.timers.size());
		assertEquals("COMPLETED", push.data.timers.get("HERB").state);
		// A tab that appears or goes (the farming contract) needs a full.
		Map<String, IngestContract.Timer> more = clocks(SummaryState.IN_PROGRESS, 1762720000L);
		more.put("FARMING_CONTRACT", IngestContract.Timer.of("Farming contract: Ranarr", SummaryState.EMPTY, -1));
		assertTrue(PushBuilder.requiresFull(acked, at(40, more, null)));
	}

	@Test
	void positionAloneWaitsForARealMoveAndRidesAlongOtherwise()
	{
		IngestContract.Position here = new IngestContract.Position(330, 3222, 3218, 0);
		PlayerSnapshot acked = at(40, null, here);
		// A few tiles away and nothing else: no push.
		assertNull(PushBuilder.delta(acked, at(40, null, new IngestContract.Position(330, 3230, 3218, 0)), 2, SENT));
		// Far enough, another plane or another world: a push with the position only.
		IngestContract.Push far = PushBuilder.delta(acked, at(40, null, new IngestContract.Position(330, 3240, 3218, 0)), 2, SENT);
		assertEquals(3240, far.data.position.x);
		assertNull(far.data.varb);
		assertEquals(1, PushBuilder.delta(acked, at(40, null, new IngestContract.Position(330, 3222, 3218, 1)), 2, SENT).data.position.plane);
		assertEquals(301, PushBuilder.delta(acked, at(40, null, new IngestContract.Position(301, 3222, 3218, 0)), 2, SENT).data.position.world);
		// Any other change takes the small move along; the same tile stays out.
		assertEquals(3230, PushBuilder.delta(acked, at(41, null, new IngestContract.Position(330, 3230, 3218, 0)), 2, SENT).data.position.x);
		assertNull(PushBuilder.delta(acked, at(41, null, here), 2, SENT).data.position);
	}
}
