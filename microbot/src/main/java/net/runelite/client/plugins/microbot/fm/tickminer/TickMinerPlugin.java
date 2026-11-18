package net.runelite.client.plugins.microbot.fm.tickminer;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.ChatMessageType;
import net.runelite.api.Skill;
import net.runelite.api.events.ChatMessage;
import net.runelite.api.events.GameTick;
import net.runelite.api.events.StatChanged;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.ui.overlay.OverlayManager;

/**
 * 3-tick sandstone mining at the Desert Quarry (3t4s): knife on a teak log (or a herb on swamp tar)
 * and a rock every 3 ticks, stepping between the four rocks of one spot. The sandstone goes into
 * Drew's Sandstorm grinder next to it, for buckets of sand claimed from Drew later.
 */
@PluginDescriptor(
	name = TickMinerPlugin.TAG + "Tick Miner",
	description = "3-tick sandstone mining at the Desert Quarry (3t4s), the sandstone deposited in Drew's grinder for buckets of sand",
	tags = {"mining", "3-tick", "tick manipulation", "sandstone", "quarry", "3t4s", "sand", "ironman"},
	authors = {"FelipeMalacarne"},
	version = TickMinerPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class TickMinerPlugin extends Plugin
{
	static final String VERSION = "0.3.2";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";

	@Inject
	private TickMinerConfig config;

	@Inject
	private TickMinerScript script;

	@Inject
	private TickMinerOverlay overlay;

	@Inject
	private OverlayManager overlayManager;

	@Provides
	TickMinerConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(TickMinerConfig.class);
	}

	TickMinerScript script()
	{
		return script;
	}

	@Override
	protected void startUp()
	{
		log.info("Tick Miner {} starting", VERSION);
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
	public void onChatMessage(ChatMessage event)
	{
		if (event.getType() == ChatMessageType.GAMEMESSAGE || event.getType() == ChatMessageType.SPAM)
		{
			script.events().onGameMessage(event.getMessage());
		}
	}

	@Subscribe
	public void onStatChanged(StatChanged event)
	{
		if (event.getSkill() == Skill.MINING)
		{
			script.events().onMining(event.getLevel(), event.getXp());
		}
		else if (event.getSkill() == Skill.HITPOINTS)
		{
			script.events().onHitpoints(event.getBoostedLevel(), event.getLevel());
		}
	}
}
