package net.runelite.client.plugins.microbot.fm.tickfisher;

public enum FishHandling
{
	DROP("Drop"),
	/** Shilo only: deposit at the bank when the inventory is full. */
	BANK("Bank"),
	/** Barbarian fish: knife on the fish for Cooking XP, roe or caviar (eaten as the delay) and offcuts (bait). */
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
