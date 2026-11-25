package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.util.Map;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.magic.Runes;
import org.junit.jupiter.api.Test;

class MtaPrepTest
{
	/** The Enchanting Chamber's spell at each Magic level, with its runes per cast (OSRS Wiki). */
	@Test
	void enchantRunesAreTheWikis()
	{
		assertEquals(Map.of(Runes.WATER, 1, Runes.COSMIC, 1), runesAt(7));
		assertEquals(Map.of(Runes.AIR, 3, Runes.COSMIC, 1), runesAt(27));
		assertEquals(Map.of(Runes.FIRE, 5, Runes.COSMIC, 1), runesAt(49));
		assertEquals(Map.of(Runes.EARTH, 10, Runes.COSMIC, 1), runesAt(57));
		// Rs2Spells has these two without their earth runes.
		assertEquals(Map.of(Runes.WATER, 15, Runes.EARTH, 15, Runes.COSMIC, 1), runesAt(70));
		assertEquals(Map.of(Runes.EARTH, 20, Runes.FIRE, 20, Runes.COSMIC, 1), runesAt(87));
	}

	@Test
	void otherSpellsKeepTheClientsRunes()
	{
		assertEquals(Rs2Spells.HIGH_LEVEL_ALCHEMY.getRequiredRunes(),
			MtaPrep.runes(Rs2Spells.HIGH_LEVEL_ALCHEMY));
	}

	private static Map<Runes, Integer> runesAt(int magic)
	{
		int level = MtaCalculator.enchantLevelFor(magic, 0);
		return MtaPrep.runes(MtaPrep.spell(MtaWorld.Room.ENCHANTMENT, magic, level, false));
	}
}
