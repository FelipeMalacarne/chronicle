package net.runelite.client.plugins.microbot.fm.tickminer;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.Set;
import net.runelite.api.coords.WorldPoint;
import org.junit.jupiter.api.Test;

class RocksTest
{
	private static final WorldPoint SW = new WorldPoint(3164, 2905, 0);
	private static final WorldPoint SE = new WorldPoint(3166, 2905, 0);
	private static final WorldPoint NW = new WorldPoint(3164, 2906, 0);
	private static final WorldPoint NE = new WorldPoint(3166, 2906, 0);
	private static final Set<WorldPoint> ALL_UP = up(SW, SE, NW, NE);

	private static Set<WorldPoint> up(WorldPoint... rocks)
	{
		return new HashSet<>(Arrays.asList(rocks));
	}

	@Test
	void stepsToTheOtherRowWhileOneOfItsRocksIsUp()
	{
		assertEquals(NE, Rocks.next(Rocks.SOUTH, ALL_UP, null, true));
		assertEquals(NW, Rocks.next(Rocks.SOUTH, ALL_UP, null, false));
		assertEquals(SW, Rocks.next(Rocks.NORTH, ALL_UP, SE, false));
		assertEquals(NW, Rocks.next(Rocks.SOUTH, up(SW, SE, NW), null, true));
	}

	@Test
	void minesInReachWhenTheOtherRowIsDown()
	{
		assertEquals(SE, Rocks.next(Rocks.SOUTH, up(SW, SE), null, true));
		// The rock clicked last failed and is still up: the other one in reach comes first.
		assertEquals(SW, Rocks.next(Rocks.SOUTH, up(SW, SE), SE, true));
		assertEquals(SE, Rocks.next(Rocks.SOUTH, up(SE), SE, false));
	}

	@Test
	void noRockUp()
	{
		assertNull(Rocks.next(Rocks.SOUTH, Collections.emptySet(), SW, true));
		assertNull(Rocks.next(null, ALL_UP, null, true));
	}

	@Test
	void awayFromTheCorridorTheNearestRowComesFirst()
	{
		// North of the spot (from the grinder the path comes in from the north-west).
		WorldPoint north = new WorldPoint(3165, 2910, 0);
		assertEquals(NE, Rocks.next(north, ALL_UP, null, true));
		assertEquals(SW, Rocks.next(north, up(SW, SE), null, false));
		WorldPoint south = new WorldPoint(3165, 2901, 0);
		assertEquals(SW, Rocks.next(south, ALL_UP, null, false));
	}

	@Test
	void corridorAndStandTiles()
	{
		assertTrue(Rocks.inCorridor(new WorldPoint(3165, 2905, 0)));
		assertFalse(Rocks.inCorridor(new WorldPoint(3165, 2907, 0)));
		assertEquals(Rocks.SOUTH, Rocks.standTile(SE));
		assertEquals(Rocks.NORTH, Rocks.standTile(NW));
		assertTrue(Rocks.isSpotRock(NE));
		assertFalse(Rocks.isSpotRock(new WorldPoint(3163, 2906, 0)));
	}
}
