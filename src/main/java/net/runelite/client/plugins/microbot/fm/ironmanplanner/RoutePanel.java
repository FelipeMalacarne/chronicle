package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.awt.BasicStroke;
import java.awt.BorderLayout;
import java.awt.Color;
import java.awt.Cursor;
import java.awt.Dimension;
import java.awt.FlowLayout;
import java.awt.Font;
import java.awt.Graphics2D;
import java.awt.GridLayout;
import java.awt.RenderingHints;
import java.awt.event.MouseAdapter;
import java.awt.event.MouseEvent;
import java.awt.image.BufferedImage;
import java.time.Instant;
import java.util.HashSet;
import java.util.Set;
import javax.swing.BorderFactory;
import javax.swing.BoxLayout;
import javax.swing.ImageIcon;
import javax.swing.JButton;
import javax.swing.JComponent;
import javax.swing.JLabel;
import javax.swing.JPanel;
import javax.swing.SwingUtilities;
import javax.swing.border.EmptyBorder;
import net.runelite.client.ui.ColorScheme;
import net.runelite.client.ui.FontManager;
import net.runelite.client.ui.PluginPanel;

/**
 * The route panel in RuneLite's own look: the active route from the cursor, the current step
 * with its objectives and Feito/Pular/Desfazer, the next steps (click to expand), and the items
 * to carry for the leg. Swing only: every method runs on the Swing thread, and actions go to
 * {@link Actions}, which hands them to the panel executor.
 */
final class RoutePanel extends PluginPanel
{
	/** What the buttons ask for; implementations must leave the Swing thread at once. */
	interface Actions
	{
		void mark(String stepId, String kind);

		void undo();

		void refresh();

		/** The panel became visible (true) or hidden (false). */
		void visible(boolean visible);
	}

	private static final Color MET = new Color(0x4caf50);
	private static final Color UNMET = new Color(0xe57373);
	private static final Color UNKNOWN = ColorScheme.LIGHT_GRAY_COLOR;
	private static final Color MUTED = new Color(0x9e9e9e);
	/** Text width inside the panel (borders, card edge and scrollbar off), for the HTML wrapping. */
	private static final int TEXT_WIDTH = PANEL_WIDTH - 52;

	private static final ImageIcon LOCK = new ImageIcon(lockImage());

	private final Actions actions;
	private final JPanel content = new JPanel();
	/** Steps of the list the owner expanded. */
	private final Set<String> expanded = new HashSet<>();
	private PanelState state = new PanelState(PanelState.Connection.NOT_CONFIGURED, null, null, null, null);
	private boolean busy;

	RoutePanel(Actions actions)
	{
		super(false);
		this.actions = actions;
		setLayout(new BorderLayout());
		setBackground(ColorScheme.DARK_GRAY_COLOR);
		setBorder(new EmptyBorder(8, 8, 8, 8));
		content.setLayout(new BoxLayout(content, BoxLayout.Y_AXIS));
		content.setBackground(ColorScheme.DARK_GRAY_COLOR);
		JPanel north = new WidthTrackingPanel();
		north.setBackground(ColorScheme.DARK_GRAY_COLOR);
		north.add(content, BorderLayout.NORTH);
		javax.swing.JScrollPane scroll = new javax.swing.JScrollPane(north);
		scroll.setBorder(null);
		scroll.getViewport().setBackground(ColorScheme.DARK_GRAY_COLOR);
		scroll.getVerticalScrollBar().setUnitIncrement(16);
		scroll.setHorizontalScrollBarPolicy(javax.swing.ScrollPaneConstants.HORIZONTAL_SCROLLBAR_NEVER);
		add(scroll, BorderLayout.CENTER);
		render();
	}

	@Override
	public void onActivate()
	{
		actions.visible(true);
	}

	@Override
	public void onDeactivate()
	{
		actions.visible(false);
	}

	/** New state from the controller (Swing thread). */
	void update(PanelState next)
	{
		state = next;
		busy = false;
		render();
	}

	/** Refreshes the "h\u00e1 N s" texts (Swing thread). */
	void tickClock()
	{
		render();
	}

	private void render()
	{
		content.removeAll();
		RouteWindow w = state.window;
		content.add(header(w));
		if (w != null)
		{
			RouteWindow.Step current = w.current();
			if (current == null)
			{
				content.add(note("Rota conclu\u00edda: nenhum passo pendente."));
			}
			else
			{
				content.add(currentCard(current));
			}
			JComponent carry = carry(w.carry);
			if (carry != null)
			{
				content.add(carry);
			}
			JComponent next = nextSteps(w);
			if (next != null)
			{
				content.add(next);
			}
		}
		content.add(footer());
		content.revalidate();
		content.repaint();
	}

	private JComponent header(RouteWindow w)
	{
		JPanel p = section();
		if (w != null)
		{
			JLabel title = wrappedLabel(w.title, Color.WHITE);
			title.setFont(FontManager.getRunescapeBoldFont());
			p.add(title);
			p.add(wrapped(w.attribution, MUTED));
			String counts = PanelText.counts(w.counts);
			if (!counts.isEmpty())
			{
				p.add(label(counts, FontManager.getRunescapeSmallFont(), ColorScheme.LIGHT_GRAY_COLOR));
			}
		}
		else
		{
			p.add(label("Ironman Planner", FontManager.getRunescapeBoldFont(), Color.WHITE));
		}
		Color color;
		switch (state.connection)
		{
			case ONLINE:
				color = MET;
				break;
			case STALE:
			case OFFLINE:
				color = UNMET;
				break;
			default:
				color = MUTED;
		}
		p.add(wrapped(PanelText.connection(state, Instant.now()), color));
		return p;
	}

	private JComponent currentCard(RouteWindow.Step s)
	{
		JPanel p = section();
		p.setBackground(ColorScheme.DARKER_GRAY_COLOR);
		p.setBorder(BorderFactory.createCompoundBorder(
			BorderFactory.createMatteBorder(0, 3, 0, 0, ColorScheme.BRAND_ORANGE),
			new EmptyBorder(6, 6, 6, 6)));
		String where = s.sectionTitle == null || s.sectionTitle.isEmpty() ? "" : " \u00b7 " + s.sectionTitle;
		p.add(label("AGORA \u00b7 " + s.ref + where, FontManager.getRunescapeSmallFont(), ColorScheme.BRAND_ORANGE));
		addStepBody(p, s, true);
		p.add(buttons(s, true));
		return p;
	}

	/** Text, location, blocked line and objective chips of a step. */
	private void addStepBody(JPanel p, RouteWindow.Step s, boolean full)
	{
		String status = PanelText.status(s.status);
		if (!status.isEmpty())
		{
			p.add(label(status, FontManager.getRunescapeSmallFont(), MUTED));
		}
		p.add(wrapped(full ? s.text : s.title, Color.WHITE));
		if (s.location != null && !s.location.isEmpty())
		{
			p.add(label("Local: " + s.location, FontManager.getRunescapeSmallFont(), ColorScheme.LIGHT_GRAY_COLOR));
		}
		if (s.blocked())
		{
			JLabel lock = label(PanelText.blocked(s.blockedOrEmpty()), FontManager.getRunescapeSmallFont(), UNMET);
			lock.setIcon(LOCK);
			lock.setToolTipText("Bloqueado: o cursor passa por este passo at\u00e9 o requisito ser cumprido");
			p.add(lock);
		}
		if (!s.objectivesOrEmpty().isEmpty())
		{
			JPanel chips = new JPanel(new WrapLayout());
			chips.setOpaque(false);
			chips.setAlignmentX(LEFT_ALIGNMENT);
			for (RouteWindow.Objective o : s.objectivesOrEmpty())
			{
				chips.add(chip(o));
			}
			p.add(chips);
		}
	}

	private JComponent chip(RouteWindow.Objective o)
	{
		JLabel chip = new JLabel(PanelText.chip(o));
		chip.setFont(new Font(Font.DIALOG, Font.PLAIN, 11));
		Color c = "met".equals(o.verdict) ? MET : "unmet".equals(o.verdict) ? UNMET : UNKNOWN;
		chip.setForeground(c);
		chip.setBorder(BorderFactory.createCompoundBorder(
			"unknown".equals(o.verdict) || o.verdict == null
				? BorderFactory.createDashedBorder(c, 1, 3, 2, true)
				: BorderFactory.createLineBorder(c),
			new EmptyBorder(1, 4, 1, 4)));
		chip.setToolTipText(PanelText.role(o.role) + " \u00b7 " + PanelText.verdictWord(o.verdict));
		return chip;
	}

	private JComponent buttons(RouteWindow.Step s, boolean withUndo)
	{
		JPanel row = new JPanel(new GridLayout(1, withUndo ? 3 : 2, 4, 0));
		row.setOpaque(false);
		row.setAlignmentX(LEFT_ALIGNMENT);
		row.setBorder(new EmptyBorder(6, 0, 0, 0));
		row.add(button("Feito", "Marca o passo como feito (manual_done)", () -> actions.mark(s.id, RoutePanelController.MANUAL_DONE)));
		row.add(button("Pular", "Pula o passo (skipped)", () -> actions.mark(s.id, RoutePanelController.SKIPPED)));
		if (withUndo)
		{
			JButton undo = button("Desfazer", state.undoRef == null ? "Nada para desfazer" : "Desfaz a \u00faltima marca (" + state.undoRef + ")", actions::undo);
			undo.setEnabled(!busy && state.undoRef != null);
			row.add(undo);
		}
		return row;
	}

	private JButton button(String text, String tooltip, Runnable run)
	{
		JButton b = new JButton(text);
		b.setToolTipText(tooltip);
		b.setFocusable(false);
		b.setFont(FontManager.getRunescapeSmallFont());
		b.setMargin(new java.awt.Insets(2, 1, 2, 1));
		b.setEnabled(!busy && state.window != null);
		b.addActionListener(e ->
		{
			busy = true;
			render();
			run.run();
		});
		return b;
	}

	private JComponent carry(RouteWindow.Carry c)
	{
		if (c == null || (c.itemsOrEmpty().isEmpty() && c.bankActionsOrEmpty().isEmpty()))
		{
			return null;
		}
		JPanel p = section();
		String title = c.derived ? "Itens dos pr\u00f3ximos passos" : "Itens do trecho";
		p.add(label(title, FontManager.getRunescapeBoldFont(), Color.WHITE));
		if (c.derived)
		{
			p.add(label("pr\u00e9via: o guia n\u00e3o lista o setup do trecho", FontManager.getRunescapeSmallFont(), MUTED));
		}
		for (RouteWindow.CarryItem it : c.itemsOrEmpty())
		{
			String text = PanelText.carryGlyph(it) + " " + PanelText.carryName(it) + " \u00b7 " + PanelText.carryWhere(it)
				+ (it.optional ? " (opcional)" : "");
			JLabel l = label(text, new Font(Font.DIALOG, Font.PLAIN, 11),
				"met".equals(it.verdict) ? MET : PanelText.GLYPH_UNMET.equals(PanelText.carryGlyph(it)) ? UNMET : ColorScheme.LIGHT_GRAY_COLOR);
			p.add(l);
		}
		for (String action : c.bankActionsOrEmpty())
		{
			p.add(wrapped("Banco: " + action, ColorScheme.LIGHT_GRAY_COLOR));
		}
		return p;
	}

	private JComponent nextSteps(RouteWindow w)
	{
		JPanel p = section();
		int n = 0;
		for (RouteWindow.Step s : w.stepsOrEmpty())
		{
			if (s.current)
			{
				continue;
			}
			if (n == 0)
			{
				p.add(label("Pr\u00f3ximos", FontManager.getRunescapeBoldFont(), Color.WHITE));
			}
			n++;
			p.add(row(s));
		}
		return n == 0 ? null : p;
	}

	private JComponent row(RouteWindow.Step s)
	{
		JPanel r = new JPanel();
		r.setLayout(new BoxLayout(r, BoxLayout.Y_AXIS));
		r.setBackground(ColorScheme.DARKER_GRAY_COLOR);
		r.setBorder(new EmptyBorder(3, 4, 3, 4));
		r.setAlignmentX(LEFT_ALIGNMENT);
		boolean open = expanded.contains(s.id);
		String head = PanelText.statusGlyph(s) + " " + s.ref + "  " + (s.title == null ? "" : s.title);
		JLabel title = wrappedLabel(head, s.blocked() ? MUTED : Color.WHITE);
		title.setFont(new Font(Font.DIALOG, Font.PLAIN, 11));
		if (s.blocked())
		{
			title.setIcon(LOCK);
		}
		r.add(title);
		if (s.blocked() && !open)
		{
			r.add(label(PanelText.blocked(s.blockedOrEmpty()), FontManager.getRunescapeSmallFont(), UNMET));
		}
		if (open)
		{
			addStepBody(r, s, true);
			r.add(buttons(s, false));
		}
		r.setCursor(Cursor.getPredefinedCursor(Cursor.HAND_CURSOR));
		r.setToolTipText(open ? "Clique para recolher" : "Clique para ver o texto e as a\u00e7\u00f5es");
		r.addMouseListener(new MouseAdapter()
		{
			@Override
			public void mouseClicked(MouseEvent e)
			{
				if (!expanded.remove(s.id))
				{
					expanded.add(s.id);
				}
				render();
			}
		});
		JPanel wrap = new JPanel(new BorderLayout());
		wrap.setOpaque(false);
		wrap.setBorder(new EmptyBorder(2, 0, 2, 0));
		wrap.setAlignmentX(LEFT_ALIGNMENT);
		wrap.add(r, BorderLayout.CENTER);
		return wrap;
	}

	private JComponent footer()
	{
		JPanel p = new JPanel(new FlowLayout(FlowLayout.LEFT, 0, 0));
		p.setOpaque(false);
		p.setAlignmentX(LEFT_ALIGNMENT);
		JButton refresh = new JButton("Atualizar");
		refresh.setFocusable(false);
		refresh.setEnabled(!busy && state.connection != PanelState.Connection.NOT_CONFIGURED);
		refresh.addActionListener(e ->
		{
			busy = true;
			render();
			actions.refresh();
		});
		p.add(refresh);
		return p;
	}

	private JComponent note(String text)
	{
		JPanel p = section();
		p.add(wrapped(text, ColorScheme.LIGHT_GRAY_COLOR));
		return p;
	}

	private static JPanel section()
	{
		JPanel p = new JPanel();
		p.setLayout(new BoxLayout(p, BoxLayout.Y_AXIS));
		p.setBackground(ColorScheme.DARK_GRAY_COLOR);
		p.setBorder(new EmptyBorder(0, 0, 8, 0));
		p.setAlignmentX(LEFT_ALIGNMENT);
		return p;
	}

	private static JLabel label(String text, Font font, Color color)
	{
		JLabel l = new JLabel(text == null ? "" : text);
		l.setFont(font);
		l.setForeground(color);
		l.setAlignmentX(LEFT_ALIGNMENT);
		return l;
	}

	/** A label whose text wraps at the panel width. */
	private static JLabel wrapped(String text, Color color)
	{
		JLabel l = wrappedLabel(text, color);
		l.setFont(FontManager.getRunescapeSmallFont());
		return l;
	}

	private static JLabel wrappedLabel(String text, Color color)
	{
		String body = escape(text == null ? "" : text).replace("\n", "<br>");
		// Swing's HTML scales CSS px by 4/3 (96 dpi over 72), so the CSS width is 3/4 of the pixels
		JLabel l = new JLabel("<html><div style='width:" + (TEXT_WIDTH * 3 / 4) + "px'>" + body + "</div></html>");
		l.setForeground(color);
		l.setAlignmentX(LEFT_ALIGNMENT);
		l.setBorder(new EmptyBorder(2, 0, 2, 0));
		return l;
	}

	private static String escape(String s)
	{
		return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;");
	}

	/** A small padlock, drawn so the panel needs no resource files. */
	private static BufferedImage lockImage()
	{
		BufferedImage img = new BufferedImage(10, 12, BufferedImage.TYPE_INT_ARGB);
		Graphics2D g = img.createGraphics();
		g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
		g.setColor(UNMET);
		g.setStroke(new BasicStroke(1.6f));
		g.drawArc(2, 0, 6, 8, 0, 180);
		g.drawLine(2, 4, 2, 6);
		g.drawLine(8, 4, 8, 6);
		g.fillRoundRect(0, 5, 10, 7, 2, 2);
		g.dispose();
		return img;
	}

	/** The toolbar icon: a scroll with a route line, drawn in code. */
	static BufferedImage navigationIcon()
	{
		BufferedImage img = new BufferedImage(16, 16, BufferedImage.TYPE_INT_ARGB);
		Graphics2D g = img.createGraphics();
		g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
		g.setColor(new Color(0xd8c38f));
		g.fillRoundRect(2, 1, 12, 14, 3, 3);
		g.setColor(new Color(0x6b4f2a));
		g.setStroke(new BasicStroke(1.4f));
		g.drawRoundRect(2, 1, 12, 14, 3, 3);
		g.setColor(ColorScheme.BRAND_ORANGE);
		g.drawPolyline(new int[]{5, 8, 6, 11}, new int[]{4, 7, 10, 12}, 4);
		g.fillOval(4, 3, 3, 3);
		g.fillOval(10, 11, 3, 3);
		g.dispose();
		return img;
	}

	/** The scrolled content: as wide as the viewport, so long texts wrap instead of widening it. */
	private static final class WidthTrackingPanel extends JPanel implements javax.swing.Scrollable
	{
		WidthTrackingPanel()
		{
			super(new BorderLayout());
			setBackground(ColorScheme.DARK_GRAY_COLOR);
		}

		@Override
		public Dimension getPreferredScrollableViewportSize()
		{
			return getPreferredSize();
		}

		@Override
		public int getScrollableUnitIncrement(java.awt.Rectangle visible, int orientation, int direction)
		{
			return 16;
		}

		@Override
		public int getScrollableBlockIncrement(java.awt.Rectangle visible, int orientation, int direction)
		{
			return Math.max(16, visible.height - 16);
		}

		@Override
		public boolean getScrollableTracksViewportWidth()
		{
			return true;
		}

		@Override
		public boolean getScrollableTracksViewportHeight()
		{
			return false;
		}
	}

	/** FlowLayout that wraps its rows inside a vertical box (the objective chips). */
	private static final class WrapLayout extends FlowLayout
	{
		WrapLayout()
		{
			super(FlowLayout.LEFT, 3, 3);
		}

		@Override
		public Dimension preferredLayoutSize(java.awt.Container target)
		{
			return layoutSize(target);
		}

		@Override
		public Dimension minimumLayoutSize(java.awt.Container target)
		{
			return layoutSize(target);
		}

		private Dimension layoutSize(java.awt.Container target)
		{
			synchronized (target.getTreeLock())
			{
				int maxWidth = TEXT_WIDTH;
				int x = 0;
				int y = 0;
				int rowHeight = 0;
				for (java.awt.Component c : target.getComponents())
				{
					if (!c.isVisible())
					{
						continue;
					}
					Dimension d = c.getPreferredSize();
					if (x > 0 && x + d.width > maxWidth)
					{
						y += rowHeight + getVgap();
						x = 0;
						rowHeight = 0;
					}
					x += d.width + getHgap();
					rowHeight = Math.max(rowHeight, d.height);
				}
				return new Dimension(maxWidth, y + rowHeight + getVgap() * 2);
			}
		}
	}

	static void onSwing(Runnable r)
	{
		if (SwingUtilities.isEventDispatchThread())
		{
			r.run();
		}
		else
		{
			SwingUtilities.invokeLater(r);
		}
	}
}
