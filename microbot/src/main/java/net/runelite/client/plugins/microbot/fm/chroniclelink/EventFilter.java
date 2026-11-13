package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import net.runelite.api.gameval.InventoryID;
import net.runelite.api.gameval.VarPlayerID;

/**
 * Which game events can change what the plugin sends under one manifest, and how urgently
 * (see {@link CaptureTrigger}). Immutable: built on the sync thread when a manifest arrives and
 * read by the client thread's event handlers. Pure: event ids in, a strength out.
 */
final class EventFilter
{
	private final Set<Integer> varbits;
	private final Set<Integer> varps;
	private final boolean skills;
	private final boolean quests;
	private final boolean collectionLog;
	private final boolean bank;
	private final boolean inventory;
	private final boolean equipment;

	private EventFilter(IngestContract.Manifest m)
	{
		varbits = ids(m.varbits);
		varps = ids(m.varps);
		skills = m.skillsEnabled();
		quests = m.questsEnabled();
		collectionLog = m.collectionLogEnabled();
		bank = m.allows(ContainerKind.BANK);
		inventory = m.allows(ContainerKind.INVENTORY);
		equipment = m.allows(ContainerKind.EQUIPMENT);
	}

	/** Null (every event ignored) until there is a manifest. */
	static EventFilter of(IngestContract.Manifest manifest)
	{
		return manifest == null ? null : new EventFilter(manifest);
	}

	/** The contract's container for a RuneLite inventory id, or null for every other one. */
	static ContainerKind container(int containerId)
	{
		switch (containerId)
		{
			case InventoryID.BANK:
				return ContainerKind.BANK;
			case InventoryID.INV:
				return ContainerKind.INVENTORY;
			case InventoryID.WORN:
				return ContainerKind.EQUIPMENT;
			default:
				return null;
		}
	}

	/**
	 * A container the capture sends: the manifest and the token scope allow it and the owner's
	 * toggle for it is on, exactly the condition {@code StateCollector.capture} applies.
	 */
	CaptureTrigger.Strength container(int containerId, ChronicleLinkConfig config)
	{
		ContainerKind kind = container(containerId);
		if (kind == null)
		{
			return CaptureTrigger.Strength.NONE;
		}
		boolean sent;
		switch (kind)
		{
			case BANK:
				sent = bank && config.sendBank();
				break;
			case INVENTORY:
				sent = inventory && config.sendInventory();
				break;
			default:
				sent = equipment && config.sendEquipment();
				break;
		}
		return sent ? CaptureTrigger.Strength.HARD : CaptureTrigger.Strength.NONE;
	}

	/**
	 * A var listed in the manifest, or quest points when quests are sent, is hard: it is what a
	 * diary step or a finished quest changes. Any other var is soft when quests are sent, since
	 * quest states derive from vars nobody lists (a quest's progress varbit), and ignored
	 * otherwise. Soft keeps the cost bounded: vars that churn (combat, timers) never pass the
	 * debounce, so they wait for the periodic capture as before, while a quest step's vars
	 * settle and go out about a second later.
	 *
	 * @param varbitId the changed varbit, or -1 when the event is for a whole varp
	 */
	CaptureTrigger.Strength var(int varpId, int varbitId)
	{
		boolean listed = varbitId != -1 ? varbits.contains(varbitId) : varps.contains(varpId);
		if (listed || (quests && varbitId == -1 && varpId == VarPlayerID.QP))
		{
			return CaptureTrigger.Strength.HARD;
		}
		return quests ? CaptureTrigger.Strength.SOFT : CaptureTrigger.Strength.NONE;
	}

	/**
	 * A level-up is hard (requirements); experience and boosted levels move all the time while
	 * training or fighting, so they are soft and a steady stream of them keeps the periodic
	 * cadence.
	 */
	CaptureTrigger.Strength stat(boolean realLevelChanged)
	{
		if (!skills)
		{
			return CaptureTrigger.Strength.NONE;
		}
		return realLevelChanged ? CaptureTrigger.Strength.HARD : CaptureTrigger.Strength.SOFT;
	}

	/** A collection log page read with new slots is hard when the manifest takes the pages. */
	CaptureTrigger.Strength collectionLog()
	{
		return collectionLog ? CaptureTrigger.Strength.HARD : CaptureTrigger.Strength.NONE;
	}

	private static Set<Integer> ids(List<Integer> list)
	{
		if (list == null)
		{
			return Collections.emptySet();
		}
		Set<Integer> out = new HashSet<>();
		for (Integer id : list)
		{
			if (id != null)
			{
				out.add(id);
			}
		}
		return Collections.unmodifiableSet(out);
	}
}
