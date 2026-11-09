package net.runelite.client.plugins.microbot.fm.tickfisher;

public enum FishHandling
{
	DROP("Drop"),
	/** Deposit the fish at the bank when the inventory is full (Shilo, Mount Quidamortem). */
	BANK("Bank"),
	/**
	 * Barbarian fish: knife on the fish for Cooking XP, roe or caviar (eaten as the delay) and
	 * offcuts (bait). Where there is a bank (Mount Quidamortem) only the roe is eaten and the
	 * caviar is banked.
	 */
	CUT("Cut");

	private final String label;

	FishHandling(String label)
	{
		this.label = label;
	}

	@Override
	public String toString()
	{
		return label;
	}
}
