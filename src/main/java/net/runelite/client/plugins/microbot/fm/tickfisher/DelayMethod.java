package net.runelite.client.plugins.microbot.fm.tickfisher;

/**
 * What sets the skilling timer to 3 on phase 0. Both measured the same at Otto; teak is cheaper
 * when a cycle slips and the action completes (one log, against 15 swamp tar and the herb).
 * Public: the config proxy reads it from the overlay.
 */
public enum DelayMethod
{
	/** Knife on one teak log. */
	TEAK("Knife on teak"),
	/** A clean herb on 15+ swamp tar (the herb's tar level, a pestle and mortar). */
	HERB_TAR("Herb on swamp tar");

	private final String label;

	DelayMethod(String label)
	{
		this.label = label;
	}

	@Override
	public String toString()
	{
		return label;
	}
}
