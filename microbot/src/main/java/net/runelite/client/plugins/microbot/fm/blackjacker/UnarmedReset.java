package net.runelite.client.plugins.microbot.fm.blackjacker;

import lombok.extern.slf4j.Slf4j;
import net.runelite.api.EquipmentInventorySlot;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.tileitem.models.Rs2TileItemModel;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * Ends a fight in a room without a ladder: take the blackjack off and use Knock-Out to drop
 * combat ("I need a different weapon..."), wait for the hits to stop, wield it again (wiki). If
 * hits keep coming after the unarmed knock-out it did not work: walk away and stop. A full
 * inventory makes room for the blackjack by dropping a jug, taken back after: the free slot would
 * send the run on a wine trip however much wine is left.
 */
@Slf4j
final class UnarmedReset implements Step
{
	private static final long CLEAR_MS = 4200;
	private static final long CLICK_MS = 600;
	// Hits taken after the unarmed knock-out before giving the trick up.
	private static final int MAX_HITS = 2;
	// Taking the dropped jug back: how near it must lie and how long to try.
	private static final int PICK_UP_RANGE = 3;
	private static final long PICK_UP_MS = 4000;

	private enum Phase
	{
		NONE,
		UNEQUIP,
		KNOCK_OUT,
		WAIT_CLEAR,
		WIELD,
		PICK_UP
	}

	private final LadderEscape ladder;
	private Phase phase = Phase.NONE;
	private long clickedAt;
	private int hitsAtKnockOut;
	private String weapon;
	/** Item id dropped to make room for the blackjack, or -1. */
	private int dropped = -1;
	private long pickUpSince;

	UnarmedReset(LadderEscape ladder)
	{
		this.ladder = ladder;
	}

	@Override
	public boolean run(Session s)
	{
		if (phase == Phase.NONE)
		{
			if (!s.mustEscape || s.victim == null || ladder.available(s.room))
			{
				return false;
			}
			s.combatResets++;
			log.info("Blackjacker: under attack at {}/{} HP, unarmed reset (#{})", s.hp(), s.events.maxHp(), s.combatResets);
			phase = Phase.UNEQUIP;
			clickedAt = 0;
			dropped = -1;
		}
		switch (phase)
		{
			case UNEQUIP:
				unequip(s);
				break;
			case KNOCK_OUT:
				Microbot.status = "Blackjacker: resetting combat (knock-out unarmed)";
				if (s.victim != null)
				{
					s.victim.click("Knock-Out");
				}
				hitsAtKnockOut = s.events.hits();
				phase = Phase.WAIT_CLEAR;
				break;
			case WAIT_CLEAR:
				Microbot.status = "Blackjacker: resetting combat (waiting for hits to stop)";
				if (s.events.hits() - hitsAtKnockOut >= MAX_HITS)
				{
					s.stop("The unarmed knock-out did not stop the " + s.target.npcName + " attacking");
					log.warn("Blackjacker: {}; walking away", s.stopReason);
					phase = Phase.NONE;
					Rs2Walker.walkTo(Restock.BAR, 3);
				}
				else if (s.now - s.events.lastHitAt() >= CLEAR_MS)
				{
					phase = Phase.WIELD;
				}
				break;
			case WIELD:
				wield(s);
				break;
			case PICK_UP:
				if (!pickUp(s))
				{
					return false;
				}
				break;
			default:
				phase = Phase.NONE;
		}
		return true;
	}

	private void unequip(Session s)
	{
		Microbot.status = "Blackjacker: resetting combat (unequip)";
		Rs2ItemModel wielded = Rs2Equipment.get(EquipmentInventorySlot.WEAPON);
		if (wielded == null)
		{
			phase = Phase.KNOCK_OUT;
			return;
		}
		if (s.now - clickedAt < CLICK_MS)
		{
			return;
		}
		clickedAt = s.now;
		weapon = wielded.getName();
		if (Rs2Inventory.isFull())
		{
			dropped = Rs2Inventory.hasItem(Healing.JUG) ? Healing.JUG : Healing.JUG_OF_WINE;
			Rs2Inventory.drop(dropped);
			return;
		}
		Rs2Equipment.unEquip(EquipmentInventorySlot.WEAPON);
	}

	private void wield(Session s)
	{
		Microbot.status = "Blackjacker: resetting combat (wield)";
		if (Rs2Equipment.get(EquipmentInventorySlot.WEAPON) != null)
		{
			s.events.forgetFight();
			phase = dropped < 0 ? Phase.NONE : Phase.PICK_UP;
			pickUpSince = s.now;
			clickedAt = 0;
			return;
		}
		if (s.now - clickedAt >= CLICK_MS)
		{
			clickedAt = s.now;
			Rs2Inventory.interact(weapon, "Wield", true);
		}
	}

	/** Takes the dropped item back; false once there is nothing left to do (or a fight is back). */
	private boolean pickUp(Session s)
	{
		Rs2TileItemModel item = s.attacked || Rs2Inventory.isFull() || s.now - pickUpSince > PICK_UP_MS
			? null
			: Microbot.getRs2TileItemCache().query().withId(dropped).nearestOnClientThread(PICK_UP_RANGE);
		if (item == null)
		{
			phase = Phase.NONE;
			return false;
		}
		Microbot.status = "Blackjacker: resetting combat (taking the jug back)";
		if (s.now - clickedAt >= CLICK_MS)
		{
			clickedAt = s.now;
			item.pickup();
		}
		return true;
	}
}
