package net.runelite.client.plugins.microbot.fm.blackjacker;

import lombok.extern.slf4j.Slf4j;
import net.runelite.api.EquipmentInventorySlot;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * Ends a fight in a room without a ladder: take the blackjack off and use Knock-Out to drop
 * combat ("I need a different weapon..."), wait for the hits to stop, wield it again (wiki). If
 * hits keep coming after the unarmed knock-out it did not work: walk away and stop.
 */
@Slf4j
final class UnarmedReset implements Step
{
	private static final long CLEAR_MS = 4200;
	private static final long CLICK_MS = 600;
	// Hits taken after the unarmed knock-out before giving the trick up.
	private static final int MAX_HITS = 2;

	private enum Phase
	{
		NONE,
		UNEQUIP,
		KNOCK_OUT,
		WAIT_CLEAR,
		WIELD
	}

	private final LadderEscape ladder;
	private Phase phase = Phase.NONE;
	private long clickedAt;
	private int hitsAtKnockOut;
	private String weapon;

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
			Rs2Inventory.drop(Rs2Inventory.hasItem(Healing.JUG) ? Healing.JUG : Healing.JUG_OF_WINE);
			return;
		}
		Rs2Equipment.unEquip(EquipmentInventorySlot.WEAPON);
	}

	private void wield(Session s)
	{
		Microbot.status = "Blackjacker: resetting combat (wield)";
		if (Rs2Equipment.get(EquipmentInventorySlot.WEAPON) != null)
		{
			phase = Phase.NONE;
			s.events.forgetFight();
			return;
		}
		if (s.now - clickedAt >= CLICK_MS)
		{
			clickedAt = s.now;
			Rs2Inventory.interact(weapon, "Wield", true);
		}
	}
}
