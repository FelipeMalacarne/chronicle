package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.widgets.Widget;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.Script;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaPlanner.Kind;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaPlanner.Step;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.keyboard.Rs2Keyboard;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;
import net.runelite.client.plugins.microbot.util.widget.Rs2Widget;
import net.runelite.client.plugins.skillcalculator.skills.MagicAction;

/**
 * Runs the plan: asks {@link MtaPlanner} what is next, buys at the Rewards Guardian or earns the
 * missing points in a room, and asks again. Acts only while the plugin's "Run the plan" setting is on.
 * The Enchanting Chamber is automated; for the other rooms it says which one to do by hand and
 * resumes when the points change. One short step per loop: the planner reads the points again
 * each time, so a purchase or a level-up needs no special case.
 */
@Slf4j
final class MtaRunner extends Script
{
	private static final int LOOP_MS = 600;
	/** Give up on a purchase that did not take after this many tries and say so. */
	private static final int BUY_TRIES = 3;
	private static final int STAFF_AIR = 1381;
	private static final int STAFF_WATER = 1383;
	private static final int STAFF_EARTH = 1385;
	private static final int STAFF_FIRE = 1387;
	private static final int COSMIC_RUNE = 564;

	/** Lvl-1 to Lvl-6 Enchant, index = level - 1. */
	private static final MagicAction[] ENCHANT = {
		MagicAction.ENCHANT_SAPPHIRE_JEWELLERY, MagicAction.ENCHANT_EMERALD_JEWELLERY,
		MagicAction.ENCHANT_RUBY_JEWELLERY, MagicAction.ENCHANT_DIAMOND_JEWELLERY,
		MagicAction.ENCHANT_DRAGONSTONE_JEWELLERY, MagicAction.ENCHANT_ONYX_JEWELLERY,
	};

	private final MtaArenaPlugin plugin;
	private final MtaArenaConfig config;
	private String boughtTry = "";
	private int tries;
	/** The Enchanting Chamber: true from a full inventory until the shapes are cast and deposited. */
	private boolean casting;

	MtaRunner(MtaArenaPlugin plugin, MtaArenaConfig config)
	{
		this.plugin = plugin;
		this.config = config;
	}

	public boolean run()
	{
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				loop();
			}
			catch (Exception ex)
			{
				log.error("MTA runner loop failed", ex);
			}
		}, 0, LOOP_MS, TimeUnit.MILLISECONDS);
		return true;
	}

	private static void status(String text)
	{
		Microbot.status = "MTA: " + text;
	}

	private void loop()
	{
		if (!Microbot.isLoggedIn() || !super.run() || !config.automate())
		{
			return;
		}
		MtaArenaPlugin.Snapshot s = plugin.snapshot();
		if (!s.pointsKnown)
		{
			status("open the points panel once so the plugin knows your points");
			return;
		}
		Step step = s.step;
		if (step == null)
		{
			status(s.message == null ? "nothing chosen" : s.message);
			return;
		}
		switch (step.kind)
		{
			case DONE:
				status("every chosen reward is yours");
				return;
			case BLOCKED:
				status("Magic is too low for the Enchanting Chamber: " + step.reward + " waits");
				return;
			case BUY:
				buy(step.reward);
				return;
			case EARN:
				earn(step, s);
				return;
			default:
				return;
		}
	}

	// ---- the Rewards Guardian ------------------------------------------------------------

	private void buy(String id)
	{
		MtaReward r = MtaCalculator.reward(id);
		if (!id.equals(boughtTry))
		{
			boughtTry = id;
			tries = 0;
		}
		if (tries >= BUY_TRIES)
		{
			status("could not buy " + r.name + " after " + BUY_TRIES + " tries: do it by hand");
			return;
		}
		if (r.requires != null && !Rs2Inventory.contains(MtaCalculator.reward(r.requires).itemId))
		{
			status("bring the " + MtaCalculator.reward(r.requires).name + " to the shop to upgrade it");
			return;
		}
		if (MtaWorld.Room.at(Rs2Player.getWorldLocation()) != null)
		{
			leaveRoom();
			return;
		}
		if (!Rs2Widget.isWidgetVisible(MtaWorld.SHOP_GROUP, MtaWorld.SHOP_ITEMS))
		{
			status("walking to the Rewards Guardian");
			if (!near(MtaWorld.SHOP, 3) && !Rs2Walker.walkTo(MtaWorld.SHOP))
			{
				return;
			}
			Microbot.getRs2NpcCache().query().withId(MtaWorld.REWARDS_GUARDIAN).interact("Trade-with");
			sleepUntil(() -> Rs2Widget.isWidgetVisible(MtaWorld.SHOP_GROUP, MtaWorld.SHOP_ITEMS), 5000);
			return;
		}
		Widget list = Rs2Widget.getWidget(MtaWorld.SHOP_GROUP, MtaWorld.SHOP_ITEMS);
		Widget[] items = list == null ? null : list.getDynamicChildren();
		int index = findItem(items, r.itemId);
		if (index < 0)
		{
			status(r.name + " is not in the shop's list (the item id may be wrong)");
			tries = BUY_TRIES;
			return;
		}
		status("buying " + r.name);
		tries++;
		Rs2Widget.clickWidgetFast(items[index], index);
		sleepGaussian(600, 150);
		Rs2Widget.clickWidget(MtaWorld.SHOP_GROUP, MtaWorld.SHOP_BUY);
		sleepGaussian(600, 150);
		Rs2Keyboard.keyPress(27);
	}

	private static int findItem(Widget[] items, int itemId)
	{
		if (items == null)
		{
			return -1;
		}
		for (int i = 0; i < items.length; i++)
		{
			if (items[i] != null && items[i].getItemId() == itemId)
			{
				return i;
			}
		}
		return -1;
	}

	// ---- rooms ---------------------------------------------------------------------------

	private void earn(Step step, MtaArenaPlugin.Snapshot s)
	{
		MtaWorld.Room room = MtaWorld.Room.of(step.room);
		if (room != MtaWorld.Room.ENCHANTMENT)
		{
			status("do the " + room.name().toLowerCase() + " room by hand (" + step.points + " points for "
				+ step.reward + "); the plugin goes on when they are in");
			return;
		}
		if (!enterRoom(room))
		{
			return;
		}
		enchant(s);
	}

	/** True when the player is in {@code room}; otherwise takes one step towards it. */
	private boolean enterRoom(MtaWorld.Room room)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		MtaWorld.Room at = MtaWorld.Room.at(here);
		if (at == room)
		{
			return true;
		}
		if (at != null)
		{
			leaveRoom();
			return false;
		}
		status("entering the " + room.name().toLowerCase() + " room");
		if (Rs2Widget.hasWidget(MtaWorld.FIRST_VISIT_TEXT))
		{
			status("talk to the Entrance Guardian first (first visit)");
			return false;
		}
		if (!near(MtaWorld.LOBBY, 4) && !Rs2Walker.walkTo(MtaWorld.LOBBY))
		{
			return false;
		}
		Microbot.getRs2TileObjectCache().query().interact(room.teleporter, "Enter");
		Rs2Player.waitForAnimation();
		return false;
	}

	private void leaveRoom()
	{
		MtaWorld.Room at = MtaWorld.Room.at(Rs2Player.getWorldLocation());
		if (at == null)
		{
			return;
		}
		status("leaving the " + at.name().toLowerCase() + " room");
		if (!near(at.exit, 3) && !Rs2Walker.walkTo(at.exit))
		{
			return;
		}
		Microbot.getRs2TileObjectCache().query().interact(MtaWorld.EXIT_PORTAL, "Enter");
		Rs2Player.waitForWalking();
	}

	// ---- the Enchanting Chamber ----------------------------------------------------------

	private void enchant(MtaArenaPlugin.Snapshot s)
	{
		int level = MtaCalculator.enchantLevelFor(Microbot.getClient().getRealSkillLevel(net.runelite.api.Skill.MAGIC),
			config.enchantLevel());
		if (level == 0)
		{
			status("no Enchant castable");
			return;
		}
		if (!Rs2Inventory.contains(COSMIC_RUNE))
		{
			status("no cosmic runes in the inventory");
			return;
		}
		wieldStaff(level);

		MtaWorld.Shape carried = carriedShape();
		boolean dragonstone = Rs2Inventory.contains(MtaWorld.DRAGONSTONE);
		if (!casting && Rs2Inventory.emptySlotCount() <= 1)
		{
			casting = true;
		}
		if (casting)
		{
			if (carried != null || dragonstone)
			{
				status("enchanting (Lvl-" + level + ")");
				int item = dragonstone ? MtaWorld.DRAGONSTONE : carried.itemId;
				if (Rs2Magic.cast(ENCHANT[level - 1]))
				{
					sleepGaussian(600, 150);
					Rs2Inventory.interact(item);
					Rs2Inventory.waitForInventoryChanges(3000);
				}
				return;
			}
			status("depositing");
			if (!near(MtaWorld.ENCHANT_HOLE_SPOT, 2) && !Rs2Walker.walkTo(MtaWorld.ENCHANT_HOLE_SPOT))
			{
				return;
			}
			Microbot.getRs2TileObjectCache().query().interact(MtaWorld.ENCHANT_HOLE, "Deposit");
			Rs2Player.waitForWalking();
			casting = false;
			return;
		}
		status("collecting shapes");
		if (Microbot.getRs2TileItemCache().query().withId(MtaWorld.DRAGONSTONE).interact("Take"))
		{
			Rs2Inventory.waitForInventoryChanges(5000);
			return;
		}
		MtaWorld.Shape bonus = bonusShape();
		if (bonus == null)
		{
			return;
		}
		if (Microbot.getRs2TileObjectCache().query().interact(bonus.pedestal, "Take-from"))
		{
			Rs2Inventory.waitForInventoryChanges(1000);
			return;
		}
		// No pedestal in sight: walk to one of the corners where they stand.
		WorldPoint spot = MtaWorld.ENCHANT_SEARCH[(int) (Math.random() * MtaWorld.ENCHANT_SEARCH.length)];
		Rs2Walker.walkTo(spot);
		Rs2Player.waitForWalking();
	}

	private static MtaWorld.Shape bonusShape()
	{
		for (MtaWorld.Shape shape : MtaWorld.Shape.values())
		{
			if (Rs2Widget.isWidgetVisible(shape.widgetGroup, shape.widgetChild))
			{
				return shape;
			}
		}
		return null;
	}

	private static MtaWorld.Shape carriedShape()
	{
		for (MtaWorld.Shape shape : MtaWorld.Shape.values())
		{
			if (Rs2Inventory.contains(shape.itemId))
			{
				return shape;
			}
		}
		return null;
	}

	/** The staff the Enchant level's elemental runes come from, when one is in the inventory. */
	private static void wieldStaff(int level)
	{
		int[] staff = {STAFF_WATER, STAFF_AIR, STAFF_FIRE, STAFF_EARTH};
		if (level >= 1 && level <= 4 && Rs2Inventory.contains(staff[level - 1]))
		{
			Rs2Inventory.wield(staff[level - 1]);
		}
		else if (level == 6 && Rs2Inventory.contains(STAFF_EARTH))
		{
			Rs2Inventory.wield(STAFF_EARTH);
		}
	}

	private static boolean near(WorldPoint p, int distance)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		return here != null && here.getPlane() == p.getPlane() && here.distanceTo(p) <= distance;
	}
}
