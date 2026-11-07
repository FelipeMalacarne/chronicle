package net.runelite.client.plugins.microbot.fm.tickfisher;

public enum FishHandling
{
	DROP("Drop"),
	BANK("Bank");

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
