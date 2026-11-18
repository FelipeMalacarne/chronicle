package net.runelite.client.plugins.microbot.fm.processor;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;

@ConfigGroup(ProcessorConfig.GROUP)
public interface ProcessorConfig extends Config
{
	String GROUP = "fm-processor";

	@ConfigItem(
		keyName = "product",
		name = "Product",
		description = "What to make",
		position = 0
	)
	default Product product()
	{
		return Product.MOLTEN_GLASS;
	}

	@ConfigItem(
		keyName = "site",
		name = "Site",
		description = "Bank and station to make it at (must have the product's station)",
		position = 1
	)
	default Site site()
	{
		return Site.EDGEVILLE_FURNACE;
	}

	@ConfigItem(
		keyName = "maxTrips",
		name = "Max trips",
		description = "Stop after this many bank trips (0 = until the bank runs out of an input)",
		position = 2
	)
	default int maxTrips()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "testTrips",
		name = "Test trips",
		description = "In test mode, trips to complete before writing the result and exiting",
		position = 3
	)
	default int testTrips()
	{
		return 1;
	}
}
