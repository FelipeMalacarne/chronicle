package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/** With a target inside: get in with it and close the curtain behind us. */
final class EnterRoom implements Step
{
	@Override
	public boolean run(Session s)
	{
		if (s.victim == null)
		{
			return false;
		}
		Room room = s.room;
		if (!room.area.contains(s.me))
		{
			Microbot.status = "Blackjacker: going into the room";
			if (s.me.distanceTo(room.outside) > 3)
			{
				Rs2Walker.walkTo(room.outside, 1);
				return true;
			}
			if (!RoomObjects.curtainOpen(room))
			{
				RoomObjects.clickCurtain(room, "Open");
				return true;
			}
			Rs2Walker.walkFastCanvas(room.inside);
			Global.sleepUntil(() -> room.area.contains(Rs2Player.getWorldLocation()), 3000);
			return true;
		}
		Rs2TileObjectModel curtain = RoomObjects.curtain(room);
		if (curtain == null)
		{
			s.stop("No curtain (ids " + RoomObjects.CURTAIN_CLOSED + "/" + RoomObjects.CURTAIN_OPEN + ") at " + room.curtain);
			return true;
		}
		if (curtain.getId() == RoomObjects.CURTAIN_OPEN)
		{
			Microbot.status = "Blackjacker: closing the curtain";
			RoomObjects.clickCurtain(room, "Close");
			return true;
		}
		return false;
	}
}
