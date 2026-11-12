package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Predicate;
import net.runelite.api.coords.WorldPoint;

/**
 * Plans how to lead a lured NPC into a room. A following NPC moves in "follow mode" (OSRS wiki,
 * Pathfinding): each tick one step diagonally toward the player if it can, else one step along a
 * single axis, else it stands still. It never walks around a wall, so the player has to keep to
 * tiles the NPC can reach in a straight line, a couple of tiles at a time. Pure logic over a
 * {@link Walls} snapshot.
 */
final class LeadPlanner
{
	static final int UNREACHABLE = Integer.MAX_VALUE;
	// Ticks of following to simulate after a hop of at most two tiles.
	private static final int FOLLOW_STEPS = 10;
	// The walking distances to the goal cover this many tiles around it.
	private static final int FIELD_RADIUS = 20;
	// Tiles the player covers in one hop: one tick of running.
	private static final int HOP = 2;

	private final Walls walls;
	private final WorldPoint goal;
	private final Map<WorldPoint, Integer> toGoal;

	LeadPlanner(Walls walls, WorldPoint goal)
	{
		this.walls = walls;
		this.goal = goal;
		this.toGoal = distancesTo(goal);
	}

	static boolean adjacent(WorldPoint a, WorldPoint b)
	{
		return a.getPlane() == b.getPlane() && Math.abs(a.getX() - b.getX()) <= 1 && Math.abs(a.getY() - b.getY()) <= 1;
	}

	/**
	 * One follow-mode step from {@code from} toward {@code to}: diagonal if free, else along one
	 * axis; {@code xFirst} says which axis is tried first. Returns {@code from} when stuck.
	 */
	WorldPoint step(WorldPoint from, WorldPoint to, boolean xFirst)
	{
		int dx = Integer.signum(to.getX() - from.getX());
		int dy = Integer.signum(to.getY() - from.getY());
		if (dx != 0 && dy != 0 && walls.canStep(from, dx, dy))
		{
			return from.dx(dx).dy(dy);
		}
		int firstX = xFirst ? dx : 0;
		int firstY = xFirst ? 0 : dy;
		if ((firstX != 0 || firstY != 0) && walls.canStep(from, firstX, firstY))
		{
			return from.dx(firstX).dy(firstY);
		}
		int secondX = xFirst ? 0 : dx;
		int secondY = xFirst ? dy : 0;
		if ((secondX != 0 || secondY != 0) && walls.canStep(from, secondX, secondY))
		{
			return from.dx(secondX).dy(secondY);
		}
		return from;
	}

	/** Where the NPC ends up following a player who stands on {@code player}. */
	WorldPoint follow(WorldPoint npc, WorldPoint player, boolean xFirst)
	{
		WorldPoint at = npc;
		for (int i = 0; i < FOLLOW_STEPS && !adjacent(at, player); i++)
		{
			WorldPoint next = step(at, player, xFirst);
			if (next.equals(at))
			{
				break;
			}
			at = next;
		}
		return at;
	}

	/** Whether the NPC gets next to {@code player}, whichever axis it tries first. */
	boolean catchesUp(WorldPoint npc, WorldPoint player)
	{
		return adjacent(follow(npc, player, true), player) && adjacent(follow(npc, player, false), player);
	}

	/** The player's walking distance to the goal in steps, or {@link #UNREACHABLE}. */
	int distanceToGoal(WorldPoint tile)
	{
		return toGoal.getOrDefault(tile, UNREACHABLE);
	}

	/**
	 * Next tile for the player while leading: at most a hop away, closer to the goal, and one the
	 * NPC can follow to in a straight line. Null when there is none.
	 */
	WorldPoint nextHop(WorldPoint player, WorldPoint npc)
	{
		return bestHop(player, npc, distanceToGoal(player));
	}

	/**
	 * A tile at most a hop away where the stuck NPC can catch up with us, the one nearest the
	 * goal. Null when there is none.
	 */
	WorldPoint regroup(WorldPoint player, WorldPoint npc)
	{
		return bestHop(player, npc, UNREACHABLE);
	}

	/** The tile within a hop that the NPC can follow to, nearest the goal and nearer than {@code below}. */
	private WorldPoint bestHop(WorldPoint player, WorldPoint npc, int below)
	{
		WorldPoint best = null;
		int bestDistance = below;
		for (WorldPoint tile : within(player, HOP))
		{
			int distance = distanceToGoal(tile);
			if (distance < bestDistance && !tile.equals(npc) && catchesUp(npc, tile))
			{
				best = tile;
				bestDistance = distance;
			}
		}
		return best;
	}

	/** Whether {@link #route} finds a way to lead the NPC in. */
	boolean canLead(WorldPoint npc, Predicate<WorldPoint> inside, int maxHops)
	{
		return route(npc, inside, maxHops) != null;
	}

	/**
	 * The player's tiles, hop by hop, that lead an NPC on {@code npc} (lured from a tile next to
	 * it) to the goal with it ending up {@code inside}; null when there is no such lead. Plays the
	 * whole lead out with the NPC trying the x axis first.
	 */
	List<WorldPoint> route(WorldPoint npc, Predicate<WorldPoint> inside, int maxHops)
	{
		WorldPoint player = null;
		for (WorldPoint tile : within(npc, 1))
		{
			// Lure is clicked from a tile beside the NPC; take the one nearest the goal.
			if (!tile.equals(npc) && Math.abs(tile.getX() - npc.getX()) + Math.abs(tile.getY() - npc.getY()) == 1
				&& (player == null || distanceToGoal(tile) < distanceToGoal(player)))
			{
				player = tile;
			}
		}
		if (player == null || distanceToGoal(player) == UNREACHABLE)
		{
			return null;
		}
		List<WorldPoint> route = new ArrayList<>();
		route.add(player);
		WorldPoint at = npc;
		for (int hop = 0; hop < maxHops; hop++)
		{
			if (player.equals(goal))
			{
				return inside.test(at) ? route : null;
			}
			WorldPoint next = nextHop(player, at);
			if (next == null)
			{
				return null;
			}
			player = next;
			route.add(player);
			at = follow(at, player, true);
		}
		return null;
	}

	/** Tiles reachable from {@code start} in 1..{@code steps} steps, nearest first. */
	private List<WorldPoint> within(WorldPoint start, int steps)
	{
		List<WorldPoint> tiles = new ArrayList<>();
		Map<WorldPoint, Integer> seen = new HashMap<>();
		ArrayDeque<WorldPoint> queue = new ArrayDeque<>();
		seen.put(start, 0);
		queue.add(start);
		while (!queue.isEmpty())
		{
			WorldPoint at = queue.poll();
			int depth = seen.get(at);
			if (depth >= steps)
			{
				continue;
			}
			for (int d = 0; d < 8; d++)
			{
				WorldPoint next = at.dx(Walls.Direction.DX[d]).dy(Walls.Direction.DY[d]);
				if (!seen.containsKey(next) && walls.canStep(at, Walls.Direction.DX[d], Walls.Direction.DY[d]))
				{
					seen.put(next, depth + 1);
					tiles.add(next);
					queue.add(next);
				}
			}
		}
		return tiles;
	}

	/** Walking distance from every tile near the goal to it: a breadth-first search outward from it. */
	private Map<WorldPoint, Integer> distancesTo(WorldPoint goal)
	{
		Map<WorldPoint, Integer> distances = new HashMap<>();
		ArrayDeque<WorldPoint> queue = new ArrayDeque<>();
		distances.put(goal, 0);
		queue.add(goal);
		while (!queue.isEmpty())
		{
			WorldPoint at = queue.poll();
			int distance = distances.get(at);
			for (int d = 0; d < 8; d++)
			{
				int dx = Walls.Direction.DX[d];
				int dy = Walls.Direction.DY[d];
				// A tile that reaches `at` with this step.
				WorldPoint from = at.dx(-dx).dy(-dy);
				if (!distances.containsKey(from) && from.distanceTo2D(goal) <= FIELD_RADIUS && walls.canStep(from, dx, dy))
				{
					distances.put(from, distance + 1);
					queue.add(from);
				}
			}
		}
		return distances;
	}
}
