package net.runelite.client.plugins.microbot.fm.tickfisher;

// Public: the config proxy has to reach it (a package-private enum kills the client thread).
public enum MouseMode
{
	NATURAL("Natural"),
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
