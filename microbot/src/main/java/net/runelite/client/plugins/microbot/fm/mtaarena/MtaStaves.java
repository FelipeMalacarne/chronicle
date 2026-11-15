package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.ArrayList;
import java.util.EnumMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import net.runelite.api.EquipmentInventorySlot;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.inventory.Rs2RunePouch;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.magic.Rs2Staff;
import net.runelite.client.plugins.microbot.util.magic.Rs2Tome;
import net.runelite.client.plugins.microbot.util.magic.Runes;

/**
 * Wields the staff that covers the most of a spell's runes (staves and the worn tome give elemental
 * runes) and says whether the spell can then be cast with what is carried.
 */
final class MtaStaves
{
	private MtaStaves()
	{
	}

	static boolean equipFor(Rs2Spells spell)
	{
		Map<Runes, Integer> required = spell.getRequiredRunes();
		List<Rs2ItemModel> candidates = new ArrayList<>();
		Rs2ItemModel weapon = Rs2Equipment.get(EquipmentInventorySlot.WEAPON);
		if (weapon != null)
		{
			candidates.add(weapon);
		}
		candidates.addAll(Rs2Inventory.items().collect(java.util.stream.Collectors.toList()));
		Set<Runes> tomeRunes = new HashSet<>();
		Rs2ItemModel shield = Rs2Equipment.get(EquipmentInventorySlot.SHIELD);
		if (shield != null)
		{
			Rs2Tome tome = Rs2Magic.getRs2Tome(shield.getId());
			if (tome != Rs2Tome.NONE)
			{
				tomeRunes.addAll(tome.getRunes());
			}
		}
		Map<Runes, Integer> carried = new EnumMap<>(Runes.class);
		Rs2Inventory.items().forEach(item -> {
			Runes rune = Runes.byItemId(item.getId());
			if (rune != null)
			{
				carried.merge(rune, item.getQuantity(), Integer::sum);
			}
		});
		if (Rs2Inventory.hasRunePouch())
		{
			Rs2RunePouch.getRunes().forEach((rune, qty) -> carried.merge(rune, qty, Integer::sum));
		}
		int bestSavings = -1;
		Integer bestStaff = null;
		Set<Runes> bestProvided = null;
		for (Rs2ItemModel c : candidates)
		{
			Rs2Staff staff = Rs2Magic.getRs2Staff(c.getId());
			if (staff == Rs2Staff.NONE)
			{
				continue;
			}
			Set<Runes> provided = new HashSet<>(staff.getRunes());
			provided.addAll(tomeRunes);
			boolean castable = true;
			int savings = 0;
			for (Map.Entry<Runes, Integer> need : required.entrySet())
			{
				if (provided.contains(need.getKey()))
				{
					savings += need.getValue();
				}
				else if (carried.getOrDefault(need.getKey(), 0) < need.getValue())
				{
					castable = false;
					break;
				}
			}
			if (castable && savings > bestSavings)
			{
				bestSavings = savings;
				bestStaff = staff.getItemID();
				bestProvided = provided;
			}
		}
		if (bestStaff == null)
		{
			// No staff helps: castable only if the runes themselves are carried.
			for (Map.Entry<Runes, Integer> need : required.entrySet())
			{
				if (tomeRunes.contains(need.getKey()))
				{
					continue;
				}
				if (carried.getOrDefault(need.getKey(), 0) < need.getValue())
				{
					return false;
				}
			}
			return true;
		}
		if (!Rs2Equipment.isWearing(bestStaff))
		{
			Rs2Inventory.wear(bestStaff);
		}
		return true;
	}
}
