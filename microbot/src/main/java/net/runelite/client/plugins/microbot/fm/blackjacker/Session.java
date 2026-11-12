package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.LinkedHashMap;
import java.util.Map;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.gameval.AnimationID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;

/**
 * One run's shared state: the target and its room, the game events, counters and the stop
 * reason, plus what this tick saw (where we are, the victim, whether it is attacking us).
 */
final class Session
{
	final BlackjackerConfig config;
	final GameEvents events;
	final BlackjackTarget target;
	final Room room;
	final long startedAt = System.currentTimeMillis();
	String stopReason;

	// A hidden or minimized window drops the client to ~1 FPS and every reaction to seconds.
	private static final int MIN_FPS = 15;
	private static final long LOW_FPS_GRACE_MS = 3000;
	private long lowFpsSince;

	// This tick, from begin().
	long now;
	WorldPoint me;
	Rs2NpcModel victim;
	/** Being hit by, or targeted by, a conscious target. */
	boolean attacked;
	/**
	 * A fight we have to get out of. A glance is not one: knocking out again ends it (wiki:
	 * "interrupted before its second hit"), with a drink first when low. It is one when the game
	 * refuses the knock-out "during combat", or, as an emergency, when we are still being hit
	 * below {@link #dangerHp()}.
	 */
	boolean mustEscape;
	/** Too few frames to react in time: only safety runs, nothing new is started. */
	boolean throttled;
	/** When the game last refused a knock-out because someone could see it; 0 once handled. */
	long seenAt;
	/** Jugs of wine in the inventory, for the overlay. */
	volatile int wines;

	// Counters for the progress report.
	int knockouts;
	int knockoutFails;
	int picks;
	int pickFails;
	int winesBought;
	int lures;
	int combatResets;

	Session(BlackjackerConfig config, GameEvents events, BlackjackTarget target)
	{
		this.config = config;
		this.events = events;
		this.target = target;
		this.room = target.room;
	}

	/** Reads this tick's state; false when there is nothing to act on yet. */
	boolean begin()
	{
		now = System.currentTimeMillis();
		me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return false;
		}
		victim = Microbot.getRs2NpcCache().query()
			.where(npc -> target.matches(npc.getName(), npc.getCombatLevel()))
			.where(npc -> room.area.contains(npc.getWorldLocation()))
			.nearestOnClientThread();
		// Hits tell us after the fact; a target in the room that has us as its target and is not
		// down is fighting us too, even before (or without) a hit seen by this run, e.g. right
		// after a restart in the middle of a fight.
		boolean targeting = victim != null && victim.isInteractingWithPlayer() && !unconscious(victim);
		attacked = (events.beingAttacked(now) && (victim == null || !unconscious(victim))) || targeting;
		mustEscape = attacked && (events.refused(now) || hp() < dangerHp());
		throttled = throttled();
		wines = Healing.wines();
		return true;
	}

	private boolean throttled()
	{
		int fps = Microbot.getClient().getFPS();
		if (fps >= MIN_FPS)
		{
			lowFpsSince = 0;
			return false;
		}
		if (lowFpsSince == 0)
		{
			lowFpsSince = now;
		}
		return now - lowFpsSince >= LOW_FPS_GRACE_MS;
	}

	/** The first reason wins; the loop stops once it is safe to. */
	void stop(String reason)
	{
		if (stopReason == null)
		{
			stopReason = reason;
		}
	}

	int hp()
	{
		return events.hp();
	}

	/**
	 * Drink as soon as a whole jug fits: wine used is the damage taken over its heal either way,
	 * and drinking early keeps the most HP above {@link #dangerHp()}, so fewer glances end on the
	 * ladder. Never below dangerHp: a knock-out is only started above it (the drink goes first,
	 * once the target is up).
	 */
	int drinkAt()
	{
		return Math.max(events.maxHp() - Healing.HEALS, dangerHp());
	}

	/**
	 * The lowest HP a knock-out is started or retried at. From here a glance's hit and one more
	 * on the way up the ladder (it lands in practice) leave at least 1 HP; below it, a target
	 * still hitting us sends us up the ladder.
	 */
	int dangerHp()
	{
		return 2 * target.maxHit + 1;
	}

	boolean lowHp()
	{
		return hp() <= drinkAt();
	}

	static boolean unconscious(Rs2NpcModel npc)
	{
		return npc.getAnimation() == AnimationID.HUMAN_UNCONSCIOUS;
	}

	Map<String, Object> counters()
	{
		Map<String, Object> c = new LinkedHashMap<>();
		c.put("knockouts", knockouts);
		c.put("knockoutFails", knockoutFails);
		c.put("picks", picks);
		c.put("pickFails", pickFails);
		c.put("combatResets", combatResets);
		c.put("lures", lures);
		c.put("winesBought", winesBought);
		return c;
	}
}
