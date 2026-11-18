package net.runelite.client.plugins.microbot.fm.processor;

import java.util.function.IntUnaryOperator;

/** How much one trip carries, from the stock of each input (pure, table-tested). */
final class Batch
{
	static final int INVENTORY = 28;

	private Batch()
	{
	}

	/**
	 * Actions one trip carries: as many as the slots left after the tools hold, limited by the
	 * input that runs out first. stock is what the bank and the inventory hold of an item id.
	 */
	static int actions(Product product, IntUnaryOperator stock)
	{
		int slotsPerAction = 0;
		for (Product.Input in : product.inputs)
		{
			slotsPerAction += in.perAction;
		}
		int n = (INVENTORY - product.tools.length) / slotsPerAction;
		for (Product.Input in : product.inputs)
		{
			n = Math.min(n, stock.applyAsInt(in.itemId) / in.perAction);
		}
		return Math.max(n, 0);
	}

	/** The input with the fewest actions left in stock: the reason a trip carries nothing. */
	static Product.Input scarcest(Product product, IntUnaryOperator stock)
	{
		Product.Input scarcest = null;
		int least = Integer.MAX_VALUE;
		for (Product.Input in : product.inputs)
		{
			int left = stock.applyAsInt(in.itemId) / in.perAction;
			if (left < least)
			{
				least = left;
				scarcest = in;
			}
		}
		return scarcest;
	}

	/** Whether the inventory still holds the inputs of one more action. */
	static boolean canAct(Product product, IntUnaryOperator held)
	{
		for (Product.Input in : product.inputs)
		{
			if (held.applyAsInt(in.itemId) < in.perAction)
			{
				return false;
			}
		}
		return true;
	}
}
