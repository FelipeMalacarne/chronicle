package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.Collections;
import java.util.Map;
import java.util.Objects;
import java.util.TreeMap;

/**
 * One collection log page as read from the open interface, immutable. The planner keeps each
 * page as one fact and replaces it whole with a later read (docs/06, adendo 2026-11-13).
 */
final class CollectionLogPage
{
	/** BOSSES, RAIDS, CLUES, MINIGAMES or OTHER; null when the reader could not tell. */
	final String tab;
	/** When the page was read. */
	final Instant observedAt;
	/** Every slot the page showed: item id -> how many were obtained, 0 for one not obtained. */
	final Map<String, Long> items;

	CollectionLogPage(String tab, Instant observedAt, Map<String, Long> items)
	{
		this.tab = tab;
		this.observedAt = Objects.requireNonNull(observedAt);
		this.items = Collections.unmodifiableMap(new TreeMap<>(items));
	}

	/** Same tab and slots, whenever each was read. */
	boolean sameContent(CollectionLogPage other)
	{
		return other != null && Objects.equals(tab, other.tab) && items.equals(other.items);
	}

	/** The wire form; observed_at may never be after sent_at. */
	IngestContract.CollectionLogPage wire(Instant sentAt)
	{
		IngestContract.CollectionLogPage out = new IngestContract.CollectionLogPage();
		out.tab = tab;
		out.observedAt = PushBuilder.timestamp(observedAt.isAfter(sentAt) ? sentAt : observedAt);
		out.items = new TreeMap<>(items);
		return out;
	}

	@Override
	public boolean equals(Object o)
	{
		if (!(o instanceof CollectionLogPage))
		{
			return false;
		}
		CollectionLogPage p = (CollectionLogPage) o;
		return sameContent(p) && observedAt.equals(p.observedAt);
	}

	@Override
	public int hashCode()
	{
		return Objects.hash(tab, observedAt, items);
	}
}
