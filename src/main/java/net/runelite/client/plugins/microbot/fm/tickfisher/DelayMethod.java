package net.runelite.client.plugins.microbot.fm.tickfisher;

/** What sets the skilling timer to 3 on phase 0. Public: the config proxy reads it from the overlay. */
public enum DelayMethod
{
	/**
	 * Knife on one teak log. Measured at Otto: it does not bring the roll after a catch closer
	 * than 5 ticks, so after a catch the cycle moves to put that roll at +5.
	 */
	TEAK("Knife on teak", 2),
	/**
	 * A clean herb on swamp tar (15+, the herb's tar level, a pestle and mortar). The standard
	 * 3-tick method; expected to work right after a catch too (its timer check passes at 0, not
	 * only below 0), so the cycle stays on 3 ticks.
	 */
	HERB_TAR("Herb on swamp tar", 0);

	private final String label;
	/** Ticks from a catch to the next delay click: 2 puts the next roll at +5, 0 keeps it at +3. */
	final int afterCatch;

	DelayMethod(String label, int afterCatch)
	{
		this.label = label;
		this.afterCatch = afterCatch;
	}

	@Override
	public String toString()
	{
		return label;
	}
}
