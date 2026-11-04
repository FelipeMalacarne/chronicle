package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.Graphics2D;
import java.time.Instant;
import javax.inject.Inject;
import net.runelite.client.ui.overlay.OverlayPanel;
import net.runelite.client.ui.overlay.OverlayPosition;
import net.runelite.client.ui.overlay.components.LineComponent;
import net.runelite.client.ui.overlay.components.TitleComponent;

/** Small sync indicator. Renders only the precomputed status; no work on the client thread. */
class PlannerStatusOverlay extends OverlayPanel
{
	private final IronmanPlannerPlugin plugin;
	private final IronmanPlannerConfig config;

	@Inject
	PlannerStatusOverlay(IronmanPlannerPlugin plugin, IronmanPlannerConfig config)
	{
		super(plugin);
		this.plugin = plugin;
		this.config = config;
		setPosition(OverlayPosition.TOP_LEFT);
	}

	@Override
	public Dimension render(Graphics2D graphics)
	{
		if (!config.showStatus())
		{
			return null;
		}
		SyncStatus status = plugin.getStatus();
		panelComponent.getChildren().add(TitleComponent.builder()
			.text("Ironman Planner")
			.color(color(status.kind))
			.build());
		panelComponent.getChildren().add(LineComponent.builder()
			.left(status.describe(Instant.now()))
			.build());
		return super.render(graphics);
	}

	private static Color color(SyncStatus.Kind kind)
	{
		switch (kind)
		{
			case SYNCED:
				return Color.GREEN;
			case CONNECTING:
			case WAITING_LOGIN:
				return Color.YELLOW;
			case NOT_CONFIGURED:
				return Color.LIGHT_GRAY;
			default:
				return Color.RED;
		}
	}
}
