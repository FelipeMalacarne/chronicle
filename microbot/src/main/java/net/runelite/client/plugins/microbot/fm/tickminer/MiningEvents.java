package net.runelite.client.plugins.microbot.fm.tickminer;

/**
 * What the plugin's event subscribers saw, for the script thread: the tick count, Mining XP and
 * level, which tick the last XP drop belongs to, and the hitpoints. Written on the client thread.
 */
final class MiningEvents
{
	private volatile int tick;
	private volatile int miningXp;
	private volatile int miningLevel;
	private volatile int lastDropTick = -1;
	private volatile int hitpoints;
	private volatile int maxHitpoints;
	/** "You top up your skin with water from the cactus", and "You fail to cut the cactus correctly". */
	private volatile int cactusTopUps;
	private volatile int cactusFails;
	private boolean dropPending;

	void reset()
	{
		miningXp = 0;
		miningLevel = 0;
		lastDropTick = -1;
		hitpoints = 0;
		maxHitpoints = 0;
		dropPending = false;
	}

	/**
	 * A new server tick. An XP drop seen since the previous one belongs to this tick: stat updates
	 * arrive with the tick's packets, before its GameTick.
	 */
	void onTick()
	{
		tick++;
		if (dropPending)
		{
			dropPending = false;
			lastDropTick = tick;
		}
	}

	void onMining(int level, int xp)
	{
		if (miningXp > 0 && xp > miningXp)
		{
			dropPending = true;
		}
		miningXp = xp;
		miningLevel = level;
	}

	void onHitpoints(int boosted, int real)
	{
		hitpoints = boosted;
		maxHitpoints = real;
	}

	void onGameMessage(String message)
	{
		if (message.startsWith("You top up your skin with water from the cactus"))
		{
			cactusTopUps++;
		}
		else if (message.startsWith("You fail to cut the cactus"))
		{
			cactusFails++;
		}
	}

	int cactusTopUps()
	{
		return cactusTopUps;
	}

	int cactusFails()
	{
		return cactusFails;
	}

	int tick()
	{
		return tick;
	}

	int miningXp()
	{
		return miningXp;
	}

	int miningLevel()
	{
		return miningLevel;
	}

	int lastDropTick()
	{
		return lastDropTick;
	}

	int hitpoints()
	{
		return hitpoints;
	}

	int maxHitpoints()
	{
		return maxHitpoints;
	}
}
