package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.Collections;
import java.util.Map;
import java.util.TreeMap;

/**
 * The collection log pages read in the current login, and whose log is on screen. Pure: the
 * client thread feeds it (page draws, the adventure log's owner) and copies the pages into each
 * capture; nothing here touches client APIs.
 *
 * <p>The game tells which items a page holds only while the log is open on that page, so the
 * plugin sends the pages the player opened: each one replaces the planner's copy of that page,
 * and the pages never opened stay unknown there, never missing.
 */
final class CollectionLogTracker
{
	private final Map<String, CollectionLogPage> pages = new TreeMap<>();
	/** An adventure log loaded and its owner is read on the next tick. */
	private boolean adventureLogPending;
	/** Whose adventure log was last opened in this house, or null. */
	private String adventureLogOwner;

	/** A new login: the pages read before belonged to that session's character. */
	void reset()
	{
		pages.clear();
		adventureLogPending = false;
		adventureLogOwner = null;
	}

	void onAdventureLogLoaded()
	{
		adventureLogPending = true;
	}

	boolean adventureLogPending()
	{
		return adventureLogPending;
	}

	/** The owner read on the tick after the load; null (no title read) keeps the last one. */
	void onAdventureLogOwner(String owner)
	{
		adventureLogPending = false;
		if (owner != null)
		{
			adventureLogOwner = owner;
		}
	}

	/** A region load or a hop: the house whose adventure log was open is left behind. */
	void onRegionChange()
	{
		adventureLogOwner = null;
	}

	/**
	 * Whether the log on screen may be another player's: an adventure log is waiting for its
	 * owner, or it named someone else. The planner never gets another player's log.
	 */
	boolean othersLog(String localName)
	{
		return adventureLogPending
			|| (adventureLogOwner != null && !CollectionLogReader.sameName(adventureLogOwner, localName));
	}

	/**
	 * Keeps a page read now. True when it is new or its tab or slots changed; a page read again
	 * unchanged keeps its earlier read, so it is not sent again.
	 */
	boolean record(String title, String tab, Map<String, Long> items, Instant now)
	{
		CollectionLogPage page = new CollectionLogPage(tab, now, items);
		if (page.sameContent(pages.get(title)))
		{
			return false;
		}
		pages.put(title, page);
		return true;
	}

	/** Every page read in this login, by title; empty before the first. */
	Map<String, CollectionLogPage> pages()
	{
		return Collections.unmodifiableMap(new TreeMap<>(pages));
	}
}
