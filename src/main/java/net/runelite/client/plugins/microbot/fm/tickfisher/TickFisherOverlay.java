package net.runelite.client.plugins.microbot.fm.tickfisher;

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
class TickFisherOverlay extends OverlayPanel
{
	private static final String STATUS_PREFIX = "Tick Fisher: ";
	// XP/h recomputed this often: per frame it changes with every millisecond and flickers.
	private static final long RATE_REFRESH_MS = 1000;

	private final TickFisherPlugin plugin;
	private final TickFisherConfig config;
	private long rateAt;
	private long rate;
	private Session rateSession;

	@Inject
	TickFisherOverlay(TickFisherPlugin plugin, TickFisherConfig config)
	{
		super(plugin);
		this.plugin = plugin;
		this.config = config;
		setPosition(OverlayPosition.TOP_LEFT);
		panelComponent.setPreferredSize(new Dimension(210, 0));
	}

	@Override
	public Dimension render(Graphics2D graphics)
	{
		if (!config.showOverlay())
		{
			return null;
		}
		// The plugin's script: injecting TickFisherScript here would get a second, idle instance.
		TickFisherScript script = plugin.script();
		Session s = script.session();
		panelComponent.getChildren().add(TitleComponent.builder()
			.text("Tick Fisher " + TickFisherPlugin.VERSION)
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
		line("Place", config.place().toString());
		line("Mode", (config.threeTick() ? "3-tick" : "Plain") + ", " + config.fishHandling() + ", " + config.mouse());
		line("Runtime", duration(elapsed));
		int stopAt = config.stopAtLevel();
		line("Fishing", script.events().fishingLevel() + (stopAt > 0 ? " (stop at " + stopAt + ")" : ""));
		int agilityAt = config.stopAtAgilityLevel();
		if (config.place() == FishingPlace.OTTO || agilityAt > 0)
		{
			line("Agility", script.events().agilityLevel() + (agilityAt > 0 ? " (stop at " + agilityAt + ")" : ""));
		}
		long now = System.currentTimeMillis();
		if (s != rateSession || now - rateAt >= RATE_REFRESH_MS)
		{
			rateSession = s;
			rateAt = now;
			rate = xp * 3_600_000L / elapsed;
		}
		line("XP gained", String.format("%,d (%,d/h)", xp, rate));
		int ticks = s.fishingTicks;
		line("Catches", s.catches + (ticks > 0 ? String.format(" (%.1f/100 ticks)", 100.0 * s.catches / ticks) : ""));
		if (config.threeTick())
		{
			line("Cycles", s.cycles + " (" + s.onTimePercent() + "% on time)");
			if (s.spotNextTick > 0)
			{
				line("Spot click a tick late", String.valueOf(s.spotNextTick));
			}
			line("Rolls in step", s.inStepPercent() + "%");
			if (config.varyRhythm())
			{
				line("Hesitations / breathers", s.slips + " / " + s.breathers);
				line("Re-anchored", String.valueOf(s.realigns));
			}
			if (s.stocksMade > 0)
			{
				line("Teak stocks made", String.valueOf(s.stocksMade));
			}
		}
		if (config.fishHandling() == FishHandling.CUT)
		{
			line("Cut / roe eaten", s.fishCut + " / " + s.roeEaten);
		}
		if (config.fishHandling() == FishHandling.BANK)
		{
			line("Bank trips", String.valueOf(s.bankTrips));
		}
		line("Spot moves", s.spotMoves + (s.spotWaitTicks > 0 ? String.format(" (waited %ds)", s.spotWaitTicks * 600 / 1000) : ""));
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
