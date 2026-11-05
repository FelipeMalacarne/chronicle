package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.time.Instant;
import java.util.Arrays;
import java.util.Collections;
import java.util.EnumMap;
import java.util.HashMap;
import java.util.Map;

final class Fixtures
{
	static final Instant T0 = Instant.parse("2026-11-04T12:00:00Z");
	static final String EPOCH = "session-a";

	private Fixtures()
	{
	}

	static Map<String, Long> map(Object... kv)
	{
		Map<String, Long> out = ItemQuantities.empty();
		for (int i = 0; i < kv.length; i += 2)
		{
			out.put(String.valueOf(kv[i]), ((Number) kv[i + 1]).longValue());
		}
		return out;
	}

	static Map<String, Long> vars(Object... kv)
	{
		Map<String, Long> out = new HashMap<>();
		for (int i = 0; i < kv.length; i += 2)
		{
			out.put(String.valueOf(kv[i]), ((Number) kv[i + 1]).longValue());
		}
		return out;
	}

	static Map<ContainerKind, ContainerSnapshot> allUnavailable(Instant at)
	{
		Map<ContainerKind, ContainerSnapshot> c = new EnumMap<>(ContainerKind.class);
		for (ContainerKind kind : ContainerKind.values())
		{
			c.put(kind, ContainerSnapshot.unavailable(at));
		}
		return c;
	}

	static PlayerSnapshot snapshot(long manifestVersion, Map<String, Long> varb, Map<String, Long> level,
		Map<ContainerKind, ContainerSnapshot> containers)
	{
		return withQuests(manifestVersion, varb, Collections.emptyMap(), level, null, containers);
	}

	static PlayerSnapshot withQuests(long manifestVersion, Map<String, Long> varb, Map<String, Long> varp,
		Map<String, Long> level, Map<String, String> quest, Map<ContainerKind, ContainerSnapshot> containers)
	{
		return new PlayerSnapshot("Iron Test", "STANDARD", "IRONMAN", manifestVersion,
			varb, varp, level, level, level, quest, containers);
	}

	static PlayerSnapshot quests(long manifestVersion, Map<String, String> quest)
	{
		return withQuests(manifestVersion, vars("4458", 0), vars("101", 33), vars("attack", 40), quest, allUnavailable(T0));
	}

	static Map<String, String> states(String... kv)
	{
		Map<String, String> out = new HashMap<>();
		for (int i = 0; i < kv.length; i += 2)
		{
			out.put(kv[i], kv[i + 1]);
		}
		return out;
	}

	static PlayerSnapshot simple(long manifestVersion, long attackLevel)
	{
		return snapshot(manifestVersion, vars("1777", 1), vars("attack", attackLevel), allUnavailable(T0));
	}

	static IngestContract.Manifest manifest(long version, String... scopes)
	{
		IngestContract.Manifest m = new IngestContract.Manifest();
		m.contractVersion = 1;
		m.manifestVersion = version;
		m.tokenScopes = Arrays.asList(scopes);
		m.varbits = Collections.singletonList(1777);
		m.varps = Collections.emptyList();
		m.collections = Collections.emptyList();
		m.items = new IngestContract.ItemSelection();
		m.items.bank = m.tokenScopes.contains(IngestContract.SCOPE_BANK);
		m.items.inventory = m.tokenScopes.contains(IngestContract.SCOPE_INVENTORY);
		m.items.equipment = m.tokenScopes.contains(IngestContract.SCOPE_EQUIPMENT);
		m.intervalSeconds = 10;
		return m;
	}

	static IngestContract.Response ack(long seq, long manifestVersion)
	{
		IngestContract.Response r = new IngestContract.Response();
		r.contractVersion = 1;
		r.ok = true;
		r.acceptedSeq = seq;
		r.manifestVersion = manifestVersion;
		return r;
	}

	static IngestContract.Response stale(long lastSeq, long manifestVersion)
	{
		IngestContract.Response r = ack(lastSeq, manifestVersion);
		r.ok = false;
		r.resync = true;
		return r;
	}
}
