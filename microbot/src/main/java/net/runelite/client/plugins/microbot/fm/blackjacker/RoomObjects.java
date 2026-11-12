package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.util.Global;

/** A room's curtain and ladder, found by id and tile (cheap: no client-thread name lookups). */
final class RoomObjects
{
	static final int CURTAIN_CLOSED = 1533;
	static final int CURTAIN_OPEN = 1534;
	static final int LADDER_UP = 6261;
	static final int LADDER_DOWN = 6260;

	private RoomObjects()
	{
	}

	static Rs2TileObjectModel curtain(Room room)
	{
		return Microbot.getRs2TileObjectCache().query()
			.withIds(CURTAIN_OPEN, CURTAIN_CLOSED)
			.where(o -> o.getWorldLocation().equals(room.curtain))
			.first();
	}

	/** Open, or not loaded: either way nothing to click. */
	static boolean curtainOpen(Room room)
	{
		Rs2TileObjectModel curtain = curtain(room);
		return curtain == null || curtain.getId() == CURTAIN_OPEN;
	}

	/** Clicks Open or Close and waits for the curtain to change. */
	static void clickCurtain(Room room, String action)
	{
		Rs2TileObjectModel curtain = curtain(room);
		if (curtain == null)
		{
			return;
		}
		int id = curtain.getId();
		if (curtain.click(action))
		{
			Global.sleepUntil(() -> {
				Rs2TileObjectModel now = curtain(room);
				return now != null && now.getId() != id;
			}, 3000);
		}
	}

	/** The room's ladder with this id on this plane, or null. */
	static Rs2TileObjectModel ladder(Room room, int id, int plane)
	{
		WorldPoint at = new WorldPoint(room.ladder.getX(), room.ladder.getY(), plane);
		return Microbot.getRs2TileObjectCache().query()
			.withIds(id)
			.where(o -> o.getWorldLocation().distanceTo(at) <= 2)
			.first();
	}
}
