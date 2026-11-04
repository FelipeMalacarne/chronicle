package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.EPOCH;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.map;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.vars;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import com.google.gson.JsonObject;
import java.time.Instant;
import java.util.EnumMap;
import java.util.Map;
import org.junit.jupiter.api.Test;

class PushBuilderTest
{
	private static final Gson GSON = new Gson();
	private static final Instant SENT = T0.plusSeconds(5);

	private static Map<ContainerKind, ContainerSnapshot> containers(ContainerSnapshot bank, ContainerSnapshot inv,
		ContainerSnapshot equip)
	{
		Map<ContainerKind, ContainerSnapshot> c = new EnumMap<>(ContainerKind.class);
		c.put(ContainerKind.BANK, bank);
		c.put(ContainerKind.INVENTORY, inv);
		c.put(ContainerKind.EQUIPMENT, equip);
		return c;
	}

	@Test
	void fullDescribesEveryFacetAndAllThreeContainers()
	{
		PlayerSnapshot s = Fixtures.snapshot(7, vars(), vars(), containers(
			ContainerSnapshot.restored(T0.minusSeconds(300), 41, map(4151, 1, 995, 250000)),
			ContainerSnapshot.live(T0, 3, EPOCH, map(995, 1200)),
			ContainerSnapshot.unavailable(T0)));

		JsonObject json = GSON.fromJson(GSON.toJson(PushBuilder.full(s, 1, SENT)), JsonObject.class);
		assertEquals(1, json.get("contract_version").getAsInt());
		assertEquals("full", json.get("kind").getAsString());
		assertEquals(1, json.get("client_seq").getAsLong());
		assertEquals(7, json.get("manifest_version").getAsLong());
		assertEquals("2026-11-04T12:00:05Z", json.get("sent_at").getAsString());

		JsonObject data = json.getAsJsonObject("data");
		for (String facet : new String[]{"varb", "varp", "level", "xp", "boosted"})
		{
			// present even when empty
			assertTrue(data.get(facet).isJsonObject(), facet);
		}

		JsonObject items = data.getAsJsonObject("items");
		JsonObject bank = items.getAsJsonObject("bank");
		assertEquals("restored", bank.get("state").getAsString());
		assertEquals(41, bank.get("revision").getAsLong());
		assertFalse(bank.has("live_epoch"));
		assertEquals(250000, bank.getAsJsonObject("quantities").get("995").getAsLong());

		JsonObject inv = items.getAsJsonObject("inventory");
		assertEquals("live", inv.get("state").getAsString());
		assertEquals(EPOCH, inv.get("live_epoch").getAsString());
		assertEquals(3, inv.get("revision").getAsLong());

		JsonObject equip = items.getAsJsonObject("equipment");
		assertEquals("unavailable", equip.get("state").getAsString());
		assertEquals(2, equip.size(), "unavailable carries only state and observed_at");
	}

	@Test
	void observedAtNeverExceedsSentAt()
	{
		ContainerSnapshot future = ContainerSnapshot.live(SENT.plusSeconds(1), 0, EPOCH, map());
		assertEquals(PushBuilder.timestamp(SENT), PushBuilder.observation(future, null, SENT).observedAt);
	}

	@Test
	void fullOfAnObservedEmptyContainerSendsAnEmptyMap()
	{
		IngestContract.ItemObservation obs = PushBuilder.observation(ContainerSnapshot.live(T0, 0, EPOCH, map()), null, SENT);
		assertNotNull(obs.quantities);
		assertTrue(obs.quantities.isEmpty());
	}

	@Test
	void deltaCarriesOnlyChangedFacetsAndContainers()
	{
		Map<ContainerKind, ContainerSnapshot> before = containers(ContainerSnapshot.unavailable(T0),
			ContainerSnapshot.live(T0, 0, EPOCH, map(995, 100, 4151, 1)), ContainerSnapshot.live(T0, 0, EPOCH, map(1163, 1)));
		Map<ContainerKind, ContainerSnapshot> after = containers(ContainerSnapshot.unavailable(T0.plusSeconds(10)),
			ContainerSnapshot.live(T0.plusSeconds(10), 1, EPOCH, map(995, 90)), ContainerSnapshot.live(T0.plusSeconds(10), 0, EPOCH, map(1163, 1)));
		PlayerSnapshot acked = Fixtures.snapshot(7, vars("1777", 1, "4000", 0), vars("attack", 40, "magic", 33), before);
		PlayerSnapshot current = Fixtures.snapshot(7, vars("1777", 1, "4000", 2), vars("attack", 41, "magic", 33), after);

		assertFalse(PushBuilder.requiresFull(acked, current));
		IngestContract.Push push = PushBuilder.delta(acked, current, 5, T0.plusSeconds(11));
		assertEquals("delta", push.kind);
		assertEquals(5, push.clientSeq);
		assertEquals(vars("4000", 2), push.data.varb);
		assertNull(push.data.varp);
		assertEquals(vars("attack", 41), push.data.level);
		assertNull(push.data.items.bank);
		assertNull(push.data.items.equipment);
		assertEquals("live", push.data.items.inventory.state);
		assertEquals(1, push.data.items.inventory.revision);
		assertEquals(map(995, 90, 4151, 0), push.data.items.inventory.quantities);

		JsonObject data = GSON.fromJson(GSON.toJson(push), JsonObject.class).getAsJsonObject("data");
		assertFalse(data.has("varp"), "unchanged facets are omitted, not sent empty");
		assertFalse(data.getAsJsonObject("items").has("bank"));
	}

	@Test
	void nothingChangedMeansNoDelta()
	{
		PlayerSnapshot a = Fixtures.simple(7, 40);
		assertNull(PushBuilder.delta(a, Fixtures.simple(7, 40), 2, SENT));
	}

	@Test
	void bankTurningLiveNeedsAFull()
	{
		Map<ContainerKind, ContainerSnapshot> restored = Fixtures.allUnavailable(T0);
		restored.put(ContainerKind.BANK, ContainerSnapshot.restored(T0, 41, map(995, 1)));
		Map<ContainerKind, ContainerSnapshot> live = Fixtures.allUnavailable(T0);
		live.put(ContainerKind.BANK, ContainerSnapshot.live(T0, 0, EPOCH, map(995, 1)));
		PlayerSnapshot acked = Fixtures.snapshot(7, vars(), vars(), restored);
		assertTrue(PushBuilder.requiresFull(acked, Fixtures.snapshot(7, vars(), vars(), live)));
	}

	@Test
	void baselineIsRequiredWithoutAckNewEpochOrNewManifest()
	{
		PlayerSnapshot a = Fixtures.simple(7, 40);
		assertTrue(PushBuilder.requiresFull(null, a));
		assertTrue(PushBuilder.requiresFull(a, Fixtures.simple(8, 40)));

		Map<ContainerKind, ContainerSnapshot> e1 = Fixtures.allUnavailable(T0);
		e1.put(ContainerKind.INVENTORY, ContainerSnapshot.live(T0, 4, EPOCH, map()));
		Map<ContainerKind, ContainerSnapshot> e2 = Fixtures.allUnavailable(T0);
		e2.put(ContainerKind.INVENTORY, ContainerSnapshot.live(T0, 0, "session-b", map()));
		assertTrue(PushBuilder.requiresFull(Fixtures.snapshot(7, vars(), vars(), e1), Fixtures.snapshot(7, vars(), vars(), e2)));
	}

	@Test
	void togglingAContainerOffNeedsAFull()
	{
		Map<ContainerKind, ContainerSnapshot> on = Fixtures.allUnavailable(T0);
		on.put(ContainerKind.EQUIPMENT, ContainerSnapshot.live(T0, 0, EPOCH, map(1163, 1)));
		assertTrue(PushBuilder.requiresFull(Fixtures.snapshot(7, vars(), vars(), on), Fixtures.simple(7, 40)));
	}
}
