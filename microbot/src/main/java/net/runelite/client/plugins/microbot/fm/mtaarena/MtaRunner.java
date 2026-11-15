package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Skill;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.widgets.Widget;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.Script;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaPlanner.Step;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.dialogues.Rs2Dialogue;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.keyboard.Rs2Keyboard;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spellbook;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;
import net.runelite.client.plugins.microbot.util.widget.Rs2Widget;
import net.runelite.client.plugins.mta.MTAPlugin;

/**
 * Runs the plan: asks {@link MtaPlanner} what is next, buys at the Rewards Guardian or earns the
 * missing points in a room ({@link MtaRooms}), and asks again. Acts only while the plugin's "Run the
 * plan" setting is on. One short step per loop: the planner reads the points each time, so a
 * purchase or a level-up needs no special case, and a room is left as soon as the plan no longer
 * needs its points.
 */
@Slf4j
final class MtaRunner extends Script
{
	/**
	 * One step per loop. Script.run() is called once per loop and must not be called much faster: it
	 * writes settings and drives the client's blocking events, which then hold every script. The
	 * Telekinetic Theatre keeps its tick precision inside its own step ({@link MtaRooms#telekinetic}).
	 */
	private static final int LOOP_MS = 600;
	/** Give up on a purchase that did not take after this many tries and say so. */
	private static final int BUY_TRIES = 3;

	private final MtaArenaPlugin plugin;
	private final MtaArenaConfig config;
	private final MtaRooms rooms;
	private final MtaPrep prep;
	/** Rooms whose spell's runes are neither carried nor banked: the planner leaves them to the owner. */
	private final Set<MtaPlanner.Room> blocked = ConcurrentHashMap.newKeySet();
	private volatile String state = "";
	private MTAPlugin core;
	private String boughtTry = "";
	private int tries;
	private boolean firstVisit;
	/** The last room seen and when: the Telekinetic Theatre between two mazes shows none. */
	private MtaWorld.Room lastRoom;
	private long lastRoomAt;
	private static final long THEATRE_GRACE_MS = 8000;


	MtaRunner(MtaArenaPlugin plugin, MtaArenaConfig config)
	{
		this.plugin = plugin;
		this.config = config;
		this.rooms = new MtaRooms(config, () -> isRunning() && config.automate());
		this.prep = new MtaPrep(config);
	}

	/** What the runner is doing, for the overlay; empty while it is off. */
	String state()
	{
		return config.automate() ? state : "";
	}

	Set<MtaPlanner.Room> blocked()
	{
		return blocked;
	}

	public boolean run()
	{
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				loop();
			}
			catch (Exception ex)
			{
				log.error("MTA runner loop failed", ex);
			}
		}, 0, LOOP_MS, TimeUnit.MILLISECONDS);
		return true;
	}

	private void status(String text)
	{
		if (!text.equals(state))
		{
			log.info("MTA: {}", text);
		}
		state = text;
		Microbot.status = "MTA: " + text;
	}

	private void loop()
	{
		if (!config.automate())
		{
			// Switching the run off and on again is how the owner says the runes are back.
			blocked.clear();
			return;
		}
		if (!Microbot.isLoggedIn())
		{
			return;
		}
		if (!super.run())
		{
			idleReason();
			return;
		}
		if (!coreReady())
		{
			return;
		}
		if (!Rs2Magic.isSpellbook(Rs2Spellbook.MODERN))
		{
			status("use the standard spellbook");
			return;
		}
		if (firstVisit)
		{
			talkToEntranceGuardian();
			return;
		}
		MtaArenaPlugin.Snapshot s = plugin.snapshot();
		if (!s.pointsKnown)
		{
			status("open the points panel once so the plugin knows your points");
			return;
		}
		Step step = s.step;
		if (step == null)
		{
			status(s.message == null ? "nothing chosen" : s.message);
			return;
		}
		MtaWorld.Room at = roomNow();
		boolean needsHere = at != null && step.kind == MtaPlanner.Kind.EARN && MtaWorld.Room.of(step.room) == at;
		boolean byHand = at != null && step.kind == MtaPlanner.Kind.MANUAL && MtaWorld.Room.of(step.room) == at;
		if (byHand)
		{
			status("you are playing the " + at.name().toLowerCase() + " room: " + step.points + " points to go");
			return;
		}
		if (at != null && !needsHere)
		{
			status("done in the " + at.name().toLowerCase() + " room");
			rooms.flush(at);
			leaveRoom();
			return;
		}
		switch (step.kind)
		{
			case DONE:
				status("every chosen reward is yours");
				return;
			case BLOCKED:
				status("Magic is too low for the Enchanting Chamber: " + step.reward + " waits");
				return;
			case MANUAL:
				status("play the " + step.room.name().toLowerCase() + " room by hand (" + step.points
					+ " points for " + step.reward + ")");
				return;
			case BUY:
				buy(step.reward);
				return;
			case EARN:
				earn(MtaWorld.Room.of(step.room), step, s);
				return;
			default:
				return;
		}
	}

	private long idleLoggedAt;

	/** Why Script.run() holds the loop (the owner's input, a pause, a blocking event), at most every 5 s. */
	private void idleReason()
	{
		long now = System.currentTimeMillis();
		if (now - idleLoggedAt < 5000)
		{
			return;
		}
		idleLoggedAt = now;
		String why = net.runelite.client.plugins.microbot.util.input.InputArbiter.isHuman() ? "your mouse or keys in the client"
			: Microbot.pauseAllScripts.get() ? "scripts paused" : "a blocking event";
		status("waiting: " + why);
	}

	/** The RuneLite Mage Training Arena plugin has the solvers the rooms use: it must be on. */
	private boolean coreReady()
	{
		if (Microbot.getPluginManager() == null)
		{
			return false;
		}
		if (core == null)
		{
			for (Plugin p : Microbot.getPluginManager().getPlugins())
			{
				if (p instanceof MTAPlugin)
				{
					core = (MTAPlugin) p;
					rooms.core(core);
				}
			}
			if (core == null)
			{
				status("the RuneLite Mage Training Arena plugin was not found");
				return false;
			}
		}
		if (!Microbot.getPluginManager().isActive(core))
		{
			status("turning on the RuneLite Mage Training Arena plugin");
			try
			{
				Microbot.getPluginManager().setPluginEnabled(core, true);
				Microbot.getPluginManager().startPlugin(core);
			}
			catch (Exception ex)
			{
				log.warn("could not start the MTA plugin", ex);
			}
			return false;
		}
		return true;
	}

	// ---- the Rewards Guardian ------------------------------------------------------------

	private void buy(String id)
	{
		MtaReward r = MtaCalculator.reward(id);
		if (!id.equals(boughtTry))
		{
			boughtTry = id;
			tries = 0;
		}
		if (tries >= BUY_TRIES)
		{
			status("could not buy " + r.name + " after " + BUY_TRIES + " tries: do it by hand");
			return;
		}
		if (r.requires != null && !Rs2Inventory.contains(MtaCalculator.reward(r.requires).itemId))
		{
			status("bring the " + MtaCalculator.reward(r.requires).name + " to the shop to upgrade it");
			return;
		}
		if (!near(MtaWorld.SHOP, 3) && !Rs2Walker.walkTo(MtaWorld.SHOP))
		{
			status("walking to the Rewards Guardian");
			return;
		}
		if (!Rs2Widget.isWidgetVisible(MtaWorld.SHOP_GROUP, 0))
		{
			Microbot.getRs2NpcCache().query().withId(MtaWorld.REWARDS_GUARDIAN).interact("Trade-with");
			sleepUntil(() -> Rs2Widget.isWidgetVisible(MtaWorld.SHOP_GROUP, 0), 5000);
			sleepGaussian(600, 150);
			return;
		}
		Widget list = Rs2Widget.getWidget(MtaWorld.SHOP_GROUP, MtaWorld.SHOP_ITEMS);
		Widget[] items = list == null ? null : list.getDynamicChildren();
		int index = findItem(items, r.itemId);
		if (index < 0)
		{
			status(r.name + " is not in the shop's list (the item id may be wrong)");
			tries = BUY_TRIES;
			return;
		}
		status("buying " + r.name);
		tries++;
		Rs2Inventory.waitForInventoryChanges(() -> {
			Rs2Widget.clickWidgetFast(items[index], index);
			sleepGaussian(600, 150);
			Rs2Widget.clickWidget(MtaWorld.SHOP_GROUP, MtaWorld.SHOP_BUY);
			sleepGaussian(600, 150);
			Rs2Keyboard.keyPress(27);
		});
	}

	private static int findItem(Widget[] items, int itemId)
	{
		if (items == null)
		{
			return -1;
		}
		for (int i = 0; i < items.length; i++)
		{
			if (items[i] != null && items[i].getItemId() == itemId)
			{
				return i;
			}
		}
		return -1;
	}

	// ---- rooms ---------------------------------------------------------------------------

	private void earn(MtaWorld.Room room, Step step, MtaArenaPlugin.Snapshot s)
	{
		int magic = Microbot.getClient().getBoostedSkillLevel(Skill.MAGIC);
		Rs2Spells spell = MtaPrep.spell(room, magic, rooms.enchantLevel(), s.owned.contains("bones_to_peaches"));
		if (spell == null)
		{
			status("no Enchant castable");
			return;
		}
		String where = room.name().toLowerCase();
		MtaWorld.Room at = roomNow();
		if (at != room)
		{
			// In the lobby: the room's setup first, at the bank chest upstairs.
			MtaPrep.Plan plan = prep.plan(room, spell);
			switch (prep.step(plan))
			{
				case MISSING:
					blocked.add(step.room);
					status("no " + String.join(", ", plan.missing()) + " runes for the " + where
						+ " room: it is left to you until the run is switched off and on");
					Rs2Bank.closeBank();
					return;
				case WORKING:
					status("setting up for the " + where + " room" + gear(plan));
					return;
				default:
					enterRoom(room);
					return;
			}
		}
		if (!net.runelite.client.plugins.microbot.util.magic.Rs2Magic.canCast(spell))
		{
			status("out of runes for the " + where + " room: back to the bank");
			rooms.flush(room);
			leaveRoom();
			return;
		}
		status(where + ": " + step.points + " points to go"
			+ (step.ahead ? " for the rest of the plan (" + step.reward + " waits for you)" : " for " + step.reward));
		switch (room)
		{
			case ENCHANTMENT:
				rooms.enchant();
				break;
			case ALCHEMIST:
				rooms.alchemist();
				break;
			case GRAVEYARD:
				rooms.graveyard(spell == Rs2Spells.BONES_TO_PEACHES);
				break;
			default:
				rooms.telekinetic(this::leaveRoom);
				break;
		}
	}

	private static String gear(MtaPrep.Plan plan)
	{
		StringBuilder b = new StringBuilder();
		if (plan.choice.staff != null)
		{
			b.append(" (staff ").append(plan.choice.staff.itemId);
		}
		if (plan.choice.tome != null)
		{
			b.append(b.length() == 0 ? " (" : ", ").append("tome ").append(plan.choice.tome.itemId);
		}
		return b.length() == 0 ? "" : b.append(")").toString();
	}

	private void enterRoom(MtaWorld.Room room)
	{
		status("entering the " + room.name().toLowerCase() + " room");
		if (!near(MtaWorld.LOBBY, 4) && !Rs2Walker.walkTo(MtaWorld.LOBBY))
		{
			return;
		}
		Microbot.getRs2TileObjectCache().query().interact(room.teleporter, "Enter");
		Rs2Player.waitForAnimation();
		if (Rs2Widget.hasWidget(MtaWorld.FIRST_VISIT_TEXT))
		{
			firstVisit = true;
		}
	}

	private void leaveRoom()
	{
		MtaWorld.Room at = roomNow();
		if (at == null)
		{
			return;
		}
		WorldPoint exit = at.exit;
		if (at == MtaWorld.Room.TELEKINETIC)
		{
			MtaWorld.Maze maze = MtaWorld.Maze.at(Rs2Player.getWorldLocation());
			if (maze == null)
			{
				return;
			}
			exit = maze.exit;
		}
		if (!Rs2Walker.walkTo(exit))
		{
			return;
		}
		Microbot.getRs2TileObjectCache().query().interact(MtaWorld.EXIT_PORTAL, "Enter");
		Rs2Player.waitForWalking();
	}

	/** The first visit: the Entrance Guardian has to be spoken to once. */
	private void talkToEntranceGuardian()
	{
		status("talking to the Entrance Guardian (first visit)");
		if (!near(MtaWorld.ENTRANCE_GUARDIAN_SPOT, 3) && !Rs2Walker.walkTo(MtaWorld.ENTRANCE_GUARDIAN_SPOT))
		{
			return;
		}
		if (!Rs2Dialogue.isInDialogue())
		{
			Microbot.getRs2NpcCache().query().withId(MtaWorld.ENTRANCE_GUARDIAN).interact("Talk-to");
		}
		else if (Rs2Dialogue.hasSelectAnOption() && Rs2Widget.hasWidget("I'm new to this place"))
		{
			Rs2Widget.clickWidget("I'm new to this place");
		}
		else if (Rs2Dialogue.hasSelectAnOption() && Rs2Widget.hasWidget("Thanks, bye!"))
		{
			Rs2Widget.clickWidget("Thanks, bye!");
			firstVisit = false;
		}
		else
		{
			Rs2Dialogue.clickContinue();
		}
	}

	/**
	 * The room the player is in. Between two mazes of the Telekinetic Theatre the player stands
	 * outside every maze's area, so the Theatre is also recognised by its interface (the solver's inside()).
	 */
	private MtaWorld.Room roomNow()
	{
		MtaWorld.Room at = roomSeen();
		long now = System.currentTimeMillis();
		if (at != null)
		{
			lastRoom = at;
			lastRoomAt = now;
			return at;
		}
		// Loading the next maze, the Theatre's interface is gone for a moment: still in the Theatre.
		if (lastRoom == MtaWorld.Room.TELEKINETIC && now - lastRoomAt < THEATRE_GRACE_MS)
		{
			return MtaWorld.Room.TELEKINETIC;
		}
		return null;
	}

	private MtaWorld.Room roomSeen()
	{
		MtaWorld.Room at = MtaWorld.Room.at(Rs2Player.getWorldLocation());
		if (at == null && core != null)
		{
			boolean theatre = Microbot.getClientThread().runOnClientThreadOptional(
				() -> core.getTelekineticRoom().inside()).orElse(false);
			if (theatre)
			{
				return MtaWorld.Room.TELEKINETIC;
			}
		}
		return at;
	}

	private static boolean near(WorldPoint p, int distance)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		return here != null && here.getPlane() == p.getPlane() && here.distanceTo(p) <= distance;
	}
}
