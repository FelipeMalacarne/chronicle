package net.runelite.client.plugins.microbot.fm.tickminer;

import net.runelite.api.coords.WorldPoint;

/**
 * Water for the desert heat: the game drinks a sip from a waterskin at each heat tick (every 90 s,
 * 120 s in desert shirt, robe and boots; wiki, Desert heat). A healthy Kharidian cactus cut with the
 * knife tops a waterskin up by one sip, then stays dry for a minute (wiki, Kharidian cactus); seen
 * in game, a failed cut dries it too. The three east of the quarry are the only ones reachable from
 * it (the ones by the Ruins of Unkah are past a gate): 32 tiles from the rocks, about 30 s a trip.
 * From the rocks the client keeps showing them as it last saw them (dry two minutes after they
 * regrew, healthy at once up close, 2026-11-17), so from afar only the time of the cut counts.
 * Pure: tested without the client.
 */
final class Water
{
	static final WorldPoint[] CACTI = {
		new WorldPoint(3190, 2906, 0), new WorldPoint(3192, 2903, 0), new WorldPoint(3193, 2901, 0)
	};
	/** A walkable tile two or three tiles from each cactus. */
	static final WorldPoint STAND = new WorldPoint(3191, 2904, 0);
	/**
	 * Within this, the client's view of a cactus is current. Updates come by 8x8 zone: from the
	 * corridor the one three zones east was current, the two four zones east were not.
	 */
	static final int SIGHT = 8;
	/**
	 * The cacti are a reserve, walked to only at or below this. Seen in game a cut succeeded about
	 * one time in three (5 of 14 at Woodcutting 64, against ~67% by the wiki's chart), about a sip
	 * per minute-long trip against one drunk every two: keeping the skins full that way took half
	 * the mining time, a waterskin more in the inventory about 2% (2026-11-17). Eight sips, the
	 * owner's margin: 16 minutes of heat, so a run of failed cuts never comes near an empty skin.
	 */
	static final int LOW_SIPS = 8;

	private Water()
	{
	}

	static boolean isCactus(WorldPoint point)
	{
		for (WorldPoint cactus : CACTI)
		{
			if (cactus.equals(point))
			{
				return true;
			}
		}
		return false;
	}

	/** Whether to walk to the cacti now: the water runs low, the skins have room and a cactus is up. */
	static boolean worthATrip(int sips, int capacity, int healthy)
	{
		return sips <= LOW_SIPS && sips < capacity && healthy > 0;
	}
}
