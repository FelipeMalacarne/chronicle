package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * A room with a second curtain (the thug room's south one, open by default) is a way through for
 * every NPC around: targets walk in to watch the knock-outs and the lured one walks out. Closed
 * from inside before the lure, and again whenever it reopens (it resets, other players open it).
 */
final class BackCurtain implements Step
{
	@Override
	public boolean run(Session s)
	{
		Room room = s.room;
		// A fight is the safety steps' to end first.
		if (s.attacked || !RoomObjects.backCurtainOpen(room))
		{
			return false;
		}
		if (!room.area.contains(s.me))
		{
			Microbot.status = "Blackjacker: going in to close the back curtain";
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
		Microbot.status = "Blackjacker: closing the back curtain";
		RoomObjects.clickCurtain(room.backCurtain, "Close");
		return true;
	}
}
