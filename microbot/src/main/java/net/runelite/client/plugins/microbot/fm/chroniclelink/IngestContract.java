package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.annotations.SerializedName;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import net.runelite.client.plugins.timetracking.SummaryState;

/**
 * Wire DTOs of the planner's ingest contract v1 (docs/04-arquitetura.md 6.4).
 * Field names follow the JSON exactly; null fields are omitted by Gson, which is how optional
 * fields and unchanged delta facets stay out of the payload. Maps are {@code Map<String, ...>}
 * so ids serialize as canonical decimal keys ({@code {"4151": 1}}).
 */
final class IngestContract
{
	static final int CONTRACT_VERSION = 1;

	static final String SCOPE_STATE_WRITE = "state:write";
	static final String SCOPE_BANK = "items:bank";
	static final String SCOPE_INVENTORY = "items:inventory";
	static final String SCOPE_EQUIPMENT = "items:equipment";

	private IngestContract()
	{
	}

	static final class Manifest
	{
		@SerializedName("contract_version")
		int contractVersion;
		@SerializedName("manifest_version")
		long manifestVersion;
		@SerializedName("token_scopes")
		List<String> tokenScopes;
		List<Integer> varbits;
		List<Integer> varps;
		List<Integer> collections;
		Boolean skills;
		/** The server accepts {@code data.quest}; absent (older server) means it would reject it. */
		Boolean quests;
		/** The server accepts {@code data.timers} and {@code data.position} (planner 2026-11-10). */
		Boolean timers;
		Boolean position;
		ItemSelection items;
		@SerializedName("interval_seconds")
		Integer intervalSeconds;
		boolean resync;

		boolean skillsEnabled()
		{
			return skills == null || skills;
		}

		boolean questsEnabled()
		{
			return quests != null && quests;
		}

		boolean timersEnabled()
		{
			return timers != null && timers;
		}

		boolean positionEnabled()
		{
			return position != null && position;
		}

		int intervalSecondsOrDefault()
		{
			return intervalSeconds == null ? 10 : Math.max(5, intervalSeconds);
		}

		/** Effective permission: the manifest flag and the token scope must both grant it. */
		boolean allows(ContainerKind kind)
		{
			if (items == null || tokenScopes == null)
			{
				return false;
			}
			switch (kind)
			{
				case BANK:
					return items.bank && tokenScopes.contains(SCOPE_BANK);
				case INVENTORY:
					return items.inventory && tokenScopes.contains(SCOPE_INVENTORY);
				case EQUIPMENT:
					return items.equipment && tokenScopes.contains(SCOPE_EQUIPMENT);
				default:
					return false;
			}
		}
	}

	static final class ItemSelection
	{
		boolean bank;
		boolean inventory;
		boolean equipment;
		@SerializedName("only_item_ids")
		List<Integer> onlyItemIds;
	}

	static final class Push
	{
		@SerializedName("contract_version")
		int contractVersion = CONTRACT_VERSION;
		String username;
		String profile;
		@SerializedName("account_type")
		String accountType;
		@SerializedName("manifest_version")
		long manifestVersion;
		@SerializedName("client_seq")
		long clientSeq;
		String kind;
		@SerializedName("sent_at")
		String sentAt;
		PushData data;
	}

	static final class PushData
	{
		Map<String, Long> varb;
		Map<String, Long> varp;
		Map<String, Long> level;
		Map<String, Long> xp;
		Map<String, Long> boosted;
		/** RuneLite {@code Quest.getName()} -> {@code QuestState} name; only when the manifest allows it. */
		Map<String, String> quest;
		/** Time Tracking tab -> clock; only when the manifest allows it and the plugin is on. */
		Map<String, Timer> timers;
		/** Where the character stands; only with the owner's opt-in and the manifest's flag. */
		Position position;
		ItemContainers items;
	}

	/** One Time Tracking clock: the summary and, growing or done, when it is ready (epoch seconds). */
	static final class Timer
	{
		String state;
		Long readyAt;
		String label;

		static Timer of(String label, SummaryState state, long completion)
		{
			Timer t = new Timer();
			t.label = label;
			t.state = state == null ? "UNKNOWN" : state.name();
			// The trackers use -1 (and 0 before the first load) for "not known".
			if (completion > 0 && ("IN_PROGRESS".equals(t.state) || "COMPLETED".equals(t.state)))
			{
				t.readyAt = completion;
			}
			return t;
		}

		@Override
		public boolean equals(Object o)
		{
			if (!(o instanceof Timer))
			{
				return false;
			}
			Timer t = (Timer) o;
			return Objects.equals(state, t.state) && Objects.equals(readyAt, t.readyAt) && Objects.equals(label, t.label);
		}

		@Override
		public int hashCode()
		{
			return Objects.hash(state, readyAt, label);
		}
	}

	/** A tile and the world. */
	static final class Position
	{
		int world;
		int x;
		int y;
		int plane;

		Position(int world, int x, int y, int plane)
		{
			this.world = world;
			this.x = x;
			this.y = y;
			this.plane = plane;
		}

		/**
		 * Worth a push: another world or plane, or at least {@code tiles} away. Walking sends the
		 * position again every few seconds otherwise.
		 */
		boolean movedFrom(Position before, int tiles)
		{
			return before == null || world != before.world || plane != before.plane
				|| Math.max(Math.abs(x - before.x), Math.abs(y - before.y)) >= tiles;
		}
	}

	static final class ItemContainers
	{
		ItemObservation bank;
		ItemObservation inventory;
		ItemObservation equipment;

		boolean isEmpty()
		{
			return bank == null && inventory == null && equipment == null;
		}
	}

	static final class ItemObservation
	{
		String state;
		@SerializedName("observed_at")
		String observedAt;
		Long revision;
		@SerializedName("live_epoch")
		String liveEpoch;
		Map<String, Long> quantities;
	}

	static final class Response
	{
		@SerializedName("contract_version")
		int contractVersion;
		boolean ok;
		@SerializedName("accepted_seq")
		long acceptedSeq;
		boolean resync;
		@SerializedName("manifest_version")
		long manifestVersion;
		@SerializedName("steps_changed")
		Long stepsChanged;
		/** The active route after the push (planner with the plugin panel); null from older servers. */
		RouteUpdate route;
	}

	/** What an accepted push changed in the active route: its cursor and the changed steps. */
	static final class RouteUpdate
	{
		String guide;
		/** Cursor step id after the push; null when the route is finished. */
		String cursor;
		@SerializedName("cursor_cause")
		String cursorCause;
		List<String> changed;
	}
}
