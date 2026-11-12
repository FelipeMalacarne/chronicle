package net.runelite.client.plugins.microbot.fm.tickfisher;

/**
 * What the plugin's event subscribers saw, for the script thread: the tick count, Fishing XP and
 * level, and which tick the last XP drop belongs to. Written on the client thread.
 */
final class FishingEvents
{
	private volatile int tick;
	private volatile int fishingXp;
	private volatile int fishingLevel;
	private volatile int agilityLevel;
	private volatile int agilityXp;
	private volatile int lastDropTick = -1;
	private volatile int despawnedIndex = -1;
	private boolean dropPending;

	void reset()
	{
		fishingXp = 0;
		fishingLevel = 0;
		lastDropTick = -1;
		despawnedIndex = -1;
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

	void onFishing(int level, int xp)
	{
		if (fishingXp > 0 && xp > fishingXp)
		{
			dropPending = true;
		}
		fishingXp = xp;
		fishingLevel = level;
	}

	void onAgility(int level, int xp)
	{
		agilityLevel = level;
		agilityXp = xp;
	}

	void onNpcDespawned(int index)
	{
		despawnedIndex = index;
	}

	void clearDespawned()
	{
		despawnedIndex = -1;
	}

	int tick()
	{
		return tick;
	}

	int fishingXp()
	{
		return fishingXp;
	}

	int fishingLevel()
	{
		return fishingLevel;
	}

	int agilityLevel()
	{
		return agilityLevel;
	}

	int agilityXp()
	{
		return agilityXp;
	}

	int lastDropTick()
	{
		return lastDropTick;
	}

	int despawnedIndex()
	{
		return despawnedIndex;
	}
}
