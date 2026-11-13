package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.util.List;
import org.junit.jupiter.api.Test;

class RouteWindowTest
{
	@Test
	void parsesThePlannerWindowAndIgnoresUnknownFields()
	{
		RouteWindow w = RouteWindowFixtures.window();
		assertEquals("oziris-main", w.guide);
		assertEquals("Guia por OzirisLoL \u00b7 ironman.guide", w.attribution);
		assertEquals("1.1.7", w.cursorStepId());
		assertEquals(576, w.counts.total);
		assertEquals(4, w.stepsOrEmpty().size());

		RouteWindow.Step cur = w.current();
		assertEquals("1.1.7", cur.id);
		assertEquals("Early game", cur.sectionTitle);
		assertEquals("Lumbridge", cur.location);
		assertFalse(cur.blocked());
		assertEquals("met", cur.objectivesOrEmpty().get(0).verdict);
		assertEquals("15 Woodcutting", cur.objectivesOrEmpty().get(0).label);

		RouteWindow.Step blocked = w.step("1.1.8");
		assertTrue(blocked.blocked());
		assertEquals("32 quest points", blocked.blockedOrEmpty().get(0).label);

		RouteWindow.Step implied = w.step("1.1.10");
		assertEquals("implied", implied.mark.kind);
		assertEquals("1.1.12", implied.mark.impliedBy);

		assertEquals("known", w.carry.loadoutStatus);
		assertEquals(3, w.carry.itemsOrEmpty().size());
		assertEquals(120, w.carry.itemsOrEmpty().get(1).qty);
		assertEquals("Withdraw the hammer and 120 feathers.", w.carry.bankActionsOrEmpty().get(0));
	}

	@Test
	void aFinishedRouteHasNoCursorNorCarry()
	{
		RouteWindow w = new Gson().fromJson("{\"guide\":\"g\",\"title\":\"G\",\"attribution\":\"Guia por X\",\"cursor\":null,\"steps\":[],\"carry\":null}", RouteWindow.class);
		assertNull(w.current());
		assertNull(w.cursorStepId());
		assertTrue(w.stepsOrEmpty().isEmpty());
	}

	@Test
	void ingestResponseCarriesTheRouteAndOldResponsesStillParse()
	{
		Gson gson = new Gson();
		IngestContract.Response r = gson.fromJson("{\"contract_version\":1,\"ok\":true,\"accepted_seq\":3,\"resync\":false,"
			+ "\"manifest_version\":7,\"steps_changed\":2,\"route\":{\"guide\":\"oziris-main\",\"cursor\":\"1.1.8\","
			+ "\"cursor_cause\":\"auto\",\"changed\":[\"1.1.7\",\"1.1.6\"]}}", IngestContract.Response.class);
		assertEquals("1.1.8", r.route.cursor);
		assertEquals(2, r.route.changed.size());
		IngestContract.Response old = gson.fromJson("{\"contract_version\":1,\"ok\":true,\"accepted_seq\":3,\"resync\":false,\"manifest_version\":7}",
			IngestContract.Response.class);
		assertNull(old.route);
	}

	@Test
	void parsesNotesAndTheSectionAndOlderWindowsStillParse()
	{
		RouteWindow real = PanelScreenshotFixture.real();
		assertEquals("1.1", real.section.id);
		assertEquals("Early quests, wintertodt and ardy cloak 1", real.section.title);
		assertTrue(real.section.description.startsWith("Starting your ironman journey."));
		RouteWindow.Step lostTribe = real.step("1.1.254");
		assertEquals(1, lostTribe.visibleNotes().size());
		assertEquals(RouteWindow.Note.UPDATE, lostTribe.visibleNotes().get(0).kind);
		assertEquals(RouteWindow.Note.TIP, real.step("1.1.258a").notesOrEmpty().get(0).kind);
		assertTrue(real.current().notesOrEmpty().isEmpty(), "notes are omitted when a step has none");

		RouteWindow older = RouteWindowFixtures.window();
		assertNull(older.section, "an older planner sends no section");
		assertTrue(older.current().visibleNotes().isEmpty());

		RouteWindow hcim = new Gson().fromJson("{\"guide\":\"g\",\"steps\":[{\"id\":\"a\",\"current\":true,\"notes\":["
			+ "{\"kind\":\"hcim\",\"text\":\"Careful\"},{\"kind\":\"tip\",\"text\":\"\"},{\"kind\":\"tip\",\"text\":\"Bring food\"}]}]}",
			RouteWindow.class);
		assertEquals(3, hcim.current().notesOrEmpty().size());
		assertEquals(1, hcim.current().visibleNotes().size(), "hcim and empty notes are hidden");
		assertEquals("Bring food", hcim.current().visibleNotes().get(0).text);
	}

	/**
	 * The assisted mode's checklist, from the planner's own JSON: route-tasks.json is written by
	 * the planner's TestAssistedModeWire (internal/core/panel), so both sides read one contract.
	 */
	@Test
	void parsesTheCurrentStepsTasksFromThePlannersFixture()
	{
		RouteWindow w = PanelScreenshotFixture.tasks();
		RouteWindow.Step cur = w.current();
		List<RouteWindow.Task> tasks = cur.tasksOrEmpty();
		assertEquals(7, tasks.size());
		RouteWindow.Task train = tasks.get(0);
		assertEquals("train", train.type);
		assertTrue(train.recipe, "inherited from the recipe of the step's objective");
		assertEquals("10 Agility", train.until);
		RouteWindow.Task travel = tasks.get(1);
		assertEquals("Ir até Lumbridge Castle", travel.label);
		assertEquals("met", travel.state);
		assertFalse(travel.recipe);
		assertEquals("Falar com Hans: “Who are you?”", tasks.get(4).label);
		assertEquals("unmet", tasks.get(6).state);
		assertTrue(w.step("2").tasksOrEmpty().isEmpty(), "only the current step carries tasks");
		assertTrue(RouteWindowFixtures.window().current().tasksOrEmpty().isEmpty(), "an older planner sends none");

		assertEquals("feito", PanelText.taskState(travel));
		assertEquals("a fazer", PanelText.taskState(tasks.get(6)));
		RouteWindow.Task unknown = new RouteWindow.Task();
		unknown.state = "unknown";
		unknown.reason = "sem posição: ligue Send position";
		unknown.until = "na área do local";
		assertEquals("sem dado", PanelText.taskState(unknown), "unknown is never shown as to do");
		assertEquals("Termina: na área do local · sem posição: ligue Send position", PanelText.taskTooltip(unknown));
		assertEquals("Termina: 10 Agility · da receita do objetivo", PanelText.taskTooltip(train));
	}
}
