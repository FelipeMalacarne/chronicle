package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.awt.Color;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import net.runelite.client.config.Keybind;
import org.junit.jupiter.api.Test;

class ConfigSchemaExporterTest
{
	enum Logs
	{
		TEAK, MAHOGANY;

		@Override
		public String toString()
		{
			return name().charAt(0) + name().substring(1).toLowerCase();
		}
	}

	private static Map<String, Object> type(java.lang.reflect.Type t)
	{
		Map<String, Object> m = new HashMap<>();
		ConfigSchemaExporter.typeOf(t, m);
		return m;
	}

	@Test
	void mapsTheFormTypes()
	{
		assertEquals("boolean", type(boolean.class).get("type"));
		assertEquals("int", type(Integer.class).get("type"));
		assertEquals("double", type(double.class).get("type"));
		assertEquals("string", type(String.class).get("type"));
		assertEquals("color", type(Color.class).get("type"));
		assertEquals("keybind", type(Keybind.class).get("type"));
		assertEquals("other", type(Set.class).get("type"));
	}

	@Test
	void enumsCarryTheStoredNameAndTheLabel()
	{
		Map<String, Object> m = type(Logs.class);
		assertEquals("enum", m.get("type"));
		assertEquals(
			List.of(Map.of("value", "TEAK", "label", "Teak"), Map.of("value", "MAHOGANY", "label", "Mahogany")),
			m.get("values"));
	}
}
