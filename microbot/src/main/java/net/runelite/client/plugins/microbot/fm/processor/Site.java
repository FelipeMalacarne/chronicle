package net.runelite.client.plugins.microbot.fm.processor;

import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.util.bank.enums.BankLocation;

/** Where a trip runs: the bank, the station's object and the tile to work it from. */
public enum Site
{
	// The furnace east of the bank (3110,3499); the owner works it from 3109,3499.
	EDGEVILLE_FURNACE("Edgeville furnace", BankLocation.EDGEVILLE, Station.FURNACE, 16469,
		new WorldPoint(3109, 3499, 0));

	private final String displayName;
	final BankLocation bank;
	final Station station;
	final int objectId;
	final WorldPoint stand;

	Site(String displayName, BankLocation bank, Station station, int objectId, WorldPoint stand)
	{
		this.displayName = displayName;
		this.bank = bank;
		this.station = station;
		this.objectId = objectId;
		this.stand = stand;
	}

	@Override
	public String toString()
	{
		return displayName;
	}
}
