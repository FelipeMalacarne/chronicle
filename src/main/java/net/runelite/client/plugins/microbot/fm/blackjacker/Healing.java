package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;

/**
 * Jugs of wine. As a step it drinks when HP is at or below the session's threshold; the cooldown
 * lets one drink land (it shows up as a StatChanged) before the next.
 */
final class Healing implements Step
{
	static final int JUG_OF_WINE = 1993;
	static final int JUG = 1935;
	// Wine has a 3-tick delay.
	private static final long COOLDOWN_MS = 1800;

	private long lastDrinkAt;

	void reset()
	{
		lastDrinkAt = 0;
	}

	static boolean hasWine()
	{
		return Rs2Inventory.hasItem(JUG_OF_WINE);
	}

	static int wines()
	{
		return Rs2Inventory.count(JUG_OF_WINE);
	}

	@Override
	public boolean run(Session s)
	{
		return s.lowHp() && drinkIfReady(s);
	}

	/** Drinks if there is wine and the last drink has landed; true when it drank. */
	boolean drinkIfReady(Session s)
	{
		if (s.now - lastDrinkAt < COOLDOWN_MS || !hasWine())
		{
			return false;
		}
		Microbot.status = "Blackjacker: drinking wine at " + s.hp() + "/" + s.events.maxHp() + " HP";
		lastDrinkAt = s.now;
		Rs2Inventory.interact(JUG_OF_WINE, "Drink");
		return true;
	}
}
