package net.runelite.client.plugins.microbot.fm.processor;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;

/**
 * Bank, make a product at a station, bank again, until the bank runs out of an input. The
 * product and the site are picked in the settings; the first is molten glass at the Edgeville
 * furnace.
 */
@PluginDescriptor(
	name = ProcessorPlugin.TAG + "Processor",
	description = "Banks, makes a product at a station (molten glass at a furnace), banks it",
	tags = {"crafting", "skilling", "furnace", "molten glass", "ironman"},
	authors = {"FelipeMalacarne"},
	version = ProcessorPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class ProcessorPlugin extends Plugin
{
	static final String VERSION = "0.1.0";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";

	@Inject
	private ProcessorConfig config;

	@Inject
	private ProcessorScript script;

	@Provides
	ProcessorConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(ProcessorConfig.class);
	}

	@Override
	protected void startUp()
	{
		log.info("Processor {} starting", VERSION);
		script.run(config);
	}

	@Override
	protected void shutDown()
	{
		script.shutdown();
	}
}
