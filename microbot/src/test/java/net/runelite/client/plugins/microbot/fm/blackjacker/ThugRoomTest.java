package net.runelite.client.plugins.microbot.fm.blackjacker;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.List;
import net.runelite.api.coords.WorldPoint;
import org.junit.jupiter.api.Test;

/** The Menaphite Thug room, on the collision map captured in game (back curtain closed). */
class ThugRoomTest
{
	private static final Room ROOM = Room.THUG_TENT;
	private static final Walls WALLS = LeadPlannerTest.GridWalls.load("pollnivneach-south-walls.txt");

	private static WorldPoint at(int x, int y)
	{
		return new WorldPoint(x, y, 0);
	}

	private final LeadPlanner planner = new LeadPlanner(WALLS, ROOM.leadGoal);

	@Test
	void areaIsTheFloorBehindTheCurtains()
	{
		// The only way out of the area is the front doorway: the alleys either side are outside.
		for (int x = ROOM.area.getX(); x < ROOM.area.getX() + ROOM.area.getWidth(); x++)
		{
			for (int y = ROOM.area.getY(); y < ROOM.area.getY() + ROOM.area.getHeight(); y++)
			{
				WorldPoint tile = at(x, y);
				for (int d = 0; d < 8; d++)
				{
					WorldPoint next = tile.dx(Walls.Direction.DX[d]).dy(Walls.Direction.DY[d]);
					if (WALLS.canStep(tile, Walls.Direction.DX[d], Walls.Direction.DY[d]) && !ROOM.area.contains(next))
					{
						assertEquals(ROOM.inside, tile, tile + " -> " + next);
						assertEquals(ROOM.curtain, next, tile + " -> " + next);
					}
				}
			}
		}
		assertFalse(ROOM.area.contains(at(3347, 2955)));
		assertFalse(ROOM.area.contains(at(3352, 2955)));
	}

	@Test
	void leadGoalIsFloorReachedFromTheDoor()
	{
		// The first goal was the stool at 3348,2954: no tile led to it, so no thug could be lured.
		assertNotEquals(LeadPlanner.UNREACHABLE, planner.distanceToGoal(ROOM.inside));
		assertNotEquals(LeadPlanner.UNREACHABLE, planner.distanceToGoal(ROOM.outside));
	}

	@Test
	void thugByTheDoorIsLedIntoTheFarCorner()
	{
		assertEquals(List.of(at(3350, 2958), at(3350, 2956), at(3350, 2954), at(3351, 2953)),
			planner.route(at(3349, 2958), ROOM.area::contains, 20));
	}

	@Test
	void thugsDownTheStreetCanBeLed()
	{
		List<WorldPoint> route = planner.route(at(3340, 2957), ROOM.area::contains, 20);
		assertNotNull(route);
		assertTrue(route.contains(at(3349, 2958)), route.toString());
		// From the north house, through its open curtain.
		assertNotNull(planner.route(at(3347, 2966), ROOM.area::contains, 20));
	}
}
