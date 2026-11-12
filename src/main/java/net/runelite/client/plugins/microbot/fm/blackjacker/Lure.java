package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.dialogues.Rs2Dialogue;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * Gets a target into the empty room. Lures only one the {@link LeadPlanner} can lead in (it walks
 * straight at us and stalls on walls), clicks through the dialogue, then leads it a hop at a time,
 * waiting for it to catch up after each. The curtain stays open until it is inside; EnterRoom
 * closes it.
 */
@Slf4j
final class Lure implements Step
{
	private static final int WALLS_RADIUS = 18;
	// Only lure targets this close to the room.
	private static final int RANGE = 12;
	private static final int MAX_HOPS = 20;
	// How long a follower may take to come next to us before it counts as a stall.
	private static final long CATCH_UP_MS = 3000;
	private static final int MAX_STALLS = 3;
	private static final int MAX_FAILURES = 5;
	// A target lured this often that never got in stops following short of the room each time
	// (likely the edge of its roaming range): lure another.
	private static final int MAX_LURES_PER_TARGET = 3;

	/** NPC indexes that stalled while being led; not followed or lured again this run. */
	private final Set<Integer> dropped = new HashSet<>();
	/** Lures sent to each NPC index this run. */
	private final Map<Integer, Integer> lured = new HashMap<>();
	private long waitSince;
	private int stalls;
	private int failures;

	@Override
	public boolean run(Session s)
	{
		if (s.victim != null)
		{
			stalls = 0;
			waitSince = 0;
			return false;
		}
		if (Rs2Dialogue.isInDialogue())
		{
			if (Rs2Dialogue.hasSelectAnOption())
			{
				Rs2Dialogue.keyPressForDialogueOption(1);
			}
			else
			{
				Rs2Dialogue.clickContinue();
			}
			Global.sleep(400, 700);
			return true;
		}
		if (failures >= MAX_FAILURES)
		{
			s.stop("Could not lure a " + s.target + " into the room after " + MAX_FAILURES + " tries");
			return true;
		}
		Room room = s.room;
		if (s.me.distanceTo(room.outside) > RANGE)
		{
			Microbot.status = "Blackjacker: walking to the room";
			Rs2Walker.walkTo(room.outside, 1);
			return true;
		}
		if (!RoomObjects.curtainOpen(room))
		{
			// Open for the planner and for the target to walk in.
			RoomObjects.clickCurtain(room, "Open");
			return true;
		}

		LeadPlanner planner = new LeadPlanner(Walls.around(room.leadGoal, WALLS_RADIUS), room.leadGoal);
		Rs2NpcModel follower = follower(s);
		if (follower != null)
		{
			lead(s, follower, planner);
			return true;
		}
		stalls = 0;
		waitSince = 0;

		if (room.area.contains(s.me))
		{
			// Nobody inside: step out to find one.
			Rs2Walker.walkFastCanvas(room.outside);
			Global.sleepUntil(() -> !room.area.contains(Rs2Player.getWorldLocation()), 3000);
			return true;
		}
		Rs2NpcModel candidate = candidate(s, planner);
		if (candidate == null)
		{
			Microbot.status = "Blackjacker: no " + s.target + " that can be led into the room";
			return true;
		}
		Microbot.status = "Blackjacker: luring " + candidate.getName() + " at " + candidate.getWorldLocation();
		if (candidate.getWorldLocation().distanceTo(s.me) > 8)
		{
			Rs2Walker.walkTo(candidate.getWorldLocation(), 3);
			return true;
		}
		if (lured.getOrDefault(candidate.getIndex(), 0) >= MAX_LURES_PER_TARGET)
		{
			drop(candidate, candidate.getWorldLocation());
			return true;
		}
		if (candidate.click("Lure") && Global.sleepUntil(Rs2Dialogue::isInDialogue, 4000))
		{
			s.lures++;
			lured.merge(candidate.getIndex(), 1, Integer::sum);
			return true;
		}
		failures++;
		return true;
	}

	/**
	 * The free target with the shortest planned lead into the room: every hop is a chance for the
	 * simulation and the game to disagree. Null when none can be led in.
	 */
	private Rs2NpcModel candidate(Session s, LeadPlanner planner)
	{
		Room room = s.room;
		List<Rs2NpcModel> free = Microbot.getRs2NpcCache().query()
			.where(npc -> s.target.matches(npc.getName(), npc.getCombatLevel()))
			.where(npc -> !room.area.contains(npc.getWorldLocation()))
			.where(npc -> npc.getInteracting() == null)
			.where(npc -> !dropped.contains(npc.getIndex()))
			.where(npc -> npc.getWorldLocation().distanceTo(room.leadGoal) <= RANGE)
			.toListOnClientThread();
		Rs2NpcModel best = null;
		List<WorldPoint> bestRoute = null;
		for (Rs2NpcModel npc : free)
		{
			List<WorldPoint> route = planner.route(npc.getWorldLocation(), room.area::contains, MAX_HOPS);
			if (route != null && (bestRoute == null || route.size() < bestRoute.size()))
			{
				best = npc;
				bestRoute = route;
			}
		}
		if (best != null)
		{
			log.info("Blackjacker: luring {} at {}, planned lead of {} hops: {}", best.getName(), best.getWorldLocation(), bestRoute.size(), bestRoute);
		}
		return best;
	}

	/** A target outside the room that is following us (interacting with the player after a Lure). */
	private Rs2NpcModel follower(Session s)
	{
		return Microbot.getRs2NpcCache().query()
			.where(npc -> s.target.matches(npc.getName(), npc.getCombatLevel()))
			.where(npc -> !s.room.area.contains(npc.getWorldLocation()))
			.where(npc -> !dropped.contains(npc.getIndex()))
			.where(Rs2NpcModel::isInteractingWithPlayer)
			.nearestOnClientThread();
	}

	/**
	 * One decision of the lead: wait while it catches up, move to where it can reach us when it
	 * cannot, otherwise hop to the next tile the planner says it can follow to.
	 */
	private void lead(Session s, Rs2NpcModel follower, LeadPlanner planner)
	{
		WorldPoint me = s.me;
		WorldPoint at = follower.getWorldLocation();
		String name = follower.getName();
		if (!LeadPlanner.adjacent(at, me))
		{
			if (planner.catchesUp(at, me))
			{
				Microbot.status = "Blackjacker: waiting for " + name + " to catch up";
				if (waited(s))
				{
					stall(follower, at);
				}
				return;
			}
			WorldPoint spot = planner.regroup(me, at);
			if (spot == null)
			{
				drop(follower, at);
				return;
			}
			Microbot.status = "Blackjacker: moving to where " + name + " can reach us";
			stall(follower, at);
			hop(spot);
			return;
		}
		if (me.equals(s.room.leadGoal))
		{
			Microbot.status = "Blackjacker: waiting for " + name + " to step in";
			if (waited(s))
			{
				stall(follower, at);
			}
			return;
		}
		WorldPoint next = planner.nextHop(me, at);
		if (next == null)
		{
			Microbot.status = "Blackjacker: no tile " + name + " can follow to from here";
			if (waited(s))
			{
				stall(follower, at);
			}
			return;
		}
		waitSince = 0;
		Microbot.status = "Blackjacker: leading " + name + " (" + planner.distanceToGoal(me) + " tiles to go)";
		hop(next);
	}

	/** True once, after waiting {@link #CATCH_UP_MS} in the same spot. */
	private boolean waited(Session s)
	{
		if (waitSince == 0)
		{
			waitSince = s.now;
			return false;
		}
		if (s.now - waitSince < CATCH_UP_MS)
		{
			return false;
		}
		waitSince = 0;
		return true;
	}

	private void stall(Rs2NpcModel follower, WorldPoint at)
	{
		if (++stalls > MAX_STALLS)
		{
			drop(follower, at);
		}
	}

	private void drop(Rs2NpcModel npc, WorldPoint at)
	{
		log.info("Blackjacker: {} stuck at {} while leading, luring another", npc.getName(), at);
		dropped.add(npc.getIndex());
		failures++;
		stalls = 0;
		waitSince = 0;
	}

	private static void hop(WorldPoint next)
	{
		if (Rs2Walker.walkFastCanvas(next))
		{
			Global.sleepUntil(() -> next.equals(Rs2Player.getWorldLocation()), 1800);
		}
		Global.sleep(60, 160);
	}
}
