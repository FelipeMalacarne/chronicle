package net.runelite.client.plugins.microbot.fm.tickfisher;

// Public: the config proxy has to reach it (a package-private enum kills the client thread).
public enum MouseMode
{
	NATURAL("Natural"),
	/** Natural, except the clicks of the delay tick, which go out at once (3-4 in 0.6 s). */
	HYBRID("Natural, quick delay tick"),
	QUICK("Quick");

	private final String label;

	MouseMode(String label)
	{
		this.label = label;
	}

	@Override
	public String toString()
	{
		return label;
	}
}
