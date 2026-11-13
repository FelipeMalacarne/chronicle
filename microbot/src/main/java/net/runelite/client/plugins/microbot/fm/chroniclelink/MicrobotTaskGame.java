package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.Set;
import java.util.function.BooleanSupplier;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.shop.Rs2Shop;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

import static net.runelite.client.plugins.microbot.util.Global.sleepUntil;

/**
 * {@link TaskGame} over Microbot's primitives: Rs2Walker for travel, Rs2Bank and Rs2Inventory for
 * the bank, Rs2Shop and the NPC cache for the shop. Called only from the task executor's thread.
 */
final class MicrobotTaskGame implements TaskGame
{
	/** How near a shop keeper must be: the executor never uses the web walker to reach one. */
	private static final int NEAR = 15;
	/** Arrival distance handed to the walker; the machine decides arrival by the area. */
	private static final int WALK_DISTANCE = 2;

	@Override
	public boolean loggedIn()
	{
		return Microbot.isLoggedIn();
	}

	@Override
	public WorldPoint position()
	{
		return Microbot.getRs2PlayerStateCache().getLocalPlayerPosition();
	}

	@Override
	public boolean scriptsPaused()
	{
		return Microbot.pauseAllScripts.get();
	}

	@Override
	public boolean walkerReady()
	{
		return Rs2Walker.config != null;
	}

	@Override
	public boolean walkerAvoidsWilderness()
	{
		return Rs2Walker.config != null && Rs2Walker.config.avoidWilderness();
	}

	@Override
	public boolean walk(WorldPoint target, BooleanSupplier stop)
	{
		return Rs2Walker.walkUntil(target, WALK_DISTANCE, stop);
	}

	@Override
	public void stopWalking()
	{
		Rs2Walker.clearWalkingRoute("Chronicle Link task stopped");
	}

	@Override
	public boolean bankOpen()
	{
		return Rs2Bank.isOpen();
	}

	@Override
	public boolean openBank()
	{
		// Rs2Bank.openBank clicks the nearest booth, chest or banker in its own search radius (the
		// game walks the few tiles to it); it never uses the web walker.
		return Rs2Bank.openBank();
	}

	@Override
	public void closeBank()
	{
		Rs2Bank.closeBank();
	}

	@Override
	public boolean depositAllExcept(Set<Integer> keep)
	{
		return Rs2Bank.depositAllExcept((Rs2ItemModel it) -> keep.contains(it.getId()) || (it.isNoted() && keep.contains(it.getUnNotedId())));
	}

	@Override
	public int inventoryCount(int unnotedId)
	{
		return Rs2Inventory.items(it -> it.getId() == unnotedId || (it.isNoted() && it.getUnNotedId() == unnotedId))
			.mapToInt(Rs2ItemModel::getQuantity).sum();
	}

	@Override
	public int bankCount(int unnotedId)
	{
		return Rs2Bank.isOpen() ? Rs2Bank.count(unnotedId) : 0;
	}

	@Override
	public boolean wearing(int id)
	{
		return Rs2Equipment.isWearing(id);
	}

	@Override
	public boolean withdraw(int unnotedId, int qty, boolean noted)
	{
		if (!Rs2Bank.setWithdrawAs(noted))
		{
			return false;
		}
		try
		{
			return qty == 1 ? Rs2Bank.withdrawOne(unnotedId) : Rs2Bank.withdrawX(unnotedId, qty);
		}
		finally
		{
			if (noted)
			{
				Rs2Bank.setWithdrawAs(false);
			}
		}
	}

	@Override
	public boolean wear(int id)
	{
		return Rs2Bank.isOpen() ? Rs2Bank.wearItem(id) : Rs2Inventory.wear(id);
	}

	@Override
	public boolean openShop(int[] npcIds)
	{
		Rs2NpcModel keeper = Microbot.getRs2NpcCache().query().withIds(npcIds).nearest(NEAR);
		return keeper != null && keeper.click("Trade");
	}

	@Override
	public boolean shopOpen()
	{
		return Rs2Shop.isOpen();
	}

	@Override
	public int shopStock(int itemId)
	{
		for (Rs2ItemModel it : Rs2Shop.shopItems)
		{
			if (it != null && it.getId() == itemId)
			{
				return it.getQuantity();
			}
		}
		return -1;
	}

	@Override
	public boolean buy(int itemId, int qty)
	{
		return Rs2Shop.buyItem(itemId, String.valueOf(qty));
	}

	@Override
	public void closeShop()
	{
		Rs2Shop.closeShop();
	}

	@Override
	public boolean waitUntil(BooleanSupplier cond, long timeoutMs) throws InterruptedException
	{
		if (Thread.currentThread().isInterrupted())
		{
			throw new InterruptedException();
		}
		return sleepUntil(cond, (int) timeoutMs);
	}
}
