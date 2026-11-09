package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import com.sun.net.httpserver.HttpExchange;
import java.io.File;
import java.io.IOException;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.stream.Collectors;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.EquipmentInventorySlot;
import net.runelite.api.NPCComposition;
import net.runelite.api.ObjectComposition;
import net.runelite.api.Skill;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.RuneLite;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.agentserver.AgentServerConfig;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import net.runelite.client.plugins.microbot.agentserver.uds.UdsHttpServer;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spellbook;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.prayer.Rs2Prayer;
import net.runelite.client.plugins.microbot.util.prayer.Rs2PrayerEnum;

/**
 * Chronicle's control bridge to what the Agent Server does not expose (docs/08 in ironman-planner),
 * started by the plugin only in the dev harness client (-Dmicrobot.dev.harness=true):
 * the real menu options of nearby NPCs and objects and of worn items, the prayer book and the
 * spellbook, and typed actions for them (Remove/Rub a worn item, toggle a prayer, cast a spell,
 * Deposit-X). It listens on its own Unix socket, .runelite/agent/bridge.sock, next to the Agent
 * Server's, and every request needs the Agent Server's token (the same X-Agent-Token, loopback
 * Host and no Origin as AgentHandler checks); with no token configured it answers nothing.
 */
@Slf4j
final class AgentBridge
{
	static final String SOCKET = "bridge.sock";
	private static final int MAX_DISTANCE = 30;
	private static final int MAX_LIMIT = 30;

	private final Gson gson;
	private final ConfigManager configManager;
	private UdsHttpServer server;
	private ExecutorService executor;

	AgentBridge(Gson gson, ConfigManager configManager)
	{
		this.gson = gson;
		this.configManager = configManager;
	}

	void start()
	{
		Path socket = new File(new File(RuneLite.RUNELITE_DIR, "agent"), SOCKET).toPath();
		executor = Executors.newFixedThreadPool(2, r -> {
			Thread t = new Thread(r, "ChronicleLink-Bridge");
			t.setDaemon(true);
			return t;
		});
		try
		{
			server = new UdsHttpServer(socket, executor);
			for (Handler h : handlers())
			{
				server.createContext(h.getPath(), h);
			}
			server.start();
			log.info("Chronicle Link: control bridge on {}", socket);
		}
		catch (IOException | RuntimeException e)
		{
			log.warn("Chronicle Link: control bridge did not start: {}", e.getMessage());
			stop();
		}
	}

	void stop()
	{
		if (server != null)
		{
			server.stop();
			server = null;
		}
		if (executor != null)
		{
			executor.shutdownNow();
			executor = null;
		}
	}

	private List<Handler> handlers()
	{
		return Arrays.asList(
			new Handler(gson, "/bridge/entities")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					Map<String, String> q = parseQuery(ex.getRequestURI());
					int distance = clamp(getIntParam(q, "distance", 15), 1, MAX_DISTANCE);
					int limit = clamp(getIntParam(q, "limit", 12), 1, MAX_LIMIT);
					Map<String, Object> out = new LinkedHashMap<>();
					out.put("npcs", npcs(distance, limit));
					out.put("objects", objects(distance, limit));
					sendJson(ex, 200, out);
				}
			},
			new Handler(gson, "/bridge/equipment")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					sendJson(ex, 200, Collections.singletonMap("items", equipment()));
				}
			},
			new Handler(gson, "/bridge/prayers")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					sendJson(ex, 200, prayers());
				}
			},
			new Handler(gson, "/bridge/spells")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					sendJson(ex, 200, spells());
				}
			},
			new Handler(gson, "/bridge/equipment/interact")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					Map<String, Object> body = post(ex);
					String name = required(body, "name");
					String action = required(body, "action");
					boolean ok = Rs2Equipment.interact(name, action);
					sendJson(ex, ok ? 200 : 409, result(ok, ok ? null : "the worn item or its action was not found"));
				}
			},
			new Handler(gson, "/bridge/prayers/toggle")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					Map<String, Object> body = post(ex);
					Rs2PrayerEnum prayer = enumOf(Rs2PrayerEnum.class, required(body, "key"));
					boolean on = Boolean.TRUE.equals(getBooleanField(body, "on"));
					boolean ok = Rs2Prayer.toggle(prayer, on);
					sendJson(ex, ok ? 200 : 409, result(ok, ok ? null : "the prayer did not toggle (level, points or the prayer book)"));
				}
			},
			new Handler(gson, "/bridge/spells/cast")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					Map<String, Object> body = post(ex);
					Rs2Spells spell = enumOf(Rs2Spells.class, required(body, "key"));
					String item = getStringField(body, "item");
					boolean ok;
					if (item != null && !item.isEmpty())
					{
						if (spell != Rs2Spells.LOW_LEVEL_ALCHEMY && spell != Rs2Spells.HIGH_LEVEL_ALCHEMY)
						{
							throw new BadRequestException("only the alchemy spells take an item");
						}
						Rs2Magic.alch(item);
						ok = true;
					}
					else
					{
						ok = Rs2Magic.cast(spell);
					}
					sendJson(ex, ok ? 200 : 409, result(ok, ok ? null : "the spell was not cast (level, runes or the spellbook)"));
				}
			},
			new Handler(gson, "/bridge/bank/deposit-x")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					Map<String, Object> body = post(ex);
					String name = required(body, "name");
					Integer amount = getIntField(body, "amount");
					if (amount == null || amount < 1)
					{
						throw new BadRequestException("amount of at least 1");
					}
					boolean ok = Rs2Bank.isOpen() && Rs2Bank.depositX(name, amount);
					sendJson(ex, ok ? 200 : 409, result(ok, ok ? null : "the bank is closed or the item is not in the inventory"));
				}
			}
		);
	}

	static int clamp(int v, int lo, int hi)
	{
		return Math.max(lo, Math.min(hi, v));
	}

	/** The menu options of a composition, as the game lists them (no empty slots). */
	static List<String> actions(String[] raw)
	{
		if (raw == null)
		{
			return Collections.emptyList();
		}
		return Arrays.stream(raw).filter(a -> a != null && !a.isEmpty()).collect(Collectors.toList());
	}

	private static Map<String, Integer> position(WorldPoint p)
	{
		if (p == null)
		{
			return null;
		}
		Map<String, Integer> m = new LinkedHashMap<>();
		m.put("x", p.getX());
		m.put("y", p.getY());
		m.put("plane", p.getPlane());
		return m;
	}

	private static List<Map<String, Object>> npcs(int distance, int limit)
	{
		List<Rs2NpcModel> near = Microbot.getClientThread().runOnClientThreadOptional(() ->
			Microbot.getRs2NpcCache().query().within(distance).toList()).orElse(Collections.emptyList());
		return near.stream()
			.sorted(Comparator.comparingInt(Rs2NpcModel::getDistanceFromPlayer))
			.limit(limit)
			.map(npc -> {
				// The same definition the interaction uses to find the action's index.
				NPCComposition def = Microbot.getClientThread().runOnClientThreadOptional(() ->
					Microbot.getClient().getNpcDefinition(npc.getId())).orElse(null);
				Map<String, Object> m = new LinkedHashMap<>();
				m.put("id", npc.getId());
				m.put("index", npc.getIndex());
				m.put("name", npc.getName());
				m.put("combatLevel", npc.getCombatLevel());
				m.put("actions", def == null ? Collections.emptyList() : actions(def.getActions()));
				m.put("position", position(npc.getWorldLocation()));
				return m;
			})
			.collect(Collectors.toList());
	}

	private static List<Map<String, Object>> objects(int distance, int limit)
	{
		WorldPoint me = Microbot.getRs2PlayerStateCache().getLocalPlayerPosition();
		return Microbot.getRs2TileObjectCache().query().within(distance).toListOnClientThread().stream()
			.sorted(Comparator.comparingInt((Rs2TileObjectModel o) ->
				me == null || o.getWorldLocation() == null ? Integer.MAX_VALUE : me.distanceTo(o.getWorldLocation())))
			.limit(limit)
			.map(o -> {
				ObjectComposition def = o.getObjectComposition();
				Map<String, Object> m = new LinkedHashMap<>();
				m.put("id", o.getId());
				m.put("name", def != null ? def.getName() : o.getName());
				m.put("actions", def == null ? Collections.emptyList() : actions(def.getActions()));
				m.put("position", position(o.getWorldLocation()));
				return m;
			})
			.filter(m -> m.get("name") != null && !"null".equals(m.get("name")))
			.collect(Collectors.toList());
	}

	private static List<Map<String, Object>> equipment()
	{
		List<Map<String, Object>> out = new ArrayList<>();
		for (Rs2ItemModel item : Rs2Equipment.items())
		{
			Map<String, Object> m = new LinkedHashMap<>();
			m.put("slot", slotName(item.getSlot()));
			m.put("id", item.getId());
			m.put("name", item.getName());
			m.put("quantity", item.getQuantity());
			m.put("actions", item.getEquipmentActions() == null ? Collections.emptyList()
				: item.getEquipmentActions().stream().filter(Objects::nonNull).filter(a -> !a.isEmpty()).collect(Collectors.toList()));
			out.add(m);
		}
		return out;
	}

	static String slotName(int idx)
	{
		for (EquipmentInventorySlot s : EquipmentInventorySlot.values())
		{
			if (s.getSlotIdx() == idx)
			{
				return s.name();
			}
		}
		return String.valueOf(idx);
	}

	private static Map<String, Object> prayers()
	{
		int level = Microbot.getClientThread().runOnClientThreadOptional(() ->
			Microbot.getClient().getRealSkillLevel(Skill.PRAYER)).orElse(0);
		List<Map<String, Object>> list = new ArrayList<>();
		for (Rs2PrayerEnum p : Rs2PrayerEnum.values())
		{
			Map<String, Object> m = new LinkedHashMap<>();
			m.put("key", p.name());
			m.put("name", p.getName());
			m.put("level", p.getLevel());
			m.put("unlocked", level >= p.getLevel());
			m.put("active", Rs2Prayer.isPrayerActive(p));
			list.add(m);
		}
		Map<String, Object> out = new LinkedHashMap<>();
		out.put("level", level);
		out.put("points", Rs2Prayer.getPrayerPoints());
		out.put("prayers", list);
		return out;
	}

	private static Map<String, Object> spells()
	{
		Rs2Spellbook book = Rs2Spellbook.getCurrentSpellbook();
		List<Map<String, Object>> list = new ArrayList<>();
		for (Rs2Spells s : Rs2Spells.values())
		{
			if (s.getSpellbook() != book)
			{
				continue;
			}
			Map<String, Object> m = new LinkedHashMap<>();
			m.put("key", s.name());
			m.put("name", s.getName());
			m.put("level", s.getRequiredLevel());
			m.put("castable", Rs2Magic.canCast(s));
			m.put("onItem", s == Rs2Spells.LOW_LEVEL_ALCHEMY || s == Rs2Spells.HIGH_LEVEL_ALCHEMY);
			list.add(m);
		}
		list.sort(Comparator.comparingInt(m -> (int) m.get("level")));
		Map<String, Object> out = new LinkedHashMap<>();
		out.put("spellbook", book == null ? null : book.name());
		out.put("spells", list);
		return out;
	}

	static <E extends Enum<E>> E enumOf(Class<E> type, String key)
	{
		try
		{
			return Enum.valueOf(type, key);
		}
		catch (IllegalArgumentException e)
		{
			throw new AgentHandler.BadRequestException("unknown " + type.getSimpleName() + ": " + key);
		}
	}

	private static Map<String, Object> result(boolean ok, String reason)
	{
		Map<String, Object> m = new LinkedHashMap<>();
		m.put("success", ok);
		if (reason != null)
		{
			m.put("error", reason);
		}
		return m;
	}

	/** A bridge endpoint: AgentHandler's checks, and nothing at all without the Agent Server's token. */
	private abstract class Handler extends AgentHandler
	{
		private final String path;

		Handler(Gson gson, String path)
		{
			super(gson);
			this.path = path;
		}

		@Override
		public String getPath()
		{
			return path;
		}

		@Override
		public void handle(HttpExchange exchange) throws IOException
		{
			String token = configManager.getConfiguration(AgentServerConfig.GROUP, AgentServerConfig.KEY_TOKEN);
			if (token == null || token.isEmpty())
			{
				sendOpaqueNotFound(exchange);
				return;
			}
			super.handle(exchange);
		}

		@Override
		protected void handleRequest(HttpExchange exchange) throws IOException
		{
			// A context matches its path prefix: answer only the exact path.
			if (!path.equals(exchange.getRequestURI().getPath()))
			{
				sendOpaqueNotFound(exchange);
				return;
			}
			serve(exchange);
		}

		protected abstract void serve(HttpExchange exchange) throws IOException;

		Map<String, Object> post(HttpExchange ex) throws IOException
		{
			try
			{
				requirePost(ex);
			}
			catch (HttpMethodException e)
			{
				throw new BadRequestException(e.getMessage());
			}
			return readJsonBody(ex);
		}

		String required(Map<String, Object> body, String key)
		{
			String v = getStringField(body, key);
			if (v == null || v.isEmpty())
			{
				throw new BadRequestException("required: " + key);
			}
			return v;
		}
	}
}
