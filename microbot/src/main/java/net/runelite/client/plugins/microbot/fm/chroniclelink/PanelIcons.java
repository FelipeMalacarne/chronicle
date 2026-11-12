package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Component;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.geom.Path2D;
import javax.swing.Icon;
import net.runelite.client.ui.ColorScheme;

/**
 * The panel's status icons, drawn in code (no resource files, and no reliance on which glyphs
 * a font has). Each verdict, item and step kind has its own shape, never only a colour.
 */
final class PanelIcons
{
	static final Color MET = ColorScheme.PROGRESS_COMPLETE_COLOR;
	static final Color UNMET = ColorScheme.PROGRESS_ERROR_COLOR;
	static final Color BLOCKED = ColorScheme.PROGRESS_INPROGRESS_COLOR;
	static final Color MUTED = ColorScheme.LIGHT_GRAY_COLOR;

	static final int SIZE = 12;

	enum Shape
	{
		/** Green check: met objective, carried item. */
		CHECK,
		/** Red cross: unmet objective, missing item. */
		CROSS,
		/** Grey circle with a question mark: no data. */
		QUESTION,
		/** Hollow grey circle: pending step, item in the bank. */
		RING,
		/** Filled green disc with a check: a done step. */
		DONE,
		/** Dashed green ring with a small check: a presumed (implied) step. */
		IMPLIED,
		/** Two grey chevrons: a skipped step. */
		SKIP,
		/** Amber padlock: blocked. */
		LOCK,
		/** Small grey speech mark: the step has a tip. */
		NOTE,
		/** Orange exclamation badge: the step has an update note. */
		UPDATE,
		CHEVRON_RIGHT,
		CHEVRON_DOWN
	}

	private PanelIcons()
	{
	}

	static Icon of(Shape shape)
	{
		return new Drawn(shape, null, SIZE);
	}

	/** A filled dot, for the connection status. */
	static Icon dot(Color color)
	{
		return new Drawn(null, color, 8);
	}

	static Icon step(PanelText.StepKind kind)
	{
		switch (kind)
		{
			case DONE:
				return of(Shape.DONE);
			case IMPLIED:
				return of(Shape.IMPLIED);
			case SKIPPED:
				return of(Shape.SKIP);
			case BLOCKED:
				return of(Shape.LOCK);
			case PENDING:
			default:
				return of(Shape.RING);
		}
	}

	static Icon verdict(String verdict)
	{
		if ("met".equals(verdict))
		{
			return of(Shape.CHECK);
		}
		if ("unmet".equals(verdict))
		{
			return of(Shape.CROSS);
		}
		return of(Shape.QUESTION);
	}

	static Color verdictColor(String verdict)
	{
		return "met".equals(verdict) ? MET : "unmet".equals(verdict) ? UNMET : MUTED;
	}

	static Icon item(PanelText.ItemKind kind)
	{
		switch (kind)
		{
			case CARRIED:
				return of(Shape.CHECK);
			case MISSING:
				return of(Shape.CROSS);
			case BANK:
				return of(Shape.RING);
			case UNKNOWN:
			default:
				return of(Shape.QUESTION);
		}
	}

	static Color itemColor(PanelText.ItemKind kind)
	{
		switch (kind)
		{
			case CARRIED:
				return MET;
			case MISSING:
				return UNMET;
			default:
				return MUTED;
		}
	}

	private static final class Drawn implements Icon
	{
		private final Shape shape;
		private final Color dot;
		private final int size;

		Drawn(Shape shape, Color dot, int size)
		{
			this.shape = shape;
			this.dot = dot;
			this.size = size;
		}

		@Override
		public int getIconWidth()
		{
			return size;
		}

		@Override
		public int getIconHeight()
		{
			return size;
		}

		@Override
		public void paintIcon(Component c, Graphics graphics, int x, int y)
		{
			Graphics2D g = (Graphics2D) graphics.create();
			g.translate(x, y);
			g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
			g.setRenderingHint(RenderingHints.KEY_STROKE_CONTROL, RenderingHints.VALUE_STROKE_PURE);
			if (shape == null)
			{
				g.setColor(dot);
				g.fillOval(0, 0, size, size);
				g.dispose();
				return;
			}
			BasicStroke thin = new BasicStroke(1.4f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND);
			BasicStroke bold = new BasicStroke(2f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND);
			switch (shape)
			{
				case CHECK:
					g.setColor(MET);
					g.setStroke(bold);
					g.draw(check(1.5, 6.5, 4.5, 9.5, 10.5, 2.5));
					break;
				case CROSS:
					g.setColor(UNMET);
					g.setStroke(bold);
					g.drawLine(2, 2, 10, 10);
					g.drawLine(10, 2, 2, 10);
					break;
				case QUESTION:
					g.setColor(MUTED);
					g.setStroke(thin);
					g.drawOval(1, 1, 10, 10);
					g.setStroke(new BasicStroke(1.3f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND));
					Path2D q = new Path2D.Double();
					q.moveTo(4.6, 4.8);
					q.curveTo(4.6, 2.9, 7.6, 2.9, 7.6, 4.7);
					q.curveTo(7.6, 5.9, 6.1, 6.0, 6.1, 7.3);
					g.draw(q);
					g.fillRect(5, 8, 2, 2);
					break;
				case RING:
					g.setColor(MUTED);
					g.setStroke(thin);
					g.drawOval(1, 1, 10, 10);
					break;
				case DONE:
					g.setColor(MET);
					g.fillOval(0, 0, 12, 12);
					g.setColor(ColorScheme.DARKER_GRAY_COLOR);
					g.setStroke(new BasicStroke(1.8f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND));
					g.draw(check(3, 6.2, 5.2, 8.5, 9.2, 3.6));
					break;
				case IMPLIED:
					g.setColor(MET);
					g.setStroke(new BasicStroke(1.3f, BasicStroke.CAP_BUTT, BasicStroke.JOIN_ROUND, 1f, new float[]{2.2f, 1.8f}, 0f));
					g.drawOval(1, 1, 10, 10);
					g.setStroke(new BasicStroke(1.2f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND));
					g.draw(check(4, 6.2, 5.6, 7.8, 8.2, 4.4));
					break;
				case SKIP:
					g.setColor(MUTED);
					g.setStroke(thin);
					g.drawPolyline(new int[]{2, 5, 2}, new int[]{3, 6, 9}, 3);
					g.drawPolyline(new int[]{6, 9, 6}, new int[]{3, 6, 9}, 3);
					break;
				case LOCK:
					g.setColor(BLOCKED);
					g.setStroke(new BasicStroke(1.6f));
					g.drawArc(3, 1, 6, 8, 0, 180);
					g.drawLine(3, 5, 3, 6);
					g.drawLine(9, 5, 9, 6);
					g.fillRoundRect(1, 5, 10, 7, 2, 2);
					g.setColor(ColorScheme.DARKER_GRAY_COLOR);
					g.fillRect(5, 7, 2, 3);
					break;
				case NOTE:
					g.setColor(MUTED);
					g.setStroke(thin);
					g.drawRoundRect(1, 2, 10, 7, 3, 3);
					g.fillPolygon(new int[]{3, 6, 3}, new int[]{9, 9, 12}, 3);
					break;
				case UPDATE:
					g.setColor(ColorScheme.BRAND_ORANGE);
					g.fillOval(0, 0, 12, 12);
					g.setColor(ColorScheme.DARKER_GRAY_COLOR);
					g.fillRect(5, 2, 2, 5);
					g.fillRect(5, 8, 2, 2);
					break;
				case CHEVRON_RIGHT:
					g.setColor(MUTED);
					g.setStroke(bold);
					g.drawPolyline(new int[]{4, 8, 4}, new int[]{2, 6, 10}, 3);
					break;
				case CHEVRON_DOWN:
				default:
					g.setColor(MUTED);
					g.setStroke(bold);
					g.drawPolyline(new int[]{2, 6, 10}, new int[]{4, 8, 4}, 3);
					break;
			}
			g.dispose();
		}

		private static Path2D check(double x1, double y1, double x2, double y2, double x3, double y3)
		{
			Path2D p = new Path2D.Double();
			p.moveTo(x1, y1);
			p.lineTo(x2, y2);
			p.lineTo(x3, y3);
			return p;
		}
	}
}
