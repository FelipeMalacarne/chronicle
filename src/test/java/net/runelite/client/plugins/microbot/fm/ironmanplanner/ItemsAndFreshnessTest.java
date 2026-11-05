package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.EPOCH;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.map;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.util.Arrays;
import java.util.Collections;
import java.util.Map;
import org.junit.jupiter.api.Test;

class ItemsAndFreshnessTest
{
	@Test
	void stacksOfTheSameIdAddUpAndJunkIsSkipped()
	{
		Map<String, Long> totals = ItemQuantities.empty();
		ItemQuantities.add(totals, 995, 100, null);
		ItemQuantities.add(totals, 995, 50, null);
		ItemQuantities.add(totals, 4151, 1, null);
		ItemQuantities.add(totals, -1, 1, null);
		ItemQuantities.add(totals, 0, 1, null);
		ItemQuantities.add(totals, 1511, 0, null);
		assertEquals(map(995, 150, 4151, 1), totals);
	}

	@Test
	void onlyItemIdsLimitsTheKeys()
	{
		Map<String, Long> totals = ItemQuantities.empty();
		ItemQuantities.add(totals, 995, 100, Arrays.asList(4151));
		ItemQuantities.add(totals, 4151, 1, Arrays.asList(4151));
		assertEquals(map(4151, 1), totals);
	}

	@Test
	void absentOrEmptyOnlyItemIdsSendsTheWholeContainer()
	{
		Map<String, Long> absent = ItemQuantities.empty();
		ItemQuantities.add(absent, 995, 100, null);
		ItemQuantities.add(absent, 4151, 1, null);
		Map<String, Long> empty = ItemQuantities.empty();
		ItemQuantities.add(empty, 995, 100, Collections.emptyList());
		ItemQuantities.add(empty, 4151, 1, Collections.emptyList());
		assertEquals(map(995, 100, 4151, 1), absent);
		assertEquals(absent, empty);
	}

	@Test
	void manifestWithoutOnlyItemIdsParsesAsNoFilter()
	{
		IngestContract.Manifest m = new Gson().fromJson("{\"contract_version\":1,\"manifest_version\":3,"
			+ "\"token_scopes\":[\"state:write\",\"items:bank\"],\"varbits\":[],\"varps\":[101],"
			+ "\"collections\":[],\"items\":{\"bank\":true,\"inventory\":false,\"equipment\":false}}",
			IngestContract.Manifest.class);
		assertNull(m.items.onlyItemIds);
		Map<String, Long> totals = ItemQuantities.empty();
		ItemQuantities.add(totals, 995, 100, m.items.onlyItemIds);
		assertEquals(map(995, 100), totals);
	}

	@Test
	void patchCarriesNewTotalsAndZeroTombstones()
	{
		Map<String, Long> before = map(995, 100, 4151, 1, 1511, 27);
		Map<String, Long> after = map(995, 250, 1511, 27, 2434, 4);
		assertEquals(map(995, 250, 2434, 4, 4151, 0), ItemQuantities.patch(before, after));
		assertTrue(ItemQuantities.patch(after, after).isEmpty());
	}

	@Test
	void revisionStartsAtZeroPerEpochAndGrowsOnlyOnChange()
	{
		ContainerTracker tracker = new ContainerTracker();
		assertEquals(0, tracker.live(EPOCH, map(995, 1), T0).revision);
		assertEquals(0, tracker.live(EPOCH, map(995, 1), T0.plusSeconds(10)).revision);
		ContainerSnapshot changed = tracker.live(EPOCH, map(995, 2), T0.plusSeconds(20));
		assertEquals(1, changed.revision);
		assertEquals(EPOCH, changed.liveEpoch);
		assertEquals(T0.plusSeconds(20), changed.observedAt);
		assertEquals(0, tracker.live("session-b", map(995, 2), T0.plusSeconds(30)).revision);
	}

	@Test
	void liveObservationNeedsAnEpoch()
	{
		assertThrows(IllegalArgumentException.class, () -> ContainerSnapshot.live(T0, 0, " ", map()));
	}

	@Test
	void unavailableCarriesNoRevisionEpochOrQuantities()
	{
		ContainerSnapshot u = ContainerSnapshot.unavailable(T0);
		assertNull(u.quantities);
		assertNull(u.liveEpoch);
	}

	@Test
	void bankIsRestoredUntilALiveEventInThisLogin()
	{
		long openedAt = T0.toEpochMilli();
		// restored mirror at login: Rs2Bank epoch reset to 0, no BANK event yet
		assertEquals(ContainerSnapshot.State.RESTORED, BankFreshness.classify(true, false, 0, true, openedAt));
		// the cache saw a live epoch earlier in this client, but not in this login
		assertEquals(ContainerSnapshot.State.RESTORED, BankFreshness.classify(true, false, 3, true, openedAt));
		// event seen, cache not applied yet (or invalidated since)
		assertEquals(ContainerSnapshot.State.RESTORED, BankFreshness.classify(true, true, 0, true, openedAt));
		// a BANK container event in this login and an applied live epoch
		assertEquals(ContainerSnapshot.State.LIVE, BankFreshness.classify(true, true, 1, true, openedAt));
	}

	@Test
	void bankWithoutPermissionOrCaptureTimeIsUnavailable()
	{
		assertEquals(ContainerSnapshot.State.UNAVAILABLE, BankFreshness.classify(false, true, 5, true, 1L));
		assertEquals(ContainerSnapshot.State.UNAVAILABLE, BankFreshness.classify(true, false, 0, true, 0L));
		assertEquals(ContainerSnapshot.State.UNAVAILABLE, BankFreshness.classify(true, false, 0, false, 1L));
	}

	@Test
	void restoredOnlyMatchesTheSameMirror()
	{
		ContainerSnapshot a = ContainerSnapshot.restored(T0, 41, map(995, 1));
		assertTrue(a.deltaCompatible(ContainerSnapshot.restored(T0, 41, map(995, 1))));
		assertFalse(a.deltaCompatible(ContainerSnapshot.restored(T0, 42, map(995, 1))));
		assertFalse(a.deltaCompatible(ContainerSnapshot.live(T0, 0, EPOCH, map(995, 1))));
	}

	@Test
	void liveIsDeltaCompatibleWithinItsEpochOnly()
	{
		ContainerSnapshot a = ContainerSnapshot.live(T0, 2, EPOCH, map(995, 1));
		assertTrue(a.deltaCompatible(ContainerSnapshot.live(T0, 3, EPOCH, map(995, 2))));
		assertFalse(a.deltaCompatible(ContainerSnapshot.live(T0, 0, "session-b", map(995, 2))));
		assertFalse(a.deltaCompatible(ContainerSnapshot.unavailable(T0)));
	}

	@Test
	void accountTypeAndProfileMapping()
	{
		assertEquals("IRONMAN", StateCollector.accountType(1));
		assertEquals("HARDCORE_IRONMAN", StateCollector.accountType(3));
		assertEquals("UNRANKED_GROUP_IRONMAN", StateCollector.accountType(6));
		assertNull(StateCollector.accountType(42));
		assertEquals("STANDARD", StateCollector.profile(net.runelite.client.config.RuneScapeProfileType.STANDARD));
		assertEquals("OTHER", StateCollector.profile(net.runelite.client.config.RuneScapeProfileType.DEADMAN));
	}
}
