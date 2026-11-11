package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import net.runelite.api.ChatMessageType;
import net.runelite.client.util.Text;

/**
 * The game events the planner's agent waits for (its triggers, docs/08 phase 4 delivery 3), as
 * the bridge's /bridge/events serves them. Only the game's own messages of a closed list are
 * recorded, never player chat, so nobody can fire a trigger by typing its text. The numbering
 * restarts with the plugin; {@link #boot} tells the agent it did.
 */
final class GameEvents
{
	static final String KITTEN_GROWN = "kitten_grown";
	private static final int KEEP = 50;

	private final String boot = UUID.randomUUID().toString();
	private final Deque<Map<String, Object>> events = new ArrayDeque<>();
	private long seq;

	/** The event a chat message is, or null for every other message. */
	static String kindOf(ChatMessageType type, String message)
	{
		if (message == null || !(type == ChatMessageType.GAMEMESSAGE || type == ChatMessageType.SPAM
			|| type == ChatMessageType.MESBOX || type == ChatMessageType.ENGINE))
		{
			return null;
		}
		String text = Text.removeTags(message).trim();
		if (text.startsWith("Your kitten has grown into a healthy cat"))
		{
			return KITTEN_GROWN;
		}
		return null;
	}

	/** Records the message when it is one of the events. */
	synchronized void onChat(ChatMessageType type, String message, Instant at)
	{
		String kind = kindOf(type, message);
		if (kind == null)
		{
			return;
		}
		Map<String, Object> e = new LinkedHashMap<>();
		e.put("seq", ++seq);
		e.put("kind", kind);
		e.put("at", at.getEpochSecond());
		events.addLast(e);
		while (events.size() > KEEP)
		{
			events.removeFirst();
		}
	}

	/** The events after seq {@code after}, the last seq and the boot id. */
	synchronized Map<String, Object> since(long after)
	{
		List<Map<String, Object>> out = new ArrayList<>();
		for (Map<String, Object> e : events)
		{
			if ((long) e.get("seq") > after)
			{
				out.add(e);
			}
		}
		Map<String, Object> m = new LinkedHashMap<>();
		m.put("boot", boot);
		m.put("last", seq);
		m.put("events", out);
		return m;
	}
}
