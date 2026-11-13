package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;

/**
 * The executor's wire, pinned on both sides: task-wire.json is what the planner's
 * internal/core/execspec TestPluginWire writes (the agent's start bodies, the wilderness probes and
 * the tasks both sides refuse).
 */
class TaskSpecTest
{
	static JsonObject wire()
	{
		try (InputStream in = TaskSpecTest.class.getResourceAsStream("task-wire.json"))
		{
			return new JsonParser().parse(new InputStreamReader(in, StandardCharsets.UTF_8)).getAsJsonObject();
		}
		catch (IOException e)
		{
			throw new java.io.UncheckedIOException(e);
		}
	}

	/** The start body of a type, as the agent posts it. */
	static JsonObject start(String type)
	{
		for (JsonElement e : wire().getAsJsonArray("starts"))
		{
			JsonObject s = e.getAsJsonObject();
			if (type.equals(s.getAsJsonObject("task").get("type").getAsString()))
			{
				return s;
			}
		}
		throw new AssertionError("no start of type " + type);
	}

	static TaskSpec spec(String type)
	{
		return TaskSpec.parse(new Gson(), start(type).get("task"));
	}

	@Test
	void everyStartOfThePlannerParsesAndChecks()
	{
		Gson gson = new Gson();
		for (JsonElement e : wire().getAsJsonArray("starts"))
		{
			TaskSpec t = TaskSpec.parse(gson, e.getAsJsonObject().get("task"));
			t.check();
			assertFalse(t.label().isEmpty());
		}
	}

	@Test
	void travelCarriesTheAreaAndTheTargetInsideIt()
	{
		TaskSpec t = spec(TaskSpec.TRAVEL);
		assertEquals("location:lumbridge_castle", t.travel.location);
		assertTrue(t.travel.area.contains(t.travel.target.x, t.travel.target.y, t.travel.target.plane));
		assertFalse(t.travel.wilderness);
		// The polygon decides, not only the box: the box's corner is outside the castle.
		assertFalse(t.travel.area.contains(t.travel.area.minX, t.travel.area.maxY, 0));
		assertFalse(t.travel.area.contains(t.travel.target.x, t.travel.target.y, 1));
	}

	@Test
	void bankAndBuyKeepEveryField()
	{
		TaskSpec bank = spec(TaskSpec.BANK);
		assertEquals(5, bank.bank.items.size());
		TaskSpec.BankItem shrimps = bank.bank.items.get(2);
		assertEquals(50, shrimps.qty);
		assertTrue(shrimps.noted);
		TaskSpec.BankItem axe = bank.bank.items.get(3);
		assertTrue(axe.equip);
		assertArrayEquals(new int[]{1359, 1357, 1355, 1353, 1349, 1351}, axe.ids);
		assertTrue(bank.bank.items.get(4).optional);

		TaskSpec buy = spec(TaskSpec.BUY);
		assertEquals(1351, buy.buy.item);
		assertEquals(1, buy.buy.qty);
		assertArrayEquals(new int[]{2812, 10619}, buy.buy.npcs);
		assertEquals("comprar 1 Bronze axe em Bob's Brilliant Axes", buy.label());
	}

	@Test
	void theWildernessHasTheSameBoundsOnBothSides()
	{
		for (JsonElement e : wire().getAsJsonArray("wilderness"))
		{
			JsonObject p = e.getAsJsonObject();
			int x = p.get("x").getAsInt();
			int y = p.get("y").getAsInt();
			assertEquals(p.get("in").getAsBoolean(), TaskSpec.inWilderness(x, y), x + ", " + y);
		}
		for (JsonElement e : wire().getAsJsonArray("starts"))
		{
			JsonObject travel = e.getAsJsonObject().getAsJsonObject("task").getAsJsonObject("travel");
			if (travel != null && "location:mage_bank".equals(travel.get("location").getAsString()))
			{
				assertTrue(travel.get("wilderness").getAsBoolean());
				assertTrue(TaskSpec.parse(new Gson(), e.getAsJsonObject().get("task")).travel.area.inWilderness());
			}
		}
	}

	@Test
	void whatThePlannerRefusesIsRefusedHereToo()
	{
		Gson gson = new Gson();
		for (JsonElement e : wire().getAsJsonArray("refused"))
		{
			assertThrows(IllegalArgumentException.class, () -> TaskSpec.parse(gson, e), e.toString());
		}
		IllegalArgumentException talk = assertThrows(IllegalArgumentException.class,
			() -> TaskSpec.parse(gson, new JsonParser().parse("{\"type\":\"talk\"}")));
		assertTrue(talk.getMessage().contains("not executed yet"));
	}

	@Test
	void aTamperedTaskIsRefused()
	{
		Gson gson = new Gson();
		JsonObject outside = start(TaskSpec.TRAVEL).getAsJsonObject("task").deepCopy();
		outside.getAsJsonObject("travel").getAsJsonObject("target").addProperty("x", 1);
		assertThrows(IllegalArgumentException.class, () -> TaskSpec.parse(gson, outside));

		JsonObject hidden = start(TaskSpec.TRAVEL).getAsJsonObject("task").deepCopy();
		hidden.getAsJsonObject("travel").addProperty("wilderness", true);
		assertThrows(IllegalArgumentException.class, () -> TaskSpec.parse(gson, hidden));

		JsonObject twice = start(TaskSpec.BUY).getAsJsonObject("task").deepCopy();
		twice.add("bank", start(TaskSpec.BANK).getAsJsonObject("task").get("bank"));
		assertThrows(IllegalArgumentException.class, () -> TaskSpec.parse(gson, twice));

		JsonObject tooMany = start(TaskSpec.BUY).getAsJsonObject("task").deepCopy();
		tooMany.getAsJsonObject("buy").addProperty("qty", 10_001);
		assertThrows(IllegalArgumentException.class, () -> TaskSpec.parse(gson, tooMany));

		assertThrows(IllegalArgumentException.class, () -> TaskSpec.parse(gson, new JsonParser().parse("[1]")));
	}
}
