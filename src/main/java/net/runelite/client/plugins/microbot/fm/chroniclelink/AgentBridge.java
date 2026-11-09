package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import com.sun.net.httpserver.HttpExchange;
import java.io.File;
import java.io.IOException;
import java.nio.file.Path;
import java.time.Instant;
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
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginManager;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.agentserver.AgentServerConfig;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import net.runelite.client.plugins.microbot.agentserver.uds.UdsHttpServer;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.magic.Rs2CombatSpells;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spellbook;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.magic.Spell;
import net.runelite.client.plugins.microbot.util.prayer.Rs2Prayer;
import net.runelite.client.plugins.microbot.util.prayer.Rs2PrayerEnum;
import net.runelite.client.plugins.timetracking.SummaryState;
import net.runelite.client.plugins.timetracking.Tab;
import net.runelite.client.plugins.timetracking.TimeTrackingPlugin;
import net.runelite.client.plugins.timetracking.farming.FarmingContractManager;
import net.runelite.client.plugins.timetracking.farming.FarmingTracker;
import net.runelite.client.plugins.timetracking.hunter.BirdHouseTracker;

/**
 * Chronicle's control bridge to what the Agent Server does not expose (docs/08 in ironman-planner),
 * started by the plugin only in the dev harness client (-Dmicrobot.dev.harness=true):
 * the real menu options of nearby NPCs and objects and of worn items, the prayer book and the
 * spellbook, the clocks of RuneLite's Time Tracking (farming tabs, bird houses, the farming
 * contract), and typed actions (Remove/Rub a worn item, toggle a prayer, cast a spell, Deposit-X). It listens on its own Unix socket, .runelite/agent/bridge.sock, next to the Agent
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
			new Handler(gson, "/bridge/timers")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					sendJson(ex, 200, timers());
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
					String key = required(body, "key");
					Spell spell = spellOf(key);
					String target = targetOf(key);
					String item = getStringField(body, "item");
					Integer npcIndex = getIntField(body, "npc");
					boolean ok;
					if (item != null && !item.isEmpty())
					{
						if (!"item".equals(target))
						{
							throw new BadRequestException("this spell does not take an item");
						}
						Rs2ItemModel it = inventoryItem(item);
						if (spell == Rs2Spells.SUPERHEAT_ITEM)
						{
							Rs2Magic.superHeat(it);
						}
						else
						{
							Rs2Magic.alch(it);
						}
						ok = true;
					}
					else if (npcIndex != null)
					{
						if (!"npc".equals(target))
						{
							throw new BadRequestException("this spell is not cast on an NPC");
						}
						Rs2NpcModel npc = npcAt(npcIndex);
						ok = Rs2Magic.castOn(spell, npc.getNpc());
					}
					else
					{
						if (!"none".equals(target))
						{
							throw new BadRequestException("unsupported".equals(target)
								? "the bridge cannot cast this spell on its target yet"
								: "this spell needs a target (item or npc)");
						}
						ok = Rs2Magic.cast(spell);
					}
					sendJson(ex, ok ? 200 : 409, result(ok, ok ? null : "the spell was not cast (level, runes or the spellbook)"));
				}
			},
			new Handler(gson, "/bridge/inventory/use-on")
			{
				@Override
				protected void serve(HttpExchange ex) throws IOException
				{
					// The game's "Use X -> Y": an inventory item on another item, an NPC or an object.
					Map<String, Object> body = post(ex);
					Rs2ItemModel it = inventoryItem(required(body, "item"));
					String other = getStringField(body, "onItem");
					Integer npcIndex = getIntField(body, "npc");
					Integer objectId = getIntField(body, "object");
					int targets = (other != null && !other.isEmpty() ? 1 : 0) + (npcIndex != null ? 1 : 0)
						+ (objectId != null ? 1 : 0);
					if (targets != 1)
					{
						throw new BadRequestException("one target: onItem, npc or object");
					}
					boolean ok;
					if (npcIndex != null)
					{
						ok = Rs2Inventory.useItemOnNpc(it.getId(),
							new net.runelite.client.plugins.microbot.util.npc.Rs2NpcModel(npcAt(npcIndex).getNpc()));
					}
					else if (objectId != null)
					{
						ok = Rs2Inventory.useItemOnObject(it.getId(), objectId);
					}
					else
					{
						ok = Rs2Inventory.combine(it, inventoryItem(other));
					}
					sendJson(ex, ok ? 200 : 409, result(ok, ok ? null : "the game did not take the use (target gone or out of reach)"));
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
			.map(o -> {
				ObjectComposition def = o.getObjectComposition();
				Map<String, Object> m = new LinkedHashMap<>();
				m.put("id", o.getId());
				m.put("name", def != null ? def.getName() : o.getName());
				m.put("actions", def == null ? Collections.emptyList() : actions(def.getActions()));
				m.put("position", position(o.getWorldLocation()));
				return m;
			})
			// Most nearby tiles hold scenery (unnamed, or named with no action, like bullrushes): drop
			// those first, then take the nearest usable ones (the stream is lazy past the sort, so it
			// reads compositions only until it has them). The composition resolves impostors, so
			// farming patches and varbit doors keep their current actions.
			.filter(m -> m.get("name") != null && !"null".equals(m.get("name"))
				&& !((List<?>) m.get("actions")).isEmpty())
			.limit(limit)
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

	/** Spells cast on an inventory item that the client has a way to cast (enchanting is not one yet). */
	static final java.util.Set<String> ITEM_SPELLS = new java.util.HashSet<>(Arrays.asList(
		"LOW_LEVEL_ALCHEMY", "HIGH_LEVEL_ALCHEMY", "SUPERHEAT_ITEM"));

	/** Spells of the client's Rs2Spells cast on an NPC (curses, grasps, demonbanes); every combat spell is too. */
	static final java.util.Set<String> NPC_SPELLS = new java.util.HashSet<>(Arrays.asList(
		"CONFUSE", "WEAKEN", "CURSE", "BIND", "SNARE", "VULNERABILITY", "ENFEEBLE", "ENTANGLE", "STUN",
		"MONSTER_EXAMINE", "GHOSTLY_GRASP", "SKELETAL_GRASP", "UNDEAD_GRASP", "INFERIOR_DEMONBANE",
		"SUPERIOR_DEMONBANE", "DARK_DEMONBANE", "DARK_LURE"));

	/**
	 * Spells that wait for a target the bridge cannot give yet (jewellery and orbs, ground items,
	 * other players, farming patches, heads): a plain cast would leave the spell selected in game.
	 */
	static final java.util.Set<String> UNSUPPORTED_SPELLS = new java.util.HashSet<>(Arrays.asList(
		"ENCHANT_SAPPHIRE_JEWELLERY", "ENCHANT_EMERALD_JEWELLERY", "ENCHANT_RUBY_JEWELLERY",
		"ENCHANT_DIAMOND_JEWELLERY", "ENCHANT_DRAGONSTONE_JEWELLERY", "ENCHANT_ONYX_JEWELLERY",
		"ENCHANT_ZENYTE_JEWELLERY", "CHARGE_WATER_ORB", "CHARGE_EARTH_ORB", "CHARGE_FIRE_ORB",
		"CHARGE_AIR_ORB", "TELEKINETIC_GRAB", "TELE_BLOCK", "CURE_OTHER", "STAT_SPY", "ENERGY_TRANSFER",
		"HEAL_OTHER", "VENGEANCE_OTHER", "PLANK_MAKE", "CURE_PLANT", "FERTILE_SOIL", "RESURRECT_CROPS",
		"BASIC_REANIMATION", "ADEPT_REANIMATION", "EXPERT_REANIMATION", "MASTER_REANIMATION"));

	/**
	 * What a spell is cast on: "item", "npc", "none" (teleports, Bones to Bananas, Vengeance) or
	 * "unsupported" (a target the bridge cannot give yet; the planner does not offer it).
	 */
	static String targetOf(String key)
	{
		if (UNSUPPORTED_SPELLS.contains(key))
		{
			return "unsupported";
		}
		if (ITEM_SPELLS.contains(key))
		{
			return "item";
		}
		if (NPC_SPELLS.contains(key) || isCombatSpell(key))
		{
			return "npc";
		}
		return "none";
	}

	static boolean isCombatSpell(String key)
	{
		return Arrays.stream(Rs2CombatSpells.values()).anyMatch(s -> s.name().equals(key));
	}

	/** A spell by the client's key, from Rs2Spells or, for combat spells, Rs2CombatSpells. */
	static Spell spellOf(String key)
	{
		return isCombatSpell(key) ? enumOf(Rs2CombatSpells.class, key) : enumOf(Rs2Spells.class, key);
	}

	private static Map<String, Object> spells()
	{
		Rs2Spellbook book = Rs2Spellbook.getCurrentSpellbook();
		List<Map<String, Object>> list = new ArrayList<>();
		List<Map.Entry<String, Spell>> all = new ArrayList<>();
		for (Rs2Spells s : Rs2Spells.values())
		{
			all.add(new java.util.AbstractMap.SimpleEntry<>(s.name(), s));
		}
		for (Rs2CombatSpells s : Rs2CombatSpells.values())
		{
			all.add(new java.util.AbstractMap.SimpleEntry<>(s.name(), s));
		}
		for (Map.Entry<String, Spell> e : all)
		{
			Spell s = e.getValue();
			if (s.getSpellbook() != book)
			{
				continue;
			}
			String target = targetOf(e.getKey());
			Map<String, Object> m = new LinkedHashMap<>();
			m.put("key", e.getKey());
			m.put("name", s instanceof Rs2Spells ? ((Rs2Spells) s).getName() : ((Rs2CombatSpells) s).getName());
			m.put("level", s.getRequiredLevel());
			m.put("castable", Rs2Magic.canCast(s));
			m.put("target", target);
			m.put("onItem", "item".equals(target)); // planner before 1.5
			list.add(m);
		}
		list.sort(Comparator.comparingInt(m -> (int) m.get("level")));
		Map<String, Object> out = new LinkedHashMap<>();
		out.put("spellbook", book == null ? null : book.name());
		out.put("spells", list);
		return out;
	}

	/** An inventory item by its exact name (case-insensitive), or a 409 when it is not there. */
	private static Rs2ItemModel inventoryItem(String name)
	{
		Rs2ItemModel it = Rs2Inventory.get(i -> i != null && name.equalsIgnoreCase(i.getName()));
		if (it == null)
		{
			throw new ConflictException("not in the inventory: " + name);
		}
		return it;
	}

	/** A nearby NPC by its index (unique while it is loaded), or a 409 when it is gone. */
	private static Rs2NpcModel npcAt(int index)
	{
		return Microbot.getClientThread().runOnClientThreadOptional(() ->
				Microbot.getRs2NpcCache().query().within(MAX_DISTANCE).toList().stream()
					.filter(n -> n.getIndex() == index).findFirst())
			.flatMap(o -> o)
			.orElseThrow(() -> new ConflictException("the NPC is gone"));
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

	/**
	 * The Time Tracking panel's clocks, read from the plugin's own trackers (singletons of its
	 * injector), so they are the numbers the client shows. Farming patches are only seen near them;
	 * the tracker predicts from the last visit. Nothing when the plugin is off.
	 */
	private static Map<String, Object> timers()
	{
		Map<String, Object> out = new LinkedHashMap<>();
		out.put("now", Instant.now().getEpochSecond());
		TimeTrackingPlugin plugin = timeTracking();
		out.put("enabled", plugin != null);
		List<Map<String, Object>> rows = new ArrayList<>();
		if (plugin != null)
		{
			BirdHouseTracker birds = plugin.getInjector().getInstance(BirdHouseTracker.class);
			rows.add(timer(Tab.BIRD_HOUSE.name(), Tab.BIRD_HOUSE.getName(), birds.getSummary(), birds.getCompletionTime()));
			FarmingTracker farming = plugin.getInjector().getInstance(FarmingTracker.class);
			for (Tab tab : Tab.FARMING_TABS)
			{
				rows.add(timer(tab.name(), tab.getName(), farming.getSummary(tab), farming.getCompletionTime(tab)));
			}
			FarmingContractManager contract = plugin.getInjector().getInstance(FarmingContractManager.class);
			if (contract.hasContract())
			{
				rows.add(timer("FARMING_CONTRACT", "Farming contract: " + contract.getContractName(),
					contract.getSummary(), contract.getCompletionTime()));
			}
		}
		out.put("timers", rows);
		return out;
	}

	private static TimeTrackingPlugin timeTracking()
	{
		PluginManager pm = Microbot.getPluginManager();
		if (pm == null)
		{
			return null;
		}
		for (Plugin p : pm.getPlugins())
		{
			if (p instanceof TimeTrackingPlugin && pm.isPluginActive(p) && p.getInjector() != null)
			{
				return (TimeTrackingPlugin) p;
			}
		}
		return null;
	}

	/** One clock: the tab's summary and, while growing or done, when it is ready (epoch seconds). */
	static Map<String, Object> timer(String key, String label, SummaryState state, long completion)
	{
		Map<String, Object> m = new LinkedHashMap<>();
		m.put("key", key);
		m.put("label", label);
		m.put("state", state == null ? SummaryState.UNKNOWN.name() : state.name());
		// The trackers use -1 (and 0 before the first load) for "not known".
		if (completion > 0 && (state == SummaryState.IN_PROGRESS || state == SummaryState.COMPLETED))
		{
			m.put("readyAt", completion);
		}
		return m;
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

	/** The game state does not allow the action (item not in the inventory, NPC gone): a 409. */
	static final class ConflictException extends RuntimeException
	{
		ConflictException(String message)
		{
			super(message);
		}
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
			try
			{
				serve(exchange);
			}
			catch (ConflictException e)
			{
				sendJson(exchange, 409, result(false, e.getMessage()));
			}
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
