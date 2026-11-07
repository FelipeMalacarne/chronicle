package net.runelite.client.plugins.microbot.fm.blackjacker;

import lombok.extern.slf4j.Slf4j;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;

/**
 * Ends a fight that knocking out again cannot ({@link Session#mustEscape}) by climbing the room's
 * ladder: the target cannot follow, so the fight is over for sure (the unarmed knock-out did not
 * stop a bandit in testing). The climb goes before drinking because it is what stops the damage;
 * healing happens upstairs, then back down.
 */
@Slf4j
final class LadderEscape implements Step
{
	// Wiki: "briefly climb a nearby ladder"; the NPC drops the fight once we are gone.
	private static final long UPSTAIRS_MS = 3000;
	// A click that has not moved us by then (stunned, misclick) is sent again.
	private static final long RETRY_MS = 1800;
	// The ladder only counts as missing after this long without finding it: right after a climb
	// the old floor's objects are gone before our position catches up.
	private static final long MISSING_MS = 3000;

	private enum Phase
	{
		NONE,
		CLIMBING_UP,
		UPSTAIRS,
		CLIMBING_DOWN
	}

	private final Healing healing;
	private Phase phase = Phase.NONE;
	// Time of the last click, or of arriving upstairs.
	private long phaseAt;
	private boolean missing;
	private long notFoundSince;

	LadderEscape(Healing healing)
	{
		this.healing = healing;
	}

	/** The room has a ladder and it was found. */
	boolean available(Room room)
	{
		return room.hasLadder() && !missing;
	}

	@Override
	public boolean run(Session s)
	{
		if (s.room.upstairs(s.me))
		{
			upstairs(s);
			return true;
		}
		if (phase == Phase.UPSTAIRS || phase == Phase.CLIMBING_DOWN)
		{
			// Back down: the fight is over.
			phase = Phase.NONE;
			s.events.forgetFight();
		}
		if (available(s.room) && (phase == Phase.CLIMBING_UP || (s.mustEscape && s.room.inOrAt(s.me))))
		{
			climbUp(s);
			return true;
		}
		return false;
	}

	private void climbUp(Session s)
	{
		if (phase != Phase.CLIMBING_UP)
		{
			s.combatResets++;
			log.info("Blackjacker: {} at {}/{} HP, climbing the ladder (#{})",
				s.events.refused(s.now) ? "knock-out refused during combat" : "hit at low HP", s.hp(), s.events.maxHp(), s.combatResets);
			phase = Phase.CLIMBING_UP;
			phaseAt = 0;
		}
		Microbot.status = "Blackjacker: under attack, climbing the ladder";
		if (s.now - phaseAt < RETRY_MS)
		{
			return;
		}
		// Still down here after a click (stunned, or the click missed): drink first if low.
		if (phaseAt != 0 && s.lowHp() && healing.drinkIfReady(s))
		{
			return;
		}
		Rs2TileObjectModel ladder = RoomObjects.ladder(s.room, RoomObjects.LADDER_UP, 0);
		if (ladder == null)
		{
			if (notFoundSince == 0)
			{
				notFoundSince = s.now;
			}
			if (s.now - notFoundSince >= MISSING_MS)
			{
				log.warn("Blackjacker: no ladder (id {}) near {}", RoomObjects.LADDER_UP, s.room.ladder);
				missing = true;
				phase = Phase.NONE;
				s.stop("No ladder (id " + RoomObjects.LADDER_UP + ") near " + s.room.ladder);
			}
			return;
		}
		notFoundSince = 0;
		ladder.click("Climb-up");
		phaseAt = s.now;
	}

	/** Heal, give the target a moment to drop the fight, climb back down. */
	private void upstairs(Session s)
	{
		if (phase != Phase.UPSTAIRS && phase != Phase.CLIMBING_DOWN)
		{
			phase = Phase.UPSTAIRS;
			phaseAt = s.now;
		}
		if (s.lowHp() && Healing.hasWine())
		{
			healing.drinkIfReady(s);
			return;
		}
		if (s.hp() <= s.dangerHp())
		{
			// Going back down this low with nothing to drink is not worth it; the loop stops up here.
			s.stop("Out of wine at " + s.hp() + "/" + s.events.maxHp() + " HP");
			return;
		}
		if (s.now - phaseAt < UPSTAIRS_MS || s.throttled)
		{
			Microbot.status = "Blackjacker: upstairs, waiting for the fight to drop";
			return;
		}
		if (phase == Phase.CLIMBING_DOWN && s.now - phaseAt < RETRY_MS)
		{
			return;
		}
		Rs2TileObjectModel ladder = RoomObjects.ladder(s.room, RoomObjects.LADDER_DOWN, 1);
		if (ladder == null)
		{
			s.stop("No ladder (id " + RoomObjects.LADDER_DOWN + ") upstairs near " + s.room.ladder);
			return;
		}
		Microbot.status = "Blackjacker: climbing back down";
		ladder.click("Climb-down");
		phase = Phase.CLIMBING_DOWN;
		phaseAt = s.now;
	}
}
