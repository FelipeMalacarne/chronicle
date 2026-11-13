package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.awt.BasicStroke;
import java.awt.BorderLayout;
import java.awt.Color;
import java.awt.Cursor;
import java.awt.Dimension;
import java.awt.FlowLayout;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.GridLayout;
import java.awt.Insets;
import java.awt.RenderingHints;
import java.awt.event.ComponentAdapter;
import java.awt.event.ComponentEvent;
import java.awt.event.MouseAdapter;
import java.awt.event.MouseEvent;
import java.awt.image.BufferedImage;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import javax.swing.BorderFactory;
import javax.swing.Box;
import javax.swing.BoxLayout;
import javax.swing.Icon;
import javax.swing.JButton;
import javax.swing.JComponent;
import javax.swing.JLabel;
import javax.swing.JPanel;
import javax.swing.JScrollPane;
import javax.swing.ScrollPaneConstants;
import javax.swing.SwingUtilities;
import javax.swing.border.EmptyBorder;
import javax.swing.plaf.basic.BasicGraphicsUtils;
import net.runelite.client.ui.ColorScheme;
import net.runelite.client.ui.FontManager;
import net.runelite.client.ui.PluginPanel;
import net.runelite.client.ui.components.ThinProgressBar;

/**
 * The route panel in RuneLite's own look: a header with the route, its progress and the
 * cursor's section; the current step (AGORA) with its notes, objectives, the assisted mode's
 * checklist of its tasks (1.14.0: what the planner sees done, never an action) and Feito/Pular/
 * Desfazer; the items to carry, collapsed into one summary; and the next steps, one line each
 * (click to expand). Swing only: every method runs on the Swing thread, and actions go to
 * {@link Actions}, which hands them to the panel executor.
 *
 * <p>Type: bold RuneScape for titles, regular for body text, small only for metadata.
 * Colours: {@link ColorScheme} (dark grey cards, brand orange accents, the progress colours for
 * verdicts), and every verdict also has its own drawn icon ({@link PanelIcons}).
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

		/** The owner opened or closed the item list (remembered in the config). */
		default void itemsExpanded(boolean expanded)
		{
		}

		/** Starts the planner login (Credential Login). */
		default void login()
		{
		}

		/** Cancels a login waiting for the browser. */
		default void cancelLogin()
		{
		}

		/** Forgets the login. */
		default void logout()
		{
		}

		/** Opens the login page in the browser. */
		default void open(String url)
		{
		}
	}

	/** Rows of the "Próximos" list. */
	static final int NEXT_ROWS = 8;
	/** Items of a derived preview shown when expanded, besides the missing ones. */
	static final int PREVIEW_ITEMS = 6;

	private static final Font BOLD = FontManager.getRunescapeBoldFont();
	private static final Font BODY = FontManager.getRunescapeFont();
	private static final Font SMALL = FontManager.getRunescapeSmallFont();

	private static final Color TITLE = Color.WHITE;
	private static final Color TEXT = ColorScheme.TEXT_COLOR;
	private static final Color MUTED = ColorScheme.LIGHT_GRAY_COLOR;
	private static final Color CARD = ColorScheme.DARKER_GRAY_COLOR;
	private static final Color CARD_HOVER = ColorScheme.DARKER_GRAY_HOVER_COLOR;
	private static final Color BG = ColorScheme.DARK_GRAY_COLOR;

	/** Padding inside a card. */
	private static final int PAD = 8;
	/** Width of the AGORA card's orange edge. */
	private static final int EDGE = 3;
	/** Icon column of rows. */
	private static final int ICON_COL = PanelIcons.SIZE + 6;
	private static final int BUTTON_HEIGHT = 28;

	private final Actions actions;
	private final JPanel content = new JPanel();
	/** Steps of the list the owner expanded. */
	private final Set<String> expanded = new HashSet<>();
	private boolean itemsOpen;
	private boolean sectionOpen;
	private PanelState state = new PanelState(PanelState.Connection.NOT_CONFIGURED, null, null, null, null);
	/** The login of Credential Login, or null in the other modes. */
	private LoginSession.View login;
	/** URL and token come from the environment (a managed worker), not from the config. */
	private boolean fromEnvironment;
	private boolean busy;
	private int renderedWidth;

	RoutePanel(Actions actions)
	{
		this(actions, false);
	}

	RoutePanel(Actions actions, boolean itemsExpanded)
	{
		super(false);
		this.actions = actions;
		this.itemsOpen = itemsExpanded;
		setLayout(new BorderLayout());
		setBackground(BG);
		setBorder(new EmptyBorder(BORDER_OFFSET, BORDER_OFFSET, BORDER_OFFSET, BORDER_OFFSET));
		content.setLayout(new BoxLayout(content, BoxLayout.Y_AXIS));
		content.setBackground(BG);
		content.addComponentListener(new ComponentAdapter()
		{
			@Override
			public void componentResized(ComponentEvent e)
			{
				if (content.getWidth() != renderedWidth)
				{
					render();
				}
			}
		});
		JPanel north = new WidthTrackingPanel();
		north.add(content, BorderLayout.NORTH);
		JScrollPane scroll = new JScrollPane(north);
		scroll.setBorder(null);
		scroll.getViewport().setBackground(BG);
		scroll.getVerticalScrollBar().setUnitIncrement(16);
		scroll.setHorizontalScrollBarPolicy(ScrollPaneConstants.HORIZONTAL_SCROLLBAR_NEVER);
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

	/** New login state (Swing thread); null outside Credential Login. */
	void updateLogin(LoginSession.View next)
	{
		login = next;
		busy = false;
		render();
	}

	/** Whether URL and token come from the environment (Swing thread). */
	void setFromEnvironment(boolean next)
	{
		fromEnvironment = next;
		render();
	}

	/** Refreshes the "há N s" texts (Swing thread). */
	void tickClock()
	{
		render();
	}

	private void render()
	{
		renderedWidth = content.getWidth();
		content.removeAll();
		int width = contentWidth();
		RouteWindow w = state.window;
		content.add(header(w, width));
		if (w == null && needsLogin())
		{
			content.add(gap(8));
			content.add(loginCard(width));
		}
		else if (w == null)
		{
			content.add(gap(8));
			content.add(noteCard(emptyText(), width));
		}
		else
		{
			RouteWindow.Step current = w.current();
			content.add(gap(8));
			if (current == null)
			{
				content.add(noteCard("Rota concluída: nenhum passo pendente.", width));
			}
			else
			{
				content.add(currentCard(w, current, width));
			}
			JComponent carry = carry(w.carry, width);
			if (carry != null)
			{
				content.add(gap(8));
				content.add(carry);
			}
			JComponent next = nextSteps(w, width);
			if (next != null)
			{
				content.add(gap(12));
				content.add(next);
			}
		}
		content.add(gap(10));
		content.add(footer(width));
		content.revalidate();
		content.repaint();
	}

	/** The width the content gets: the laid-out one, or the side panel's before the first layout. */
	private int contentWidth()
	{
		int w = content.getWidth();
		return w > 0 ? w : PANEL_WIDTH - 2 * BORDER_OFFSET - 7;
	}

	private String emptyText()
	{
		switch (state.connection)
		{
			case NOT_CONFIGURED:
				return fromEnvironment ? PanelText.ENVIRONMENT_URL_INVALID : "Configure a URL do planner nas opções do plugin.";
			case CONNECTING:
				return "Buscando a rota no planner…";
			default:
				return "Sem conexão com o planner" + (state.message == null ? "." : ": " + state.message + ".");
		}
	}

	// ---- header ------------------------------------------------------------------------------

	private JComponent header(RouteWindow w, int width)
	{
		JPanel p = column(BG);
		String name = w == null ? "Chronicle" : PanelText.routeName(w.guide, w.title);
		boolean signIn = needsLogin();
		JLabel status = label(signIn ? "sem login" : PanelText.connectionShort(state, Instant.now()), SMALL, MUTED);
		status.setIcon(PanelIcons.dot(connectionColor()));
		status.setIconTextGap(4);
		status.setToolTipText(signIn ? "Entre no planner pelo botão abaixo" : PanelText.connection(state, Instant.now()));
		JLabel title = label(PanelText.ellipsize(name, width - status.getPreferredSize().width - 8, measure(BOLD)), BOLD, TITLE);
		if (w != null && w.title != null)
		{
			title.setToolTipText(w.title);
		}
		p.add(spread(title, status));
		if (w == null)
		{
			return p;
		}
		RouteWindow.Counts c = w.counts;
		if (c != null && c.total > 0)
		{
			p.add(gap(4));
			ThinProgressBar bar = new ThinProgressBar();
			bar.setMaximumValue(c.total);
			bar.setValue(c.done);
			bar.setForeground(ColorScheme.PROGRESS_COMPLETE_COLOR);
			bar.setBackground(ColorScheme.MEDIUM_GRAY_COLOR);
			fix(bar, width, 4);
			p.add(bar);
			p.add(gap(3));
			JLabel done = label(PanelText.progress(c) + " feitos", SMALL, MUTED);
			JLabel blocked = label(PanelText.blockedCount(c), SMALL, MUTED);
			blocked.setToolTipText("Passos com requisito faltando: o cursor passa por eles");
			p.add(spread(done, blocked));
		}
		JComponent section = section(w, width);
		if (section != null)
		{
			p.add(gap(6));
			p.add(section);
		}
		if (w.attribution != null && !w.attribution.isEmpty())
		{
			p.add(gap(4));
			p.add(text(w.attribution, SMALL, MUTED, width));
		}
		return p;
	}

	private Color connectionColor()
	{
		switch (state.connection)
		{
			case ONLINE:
				return ColorScheme.PROGRESS_COMPLETE_COLOR;
			case STALE:
				return ColorScheme.PROGRESS_INPROGRESS_COLOR;
			case OFFLINE:
				return ColorScheme.PROGRESS_ERROR_COLOR;
			default:
				return ColorScheme.MEDIUM_GRAY_COLOR;
		}
	}

	/** The cursor's section: its title, and the description's first line (all of it on click). */
	private JComponent section(RouteWindow w, int width)
	{
		String id;
		String title;
		String description;
		if (w.section != null)
		{
			id = w.section.id;
			title = w.section.title;
			description = w.section.description;
		}
		else
		{
			RouteWindow.Step cur = w.current();
			if (cur == null)
			{
				return null;
			}
			id = cur.sectionId;
			title = cur.sectionTitle;
			description = null;
		}
		if ((title == null || title.isEmpty()) && (id == null || id.isEmpty()))
		{
			return null;
		}
		String head = (id == null || id.isEmpty() ? "" : id + "  ") + (title == null ? "" : title);
		boolean hasDescription = description != null && !description.trim().isEmpty();
		JPanel p = column(BG);
		int textWidth = hasDescription ? width - ICON_COL : width;
		JComponent headText = text(head, BODY, TEXT, textWidth);
		if (!hasDescription)
		{
			p.add(headText);
			return p;
		}
		JLabel chevron = new JLabel(PanelIcons.of(sectionOpen ? PanelIcons.Shape.CHEVRON_DOWN : PanelIcons.Shape.CHEVRON_RIGHT));
		chevron.setVerticalAlignment(JLabel.TOP);
		chevron.setBorder(new EmptyBorder(2, 0, 0, 6));
		JPanel top = new JPanel(new BorderLayout());
		top.setOpaque(false);
		top.setAlignmentX(LEFT_ALIGNMENT);
		top.add(chevron, BorderLayout.WEST);
		top.add(headText, BorderLayout.CENTER);
		p.add(top);
		List<String> lines = PanelText.wrap(description.trim(), textWidth, measure(SMALL));
		List<String> shown = sectionOpen || lines.size() <= 1 ? lines : List.of(
			lines.size() > 1 ? PanelText.ellipsize(lines.get(0) + " " + lines.get(1), textWidth, measure(SMALL)) : lines.get(0));
		JComponent desc = new TextBlock(shown, SMALL, MUTED, null, null);
		desc.setBorder(new EmptyBorder(2, ICON_COL, 0, 0));
		p.add(desc);
		p.setCursor(Cursor.getPredefinedCursor(Cursor.HAND_CURSOR));
		p.setToolTipText(sectionOpen ? "Clique para recolher a descrição" : "Clique para ler a descrição da seção");
		p.addMouseListener(click(() ->
		{
			sectionOpen = !sectionOpen;
			render();
		}));
		return p;
	}

	// ---- AGORA -------------------------------------------------------------------------------

	private JComponent currentCard(RouteWindow w, RouteWindow.Step s, int width)
	{
		JPanel p = column(CARD);
		p.setBorder(BorderFactory.createCompoundBorder(
			BorderFactory.createMatteBorder(0, EDGE, 0, 0, ColorScheme.BRAND_ORANGE),
			new EmptyBorder(PAD, PAD, PAD, PAD)));
		int inner = width - EDGE - 2 * PAD;
		JLabel now = label("AGORA", SMALL, ColorScheme.BRAND_ORANGE);
		JLabel ref = label("  " + (s.ref == null ? "" : s.ref), SMALL, MUTED);
		JPanel left = new JPanel(new FlowLayout(FlowLayout.LEFT, 0, 0));
		left.setOpaque(false);
		left.add(now);
		left.add(ref);
		JLabel status = label(PanelText.status(s.status), SMALL, MUTED);
		JPanel top = spread(left, status);
		if (s.sectionTitle != null)
		{
			top.setToolTipText((s.sectionId == null ? "" : s.sectionId + " ") + s.sectionTitle);
		}
		p.add(top);
		p.add(gap(4));
		addStepBody(p, w, s, inner, true);
		if (!s.tasksOrEmpty().isEmpty())
		{
			p.add(gap(8));
			p.add(label("Tarefas", SMALL, MUTED));
			for (RouteWindow.Task t : s.tasksOrEmpty())
			{
				p.add(taskRow(t, inner));
			}
		}
		p.add(gap(8));
		p.add(buttons(s, true, inner));
		return p;
	}

	/**
	 * One line of the assisted mode's checklist: a check when the planner sees it done, a ring
	 * while to do, the unknown mark otherwise; what ends it in the tooltip. Read-only: no action.
	 */
	private JComponent taskRow(RouteWindow.Task t, int width)
	{
		// a task still to do is not a failure: only done is coloured
		JLabel state = label(PanelText.taskState(t), SMALL, "met".equals(t.state) ? PanelIcons.verdictColor(t.state) : MUTED);
		state.setVerticalAlignment(JLabel.TOP);
		state.setBorder(new EmptyBorder(2, 6, 0, 0));
		int labelWidth = width - ICON_COL - state.getPreferredSize().width;
		Icon icon = "met".equals(t.state) ? PanelIcons.of(PanelIcons.Shape.CHECK)
			: "unmet".equals(t.state) ? PanelIcons.of(PanelIcons.Shape.RING) : PanelIcons.verdict(t.state);
		JComponent row = iconRow(icon, text(t.label, BODY, "met".equals(t.state) ? MUTED : TEXT, labelWidth), state);
		row.setBorder(new EmptyBorder(1, 0, 1, 0));
		row.setToolTipText(PanelText.taskTooltip(t));
		return row;
	}

	/** Text, location, notes, objectives and the blocked line of a step. */
	private void addStepBody(JPanel p, RouteWindow w, RouteWindow.Step s, int width, boolean current)
	{
		String body = s.text == null || s.text.isEmpty() ? s.title : s.text;
		p.add(text(body, BODY, current ? TITLE : TEXT, width));
		if (s.location != null && !s.location.isEmpty())
		{
			p.add(gap(2));
			p.add(text("Local: " + s.location, SMALL, MUTED, width));
		}
		for (RouteWindow.Note n : s.visibleNotes())
		{
			p.add(gap(5));
			boolean update = RouteWindow.Note.UPDATE.equals(n.kind);
			p.add(new TextBlock(PanelText.wrap(PanelText.noteLabel(n) + " " + n.text, width, measure(BODY)), BODY,
				update ? TEXT : MUTED, PanelText.noteLabel(n), update ? ColorScheme.BRAND_ORANGE : TEXT));
		}
		if (!s.objectivesOrEmpty().isEmpty())
		{
			p.add(gap(6));
			for (RouteWindow.Objective o : s.objectivesOrEmpty())
			{
				p.add(objectiveRow(o, width));
			}
		}
		if (s.blocked())
		{
			p.add(gap(4));
			JComponent row = iconRow(PanelIcons.of(PanelIcons.Shape.LOCK),
				text(PanelText.blocked(s.blockedOrEmpty()), BODY, TEXT, width - ICON_COL), null);
			if (current)
			{
				row.setToolTipText(PanelText.blockedAll(s.blockedOrEmpty()) + " · o cursor passa por este passo");
			}
			p.add(row);
		}
	}

	private JComponent objectiveRow(RouteWindow.Objective o, int width)
	{
		JLabel verdict = label(PanelText.verdictShort(o), SMALL, PanelIcons.verdictColor(o.verdict));
		verdict.setVerticalAlignment(JLabel.TOP);
		verdict.setBorder(new EmptyBorder(2, 6, 0, 0));
		int labelWidth = width - ICON_COL - verdict.getPreferredSize().width;
		JComponent row = iconRow(PanelIcons.verdict(o.verdict), text(PanelText.objectiveLabel(o), BODY, TEXT, labelWidth), verdict);
		row.setBorder(new EmptyBorder(1, 0, 1, 0));
		return row;
	}

	private JComponent buttons(RouteWindow.Step s, boolean withUndo, int width)
	{
		boolean undo = withUndo && state.undoRef != null;
		JPanel row = new JPanel(new GridLayout(1, undo ? 3 : 2, 4, 0));
		row.setOpaque(false);
		row.setAlignmentX(LEFT_ALIGNMENT);
		row.add(button("Feito", true, "Marca o passo como feito", () -> actions.mark(s.id, RoutePanelController.MANUAL_DONE)));
		row.add(button("Pular", false, "Pula o passo", () -> actions.mark(s.id, RoutePanelController.SKIPPED)));
		if (undo)
		{
			row.add(button("Desfazer", false, "Desfaz a última marca (" + state.undoRef + ")", actions::undo));
		}
		fix(row, width, BUTTON_HEIGHT);
		return row;
	}

	private JButton button(String text, boolean primary, String tooltip, Runnable run)
	{
		FlatButton b = new FlatButton(text, primary, BUTTON_HEIGHT);
		b.setToolTipText(tooltip);
		b.setEnabled(!busy && state.window != null);
		b.addActionListener(e ->
		{
			busy = true;
			render();
			run.run();
		});
		return b;
	}

	// ---- items -------------------------------------------------------------------------------

	private JComponent carry(RouteWindow.Carry c, int width)
	{
		if (c == null || (c.itemsOrEmpty().isEmpty() && c.bankActionsOrEmpty().isEmpty()))
		{
			return null;
		}
		JPanel p = column(CARD);
		p.setBorder(new EmptyBorder(PAD, PAD, PAD, PAD));
		int inner = width - 2 * PAD;
		List<RouteWindow.CarryItem> ordered = PanelText.carryOrdered(c.itemsOrEmpty());

		JLabel title = label("Itens", BOLD, TITLE);
		title.setIcon(PanelIcons.of(itemsOpen ? PanelIcons.Shape.CHEVRON_DOWN : PanelIcons.Shape.CHEVRON_RIGHT));
		title.setIconTextGap(6);
		JLabel kind = label(c.derived ? "prévia" : "do trecho", SMALL, MUTED);
		JPanel head = column(CARD);
		head.add(spread(title, kind));
		head.add(gap(2));
		head.add(text(PanelText.carrySummary(c.itemsOrEmpty()), SMALL, MUTED, inner));
		head.setCursor(Cursor.getPredefinedCursor(Cursor.HAND_CURSOR));
		head.setToolTipText((c.derived
			? "O guia não lista o setup deste trecho: prévia do que os próximos passos usam. "
			: (c.title == null ? "" : c.title + ". "))
			+ (itemsOpen ? "Clique para recolher" : "Clique para ver todos"));
		head.addMouseListener(click(() ->
		{
			itemsOpen = !itemsOpen;
			actions.itemsExpanded(itemsOpen);
			render();
		}));
		p.add(head);

		List<RouteWindow.CarryItem> shown = new ArrayList<>();
		List<RouteWindow.CarryItem> hidden = new ArrayList<>();
		int missing = 0;
		for (RouteWindow.CarryItem it : ordered)
		{
			if (PanelText.itemKind(it) == PanelText.ItemKind.MISSING)
			{
				missing++;
			}
		}
		int limit = !itemsOpen ? missing : c.derived ? missing + PREVIEW_ITEMS : ordered.size();
		for (RouteWindow.CarryItem it : ordered)
		{
			(shown.size() < limit ? shown : hidden).add(it);
		}
		if (!shown.isEmpty())
		{
			p.add(gap(4));
		}
		for (RouteWindow.CarryItem it : shown)
		{
			p.add(itemRow(it, inner));
		}
		if (itemsOpen && !hidden.isEmpty())
		{
			JLabel more = label("+" + hidden.size() + (hidden.size() == 1 ? " item" : " itens"), SMALL, MUTED);
			more.setBorder(new EmptyBorder(2, ICON_COL, 0, 0));
			StringBuilder tip = new StringBuilder("<html>");
			for (int i = 0; i < hidden.size(); i++)
			{
				RouteWindow.CarryItem it = hidden.get(i);
				tip.append(i == 0 ? "" : "<br>").append(escape(PanelText.carryName(it))).append(" · ").append(PanelText.carryWhere(it));
			}
			more.setToolTipText(tip.append("</html>").toString());
			p.add(more);
		}
		if (itemsOpen)
		{
			for (String action : c.bankActionsOrEmpty())
			{
				p.add(gap(4));
				p.add(text("Banco: " + action, SMALL, MUTED, inner));
			}
		}
		return p;
	}

	private JComponent itemRow(RouteWindow.CarryItem it, int width)
	{
		PanelText.ItemKind kind = PanelText.itemKind(it);
		JLabel where = label(PanelText.carryWhere(it) + (it.optional ? " (opcional)" : ""), SMALL, PanelIcons.itemColor(kind));
		int nameWidth = width - ICON_COL - where.getPreferredSize().width - 6;
		JLabel name = label(PanelText.ellipsize(PanelText.carryName(it), nameWidth, measure(BODY)), BODY,
			kind == PanelText.ItemKind.BANK ? MUTED : TEXT);
		JLabel icon = new JLabel(PanelIcons.item(kind));
		icon.setBorder(new EmptyBorder(0, 0, 0, ICON_COL - PanelIcons.SIZE));
		JPanel row = new JPanel(new BorderLayout());
		row.setOpaque(false);
		row.setAlignmentX(LEFT_ALIGNMENT);
		row.add(icon, BorderLayout.WEST);
		row.add(name, BorderLayout.CENTER);
		row.add(where, BorderLayout.EAST);
		row.setBorder(new EmptyBorder(1, 0, 1, 0));
		return row;
	}

	// ---- next steps --------------------------------------------------------------------------

	private JComponent nextSteps(RouteWindow w, int width)
	{
		List<RouteWindow.Step> next = new ArrayList<>();
		for (RouteWindow.Step s : w.stepsOrEmpty())
		{
			if (!s.current && next.size() < NEXT_ROWS)
			{
				next.add(s);
			}
		}
		if (next.isEmpty())
		{
			return null;
		}
		JPanel p = column(BG);
		p.add(label("Próximos", BOLD, TITLE));
		p.add(gap(4));
		JPanel list = column(CARD);
		for (int i = 0; i < next.size(); i++)
		{
			JComponent row = stepRow(w, next.get(i), width);
			if (i > 0)
			{
				row.setBorder(BorderFactory.createCompoundBorder(
					BorderFactory.createMatteBorder(1, 0, 0, 0, BG), row.getBorder()));
			}
			list.add(row);
		}
		p.add(list);
		return p;
	}

	private JComponent stepRow(RouteWindow w, RouteWindow.Step s, int width)
	{
		PanelText.StepKind kind = PanelText.stepKind(s);
		boolean open = expanded.contains(s.id);
		int inner = width - 2 * 6;
		JPanel r = column(CARD);
		r.setBorder(new EmptyBorder(5, 6, 5, 6));

		List<RouteWindow.Note> notes = s.visibleNotes();
		String marker = PanelText.noteMarker(notes);
		String ref = s.ref == null ? "" : s.ref;
		int refWidth = measure(SMALL).applyAsInt(ref) + 6;
		int markerWidth = marker == null ? 0 : PanelIcons.SIZE + 4;
		String title = s.title == null ? "" : s.title;
		boolean dim = kind != PanelText.StepKind.PENDING;

		JPanel head = new JPanel();
		head.setLayout(new BoxLayout(head, BoxLayout.X_AXIS));
		head.setOpaque(false);
		head.setAlignmentX(LEFT_ALIGNMENT);
		JLabel icon = new JLabel(PanelIcons.step(kind));
		icon.setBorder(new EmptyBorder(0, 0, 0, ICON_COL - PanelIcons.SIZE));
		head.add(icon);
		JLabel refLabel = label(ref, SMALL, MUTED);
		refLabel.setBorder(new EmptyBorder(1, 0, 0, 6));
		head.add(refLabel);
		JLabel titleLabel = label(open ? "" : PanelText.ellipsize(title, inner - ICON_COL - refWidth - markerWidth, measure(BODY)),
			BODY, dim ? MUTED : TEXT);
		head.add(titleLabel);
		head.add(Box.createHorizontalGlue());
		if (marker != null)
		{
			JLabel m = new JLabel(PanelIcons.of(RouteWindow.Note.UPDATE.equals(marker) ? PanelIcons.Shape.UPDATE : PanelIcons.Shape.NOTE));
			m.setBorder(new EmptyBorder(0, 4, 0, 0));
			head.add(m);
		}
		r.add(head);

		StringBuilder tip = new StringBuilder("<html>").append(escape(ref)).append(" · ").append(escape(title))
			.append("<br>").append(PanelText.stepKindWord(kind));
		if (s.blocked())
		{
			tip.append("<br>").append(escape(PanelText.blockedAll(s.blockedOrEmpty())));
		}
		if (marker != null)
		{
			tip.append("<br>").append(RouteWindow.Note.UPDATE.equals(marker) ? "tem atualização do guia" : "tem dica");
		}
		tip.append("<br>").append(open ? "Clique para recolher" : "Clique para ver o texto e as ações").append("</html>");
		r.setToolTipText(tip.toString());

		if (open)
		{
			r.add(gap(4));
			addStepBody(r, w, s, inner, false);
			r.add(gap(6));
			r.add(buttons(s, false, inner));
		}
		r.setCursor(Cursor.getPredefinedCursor(Cursor.HAND_CURSOR));
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

			@Override
			public void mouseEntered(MouseEvent e)
			{
				r.setBackground(CARD_HOVER);
			}

			@Override
			public void mouseExited(MouseEvent e)
			{
				r.setBackground(CARD);
			}
		});
		return r;
	}

	// ---- footer ------------------------------------------------------------------------------

	private JComponent footer(int width)
	{
		JPanel p = column(BG);
		FlatButton refresh = new FlatButton("Atualizar", false, 24);
		refresh.setFont(SMALL);
		refresh.setToolTipText("Busca a rota no planner agora");
		refresh.setEnabled(!busy && state.connection != PanelState.Connection.NOT_CONFIGURED);
		refresh.addActionListener(e ->
		{
			busy = true;
			render();
			actions.refresh();
		});
		JPanel row = new JPanel(new FlowLayout(FlowLayout.LEFT, 0, 0));
		row.setOpaque(false);
		row.setAlignmentX(LEFT_ALIGNMENT);
		row.add(refresh);
		if (login != null && login.kind == LoginSession.Kind.SIGNED_IN)
		{
			FlatButton out = new FlatButton("Sair", false, 24);
			out.setFont(SMALL);
			out.setToolTipText("Esquece o login deste plugin no planner");
			out.setEnabled(!busy);
			out.addActionListener(e ->
			{
				busy = true;
				render();
				actions.logout();
			});
			row.add(Box.createHorizontalStrut(4));
			row.add(out);
		}
		p.add(row);
		if (fromEnvironment)
		{
			p.add(gap(4));
			JLabel source = label(PanelText.ENVIRONMENT_SOURCE, SMALL, MUTED);
			source.setToolTipText(PanelText.ENVIRONMENT_SOURCE_TOOLTIP);
			p.add(source);
		}
		if (state.message != null && state.window != null)
		{
			p.add(gap(4));
			p.add(text(PanelText.connection(state, Instant.now()), SMALL, ColorScheme.PROGRESS_ERROR_COLOR, width));
		}
		return p;
	}

	/** Whether the empty panel should offer the login instead of a note. */
	private boolean needsLogin()
	{
		return login != null && login.kind != LoginSession.Kind.SIGNED_IN && state.connection == PanelState.Connection.NOT_CONFIGURED;
	}

	/** Entrar, or the code to confirm in the browser while the login waits. */
	private JComponent loginCard(int width)
	{
		JPanel p = column(CARD);
		p.setBorder(new EmptyBorder(PAD, PAD, PAD, PAD));
		int inner = width - 2 * PAD;
		JPanel row = new JPanel(new GridLayout(1, 2, 4, 0));
		row.setOpaque(false);
		row.setAlignmentX(LEFT_ALIGNMENT);
		if (login.kind == LoginSession.Kind.WAITING)
		{
			p.add(text("Abra a página e confirme este código:", BODY, TEXT, inner));
			p.add(gap(6));
			JLabel code = label(login.userCode, BOLD, TITLE);
			code.setToolTipText("Código do login no planner");
			p.add(code);
			p.add(gap(8));
			row.add(loginButton("Abrir página", true, "Abre a página de login no navegador", () -> actions.open(login.url), false));
			row.add(loginButton("Cancelar", false, "Cancela este login", actions::cancelLogin, true));
		}
		else
		{
			p.add(text("Entre no planner para o plugin enviar o progresso e mostrar a rota.", BODY, TEXT, inner));
			if (login.message != null)
			{
				p.add(gap(4));
				p.add(text("Falhou: " + login.message + ".", SMALL, ColorScheme.PROGRESS_ERROR_COLOR, inner));
			}
			p.add(gap(8));
			row = new JPanel(new GridLayout(1, 1, 4, 0));
			row.setOpaque(false);
			row.setAlignmentX(LEFT_ALIGNMENT);
			row.add(loginButton("Entrar", true, "Login pelo navegador, com a conta do planner", actions::login, true));
		}
		fix(row, inner, BUTTON_HEIGHT);
		p.add(row);
		return p;
	}

	/** A login button; {@code waits}: disabled until the next login state arrives. */
	private JButton loginButton(String text, boolean primary, String tooltip, Runnable run, boolean waits)
	{
		FlatButton b = new FlatButton(text, primary, BUTTON_HEIGHT);
		b.setToolTipText(tooltip);
		b.setEnabled(!busy);
		b.addActionListener(e ->
		{
			if (waits)
			{
				busy = true;
				render();
			}
			run.run();
		});
		return b;
	}

	private JComponent noteCard(String text, int width)
	{
		JPanel p = column(CARD);
		p.setBorder(new EmptyBorder(PAD, PAD, PAD, PAD));
		p.add(text(text, BODY, TEXT, width - 2 * PAD));
		return p;
	}

	// ---- building blocks ---------------------------------------------------------------------

	private java.util.function.ToIntFunction<String> measure(Font font)
	{
		FontMetrics fm = content.getFontMetrics(font);
		return fm::stringWidth;
	}

	private JComponent text(String text, Font font, Color color, int width)
	{
		return new TextBlock(PanelText.wrap(text, width, measure(font)), font, color, null, null);
	}

	private static JPanel column(Color background)
	{
		JPanel p = new JPanel();
		p.setLayout(new BoxLayout(p, BoxLayout.Y_AXIS));
		p.setBackground(background);
		p.setAlignmentX(LEFT_ALIGNMENT);
		return p;
	}

	/** Left and right components on one line. */
	private static JPanel spread(JComponent left, JComponent right)
	{
		JPanel p = new JPanel(new BorderLayout(6, 0));
		p.setOpaque(false);
		p.setAlignmentX(LEFT_ALIGNMENT);
		p.add(left, BorderLayout.WEST);
		p.add(right, BorderLayout.EAST);
		return p;
	}

	/** An icon in the left column, then a component, then an optional one on the right. */
	private static JPanel iconRow(Icon icon, JComponent center, JComponent east)
	{
		JLabel i = new JLabel(icon);
		i.setVerticalAlignment(JLabel.TOP);
		i.setBorder(new EmptyBorder(2, 0, 0, ICON_COL - PanelIcons.SIZE));
		JPanel p = new JPanel(new BorderLayout());
		p.setOpaque(false);
		p.setAlignmentX(LEFT_ALIGNMENT);
		p.add(i, BorderLayout.WEST);
		p.add(center, BorderLayout.CENTER);
		if (east != null)
		{
			p.add(east, BorderLayout.EAST);
		}
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

	private static Box.Filler gap(int height)
	{
		Box.Filler f = (Box.Filler) Box.createRigidArea(new Dimension(0, height));
		f.setAlignmentX(LEFT_ALIGNMENT);
		return f;
	}

	private static void fix(JComponent c, int width, int height)
	{
		c.setPreferredSize(new Dimension(width, height));
		c.setMaximumSize(new Dimension(Integer.MAX_VALUE, height));
		c.setAlignmentX(LEFT_ALIGNMENT);
	}

	private static MouseAdapter click(Runnable run)
	{
		return new MouseAdapter()
		{
			@Override
			public void mouseClicked(MouseEvent e)
			{
				run.run();
			}
		};
	}

	private static String escape(String s)
	{
		return s == null ? "" : s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;");
	}

	/**
	 * Pre-wrapped lines of text in one font; the first line may start with a prefix in its own
	 * colour (the "Atualização:" of a note).
	 */
	private static final class TextBlock extends JComponent
	{
		private final List<String> lines;
		private final Color color;
		private final String prefix;
		private final Color prefixColor;

		TextBlock(List<String> lines, Font font, Color color, String prefix, Color prefixColor)
		{
			this.lines = lines;
			this.color = color;
			this.prefix = prefix;
			this.prefixColor = prefixColor;
			setFont(font);
			setForeground(color);
			setAlignmentX(LEFT_ALIGNMENT);
		}

		@Override
		public Dimension getPreferredSize()
		{
			FontMetrics fm = getFontMetrics(getFont());
			int w = 0;
			for (String l : lines)
			{
				w = Math.max(w, fm.stringWidth(l));
			}
			Insets in = getInsets();
			return new Dimension(w + in.left + in.right, lines.size() * fm.getHeight() + in.top + in.bottom);
		}

		@Override
		public Dimension getMaximumSize()
		{
			return new Dimension(Integer.MAX_VALUE, getPreferredSize().height);
		}

		@Override
		protected void paintComponent(Graphics g)
		{
			FontMetrics fm = getFontMetrics(getFont());
			Insets in = getInsets();
			g.setFont(getFont());
			int y = in.top + fm.getAscent();
			for (int i = 0; i < lines.size(); i++)
			{
				String line = lines.get(i);
				int x = in.left;
				if (i == 0 && prefix != null && line.startsWith(prefix))
				{
					g.setColor(prefixColor);
					BasicGraphicsUtils.drawString(this, (Graphics2D) g, prefix, x, y);
					x += fm.stringWidth(prefix);
					line = line.substring(prefix.length());
				}
				g.setColor(color);
				BasicGraphicsUtils.drawString(this, (Graphics2D) g, line, x, y);
				y += fm.getHeight();
			}
		}
	}

	/** A flat, slightly rounded button: brand orange when primary, medium grey otherwise. */
	private static final class FlatButton extends JButton
	{
		private final boolean primary;
		private final int minHeight;

		FlatButton(String text, boolean primary, int minHeight)
		{
			super(text);
			this.primary = primary;
			this.minHeight = minHeight;
			setFont(primary ? BOLD : BODY);
			setForeground(primary ? ColorScheme.DARKER_GRAY_COLOR : Color.WHITE);
			setContentAreaFilled(false);
			setBorderPainted(false);
			setFocusPainted(false);
			setFocusable(false);
			setOpaque(false);
			setRolloverEnabled(true);
			setBorder(new EmptyBorder(2, 4, 2, 4));
			setMargin(new Insets(2, 4, 2, 4));
			setCursor(Cursor.getPredefinedCursor(Cursor.HAND_CURSOR));
		}

		@Override
		public Dimension getPreferredSize()
		{
			Dimension d = super.getPreferredSize();
			return new Dimension(d.width, Math.max(d.height, minHeight));
		}

		@Override
		protected void paintComponent(Graphics g)
		{
			Graphics2D g2 = (Graphics2D) g.create();
			g2.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
			Color base = primary ? ColorScheme.BRAND_ORANGE : ColorScheme.MEDIUM_GRAY_COLOR;
			Color bg;
			if (!isEnabled())
			{
				bg = primary ? new Color(0x6e4a10) : new Color(0x3a3a3a);
			}
			else if (getModel().isPressed())
			{
				bg = base.darker();
			}
			else if (getModel().isRollover())
			{
				bg = primary ? new Color(0xf09a10) : new Color(0x5c5c5c);
			}
			else
			{
				bg = base;
			}
			g2.setColor(bg);
			g2.fillRoundRect(0, 0, getWidth(), getHeight(), 5, 5);
			g2.dispose();
			super.paintComponent(g);
		}
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
			setBackground(BG);
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
