package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import java.io.InputStreamReader;
import java.io.Reader;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;

/**
 * The same vectors as the Go core's TestVectors (internal/core/mta/testdata/vectors.json, copied
 * to src/test/resources/mta-vectors.json; the Go test TestVectorsCopy fails when they differ):
 * both calculators must give the same answers.
 */
class MtaCalculatorTest
{
	private static JsonArray vectors() throws Exception
	{
		try (Reader r = new InputStreamReader(
			MtaCalculatorTest.class.getResourceAsStream("/mta-vectors.json"), StandardCharsets.UTF_8))
		{
			return new JsonParser().parse(r).getAsJsonArray();
		}
	}

	private static List<String> strings(JsonArray a)
	{
		List<String> out = new ArrayList<>();
		for (JsonElement e : a)
		{
			out.add(e.getAsString());
		}
		return out;
	}

	private static MtaCalculator.Points points(JsonObject o)
	{
		return new MtaCalculator.Points(o.get("telekinetic").getAsInt(), o.get("graveyard").getAsInt(),
			o.get("enchantment").getAsInt(), o.get("alchemist").getAsInt());
	}

	@Test
	void sharedVectors() throws Exception
	{
		JsonArray all = vectors();
		assertTrue(all.size() >= 9);
		for (JsonElement el : all)
		{
			JsonObject v = el.getAsJsonObject();
			String name = v.get("name").getAsString();
			JsonObject in = v.getAsJsonObject("input");
			List<String> rewards = strings(in.getAsJsonArray("rewards"));
			List<String> owned = strings(in.getAsJsonArray("owned"));
			MtaCalculator.Points held = points(in.getAsJsonObject("points"));
			int magic = in.get("magic").getAsInt();
			int level = in.get("enchant_level").getAsInt();
			if (v.has("error") && v.get("error").getAsBoolean())
			{
				assertThrows(MtaCalculator.MtaException.class,
					() -> MtaCalculator.calculate(rewards, owned, held, magic, level), name);
				continue;
			}
			JsonObject want = v.getAsJsonObject("want");
			MtaCalculator.Result got = MtaCalculator.calculate(rewards, owned, held, magic, level);
			assertEquals(strings(want.getAsJsonArray("buy")), got.buy, name + ": buy");
			assertEquals(points(want.getAsJsonObject("needed")), got.needed, name + ": needed");
			assertEquals(points(want.getAsJsonObject("missing")), got.missing, name + ": missing");
			JsonObject runes = want.getAsJsonObject("runes");
			assertEquals(new MtaCalculator.Runes(runes.get("law").getAsInt(), runes.get("nature").getAsInt(),
				runes.get("cosmic").getAsInt()), got.runes, name + ": runes");
			assertEquals(want.get("enchant_level").getAsInt(), got.enchantLevel, name + ": enchant level");
		}
	}

	@Test
	void shopTotalsMatchTheWiki()
	{
		int t = 0;
		int g = 0;
		int e = 0;
		int a = 0;
		for (MtaReward r : MtaData.REWARDS)
		{
			t += r.telekinetic;
			g += r.graveyard;
			e += r.enchantment;
			a += r.alchemist;
		}
		assertEquals(2825, t);
		assertEquals(2825, g);
		assertEquals(29000, e);
		assertEquals(3275, a);
	}

	@Test
	void enchantLevelFollowsMagic()
	{
		assertEquals(0, MtaCalculator.enchantLevelFor(6, 0));
		assertEquals(1, MtaCalculator.enchantLevelFor(7, 0));
		assertEquals(5, MtaCalculator.enchantLevelFor(86, 0));
		assertEquals(6, MtaCalculator.enchantLevelFor(99, 0));
		assertEquals(2, MtaCalculator.enchantLevelFor(99, 2));
		assertEquals(0, MtaCalculator.enchantLevelFor(99, 7));
	}
}
