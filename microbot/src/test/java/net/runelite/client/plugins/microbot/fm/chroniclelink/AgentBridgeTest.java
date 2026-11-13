package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.google.gson.Gson;
import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpContext;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpPrincipal;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import net.runelite.client.plugins.microbot.util.magic.Rs2CombatSpells;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.prayer.Rs2PrayerEnum;
import net.runelite.client.plugins.timetracking.SummaryState;
import org.junit.jupiter.api.Test;

class AgentBridgeTest
{
	@Test
	void actionsKeepTheGameOrderWithoutEmptySlots()
	{
		assertEquals(List.of("Talk-to", "Bank", "Collect"), AgentBridge.actions(new String[]{"Talk-to", null, "Bank", "", "Collect"}));
		assertEquals(List.of(), AgentBridge.actions(null));
	}

	@Test
	void equipmentSlotsByIndex()
	{
		assertEquals("WEAPON", AgentBridge.slotName(3));
		assertEquals("AMULET", AgentBridge.slotName(2));
		assertEquals("99", AgentBridge.slotName(99));
	}

	@Test
	void limitsAndKeys()
	{
		assertEquals(30, AgentBridge.clamp(500, 1, 30));
		assertEquals(1, AgentBridge.clamp(-4, 1, 30));
		assertEquals(Rs2PrayerEnum.PROTECT_ITEM, AgentBridge.enumOf(Rs2PrayerEnum.class, "PROTECT_ITEM"));
		assertThrows(AgentHandler.BadRequestException.class, () -> AgentBridge.enumOf(Rs2PrayerEnum.class, "SMITE_EVERYONE"));
	}

	@Test
	void spellsKnowWhatTheyAreCastOn()
	{
		assertEquals("item", AgentBridge.targetOf("HIGH_LEVEL_ALCHEMY"));
		assertEquals("item", AgentBridge.targetOf("SUPERHEAT_ITEM"));
		assertEquals("npc", AgentBridge.targetOf("WIND_STRIKE"));
		assertEquals("npc", AgentBridge.targetOf("CONFUSE"));
		assertEquals("none", AgentBridge.targetOf("VARROCK_TELEPORT"));
		// Enchanting has no way to be cast on an item in the client yet: never a plain cast.
		assertEquals("unsupported", AgentBridge.targetOf("ENCHANT_SAPPHIRE_JEWELLERY"));
		assertEquals("unsupported", AgentBridge.targetOf("TELEKINETIC_GRAB"));
		assertEquals("none", AgentBridge.targetOf("BONES_TO_BANANAS"));
		assertEquals(Rs2CombatSpells.FIRE_BOLT, AgentBridge.spellOf("FIRE_BOLT"));
		assertEquals(Rs2Spells.HIGH_LEVEL_ALCHEMY, AgentBridge.spellOf("HIGH_LEVEL_ALCHEMY"));
		assertThrows(AgentHandler.BadRequestException.class, () -> AgentBridge.spellOf("SUMMON_DRAGON"));
	}

	@Test
	void versionRouteAnswersThePluginVersionBehindTheToken() throws IOException
	{
		AgentHandler.setTokenSupplier(() -> "agent-token");
		try
		{
			AgentHandler route = new AgentBridge(new Gson(), () -> "agent-token").handler("/bridge/version");
			FakeExchange ok = new FakeExchange("/bridge/version", "agent-token");
			route.handle(ok);
			assertEquals(200, ok.status);
			assertEquals("{\"version\":\"" + ChronicleLinkPlugin.VERSION + "\"}", ok.body());

			// The Agent Server's checks: a wrong token gets the opaque 404.
			FakeExchange wrong = new FakeExchange("/bridge/version", "other");
			route.handle(wrong);
			assertEquals(404, wrong.status);

			// Only the exact path.
			FakeExchange longer = new FakeExchange("/bridge/version/x", "agent-token");
			route.handle(longer);
			assertEquals(404, longer.status);

			// No token configured for the Agent Server: the bridge answers nothing.
			FakeExchange none = new FakeExchange("/bridge/version", "agent-token");
			new AgentBridge(new Gson(), () -> null).handler("/bridge/version").handle(none);
			assertEquals(404, none.status);
		}
		finally
		{
			AgentHandler.setTokenSupplier(null);
		}
	}

	/** One request to a handler, as the Agent Server's UDS server would hand it over. */
	private static final class FakeExchange extends HttpExchange
	{
		private final URI uri;
		private final Headers request = new Headers();
		private final Headers response = new Headers();
		private final ByteArrayOutputStream out = new ByteArrayOutputStream();
		int status = -1;

		FakeExchange(String path, String token)
		{
			uri = URI.create(path);
			request.add("Host", "localhost");
			request.add("X-Agent-Token", token);
		}

		String body()
		{
			return out.toString(StandardCharsets.UTF_8);
		}

		@Override
		public Headers getRequestHeaders()
		{
			return request;
		}

		@Override
		public Headers getResponseHeaders()
		{
			return response;
		}

		@Override
		public URI getRequestURI()
		{
			return uri;
		}

		@Override
		public String getRequestMethod()
		{
			return "GET";
		}

		@Override
		public HttpContext getHttpContext()
		{
			return null;
		}

		@Override
		public void close()
		{
		}

		@Override
		public InputStream getRequestBody()
		{
			return new ByteArrayInputStream(new byte[0]);
		}

		@Override
		public OutputStream getResponseBody()
		{
			return out;
		}

		@Override
		public void sendResponseHeaders(int rCode, long responseLength)
		{
			status = rCode;
		}

		@Override
		public InetSocketAddress getRemoteAddress()
		{
			return null;
		}

		@Override
		public int getResponseCode()
		{
			return status;
		}

		@Override
		public InetSocketAddress getLocalAddress()
		{
			return null;
		}

		@Override
		public String getProtocol()
		{
			return "HTTP/1.1";
		}

		@Override
		public Object getAttribute(String name)
		{
			return null;
		}

		@Override
		public void setAttribute(String name, Object value)
		{
		}

		@Override
		public void setStreams(InputStream i, OutputStream o)
		{
		}

		@Override
		public HttpPrincipal getPrincipal()
		{
			return null;
		}
	}

	@Test
	void timersCarryATimeOnlyWhenGrowingOrDone()
	{
		assertEquals(Map.of("key", "HERB", "label", "Herbs", "state", "IN_PROGRESS", "readyAt", 1762720000L),
			AgentBridge.timer("HERB", "Herbs", SummaryState.IN_PROGRESS, 1762720000L));
		assertEquals(Map.of("key", "TREE", "label", "Trees", "state", "COMPLETED", "readyAt", 1762700000L),
			AgentBridge.timer("TREE", "Trees", SummaryState.COMPLETED, 1762700000L));
		assertEquals(Map.of("key", "HERB", "label", "Herbs", "state", "EMPTY"),
			AgentBridge.timer("HERB", "Herbs", SummaryState.EMPTY, 1762720000L));
		assertEquals(Map.of("key", "BIRD_HOUSE", "label", "Bird Houses", "state", "UNKNOWN"),
			AgentBridge.timer("BIRD_HOUSE", "Bird Houses", null, -1));
		assertEquals(Map.of("key", "HERB", "label", "Herbs", "state", "IN_PROGRESS"),
			AgentBridge.timer("HERB", "Herbs", SummaryState.IN_PROGRESS, 0));
	}
}
