package net.runelite.client.plugins.microbot.fm.devbootstrap;

import com.google.gson.Gson;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.events.ExternalPluginsChanged;
import net.runelite.client.events.PluginChanged;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.agentserver.AgentServerConfig;
import net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin;

/**
 * Turns the isolated dev client into an agent-drivable one: starts the Agent Server on a
 * Unix socket inside the dev home, and keeps .runelite/agent/config-schema.json with the config
 * options of every loaded Microbot plugin (the planner's agent builds its config form from it).
 * Only installed by bin/mb-client, and inert unless the JVM was launched with
 * -Dmicrobot.dev.harness=true, so it never touches the everyday client.
 */
@PluginDescriptor(
	name = DevBootstrapPlugin.TAG + "Dev Bootstrap",
	description = "Starts the Agent Server for the agent dev harness",
	tags = {"dev", "agent", "harness"},
	authors = {"FelipeMalacarne"},
	version = DevBootstrapPlugin.VERSION,
	enabledByDefault = true,
	isExternal = true
)
@Slf4j
public class DevBootstrapPlugin extends Plugin
{
	static final String VERSION = "1.1.0";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";
	static final String HARNESS_PROPERTY = "microbot.dev.harness";
	// Used only if the socket cannot bind and the server falls back to TCP; keeps it off 8081.
	static final int FALLBACK_PORT = 8091;

	@Inject
	private ConfigManager configManager;

	@Inject
	private Gson gson;

	private ConfigSchemaExporter exporter;

	@Override
	protected void startUp()
	{
		if (!Boolean.getBoolean(HARNESS_PROPERTY))
		{
			log.warn("Dev Bootstrap loaded without -D{}=true; doing nothing", HARNESS_PROPERTY);
			return;
		}

		configManager.setConfiguration(AgentServerConfig.GROUP, "bindMode", AgentServerConfig.BindMode.UDS);
		configManager.setConfiguration(AgentServerConfig.GROUP, AgentServerConfig.KEY_PORT, FALLBACK_PORT);
		configManager.setConfiguration(AgentServerConfig.GROUP, "bindOnlyWhileScriptsActive", false);

		AgentServerPlugin agentServer = Microbot.getPlugin(AgentServerPlugin.class);
		if (agentServer == null)
		{
			log.error("Agent Server plugin is not registered in this client");
			return;
		}
		if (!Microbot.getPluginManager().isPluginActive(agentServer))
		{
			// Defers to the EDT, so it runs after the current startPlugins pass.
			Microbot.startPlugin(agentServer);
		}
		log.info("Dev Bootstrap: Agent Server requested over UDS");

		exporter = new ConfigSchemaExporter(Microbot.getPluginManager(), configManager, gson);
		exporter.export();
	}

	@Override
	protected void shutDown()
	{
		exporter = null;
	}

	/** A plugin loaded, unloaded, installed or removed: its options come or go. */
	@Subscribe
	public void onPluginChanged(PluginChanged event)
	{
		if (exporter != null)
		{
			exporter.export();
		}
	}

	@Subscribe
	public void onExternalPluginsChanged(ExternalPluginsChanged event)
	{
		if (exporter != null)
		{
			exporter.export();
		}
	}
}
