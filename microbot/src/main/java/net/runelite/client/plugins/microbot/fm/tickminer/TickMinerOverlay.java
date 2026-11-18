package net.runelite.client.plugins.microbot.fm.tickminer;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.Graphics2D;
import javax.inject.Inject;
import net.runelite.api.Experience;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.ui.overlay.OverlayPanel;
import net.runelite.client.ui.overlay.OverlayPosition;
import net.runelite.client.ui.overlay.components.LineComponent;
import net.runelite.client.ui.overlay.components.TitleComponent;

/** Progress panel. Renders what the script already counted; no game calls on the client thread. */
class TickMinerOverlay extends OverlayPanel
{
	private static final String STATUS_PREFIX = "Tick Miner: ";
	// Rates recomputed this often: per frame they change with every millisecond and flicker.
	private static final long RATE_REFRESH_MS = 1000;

	private final TickMinerPlugin plugin;
	private final TickMinerConfig config;
	private long rateAt;
	private long rate;
	private String eta;
	private Session rateSession;

	@Inject
	TickMinerOverlay(TickMinerPlugin plugin, TickMinerConfig config)
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
		// The plugin's script: injecting TickMinerScript here would get a second, idle instance.
		TickMinerScript script = plugin.script();
		Session s = script.session();
		panelComponent.getChildren().add(TitleComponent.builder()
			.text("Tick Miner " + TickMinerPlugin.VERSION)
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
		MiningEvents events = script.events();
		line("Mode", (config.threeTick() && s.delayLost == null ? "3-tick, " + config.delay() : "Plain") + ", " + config.mouse());
		line("Runtime", duration(elapsed));
		long now = System.currentTimeMillis();
		boolean refresh = s != rateSession || now - rateAt >= RATE_REFRESH_MS;
		if (refresh)
		{
			rateSession = s;
			rateAt = now;
			rate = xp * 3_600_000L / elapsed;
		}
		int level = events.miningLevel();
		int goal = config.stopAtLevel();
		if (goal > level && events.miningXp() > 0)
		{
			line("Mining", level + " -> " + goal);
			if (refresh || eta == null)
			{
				int left = Math.max(0, Experience.getXpForLevel(goal) - events.miningXp());
				eta = String.format("%s, %,d xp", xp > 0 ? duration((long) ((double) left * elapsed / xp)) : "...", left);
			}
			line("  to " + goal, eta);
		}
		else
		{
			line("Mining", String.valueOf(level));
		}
		line("XP gained", String.format("%,d (%,d/h)", xp, rate));
		int ticks = s.miningTicks;
		line("Sandstone", s.mined + (ticks > 0 ? String.format(" (%.1f/100 ticks)", 100.0 * s.mined / ticks) : ""));
		line("Deposited", s.depositedCount() + " (" + s.deposits + " trips)");
		line("Sand stored", s.sandStored + " buckets");
		if (config.threeTick())
		{
			line("Cycles", s.cycles + " (" + s.onTimePercent() + "% on time)");
			if (s.cycles > 0)
			{
				line("Stepped to the rock", 100 * s.steps / s.cycles + "%");
			}
			if (s.rockNextTick > 0)
			{
				line("Rock click a tick late", String.valueOf(s.rockNextTick));
			}
			line("Rolls in step", s.inStepPercent() + "%");
			if (config.varyRhythm())
			{
				line("Hesitations / breathers", s.slips + " / " + s.breathers);
				line("Re-anchored", String.valueOf(s.realigns));
			}
			if (s.delayLost != null)
			{
				line("Plain since", s.delayLost);
			}
		}
		if (s.rockWaitTicks > 0)
		{
			line("Waited for rocks", String.format("%ds", s.rockWaitTicks * 600 / 1000));
		}
		line("HP / water", events.hitpoints() + "/" + events.maxHitpoints() + " / " + s.waterSips + " sips");
		if (s.refillTrips > 0)
		{
			line("Cactus trips", s.refillTrips + " (+" + s.sipsRefilled + " sips, " + s.cactusFails + " failed cuts)");
		}
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
