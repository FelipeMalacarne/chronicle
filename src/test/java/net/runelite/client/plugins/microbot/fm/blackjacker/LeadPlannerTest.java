package net.runelite.client.plugins.microbot.fm.blackjacker;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import net.runelite.api.coords.WorldPoint;
import org.junit.jupiter.api.Test;

/** Leading bandits into Rana the Dyer's house, on the collision map captured in game. */
class LeadPlannerTest
{
	private static final Room ROOM = Room.ALI_THE_DYER;
	private static final Walls WALLS = GridWalls.load("pollnivneach-north-walls.txt");

	private static WorldPoint at(int x, int y)
	{
		return new WorldPoint(x, y, 0);
	}

	private final LeadPlanner planner = new LeadPlanner(WALLS, ROOM.leadGoal);

	@Test
	void followModeStepsDiagonallyThenAlongAnAxis()
	{
		// Open street: straight diagonal.
		assertEquals(at(3359, 2997), planner.step(at(3358, 2998), at(3362, 2994), true));
		// South of the block at 3363-3364,2995 the bandit cannot step north toward the door.
		assertEquals(at(3364, 2994), planner.step(at(3364, 2994), at(3364, 2996), true));
	}

	@Test
	void banditSouthOfTheBlockOnlyComesTheLongWayRound()
	{
		// Where today's first lure got stuck: the player waited north of the block.
		assertFalse(planner.catchesUp(at(3363, 2994), at(3364, 2996)));
		// Led a hop at a time it can still come: south, west through the gap at x 3362, north.
		List<WorldPoint> route = planner.route(at(3363, 2994), ROOM.area::contains, 40);
		assertNotNull(route);
		assertTrue(route.contains(at(3362, 2993)), route.toString());
		assertTrue(route.size() > 10, route.toString());
	}

	@Test
	void banditInsideTheWestHouseCannotBeLed()
	{
		// The second one: the player's path leaves through that house's curtain, the bandit's cannot.
		assertNull(planner.route(at(3359, 3000), ROOM.area::contains, 40));
	}

	@Test
	void banditInTheStreetIsLedStraightInside()
	{
		assertEquals(List.of(at(3359, 2998), at(3361, 2998), at(3363, 2998), at(3364, 2998), at(3364, 3000), at(3364, 3002)),
			planner.route(at(3358, 2998), ROOM.area::contains, 40));
		assertEquals(List.of(at(3365, 2997), at(3364, 2999), at(3364, 3001), at(3364, 3002)),
			planner.route(at(3366, 2997), ROOM.area::contains, 40));
	}

	@Test
	void hopsThroughTheDoorwayKeepTheBanditBehind()
	{
		// Player outside the curtain, bandit beside them in the street: the next hop goes in.
		WorldPoint next = planner.nextHop(at(3364, 2998), at(3363, 2998));
		assertNotNull(next);
		assertTrue(planner.distanceToGoal(next) < planner.distanceToGoal(at(3364, 2998)));
		assertTrue(planner.catchesUp(at(3363, 2998), next));
	}

	@Test
	void noHopWhenTheBanditIsBehindAWall()
	{
		assertNull(planner.nextHop(at(3364, 2996), at(3364, 2994)));
	}

	/** Walls from the fixture: cardinal moves as captured, diagonals by the usual size-1 rule. */
	static final class GridWalls implements Walls
	{
		private final Map<WorldPoint, int[]> tiles = new HashMap<>();

		static GridWalls load(String resource)
		{
			GridWalls walls = new GridWalls();
			try (BufferedReader in = new BufferedReader(new InputStreamReader(
				LeadPlannerTest.class.getResourceAsStream(resource), StandardCharsets.UTF_8)))
			{
				String line;
				while ((line = in.readLine()) != null)
				{
					if (line.isBlank() || line.startsWith("#"))
					{
						continue;
					}
					String[] f = line.trim().split("\\s+");
					int[] v = new int[f.length];
					for (int i = 0; i < f.length; i++)
					{
						v[i] = Integer.parseInt(f[i]);
					}
					// blocked, north, east, south, west
					walls.tiles.put(new WorldPoint(v[0], v[1], 0), new int[]{v[2], v[3], v[4], v[5], v[6]});
				}
			}
			catch (IOException e)
			{
				throw new IllegalStateException(e);
			}
			return walls;
		}

		private boolean free(WorldPoint p)
		{
			int[] t = tiles.get(p);
			return t != null && t[0] == 0;
		}

		private boolean cardinal(WorldPoint from, int dx, int dy)
		{
			int[] t = tiles.get(from);
			if (t == null || !free(from.dx(dx).dy(dy)))
			{
				return false;
			}
			if (dy == 1)
			{
				return t[1] == 1;
			}
			if (dx == 1)
			{
				return t[2] == 1;
			}
			if (dy == -1)
			{
				return t[3] == 1;
			}
			return t[4] == 1;
		}

		@Override
		public boolean canStep(WorldPoint from, int dx, int dy)
		{
			if (dx == 0 || dy == 0)
			{
				return (dx != 0 || dy != 0) && cardinal(from, dx, dy);
			}
			return cardinal(from, dx, 0) && cardinal(from, 0, dy)
				&& cardinal(from.dx(dx), 0, dy) && cardinal(from.dy(dy), dx, 0);
		}
	}
}
