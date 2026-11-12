package net.runelite.client.plugins.microbot.fm.blackjacker;

import lombok.extern.slf4j.Slf4j;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;

/**
 * Another target-named NPC inside the room (any level) sees every knock-out: the game refuses it
 * with "Perhaps I shouldn't do this here". One can walk in while the curtain is open for a lure,
 * and with it closed it is stuck in there. Hop to another world, where the room is fresh.
 */
@Slf4j
final class Witness implements Step
{
	// Hops in a row with no knock-out landing in between before the run gives up.
	private static final int MAX_HOPS = 3;
	// How long a refused knock-out counts after its message.
	private static final long SEEN_MS = 5000;

	private int hops;
	private int knockoutsAtHop;

	@Override
	public boolean run(Session s)
	{
		boolean seen = s.now - s.seenAt < SEEN_MS;
		Rs2NpcModel witness = seen ? null : witness(s);
		// A fight is the safety steps' to end first; a hop is refused during one anyway.
		if ((!seen && witness == null) || s.attacked)
		{
			return false;
		}
		if (s.knockouts > knockoutsAtHop)
		{
			hops = 0;
		}
		if (hops >= MAX_HOPS)
		{
			s.stop("Another " + s.target.npcName + " could see the room after " + MAX_HOPS + " hops");
			return true;
		}
		int from = Microbot.getClient().getWorld();
		int world = Worlds.otherMembersWorld(from);
		if (world < 0)
		{
			return true;
		}
		String why = seen ? "knock-out seen (\"Perhaps I shouldn't do this here\")" : witness.getName() + " (" + witness.getCombatLevel() + ") in the room";
		Microbot.status = "Blackjacker: " + why + ", hopping to world " + world;
		log.info("Blackjacker: {}, hopping {} -> {}", why, from, world);
		hops++;
		knockoutsAtHop = s.knockouts;
		s.seenAt = 0;
		Worlds.hop(world);
		return true;
	}

	/** A target-named NPC in the room other than the victim. */
	private static Rs2NpcModel witness(Session s)
	{
		int victim = s.victim == null ? -1 : s.victim.getIndex();
		return Microbot.getRs2NpcCache().query()
			.where(npc -> npc.getName() != null && npc.getName().equalsIgnoreCase(s.target.npcName))
			.where(npc -> npc.getIndex() != victim)
			.where(npc -> s.room.area.contains(npc.getWorldLocation()))
			.nearestOnClientThread();
	}
}
