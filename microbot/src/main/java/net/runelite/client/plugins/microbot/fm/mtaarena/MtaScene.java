package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.awt.Rectangle;
import java.util.ArrayDeque;
import java.util.Set;
import net.runelite.api.CollisionDataFlag;
import net.runelite.api.coords.Direction;

/**
 * Where to stand in the Telekinetic Theatre, worked out in scene coordinates only: the solver's maze
 * box, the player and the collision flags all live there (the Theatre is an instance, and converting
 * to the template's coordinates and back put the player a tile off). Pure, tested over made-up flags.
 */
final class MtaScene
{
	private MtaScene()
	{
	}

	/** A scene tile: x and y index the collision flags. */
	static final class Tile
	{
		final int x;
		final int y;

		Tile(int x, int y)
		{
			this.x = x;
			this.y = y;
		}

		@Override
		public boolean equals(Object o)
		{
			return o instanceof Tile && ((Tile) o).x == x && ((Tile) o).y == y;
		}

		@Override
		public int hashCode()
		{
			return x * 31 + y;
		}

		@Override
		public String toString()
		{
			return "(" + x + "," + y + ")";
		}
	}

	/**
	 * Steps from {@code start} to every tile the player can walk to, one per tile moved, diagonals
	 * included where both of their sides are open (the game's movement rule); -1 where unreachable.
	 */
	static int[][] steps(int[][] flags, Tile start)
	{
		int w = flags.length;
		int h = flags[0].length;
		int[][] dist = new int[w][h];
		for (int[] row : dist)
		{
			java.util.Arrays.fill(row, -1);
		}
		if (start.x < 0 || start.y < 0 || start.x >= w || start.y >= h)
		{
			return dist;
		}
		ArrayDeque<int[]> queue = new ArrayDeque<>();
		dist[start.x][start.y] = 0;
		queue.add(new int[]{start.x, start.y});
		int[][] moves = {{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {-1, 1}, {1, -1}, {1, 1}};
		while (!queue.isEmpty())
		{
			int[] at = queue.poll();
			for (int[] m : moves)
			{
				int nx = at[0] + m[0];
				int ny = at[1] + m[1];
				if (nx < 0 || ny < 0 || nx >= w || ny >= h || dist[nx][ny] >= 0 || !canMove(flags, at[0], at[1], m[0], m[1]))
				{
					continue;
				}
				dist[nx][ny] = dist[at[0]][at[1]] + 1;
				queue.add(new int[]{nx, ny});
			}
		}
		return dist;
	}

	private static boolean canMove(int[][] flags, int x, int y, int dx, int dy)
	{
		if (dx != 0 && dy != 0)
		{
			return canMove(flags, x, y, dx, 0) && canMove(flags, x, y, 0, dy)
				&& canMove(flags, x + dx, y, 0, dy) && canMove(flags, x, y + dy, dx, 0);
		}
		int out = dx < 0 ? CollisionDataFlag.BLOCK_MOVEMENT_WEST : dx > 0 ? CollisionDataFlag.BLOCK_MOVEMENT_EAST
			: dy < 0 ? CollisionDataFlag.BLOCK_MOVEMENT_SOUTH : CollisionDataFlag.BLOCK_MOVEMENT_NORTH;
		return (flags[x][y] & out) == 0 && (flags[x + dx][y + dy] & CollisionDataFlag.BLOCK_MOVEMENT_FULL) == 0;
	}

	/**
	 * The reachable tile on {@code side} of the maze with the fewest steps now plus the fewest from
	 * there to the side of {@code after} (one walk serving two pulls). {@code box} is the solver's,
	 * in world coordinates of this scene: world = base + scene.
	 */
	static Tile best(int[][] dist, int baseX, int baseY, Rectangle box, Direction side, Direction after, Set<Tile> bad)
	{
		Tile best = null;
		int bestScore = Integer.MAX_VALUE;
		for (int x = 0; x < dist.length; x++)
		{
			for (int y = 0; y < dist[x].length; y++)
			{
				if (dist[x][y] < 0)
				{
					continue;
				}
				int wx = baseX + x;
				int wy = baseY + y;
				if (!onSide(wx, wy, side, box) || bad.contains(new Tile(x, y)))
				{
					continue;
				}
				int score = dist[x][y] + (after == null ? 0 : toSide(wx, wy, after, box));
				if (score < bestScore)
				{
					best = new Tile(x, y);
					bestScore = score;
				}
			}
		}
		return best;
	}

	/** TelekineticRoom.getPosition's test, for any tile. */
	static boolean onSide(int x, int y, Direction side, Rectangle b)
	{
		boolean alongX = x > b.getX() && x < b.getMaxX();
		boolean alongY = y > b.getY() && y < b.getMaxY();
		switch (side)
		{
			case NORTH:
				return y >= b.getMaxY() && alongX;
			case SOUTH:
				return y <= b.getY() && alongX;
			case EAST:
				return x >= b.getMaxX() && alongY;
			case WEST:
				return x <= b.getX() && alongY;
			default:
				return false;
		}
	}

	/** Steps (ignoring walls) from a tile to the nearest tile on {@code side} of the box. */
	static int toSide(int x, int y, Direction side, Rectangle b)
	{
		int x0 = b.x;
		int x1 = (int) b.getMaxX();
		int y0 = b.y;
		int y1 = (int) b.getMaxY();
		int alongX = x <= x0 ? x0 + 1 - x : x >= x1 ? x - (x1 - 1) : 0;
		int alongY = y <= y0 ? y0 + 1 - y : y >= y1 ? y - (y1 - 1) : 0;
		switch (side)
		{
			case NORTH:
				return Math.max(0, y1 - y) + alongX;
			case SOUTH:
				return Math.max(0, y - y0) + alongX;
			case EAST:
				return Math.max(0, x1 - x) + alongY;
			case WEST:
				return Math.max(0, x - x0) + alongY;
			default:
				return 0;
		}
	}
}
