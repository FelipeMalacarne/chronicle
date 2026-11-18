package net.runelite.client.plugins.microbot.fm.processor;

import net.runelite.api.Skill;
import net.runelite.api.gameval.ItemID;

/**
 * What the plugin makes: the inputs one action uses, the tools it keeps in the inventory, the
 * station it is made at and the option of the make-X interface that picks it. Inputs are
 * unstackable, one slot per unit; whatever else the action leaves behind (the empty buckets of
 * molten glass) is banked with the product.
 */
public enum Product
{
	// Smelt on a furnace opens the make-X; "Make | Molten glass", one glass every 2 ticks (seen in
	// game, 2026-11-18).
	MOLTEN_GLASS("Molten glass", ItemID.MOLTEN_GLASS, "Molten glass", Skill.CRAFTING, Station.FURNACE, 2,
		new Input[]{
			new Input(ItemID.BUCKET_SAND, "Bucket of sand", 1),
			new Input(ItemID.SODA_ASH, "Soda ash", 1)
		},
		new Input[0]);

	private final String displayName;
	final int itemId;
	final String option;
	final Skill skill;
	final Station station;
	/** Game ticks between two actions of a batch. */
	final int ticksPerAction;
	final Input[] inputs;
	final Input[] tools;

	Product(String displayName, int itemId, String option, Skill skill, Station station, int ticksPerAction,
		Input[] inputs, Input[] tools)
	{
		this.displayName = displayName;
		this.itemId = itemId;
		this.option = option;
		this.skill = skill;
		this.station = station;
		this.ticksPerAction = ticksPerAction;
		this.inputs = inputs;
		this.tools = tools;
	}

	/**
	 * How long a batch may go without using an input before it counts as stopped (a level-up, or
	 * the click did not land): two actions' worth plus a tick.
	 */
	long stalledAfterMs()
	{
		return (2L * ticksPerAction + 1) * 600;
	}

	@Override
	public String toString()
	{
		return displayName;
	}

	/** An item of a product: an input (perAction used by each action) or a tool (perAction 1, kept). */
	static final class Input
	{
		final int itemId;
		final String name;
		final int perAction;

		Input(int itemId, String name, int perAction)
		{
			this.itemId = itemId;
			this.name = name;
			this.perAction = perAction;
		}
	}
}
