package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.ArrayList;
import java.util.List;

/**
 * A window like the owner's screenshot of 2026-11-05: Oziris at 1.1.243 with an unmet quest,
 * a long derived item preview (27 items, most in the bank) and ten next steps with done,
 * implied, skipped and blocked ones.
 */
final class PanelScreenshotFixture
{
	private PanelScreenshotFixture()
	{
	}

	static RouteWindow window()
	{
		RouteWindow w = new RouteWindow();
		w.guide = "oziris-main";
		w.title = "OSRS Ironman Efficiency Guide (OzirisLoL)";
		w.attribution = "Guia por OzirisLoL · ironman.guide";
		w.cursor = new RouteWindow.Cursor();
		w.cursor.stepId = "1.1.243";
		w.cursor.cause = "auto";
		w.counts = new RouteWindow.Counts();
		w.counts.total = 564;
		w.counts.done = 249;
		w.counts.remaining = 315;
		w.counts.blocked = 62;
		w.steps = new ArrayList<>();
		w.section = new RouteWindow.Section();
		w.section.id = "1.1";
		w.section.title = "Early quests, wintertodt and ardy cloak 1";
		w.section.description = "Starting your ironman journey. Complete early quests, get 50+ firemaking at Wintertodt, "
			+ "and unlock the Ardougne cloak 1 for its teleport.";

		RouteWindow.Step cur = step("1.1.243", "Do Black knight's fortress", "pending");
		cur.current = true;
		cur.location = "Falador";
		cur.objectives = List.of(
			objective("quest:black_knights_fortress", "completes", "Black Knights' Fortress", "unmet"),
			objective("qp>=12", "requires", "12 quest points", "met"),
			objective("item:iron_chainbody", "uses", "Iron chainbody", "unknown"));
		cur.notes = List.of(
			note("update", "The quest now needs 12 quest points to start; do it after the Port Sarim run if you are short."),
			note("tip", "Bring the Iron chainbody and the cabbage from the Draynor manor garden."),
			note("hcim", "Hardcores: the Black Knights hit hard, bring food."));
		w.steps.add(cur);

		RouteWindow.Step s244 = step("1.1.244", "Do Recruitment drive", "pending");
		s244.blockedBy = List.of(requirement("quest:black_knights_fortress", "Black Knights' Fortress"));
		s244.objectives = List.of(objective("quest:recruitment_drive", "completes", "Recruitment Drive", "unmet"));
		w.steps.add(s244);
		w.steps.add(step("1.1.245", "Run south to Port sarim", "manual_done"));
		w.steps.add(step("1.1.246", "Trade Sarah's farming shop for a falador diary task", "auto_done"));
		w.steps.add(step("1.1.247", "Buy a black wizards hat from Betty at port sarim magic store", "pending"));
		RouteWindow.Step s248 = step("1.1.248", "Port Sarim jewellery store, give the bread to the beggar to get the excalibur", "implied");
		s248.mark = new RouteWindow.Mark("implied", "1.1.252");
		w.steps.add(s248);
		RouteWindow.Step s249 = step("1.1.249", "Minigame tele to clan wars, recharge energy and go to castle wars", "pending");
		s249.notes = List.of(note("tip", "Castle wars has a free bank chest next to the energy pool."));
		w.steps.add(s249);
		RouteWindow.Step s250 = step("1.1.250", "Hop worlds for super antipoison(1), decant them until you have like 6 full pots", "pending");
		s250.notes = List.of(note("update", "Skip this step: super antipoison is no longer sold in that shop."));
		w.steps.add(s250);
		RouteWindow.Step s251 = step("1.1.251", "Do the Observatory quest", "skipped");
		s251.mark = new RouteWindow.Mark("skipped", null);
		w.steps.add(s251);
		w.steps.add(step("1.1.252", "HOME TELE, Lumby", "pending"));
		w.steps.add(step("1.1.253", "Finish The Restless ghost", "pending"));

		w.carry = new RouteWindow.Carry();
		w.carry.segmentId = "seg-1.1-12";
		w.carry.title = "trecho de banco 1.1.178 – 1.1.282";
		w.carry.status = "not_ready";
		w.carry.loadoutStatus = "derived";
		w.carry.derived = true;
		w.carry.items = new ArrayList<>();
		String[][] items = {
			{"GP", "1", "met", "met"}, {"Bread", "1", "unmet", "met"}, {"Plank", "3", "unmet", "met"},
			{"Bronze bar", "1", "unmet", "met"}, {"Molten glass", "1", "unmet", "met"}, {"Ghost's skull", "1", "unmet", "met"},
			{"Chronicle", "1", "met", "met"}, {"Cat", "1", "unmet", "unmet"}, {"Pickaxe", "1", "unmet", "met"},
			{"Hammer", "1", "unmet", "met"}, {"Lit candle", "1", "unmet", "unmet"}, {"Bat bones", "1", "unmet", "met"},
			{"Black candle", "1", "unmet", "met"}, {"Tinderbox", "1", "unmet", "met"}, {"Knife", "1", "unmet", "met"},
			{"Needle", "1", "unmet", "met"}, {"Thread", "1", "unmet", "met"}, {"Coal", "4", "unmet", "met"},
			{"Lost tribe brooch", "1", "unmet", "unmet"}, {"Goblin symbol book", "1", "unmet", "unmet"},
			{"Blue dye", "1", "unmet", "met"}, {"Orange dye", "1", "unmet", "unmet"}, {"Silver ore", "5", "unmet", "met"},
			{"Sickle mould", "1", "unmet", "met"}, {"Holy mould", "1", "unmet", "met"}, {"Key print", "1", "unmet", "met"},
			{"Iron chainbody", "1", "unknown", null},
		};
		for (String[] it : items)
		{
			RouteWindow.CarryItem c = new RouteWindow.CarryItem();
			c.name = it[0];
			c.qty = Long.parseLong(it[1]);
			c.qtyStated = c.qty > 1;
			c.slot = "inventory";
			c.verdict = it[2];
			c.owned = it[3];
			w.carry.items.add(c);
		}
		w.carry.bankActions = List.of("Deposit everything except the GP.");
		return w;
	}

	private static RouteWindow.Step step(String ref, String title, String status)
	{
		RouteWindow.Step s = new RouteWindow.Step();
		s.id = ref;
		s.ref = ref;
		s.sectionId = "1.1";
		s.sectionTitle = "Early quests, wintertodt and ardy cloak 1";
		s.title = title;
		s.text = title;
		s.status = status;
		return s;
	}

	private static RouteWindow.Note note(String kind, String text)
	{
		RouteWindow.Note n = new RouteWindow.Note();
		n.kind = kind;
		n.text = text;
		return n;
	}

	/** The planner's real answer at cursor 1.1.244 (2026-11-05, read-only GET), with notes and the section. */
	static RouteWindow real()
	{
		try (java.io.InputStream in = PanelScreenshotFixture.class.getResourceAsStream("route-oziris-1.1.244.json"))
		{
			return new com.google.gson.Gson().fromJson(
				new java.io.InputStreamReader(in, java.nio.charset.StandardCharsets.UTF_8), RouteWindow.class);
		}
		catch (java.io.IOException e)
		{
			throw new java.io.UncheckedIOException(e);
		}
	}

	private static RouteWindow.Objective objective(String key, String role, String label, String verdict)
	{
		RouteWindow.Objective o = new RouteWindow.Objective();
		o.key = key;
		o.role = role;
		o.label = label;
		o.verdict = verdict;
		return o;
	}

	private static RouteWindow.Requirement requirement(String key, String label)
	{
		RouteWindow.Requirement r = new RouteWindow.Requirement();
		r.key = key;
		r.label = label;
		return r;
	}
}
