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
import org.junit.jupiter.api.Test;

/** data.minigame (planner docs/06, adendo 2026-11-16). */
class MinigameTest
{
	private static final Gson GSON = new Gson();
	private static final Instant SENT = T0.plusSeconds(5);

	private static PlayerSnapshot at(Map<String, Long> counters)
	{
		return new PlayerSnapshot("Iron Test", "STANDARD", "IRONMAN", 7, vars("1777", 1), vars(),
			vars("attack", 40), vars("attack", 40), vars("attack", 40), null, allUnavailable(T0))
			.withMinigame(counters);
	}

	private static Map<String, Long> counters(long graveyard)
	{
		Map<String, Long> m = new TreeMap<>();
		m.put("mta.bones_to_peaches", 1L);
		m.put("mta.graveyard", graveyard);
		m.put("mta.telekinetic", 204L);
		return m;
	}

	@Test
	void readsTheLastNumberOfAPointsWidget()
	{
		assertEquals(1342, MinigameCounters.lastNumber("Enchantment: <col=ffffff>1,342</col>"));
		assertEquals(204, MinigameCounters.lastNumber("204"));
		assertEquals(-1, MinigameCounters.lastNumber("Points:"));
		assertEquals(-1, MinigameCounters.lastNumber(null));
	}

	@Test
	void sentOnlyWhenTheManifestTakesThem()
	{
		IngestContract.Manifest m = Fixtures.manifest(7);
		assertNull(new MinigameCounters().forIngest(m));
		m.minigame = true;
		assertEquals(new TreeMap<String, Long>(), new MinigameCounters().forIngest(m));
	}

	@Test
	void fullCarriesTheCountersAndOmitsThemWhenNotCollected()
	{
		String json = GSON.toJson(PushBuilder.full(at(counters(152)), 1, SENT).data);
		assertTrue(json.contains("\"minigame\":{\"mta.bones_to_peaches\":1,\"mta.graveyard\":152,\"mta.telekinetic\":204}"), json);
		assertFalse(GSON.toJson(PushBuilder.full(at(null), 1, SENT).data).contains("minigame"));
	}

	@Test
	void deltaCarriesTheCountersThatChangedAndANewOneNeedsNoFull()
	{
		PlayerSnapshot acked = at(counters(152));
		assertNull(PushBuilder.delta(acked, at(counters(152)), 2, SENT));
		IngestContract.Push push = PushBuilder.delta(acked, at(counters(170)), 2, SENT);
		assertEquals(1, push.data.minigame.size());
		assertEquals(Long.valueOf(170), push.data.minigame.get("mta.graveyard"));
		Map<String, Long> more = counters(152);
		more.put("mta.alchemist", 140L);
		assertFalse(PushBuilder.requiresFull(acked, at(more)));
		assertEquals(Long.valueOf(140), PushBuilder.delta(acked, at(more), 2, SENT).data.minigame.get("mta.alchemist"));
	}
}
