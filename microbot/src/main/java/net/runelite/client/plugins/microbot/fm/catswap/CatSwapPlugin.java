package net.runelite.client.plugins.microbot.fm.catswap;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;

/**
 * One cat swap, then it switches itself off: picks up the grown cat, sells it to a Civilian in
 * West Ardougne for death runes, buys a new kitten from Gertrude west of Varrock and lets it
 * follow. Needs Biohazard (the Ardougne wall door), Gertrude's Cat and 100 coins in the
 * inventory. Chronicle (docs/08, phase 4) starts it between its main activity
 * and the return; the outcome is the last entry of its ScriptResultStore results.
 */
@PluginDescriptor(
	name = CatSwapPlugin.TAG + "Cat Swap",
	description = "Sells the grown cat to a West Ardougne Civilian for death runes and buys a new kitten from Gertrude",
	tags = {"cat", "kitten", "death runes", "gertrude", "ironman"},
	authors = {"FelipeMalacarne"},
	version = CatSwapPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class CatSwapPlugin extends Plugin
{
	static final String VERSION = "1.0.0";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";

	@Inject
	private CatSwapConfig config;

	@Inject
	private CatSwapScript script;

	@Provides
	CatSwapConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(CatSwapConfig.class);
	}

	@Override
	protected void startUp()
	{
		log.info("Cat Swap {} starting", VERSION);
		script.run(config);
	}

	@Override
	protected void shutDown()
	{
		script.shutdown();
	}
}
