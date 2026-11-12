package net.runelite.client.plugins.microbot.fm.blackjacker;

/**
 * One thing the loop may do this tick. Steps run in priority order and the first that returns
 * true owns the tick: getting out of a fight and healing come before upkeep, upkeep before
 * thieving. Each step re-derives what to do from game state, so a run can start anywhere.
 */
@FunctionalInterface
interface Step
{
	boolean run(Session s);
}
