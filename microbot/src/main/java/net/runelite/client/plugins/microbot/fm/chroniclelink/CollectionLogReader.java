package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TreeMap;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import net.runelite.api.Client;
import net.runelite.api.gameval.InterfaceID;
import net.runelite.api.widgets.Widget;
import net.runelite.client.util.Text;

/**
 * Reads the collection log page on screen, read-only. The ids and the way to read them follow
 * the pinned client's own ChatCommandsPlugin (2.6.52), which reads the "All Pets" page the
 * same way: after {@code ScriptID.COLLECTION_DRAW_LIST} (2731) ran, the first child of
 * {@link InterfaceID.Collection#HEADER_TEXT} is the page title and the children of
 * {@link InterfaceID.Collection#ITEMS_CONTENTS} are its slots, an obtained one drawn with
 * opacity 0. The same plugin guards against the log of another player opened from a house's
 * adventure log ("The Exploits of &lt;name&gt;", {@link InterfaceID.MenuNew#TITLE} or
 * {@link InterfaceID.Menu#LJ_LAYER2}), and so does this reader through
 * {@link CollectionLogTracker}. The tab is the one tab list left visible, or none.
 *
 * <p>{@link #read} runs on the client thread; everything else is pure.
 */
final class CollectionLogReader
{
	/** The lists of the five tabs, in the game's order, with their contract names. */
	static final int[] TAB_LISTS = {
		InterfaceID.Collection.BOSS_CONTAINER, InterfaceID.Collection.RAID_CONTAINER,
		InterfaceID.Collection.CLUE_CONTAINER, InterfaceID.Collection.MINIGAME_CONTAINER,
		InterfaceID.Collection.OTHER_CONTAINER,
	};
	static final String[] TABS = {"BOSSES", "RAIDS", "CLUES", "MINIGAMES", "OTHER"};
	/** Where the adventure log shows whose it is, newer interface first (as ChatCommandsPlugin). */
	static final int[] ADVENTURE_LOG_TITLES = {InterfaceID.MenuNew.TITLE, InterfaceID.Menu.LJ_LAYER2};
	/** The interfaces of the adventure log, whose load starts a read of its owner. */
	static final int[] ADVENTURE_LOG_GROUPS = {InterfaceID.MENU_NEW, InterfaceID.MENU};
	/** The planner's bounds (internal/core/ingest): a page past them is not sent at all. */
	static final int MAX_TITLE_BYTES = 80;
	static final int MAX_SLOTS = 1000;

	private static final Pattern ADVENTURE_LOG_TITLE = Pattern.compile("The Exploits of (.+)");

	private CollectionLogReader()
	{
	}

	/** One slot as the page draws it. */
	static final class Slot
	{
		final int itemId;
		final int opacity;
		final int quantity;

		Slot(int itemId, int opacity, int quantity)
		{
			this.itemId = itemId;
			this.opacity = opacity;
			this.quantity = quantity;
		}
	}

	/** A page read from the interface, before it gets a time. */
	static final class Page
	{
		final String title;
		final String tab;
		final Map<String, Long> items;

		Page(String title, String tab, Map<String, Long> items)
		{
			this.title = title;
			this.tab = tab;
			this.items = items;
		}
	}

	/**
	 * The page on screen, or null when the log is not open or does not read as a page (no
	 * title, no slot, past the planner's bounds). Client thread only.
	 */
	static Page read(Client client)
	{
		Widget header = client.getWidget(InterfaceID.Collection.HEADER_TEXT);
		Widget contents = client.getWidget(InterfaceID.Collection.ITEMS_CONTENTS);
		if (header == null || contents == null || header.getChildren() == null || contents.getChildren() == null)
		{
			return null;
		}
		Widget first = header.getChild(0);
		String title = first == null ? null : title(first.getText());
		if (title == null)
		{
			return null;
		}
		List<Slot> slots = new ArrayList<>();
		for (Widget child : contents.getChildren())
		{
			if (child != null)
			{
				slots.add(new Slot(child.getItemId(), child.getOpacity(), child.getItemQuantity()));
			}
		}
		Map<String, Long> items = slots(slots);
		if (items.isEmpty() || items.size() > MAX_SLOTS)
		{
			return null;
		}
		boolean[] visible = new boolean[TAB_LISTS.length];
		for (int i = 0; i < TAB_LISTS.length; i++)
		{
			Widget list = client.getWidget(TAB_LISTS[i]);
			visible[i] = list != null && !list.isHidden();
		}
		return new Page(title, tab(visible), items);
	}

	/** The adventure log's owner on screen, or null when no title names one. Client thread only. */
	static String readAdventureLogOwner(Client client)
	{
		for (int id : ADVENTURE_LOG_TITLES)
		{
			Widget w = client.getWidget(id);
			if (w == null)
			{
				continue;
			}
			String owner = adventureLogOwner(w.getText());
			if (owner != null)
			{
				return owner;
			}
			Widget[] children = w.getChildren();
			for (Widget child : children == null ? new Widget[0] : children)
			{
				owner = child == null ? null : adventureLogOwner(child.getText());
				if (owner != null)
				{
					return owner;
				}
			}
		}
		return null;
	}

	/** Whether a loaded interface is one of the adventure log's. */
	static boolean adventureLogGroup(int groupId)
	{
		for (int g : ADVENTURE_LOG_GROUPS)
		{
			if (g == groupId)
			{
				return true;
			}
		}
		return false;
	}

	/** The page title from the header text: tags removed and trimmed; null when empty or too long. */
	static String title(String headerText)
	{
		if (headerText == null)
		{
			return null;
		}
		String t = Text.removeTags(headerText).replace(' ', ' ').trim();
		if (t.isEmpty() || t.getBytes(StandardCharsets.UTF_8).length > MAX_TITLE_BYTES)
		{
			return null;
		}
		return t;
	}

	/**
	 * Item id -> quantity: an obtained slot (opacity 0) counts what it shows, at least 1; any
	 * other slot is 0. Empty slots (no item) are skipped; an item twice on a page keeps the most.
	 */
	static Map<String, Long> slots(List<Slot> slots)
	{
		Map<String, Long> out = new TreeMap<>();
		for (Slot s : slots == null ? Collections.<Slot>emptyList() : slots)
		{
			if (s.itemId <= 0)
			{
				continue;
			}
			long qty = s.opacity == 0 ? Math.max(1, s.quantity) : 0;
			out.merge(Integer.toString(s.itemId), qty, Math::max);
		}
		return out;
	}

	/** The tab whose list is the only one visible; null when none or more than one is. */
	static String tab(boolean[] listVisible)
	{
		String found = null;
		for (int i = 0; i < TABS.length && i < listVisible.length; i++)
		{
			if (listVisible[i])
			{
				if (found != null)
				{
					return null;
				}
				found = TABS[i];
			}
		}
		return found;
	}

	/** The name in an adventure log title ("The Exploits of Iron Test"), or null. */
	static String adventureLogOwner(String text)
	{
		if (text == null)
		{
			return null;
		}
		Matcher m = ADVENTURE_LOG_TITLE.matcher(Text.removeTags(text));
		return m.find() ? m.group(1).trim() : null;
	}

	/** Two player names alike, the game's non-breaking spaces and case aside. */
	static boolean sameName(String a, String b)
	{
		return a != null && b != null && normalize(a).equals(normalize(b));
	}

	private static String normalize(String name)
	{
		return name.replace(' ', ' ').trim().toLowerCase(Locale.ROOT);
	}
}
