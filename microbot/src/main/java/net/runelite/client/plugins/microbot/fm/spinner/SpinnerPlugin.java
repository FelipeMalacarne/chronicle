package net.runelite.client.plugins.microbot.fm.spinner;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;

/**
 * Ironman-friendly Crafting: shears the sheep north of Lumbridge, spins the wool on the
 * castle's first-floor wheel and banks the balls of wool on the top floor.
 * Needs shears in the inventory or the bank.
 */
@PluginDescriptor(
	name = SpinnerPlugin.TAG + "Lumbridge Spinner",
	description = "Shears Lumbridge sheep, spins balls of wool in the castle, banks them",
	tags = {"crafting", "wool", "spinning", "ironman", "lumbridge"},
	authors = {"FelipeMalacarne"},
	version = SpinnerPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class SpinnerPlugin extends Plugin
{
	static final String VERSION = "1.0.2";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";

	@Inject
	private SpinnerConfig config;

	@Inject
	private SpinnerScript script;

	@Provides
	SpinnerConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(SpinnerConfig.class);
	}

	@Override
	protected void startUp()
	{
		log.info("Lumbridge Spinner {} starting", VERSION);
		script.run(config);
	}

	@Override
	protected void shutDown()
	{
		script.shutdown();
	}
}
