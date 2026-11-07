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
		description = "Drink a jug of wine at or below this share of max Hitpoints. Never lower than three of the target's max hits plus 10% of max HP; while a target is knocked out the drink takes the second pickpocket's place",
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
		keyName = "testCycles",
		name = "Test cycles",
		description = "In test mode, successful knock-outs to observe before writing the result and exiting",
		position = 4
	)
	default int testCycles()
	{
		return 5;
	}
}
