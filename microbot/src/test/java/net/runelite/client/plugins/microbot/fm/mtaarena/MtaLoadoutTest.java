package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaLoadout.Choice;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaLoadout.Gear;
import org.junit.jupiter.api.Test;

class MtaLoadoutTest
{
	private static final Gear AIR_STAFF = gear(1381, false, "AIR");
	private static final Gear WATER_STAFF = gear(1383, false, "WATER");
	private static final Gear EARTH_STAFF = gear(1385, false, "EARTH");
	private static final Gear FIRE_STAFF = gear(1387, false, "FIRE");
	private static final Gear MUD_STAFF = gear(6562, false, "WATER", "EARTH");
	private static final Gear TOME_OF_FIRE = gear(20714, true, "FIRE");
	/** What the owner has: the four elemental staves and a tome of fire. */
	private static final List<Gear> OWNED = Arrays.asList(AIR_STAFF, WATER_STAFF, EARTH_STAFF, FIRE_STAFF, TOME_OF_FIRE);
	private static final Map<String, Integer> PLENTY = stock("AIR", 500, "WATER", 2900, "EARTH", 2899, "FIRE", 700);

	private static Gear gear(int id, boolean tome, String... runes)
	{
		return new Gear(id, tome, new HashSet<>(Arrays.asList(runes)));
	}

	private static Set<String> runes(String... names)
	{
		return new HashSet<>(Arrays.asList(names));
	}

	private static Map<String, Integer> stock(Object... pairs)
	{
		Map<String, Integer> m = new HashMap<>();
		for (int i = 0; i < pairs.length; i += 2)
		{
			m.put((String) pairs[i], (Integer) pairs[i + 1]);
		}
		return m;
	}

	@Test
	void enchantLvl2WearsTheAirStaffAndNoTome()
	{
		Choice c = MtaLoadout.choose(runes("AIR"), OWNED, PLENTY, -1, -1);
		assertEquals(AIR_STAFF, c.staff);
		assertNull(c.tome);
		assertEquals(Collections.emptySet(), c.uncovered);
	}

	@Test
	void alchemyPrefersTheStaffOfFireOverTheTomeThatBurnsPages()
	{
		Choice c = MtaLoadout.choose(runes("FIRE"), OWNED, PLENTY, -1, TOME_OF_FIRE.itemId);
		assertEquals(FIRE_STAFF, c.staff);
		assertNull(c.tome);
	}

	@Test
	void enchantLvl6IsAnEarthStaffWithTheTomeOfFire()
	{
		Choice c = MtaLoadout.choose(runes("EARTH", "FIRE"), OWNED, PLENTY, -1, -1);
		assertEquals(EARTH_STAFF, c.staff);
		assertEquals(TOME_OF_FIRE, c.tome);
		assertEquals(Collections.emptySet(), c.uncovered);
	}

	@Test
	void graveyardLeavesToCarryTheRuneThereIsStockOf()
	{
		// Earth and water, no mud staff: one staff, and the other rune is carried. Without water
		// runes the water staff is the one to wear.
		Choice c = MtaLoadout.choose(runes("EARTH", "WATER"), OWNED, stock("EARTH", 2899), -1, -1);
		assertEquals(WATER_STAFF, c.staff);
		assertEquals(runes("EARTH"), c.uncovered);
	}

	@Test
	void graveyardKeepsTheStaffAlreadyWornWhenBothRunesAreStocked()
	{
		Choice c = MtaLoadout.choose(runes("EARTH", "WATER"), OWNED, PLENTY, EARTH_STAFF.itemId, -1);
		assertEquals(EARTH_STAFF, c.staff);
		assertEquals(runes("WATER"), c.uncovered);
	}

	@Test
	void aCombinationStaffCoversBothRunes()
	{
		Choice c = MtaLoadout.choose(runes("EARTH", "WATER"), Arrays.asList(EARTH_STAFF, MUD_STAFF), PLENTY, -1, -1);
		assertEquals(MUD_STAFF, c.staff);
		assertEquals(Collections.emptySet(), c.uncovered);
	}

	@Test
	void noElementalRuneNeededLeavesBothSlotsAlone()
	{
		Choice c = MtaLoadout.choose(Collections.emptySet(), OWNED, PLENTY, AIR_STAFF.itemId, -1);
		assertNull(c.staff);
		assertNull(c.tome);
	}
}
