package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.T0;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;

import java.util.HashSet;
import java.util.List;
import java.util.Set;
import org.junit.jupiter.api.Test;

class PanelTextTest
{
	@Test
	void verdictsHaveDistinctGlyphs()
	{
		Set<String> glyphs = new HashSet<>(List.of(PanelText.verdictGlyph("met"), PanelText.verdictGlyph("unmet"), PanelText.verdictGlyph("unknown")));
		assertEquals(3, glyphs.size(), "not only colour tells them apart");
		assertEquals(PanelText.GLYPH_UNKNOWN, PanelText.verdictGlyph(null));
	}

	@Test
	void chipsBlockedAndStatus()
	{
		RouteWindow w = RouteWindowFixtures.window();
		RouteWindow.Step cur = w.current();
		assertEquals("\u2713 15 Woodcutting", PanelText.chip(cur.objectivesOrEmpty().get(0)));
		assertEquals("? Bronze axe", PanelText.chip(cur.objectivesOrEmpty().get(1)));
		RouteWindow.Step blocked = w.step("1.1.8");
		assertEquals("\u2717 32 quest points (requer)", PanelText.chip(blocked.objectivesOrEmpty().get(0)));
		assertEquals("falta 32 quest points +1", PanelText.blocked(blocked.blockedOrEmpty()));
		assertEquals("", PanelText.blocked(cur.blockedOrEmpty()));
		assertEquals("pulado", PanelText.status("skipped"));
		assertEquals("detectado", PanelText.status("auto_done"));
		assertEquals("", PanelText.status("pending"));
		assertNotEquals(PanelText.statusGlyph(w.step("1.1.9")), PanelText.statusGlyph(cur));
		assertEquals("34 de 576 feitos \u00b7 2 bloqueados", PanelText.counts(w.counts));
	}

	@Test
	void carryLines()
	{
		List<RouteWindow.CarryItem> items = RouteWindowFixtures.window().carry.itemsOrEmpty();
		assertEquals("Hammer", PanelText.carryName(items.get(0)));
		assertEquals("na mochila", PanelText.carryWhere(items.get(0)));
		assertEquals("120 Feather", PanelText.carryName(items.get(1)));
		assertEquals("no banco", PanelText.carryWhere(items.get(1)));
		assertEquals("falta", PanelText.carryWhere(items.get(2)));
		assertEquals(PanelText.GLYPH_UNMET, PanelText.carryGlyph(items.get(2)));
	}

	@Test
	void connectionLine()
	{
		PanelState online = new PanelState(PanelState.Connection.ONLINE, RouteWindowFixtures.window(), T0, null, null);
		assertEquals("Conectado \u00b7 h\u00e1 12 s", PanelText.connection(online, T0.plusSeconds(12)));
		PanelState stale = new PanelState(PanelState.Connection.STALE, RouteWindowFixtures.window(), T0, "servidor fora do ar", null);
		assertEquals("Desatualizado \u00b7 h\u00e1 3 min \u00b7 servidor fora do ar", PanelText.connection(stale, T0.plusSeconds(200)));
		assertEquals("v\u00e1rias contas: preencha Conta", PanelText.error(422, false));
	}
}
