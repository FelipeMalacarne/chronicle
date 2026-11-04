package net.runelite.client.plugins.microbot.fm.ironmanplanner;

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
		ItemSelection items;
		@SerializedName("interval_seconds")
		Integer intervalSeconds;
		boolean resync;

		boolean skillsEnabled()
		{
			return skills == null || skills;
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
	}
}
