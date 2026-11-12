package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import java.awt.Color;
import java.io.File;
import java.io.IOException;
import java.lang.reflect.Type;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.RuneLite;
import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigDescriptor;
import net.runelite.client.config.ConfigItem;
import net.runelite.client.config.ConfigItemDescriptor;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.config.ConfigSectionDescriptor;
import net.runelite.client.config.Keybind;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.plugins.PluginManager;

/**
 * Writes the config options of every loaded Microbot plugin (group, sections, items with their
 * type, range and enum values) to .runelite/agent/config-schema.json, so the planner's agent
 * can show a form instead of raw key/value: the Agent Server reads and writes values but does
 * not list a plugin's options. Secret items are left out. Values are not exported; they stay
 * behind /settings/plugin.
 */
@Slf4j
final class ConfigSchemaExporter
{
	static final String FILE = "config-schema.json";
	private static final String MICROBOT_PACKAGE = "net.runelite.client.plugins.microbot";

	private final PluginManager pluginManager;
	private final ConfigManager configManager;
	private final Gson gson;

	ConfigSchemaExporter(PluginManager pluginManager, ConfigManager configManager, Gson gson)
	{
		this.pluginManager = pluginManager;
		this.configManager = configManager;
		this.gson = gson;
	}

	/** Writes the file atomically; failures are logged, never thrown. */
	void export()
	{
		try
		{
			List<Map<String, Object>> plugins = new ArrayList<>();
			for (Plugin plugin : pluginManager.getPlugins())
			{
				if (!plugin.getClass().getName().startsWith(MICROBOT_PACKAGE))
				{
					continue;
				}
				Map<String, Object> entry = describe(plugin);
				if (entry != null)
				{
					plugins.add(entry);
				}
			}
			plugins.sort(Comparator.comparing(p -> (String) p.get("className")));
			Map<String, Object> out = new LinkedHashMap<>();
			out.put("version", 1);
			out.put("generatedAt", Instant.now().toString());
			out.put("plugins", plugins);
			write(gson.toJson(out));
		}
		catch (RuntimeException | IOException e)
		{
			log.warn("Chronicle Link: config schema export failed", e);
		}
	}

	private Map<String, Object> describe(Plugin plugin)
	{
		Config proxy = pluginManager.getPluginConfigProxy(plugin);
		if (proxy == null)
		{
			return null;
		}
		ConfigDescriptor cd = configManager.getConfigDescriptor(proxy);
		if (cd == null || cd.getGroup() == null)
		{
			return null;
		}
		PluginDescriptor pd = plugin.getClass().getAnnotation(PluginDescriptor.class);
		Map<String, Object> entry = new LinkedHashMap<>();
		entry.put("className", plugin.getClass().getName());
		entry.put("name", pd != null ? pd.name() : plugin.getClass().getSimpleName());
		entry.put("group", cd.getGroup().value());

		List<Map<String, Object>> sections = new ArrayList<>();
		for (ConfigSectionDescriptor s : cd.getSections())
		{
			Map<String, Object> m = new LinkedHashMap<>();
			m.put("key", s.key());
			m.put("name", s.name());
			m.put("position", s.position());
			m.put("description", s.getSection().description());
			m.put("closedByDefault", s.getSection().closedByDefault());
			sections.add(m);
		}
		sections.sort(Comparator.comparingInt(m -> (int) m.get("position")));
		entry.put("sections", sections);

		List<Map<String, Object>> items = new ArrayList<>();
		for (ConfigItemDescriptor d : cd.getItems())
		{
			ConfigItem item = d.getItem();
			if (item.secret())
			{
				continue;
			}
			Map<String, Object> m = new LinkedHashMap<>();
			m.put("key", item.keyName());
			m.put("name", item.name());
			m.put("description", item.description());
			m.put("position", item.position());
			m.put("section", item.section());
			m.put("hidden", item.hidden());
			if (!item.warning().isEmpty())
			{
				m.put("warning", item.warning());
			}
			typeOf(d.getType(), m);
			if (d.getRange() != null)
			{
				m.put("min", d.getRange().min());
				m.put("max", d.getRange().max());
			}
			if (d.getUnits() != null)
			{
				m.put("units", d.getUnits().value());
			}
			items.add(m);
		}
		items.sort(Comparator.comparingInt(m -> (int) m.get("position")));
		entry.put("items", items);
		return entry;
	}

	/** The form type of an item: boolean, int, double, string, enum (with values), color, keybind or other. */
	static void typeOf(Type type, Map<String, Object> m)
	{
		if (!(type instanceof Class))
		{
			m.put("type", "other");
			return;
		}
		Class<?> c = (Class<?>) type;
		if (c == boolean.class || c == Boolean.class)
		{
			m.put("type", "boolean");
		}
		else if (c == int.class || c == Integer.class || c == long.class || c == Long.class)
		{
			m.put("type", "int");
		}
		else if (c == double.class || c == Double.class || c == float.class || c == Float.class)
		{
			m.put("type", "double");
		}
		else if (c == String.class)
		{
			m.put("type", "string");
		}
		else if (c.isEnum())
		{
			m.put("type", "enum");
			List<Map<String, String>> values = new ArrayList<>();
			for (Object constant : c.getEnumConstants())
			{
				Map<String, String> v = new LinkedHashMap<>();
				// ConfigManager stores an enum by name(); the panel shows toString().
				v.put("value", ((Enum<?>) constant).name());
				v.put("label", constant.toString());
				values.add(v);
			}
			m.put("values", values);
		}
		else if (c == Color.class)
		{
			m.put("type", "color");
		}
		else if (Keybind.class.isAssignableFrom(c))
		{
			m.put("type", "keybind");
		}
		else
		{
			m.put("type", "other");
		}
	}

	private static void write(String json) throws IOException
	{
		File dir = new File(RuneLite.RUNELITE_DIR, "agent");
		if (!dir.isDirectory() && !dir.mkdirs())
		{
			throw new IOException("cannot create " + dir);
		}
		File tmp = new File(dir, FILE + ".tmp");
		Files.write(tmp.toPath(), json.getBytes(StandardCharsets.UTF_8));
		Files.move(tmp.toPath(), new File(dir, FILE).toPath(), StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE);
	}
}
