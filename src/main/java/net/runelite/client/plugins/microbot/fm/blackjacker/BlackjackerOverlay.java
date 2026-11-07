package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.Graphics2D;
import javax.inject.Inject;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.ui.overlay.OverlayPanel;
import net.runelite.client.ui.overlay.OverlayPosition;
import net.runelite.client.ui.overlay.components.LineComponent;
import net.runelite.client.ui.overlay.components.TitleComponent;

/** Progress panel. Renders what the script already counted; no game calls on the client thread. */
class BlackjackerOverlay extends OverlayPanel
{
	private static final String STATUS_PREFIX = "Blackjacker: ";

	private final BlackjackerPlugin plugin;
	private final BlackjackerConfig config;

	@Inject
	BlackjackerOverlay(BlackjackerPlugin plugin, BlackjackerConfig config)
	{
		super(plugin);
		this.plugin = plugin;
		this.config = config;
		setPosition(OverlayPosition.TOP_LEFT);
		// Wide enough for "HP 30/34 (drink at 25)" on one line.
		panelComponent.setPreferredSize(new Dimension(210, 0));
	}

	@Override
	public Dimension render(Graphics2D graphics)
	{
		if (!config.showOverlay())
		{
			return null;
		}
		// The plugin's script: injecting BlackjackerScript here would get a second, idle instance.
		BlackjackerScript script = plugin.script();
		Session s = script.session();
		panelComponent.getChildren().add(TitleComponent.builder()
			.text("Blackjacker " + BlackjackerPlugin.VERSION)
			.color(s == null ? Color.LIGHT_GRAY : s.stopReason != null ? Color.ORANGE : Color.GREEN)
			.build());
		String status = Microbot.status;
		if (status != null && status.startsWith(STATUS_PREFIX))
		{
			panelComponent.getChildren().add(LineComponent.builder().left(status.substring(STATUS_PREFIX.length())).build());
		}
		if (s == null)
		{
			return super.render(graphics);
		}

		long elapsed = Math.max(1, System.currentTimeMillis() - s.startedAt);
		int xp = script.xpGained();
		int attempts = s.knockouts + s.knockoutFails;
		line("Target", s.target.toString());
		line("Runtime", duration(elapsed));
		int stopAt = config.stopAtLevel();
		line("Thieving", s.events.thievingLevel() + (stopAt > 0 ? " (stop at " + stopAt + ")" : ""));
		line("XP gained", String.format("%,d (%,d/h)", xp, xp * 3_600_000L / elapsed));
		line("Knock-outs", s.knockouts + (attempts > 0 ? " (" + (100 * s.knockoutFails / attempts) + "% glanced)" : ""));
		line("Picks", String.valueOf(s.picks));
		line("Wine", s.wines + " jugs (" + s.winesBought + " bought)");
		line("Climbs", String.valueOf(s.combatResets));
		line("HP", s.hp() + "/" + s.events.maxHp() + " (drink at " + s.drinkAt() + ")");
		if (s.stopReason != null)
		{
			line("Stopping", s.stopReason);
		}
		return super.render(graphics);
	}

	private void line(String left, String right)
	{
		panelComponent.getChildren().add(LineComponent.builder().left(left).right(right).build());
	}

	private static String duration(long ms)
	{
		long seconds = ms / 1000;
		return String.format("%d:%02d:%02d", seconds / 3600, seconds / 60 % 60, seconds % 60);
	}
}
