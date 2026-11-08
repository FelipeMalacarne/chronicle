package net.runelite.client.plugins.microbot.fm.tickfisher;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;
import net.runelite.client.config.Range;

@ConfigGroup(TickFisherConfig.GROUP)
public interface TickFisherConfig extends Config
{
	String GROUP = "fm-tickfisher";

	@ConfigItem(
		keyName = "place",
		name = "Place",
		description = "Shilo Village: fly fishing on the river by the bank (fly fishing rod, feathers). Otto's Grotto: barbarian fishing (barbarian rod, feathers or offcuts; no bank, bring the gear)",
		position = -1
	)
	default FishingPlace place()
	{
		return FishingPlace.SHILO;
	}

	@ConfigItem(
		keyName = "threeTick",
		name = "3-tick",
		description = "Knife on a teak log, then the spot, every 3 ticks: a roll every 3 ticks instead of 5. Needs the delay items (see 3-tick delay). Off: plain fishing",
		position = 0
	)
	default boolean threeTick()
	{
		return true;
	}

	@ConfigItem(
		keyName = "delay",
		name = "3-tick delay",
		description = "Knife on teak: one teak log and a knife. Herb on swamp tar: one clean guam (or marrentill, tarromin, harralander) at a time, 15+ swamp tar, a pestle and mortar; the tar is not used up. With cut-eat, roe or caviar stands in for either now and then",
		position = 1
	)
	default DelayMethod delay()
	{
		return DelayMethod.TEAK;
	}

	@ConfigItem(
		keyName = "fishHandling",
		name = "Fish",
		description = "Drop: one fish between rolls, fastest XP. Bank (Shilo): keep them, deposit at the Shilo bank when the inventory is full. Cut (Otto): knife on the fish between rolls for Cooking XP; the roe or caviar is eaten as the 3-tick delay when there is one, the offcuts are bait",
		position = 2
	)
	default FishHandling fishHandling()
	{
		return FishHandling.DROP;
	}

	@ConfigItem(
		keyName = "mouse",
		name = "Mouse",
		description = "Natural: the cursor travels a human path and waits on the next target during the free ticks. Quick: clicks land at once and the cursor jumps (tightest timing, easy to spot)",
		position = 3
	)
	default MouseMode mouse()
	{
		return MouseMode.NATURAL;
	}

	@ConfigItem(
		keyName = "varyRhythm",
		name = "Vary the rhythm",
		description = "Now and then hesitate for a cycle or two, and every few minutes fish plainly for a while, as a person does. Costs some XP",
		position = 4
	)
	default boolean varyRhythm()
	{
		return true;
	}

	@ConfigItem(
		keyName = "stopAtLevel",
		name = "Stop at Fishing level",
		description = "Stop once Fishing reaches this level (0 = never)",
		position = 5
	)
	@Range(min = 0, max = 99)
	default int stopAtLevel()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "stopAtAgilityLevel",
		name = "Stop at Agility level",
		description = "Barbarian fishing: stop once Agility reaches this level (0 = never)",
		position = 6
	)
	@Range(min = 0, max = 99)
	default int stopAtAgilityLevel()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "stopAfterMinutes",
		name = "Stop after minutes",
		description = "Stop after running this long (0 = never). Perfect 3-tick for hours is not something a person does",
		position = 7
	)
	@Range(min = 0, max = 1440)
	default int stopAfterMinutes()
	{
		return 90;
	}

	@ConfigItem(
		keyName = "showOverlay",
		name = "Show overlay",
		description = "Progress panel: XP and XP/h, catches, and how well the 3-tick cycle is kept",
		position = 8
	)
	default boolean showOverlay()
	{
		return true;
	}

	@ConfigItem(
		keyName = "catchDelayTick",
		name = "Delay after a catch",
		description = "Ticks from a catch to the delay click (with Spot after a catch above 0)",
		position = 20,
		hidden = true
	)
	default int catchDelayTick()
	{
		return 2;
	}

	@ConfigItem(
		keyName = "catchSpotTick",
		name = "Spot after a catch",
		description = "Ticks from a catch to the spot click (0: the delay, fish and spot all on the catch tick)",
		position = 21,
		hidden = true
	)
	default int catchSpotTick()
	{
		return 2;
	}

	@ConfigItem(
		keyName = "testCatches",
		name = "Test catches",
		description = "In test mode, fish to catch before writing the result and exiting",
		position = 9
	)
	default int testCatches()
	{
		return 15;
	}
}
