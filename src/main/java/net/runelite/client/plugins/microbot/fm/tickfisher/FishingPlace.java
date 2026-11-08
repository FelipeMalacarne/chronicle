package net.runelite.client.plugins.microbot.fm.tickfisher;

import net.runelite.api.coords.WorldPoint;
import net.runelite.api.gameval.ItemID;
import net.runelite.client.plugins.microbot.util.bank.enums.BankLocation;

/**
 * Where to fish and what that takes. Both are 5-tick rod fishing, so the 3-tick cycle is the same;
 * the spot, the gear and the fish differ. Public: the config proxy reads it from the overlay.
 */
public enum FishingPlace
{
	/** Fly fishing on the river by the bank (Oziris 1.2.12). */
	SHILO("Shilo Village (fly)", "Rod Fishing spot", "Lure",
		new WorldPoint(2846, 2975, 0), 24,
		// The game sends NPCs within about 15 tiles: from the west end of the river the spots at the
		// east end (x 2860 and beyond) never show, so the wait moves between the two.
		new WorldPoint[]{new WorldPoint(2842, 2969, 0), new WorldPoint(2857, 2971, 0)},
		new int[]{ItemID.FLY_FISHING_ROD, ItemID.FISHINGROD_PEARL_FLY},
		new int[]{ItemID.FEATHER},
		new int[]{ItemID.RAW_TROUT, ItemID.RAW_SALMON},
		20, 0, BankLocation.SHILO_VILLAGE, BankLocation.SHILO_VILLAGE),
	/** Barbarian fishing by Otto's Grotto (Oziris 1.2.16). No bank: bring the gear. */
	OTTO("Otto's Grotto (barbarian)", "Fishing spot", "Use-rod",
		// Spots seen at x 2500 (y 3506-3512) and x 2504 (y 3497, 3516): all in view from the shore.
		new WorldPoint(2500, 3507, 0), 20,
		new WorldPoint[]{new WorldPoint(2499, 3508, 0)},
		new int[]{ItemID.BRUT_FISHING_ROD},
		// Offcuts are used as bait before feathers.
		new int[]{ItemID.BRUT_FISH_CUTS, ItemID.FEATHER},
		new int[]{ItemID.BRUT_SPAWNING_TROUT, ItemID.BRUT_SPAWNING_SALMON, ItemID.BRUT_STURGEON},
		48, 15, null, BankLocation.BARBARIAN_OUTPOST);

	private final String label;
	final String spotName;
	final String spotAction;
	/** Spots farther than the radius from the centre are not ours. */
	final WorldPoint centre;
	final int radius;
	/** Where to wait while no spot is in view, in turn. */
	final WorldPoint[] lookouts;
	final int[] rods;
	final int[] bait;
	final int[] fish;
	final int fishingLevel;
	/** Barbarian fishing also needs this Agility and Strength. */
	final int agilityStrengthLevel;
	/** Null: no bank at hand, the gear comes along and the fish are dropped or cut. */
	final BankLocation bank;
	/** Where to fetch a knife, a teak log or feathers when one runs out (Otto: the Barbarian Outpost). */
	final BankLocation restock;

	FishingPlace(String label, String spotName, String spotAction, WorldPoint centre, int radius,
		WorldPoint[] lookouts, int[] rods, int[] bait, int[] fish, int fishingLevel, int agilityStrengthLevel,
		BankLocation bank, BankLocation restock)
	{
		this.label = label;
		this.spotName = spotName;
		this.spotAction = spotAction;
		this.centre = centre;
		this.radius = radius;
		this.lookouts = lookouts;
		this.rods = rods;
		this.bait = bait;
		this.fish = fish;
		this.fishingLevel = fishingLevel;
		this.agilityStrengthLevel = agilityStrengthLevel;
		this.bank = bank;
		this.restock = restock;
	}

	@Override
	public String toString()
	{
		return label;
	}
}
