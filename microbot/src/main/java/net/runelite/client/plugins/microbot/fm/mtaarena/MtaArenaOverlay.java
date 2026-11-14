package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.Graphics2D;
import javax.inject.Inject;
import net.runelite.client.ui.overlay.OverlayPanel;
import net.runelite.client.ui.overlay.OverlayPosition;
import net.runelite.client.ui.overlay.components.LineComponent;
import net.runelite.client.ui.overlay.components.TitleComponent;

/** The calculator's answer: points still missing per room and the runes that takes. */
class MtaArenaOverlay extends OverlayPanel
{
	private static final Color SHORT = new Color(255, 120, 120);
	private static final Color OK = new Color(140, 230, 140);

	private final MtaArenaPlugin plugin;
	private final MtaArenaConfig config;

	@Inject
	MtaArenaOverlay(MtaArenaPlugin plugin, MtaArenaConfig config)
	{
		this.plugin = plugin;
		this.config = config;
		setPosition(OverlayPosition.TOP_LEFT);
	}

	@Override
	public Dimension render(Graphics2D graphics)
	{
		if (!config.overlay())
		{
			return null;
		}
		MtaArenaPlugin.Snapshot s = plugin.snapshot();
		panelComponent.getChildren().add(TitleComponent.builder().text("MTA Planner").build());
		if (s.message != null)
		{
			panelComponent.getChildren().add(LineComponent.builder().left(s.message).build());
			return super.render(graphics);
		}
		if (s.result == null)
		{
			return super.render(graphics);
		}
		if (!s.pointsKnown)
		{
			panelComponent.getChildren().add(LineComponent.builder()
				.left("Points not read: talk to your progress hat").leftColor(SHORT).build());
		}
		MtaCalculator.Result r = s.result;
		if (r.buy.isEmpty())
		{
			panelComponent.getChildren().add(LineComponent.builder().left("Everything chosen is owned").leftColor(OK).build());
			return super.render(graphics);
		}
		line("Telekinetic", r.missing.telekinetic);
		line("Graveyard", r.missing.graveyard);
		line("Enchantment", r.missing.enchantment);
		line("Alchemist", r.missing.alchemist);
		rune("Law", r.runes.law, s.law);
		rune("Nature", r.runes.nature, s.nature);
		rune("Cosmic" + (r.enchantLevel > 0 ? " (Lvl-" + r.enchantLevel + ")" : ""), r.runes.cosmic, s.cosmic);
		return super.render(graphics);
	}

	private void line(String room, int missing)
	{
		panelComponent.getChildren().add(LineComponent.builder()
			.left(room).right(missing == 0 ? "done" : missing + " pts")
			.rightColor(missing == 0 ? OK : Color.WHITE).build());
	}

	private void rune(String name, int need, int have)
	{
		panelComponent.getChildren().add(LineComponent.builder()
			.left(name).right(have + " / " + need)
			.rightColor(have >= need ? OK : SHORT).build());
	}
}
