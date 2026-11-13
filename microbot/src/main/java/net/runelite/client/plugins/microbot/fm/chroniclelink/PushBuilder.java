package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Map;
import java.util.Objects;
import java.util.TreeMap;

/** Builds contract v1 pushes: a full baseline, or a delta against the last acknowledged snapshot. */
final class PushBuilder
{
	static final String FULL = "full";
	static final String DELTA = "delta";
	/** A position change alone is worth a push only this many tiles away (or another world or plane). */
	static final int POSITION_TILES = 16;

	private PushBuilder()
	{
	}

	/**
	 * A delta is only possible on top of an acknowledged snapshot of the same character and
	 * manifest whose containers kept their state (and live epoch). Anything else, such as the
	 * bank turning from restored to live or a toggle flipping, needs a new full.
	 */
	static boolean requiresFull(PlayerSnapshot acked, PlayerSnapshot current)
	{
		if (acked == null
			|| !acked.username.equals(current.username)
			|| acked.manifestVersion != current.manifestVersion
			|| !acked.varb.keySet().equals(current.varb.keySet())
			|| !acked.varp.keySet().equals(current.varp.keySet())
			|| !sameKeys(acked.quest, current.quest)
			|| !sameKeys(acked.timers, current.timers))
		{
			return true;
		}
		for (ContainerKind kind : ContainerKind.values())
		{
			if (!acked.container(kind).deltaCompatible(current.container(kind)))
			{
				return true;
			}
		}
		return false;
	}

	static IngestContract.Push full(PlayerSnapshot current, long clientSeq, Instant sentAt)
	{
		IngestContract.Push push = header(current, clientSeq, FULL, sentAt);
		IngestContract.PushData data = push.data;
		data.varb = new TreeMap<>(current.varb);
		data.varp = new TreeMap<>(current.varp);
		data.level = new TreeMap<>(current.level);
		data.xp = new TreeMap<>(current.xp);
		data.boosted = new TreeMap<>(current.boosted);
		// every quest, {} when there are none; absent only when quests are not collected
		data.quest = current.quest == null ? null : new TreeMap<>(current.quest);
		data.timers = current.timers == null ? null : new TreeMap<>(current.timers);
		data.position = current.position;
		data.collectionLog = collectionLog(current.collectionLog, sentAt);
		data.items = new IngestContract.ItemContainers();
		data.items.bank = observation(current.container(ContainerKind.BANK), null, sentAt);
		data.items.inventory = observation(current.container(ContainerKind.INVENTORY), null, sentAt);
		data.items.equipment = observation(current.container(ContainerKind.EQUIPMENT), null, sentAt);
		return push;
	}

	/** The delta from {@code acked} to {@code current}, or {@code null} when nothing changed. */
	static IngestContract.Push delta(PlayerSnapshot acked, PlayerSnapshot current, long clientSeq, Instant sentAt)
	{
		IngestContract.Push push = header(current, clientSeq, DELTA, sentAt);
		IngestContract.PushData data = push.data;
		data.varb = changed(acked.varb, current.varb);
		data.varp = changed(acked.varp, current.varp);
		data.level = changed(acked.level, current.level);
		data.xp = changed(acked.xp, current.xp);
		data.boosted = changed(acked.boosted, current.boosted);
		data.quest = current.quest == null ? null
			: acked.quest == null ? new TreeMap<>(current.quest) : changed(acked.quest, current.quest);
		data.timers = current.timers == null ? null
			: acked.timers == null ? new TreeMap<>(current.timers) : changed(acked.timers, current.timers);
		// Pages only come and change: a page never requires a full, a delta carries the new reads.
		data.collectionLog = current.collectionLog == null ? null
			: collectionLog(acked.collectionLog == null ? current.collectionLog : changed(acked.collectionLog, current.collectionLog), sentAt);

		IngestContract.ItemContainers items = new IngestContract.ItemContainers();
		items.bank = changedObservation(acked, current, ContainerKind.BANK, sentAt);
		items.inventory = changedObservation(acked, current, ContainerKind.INVENTORY, sentAt);
		items.equipment = changedObservation(acked, current, ContainerKind.EQUIPMENT, sentAt);
		data.items = items.isEmpty() ? null : items;

		boolean headerChanged = !acked.profile.equals(current.profile)
			|| !Objects.equals(acked.accountType, current.accountType);
		boolean other = data.varb != null || data.varp != null || data.level != null || data.xp != null
			|| data.boosted != null || data.quest != null || data.timers != null || data.collectionLog != null
			|| data.items != null || headerChanged;
		// The position rides along any push once it changed; alone it waits for a real move.
		IngestContract.Position pos = current.position;
		if (pos != null && pos.movedFrom(acked.position, other ? 1 : POSITION_TILES))
		{
			data.position = pos;
		}
		if (!other && data.position == null)
		{
			return null;
		}
		return push;
	}

	/** RFC 3339 UTC, milliseconds. */
	static String timestamp(Instant instant)
	{
		return instant.truncatedTo(ChronoUnit.MILLIS).toString();
	}

	private static IngestContract.Push header(PlayerSnapshot current, long clientSeq, String kind, Instant sentAt)
	{
		IngestContract.Push push = new IngestContract.Push();
		push.username = current.username;
		push.profile = current.profile;
		push.accountType = current.accountType;
		push.manifestVersion = current.manifestVersion;
		push.clientSeq = clientSeq;
		push.kind = kind;
		push.sentAt = timestamp(sentAt);
		push.data = new IngestContract.PushData();
		return push;
	}

	private static IngestContract.ItemObservation changedObservation(PlayerSnapshot acked, PlayerSnapshot current,
		ContainerKind kind, Instant sentAt)
	{
		ContainerSnapshot before = acked.container(kind);
		ContainerSnapshot now = current.container(kind);
		if (!now.changedSince(before))
		{
			return null;
		}
		return observation(now, ItemQuantities.patch(before.quantities, now.quantities), sentAt);
	}

	/** {@code patch} replaces the quantities in a delta; {@code null} sends the whole image. */
	static IngestContract.ItemObservation observation(ContainerSnapshot snapshot, Map<String, Long> patch, Instant sentAt)
	{
		IngestContract.ItemObservation obs = new IngestContract.ItemObservation();
		obs.state = snapshot.state.wire;
		// observed_at may never be after sent_at
		Instant observed = snapshot.observedAt.isAfter(sentAt) ? sentAt : snapshot.observedAt;
		obs.observedAt = timestamp(observed);
		if (snapshot.state == ContainerSnapshot.State.UNAVAILABLE)
		{
			return obs;
		}
		obs.revision = snapshot.revision;
		obs.liveEpoch = snapshot.liveEpoch;
		obs.quantities = patch != null ? patch : ItemQuantities.copy(snapshot.quantities);
		return obs;
	}

	/** The pages in wire form, or {@code null} (not sent) when there are none. */
	static Map<String, IngestContract.CollectionLogPage> collectionLog(Map<String, CollectionLogPage> pages, Instant sentAt)
	{
		if (pages == null || pages.isEmpty())
		{
			return null;
		}
		Map<String, IngestContract.CollectionLogPage> out = new TreeMap<>();
		for (Map.Entry<String, CollectionLogPage> e : pages.entrySet())
		{
			out.put(e.getKey(), e.getValue().wire(sentAt));
		}
		return out;
	}

	/** Both absent, or both present with the same keys: a delta can describe the change. */
	private static boolean sameKeys(Map<String, ?> before, Map<String, ?> after)
	{
		if (before == null || after == null)
		{
			return before == after;
		}
		return before.keySet().equals(after.keySet());
	}

	private static <V> Map<String, V> changed(Map<String, V> before, Map<String, V> after)
	{
		TreeMap<String, V> out = new TreeMap<>();
		for (Map.Entry<String, V> e : after.entrySet())
		{
			if (!e.getValue().equals(before.get(e.getKey())))
			{
				out.put(e.getKey(), e.getValue());
			}
		}
		return out.isEmpty() ? null : out;
	}
}
