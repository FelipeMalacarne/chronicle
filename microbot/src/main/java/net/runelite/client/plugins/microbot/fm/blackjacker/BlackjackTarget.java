package net.runelite.client.plugins.microbot.fm.blackjacker;

/** Blackjack targets in Pollnivneach (OSRS wiki, Thieving training: Blackjacking) and their rooms. */
public enum BlackjackTarget
{
	AUTO("Auto", null, 0, 0, 0, null),
	// Bearded bandits wielding a blackjack.
	BANDIT_41("Bandit (41)", "Bandit", 41, 45, 7, Room.ALI_THE_DYER),
	// Beardless bandits wielding a scimitar.
	BANDIT_56("Bandit (56)", "Bandit", 56, 55, 9, Room.ALI_THE_DYER),
	// Needs The Feud completed.
	MENAPHITE_THUG("Menaphite Thug", "Menaphite Thug", -1, 65, 7, Room.THUG_TENT);

	private final String label;
	final String npcName;
	/** -1 matches any combat level. */
	final int combatLevel;
	final int thievingLevel;
	/** Wiki max hit; healing keeps HP above two of these. */
	final int maxHit;
	final Room room;

	BlackjackTarget(String label, String npcName, int combatLevel, int thievingLevel, int maxHit, Room room)
	{
		this.label = label;
		this.npcName = npcName;
		this.combatLevel = combatLevel;
		this.thievingLevel = thievingLevel;
		this.maxHit = maxHit;
		this.room = room;
	}

	boolean matches(String name, int level)
	{
		return name != null && name.equalsIgnoreCase(npcName) && (combatLevel < 0 || combatLevel == level);
	}

	/** Best target for the level: bandit 41 until 55, bandit 56 until 65, then thugs once The Feud is done. */
	static BlackjackTarget best(int thieving, boolean feudDone)
	{
		if (thieving >= MENAPHITE_THUG.thievingLevel && feudDone)
		{
			return MENAPHITE_THUG;
		}
		return thieving >= BANDIT_56.thievingLevel ? BANDIT_56 : BANDIT_41;
	}

	@Override
	public String toString()
	{
		return label;
	}
}
