package net.runelite.client.plugins.microbot.fm.tickminer;

import java.util.Set;
import net.runelite.api.coords.WorldPoint;

/**
 * The 3t4s spot of the Desert Quarry, a dozen tiles east of Drew's Sandstorm grinder: four sandstone
 * rocks two by two on either side of a one-tile corridor (x 3165, y 2905-2906; seen in game, the
 * corridor's west and east edges are the rocks). From each corridor tile the two rocks of its row
 * are in reach; the other row's are a step away, and moving up to a rock gives it two rolls (wiki,
 * Tick manipulation), so every cycle steps to the other row while one of its rocks is up.
 * Pure: tested without the client.
 */
final class Rocks
{
	static final WorldPoint SOUTH = new WorldPoint(3165, 2905, 0);
	static final WorldPoint NORTH = new WorldPoint(3165, 2906, 0);
	/** West and east of each corridor tile. */
	static final WorldPoint[] ALL = {
		new WorldPoint(3164, 2905, 0), new WorldPoint(3166, 2905, 0),
		new WorldPoint(3164, 2906, 0), new WorldPoint(3166, 2906, 0)
	};

	private Rocks()
	{
	}

	static boolean isSpotRock(WorldPoint point)
	{
		for (WorldPoint rock : ALL)
		{
			if (rock.equals(point))
			{
				return true;
			}
		}
		return false;
	}

	static boolean inCorridor(WorldPoint me)
	{
		return SOUTH.equals(me) || NORTH.equals(me);
	}

	/** The corridor tile a rock is mined from. */
	static WorldPoint standTile(WorldPoint rock)
	{
		return rock.getY() == SOUTH.getY() ? SOUTH : NORTH;
	}

	/**
	 * The rock to click: one on the other row (a step, two rolls), else one in reach; of two, the
	 * one not clicked last, else the east one when {@code east} (the caller's coin). Away from the
	 * corridor, the rock whose corridor tile is nearest. Null when no rock is up.
	 */
	static WorldPoint next(WorldPoint me, Set<WorldPoint> up, WorldPoint last, boolean east)
	{
		if (me == null)
		{
			return null;
		}
		WorldPoint best = null;
		int bestRank = Integer.MAX_VALUE;
		for (WorldPoint rock : ALL)
		{
			if (!up.contains(rock))
			{
				continue;
			}
			boolean isEast = rock.getX() > SOUTH.getX();
			int rank = reach(me, rock) * 4 + (rock.equals(last) ? 2 : 0) + (isEast == east ? 0 : 1);
			if (rank < bestRank)
			{
				best = rock;
				bestRank = rank;
			}
		}
		return best;
	}

	/** In the corridor 0 for a step to the other row, 1 for a rock in reach; away, the walk to it. */
	private static int reach(WorldPoint me, WorldPoint rock)
	{
		if (!inCorridor(me))
		{
			// distanceTo is Integer.MAX_VALUE on another plane, which the rank's * 4 would overflow.
			return Math.min(standTile(rock).distanceTo(me), 10_000);
		}
		return standTile(rock).equals(me) ? 1 : 0;
	}
}
