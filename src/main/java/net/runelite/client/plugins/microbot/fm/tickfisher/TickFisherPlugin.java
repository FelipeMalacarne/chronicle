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
 * 3-tick rod fishing: knife on a teak log (or eating roe) and the spot every 3 ticks. Fly fishing at
 * Shilo Village, dropping a fish between rolls or banking them, or barbarian fishing at Otto's
 * Grotto, dropping or cutting them for Cooking XP. Grows into Barblore.
 */
@PluginDescriptor(
	name = PluginDescriptor.Mocrosoft + "Tick Fisher",
	description = "3-tick fly fishing at Shilo (drop or bank) and barbarian fishing at Otto's Grotto (drop or cut-eat)",
	tags = {"fishing", "3-tick", "tick manipulation", "fly fishing", "barbarian", "shilo", "otto", "ironman"},
	authors = {"FelipeMalacarne"},
	version = TickFisherPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class TickFisherPlugin extends Plugin
{
	static final String VERSION = "0.5.0";

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
		else if (event.getSkill() == Skill.AGILITY)
		{
			script.events().onAgility(event.getLevel());
		}
	}

	@Subscribe
	public void onNpcDespawned(NpcDespawned event)
	{
		if (config.place().spotName.equals(event.getNpc().getName()))
		{
			script.events().onNpcDespawned(event.getNpc().getIndex());
		}
	}
}
