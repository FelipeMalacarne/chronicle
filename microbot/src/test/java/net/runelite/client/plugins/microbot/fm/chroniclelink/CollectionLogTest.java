package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.allUnavailable;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.map;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.vars;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.time.Instant;
import java.util.Arrays;
import java.util.Collections;
import java.util.Map;
import java.util.TreeMap;
import net.runelite.api.ScriptID;
import net.runelite.api.gameval.InterfaceID;
import org.junit.jupiter.api.Test;

/** data.collectionLog (planner docs/06, adendo 2026-11-13): reading the open log and sending its pages. */
class CollectionLogTest
{
	private static final Gson GSON = new Gson();
	private static final Instant SENT = T0.plusSeconds(60);

	private static CollectionLogReader.Slot slot(int itemId, int opacity, int quantity)
	{
		return new CollectionLogReader.Slot(itemId, opacity, quantity);
	}

	private static PlayerSnapshot at(long attack, Map<String, CollectionLogPage> pages)
	{
		return new PlayerSnapshot("Iron Test", "STANDARD", "IRONMAN", 7, vars("1777", 1), vars(),
			vars("attack", attack), vars("attack", attack), vars("attack", attack), null, null, null, pages, allUnavailable(T0));
	}

	private static Map<String, CollectionLogPage> pages(Object... titleAndPage)
	{
		Map<String, CollectionLogPage> out = new TreeMap<>();
		for (int i = 0; i < titleAndPage.length; i += 2)
		{
			out.put((String) titleAndPage[i], (CollectionLogPage) titleAndPage[i + 1]);
		}
		return out;
	}

	@Test
	void theIdsAreThePinnedClientsOwn()
	{
		// The values ChatCommandsPlugin of the pinned client (2.6.52) reads the "All Pets" page with.
		assertEquals(2731, ScriptID.COLLECTION_DRAW_LIST);
		assertEquals(40697876, InterfaceID.Collection.HEADER_TEXT);
		assertEquals(40697893, InterfaceID.Collection.ITEMS_CONTENTS);
		// And its adventure log: groups 947 and 187, titles 947:3 and 187:0.
		assertTrue(CollectionLogReader.adventureLogGroup(947));
		assertTrue(CollectionLogReader.adventureLogGroup(187));
		assertFalse(CollectionLogReader.adventureLogGroup(621));
		assertEquals(Arrays.asList(62062595, 12255232),
			Arrays.asList(CollectionLogReader.ADVENTURE_LOG_TITLES[0], CollectionLogReader.ADVENTURE_LOG_TITLES[1]));
	}

	@Test
	void titleIsTheHeaderWithoutTags()
	{
		assertEquals("Barrows Chests", CollectionLogReader.title("<col=ff981f>Barrows Chests</col>"));
		assertEquals("All Pets", CollectionLogReader.title(" All Pets "));
		assertNull(CollectionLogReader.title(null));
		assertNull(CollectionLogReader.title("<col=ff981f></col>"));
		assertNull(CollectionLogReader.title(new String(new char[81]).replace('\0', 'x')));
		assertEquals(80, CollectionLogReader.title(new String(new char[80]).replace('\0', 'x')).length());
	}

	@Test
	void slotsAreObtainedOnlyWhenDrawnOpaque()
	{
		Map<String, Long> got = CollectionLogReader.slots(Arrays.asList(
			slot(4708, 0, 1),     // obtained once
			slot(4712, 0, 3),     // obtained three times
			slot(4710, 175, 0),   // not obtained: drawn faded
			slot(4714, 0, 0),     // obtained, no number shown: at least 1
			slot(-1, 0, 1),       // no item in the slot
			slot(0, 0, 1),
			slot(4708, 175, 0))); // the same item again keeps the most
		assertEquals(map("4708", 1, "4710", 0, "4712", 3, "4714", 1), got);
		assertTrue(CollectionLogReader.slots(null).isEmpty());
	}

	@Test
	void tabIsTheOnlyVisibleList()
	{
		assertEquals("BOSSES", CollectionLogReader.tab(new boolean[]{true, false, false, false, false}));
		assertEquals("OTHER", CollectionLogReader.tab(new boolean[]{false, false, false, false, true}));
		assertEquals("MINIGAMES", CollectionLogReader.tab(new boolean[]{false, false, false, true, false}));
		// None or two visible: not known, never guessed.
		assertNull(CollectionLogReader.tab(new boolean[]{false, false, false, false, false}));
		assertNull(CollectionLogReader.tab(new boolean[]{true, true, false, false, false}));
	}

	@Test
	void adventureLogOwnerAndNames()
	{
		assertEquals("Iron Test", CollectionLogReader.adventureLogOwner("<col=ff981f>The Exploits of Iron Test</col>"));
		assertNull(CollectionLogReader.adventureLogOwner("Spirit Tree Locations"));
		assertNull(CollectionLogReader.adventureLogOwner(null));
		assertTrue(CollectionLogReader.sameName("Iron Test", "iron test"));
		assertFalse(CollectionLogReader.sameName("Iron Test", "Other Iron"));
		assertFalse(CollectionLogReader.sameName(null, "Iron Test"));
	}

	@Test
	void trackerKeepsTheReadsOfThisLoginAndRefusesAnotherPlayersLog()
	{
		CollectionLogTracker t = new CollectionLogTracker();
		assertTrue(t.pages().isEmpty());
		assertTrue(t.record("Barrows Chests", "BOSSES", map("4708", 1, "4710", 0), T0));
		// Read again unchanged: kept with its first read, nothing new to send.
		assertFalse(t.record("Barrows Chests", "BOSSES", map("4708", 1, "4710", 0), T0.plusSeconds(30)));
		assertEquals(T0, t.pages().get("Barrows Chests").observedAt);
		// A slot obtained since: a new read.
		assertTrue(t.record("Barrows Chests", "BOSSES", map("4708", 1, "4710", 1), T0.plusSeconds(60)));
		assertEquals(T0.plusSeconds(60), t.pages().get("Barrows Chests").observedAt);
		// The tab learnt later is a change too.
		assertTrue(t.record("All Pets", null, map("12646", 0), T0));
		assertTrue(t.record("All Pets", "OTHER", map("12646", 0), T0.plusSeconds(1)));
		assertEquals(Arrays.asList("All Pets", "Barrows Chests"), Arrays.asList(t.pages().keySet().toArray()));

		// Whose log: this character's unless an adventure log is open for someone else.
		assertFalse(t.othersLog("Iron Test"));
		t.onAdventureLogLoaded();
		assertTrue(t.othersLog("Iron Test"));   // owner not read yet
		t.onAdventureLogOwner("Other Iron");
		assertTrue(t.othersLog("Iron Test"));
		t.onAdventureLogLoaded();
		t.onAdventureLogOwner(null);            // a menu without a title keeps the owner
		assertTrue(t.othersLog("Iron Test"));
		t.onAdventureLogLoaded();
		t.onAdventureLogOwner("Iron Test"); // the player's own adventure log
		assertFalse(t.othersLog("Iron Test"));
		t.onAdventureLogOwner("Other Iron");
		t.onRegionChange();                     // left the house
		assertFalse(t.othersLog("Iron Test"));

		t.reset();
		assertTrue(t.pages().isEmpty());
	}

	@Test
	void pagesGoOutOnlyWhenTheManifestTakesThem()
	{
		CollectionLogTracker t = new CollectionLogTracker();
		t.record("Barrows Chests", "BOSSES", map("4708", 1), T0);
		IngestContract.Manifest old = GSON.fromJson("{\"contract_version\":1,\"manifest_version\":7}", IngestContract.Manifest.class);
		assertNull(StateCollector.readCollectionLog(old, t));
		assertEquals(CaptureTrigger.Strength.NONE, EventFilter.of(old).collectionLog());

		IngestContract.Manifest m = GSON.fromJson("{\"contract_version\":1,\"manifest_version\":7,\"collection_log\":true}", IngestContract.Manifest.class);
		assertTrue(m.collectionLogEnabled());
		assertEquals(1, StateCollector.readCollectionLog(m, t).size());
		assertEquals(CaptureTrigger.Strength.HARD, EventFilter.of(m).collectionLog());
	}

	@Test
	void fullCarriesEveryPageReadAndClampsItsTime()
	{
		Map<String, CollectionLogPage> read = pages(
			"Barrows Chests", new CollectionLogPage("BOSSES", T0, map("4708", 1, "4710", 0)),
			"All Pets", new CollectionLogPage(null, SENT.plusSeconds(5), map("12646", 0)));
		String json = GSON.toJson(PushBuilder.full(at(40, read), 1, SENT).data);
		assertTrue(json.contains("\"collectionLog\":{"
			+ "\"All Pets\":{\"observed_at\":\"2026-11-04T12:01:00Z\",\"items\":{\"12646\":0}},"
			+ "\"Barrows Chests\":{\"tab\":\"BOSSES\",\"observed_at\":\"2026-11-04T12:00:00Z\",\"items\":{\"4708\":1,\"4710\":0}}}"), json);
		// Nothing read, or not collected: the field stays out.
		assertFalse(GSON.toJson(PushBuilder.full(at(40, Collections.emptyMap()), 1, SENT).data).contains("collectionLog"));
		assertFalse(GSON.toJson(PushBuilder.full(at(40, null), 1, SENT).data).contains("collectionLog"));
	}

	@Test
	void deltaCarriesTheNewReadsAndAPageNeverAsksForAFull()
	{
		CollectionLogPage barrows = new CollectionLogPage("BOSSES", T0, map("4708", 1, "4710", 0));
		PlayerSnapshot acked = at(40, pages("Barrows Chests", barrows));
		assertNull(PushBuilder.delta(acked, at(40, pages("Barrows Chests", barrows)), 2, SENT));

		// A page opened for the first time: a delta with that page alone.
		CollectionLogPage pets = new CollectionLogPage("OTHER", T0.plusSeconds(10), map("12646", 0));
		PlayerSnapshot more = at(40, pages("Barrows Chests", barrows, "All Pets", pets));
		assertFalse(PushBuilder.requiresFull(acked, more));
		IngestContract.Push push = PushBuilder.delta(acked, more, 2, SENT);
		assertEquals(Collections.singleton("All Pets"), push.data.collectionLog.keySet());
		assertNull(push.data.varb);

		// A page read again with a new slot: that page.
		CollectionLogPage staff = new CollectionLogPage("BOSSES", T0.plusSeconds(20), map("4708", 1, "4710", 1));
		push = PushBuilder.delta(more, at(40, pages("Barrows Chests", staff, "All Pets", pets)), 3, SENT);
		assertEquals(Long.valueOf(1), push.data.collectionLog.get("Barrows Chests").items.get("4710"));
		assertEquals(1, push.data.collectionLog.size());

		// The first pages after an acked snapshot without any go out whole.
		push = PushBuilder.delta(at(40, null), more, 4, SENT);
		assertEquals(2, push.data.collectionLog.size());
		// And other changes leave the pages out when they did not change.
		assertNull(PushBuilder.delta(more, at(41, pages("Barrows Chests", barrows, "All Pets", pets)), 5, SENT).data.collectionLog);
	}
}
