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
		keyName = "threeTick",
		name = "3-tick",
		description = "Knife on a teak log, then the spot, every 3 ticks: a roll every 3 ticks instead of 5. Needs a knife and exactly one teak log (taken from the bank). Off: plain fishing",
		position = 0
	)
	default boolean threeTick()
	{
		return true;
	}

	@ConfigItem(
		keyName = "fishHandling",
		name = "Fish",
		description = "Drop: one fish between rolls, fastest XP. Bank: keep them, deposit at the Shilo bank when the inventory is full",
		position = 1
	)
	default FishHandling fishHandling()
	{
		return FishHandling.DROP;
	}

	@ConfigItem(
		keyName = "mouse",
		name = "Mouse",
		description = "Natural: the cursor travels a human path and waits on the next target during the free ticks. Quick: clicks land at once and the cursor jumps (tightest timing, easy to spot)",
		position = 2
	)
	default MouseMode mouse()
	{
		return MouseMode.NATURAL;
	}

	@ConfigItem(
		keyName = "varyRhythm",
		name = "Vary the rhythm",
		description = "Now and then hesitate for a cycle or two, and every few minutes fish plainly for a while, as a person does. Costs some XP",
		position = 3
	)
	default boolean varyRhythm()
	{
		return true;
	}

	@ConfigItem(
		keyName = "stopAtLevel",
		name = "Stop at Fishing level",
		description = "Stop once Fishing reaches this level (0 = never)",
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
		description = "Stop after running this long (0 = never). Perfect 3-tick for hours is not something a person does",
		position = 5
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
		position = 6
	)
	default boolean showOverlay()
	{
		return true;
	}

	@ConfigItem(
		keyName = "testCatches",
		name = "Test catches",
		description = "In test mode, fish to catch before writing the result and exiting",
		position = 7
	)
	default int testCatches()
	{
		return 15;
	}
}
