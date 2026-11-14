package net.runelite.client.plugins.microbot.fm.mtaarena;

/** One item of the Rewards Guardian's stock and what it costs in each room's pizazz points. */
final class MtaReward
{
	final String id;
	final String name;
	final int itemId;
	/** Id of the reward whose item the purchase turns in (a wand upgrade), or null. */
	final String requires;
	final int telekinetic;
	final int graveyard;
	final int enchantment;
	final int alchemist;

	MtaReward(String id, String name, int itemId, String requires, int telekinetic, int graveyard,
		int enchantment, int alchemist)
	{
		this.id = id;
		this.name = name;
		this.itemId = itemId;
		this.requires = requires;
		this.telekinetic = telekinetic;
		this.graveyard = graveyard;
		this.enchantment = enchantment;
		this.alchemist = alchemist;
	}
}
