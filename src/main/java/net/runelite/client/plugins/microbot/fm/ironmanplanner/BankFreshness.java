package net.runelite.client.plugins.microbot.fm.ironmanplanner;

/**
 * Decides how the bank mirror may be described. Microbot restores the profile's last bank
 * snapshot at login; that mirror is {@code restored} until this login sees a BANK container
 * event and the {@code Rs2Bank} cache has applied a live epoch, and only then {@code live}.
 */
final class BankFreshness
{
	private BankFreshness()
	{
	}

	/**
	 * @param allowed            toggle on and the manifest/token grant {@code items:bank}
	 * @param bankEventThisLogin a BANK {@code ItemContainerChanged} was seen in the current live epoch
	 * @param cacheLiveEpoch     {@code Rs2Bank.getBankLiveEpoch()}; zero after a restore or invalidation
	 * @param hasMirror          {@code Rs2Bank.hasBankMirrorSnapshot()}
	 * @param lastOpenedAtMillis {@code Rs2Bank.getBankLastOpenedAt()}; zero when unknown
	 */
	static ContainerSnapshot.State classify(boolean allowed, boolean bankEventThisLogin, int cacheLiveEpoch,
		boolean hasMirror, long lastOpenedAtMillis)
	{
		if (!allowed)
		{
			return ContainerSnapshot.State.UNAVAILABLE;
		}
		if (bankEventThisLogin && cacheLiveEpoch > 0)
		{
			return ContainerSnapshot.State.LIVE;
		}
		// without the capture time the mirror cannot carry an honest observed_at
		if (hasMirror && lastOpenedAtMillis > 0)
		{
			return ContainerSnapshot.State.RESTORED;
		}
		return ContainerSnapshot.State.UNAVAILABLE;
	}
}
