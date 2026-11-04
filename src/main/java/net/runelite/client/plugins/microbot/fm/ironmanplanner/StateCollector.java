package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.time.Instant;
import java.util.Collections;
import java.util.EnumMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TreeMap;
import java.util.UUID;
import java.util.stream.Stream;
import net.runelite.api.Client;
import net.runelite.api.GameState;
import net.runelite.api.Player;
import net.runelite.api.Skill;
import net.runelite.api.gameval.VarbitID;
import net.runelite.client.config.RuneScapeProfileType;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;

/**
 * Reads player state into an immutable {@link PlayerSnapshot}. Client thread only: every
 * method is called from an event handler or from {@code runOnClientThreadOptional}. Item
 * containers come from Microbot's event-maintained caches; nothing here acts in game (no
 * {@code Rs2Bank.isOpen()}, which may type the bank PIN).
 */
final class StateCollector
{
	private final Client client;
	private final ContainerTracker bank = new ContainerTracker();
	private final ContainerTracker inventory = new ContainerTracker();
	private final ContainerTracker equipment = new ContainerTracker();

	/** Opaque id of the current login; null while logged out. */
	private String liveEpoch;
	/** Last BANK container event within {@link #liveEpoch}, or null. */
	private Instant bankEventAt;

	StateCollector(Client client)
	{
		this.client = client;
	}

	void startEpoch()
	{
		liveEpoch = "session-" + UUID.randomUUID();
		bankEventAt = null;
		bank.reset();
		inventory.reset();
		equipment.reset();
	}

	void endEpoch()
	{
		liveEpoch = null;
		bankEventAt = null;
	}

	boolean inEpoch()
	{
		return liveEpoch != null;
	}

	void onBankContainerChanged(Instant at)
	{
		if (liveEpoch != null)
		{
			bankEventAt = at;
		}
	}

	/** The current state, or null when no character is readable. */
	PlayerSnapshot capture(IngestContract.Manifest manifest, IronmanPlannerConfig config, Instant now)
	{
		if (liveEpoch == null || client.getGameState() != GameState.LOGGED_IN)
		{
			return null;
		}
		Player local = client.getLocalPlayer();
		String username = local == null ? null : local.getName();
		if (username == null || username.trim().isEmpty())
		{
			return null;
		}

		Map<String, Long> varb = new TreeMap<>();
		for (Integer id : nullSafe(manifest.varbits))
		{
			varb.put(Integer.toString(id), (long) client.getVarbitValue(id));
		}
		Map<String, Long> varp = new TreeMap<>();
		for (Integer id : nullSafe(manifest.varps))
		{
			varp.put(Integer.toString(id), (long) client.getVarpValue(id));
		}

		Map<String, Long> level = new TreeMap<>();
		Map<String, Long> xp = new TreeMap<>();
		Map<String, Long> boosted = new TreeMap<>();
		if (manifest.skillsEnabled())
		{
			for (Skill skill : Skill.values())
			{
				// OVERALL is a deprecated pseudo-skill, not part of the contract
				if ("OVERALL".equals(skill.name()))
				{
					continue;
				}
				String key = skill.name().toLowerCase(Locale.ROOT);
				level.put(key, (long) client.getRealSkillLevel(skill));
				xp.put(key, (long) client.getSkillExperience(skill));
				boosted.put(key, (long) client.getBoostedSkillLevel(skill));
			}
		}

		List<Integer> only = manifest.items == null ? null : manifest.items.onlyItemIds;
		Map<ContainerKind, ContainerSnapshot> containers = new EnumMap<>(ContainerKind.class);
		containers.put(ContainerKind.BANK, captureBank(manifest.allows(ContainerKind.BANK) && config.sendBank(), only, now));
		containers.put(ContainerKind.INVENTORY, manifest.allows(ContainerKind.INVENTORY) && config.sendInventory()
			? inventory.live(liveEpoch, totals(Rs2Inventory.items(), only), now)
			: ContainerSnapshot.unavailable(now));
		containers.put(ContainerKind.EQUIPMENT, manifest.allows(ContainerKind.EQUIPMENT) && config.sendEquipment()
			? equipment.live(liveEpoch, totals(Rs2Equipment.all(), only), now)
			: ContainerSnapshot.unavailable(now));

		return new PlayerSnapshot(username, profile(RuneScapeProfileType.getCurrent(client)),
			accountType(client.getVarbitValue(VarbitID.IRONMAN)), manifest.manifestVersion,
			varb, varp, level, xp, boosted, containers);
	}

	private ContainerSnapshot captureBank(boolean allowed, List<Integer> only, Instant now)
	{
		long openedAt = Rs2Bank.getBankLastOpenedAt();
		ContainerSnapshot.State state = BankFreshness.classify(allowed, bankEventAt != null,
			Rs2Bank.getBankLiveEpoch(), Rs2Bank.hasBankMirrorSnapshot(), openedAt);
		switch (state)
		{
			case LIVE:
				// the bank is only observable while open: its contents date from the last event
				return bank.live(liveEpoch, totals(Rs2Bank.bankItems().stream(), only), bankEventAt);
			case RESTORED:
				// the capture time doubles as the revision, so it grows across sessions
				return ContainerSnapshot.restored(Instant.ofEpochMilli(openedAt), openedAt,
					totals(Rs2Bank.bankItems().stream(), only));
			default:
				return ContainerSnapshot.unavailable(now);
		}
	}

	private static Map<String, Long> totals(Stream<Rs2ItemModel> items, List<Integer> only)
	{
		Map<String, Long> out = ItemQuantities.empty();
		items.forEach(item -> ItemQuantities.add(out, item.getId(), item.getQuantity(), only));
		return out;
	}

	private static List<Integer> nullSafe(List<Integer> ids)
	{
		return ids == null ? Collections.emptyList() : ids;
	}

	static String profile(RuneScapeProfileType type)
	{
		if (type == RuneScapeProfileType.STANDARD)
		{
			return "STANDARD";
		}
		if (type == RuneScapeProfileType.BETA)
		{
			return "BETA";
		}
		return "OTHER";
	}

	/** Varbit 1777 to the contract's account_type; null (omitted) when unknown. */
	static String accountType(int ironmanVarbit)
	{
		switch (ironmanVarbit)
		{
			case 0:
				return "NORMAL";
			case 1:
				return "IRONMAN";
			case 2:
				return "ULTIMATE_IRONMAN";
			case 3:
				return "HARDCORE_IRONMAN";
			case 4:
				return "GROUP_IRONMAN";
			case 5:
				return "HARDCORE_GROUP_IRONMAN";
			case 6:
				return "UNRANKED_GROUP_IRONMAN";
			default:
				return null;
		}
	}
}
