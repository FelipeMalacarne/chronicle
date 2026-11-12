package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.Collections;
import java.util.EnumMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TreeMap;
import java.util.UUID;
import java.util.function.IntUnaryOperator;
import java.util.stream.Stream;
import net.runelite.api.Client;
import net.runelite.api.GameState;
import net.runelite.api.Player;
import net.runelite.api.Quest;
import net.runelite.api.Skill;
import net.runelite.api.coords.WorldPoint;
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
	/**
	 * Game ticks to wait after reaching LOGGED_IN before reading anything: the server sends
	 * varps, stats and containers over the first ticks, and a full read before that would
	 * report quests as not started and diaries as incomplete.
	 */
	static final int SETTLE_TICKS = 3;

	private final Client client;
	private final QuestTracker quests;
	private final ContainerTracker bank = new ContainerTracker();
	private final ContainerTracker inventory = new ContainerTracker();
	private final ContainerTracker equipment = new ContainerTracker();

	/** Opaque id of the current login; null while logged out. */
	private String liveEpoch;
	/** Last BANK container event within {@link #liveEpoch}, or null. */
	private Instant bankEventAt;
	/** Game ticks since login, a hop or a reconnect, capped at {@link #SETTLE_TICKS}. */
	private int ticksLoggedIn;
	/** Last real level seen per skill in this epoch, 0 while unknown. */
	private final Map<Skill, Integer> realLevels = new EnumMap<>(Skill.class);

	StateCollector(Client client)
	{
		this.client = client;
		this.quests = new QuestTracker(Quest.values(), quest -> quest.getState(client));
	}

	void startEpoch()
	{
		liveEpoch = "session-" + UUID.randomUUID();
		bankEventAt = null;
		ticksLoggedIn = 0;
		realLevels.clear();
		quests.reset();
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

	/** True on the tick that ends the post-login wait: the state just became readable. */
	boolean onGameTick()
	{
		if (ticksLoggedIn < SETTLE_TICKS)
		{
			ticksLoggedIn++;
			return ticksLoggedIn == SETTLE_TICKS && liveEpoch != null;
		}
		return false;
	}

	/**
	 * Whether a game event should ask for a capture: only once the post-login wait is over. The
	 * events of the first ticks are the server sending the state, which the login's full reads
	 * anyway once {@link #capture} accepts to run.
	 */
	boolean readyForEvents()
	{
		return liveEpoch != null && ticksLoggedIn >= SETTLE_TICKS && client.getGameState() == GameState.LOGGED_IN;
	}

	/** Remembers the skill's real level; true when it changed from a level already known. */
	boolean onStatChanged(Skill skill, int realLevel)
	{
		Integer before = realLevels.put(skill, realLevel);
		return before != null && before != 0 && before != realLevel;
	}

	/**
	 * A hop, reconnect or logout makes the server send the vars again, so wait for them anew. A
	 * region LOADING keeps them and does not restart the wait.
	 */
	void onGameStateChanged(GameState state)
	{
		if (state != GameState.LOGGED_IN && state != GameState.LOADING)
		{
			ticksLoggedIn = 0;
		}
	}

	/** A varp or varbit changed, so quest states may have too. */
	void onVarsChanged()
	{
		quests.invalidate();
	}

	void onBankContainerChanged(Instant at)
	{
		if (liveEpoch != null)
		{
			bankEventAt = at;
		}
	}

	/** The current state, or null when no character is readable. */
	PlayerSnapshot capture(IngestContract.Manifest manifest, ChronicleLinkConfig config, Instant now)
	{
		if (liveEpoch == null || client.getGameState() != GameState.LOGGED_IN || ticksLoggedIn < SETTLE_TICKS)
		{
			return null;
		}
		Player local = client.getLocalPlayer();
		String username = local == null ? null : local.getName();
		if (username == null || username.trim().isEmpty())
		{
			return null;
		}

		Map<String, Long> varb = readVars(manifest.varbits, client::getVarbitValue);
		Map<String, Long> varp = readVars(manifest.varps, client::getVarpValue);
		Map<String, String> quest = readQuests(manifest, quests);
		Map<String, IngestContract.Timer> timers = TimeTrackingClocks.forIngest(manifest);
		IngestContract.Position position = null;
		if (manifest.positionEnabled() && config.sendPosition() && local != null)
		{
			WorldPoint at = WorldPoint.fromLocalInstance(client, local.getLocalLocation());
			if (at != null)
			{
				position = new IngestContract.Position(client.getWorld(), at.getX(), at.getY(), at.getPlane());
			}
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
			varb, varp, level, xp, boosted, quest, timers, position, containers);
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

	/**
	 * The quest facet, or {@code null} (not sent) unless the manifest says the server accepts
	 * it: an older server rejects {@code data.quest} as an unknown field.
	 */
	static Map<String, String> readQuests(IngestContract.Manifest manifest, QuestTracker tracker)
	{
		return manifest.questsEnabled() ? tracker.read() : null;
	}

	/**
	 * Every id the manifest lists, read without knowing what it means (diary tiers, quest
	 * points, unlock proofs): {@code "id" -> value}.
	 */
	static Map<String, Long> readVars(List<Integer> ids, IntUnaryOperator read)
	{
		Map<String, Long> out = new TreeMap<>();
		for (Integer id : ids == null ? Collections.<Integer>emptyList() : ids)
		{
			if (id != null)
			{
				out.put(Integer.toString(id), (long) read.applyAsInt(id));
			}
		}
		return out;
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
