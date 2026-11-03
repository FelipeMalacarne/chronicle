package net.runelite.client.plugins.microbot.fm.helloagent;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;

/**
 * Template plugin. It only observes (player position, nearby NPCs) and reports through the
 * channels an agent can read: Microbot.status, /scripts/results, /scripts/health, and the
 * test-mode result.json. Copy this package to start a new plugin.
 */
@PluginDescriptor(
	name = PluginDescriptor.Mocrosoft + "Hello Agent",
	description = "Harness template: observes game state and reports it to agents",
	tags = {"example", "agent"},
	authors = {"FelipeMalacarne"},
	version = HelloAgentPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class HelloAgentPlugin extends Plugin
{
	static final String VERSION = "1.0.0";

	@Inject
	private HelloAgentScript script;

	@Provides
	HelloAgentConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(HelloAgentConfig.class);
	}

	@Override
	protected void startUp()
	{
		log.info("Hello Agent {} starting", VERSION);
		script.run();
	}

	@Override
	protected void shutDown()
	{
		script.shutdown();
	}
}
