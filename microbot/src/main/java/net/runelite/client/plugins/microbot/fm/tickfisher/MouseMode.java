package net.runelite.client.plugins.microbot.fm.tickfisher;

// Public: the config proxy has to reach it (a package-private enum kills the client thread).
public enum MouseMode
{
	/** A human path everywhere; on the delay tick short fast strokes, to fit 3-4 clicks in 0.6 s. */
	NATURAL("Natural"),
	/** Every click at once, the cursor jumps. */
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
