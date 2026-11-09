package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.T0;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.function.ToIntFunction;
import org.junit.jupiter.api.Test;

class PanelTextTest
{
	/** One unit per character: wrap and ellipsis become exact. */
	private static final ToIntFunction<String> CHARS = String::length;

	@Test
	void objectivesBlockedAndStatus()
	{
		RouteWindow w = RouteWindowFixtures.window();
		RouteWindow.Step cur = w.current();
		RouteWindow.Objective met = cur.objectivesOrEmpty().get(0);
		RouteWindow.Objective unknown = cur.objectivesOrEmpty().get(1);
		assertEquals("15 Woodcutting", PanelText.objectiveLabel(met));
		assertEquals("feito", PanelText.verdictShort(met));
		assertEquals("sem dado", PanelText.verdictShort(unknown));
		RouteWindow.Step blocked = w.step("1.1.8");
		RouteWindow.Objective qp = blocked.objectivesOrEmpty().get(0);
		assertEquals("32 quest points (requer)", PanelText.objectiveLabel(qp));
		assertEquals("falta", PanelText.verdictShort(qp));
		met.regressed = true;
		assertEquals("não vale mais", PanelText.verdictShort(met));
		assertEquals("Falta: 32 quest points +1", PanelText.blocked(blocked.blockedOrEmpty()));
		assertEquals("Falta: 32 quest points, 40 Attack", PanelText.blockedAll(blocked.blockedOrEmpty()));
		assertEquals("", PanelText.blocked(cur.blockedOrEmpty()));
		assertEquals("pulado", PanelText.status("skipped"));
		assertEquals("detectado", PanelText.status("auto_done"));
		assertEquals("", PanelText.status("pending"));
	}

	@Test
	void stepKindsAreDistinctAndImpliedIsNotDone()
	{
		RouteWindow w = PanelScreenshotFixture.window();
		assertEquals(PanelText.StepKind.PENDING, PanelText.stepKind(w.step("1.1.243")));
		assertEquals(PanelText.StepKind.BLOCKED, PanelText.stepKind(w.step("1.1.244")));
		assertEquals(PanelText.StepKind.DONE, PanelText.stepKind(w.step("1.1.245")));
		assertEquals(PanelText.StepKind.DONE, PanelText.stepKind(w.step("1.1.246")));
		assertEquals(PanelText.StepKind.IMPLIED, PanelText.stepKind(w.step("1.1.248")));
		assertEquals(PanelText.StepKind.SKIPPED, PanelText.stepKind(w.step("1.1.251")));
		Set<String> words = new HashSet<>();
		for (PanelText.StepKind k : PanelText.StepKind.values())
		{
			words.add(PanelText.stepKindWord(k));
		}
		assertEquals(PanelText.StepKind.values().length, words.size(), "each kind has its own word");
	}

	@Test
	void routeNameHeaderAndCounts()
	{
		assertEquals("Oziris", PanelText.routeName("oziris-main", "OSRS Ironman Efficiency Guide (OzirisLoL)"));
		assertEquals("BRUHsailer", PanelText.routeName("bruhsailer-main", "BRUHsailer Ironman Guide"));
		assertEquals("Some Guide", PanelText.routeName(null, "Some Guide"));
		assertEquals("Chronicle", PanelText.routeName("", null));
		RouteWindow w = PanelScreenshotFixture.window();
		assertEquals("249/564", PanelText.progress(w.counts));
		assertEquals("62 bloqueados", PanelText.blockedCount(w.counts));
		w.counts.blocked = 1;
		assertEquals("1 bloqueado", PanelText.blockedCount(w.counts));
		w.counts.blocked = 0;
		assertEquals("", PanelText.blockedCount(w.counts));
		assertEquals("", PanelText.progress(null));
	}

	@Test
	void carryKindsOrderAndSummary()
	{
		List<RouteWindow.CarryItem> items = RouteWindowFixtures.window().carry.itemsOrEmpty();
		assertEquals("Hammer", PanelText.carryName(items.get(0)));
		assertEquals("com você", PanelText.carryWhere(items.get(0)));
		assertEquals("120 Feather", PanelText.carryName(items.get(1)));
		assertEquals("no banco", PanelText.carryWhere(items.get(1)));
		assertEquals("falta", PanelText.carryWhere(items.get(2)));
		assertEquals(PanelText.ItemKind.MISSING, PanelText.itemKind(items.get(2)));

		List<RouteWindow.CarryItem> preview = PanelScreenshotFixture.window().carry.itemsOrEmpty();
		assertEquals("5 faltam · 2 com você · 19 no banco · 1 sem dado", PanelText.carrySummary(preview));
		List<RouteWindow.CarryItem> ordered = PanelText.carryOrdered(preview);
		assertEquals(preview.size(), ordered.size());
		assertEquals("Cat", ordered.get(0).name, "missing first, in the guide's order");
		assertEquals("Orange dye", ordered.get(4).name);
		assertEquals(PanelText.ItemKind.UNKNOWN, PanelText.itemKind(ordered.get(5)));
		assertEquals("GP", ordered.get(6).name);
		assertEquals(PanelText.ItemKind.BANK, PanelText.itemKind(ordered.get(ordered.size() - 1)));
		assertEquals("1 falta", PanelText.carrySummary(List.of(items.get(2))));
		assertEquals("nenhum item", PanelText.carrySummary(List.of()));
	}

	@Test
	void notesLabelsMarkersAndHcimHidden()
	{
		RouteWindow w = PanelScreenshotFixture.window();
		RouteWindow.Step cur = w.current();
		assertEquals(3, cur.notesOrEmpty().size());
		List<RouteWindow.Note> visible = cur.visibleNotes();
		assertEquals(2, visible.size(), "hcim notes stay hidden: the window has no account mode");
		assertEquals("Atualização:", PanelText.noteLabel(visible.get(0)));
		assertEquals("Dica:", PanelText.noteLabel(visible.get(1)));
		assertEquals(RouteWindow.Note.UPDATE, PanelText.noteMarker(visible), "an update outranks a tip");
		assertEquals(RouteWindow.Note.TIP, PanelText.noteMarker(w.step("1.1.249").visibleNotes()));
		assertEquals(RouteWindow.Note.UPDATE, PanelText.noteMarker(w.step("1.1.250").visibleNotes()));
		assertNull(PanelText.noteMarker(w.step("1.1.247").visibleNotes()));
	}

	@Test
	void wrapAndEllipsis()
	{
		assertEquals(List.of("Do Black", "knight's", "fortress"), PanelText.wrap("Do Black knight's fortress", 9, CHARS));
		assertEquals(List.of("one two", "three"), PanelText.wrap("one two\nthree", 20, CHARS), "newlines are kept");
		assertEquals(List.of("abcde", "fghij", "k"), PanelText.wrap("abcdefghijk", 5, CHARS), "a long word is cut");
		assertEquals(List.of(), PanelText.wrap(null, 10, CHARS));
		assertEquals("Run south", PanelText.ellipsize("Run south", 9, CHARS));
		assertEquals("Run so…", PanelText.ellipsize("Run south to Port sarim", 7, CHARS));
		assertEquals("Run…", PanelText.ellipsize("Run south", 5, CHARS), "no space before the ellipsis");
		for (String line : PanelText.wrap("Minigame tele to clan wars, recharge energy and go to castle wars", 12, CHARS))
		{
			assertTrue(line.length() <= 12, line);
		}
	}

	@Test
	void connectionLines()
	{
		PanelState online = new PanelState(PanelState.Connection.ONLINE, RouteWindowFixtures.window(), T0, null, null);
		assertEquals("Conectado · há 12 s", PanelText.connection(online, T0.plusSeconds(12)));
		assertEquals("há 12 s", PanelText.connectionShort(online, T0.plusSeconds(12)));
		PanelState stale = new PanelState(PanelState.Connection.STALE, RouteWindowFixtures.window(), T0, "servidor fora do ar", null);
		assertEquals("Desatualizado · há 3 min · servidor fora do ar", PanelText.connection(stale, T0.plusSeconds(200)));
		assertEquals("desatualizado", PanelText.connectionShort(stale, T0.plusSeconds(200)));
		PanelState none = new PanelState(PanelState.Connection.NOT_CONFIGURED, null, null, null, null);
		assertEquals("sem URL", PanelText.connectionShort(none, T0));
		assertEquals("há 2 h", PanelText.ago(T0, T0.plusSeconds(7300)));
		assertEquals("várias contas: preencha Conta", PanelText.error(422, false));
	}
}
