package net.runelite.client.plugins.microbot.fm.catswap;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;

@ConfigGroup(CatSwapConfig.GROUP)
public interface CatSwapConfig extends Config
{
	String GROUP = "fm-catswap";

	@ConfigItem(
		keyName = "buyKitten",
		name = "Buy a new kitten",
		description = "After selling the cat, buy a kitten from Gertrude (100 coins) and let it follow",
		position = 0
	)
	default boolean buyKitten()
	{
		return true;
	}
}
