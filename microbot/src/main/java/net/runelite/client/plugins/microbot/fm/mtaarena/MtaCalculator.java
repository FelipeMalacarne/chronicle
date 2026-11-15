package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.ArrayList;
import java.util.Collection;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * What the rewards the owner chose cost in runes: the pizazz points still missing in each room and
 * the law, nature and cosmic runes that earning them takes. The same arithmetic as
 * internal/core/mta/mta.go, pinned by the same vectors (MtaCalculatorTest). Pure: no client state.
 */
final class MtaCalculator
{
	private static final String PEACHES = "bones_to_peaches";
	private static final Map<String, MtaReward> BY_ID = new HashMap<>();

	static
	{
		for (MtaReward r : MtaData.REWARDS)
		{
			BY_ID.put(r.id, r);
		}
	}

	private MtaCalculator()
	{
	}

	/** An amount of pizazz points in each room. */
	static final class Points
	{
		final int telekinetic;
		final int graveyard;
		final int enchantment;
		final int alchemist;

		Points(int telekinetic, int graveyard, int enchantment, int alchemist)
		{
			this.telekinetic = telekinetic;
			this.graveyard = graveyard;
			this.enchantment = enchantment;
			this.alchemist = alchemist;
		}

		@Override
		public boolean equals(Object o)
		{
			if (!(o instanceof Points))
			{
				return false;
			}
			Points p = (Points) o;
			return telekinetic == p.telekinetic && graveyard == p.graveyard
				&& enchantment == p.enchantment && alchemist == p.alchemist;
		}

		@Override
		public int hashCode()
		{
			return ((telekinetic * 31 + graveyard) * 31 + enchantment) * 31 + alchemist;
		}

		@Override
		public String toString()
		{
			return "T" + telekinetic + " G" + graveyard + " E" + enchantment + " A" + alchemist;
		}
	}

	/** The runes a plan spends, staves covering the elemental ones. */
	static final class Runes
	{
		final int law;
		final int nature;
		final int cosmic;

		Runes(int law, int nature, int cosmic)
		{
			this.law = law;
			this.nature = nature;
			this.cosmic = cosmic;
		}

		@Override
		public boolean equals(Object o)
		{
			if (!(o instanceof Runes))
			{
				return false;
			}
			Runes r = (Runes) o;
			return law == r.law && nature == r.nature && cosmic == r.cosmic;
		}

		@Override
		public int hashCode()
		{
			return (law * 31 + nature) * 31 + cosmic;
		}

		@Override
		public String toString()
		{
			return "law " + law + ", nature " + nature + ", cosmic " + cosmic;
		}
	}

	/** The plan: what to buy (upgrade prerequisites first, the peaches before everything), the points it takes and the runes. */
	static final class Result
	{
		final List<String> buy;
		final Points needed;
		final Points missing;
		final Runes runes;
		/** The Enchant the cosmic runes assume; 0 when the Enchanting Chamber is not needed. */
		final int enchantLevel;

		Result(List<String> buy, Points needed, Points missing, Runes runes, int enchantLevel)
		{
			this.buy = buy;
			this.needed = needed;
			this.missing = missing;
			this.runes = runes;
			this.enchantLevel = enchantLevel;
		}
	}

	/** Thrown for an unknown reward id, or points to earn in the Enchanting Chamber with no Enchant castable. */
	static final class MtaException extends Exception
	{
		MtaException(String message)
		{
			super(message);
		}
	}

	static MtaReward reward(String id)
	{
		return BY_ID.get(id);
	}

	/**
	 * Prices the chosen rewards. A point amount held beyond what is needed counts as zero missing,
	 * never as a credit to another room. {@code enchantLevel} (1 to 6) overrides the Enchant the
	 * Magic level implies; 0 takes the best one {@code magic} casts.
	 */
	static Result calculate(Collection<String> rewards, Collection<String> owned, Points held, int magic,
		int enchantLevel) throws MtaException
	{
		List<String> buy = buyOrder(rewards, owned);
		int t = 0;
		int g = 0;
		int e = 0;
		int a = 0;
		for (String id : buy)
		{
			MtaReward r = BY_ID.get(id);
			t += r.telekinetic;
			g += r.graveyard;
			e += r.enchantment;
			a += r.alchemist;
		}
		Points needed = new Points(t, g, e, a);
		Points missing = new Points(
			Math.max(0, t - held.telekinetic), Math.max(0, g - held.graveyard),
			Math.max(0, e - held.enchantment), Math.max(0, a - held.alchemist));
		int level = 0;
		if (missing.enchantment > 0)
		{
			level = enchantLevelFor(magic, enchantLevel);
			if (level == 0)
			{
				throw new MtaException("the Enchanting Chamber needs Magic 7 or an Enchant level");
			}
		}
		Runes runes = new Runes(
			ceil(missing.telekinetic, MtaData.TELEKINETIC_NUM, MtaData.TELEKINETIC_DEN),
			ceil(missing.graveyard, MtaData.GRAVEYARD_NUM, MtaData.GRAVEYARD_DEN)
				+ ceil(missing.alchemist, MtaData.ALCHEMIST_NUM, MtaData.ALCHEMIST_DEN),
			ceil(missing.enchantment, MtaData.ENCHANTMENT_NUM, MtaData.ENCHANTMENT_DEN * Math.max(level, 1)));
		return new Result(buy, needed, missing, runes, level);
	}

	private static void known(String id) throws MtaException
	{
		if (!BY_ID.containsKey(id))
		{
			throw new MtaException("unknown reward " + id);
		}
	}

	private static void add(String id, Set<String> have, List<String> buy)
	{
		if (have.contains(id) || buy.contains(id))
		{
			return;
		}
		MtaReward r = BY_ID.get(id);
		if (r.requires != null)
		{
			add(r.requires, have, buy);
		}
		buy.add(id);
	}

	/**
	 * What is left to buy, in purchase order: the wands' prerequisites first, the peaches before
	 * everything. An owned reward covers the tiers under it.
	 */
	static List<String> buyOrder(Collection<String> rewards, Collection<String> owned) throws MtaException
	{
		Set<String> have = new HashSet<>();
		for (String id : owned)
		{
			known(id);
			// A wand upgrade turned in the one below it: owning one means owning the tiers under it.
			for (String o = id; o != null; o = BY_ID.get(o).requires)
			{
				have.add(o);
			}
		}
		List<String> buy = new ArrayList<>();
		for (String id : rewards)
		{
			known(id);
			add(id, have, buy);
		}
		// The peaches halve the Graveyard's trips, so what they unlock is earned faster: buy them first.
		if (buy.remove(PEACHES))
		{
			buy.add(0, PEACHES);
		}
		return buy;
	}

	/** The explicit level when valid, else the best Enchant {@code magic} casts; 0 when none. */
	static int enchantLevelFor(int magic, int explicit)
	{
		if (explicit > 0)
		{
			return explicit <= MtaData.ENCHANT_MAGIC.length ? explicit : 0;
		}
		int best = 0;
		for (int i = 0; i < MtaData.ENCHANT_MAGIC.length; i++)
		{
			if (magic >= MtaData.ENCHANT_MAGIC[i])
			{
				best = i + 1;
			}
		}
		return best;
	}

	private static int ceil(int points, int num, int den)
	{
		if (points == 0)
		{
			return 0;
		}
		long n = (long) points * num;
		return (int) ((n + den - 1) / den);
	}
}
