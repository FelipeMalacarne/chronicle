package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.awt.Rectangle;
import java.util.Collections;
import java.util.HashSet;
import java.util.Set;
import net.runelite.api.CollisionDataFlag;
import net.runelite.api.coords.Direction;
import org.junit.jupiter.api.Test;

class MtaSceneTest
{
	/** Maze G's walls box from the run on 2026-11-15: x 3345..3356, y 9679..9690. */
	private static final Rectangle G = new Rectangle(3345, 9679, 11, 11);

	@Test
	void theLineTileAndTheTilesBeyondItCount()
	{
		assertTrue(MtaScene.onSide(3356, 9680, Direction.EAST, G));
		assertTrue(MtaScene.onSide(3358, 9685, Direction.EAST, G));
	}

	@Test
	void theCornersAreOnNoSide()
	{
		// (3357, 9679) is where the walker left the player when the solver's EAST tile was not standable.
		assertFalse(MtaScene.onSide(3357, 9679, Direction.EAST, G));
		assertFalse(MtaScene.onSide(3345, 9679, Direction.SOUTH, G));
	}

	@Test
	void stepsToASide()
	{
		assertEquals(0, MtaScene.toSide(3358, 9685, Direction.EAST, G));
		assertEquals(3356 - 3343, MtaScene.toSide(3343, 9685, Direction.EAST, G));
		assertEquals(1, MtaScene.toSide(3357, 9679, Direction.EAST, G));
		assertEquals(2, MtaScene.toSide(3357, 9679, Direction.SOUTH, G));
	}

	/** An open 12x12 scene with a wall along the east edge of column 5, rows 0..9 (a gap at the top). */
	private static int[][] walled()
	{
		int[][] f = new int[12][12];
		for (int y = 0; y < 10; y++)
		{
			f[5][y] |= CollisionDataFlag.BLOCK_MOVEMENT_EAST;
			f[6][y] |= CollisionDataFlag.BLOCK_MOVEMENT_WEST;
		}
		return f;
	}

	@Test
	void stepsGoAroundWalls()
	{
		int[][] d = MtaScene.steps(walled(), new MtaScene.Tile(5, 0));
		assertEquals(1, d[4][0]);
		// Across the wall it is up to row 10 and back down: no shortcut through it.
		assertTrue(d[6][0] > 10, "went through the wall: " + d[6][0]);
		assertEquals(10, d[5][10]);
	}

	@Test
	void aFullyBlockedTileIsNeverReached()
	{
		int[][] f = new int[6][6];
		f[3][3] = CollisionDataFlag.BLOCK_MOVEMENT_FULL;
		assertEquals(-1, MtaScene.steps(f, new MtaScene.Tile(0, 0))[3][3]);
	}

	@Test
	void theHighlightIsTheSolversTile()
	{
		// Box x 102..106, y 102..106: NORTH line y 106, x 103..105; EAST line x 106, y 103..105.
		Rectangle box = new Rectangle(102, 102, 4, 4);
		// From the west, NORTH next and EAST after: along the north line to its east end.
		assertArrayEquals(new int[]{105, 106}, MtaScene.highlight(101, 104, box, Direction.NORTH, Direction.EAST));
		// No move after: the north line's tile nearest the player.
		assertArrayEquals(new int[]{103, 106}, MtaScene.highlight(101, 104, box, Direction.NORTH, null));
	}

	@Test
	void bestIsTheHighlightWhenStandableElseTheClosestStandableTileOnTheSide()
	{
		Rectangle box = new Rectangle(102, 102, 4, 4);
		int[][] f = new int[12][12];
		MtaScene.Tile here = new MtaScene.Tile(1, 4);
		int[] aim = {105, 106};
		assertEquals(new MtaScene.Tile(5, 6),
			MtaScene.best(MtaScene.steps(f, here), 100, 100, box, Direction.NORTH, aim, Collections.emptySet()));
		// The highlight is a pillar: the next tile along the line instead, still on NORTH.
		f[5][6] = CollisionDataFlag.BLOCK_MOVEMENT_FULL;
		MtaScene.Tile other = MtaScene.best(MtaScene.steps(f, here), 100, 100, box, Direction.NORTH, aim, Collections.emptySet());
		assertTrue(MtaScene.onSide(100 + other.x, 100 + other.y, Direction.NORTH, box));
		assertEquals(1, Math.abs(100 + other.x - 105) + Math.abs(100 + other.y - 106));
		// A tile the game would not stand the player on is skipped too.
		Set<MtaScene.Tile> bad = new HashSet<>();
		bad.add(other);
		MtaScene.Tile third = MtaScene.best(MtaScene.steps(f, here), 100, 100, box, Direction.NORTH, aim, bad);
		assertFalse(other.equals(third));
	}
}
