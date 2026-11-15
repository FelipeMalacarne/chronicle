package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.awt.Rectangle;
import net.runelite.api.coords.Direction;
import net.runelite.api.coords.WorldPoint;
import org.junit.jupiter.api.Test;

/** The Telekinetic side test, against the solver's getPosition rule. */
class MtaRoomsTest
{
	/** Maze G's walls box from the run on 2026-11-15: x 3345..3356, y 9679..9690. */
	private static final Rectangle G = new Rectangle(3345, 9679, 11, 11);

	@Test
	void theLineTileAndTheTilesBeyondItCount()
	{
		assertTrue(MtaRooms.onSide(new WorldPoint(3356, 9680, 1), Direction.EAST, G));
		assertTrue(MtaRooms.onSide(new WorldPoint(3358, 9685, 1), Direction.EAST, G));
	}

	@Test
	void theCornersAreOnNoSide()
	{
		// (3357, 9679) is where the walker left the player when the solver's EAST tile was not
		// standable: y on the box's edge is outside EAST's open range.
		assertFalse(MtaRooms.onSide(new WorldPoint(3357, 9679, 1), Direction.EAST, G));
		assertFalse(MtaRooms.onSide(new WorldPoint(3345, 9679, 1), Direction.SOUTH, G));
	}

	@Test
	void eachSideIsItsOwn()
	{
		assertTrue(MtaRooms.onSide(new WorldPoint(3343, 9682, 1), Direction.WEST, G));
		assertFalse(MtaRooms.onSide(new WorldPoint(3343, 9682, 1), Direction.EAST, G));
		assertTrue(MtaRooms.onSide(new WorldPoint(3350, 9678, 1), Direction.SOUTH, G));
		assertTrue(MtaRooms.onSide(new WorldPoint(3350, 9691, 1), Direction.NORTH, G));
	}

	@Test
	void stepsToASideAreZeroOnItAndCountTheCorner()
	{
		assertEquals(0, MtaRooms.toSide(new WorldPoint(3358, 9685, 1), Direction.EAST, G));
		// From the WEST side's middle to EAST: across the box's width plus one past the edge.
		assertEquals(3356 - 3343, MtaRooms.toSide(new WorldPoint(3343, 9685, 1), Direction.EAST, G));
		// From the south-east corner (3357, 9679) to EAST: one step north into the open range.
		assertEquals(1, MtaRooms.toSide(new WorldPoint(3357, 9679, 1), Direction.EAST, G));
		// From that corner to SOUTH: one step west into the open range.
		assertEquals(2, MtaRooms.toSide(new WorldPoint(3357, 9679, 1), Direction.SOUTH, G));
	}
}
