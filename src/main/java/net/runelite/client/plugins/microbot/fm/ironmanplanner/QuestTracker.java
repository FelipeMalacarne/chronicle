package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.Map;
import java.util.TreeMap;
import java.util.function.Function;
import net.runelite.api.Quest;
import net.runelite.api.QuestState;

/**
 * Cached quest states for the {@code data.quest} facet. Quest states are derived from varps and
 * varbits, so the cache is only re-read after a {@code VarbitChanged} (which RuneLite also
 * posts for plain varp changes) marked it dirty, and at most once per capture. Client thread
 * only: {@code Quest.getState} runs a client script.
 */
final class QuestTracker
{
	/** {@code data.quest} keys longer than this (in UTF-8 bytes) are rejected by the server. */
	static final int MAX_NAME_BYTES = 120;

	private final Quest[] quests;
	private final Function<Quest, QuestState> reader;
	private Map<String, String> cached;
	private boolean dirty = true;

	QuestTracker(Quest[] quests, Function<Quest, QuestState> reader)
	{
		this.quests = quests.clone();
		this.reader = reader;
	}

	/** A var changed: the next {@link #read} looks at the quests again. */
	void invalidate()
	{
		dirty = true;
	}

	/** New login: forget what the previous character had. */
	void reset()
	{
		cached = null;
		dirty = true;
	}

	/**
	 * Every quest the client knows, {@code getName() -> QuestState.name()}, or {@code null} when
	 * any read failed: a full push with {@code data.quest} replaces every stored quest, so a
	 * partial list is never sent. Names the server would reject (blank, padded, too long) are
	 * left out rather than failing the whole push.
	 */
	Map<String, String> read()
	{
		if (!dirty && cached != null)
		{
			return cached;
		}
		TreeMap<String, String> out = new TreeMap<>();
		try
		{
			for (Quest quest : quests)
			{
				String name = quest.getName();
				if (!validName(name))
				{
					continue;
				}
				QuestState state = reader.apply(quest);
				if (state == null)
				{
					return failed();
				}
				out.put(name, state.name());
			}
		}
		catch (RuntimeException ex)
		{
			return failed();
		}
		cached = Collections.unmodifiableMap(out);
		dirty = false;
		return cached;
	}

	static boolean validName(String name)
	{
		return name != null && !name.isEmpty() && name.trim().equals(name)
			&& name.getBytes(StandardCharsets.UTF_8).length <= MAX_NAME_BYTES;
	}

	private Map<String, String> failed()
	{
		// stays dirty: the next capture tries again
		cached = null;
		dirty = true;
		return null;
	}
}
