package net.runelite.client.plugins.microbot.fm.processor;

/** A kind of object a product is made at, with the action that opens its make-X interface. */
enum Station
{
	FURNACE("Smelt");

	final String action;

	Station(String action)
	{
		this.action = action;
	}
}
