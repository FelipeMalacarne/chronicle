package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;

/** A route window as the planner sends it (docs/04 2.5), plus a planner response with it. */
final class RouteWindowFixtures
{
	/** Cursor 1.1.7 (pending, one met objective), a blocked step, a skipped step; a known loadout. */
	static final String WINDOW_JSON = "{"
		+ "\"guide\":\"oziris-main\",\"title\":\"OSRS Ironman Efficiency Guide (OzirisLoL)\","
		+ "\"attribution\":\"Guia por OzirisLoL \u00b7 ironman.guide\","
		+ "\"cursor\":{\"step_id\":\"1.1.7\",\"cause\":\"auto\"},"
		+ "\"counts\":{\"total\":576,\"done\":34,\"auto_done\":30,\"manual_done\":4,\"implied\":0,\"skipped\":1,\"pinned_open\":0,\"remaining\":541,\"blocked\":2},"
		+ "\"steps\":["
		+ "{\"id\":\"1.1.7\",\"ref\":\"1.1.7\",\"section_id\":\"1.1\",\"section_title\":\"Early game\",\"title\":\"Talk to Bob\","
		+ "\"text\":\"Talk to Bob in Lumbridge and buy an axe.\",\"location\":\"Lumbridge\",\"status\":\"pending\",\"current\":true,"
		+ "\"blocked_by\":[],\"objectives\":["
		+ "{\"key\":\"skill:woodcutting>=15\",\"role\":\"completes\",\"label\":\"15 Woodcutting\",\"verdict\":\"met\"},"
		+ "{\"key\":\"item:1351x1\",\"role\":\"completes\",\"label\":\"Bronze axe\",\"verdict\":\"unknown\"}]},"
		+ "{\"id\":\"1.1.8\",\"ref\":\"1.1.8\",\"section_id\":\"1.1\",\"title\":\"Start Dragon Slayer I\",\"text\":\"Start Dragon Slayer I.\","
		+ "\"status\":\"pending\",\"current\":false,\"blocked_by\":[{\"key\":\"qp>=32\",\"label\":\"32 quest points\"},{\"key\":\"skill:attack>=40\",\"label\":\"40 Attack\"}],"
		+ "\"objectives\":[{\"key\":\"qp>=32\",\"role\":\"requires\",\"label\":\"32 quest points\",\"verdict\":\"unmet\"}]},"
		+ "{\"id\":\"1.1.9\",\"ref\":\"1.1.9\",\"section_id\":\"1.1\",\"title\":\"Walk\",\"text\":\"Walk.\",\"status\":\"skipped\","
		+ "\"mark\":{\"kind\":\"skipped\"},\"current\":false,\"blocked_by\":[],\"objectives\":[]},"
		+ "{\"id\":\"1.1.10\",\"ref\":\"1.1.10\",\"section_id\":\"1.1\",\"title\":\"Cook\",\"text\":\"Cook.\",\"status\":\"pending\","
		+ "\"mark\":{\"kind\":\"implied\",\"implied_by_step_id\":\"1.1.12\"},\"current\":false,\"blocked_by\":[],\"objectives\":[],"
		+ "\"future_field\":{\"ignored\":true}}],"
		+ "\"carry\":{\"segment_id\":\"seg-1.1-3\",\"title\":\"Lumbridge bank\",\"status\":\"not_ready\",\"loadout_status\":\"known\","
		+ "\"derived\":false,\"items\":["
		+ "{\"name\":\"Hammer\",\"qty\":1,\"qty_stated\":true,\"slot\":\"inventory\",\"verdict\":\"met\",\"carried\":1,\"owned\":\"met\",\"held\":1},"
		+ "{\"name\":\"Feather\",\"qty\":120,\"qty_stated\":true,\"slot\":\"inventory\",\"verdict\":\"unmet\",\"carried\":0,\"owned\":\"met\",\"held\":300},"
		+ "{\"name\":\"Rune axe\",\"qty\":1,\"qty_stated\":false,\"slot\":\"equipment\",\"verdict\":\"unmet\",\"carried\":0,\"owned\":\"unmet\",\"held\":0}],"
		+ "\"bank_actions\":[\"Withdraw the hammer and 120 feathers.\"]}"
		+ "}";

	private RouteWindowFixtures()
	{
	}

	static RouteWindow window()
	{
		return new Gson().fromJson(WINDOW_JSON, RouteWindow.class);
	}

	/** The same window with the cursor on another step. */
	static RouteWindow windowAt(String cursor)
	{
		RouteWindow w = window();
		w.cursor.stepId = cursor;
		for (RouteWindow.Step s : w.steps)
		{
			s.current = s.id.equals(cursor);
		}
		return w;
	}

	static IngestContract.Response ackWithRoute(long seq, String guide, String cursor, String... changed)
	{
		IngestContract.Response r = Fixtures.ack(seq, 7);
		r.route = new IngestContract.RouteUpdate();
		r.route.guide = guide;
		r.route.cursor = cursor;
		r.route.changed = java.util.Arrays.asList(changed);
		return r;
	}
}
