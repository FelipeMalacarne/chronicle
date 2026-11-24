package net.runelite.client.plugins.microbot.fm.chroniclelink;

/**
 * Membership for {@code data.membership} (planner 2026-11-24): the days left (varp
 * {@code ACCOUNT_CREDIT}) and the member flag (varc {@code PLAYERMEMBER}). A member can have 0 days
 * left, so the account is a member when either says so. Both read 0 until the server sends them
 * after login, which looks like a free-to-play account, so nothing is reported before
 * {@link #SETTLE_TICKS} game ticks logged in with the local player present, longer than the wait of
 * the other facets because this one has no way to tell "not sent yet" from "not a member".
 */
final class MembershipReading
{
	/** Game ticks logged in (about six seconds) before the reading is trusted. */
	static final int SETTLE_TICKS = 10;
	/** The planner's bound for {@code days}; a reading above it is clamped, never sent as is. */
	static final int MAX_DAYS = 36_500;

	private MembershipReading()
	{
	}

	/** The reading, or {@code null} while the login has not settled. */
	static IngestContract.Membership read(int creditDays, int memberVarc, int ticksLoggedIn, boolean playerPresent)
	{
		if (!playerPresent || ticksLoggedIn < SETTLE_TICKS)
		{
			return null;
		}
		int days = Math.max(0, Math.min(creditDays, MAX_DAYS));
		return new IngestContract.Membership(days, days > 0 || memberVarc == 1);
	}
}
