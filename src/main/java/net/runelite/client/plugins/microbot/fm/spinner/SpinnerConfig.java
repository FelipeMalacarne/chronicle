package net.runelite.client.plugins.microbot.fm.spinner;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;

@ConfigGroup(SpinnerConfig.GROUP)
public interface SpinnerConfig extends Config
{
	String GROUP = "fm-spinner";

	@ConfigItem(
		keyName = "maxTrips",
		name = "Max trips",
		description = "Stop after this many shear/spin/bank trips (0 = keep going)",
		position = 0
	)
	default int maxTrips()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "testTrips",
		name = "Test trips",
		description = "In test mode, trips to complete before writing the result and exiting",
		position = 1
	)
	default int testTrips()
	{
		return 1;
	}
}
