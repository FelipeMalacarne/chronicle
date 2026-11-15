package net.runelite.client.plugins.microbot.fm.mtaarena;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;
import net.runelite.client.config.ConfigSection;
import net.runelite.client.config.Range;

@ConfigGroup(MtaArenaConfig.GROUP)
public interface MtaArenaConfig extends Config
{
	String GROUP = "fm-mtaarena";

	@ConfigSection(
		name = "Rewards",
		description = "The Rewards Guardian's items to earn pizazz points for",
		position = 0
	)
	String REWARDS = "rewards";

	@ConfigSection(
		name = "Run",
		description = "Let the plugin play the rooms and buy",
		position = 2
	)
	String RUN = "run";

	@ConfigSection(
		name = "Calculator",
		description = "How the runes are counted",
		position = 1
	)
	String CALCULATOR = "calculator";

	@ConfigItem(keyName = "infinityHat", name = "Infinity hat", description = "350 / 350 / 3000 / 400 points", section = REWARDS, position = 0)
	default boolean infinityHat()
	{
		return false;
	}

	@ConfigItem(keyName = "infinityTop", name = "Infinity top", description = "400 / 400 / 4000 / 450 points", section = REWARDS, position = 1)
	default boolean infinityTop()
	{
		return false;
	}

	@ConfigItem(keyName = "infinityBottoms", name = "Infinity bottoms", description = "450 / 450 / 5000 / 500 points", section = REWARDS, position = 2)
	default boolean infinityBottoms()
	{
		return false;
	}

	@ConfigItem(keyName = "infinityGloves", name = "Infinity gloves", description = "175 / 175 / 1500 / 225 points", section = REWARDS, position = 3)
	default boolean infinityGloves()
	{
		return false;
	}

	@ConfigItem(keyName = "infinityBoots", name = "Infinity boots", description = "120 / 120 / 1200 / 120 points", section = REWARDS, position = 4)
	default boolean infinityBoots()
	{
		return false;
	}

	@ConfigItem(keyName = "beginnerWand", name = "Beginner wand", description = "30 / 30 / 300 / 30 points", section = REWARDS, position = 5)
	default boolean beginnerWand()
	{
		return false;
	}

	@ConfigItem(keyName = "apprenticeWand", name = "Apprentice wand", description = "An upgrade: also needs the beginner wand, which the calculator adds when you do not have it", section = REWARDS, position = 6)
	default boolean apprenticeWand()
	{
		return false;
	}

	@ConfigItem(keyName = "teacherWand", name = "Teacher wand", description = "An upgrade: also needs the apprentice wand, which the calculator adds when you do not have it", section = REWARDS, position = 7)
	default boolean teacherWand()
	{
		return false;
	}

	@ConfigItem(keyName = "masterWand", name = "Master wand", description = "An upgrade: the whole wand chain below it is added when you do not have it", section = REWARDS, position = 8)
	default boolean masterWand()
	{
		return false;
	}

	@ConfigItem(keyName = "magesBook", name = "Mage's book", description = "500 / 500 / 6000 / 550 points", section = REWARDS, position = 9)
	default boolean magesBook()
	{
		return false;
	}

	@ConfigItem(keyName = "runePouch", name = "Rune pouch", description = "150 / 150 / 1500 / 200 points", section = REWARDS, position = 10)
	default boolean runePouch()
	{
		return false;
	}

	@ConfigItem(keyName = "bonesToPeaches", name = "Bones to Peaches", description = "200 / 200 / 2000 / 300 points; it nearly doubles the Creature Graveyard's points per hour", section = REWARDS, position = 11)
	default boolean bonesToPeaches()
	{
		return false;
	}

	@Range(min = 0, max = 6)
	@ConfigItem(
		keyName = "enchantLevel",
		name = "Enchant level",
		description = "Lvl-N Enchant the cosmic runes assume (1 to 6). 0 takes the best one your Magic level casts",
		section = CALCULATOR,
		position = 0
	)
	default int enchantLevel()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "overlay",
		name = "Show overlay",
		description = "The points missing, the runes they take and the runes you carry",
		section = CALCULATOR,
		position = 1
	)
	default boolean overlay()
	{
		return true;
	}

	@ConfigItem(
		keyName = "forget",
		name = "Forget remembered state",
		description = "Clears the pizazz points and items the plugin remembers (it switches itself back off)",
		section = CALCULATOR,
		position = 2
	)
	default boolean forget()
	{
		return false;
	}

	@ConfigItem(
		keyName = "automate",
		name = "Run the plan",
		description = "The plugin clicks: it plays the Enchanting Chamber and buys the rewards in the plan's order. The other rooms it asks you to do by hand. Off by default",
		section = RUN,
		position = 0
	)
	default boolean automate()
	{
		return false;
	}
}
