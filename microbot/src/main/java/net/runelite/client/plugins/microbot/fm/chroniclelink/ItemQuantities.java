package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.Collection;
import java.util.Comparator;
import java.util.Map;
import java.util.TreeMap;

/** Pure helpers for {@code item_id -> qty} maps keyed by canonical decimal ids. */
final class ItemQuantities
{
	private static final Comparator<String> NUMERIC = Comparator.comparingLong(Long::parseLong);

	private ItemQuantities()
	{
	}

	static TreeMap<String, Long> empty()
	{
		return new TreeMap<>(NUMERIC);
	}

	static TreeMap<String, Long> copy(Map<String, Long> source)
	{
		TreeMap<String, Long> out = empty();
		out.putAll(source);
		return out;
	}

	/**
	 * Accumulates one stack into {@code totals}: stacks of the same id add up, ids and
	 * quantities that are not positive are skipped, and {@code onlyItemIds} limits the accepted
	 * ids. An absent ({@code null}) or empty list means no filter, the whole container, as the
	 * server reads it: the generated manifest no longer fills {@code only_item_ids}.
	 */
	static void add(Map<String, Long> totals, int itemId, long quantity, Collection<Integer> onlyItemIds)
	{
		if (itemId <= 0 || quantity <= 0)
		{
			return;
		}
		if (onlyItemIds != null && !onlyItemIds.isEmpty() && !onlyItemIds.contains(itemId))
		{
			return;
		}
		totals.merge(Integer.toString(itemId), quantity, Long::sum);
	}

	/**
	 * Delta patch from {@code previous} to {@code current}: changed or new ids carry their new
	 * total, ids that disappeared carry the {@code 0} tombstone.
	 */
	static TreeMap<String, Long> patch(Map<String, Long> previous, Map<String, Long> current)
	{
		TreeMap<String, Long> out = empty();
		for (Map.Entry<String, Long> e : current.entrySet())
		{
			if (!e.getValue().equals(previous.get(e.getKey())))
			{
				out.put(e.getKey(), e.getValue());
			}
		}
		for (String id : previous.keySet())
		{
			if (!current.containsKey(id))
			{
				out.put(id, 0L);
			}
		}
		return out;
	}
}
