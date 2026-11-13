package net.runelite.client.plugins.microbot.fm.blackjacker;

import com.google.inject.Provides;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Skill;
import net.runelite.api.events.ChatMessage;
import net.runelite.api.events.HitsplatApplied;
import net.runelite.api.events.StatChanged;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.ui.overlay.OverlayManager;

/**
 * Pollnivneach blackjacking: picks the target for the Thieving level, lures one into a room
 * with a curtain, then knocks it out and pickpockets it twice per knock-out. A fight is ended
 * by climbing the room's ladder. Heals with jugs of wine and buys more from the Asp & Snake
 * barman with coins. Needs a blackjack, coins and partial completion of The Feud.
 */
@PluginDescriptor(
	name = BlackjackerPlugin.TAG + "Blackjacker",
	description = "Pollnivneach blackjacking: auto target, lure, knock-out + pickpocket, buys wine",
	tags = {"thieving", "blackjack", "pollnivneach", "bandit", "ironman"},
	authors = {"FelipeMalacarne"},
	version = BlackjackerPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class BlackjackerPlugin extends Plugin
{
	static final String VERSION = "1.17.0";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";

	@Inject
	private BlackjackerConfig config;

	@Inject
	private BlackjackerScript script;

	@Inject
	private BlackjackerOverlay overlay;

	@Inject
	private OverlayManager overlayManager;

	@Provides
	BlackjackerConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(BlackjackerConfig.class);
	}

	BlackjackerScript script()
	{
		return script;
	}

	@Override
	protected void startUp()
	{
		log.info("Blackjacker {} starting", VERSION);
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
	public void onChatMessage(ChatMessage event)
	{
		script.events().onChat(event.getType(), event.getMessage());
	}

	@Subscribe
	public void onHitsplatApplied(HitsplatApplied event)
	{
		if (event.getActor() == Microbot.getClient().getLocalPlayer())
		{
			script.events().onPlayerHit();
		}
	}

	@Subscribe
	public void onStatChanged(StatChanged event)
	{
		if (event.getSkill() == Skill.HITPOINTS)
		{
			script.events().onHitpoints(event.getBoostedLevel(), event.getLevel());
		}
		else if (event.getSkill() == Skill.THIEVING)
		{
			script.events().onThieving(event.getLevel(), event.getXp());
		}
	}
}
