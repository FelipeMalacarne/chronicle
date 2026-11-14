package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertEquals;

import org.junit.jupiter.api.Test;

class MtaTextTest
{
	@Test
	void lastNumber()
	{
		assertEquals(1234, MtaText.lastNumber("Telekinetic: <col=ffffff>1,234</col>"));
		assertEquals(0, MtaText.lastNumber("0"));
		assertEquals(87, MtaText.lastNumber("Level 3: 87"));
		assertEquals(-1, MtaText.lastNumber("none yet"));
		assertEquals(-1, MtaText.lastNumber(""));
		assertEquals(-1, MtaText.lastNumber(null));
		assertEquals(-1, MtaText.lastNumber("99999999999"));
	}
}
