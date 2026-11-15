package net.runelite.client.plugins.microbot.fm.mtaarena;

import static net.runelite.client.plugins.microbot.util.Global.sleepUntil;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.EnumSet;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Predicate;
import net.runelite.api.EquipmentInventorySlot;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.bank.enums.BankLocation;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.magic.Rs2Staff;
import net.runelite.client.plugins.microbot.util.magic.Rs2Tome;
import net.runelite.client.plugins.microbot.util.magic.Runes;

/**
 * A room's setup: the staff and tome that cover its spell's elemental runes ({@link MtaLoadout}),
 * the runes left to carry, and nothing else in the inventory but what the owner keeps. Everything
 * else goes in the arena's bank chest, so the inventory has room for the shapes, bones and items
 * the rooms hand out, and no coin of the owner's walks into the Alchemists' Playground.
 */
final class MtaPrep
{
	private static final Set<Runes> ELEMENTS = EnumSet.of(Runes.AIR, Runes.WATER, Runes.EARTH, Runes.FIRE);
	private static final Rs2Spells[] ENCHANT = {
		Rs2Spells.ENCHANT_SAPPHIRE_JEWELLERY, Rs2Spells.ENCHANT_EMERALD_JEWELLERY,
		Rs2Spells.ENCHANT_RUBY_JEWELLERY, Rs2Spells.ENCHANT_DIAMOND_JEWELLERY,
		Rs2Spells.ENCHANT_DRAGONSTONE_JEWELLERY, Rs2Spells.ENCHANT_ONYX_JEWELLERY,
	};
	/** The Magic levels of High Level Alchemy and Bones to Peaches (OSRS Wiki). */
	private static final int HIGH_ALCH_MAGIC = 55;

	/** What a room needs, worked out from what the account owns now. */
	static final class Plan
	{
		final MtaWorld.Room room;
		final Rs2Spells spell;
		final MtaLoadout.Choice choice;
		/** Rune item id -> how many one cast takes, for the runes no staff or tome covers. */
		final Map<Integer, Integer> runes;

		Plan(MtaWorld.Room room, Rs2Spells spell, MtaLoadout.Choice choice, Map<Integer, Integer> runes)
		{
			this.room = room;
			this.spell = spell;
			this.choice = choice;
			this.runes = runes;
		}

		/** The runes nothing provides, by name, for the status line. */
		List<String> missing()
		{
			List<String> out = new ArrayList<>();
			for (int id : runes.keySet())
			{
				if (Rs2Inventory.count(id) <= 0 && Rs2Bank.count(id) <= 0)
				{
					Runes r = Runes.byItemId(id);
					out.add(r == null ? String.valueOf(id) : r.name().toLowerCase());
				}
			}
			return out;
		}
	}

	enum Outcome
	{
		/** Worn and carried as the plan says: the room can start. */
		READY,
		/** A bank action is under way: ask again on the next loop. */
		WORKING,
		/** A rune the spell needs is neither carried nor banked. */
		MISSING
	}

	private final MtaArenaConfig config;

	MtaPrep(MtaArenaConfig config)
	{
		this.config = config;
	}

	/** The spell a room casts at this Magic level. */
	static Rs2Spells spell(MtaWorld.Room room, int magic, int enchantLevel, boolean peaches)
	{
		switch (room)
		{
			case TELEKINETIC:
				return Rs2Spells.TELEKINETIC_GRAB;
			case ALCHEMIST:
				return magic >= HIGH_ALCH_MAGIC ? Rs2Spells.HIGH_LEVEL_ALCHEMY : Rs2Spells.LOW_LEVEL_ALCHEMY;
			case ENCHANTMENT:
				return enchantLevel <= 0 ? null : ENCHANT[enchantLevel - 1];
			default:
				return peaches && magic >= MtaCalculator.PEACHES_MAGIC ? Rs2Spells.BONES_TO_PEACHES
					: Rs2Spells.BONES_TO_BANANAS;
		}
	}

	Plan plan(MtaWorld.Room room, Rs2Spells spell)
	{
		Map<Runes, Integer> required = spell.getRequiredRunes();
		Set<String> needed = new HashSet<>();
		for (Runes r : required.keySet())
		{
			if (ELEMENTS.contains(r))
			{
				needed.add(r.name());
			}
		}
		Map<String, Integer> stock = new HashMap<>();
		for (Runes r : ELEMENTS)
		{
			stock.put(r.name(), Rs2Inventory.count(r.getItemId()) + Rs2Bank.count(r.getItemId()));
		}
		MtaLoadout.Choice choice = MtaLoadout.choose(needed, ownedGear(), stock,
			wornId(EquipmentInventorySlot.WEAPON), wornId(EquipmentInventorySlot.SHIELD));
		Map<Integer, Integer> runes = new LinkedHashMap<>();
		for (Map.Entry<Runes, Integer> e : required.entrySet())
		{
			if (!ELEMENTS.contains(e.getKey()) || choice.uncovered.contains(e.getKey().name()))
			{
				runes.put(e.getKey().getItemId(), e.getValue());
			}
		}
		return new Plan(room, spell, choice, runes);
	}

	/** True when the room can start: worn as planned, a cast's runes carried and, if asked, nothing else. */
	boolean ready(Plan plan)
	{
		if (plan.choice.staff != null && !Rs2Equipment.isWearing(plan.choice.staff.itemId))
		{
			return false;
		}
		if (plan.choice.tome != null && !Rs2Equipment.isWearing(plan.choice.tome.itemId))
		{
			return false;
		}
		for (Map.Entry<Integer, Integer> e : plan.runes.entrySet())
		{
			if (Rs2Inventory.count(e.getKey()) < e.getValue())
			{
				return false;
			}
		}
		return !config.prepareInventory() || Rs2Inventory.items().allMatch(keep(plan));
	}

	/** One step towards {@link #ready}: at the arena's bank chest, wear, deposit, withdraw. */
	Outcome step(Plan plan)
	{
		if (ready(plan))
		{
			if (Rs2Bank.isOpen())
			{
				Rs2Bank.closeBank();
			}
			return Outcome.READY;
		}
		if (!Rs2Bank.isOpen())
		{
			Rs2Bank.walkToBankAndUseBank(BankLocation.MAGE_TRAINING_ARENA);
			return Outcome.WORKING;
		}
		if (!plan.missing().isEmpty())
		{
			return Outcome.MISSING;
		}
		if (config.prepareInventory() && !Rs2Inventory.items().allMatch(keep(plan)))
		{
			Rs2Bank.depositAllExcept(keep(plan));
			Rs2Inventory.waitForInventoryChanges(2000);
			return Outcome.WORKING;
		}
		for (MtaLoadout.Gear g : Arrays.asList(plan.choice.staff, plan.choice.tome))
		{
			if (g == null || Rs2Equipment.isWearing(g.itemId))
			{
				continue;
			}
			if (!Rs2Inventory.contains(g.itemId))
			{
				Rs2Bank.withdrawOne(g.itemId);
				sleepUntil(() -> Rs2Inventory.contains(g.itemId), 3000);
				return Outcome.WORKING;
			}
			Rs2Bank.wearItem(g.itemId);
			sleepUntil(() -> Rs2Equipment.isWearing(g.itemId), 3000);
			return Outcome.WORKING;
		}
		// The bank is open anyway: carry the whole stock of each rune the room spends.
		for (int id : plan.runes.keySet())
		{
			if (Rs2Bank.count(id) > 0)
			{
				Rs2Bank.withdrawAll(id);
				Rs2Inventory.waitForInventoryChanges(2000);
				return Outcome.WORKING;
			}
		}
		return Outcome.WORKING;
	}

	/** What stays in the inventory: the plan's runes, the gear on its way to be worn, the owner's keep list. */
	private Predicate<Rs2ItemModel> keep(Plan plan)
	{
		Set<Integer> ids = new HashSet<>(plan.runes.keySet());
		if (plan.choice.staff != null)
		{
			ids.add(plan.choice.staff.itemId);
		}
		if (plan.choice.tome != null)
		{
			ids.add(plan.choice.tome.itemId);
		}
		Set<String> names = keepNames();
		return item -> ids.contains(item.getId()) || names.contains(item.getName().toLowerCase());
	}

	private Set<String> keepNames()
	{
		Set<String> names = new HashSet<>();
		for (String n : config.keepItems().split(","))
		{
			if (!n.trim().isEmpty())
			{
				names.add(n.trim().toLowerCase());
			}
		}
		return names;
	}

	/** Every staff and tome the account has: carried, worn or (as the client last saw it) banked. */
	private static List<MtaLoadout.Gear> ownedGear()
	{
		Set<Integer> have = new HashSet<>();
		Rs2Inventory.items().forEach(i -> have.add(i.getId()));
		Rs2Bank.getAll().forEach(i -> have.add(i.getId()));
		have.add(wornId(EquipmentInventorySlot.WEAPON));
		have.add(wornId(EquipmentInventorySlot.SHIELD));
		List<MtaLoadout.Gear> out = new ArrayList<>();
		for (Rs2Staff s : Rs2Staff.values())
		{
			if (s != Rs2Staff.NONE && have.contains(s.getItemID()))
			{
				out.add(new MtaLoadout.Gear(s.getItemID(), false, names(s.getRunes())));
			}
		}
		for (Rs2Tome t : Rs2Tome.values())
		{
			if (t != Rs2Tome.NONE && have.contains(t.getItemID()))
			{
				out.add(new MtaLoadout.Gear(t.getItemID(), true, names(t.getRunes())));
			}
		}
		return out;
	}

	private static Set<String> names(List<Runes> runes)
	{
		Set<String> out = new HashSet<>();
		for (Runes r : runes)
		{
			out.add(r.name());
		}
		return out;
	}

	private static int wornId(EquipmentInventorySlot slot)
	{
		Rs2ItemModel item = Rs2Equipment.get(slot);
		return item == null ? -1 : item.getId();
	}
}
