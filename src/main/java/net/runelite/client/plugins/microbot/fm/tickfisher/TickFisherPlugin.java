package net.runelite.client.plugins.microbot.fm.tickfisher;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Skill;
import net.runelite.api.events.GameTick;
import net.runelite.api.events.NpcDespawned;
import net.runelite.api.events.StatChanged;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.ui.overlay.OverlayManager;

/**
 * 3-tick fly fishing at Shilo Village: knife on a teak log and the spot every 3 ticks, dropping
 * a fish between rolls or banking them at the Shilo bank. Takes rod, feathers, knife and one
 * teak log from the bank. The first mode of a fishing plugin that grows into barbarian fishing
 * and Barblore.
 */
@PluginDescriptor(
	name = PluginDescriptor.Mocrosoft + "Tick Fisher",
	description = "3-tick fly fishing at Shilo Village (knife + teak), drop or bank, natural or quick mouse",
	tags = {"fishing", "3-tick", "tick manipulation", "fly fishing", "shilo", "ironman"},
	authors = {"FelipeMalacarne"},
	version = TickFisherPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class TickFisherPlugin extends Plugin
{
	static final String VERSION = "0.3.3";

	@Inject
	private TickFisherConfig config;

	@Inject
	private TickFisherScript script;

	@Inject
	private TickFisherOverlay overlay;

	@Inject
	private OverlayManager overlayManager;

	@Provides
	TickFisherConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(TickFisherConfig.class);
	}

	TickFisherScript script()
	{
		return script;
	}

	@Override
	protected void startUp()
	{
		log.info("Tick Fisher {} starting", VERSION);
		script.events().reset();
		overlayManager.add(overlay);
		script.run(config);
	}

	@Override
	protected void shutDown()
	{
		overlayManager.remove(overlay);
		script.shutdown();
	}

	@Subscribe
	public void onGameTick(GameTick event)
	{
		script.events().onTick();
	}

	@Subscribe
	public void onStatChanged(StatChanged event)
	{
		if (event.getSkill() == Skill.FISHING)
		{
			script.events().onFishing(event.getLevel(), event.getXp());
		}
	}

	@Subscribe
	public void onNpcDespawned(NpcDespawned event)
	{
		if (TickFisherScript.SPOT_NAME.equals(event.getNpc().getName()))
		{
			script.events().onNpcDespawned(event.getNpc().getIndex());
		}
	}
}
