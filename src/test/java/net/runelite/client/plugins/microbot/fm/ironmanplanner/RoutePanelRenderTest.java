package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertTrue;

import java.awt.Component;
import java.awt.Container;
import java.awt.Graphics2D;
import java.awt.image.BufferedImage;
import java.io.File;
import java.time.Instant;
import javax.imageio.ImageIO;
import javax.swing.JScrollPane;
import javax.swing.SwingUtilities;
import net.runelite.client.ui.laf.RuneLiteLAF;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;

/**
 * Paints the route panel headlessly into a PNG, in RuneLite's look and feel, to review the
 * layout without a client. Runs only with IRONMAN_PANEL_RENDER set to the output file, e.g.
 * {@code IRONMAN_PANEL_RENDER=/tmp/painel.png gradle test --tests '*RoutePanelRenderTest*'}.
 */
class RoutePanelRenderTest
{
	static final int WIDTH = 225;

	@Test
	void rendersTheScreenshotWindow() throws Exception
	{
		String out = System.getenv("IRONMAN_PANEL_RENDER");
		Assumptions.assumeTrue(out != null && !out.isEmpty(), "IRONMAN_PANEL_RENDER not set");
		System.setProperty("java.awt.headless", "true");
		BufferedImage[] img = new BufferedImage[1];
		SwingUtilities.invokeAndWait(() ->
		{
			RuneLiteLAF.setup();
			RoutePanel panel = new RoutePanel(new NoActions());
			panel.update(new PanelState(PanelState.Connection.ONLINE, PanelScreenshotFixture.window(),
				Instant.now().minusSeconds(10), null, "1.1.242"));
			img[0] = paint(panel);
		});
		File file = new File(out);
		ImageIO.write(img[0], "png", file);
		assertTrue(file.length() > 0);
	}

	/** Lays the panel out at WIDTH and as tall as its content, then paints it. */
	static BufferedImage paint(Component panel)
	{
		panel.setSize(WIDTH, 3000);
		layout(panel);
		int height = 3000;
		JScrollPane scroll = find((Container) panel, JScrollPane.class);
		if (scroll != null)
		{
			int extra = panel.getHeight() - scroll.getViewport().getHeight();
			height = scroll.getViewport().getView().getPreferredSize().height + extra;
		}
		panel.setSize(WIDTH, height);
		layout(panel);
		BufferedImage img = new BufferedImage(WIDTH, height, BufferedImage.TYPE_INT_RGB);
		Graphics2D g = img.createGraphics();
		panel.paint(g);
		g.dispose();
		return img;
	}

	private static void layout(Component c)
	{
		c.invalidate();
		if (c instanceof Container)
		{
			((Container) c).doLayout();
			for (Component child : ((Container) c).getComponents())
			{
				layout(child);
			}
		}
	}

	private static <T> T find(Container c, Class<T> type)
	{
		for (Component child : c.getComponents())
		{
			if (type.isInstance(child))
			{
				return type.cast(child);
			}
			if (child instanceof Container)
			{
				T t = find((Container) child, type);
				if (t != null)
				{
					return t;
				}
			}
		}
		return null;
	}

	static final class NoActions implements RoutePanel.Actions
	{
		@Override
		public void mark(String stepId, String kind)
		{
		}

		@Override
		public void undo()
		{
		}

		@Override
		public void refresh()
		{
		}

		@Override
		public void visible(boolean visible)
		{
		}
	}
}
