package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.api.WorldView;
import net.runelite.api.coords.LocalPoint;
import net.runelite.api.coords.WorldArea;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;

/** Which one-tile steps are possible, the same for the player and size-1 NPCs. */
@FunctionalInterface
interface Walls
{
	/** Whether a size-1 actor on {@code from} can step by (dx, dy), each -1, 0 or 1. */
	boolean canStep(WorldPoint from, int dx, int dy);

	/**
	 * The client's collision maps around {@code center}, read once on the client thread with
	 * RuneLite's own movement rules so planning can run off it. Tiles outside are solid.
	 */
	static Walls around(WorldPoint center, int radius)
	{
		int size = radius * 2 + 1;
		int baseX = center.getX() - radius;
		int baseY = center.getY() - radius;
		int plane = center.getPlane();
		byte[][] steps = Microbot.getClientThread().runOnClientThreadOptional(() -> {
			WorldView wv = Microbot.getClient().getTopLevelWorldView();
			byte[][] grid = new byte[size][size];
			for (int i = 0; i < size; i++)
			{
				for (int j = 0; j < size; j++)
				{
					LocalPoint lp = LocalPoint.fromWorld(wv, baseX + i, baseY + j);
					// canTravelInDirection reads the neighbouring tiles too: stay off the scene edge.
					if (lp == null || lp.getSceneX() < 1 || lp.getSceneY() < 1
						|| lp.getSceneX() > wv.getSizeX() - 2 || lp.getSceneY() > wv.getSizeY() - 2)
					{
						continue;
					}
					WorldArea tile = new WorldArea(baseX + i, baseY + j, 1, 1, plane);
					for (int d = 0; d < 8; d++)
					{
						if (tile.canTravelInDirection(wv, Direction.DX[d], Direction.DY[d]))
						{
							grid[i][j] |= (byte) (1 << d);
						}
					}
				}
			}
			return grid;
		}).orElse(new byte[size][size]);
		return (from, dx, dy) -> {
			int i = from.getX() - baseX;
			int j = from.getY() - baseY;
			int d = Direction.of(dx, dy);
			return from.getPlane() == plane && i >= 0 && j >= 0 && i < size && j < size
				&& d >= 0 && (steps[i][j] & (1 << d)) != 0;
		};
	}

	/** The eight steps, in the order the game's pathfinder tries them: W, E, S, N, SW, SE, NW, NE. */
	final class Direction
	{
		static final int[] DX = {-1, 1, 0, 0, -1, 1, -1, 1};
		static final int[] DY = {0, 0, -1, 1, -1, -1, 1, 1};

		private Direction()
		{
		}

		static int of(int dx, int dy)
		{
			for (int d = 0; d < 8; d++)
			{
				if (DX[d] == dx && DY[d] == dy)
				{
					return d;
				}
			}
			return -1;
		}
	}
}
