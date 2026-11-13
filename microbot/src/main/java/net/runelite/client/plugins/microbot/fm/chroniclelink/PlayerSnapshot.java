package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.ArrayList;
import java.util.Collections;
import java.util.EnumMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.TreeMap;

/**
 * Everything one push can describe, copied out of the client on the client thread so the
 * network side never holds a reference to a mutable cache.
 */
final class PlayerSnapshot
{
	final String username;
	final String profile;
	final String accountType;
	final long manifestVersion;
	final Map<String, Long> varb;
	final Map<String, Long> varp;
	final Map<String, Long> level;
	final Map<String, Long> xp;
	final Map<String, Long> boosted;
	/** Quest name -> state, or {@code null} when quests are not collected (manifest or read failure). */
	final Map<String, String> quest;
	/** Time Tracking tab -> clock, or {@code null} when not collected (manifest, plugin off). */
	final Map<String, IngestContract.Timer> timers;
	/** Where the character stands, or {@code null} without the owner's opt-in. */
	final IngestContract.Position position;
	/** Collection log pages read in this login by title, or {@code null} when the manifest does not take them. */
	final Map<String, CollectionLogPage> collectionLog;
	final Map<ContainerKind, ContainerSnapshot> containers;
	/**
	 * The demonstration events not acknowledged yet ({@link DemonstrationRecorder}), oldest first,
	 * or {@code null}: none, or the manifest does not take them. Never diffed: every push carries
	 * the pending ones, and an acknowledged push drops them from the recorder.
	 */
	final List<IngestContract.RecordedEvent> recording;

	PlayerSnapshot(String username, String profile, String accountType, long manifestVersion,
		Map<String, Long> varb, Map<String, Long> varp,
		Map<String, Long> level, Map<String, Long> xp, Map<String, Long> boosted,
		Map<String, String> quest, Map<ContainerKind, ContainerSnapshot> containers)
	{
		this(username, profile, accountType, manifestVersion, varb, varp, level, xp, boosted, quest, null, null, containers);
	}

	PlayerSnapshot(String username, String profile, String accountType, long manifestVersion,
		Map<String, Long> varb, Map<String, Long> varp,
		Map<String, Long> level, Map<String, Long> xp, Map<String, Long> boosted,
		Map<String, String> quest, Map<String, IngestContract.Timer> timers, IngestContract.Position position,
		Map<ContainerKind, ContainerSnapshot> containers)
	{
		this(username, profile, accountType, manifestVersion, varb, varp, level, xp, boosted, quest, timers, position, null, containers);
	}

	PlayerSnapshot(String username, String profile, String accountType, long manifestVersion,
		Map<String, Long> varb, Map<String, Long> varp,
		Map<String, Long> level, Map<String, Long> xp, Map<String, Long> boosted,
		Map<String, String> quest, Map<String, IngestContract.Timer> timers, IngestContract.Position position,
		Map<String, CollectionLogPage> collectionLog, Map<ContainerKind, ContainerSnapshot> containers)
	{
		this(username, profile, accountType, manifestVersion, varb, varp, level, xp, boosted, quest, timers, position,
			collectionLog, containers, null);
	}

	private PlayerSnapshot(String username, String profile, String accountType, long manifestVersion,
		Map<String, Long> varb, Map<String, Long> varp,
		Map<String, Long> level, Map<String, Long> xp, Map<String, Long> boosted,
		Map<String, String> quest, Map<String, IngestContract.Timer> timers, IngestContract.Position position,
		Map<String, CollectionLogPage> collectionLog, Map<ContainerKind, ContainerSnapshot> containers,
		List<IngestContract.RecordedEvent> recording)
	{
		this.recording = recording == null || recording.isEmpty() ? null
			: Collections.unmodifiableList(new ArrayList<>(recording));
		this.username = Objects.requireNonNull(username);
		this.profile = Objects.requireNonNull(profile);
		this.accountType = accountType;
		this.manifestVersion = manifestVersion;
		this.varb = frozen(varb);
		this.varp = frozen(varp);
		this.level = frozen(level);
		this.xp = frozen(xp);
		this.boosted = frozen(boosted);
		this.quest = quest == null ? null : Collections.unmodifiableMap(new TreeMap<>(quest));
		this.timers = timers == null ? null : Collections.unmodifiableMap(new TreeMap<>(timers));
		this.position = position;
		this.collectionLog = collectionLog == null ? null : Collections.unmodifiableMap(new TreeMap<>(collectionLog));
		EnumMap<ContainerKind, ContainerSnapshot> c = new EnumMap<>(ContainerKind.class);
		for (ContainerKind kind : ContainerKind.values())
		{
			c.put(kind, Objects.requireNonNull(containers.get(kind), kind.name()));
		}
		this.containers = Collections.unmodifiableMap(c);
	}

	ContainerSnapshot container(ContainerKind kind)
	{
		return containers.get(kind);
	}

	/** This snapshot carrying the given demonstration events (null or empty: none). */
	PlayerSnapshot withRecording(List<IngestContract.RecordedEvent> events)
	{
		return new PlayerSnapshot(username, profile, accountType, manifestVersion, varb, varp, level, xp, boosted,
			quest, timers, position, collectionLog, containers, events);
	}

	private static Map<String, Long> frozen(Map<String, Long> values)
	{
		return Collections.unmodifiableMap(new TreeMap<>(values));
	}
}
