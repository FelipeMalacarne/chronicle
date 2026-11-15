package net.runelite.client.plugins.microbot.fm.mtaarena;

import net.runelite.api.coords.WorldArea;
import net.runelite.api.coords.WorldPoint;

/**
 * Where things are in the Mage Training Arena: object, NPC, item and widget ids and the rooms'
 * areas. Read from the arena itself and cross-checked against the Plugin Hub's Mage Training Arena
 * plugin; not yet verified one by one in game (see the plugin's AGENTS.md entry).
 */
final class MtaWorld
{
	private MtaWorld()
	{
	}

	/** The rooms with a script of ours, in the planner's room names. */
	enum Room
	{
		TELEKINETIC(23673, null, null),
		ALCHEMIST(23675, new WorldArea(3345, 9616, 38, 38, 2), new WorldPoint(3364, 9623, 2)),
		ENCHANTMENT(23674, new WorldArea(3339, 9617, 50, 46, 0), new WorldPoint(3363, 9640, 0)),
		GRAVEYARD(23676, new WorldArea(3336, 9614, 54, 51, 1), new WorldPoint(3363, 9640, 1));

		/** The lobby's teleporter object that enters this room. */
		final int teleporter;
		/** The room's floor (null for the Telekinetic Theatre: its mazes are separate areas). */
		final WorldArea area;
		final WorldPoint exit;

		Room(int teleporter, WorldArea area, WorldPoint exit)
		{
			this.teleporter = teleporter;
			this.area = area;
			this.exit = exit;
		}

		static Room at(WorldPoint p)
		{
			if (p == null)
			{
				return null;
			}
			if (Maze.at(p) != null)
			{
				return TELEKINETIC;
			}
			for (Room r : values())
			{
				if (r.area != null && r.area.distanceTo(p) == 0)
				{
					return r;
				}
			}
			return null;
		}

		static Room of(MtaPlanner.Room planned)
		{
			return valueOf(planned.name());
		}
	}

	/** In front of the lobby's teleporters. */
	static final WorldPoint LOBBY = new WorldPoint(3363, 3318, 0);
	/** In front of the Rewards Guardian. */
	static final WorldPoint SHOP = new WorldPoint(3365, 3318, 0);
	static final int REWARDS_GUARDIAN = 5984;
	/** The Rewards Guardian's shop: the interface group, its item list and its buy button. */
	static final int SHOP_GROUP = 197;
	static final int SHOP_ITEMS = 11;
	static final int SHOP_BUY = 9;
	/** The portal back to the lobby, inside every room. */
	static final int EXIT_PORTAL = 23677;
	/** "You must talk to the Entrance Guardian": the first visit. */
	static final String FIRST_VISIT_TEXT = "You must talk to the Entrance Guardian";

	/** The Enchanting Chamber: the shapes' pedestals, items and the bonus indicator of each. */
	enum Shape
	{
		CUBE(23694, 6899, 195, 10),
		CYLINDER(23695, 6898, 195, 12),
		PENTAMID(23697, 6901, 195, 14),
		ICOSAHEDRON(23696, 6900, 195, 16);

		final int pedestal;
		final int itemId;
		final int widgetGroup;
		final int widgetChild;

		Shape(int pedestal, int itemId, int widgetGroup, int widgetChild)
		{
			this.pedestal = pedestal;
			this.itemId = itemId;
			this.widgetGroup = widgetGroup;
			this.widgetChild = widgetChild;
		}
	}

	/** The dragonstone that appears on the floor of the Enchanting Chamber: worth double. */
	static final int DRAGONSTONE = 6903;
	/** The hole the enchanted shapes go in. */
	static final int ENCHANT_HOLE = 23698;
	static final WorldPoint ENCHANT_HOLE_SPOT = new WorldPoint(3363, 9640, 0);
	/** Where the pedestals are when none is in sight. */
	static final WorldPoint[] ENCHANT_SEARCH = {
		new WorldPoint(3347, 9655, 0), new WorldPoint(3378, 9655, 0),
		new WorldPoint(3379, 9624, 0), new WorldPoint(3346, 9624, 0),
	};

	/** The Entrance Guardian, and where to talk to them on the first visit. */
	static final int ENTRANCE_GUARDIAN = 5978;
	static final WorldPoint ENTRANCE_GUARDIAN_SPOT = new WorldPoint(3363, 3304, 0);

	/** The enchanted shape's item: what the hole takes. */
	static final int ENCHANTED_SHAPE = 6902;

	/** Alchemy room: the items the cupboards hold (all but the one worth the most are dropped). */
	static final int[] ALCHEMY_ITEMS = {6897, 6896, 6895, 6894, 6893};
	static final WorldPoint ALCHEMY_CENTER = new WorldPoint(3364, 9636, 2);
	/** Where the Alchemists' Playground turns coins into points, a hundred to the point. */
	static final String COIN_COLLECTOR = "Coin Collector";
	/** The Playground's own coins (not the game's 995): what its alchemy gives and its collector takes. */
	static final int COINS = 8890;

	/** Creature Graveyard: the bones pile, the fruit chute, and the fruit. */
	static final WorldPoint BONES_PILE = new WorldPoint(3352, 9637, 1);
	static final WorldPoint FRUIT_CHUTE = new WorldPoint(3354, 9639, 1);
	static final int BANANAS = 1963;
	static final int PEACHES = 6883;
	static final String BONES = "Animals' bones";

	/** The Telekinetic Theatre's ten mazes: the floor, the way out and where to stand to see the maze. */
	enum Maze
	{
		A(new WorldArea(3324, 9697, 34, 30, 0), new WorldPoint(3334, 9718, 0), new WorldPoint(3339, 9713, 0)),
		B(new WorldArea(3358, 9704, 34, 23, 0), new WorldPoint(3379, 9716, 0), new WorldPoint(3373, 9716, 0)),
		C(new WorldArea(3327, 9669, 31, 27, 0), new WorldPoint(3352, 9690, 0), new WorldPoint(3346, 9683, 0)),
		D(new WorldArea(3361, 9664, 28, 39, 0), new WorldPoint(3373, 9696, 0), new WorldPoint(3373, 9681, 0)),
		E(new WorldArea(3331, 9702, 37, 24, 1), new WorldPoint(3362, 9713, 1), new WorldPoint(3349, 9713, 1)),
		F(new WorldArea(3368, 9704, 22, 22, 1), new WorldPoint(3377, 9706, 1), new WorldPoint(3382, 9714, 1)),
		G(new WorldArea(3332, 9671, 35, 29, 1), new WorldPoint(3355, 9693, 1), new WorldPoint(3354, 9688, 1)),
		H(new WorldArea(3367, 9669, 25, 34, 1), new WorldPoint(3382, 9698, 1), new WorldPoint(3381, 9685, 1)),
		I(new WorldArea(3332, 9696, 42, 31, 2), new WorldPoint(3359, 9701, 2), new WorldPoint(3351, 9710, 2)),
		J(new WorldArea(3331, 9667, 45, 27, 2), new WorldPoint(3368, 9680, 2), new WorldPoint(3347, 9679, 2));

		final WorldArea area;
		final WorldPoint exit;
		final WorldPoint maze;

		Maze(WorldArea area, WorldPoint exit, WorldPoint maze)
		{
			this.area = area;
			this.exit = exit;
			this.maze = maze;
		}

		static Maze at(WorldPoint p)
		{
			if (p == null)
			{
				return null;
			}
			for (Maze m : values())
			{
				if (m.area.distanceTo(p) == 0)
				{
					return m;
				}
			}
			return null;
		}
	}

	/** The widget that shows a room's points while inside it (group, child), by room. */
	static int[] roomPointsWidget(Room room)
	{
		switch (room)
		{
			case TELEKINETIC:
				return new int[]{198, 6};
			case ALCHEMIST:
				return new int[]{194, 6};
			case ENCHANTMENT:
				return new int[]{195, 6};
			default:
				return new int[]{196, 6};
		}
	}
}
