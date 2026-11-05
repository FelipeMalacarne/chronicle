package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
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
}
