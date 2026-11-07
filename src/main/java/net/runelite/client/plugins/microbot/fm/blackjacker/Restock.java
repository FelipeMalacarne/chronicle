package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.List;
import java.util.stream.Collectors;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.shop.Rs2Shop;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;
import net.runelite.client.plugins.microbot.util.world.Rs2WorldUtil;
import net.runelite.http.api.worlds.World;
import net.runelite.http.api.worlds.WorldResult;
import net.runelite.http.api.worlds.WorldType;

/**
 * A wine trip to the Asp & Snake bar, finished only with an inventory full of wine: leave the
 * room (curtain closed behind us so the target stays put), open pouches, drop the empty jugs,
 * buy, and hop worlds while the barman runs out. Taken when the jugs run low, when HP is low with
 * none left, and whenever a slot is free: a full inventory with no pouch is what makes a
 * pickpocket on a target that is already up fail on "inventory full" instead of starting a fight.
 */
@Slf4j
final class Restock implements Step
{
	// The barman is Ali before The Feud and Faisal after it.
	static final WorldPoint BAR = new WorldPoint(3361, 2956, 0);
	private static final String BARMAN = "the Barman";
	private static final String WINE = "Jug of wine";
	// Worlds to try on one trip before going back with what we have.
	private static final int MAX_HOPS = 6;

	private boolean trip;
	private int hops;
	/** False after a trip that ran out of worlds with stock: a free slot alone then waits for the next trip. */
	private boolean lastTripFilled = true;

	@Override
	public boolean run(Session s)
	{
		if (!trip)
		{
			boolean needed = Healing.wines() < s.config.minWines() || (s.lowHp() && !Healing.hasWine())
				|| (lastTripFilled && Rs2Inventory.emptySlotCount() > 0);
			if (!needed)
			{
				if (Rs2Shop.isOpen())
				{
					Rs2Shop.closeShop();
					return true;
				}
				return false;
			}
			trip = true;
			hops = 0;
			log.info("Blackjacker: wine trip ({} jugs of wine left)", Healing.wines());
		}
		if (Rs2Inventory.isFull() && !Rs2Inventory.hasItem(Healing.JUG))
		{
			finish(s, "full of wine", true);
			return true;
		}
		if (Rs2Shop.isOpen())
		{
			shop(s);
			return true;
		}
		Room room = s.room;
		if (room.area.contains(s.me))
		{
			Microbot.status = "Blackjacker: leaving for wine";
			if (!RoomObjects.curtainOpen(room))
			{
				RoomObjects.clickCurtain(room, "Open");
				return true;
			}
			Rs2Walker.walkFastCanvas(room.outside);
			Global.sleepUntil(() -> room.outside.equals(Rs2Player.getWorldLocation()), 3000);
			return true;
		}
		if (s.me.distanceTo(room.outside) <= 2 && RoomObjects.curtainOpen(room) && s.victim != null)
		{
			RoomObjects.clickCurtain(room, "Close");
			return true;
		}
		if (s.me.distanceTo(BAR) > 4)
		{
			Microbot.status = "Blackjacker: walking to the bar";
			Rs2Walker.walkTo(BAR, 3);
			return true;
		}
		// Pouches first, then every empty jug: the whole bag is for wine.
		if (Blackjacking.openAllPouches())
		{
			return true;
		}
		if (Rs2Inventory.hasItem(Healing.JUG))
		{
			Rs2Inventory.dropAll(Healing.JUG);
			Global.sleepUntil(() -> !Rs2Inventory.hasItem(Healing.JUG), 3000);
			return true;
		}
		Microbot.status = "Blackjacker: opening the bar's shop";
		if (Rs2Shop.openShop(BARMAN, false))
		{
			Global.sleepUntil(Rs2Shop::isOpen, 4000);
		}
		return true;
	}

	/** Buy what the barman has; when he runs out with room left, hop and come back. */
	private void shop(Session s)
	{
		if (!Rs2Inventory.isFull() && Rs2Shop.hasStock(WINE))
		{
			int before = Healing.wines();
			Microbot.status = "Blackjacker: buying wine";
			Rs2Shop.buyItem(WINE, "50");
			Global.sleepUntil(() -> Healing.wines() > before, 2400);
			s.winesBought += Healing.wines() - before;
			return;
		}
		Rs2Shop.closeShop();
		Global.sleepUntil(() -> !Rs2Shop.isOpen(), 2000);
		if (Rs2Inventory.isFull())
		{
			return;
		}
		// The shop is still open on the server for a tick after its widget goes; the world
		// switcher refuses until then ("Please finish what you're doing...").
		Global.sleep(1200, 1500);
		if (hops >= MAX_HOPS || !s.config.hopForWine())
		{
			if (Healing.wines() >= s.config.minWines())
			{
				finish(s, "the bar is out of stock", false);
			}
			else
			{
				s.stop("The bar is out of wine after " + hops + " hops (have " + Healing.wines() + ")");
			}
			return;
		}
		hop();
	}

	private void hop()
	{
		int from = Microbot.getClient().getWorld();
		int world = otherMembersWorld(from);
		if (world < 0)
		{
			return;
		}
		Microbot.status = "Blackjacker: hopping to world " + world + " for wine";
		log.info("Blackjacker: bar out of wine, hopping {} -> {}", from, world);
		// Failed attempts count too, or a refused hop would be retried forever.
		hops++;
		if (Microbot.hopToWorld(world))
		{
			Global.sleepUntil(() -> Microbot.getClient().getWorld() == world && Microbot.isLoggedIn(), 15000);
		}
		else
		{
			Global.sleep(1500, 2500);
		}
	}

	/**
	 * A random members world other than {@code current} that the account can use (no PvP,
	 * high-risk, skill-total, Deadman, speedrun or beta worlds); -1 when the list is not loaded.
	 * Pollnivneach is members-only, so a free world would refuse the hop anyway.
	 */
	private static int otherMembersWorld(int current)
	{
		WorldResult result = Microbot.getWorldService().getWorlds();
		if (result == null)
		{
			return -1;
		}
		boolean seasonal = Microbot.getClient().getWorldType().contains(net.runelite.api.WorldType.SEASONAL);
		List<World> worlds = result.getWorlds().stream()
			.filter(w -> w.getId() != current && w.getTypes().contains(WorldType.MEMBERS))
			.filter(w -> Rs2WorldUtil.isWorldAccessible(w, true, seasonal))
			.collect(Collectors.toList());
		return worlds.isEmpty() ? -1 : worlds.get(Rs2Random.between(0, worlds.size() - 1)).getId();
	}

	private void finish(Session s, String why, boolean filled)
	{
		log.info("Blackjacker: wine trip done, {} ({} jugs of wine, {} hops)", why, Healing.wines(), hops);
		trip = false;
		lastTripFilled = filled;
		if (Rs2Shop.isOpen())
		{
			Rs2Shop.closeShop();
		}
		Microbot.status = "Blackjacker: back to the room with " + Healing.wines() + " jugs of wine";
		Rs2Walker.walkTo(s.room.outside, 1);
	}
}
