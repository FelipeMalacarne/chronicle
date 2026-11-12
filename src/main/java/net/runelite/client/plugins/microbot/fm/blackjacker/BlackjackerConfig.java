package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;
import net.runelite.client.config.Range;

@ConfigGroup(BlackjackerConfig.GROUP)
public interface BlackjackerConfig extends Config
{
	String GROUP = "fm-blackjacker";

	@ConfigItem(
		keyName = "target",
		name = "Target",
		description = "Auto picks the best target for your Thieving level (Bandit 41, Bandit 56 at 55, Menaphite Thug at 65 after The Feud)",
		position = 0
	)
	default BlackjackTarget target()
	{
		return BlackjackTarget.AUTO;
	}

	@ConfigItem(
		keyName = "drinkAtPercent",
		name = "Drink wine at HP %",
		description = "Drink a jug of wine at or below this share of max Hitpoints. Never lower than two of the target's max hits plus 10% of max HP. Taken between knock-outs, once the target is up",
		position = 1
	)
	@Range(min = 30, max = 90)
	default int drinkAtPercent()
	{
		return 65;
	}

	@ConfigItem(
		keyName = "minWines",
		name = "Restock below",
		description = "Walk to the Asp & Snake bar to buy wine when fewer jugs of wine than this are left",
		position = 2
	)
	@Range(min = 1, max = 27)
	default int minWines()
	{
		return 2;
	}

	@ConfigItem(
		keyName = "hopForWine",
		name = "Hop worlds for wine",
		description = "When the barman runs out before the inventory is full of wine, hop to another members world and buy more",
		position = 3
	)
	default boolean hopForWine()
	{
		return true;
	}

	@ConfigItem(
		keyName = "stopAtLevel",
		name = "Stop at Thieving level",
		description = "Stop once Thieving reaches this level (0 = never). The stop waits until nothing is hitting us",
		position = 4
	)
	@Range(min = 0, max = 99)
	default int stopAtLevel()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "stopAfterMinutes",
		name = "Stop after minutes",
		description = "Stop after running this long (0 = never). The stop waits until nothing is hitting us",
		position = 5
	)
	@Range(min = 0, max = 1440)
	default int stopAfterMinutes()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "showOverlay",
		name = "Show overlay",
		description = "Progress panel: what it is doing, XP and XP/h, knock-outs, picks, wine and climbs",
		position = 6
	)
	default boolean showOverlay()
	{
		return true;
	}

	@ConfigItem(
		keyName = "testCycles",
		name = "Test cycles",
		description = "In test mode, successful knock-outs to observe before writing the result and exiting",
		position = 7
	)
	default int testCycles()
	{
		return 5;
	}
}
