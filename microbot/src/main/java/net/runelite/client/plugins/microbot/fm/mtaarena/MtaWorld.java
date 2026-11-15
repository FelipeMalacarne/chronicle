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
			for (Room r : values())
			{
				if (r.area != null && r.area.contains(p))
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
}
