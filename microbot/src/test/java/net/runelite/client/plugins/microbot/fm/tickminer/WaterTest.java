package net.runelite.client.plugins.microbot.fm.tickminer;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import net.runelite.api.coords.WorldPoint;
import org.junit.jupiter.api.Test;

class WaterTest
{
	@Test
	void theCactiAreAReserveForLowWater()
	{
		assertTrue(Water.worthATrip(8, 16, 1));
		assertTrue(Water.worthATrip(0, 16, 3));
		// Room for a sip from each of the three is no reason to go any more.
		assertFalse(Water.worthATrip(9, 16, 3));
		assertFalse(Water.worthATrip(13, 16, 3));
	}

	@Test
	void noTripWithoutACactusUpOrRoomInTheSkins()
	{
		assertFalse(Water.worthATrip(2, 16, 0));
		// Two skins, full but under the reserve.
		assertFalse(Water.worthATrip(8, 8, 3));
		assertTrue(Water.worthATrip(7, 8, 3));
	}

	@Test
	void theCactiEastOfTheQuarry()
	{
		assertTrue(Water.isCactus(new WorldPoint(3192, 2903, 0)));
		assertFalse(Water.isCactus(new WorldPoint(3155, 2869, 0)));
	}
}
