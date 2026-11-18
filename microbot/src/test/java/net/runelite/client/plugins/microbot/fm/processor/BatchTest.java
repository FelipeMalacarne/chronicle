package net.runelite.client.plugins.microbot.fm.processor;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.HashMap;
import java.util.Map;
import java.util.function.IntUnaryOperator;
import net.runelite.api.gameval.ItemID;
import org.junit.jupiter.api.Test;

class BatchTest
{
	private static IntUnaryOperator stock(int sand, int sodaAsh)
	{
		Map<Integer, Integer> held = new HashMap<>();
		held.put(ItemID.BUCKET_SAND, sand);
		held.put(ItemID.SODA_ASH, sodaAsh);
		return id -> held.getOrDefault(id, 0);
	}

	@Test
	void aFullTripOfMoltenGlassIsFourteenOfEach()
	{
		assertEquals(14, Batch.actions(Product.MOLTEN_GLASS, stock(600, 600)));
		assertEquals(14, Batch.actions(Product.MOLTEN_GLASS, stock(14, 14)));
	}

	@Test
	void theLastTripCarriesWhatTheScarcestInputAllows()
	{
		assertEquals(5, Batch.actions(Product.MOLTEN_GLASS, stock(600, 5)));
		assertEquals(Product.MOLTEN_GLASS.inputs[1], Batch.scarcest(Product.MOLTEN_GLASS, stock(600, 5)));
		assertEquals(0, Batch.actions(Product.MOLTEN_GLASS, stock(0, 600)));
		assertEquals(Product.MOLTEN_GLASS.inputs[0], Batch.scarcest(Product.MOLTEN_GLASS, stock(0, 600)));
	}

	@Test
	void oneActionLeftNeedsEveryInput()
	{
		assertTrue(Batch.canAct(Product.MOLTEN_GLASS, stock(1, 1)));
		assertFalse(Batch.canAct(Product.MOLTEN_GLASS, stock(14, 0)));
		assertFalse(Batch.canAct(Product.MOLTEN_GLASS, stock(0, 0)));
	}

	@Test
	void everySiteHasAStationAProductIsMadeAt()
	{
		for (Product p : Product.values())
		{
			boolean found = false;
			for (Site s : Site.values())
			{
				found |= s.station == p.station;
			}
			assertTrue(found, p + " has no site");
		}
	}
}
