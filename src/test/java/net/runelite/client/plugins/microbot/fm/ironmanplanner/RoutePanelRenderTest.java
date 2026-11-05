package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertTrue;

import java.awt.Component;
import java.awt.Container;
import java.awt.Graphics2D;
import java.awt.image.BufferedImage;
import java.io.File;
import java.time.Instant;
import java.util.function.Consumer;
import javax.imageio.ImageIO;
import javax.swing.JScrollPane;
import javax.swing.SwingUtilities;
import net.runelite.client.ui.laf.RuneLiteLAF;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;

/**
 * Paints the route panel headlessly into PNGs, in RuneLite's look and feel, to review the
 * layout without a client. Runs only with IRONMAN_PANEL_RENDER set to the output file, e.g.
 * {@code IRONMAN_PANEL_RENDER=/tmp/painel.png gradle test --tests '*RoutePanelRenderTest*'};
 * the variants land next to it ("-expandido", "-real").
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
		Instant now = Instant.now();
		PanelState online = new PanelState(PanelState.Connection.ONLINE, PanelScreenshotFixture.window(),
			now.minusSeconds(10), null, "1.1.242");
		write(out, "", online, false, null);
		PanelState stale = new PanelState(PanelState.Connection.STALE, PanelScreenshotFixture.window(),
			now.minusSeconds(130), "servidor fora do ar", "1.1.242");
		write(out, "-expandido", stale, true, panel -> click(panel, "<html>1.1.250 ", "<html>1.1.247 ", "Clique para ler"));
		PanelState real = new PanelState(PanelState.Connection.ONLINE, PanelScreenshotFixture.real(), now.minusSeconds(4), null, null);
		write(out, "-real", real, false, panel -> click(panel, "<html>1.1.245 "));
	}

	private static void write(String out, String suffix, PanelState state, boolean itemsOpen, Consumer<RoutePanel> after) throws Exception
	{
		BufferedImage[] img = new BufferedImage[1];
		SwingUtilities.invokeAndWait(() ->
		{
			RuneLiteLAF.setup();
			RoutePanel panel = new RoutePanel(new NoActions(), itemsOpen);
			panel.update(state);
			// a first layout gives the content its width; the second render wraps at it
			panel.setSize(WIDTH, 3000);
			layout(panel);
			panel.update(state);
			if (after != null)
			{
				after.accept(panel);
			}
			img[0] = paint(panel);
		});
		File file = new File(suffix.isEmpty() ? out : out.replaceFirst("(\\.png)?$", suffix + ".png"));
		ImageIO.write(img[0], "png", file);
		assertTrue(file.length() > 0);
	}

	/** Clicks the clickable components whose tooltip starts with each key (rows, the section). */
	private static void click(RoutePanel panel, String... refs)
	{
		for (String ref : refs)
		{
			panel.setSize(WIDTH, 3000);
			layout(panel);
			Component row = rowOf(panel, ref);
			if (row != null)
			{
				for (java.awt.event.MouseListener l : row.getMouseListeners())
				{
					l.mouseClicked(new java.awt.event.MouseEvent(row, java.awt.event.MouseEvent.MOUSE_CLICKED, 0, 0, 1, 1, 1, false));
				}
			}
		}
	}

	private static Component rowOf(Container c, String ref)
	{
		for (Component child : c.getComponents())
		{
			if (child instanceof javax.swing.JComponent && ((javax.swing.JComponent) child).getMouseListeners().length > 0)
			{
				String tip = ((javax.swing.JComponent) child).getToolTipText();
				if (tip != null && tip.startsWith(ref))
				{
					return child;
				}
			}
			if (child instanceof Container)
			{
				Component r = rowOf((Container) child, ref);
				if (r != null)
				{
					return r;
				}
			}
		}
		return null;
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
