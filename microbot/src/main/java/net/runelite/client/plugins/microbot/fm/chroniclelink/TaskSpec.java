package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonSyntaxException;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * One typed task the executor runs (planner docs/10 phase 3), as the agent posts it to
 * /bridge/task/start: the planner's internal/core/execspec.Task, with game ids, tiles and numbers
 * only, already resolved from the catalog by the server. Nothing here is text to interpret: names
 * are only for the trail. {@link #check} refuses what the executor does not run, like the agent's
 * own validation; task-wire.json pins the JSON on both sides.
 */
final class TaskSpec
{
	static final String TRAVEL = "travel";
	static final String BANK = "bank";
	static final String BUY = "buy";

	/** The wilderness as Microbot's web walker bounds it (PathfinderConfig), on any plane. */
	static final int WILD_MIN_X = 2944;
	static final int WILD_MAX_X = 3391;
	static final int WILD_MIN_Y = 3525;
	static final int WILD_MAX_Y = 3972;
	static final int WILD_UNDER_MIN_X = 2944;
	static final int WILD_UNDER_MAX_X = 3461;
	static final int WILD_UNDER_MIN_Y = 9918;
	static final int WILD_UNDER_MAX_Y = 10375;

	private static final int MAX_COORD = 16_383;
	private static final int MAX_AREA_SIDE = 256;
	private static final int MAX_POLYGON = 256;
	private static final int MAX_BANK = 39;
	private static final int MAX_IDS = 16;
	private static final int MAX_NPCS = 32;
	private static final int MAX_BUY = 10_000;
	private static final int MAX_GAME_ID = 1 << 20;
	private static final Set<String> NOT_YET = new HashSet<>(Arrays.asList("talk", "interact", "train", "manual", "quest"));

	String type;
	Travel travel;
	Bank bank;
	Buy buy;

	static final class Tile
	{
		int x;
		int y;
		int plane;
	}

	static final class Area
	{
		int plane;
		int minX;
		int minY;
		int maxX;
		int maxY;
		int[][] polygon;

		/** In the box and, with a polygon, in it or on its edge (as the planner's Area.Contains). */
		boolean contains(int x, int y, int p)
		{
			if (p != plane || x < minX || x > maxX || y < minY || y > maxY)
			{
				return false;
			}
			if (polygon == null || polygon.length < 3)
			{
				return true;
			}
			boolean inside = false;
			int n = polygon.length;
			for (int i = 0, j = n - 1; i < n; j = i, i++)
			{
				int xi = polygon[i][0], yi = polygon[i][1];
				int xj = polygon[j][0], yj = polygon[j][1];
				if (onSegment(x, y, xi, yi, xj, yj))
				{
					return true;
				}
				if ((yi > y) != (yj > y))
				{
					long lhs = (long) (x - xi) * (yj - yi);
					long rhs = (long) (xj - xi) * (y - yi);
					if ((yj > yi && lhs < rhs) || (yj < yi && lhs > rhs))
					{
						inside = !inside;
					}
				}
			}
			return inside;
		}

		boolean inWilderness()
		{
			return boxInWilderness(minX, minY, maxX, maxY);
		}
	}

	static final class Travel
	{
		String location;
		String name;
		Tile target;
		Area area;
		/** The destination touches the wilderness: only then may the walk enter it. */
		boolean wilderness;
	}

	static final class Bank
	{
		List<BankItem> items;
	}

	static final class BankItem
	{
		String name;
		/** Unnoted ids, preferred first: any one of them, Qty of it. */
		int[] ids;
		int qty;
		boolean noted;
		boolean equip;
		boolean optional;
	}

	static final class Buy
	{
		String shop;
		String shopName;
		/** The shop keepers' NPC ids (every version). */
		int[] npcs;
		int item;
		String itemName;
		/** How many to buy now. */
		int qty;
	}

	/** Parses a task from the start body's "task"; IllegalArgumentException when it is not one. */
	static TaskSpec parse(Gson gson, JsonElement json)
	{
		TaskSpec t;
		try
		{
			t = gson.fromJson(json, TaskSpec.class);
		}
		catch (JsonSyntaxException e)
		{
			throw new IllegalArgumentException("task: not a task");
		}
		if (t == null)
		{
			throw new IllegalArgumentException("task: missing");
		}
		t.check();
		return t;
	}

	/** Refuses what the executor does not run, as the agent's validation does. */
	void check()
	{
		int set = (travel != null ? 1 : 0) + (bank != null ? 1 : 0) + (buy != null ? 1 : 0);
		if (type != null && NOT_YET.contains(type))
		{
			throw new IllegalArgumentException("task: " + type + " is not executed yet (phase 3 runs travel, bank and buy)");
		}
		if (TRAVEL.equals(type) && set == 1 && travel != null)
		{
			checkTravel();
		}
		else if (BANK.equals(type) && set == 1 && bank != null)
		{
			checkBank();
		}
		else if (BUY.equals(type) && set == 1 && buy != null)
		{
			checkBuy();
		}
		else
		{
			throw new IllegalArgumentException("task: a travel, bank or buy with only its own parameters");
		}
	}

	private void checkTravel()
	{
		Area a = travel.area;
		Tile t = travel.target;
		require(name(travel.name) && travel.location != null && travel.location.startsWith("location:"), "travel: location and name");
		require(a != null && t != null, "travel: area and target");
		require(a.plane >= 0 && a.plane <= 3 && a.minX >= 0 && a.minY >= 0 && a.maxX <= MAX_COORD && a.maxY <= MAX_COORD
			&& a.minX <= a.maxX && a.minY <= a.maxY, "travel: area out of the world or empty");
		require(a.maxX - a.minX < MAX_AREA_SIDE && a.maxY - a.minY < MAX_AREA_SIDE, "travel: an area is at most 256 tiles across");
		if (a.polygon != null)
		{
			require(a.polygon.length >= 3 && a.polygon.length <= MAX_POLYGON, "travel: a polygon has 3 to 256 points");
			for (int[] p : a.polygon)
			{
				require(p != null && p.length == 2 && p[0] >= a.minX && p[0] <= a.maxX && p[1] >= a.minY && p[1] <= a.maxY,
					"travel: a polygon point is outside the box");
			}
		}
		require(a.contains(t.x, t.y, t.plane), "travel: the target is not inside the area");
		require(travel.wilderness == a.inWilderness(), "travel: the wilderness flag does not match the area");
	}

	private void checkBank()
	{
		require(bank.items != null && !bank.items.isEmpty() && bank.items.size() <= MAX_BANK, "bank: 1 to 39 items");
		boolean required = false;
		for (BankItem it : bank.items)
		{
			require(it != null && name(it.name), "bank: an item has a name");
			require(it.ids != null && it.ids.length > 0 && it.ids.length <= MAX_IDS, "bank: an item has 1 to 16 ids");
			for (int id : it.ids)
			{
				require(id > 0 && id < MAX_GAME_ID, "bank: bad item id");
			}
			require(it.qty >= 1, "bank: qty of at least 1");
			require(!(it.noted && it.equip), "bank: a worn item is not noted");
			required |= !it.optional;
		}
		require(required, "bank: at least one item is required");
	}

	private void checkBuy()
	{
		require(buy.shop != null && buy.shop.startsWith("shop:") && name(buy.shopName) && name(buy.itemName), "buy: shop and names");
		require(buy.npcs != null && buy.npcs.length > 0 && buy.npcs.length <= MAX_NPCS, "buy: 1 to 32 shopkeeper ids");
		for (int id : buy.npcs)
		{
			require(id >= 0 && id < MAX_GAME_ID, "buy: bad shopkeeper id");
		}
		require(buy.item > 0 && buy.item < MAX_GAME_ID, "buy: bad item id");
		require(buy.qty >= 1 && buy.qty <= MAX_BUY, "buy: qty 1 to 10000");
	}

	/** What the trail calls the task. */
	String label()
	{
		if (travel != null)
		{
			return "ir até " + travel.name;
		}
		if (bank != null)
		{
			return "banco: " + bank.items.size() + (bank.items.size() == 1 ? " item" : " itens");
		}
		return "comprar " + buy.qty + " " + buy.itemName + " em " + buy.shopName;
	}

	static boolean inWilderness(int x, int y)
	{
		return (x >= WILD_MIN_X && x <= WILD_MAX_X && y >= WILD_MIN_Y && y <= WILD_MAX_Y)
			|| (x >= WILD_UNDER_MIN_X && x <= WILD_UNDER_MAX_X && y >= WILD_UNDER_MIN_Y && y <= WILD_UNDER_MAX_Y);
	}

	static boolean boxInWilderness(int minX, int minY, int maxX, int maxY)
	{
		return (maxX >= WILD_MIN_X && minX <= WILD_MAX_X && maxY >= WILD_MIN_Y && minY <= WILD_MAX_Y)
			|| (maxX >= WILD_UNDER_MIN_X && minX <= WILD_UNDER_MAX_X && maxY >= WILD_UNDER_MIN_Y && minY <= WILD_UNDER_MAX_Y);
	}

	private static boolean onSegment(int x, int y, int x1, int y1, int x2, int y2)
	{
		if ((long) (x - x1) * (y2 - y1) != (long) (x2 - x1) * (y - y1))
		{
			return false;
		}
		return Math.min(x1, x2) <= x && x <= Math.max(x1, x2) && Math.min(y1, y2) <= y && y <= Math.max(y1, y2);
	}

	private static boolean name(String s)
	{
		return s != null && !s.trim().isEmpty() && s.length() <= 80 && s.indexOf('\n') < 0 && s.indexOf('\r') < 0;
	}

	private static void require(boolean ok, String message)
	{
		if (!ok)
		{
			throw new IllegalArgumentException(message);
		}
	}
}
