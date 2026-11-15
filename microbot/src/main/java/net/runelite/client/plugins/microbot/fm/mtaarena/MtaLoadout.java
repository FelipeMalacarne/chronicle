package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.ArrayList;
import java.util.Collection;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Which staff and which tome to wear for a room's spell: the pair that leaves the fewest of its
 * elemental runes to carry. Pure (rune names, item ids): {@link MtaPrep} feeds it what the account
 * owns and turns the answer into bank actions.
 */
final class MtaLoadout
{
	private MtaLoadout()
	{
	}

	/** A staff (weapon slot) or a tome (shield slot) and the elemental runes it gives. */
	static final class Gear
	{
		final int itemId;
		final boolean tome;
		final Set<String> gives;

		Gear(int itemId, boolean tome, Set<String> gives)
		{
			this.itemId = itemId;
			this.tome = tome;
			this.gives = gives;
		}
	}

	/** The pieces to wear (null: leave that slot as it is) and the elemental runes still to carry. */
	static final class Choice
	{
		final Gear staff;
		final Gear tome;
		final Set<String> uncovered;

		Choice(Gear staff, Gear tome, Set<String> uncovered)
		{
			this.staff = staff;
			this.tome = tome;
			this.uncovered = uncovered;
		}
	}

	/**
	 * The best pair for {@code needed} among {@code owned}. In order: the fewest runes left to carry,
	 * the fewest of those the account has none of ({@code stock}), the fewest pieces, a staff over a
	 * tome (a tome burns its pages), and what is already worn (no swap). A piece that covers nothing
	 * is never chosen, so with no elemental rune needed both slots stay as they are.
	 */
	static Choice choose(Set<String> needed, Collection<Gear> owned, Map<String, Integer> stock,
		int wornWeapon, int wornShield)
	{
		List<Gear> staves = new ArrayList<>();
		List<Gear> tomes = new ArrayList<>();
		staves.add(null);
		tomes.add(null);
		for (Gear g : owned)
		{
			if (Collections.disjoint(g.gives, needed))
			{
				continue;
			}
			(g.tome ? tomes : staves).add(g);
		}
		Choice best = null;
		int[] bestScore = null;
		for (Gear staff : staves)
		{
			for (Gear tome : tomes)
			{
				Set<String> uncovered = new HashSet<>(needed);
				if (staff != null)
				{
					uncovered.removeAll(staff.gives);
				}
				if (tome != null)
				{
					uncovered.removeAll(tome.gives);
				}
				int[] score = score(staff, tome, uncovered, stock, wornWeapon, wornShield);
				if (bestScore == null || less(score, bestScore))
				{
					best = new Choice(staff, tome, uncovered);
					bestScore = score;
				}
			}
		}
		return best;
	}

	/** Lower is better, compared position by position. */
	private static int[] score(Gear staff, Gear tome, Set<String> uncovered, Map<String, Integer> stock,
		int wornWeapon, int wornShield)
	{
		int unstocked = 0;
		for (String rune : uncovered)
		{
			if (stock.getOrDefault(rune, 0) <= 0)
			{
				unstocked++;
			}
		}
		int pieces = (staff == null ? 0 : 1) + (tome == null ? 0 : 1);
		int swaps = (staff != null && staff.itemId != wornWeapon ? 1 : 0)
			+ (tome != null && tome.itemId != wornShield ? 1 : 0);
		int id = (staff == null ? 0 : staff.itemId) * 100000 + (tome == null ? 0 : tome.itemId);
		return new int[]{uncovered.size(), unstocked, pieces, tome == null ? 0 : 1, swaps, id};
	}

	private static boolean less(int[] a, int[] b)
	{
		for (int i = 0; i < a.length; i++)
		{
			if (a[i] != b[i])
			{
				return a[i] < b[i];
			}
		}
		return false;
	}
}
