package net.runelite.client.plugins.microbot.fm.mtaarena;

import com.google.inject.Provides;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.EnumSet;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Client;
import net.runelite.api.InventoryID;
import net.runelite.api.Item;
import net.runelite.api.ItemContainer;
import net.runelite.api.Skill;
import net.runelite.api.events.GameTick;
import net.runelite.api.gameval.InterfaceID;
import net.runelite.api.gameval.ItemID;
import net.runelite.api.gameval.VarbitID;
import net.runelite.api.widgets.Widget;
import net.runelite.client.config.ConfigManager;
import net.runelite.client.eventbus.Subscribe;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginDescriptor;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.ui.overlay.OverlayManager;

/**
 * Mage Training Arena planner: pick the Rewards Guardian's items you want and the overlay shows
 * the pizazz points still missing in each room and the law, nature and cosmic runes that earning
 * them takes, against the runes you carry and have banked. It only observes (it never clicks):
 * the points come from the arena's own interface, the owned items from the inventory, equipment,
 * bank and collection log. Independent of Chronicle: the table and the arithmetic are the ones
 * of Chronicle's core (internal/core/mta), pinned by shared test vectors.
 */
@PluginDescriptor(
	name = MtaArenaPlugin.TAG + "MTA Planner",
	description = "Pick Mage Training Arena rewards; shows the pizazz points and runes still needed",
	tags = {"mta", "mage training arena", "pizazz", "runes", "calculator", "ironman"},
	authors = {"FelipeMalacarne"},
	version = MtaArenaPlugin.VERSION,
	minClientVersion = "2.6.52",
	enabledByDefault = false,
	isExternal = true
)
@Slf4j
public class MtaArenaPlugin extends Plugin
{
	static final String VERSION = "0.5.0";
	/** Name prefix of our own plugins in the plugin list. */
	static final String TAG = "<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] ";

	private static final String K_TELE = "pointsTelekinetic";
	private static final String K_GRAVE = "pointsGraveyard";
	private static final String K_ENCH = "pointsEnchantment";
	private static final String K_ALCH = "pointsAlchemist";
	private static final String K_OWNED = "ownedRewards";
	/** Read the world every few ticks: the numbers change on purchases and casts, not per tick. */
	private static final int READ_EVERY_TICKS = 3;

	/** What the overlay draws. Built on the client thread, read on the render thread. */
	static final class Snapshot
	{
		boolean pointsKnown;
		MtaCalculator.Points held = new MtaCalculator.Points(0, 0, 0, 0);
		MtaCalculator.Result result;
		/** What to do next for the chosen rewards, or null when there is no plan. */
		MtaPlanner.Step step;
		/** The rewards the account has. */
		Set<String> owned = new HashSet<>();
		/** Why there is no result (nothing chosen, no Enchant castable), or null. */
		String message;
		int law;
		int nature;
		int cosmic;
	}

	@Inject
	private Client client;

	@Inject
	private MtaArenaConfig config;

	@Inject
	private ConfigManager configManager;

	@Inject
	private OverlayManager overlayManager;

	@Inject
	private MtaArenaOverlay overlay;

	private volatile Snapshot snapshot = new Snapshot();
	private volatile MtaRunner runner;
	private int ticks;

	@Provides
	MtaArenaConfig provideConfig(ConfigManager configManager)
	{
		return configManager.getConfig(MtaArenaConfig.class);
	}

	Snapshot snapshot()
	{
		return snapshot;
	}

	/** What the runner is doing now; empty while it is off. */
	String runnerState()
	{
		MtaRunner r = runner;
		return r == null ? "" : r.state();
	}

	@Override
	protected void startUp()
	{
		log.info("MTA Planner {} starting", VERSION);
		overlayManager.add(overlay);
		runner = new MtaRunner(this, config);
		runner.run();
	}

	@Override
	protected void shutDown()
	{
		overlayManager.remove(overlay);
		if (runner != null)
		{
			runner.shutdown();
			runner = null;
		}
	}

	@Subscribe
	public void onGameTick(GameTick tick)
	{
		if (ticks++ % READ_EVERY_TICKS != 0)
		{
			return;
		}
		if (config.forget())
		{
			forget();
		}
		readPoints();
		Set<String> owned = readOwned();
		snapshot = build(owned);
	}

	private void forget()
	{
		for (String key : new String[]{K_TELE, K_GRAVE, K_ENCH, K_ALCH, K_OWNED})
		{
			configManager.unsetConfiguration(MtaArenaConfig.GROUP, key);
		}
		configManager.setConfiguration(MtaArenaConfig.GROUP, "forget", false);
	}

	/** The arena's points panel (talk to the progress hat, or the lobby's), or the room's own counter. */
	private void readPoints()
	{
		remember(K_TELE, InterfaceID.MagictrainingMain.MAGICTRAIN_TELE_POINTS, MtaWorld.Room.TELEKINETIC);
		remember(K_GRAVE, InterfaceID.MagictrainingMain.MAGICTRAIN_GRAVE_POINTS, MtaWorld.Room.GRAVEYARD);
		remember(K_ENCH, InterfaceID.MagictrainingMain.MAGICTRAIN_ENCH_POINTS, MtaWorld.Room.ENCHANTMENT);
		remember(K_ALCH, InterfaceID.MagictrainingMain.MAGICTRAIN_ALCH_POINTS, MtaWorld.Room.ALCHEMIST);
	}

	private void remember(String key, int widgetId, MtaWorld.Room room)
	{
		int n = readWidget(client.getWidget(widgetId));
		if (n < 0)
		{
			// Inside a room the main panel is not up: the room shows its own points. The Alchemists'
			// Playground counts its coins not handed in yet, a hundred of them to the point.
			int[] w = MtaWorld.roomPointsWidget(room);
			n = readWidget(client.getWidget(w[0], w[1]));
			if (n >= 0 && room == MtaWorld.Room.ALCHEMIST)
			{
				n += coins() / 100;
			}
		}
		if (n >= 0 && n != stored(key, -1))
		{
			configManager.setConfiguration(MtaArenaConfig.GROUP, key, n);
		}
	}

	private static int readWidget(Widget w)
	{
		return w == null || w.isHidden() ? -1 : MtaText.lastNumber(w.getText());
	}

	private int coins()
	{
		ItemContainer c = client.getItemContainer(InventoryID.INVENTORY);
		if (c == null)
		{
			return 0;
		}
		int n = 0;
		for (Item item : c.getItems())
		{
			if (item.getId() == MtaWorld.COINS)
			{
				n += item.getQuantity();
			}
		}
		return n;
	}

	private int stored(String key, int fallback)
	{
		Integer v = configManager.getConfiguration(MtaArenaConfig.GROUP, key, Integer.class);
		return v == null ? fallback : v;
	}

	/** Rewards the account has: carried, worn or banked now, remembered from before, or logged. */
	private Set<String> readOwned()
	{
		Set<String> owned = new LinkedHashSet<>();
		String saved = configManager.getConfiguration(MtaArenaConfig.GROUP, K_OWNED);
		if (saved != null && !saved.isEmpty())
		{
			for (String id : saved.split(","))
			{
				if (MtaCalculator.reward(id) != null)
				{
					owned.add(id);
				}
			}
		}
		ItemContainer[] containers = {
			client.getItemContainer(InventoryID.INVENTORY),
			client.getItemContainer(InventoryID.EQUIPMENT),
			client.getItemContainer(InventoryID.BANK),
		};
		for (MtaReward r : MtaData.REWARDS)
		{
			for (ItemContainer c : containers)
			{
				if (c != null && c.contains(r.itemId))
				{
					owned.add(r.id);
				}
			}
		}
		// The spell has no item: its unlock is a varbit. The collection log marks the four items
		// the Rewards Guardian's armour and wand give, even once the item is gone.
		if (client.getVarbitValue(VarbitID.MAGICTRAINING_BONESPEACHES) > 0)
		{
			owned.add("bones_to_peaches");
		}
		if (client.getVarbitValue(VarbitID.COLLECTION_ITEM_MAGICTRAINING_WAND_MASTER) > 0)
		{
			owned.add("master_wand");
		}
		if (client.getVarbitValue(VarbitID.COLLECTION_ITEM_MAGICTRAINING_INFINITYHAT) > 0)
		{
			owned.add("infinity_hat");
		}
		if (client.getVarbitValue(VarbitID.COLLECTION_ITEM_MAGICTRAINING_INFINITYTOP) > 0)
		{
			owned.add("infinity_top");
		}
		if (client.getVarbitValue(VarbitID.COLLECTION_ITEM_MAGICTRAINING_INFINITYBOTTOM) > 0)
		{
			owned.add("infinity_bottoms");
		}
		String joined = String.join(",", owned);
		if (!joined.equals(saved))
		{
			configManager.setConfiguration(MtaArenaConfig.GROUP, K_OWNED, joined);
		}
		return owned;
	}

	private Snapshot build(Set<String> owned)
	{
		Snapshot s = new Snapshot();
		s.owned = owned;
		int t = stored(K_TELE, -1);
		int g = stored(K_GRAVE, -1);
		int e = stored(K_ENCH, -1);
		int a = stored(K_ALCH, -1);
		s.pointsKnown = t >= 0 && g >= 0 && e >= 0 && a >= 0;
		s.held = new MtaCalculator.Points(Math.max(t, 0), Math.max(g, 0), Math.max(e, 0), Math.max(a, 0));
		s.law = carried(ItemID.LAWRUNE);
		s.nature = carried(ItemID.NATURERUNE);
		s.cosmic = carried(ItemID.COSMICRUNE);
		List<String> chosen = chosen();
		if (chosen.isEmpty())
		{
			s.message = "Pick rewards in the plugin's settings";
			return s;
		}
		try
		{
			s.result = MtaCalculator.calculate(chosen, owned, s.held,
				client.getRealSkillLevel(Skill.MAGIC), config.enchantLevel());
		}
		catch (MtaCalculator.MtaException ex)
		{
			s.message = ex.getMessage();
		}
		if (s.pointsKnown)
		{
			try
			{
				s.step = MtaPlanner.next(chosen, owned, s.held, client.getRealSkillLevel(Skill.MAGIC),
					config.enchantLevel(), playable(), config.buyRewards());
			}
			catch (MtaCalculator.MtaException ex)
			{
				s.message = ex.getMessage();
			}
		}
		return s;
	}

	/**
	 * Runes in the inventory and in the bank as the client last saw it: the bank's container while it
	 * is loaded, else the Microbot client's copy (the container is gone once the bank closes).
	 */
	private int carried(int itemId)
	{
		int n = count(client.getItemContainer(InventoryID.INVENTORY), itemId);
		ItemContainer bank = client.getItemContainer(InventoryID.BANK);
		return n + (bank != null ? count(bank, itemId) : Rs2Bank.count(itemId));
	}

	private static int count(ItemContainer c, int itemId)
	{
		int n = 0;
		if (c == null)
		{
			return 0;
		}
		for (Item item : c.getItems())
		{
			if (item.getId() == itemId)
			{
				n += item.getQuantity();
			}
		}
		return n;
	}

	/** The rooms the owner lets the plugin play. */
	private Set<MtaPlanner.Room> playable()
	{
		Set<MtaPlanner.Room> rooms = EnumSet.noneOf(MtaPlanner.Room.class);
		if (config.playTelekinetic())
		{
			rooms.add(MtaPlanner.Room.TELEKINETIC);
		}
		if (config.playAlchemist())
		{
			rooms.add(MtaPlanner.Room.ALCHEMIST);
		}
		if (config.playGraveyard())
		{
			rooms.add(MtaPlanner.Room.GRAVEYARD);
		}
		if (config.playEnchantment())
		{
			rooms.add(MtaPlanner.Room.ENCHANTMENT);
		}
		MtaRunner r = runner;
		if (r != null)
		{
			rooms.removeAll(r.blocked());
		}
		return rooms;
	}

	private List<String> chosen()
	{
		List<String> ids = new ArrayList<>();
		pick(ids, config.infinityHat(), "infinity_hat");
		pick(ids, config.infinityTop(), "infinity_top");
		pick(ids, config.infinityBottoms(), "infinity_bottoms");
		pick(ids, config.infinityGloves(), "infinity_gloves");
		pick(ids, config.infinityBoots(), "infinity_boots");
		pick(ids, config.beginnerWand(), "beginner_wand");
		pick(ids, config.apprenticeWand(), "apprentice_wand");
		pick(ids, config.teacherWand(), "teacher_wand");
		pick(ids, config.masterWand(), "master_wand");
		pick(ids, config.magesBook(), "mages_book");
		pick(ids, config.runePouch(), "rune_pouch");
		pick(ids, config.bonesToPeaches(), "bones_to_peaches");
		return ids;
	}

	private static void pick(List<String> ids, boolean on, String id)
	{
		if (on)
		{
			ids.add(id);
		}
	}
}
