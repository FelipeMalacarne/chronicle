package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import net.runelite.api.ChatMessageType;
import org.junit.jupiter.api.Test;

class GameEventsTest
{
	private static final String GROWN = "Your kitten has grown into a healthy cat that can hunt for itself.";

	@Test
	void onlyTheGamesOwnMessagesCount()
	{
		assertEquals(GameEvents.KITTEN_GROWN, GameEvents.kindOf(ChatMessageType.GAMEMESSAGE, GROWN));
		assertEquals(GameEvents.KITTEN_GROWN, GameEvents.kindOf(ChatMessageType.GAMEMESSAGE, "<col=ff0000>" + GROWN + "</col>"));
		// A player typing the text fires nothing.
		assertNull(GameEvents.kindOf(ChatMessageType.PUBLICCHAT, GROWN));
		assertNull(GameEvents.kindOf(ChatMessageType.PRIVATECHAT, GROWN));
		assertNull(GameEvents.kindOf(ChatMessageType.GAMEMESSAGE, "Your kitten is hungry."));
		assertNull(GameEvents.kindOf(ChatMessageType.GAMEMESSAGE, null));
	}

	@Test
	@SuppressWarnings("unchecked")
	void eventsAfterASeq()
	{
		GameEvents ev = new GameEvents();
		Map<String, Object> empty = ev.since(0);
		assertEquals(0L, empty.get("last"));
		ev.onChat(ChatMessageType.GAMEMESSAGE, "You catch a leaping trout.", Instant.EPOCH);
		ev.onChat(ChatMessageType.GAMEMESSAGE, GROWN, Instant.ofEpochSecond(100));
		ev.onChat(ChatMessageType.GAMEMESSAGE, GROWN, Instant.ofEpochSecond(200));
		Map<String, Object> all = ev.since(0);
		assertEquals(2L, all.get("last"));
		assertEquals(empty.get("boot"), all.get("boot"));
		List<Map<String, Object>> after1 = (List<Map<String, Object>>) ev.since(1).get("events");
		assertEquals(1, after1.size());
		assertEquals(200L, after1.get(0).get("at"));
		assertEquals(0, ((List<?>) ev.since(2).get("events")).size());
	}

	@Test
	void afterParam()
	{
		assertEquals(0, AgentBridge.afterParam(null));
		assertEquals(0, AgentBridge.afterParam("-3"));
		assertEquals(7, AgentBridge.afterParam("7"));
		assertThrows(RuntimeException.class, () -> AgentBridge.afterParam("x"));
	}
}
