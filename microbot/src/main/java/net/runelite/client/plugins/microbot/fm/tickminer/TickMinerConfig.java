package net.runelite.client.plugins.microbot.fm.tickminer;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;
import net.runelite.client.config.Range;

@ConfigGroup(TickMinerConfig.GROUP)
public interface TickMinerConfig extends Config
{
	String GROUP = "fm-tickminer";

	@ConfigItem(
		keyName = "threeTick",
		name = "3-tick",
		description = "Knife on a teak log, then a rock of the other row, every 3 ticks: a roll every 3 ticks (two when stepping to the rock) instead of the pickaxe's. Needs the delay items (see 3-tick delay). Off: plain mining",
		position = 0
	)
	default boolean threeTick()
	{
		return true;
	}

	@ConfigItem(
		keyName = "delay",
		name = "3-tick delay",
		description = "Knife on teak: a knife and exactly one teak log (a second one opens the Make-X). Herb on swamp tar: one clean guam (or marrentill, tarromin, harralander) at a time, 15+ swamp tar, a pestle and mortar; the tar is not used up. Once the log or herb is used up (a cycle that slipped), it mines plainly",
		position = 1
	)
	default DelayMethod delay()
	{
		return DelayMethod.TEAK;
	}

	@ConfigItem(
		keyName = "mouse",
		name = "Mouse",
		description = "Natural: the cursor travels a human path, and on the delay tick moves in short fast strokes to fit its clicks (teak, rock) like a 3-tick player. Quick: every click at once, the cursor jumps (easy to spot)",
		position = 2
	)
	default MouseMode mouse()
	{
		return MouseMode.NATURAL;
	}

	@ConfigItem(
		keyName = "varyRhythm",
		name = "Vary the rhythm",
		description = "Now and then hesitate for a cycle or two, and every few minutes mine plainly for a while, as a person does. Costs some XP",
		position = 3
	)
	default boolean varyRhythm()
	{
		return true;
	}

	@ConfigItem(
		keyName = "stopAtLevel",
		name = "Mining goal",
		description = "Mining level to aim for: the overlay shows the time to it, and the plugin stops there if Stop at goal is on (0 = none)",
		position = 4
	)
	@Range(min = 0, max = 99)
	default int stopAtLevel()
	{
		return 0;
	}

	@ConfigItem(
		keyName = "stopAtGoal",
		name = "Stop at goal",
		description = "Stop once the goal level is reached. Off, the goal only shows the time left and the plugin keeps mining",
		position = 5
	)
	default boolean stopAtGoal()
	{
		return true;
	}

	@ConfigItem(
		keyName = "stopAfterMinutes",
		name = "Stop after minutes",
		description = "Stop after running this long (0 = never). Perfect 3-tick for hours is not something a person does",
		position = 6
	)
	@Range(min = 0, max = 1440)
	default int stopAfterMinutes()
	{
		return 90;
	}

	@ConfigItem(
		keyName = "minHitpointsPercent",
		name = "Stop below HP %",
		description = "Stop (and log out, see below) when hitpoints fall below this share of the maximum: the quarry's desert heat hurts once the waterskins are empty. It also stops when the last waterskin runs dry, unless a Desert amulet 4 or a charged circlet of water is worn",
		position = 7
	)
	@Range(min = 0, max = 100)
	default int minHitpointsPercent()
	{
		return 50;
	}

	@ConfigItem(
		keyName = "refillWater",
		name = "Refill waterskins",
		description = "With 8 sips or fewer left, top the waterskins up with the knife at the three Kharidian cacti east of the quarry (a sip per cactus when the cut succeeds, which then stays dry a minute), waiting there for them while the water stays that low. A reserve: bring the waterskins for the session, a slot each costs less mining than the trips. Off: no trips, it mines until the last waterskin runs dry, then stops (and logs out, see below) before the heat hurts",
		position = 8
	)
	default boolean refillWater()
	{
		return true;
	}

	@ConfigItem(
		keyName = "logoutOnStop",
		name = "Log out when stopping",
		description = "Log out when it stops on its own (a goal, the time limit, no water, missing gear) and keep the break handler and AutoLogin from logging back in to stand idle in the desert heat. Logging in by hand or starting another script lifts that",
		position = 9
	)
	default boolean logoutOnStop()
	{
		return true;
	}

	@ConfigItem(
		keyName = "showOverlay",
		name = "Show overlay",
		description = "Progress panel: XP and XP/h, sandstone mined and deposited, and how well the 3-tick cycle is kept",
		position = 10
	)
	default boolean showOverlay()
	{
		return true;
	}

	@ConfigItem(
		keyName = "testMines",
		name = "Test mines",
		description = "In test mode, sandstone to mine before writing the result and exiting",
		position = 11
	)
	default int testMines()
	{
		return 15;
	}
}
