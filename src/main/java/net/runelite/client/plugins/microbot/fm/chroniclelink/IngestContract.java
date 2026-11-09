package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.annotations.SerializedName;
import java.util.List;
import java.util.Map;

/**
 * Wire DTOs of the planner's ingest contract v1 (ironman-planner docs/04-arquitetura.md 6.4).
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
		ItemContainers items;
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
