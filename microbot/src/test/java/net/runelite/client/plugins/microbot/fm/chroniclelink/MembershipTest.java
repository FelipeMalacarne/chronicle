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
import org.junit.jupiter.api.Test;

/** data.membership (planner docs/06, adendo 2026-11-24). */
class MembershipTest
{
	private static final Gson GSON = new Gson();
	private static final Instant SENT = T0.plusSeconds(5);
	private static final int SETTLED = MembershipReading.SETTLE_TICKS;

	private static PlayerSnapshot at(IngestContract.Membership membership)
	{
		return new PlayerSnapshot("Iron Test", "STANDARD", "IRONMAN", 7, vars("1777", 1), vars(),
			vars("attack", 40), vars("attack", 40), vars("attack", 40), null, allUnavailable(T0))
			.withMembership(membership);
	}

	@Test
	void nothingIsReportedBeforeTheLoginSettles()
	{
		assertNull(MembershipReading.read(120, 1, SETTLED - 1, true));
		assertNull(MembershipReading.read(120, 1, 0, true));
		assertNull(MembershipReading.read(120, 1, SETTLED, false));
	}

	@Test
	void aMemberHasDaysLeftOrTheFlag()
	{
		assertEquals(new IngestContract.Membership(120, true), MembershipReading.read(120, 0, SETTLED, true));
		assertEquals(new IngestContract.Membership(0, true), MembershipReading.read(0, 1, SETTLED, true));
		assertEquals(new IngestContract.Membership(0, false), MembershipReading.read(0, 0, SETTLED, true));
	}

	@Test
	void daysAreKeptInTheServersBounds()
	{
		assertEquals(0, MembershipReading.read(-5, 0, SETTLED, true).days);
		assertEquals(MembershipReading.MAX_DAYS, MembershipReading.read(Integer.MAX_VALUE, 0, SETTLED, true).days);
	}

	@Test
	void sentOnlyWhenTheManifestTakesIt()
	{
		IngestContract.Manifest m = Fixtures.manifest(7);
		assertFalse(m.membershipEnabled());
		m.membership = true;
		assertTrue(m.membershipEnabled());
	}

	@Test
	void fullCarriesItAndOmitsItWhenNotRead()
	{
		String json = GSON.toJson(PushBuilder.full(at(new IngestContract.Membership(120, true)), 1, SENT).data);
		assertTrue(json.contains("\"membership\":{\"days\":120,\"member\":true}"), json);
		assertFalse(GSON.toJson(PushBuilder.full(at(null), 1, SENT).data).contains("membership"));
	}

	@Test
	void deltaCarriesItWhenNewOrChangedAndNeedsNoFull()
	{
		PlayerSnapshot none = at(null);
		PlayerSnapshot acked = at(new IngestContract.Membership(120, true));
		assertNull(PushBuilder.delta(acked, at(new IngestContract.Membership(120, true)), 2, SENT));
		assertFalse(PushBuilder.requiresFull(none, acked));
		assertEquals(new IngestContract.Membership(120, true), PushBuilder.delta(none, acked, 2, SENT).data.membership);
		IngestContract.Push push = PushBuilder.delta(acked, at(new IngestContract.Membership(119, true)), 3, SENT);
		assertEquals(119, push.data.membership.days);
	}
}
