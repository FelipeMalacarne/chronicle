package net.runelite.client.plugins.microbot.fm.mtaarena;

import static net.runelite.client.plugins.microbot.util.Global.sleep;
import static net.runelite.client.plugins.microbot.util.Global.sleepGaussian;
import static net.runelite.client.plugins.microbot.util.Global.sleepUntil;

import java.awt.Rectangle;
import java.util.Comparator;
import java.util.Map;
import java.util.function.BooleanSupplier;
import net.runelite.api.Actor;
import net.runelite.api.Skill;
import net.runelite.api.coords.Direction;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.util.camera.Rs2Camera;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.tile.Rs2Tile;
import net.runelite.client.plugins.microbot.util.tabs.Rs2Tab;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;
import net.runelite.client.plugins.microbot.util.widget.Rs2Widget;
import net.runelite.client.plugins.mta.MTAPlugin;
import net.runelite.client.plugins.mta.alchemy.AlchemyItem;
import net.runelite.client.plugins.mta.alchemy.AlchemyRoom;
import net.runelite.client.plugins.mta.alchemy.AlchemyRoomTimer;
import net.runelite.client.plugins.mta.telekinetic.TelekineticRoom;
import net.runelite.client.plugins.skillcalculator.skills.MagicAction;

/**
 * One short step of each room's routine, ported from the Plugin Hub's Mage Training Arena plugin.
 * The Alchemists' Playground, the Creature Graveyard and the Telekinetic Theatre lean on the
 * RuneLite Mage Training Arena plugin's solvers (the best item and cupboard, the bones counter, the
 * maze path), so that plugin has to be on; {@link MtaRunner} makes sure it is. Which spell, staff and
 * runes a room gets is {@link MtaPrep}'s; these only play.
 */
@lombok.extern.slf4j.Slf4j
final class MtaRooms
{
	private static final MagicAction[] ENCHANT = {
		MagicAction.ENCHANT_SAPPHIRE_JEWELLERY, MagicAction.ENCHANT_EMERALD_JEWELLERY,
		MagicAction.ENCHANT_RUBY_JEWELLERY, MagicAction.ENCHANT_DIAMOND_JEWELLERY,
		MagicAction.ENCHANT_DRAGONSTONE_JEWELLERY, MagicAction.ENCHANT_ONYX_JEWELLERY,
	};
	/** Coins carried before a trip to the Coin Collector: a hundred are a point. */
	private static final int COINS_PER_TRIP = 1000;
	/** The Telekinetic guardian resting, sliding, and at the finish; the grab's projectile. */
	private static final int GUARDIAN_MOVING = 6778;
	private static final int GUARDIAN_DONE = 6779;
	private static final int GRAB_PROJECTILE = 143;
	/** Pixels kept clear of the viewport's edges for a tile click. */
	private static final int VIEW_MARGIN = 12;
	/** Looks in a row with the camera unchanged before the Observe view counts as settled (~0.5 s). */
	private static final int CAMERA_STILL_LOOKS = 4;
	/** How long one runner step stays in the Telekinetic Theatre's own fast loop. */
	private static final long TELEKINETIC_STEP_MS = 6000;
	/**
	 * The Graveyard's falling bones hit about 10 hitpoints a minute: fruit is eaten below 60 to 75% of
	 * them, back up to ~90%, so a low-level account keeps a margin between two batches of fruit.
	 */
	private static final int HEAL_BELOW_MIN = 60;
	private static final int HEAL_BELOW_MAX = 75;
	private static final int HEAL_UP_TO = 90;

	private final MtaArenaConfig config;
	private final BooleanSupplier running;
	private MTAPlugin core;
	private int shapesToPick = 3;
	private int nextHealBelow = Rs2Random.between(HEAL_BELOW_MIN, HEAL_BELOW_MAX);
	/** Telekinetic tiles the game would not stand the player on, in this maze; and the last walk. */
	private final java.util.Set<MtaScene.Tile> badTiles = new java.util.HashSet<>();
	private MtaWorld.Maze badTilesMaze;
	private MtaScene.Tile lastWalk;
	/**
	 * The pull just cast and until when it counts as made: between the cast and the guardian starting
	 * to slide (the projectile's flight) the solver's plan still shows that move on top.
	 */
	private Direction inFlight;
	private long inFlightUntil;
	/** This maze's overhead view is set (the guardian's Observe, or the camera pitch after two misses). */
	private boolean observed;
	private int observeTries;

	/** The last walk was a click on the tile itself (not the minimap's way there). */
	private boolean lastWalkExact;

	MtaRooms(MtaArenaConfig config, BooleanSupplier running)
	{
		this.config = config;
		this.running = running;
	}

	void core(MTAPlugin core)
	{
		this.core = core;
	}

	int enchantLevel()
	{
		return MtaCalculator.enchantLevelFor(Microbot.getClient().getBoostedSkillLevel(Skill.MAGIC),
			config.enchantLevel());
	}

	/** What the room leaves in the inventory that has to be handed in before leaving. */
	void flush(MtaWorld.Room room)
	{
		if (room == MtaWorld.Room.ENCHANTMENT && Rs2Inventory.contains(MtaWorld.ENCHANTED_SHAPE))
		{
			if (!near(MtaWorld.ENCHANT_HOLE_SPOT, 2))
			{
				Rs2Walker.walkTo(MtaWorld.ENCHANT_HOLE_SPOT);
			}
			Microbot.getRs2TileObjectCache().query().interact(MtaWorld.ENCHANT_HOLE, "Deposit");
			Rs2Player.waitForWalking();
		}
		if (room == MtaWorld.Room.GRAVEYARD && Rs2Inventory.contains(MtaWorld.BANANAS, MtaWorld.PEACHES))
		{
			interactAt(MtaWorld.FRUIT_CHUTE, "Deposit");
			Rs2Inventory.waitForInventoryChanges(5000);
		}
		if (room == MtaWorld.Room.ALCHEMIST && Rs2Inventory.contains(MtaWorld.COINS))
		{
			depositCoins();
		}
	}

	// ---- the Enchanting Chamber ----------------------------------------------------------

	void enchant()
	{
		int level = enchantLevel();
		if (Rs2Inventory.isFull())
		{
			if (!Rs2Walker.walkTo(MtaWorld.ENCHANT_HOLE_SPOT))
			{
				return;
			}
			Microbot.getRs2TileObjectCache().query().interact(MtaWorld.ENCHANT_HOLE, "Deposit");
			Rs2Player.waitForWalking();
			return;
		}
		boolean looted = Microbot.getRs2TileItemCache().query().withId(MtaWorld.DRAGONSTONE).interact("Take")
			&& Rs2Inventory.waitForInventoryChanges(5000);
		if (looted && Rs2Inventory.emptySlotCount() > 0)
		{
			return;
		}
		MtaWorld.Shape bonus = bonusShape();
		if (bonus == null)
		{
			return;
		}
		Rs2TileObjectModel pedestal = (Rs2TileObjectModel) Microbot.getRs2TileObjectCache().query()
			.withId(bonus.pedestal).where(o -> Rs2Camera.isTileOnScreen(o.getLocalLocation())).nearest();
		if (pedestal == null)
		{
			Rs2Walker.walkTo(MtaWorld.ENCHANT_SEARCH[Rs2Random.between(0, MtaWorld.ENCHANT_SEARCH.length)]);
			Rs2Player.waitForWalking();
			return;
		}
		int itemId = Rs2Inventory.contains(MtaWorld.DRAGONSTONE) ? MtaWorld.DRAGONSTONE : bonus.itemId;
		if (Rs2Inventory.contains(MtaWorld.DRAGONSTONE) || Rs2Inventory.count(itemId) >= shapesToPick)
		{
			shapesToPick = Rs2Random.between(2, 4);
			Rs2Magic.cast(ENCHANT[level - 1]);
			sleepUntil(() -> Rs2Tab.getCurrentTab() == net.runelite.client.plugins.microbot.globval.enums.InterfaceTab.INVENTORY);
			sleepGaussian(600, 150);
			Rs2Inventory.interact(itemId);
			sleepUntil(() -> !Rs2Inventory.contains(itemId) || (itemId != MtaWorld.DRAGONSTONE && bonus != bonusShape()), 20000);
		}
		else if (pedestal.click("Take-from"))
		{
			Rs2Inventory.waitForInventoryChanges(1000);
		}
		else if (far(pedestal.getWorldLocation(), 10))
		{
			Rs2Walker.walkFastCanvas(pedestal.getWorldLocation());
		}
	}

	private static MtaWorld.Shape bonusShape()
	{
		for (MtaWorld.Shape shape : MtaWorld.Shape.values())
		{
			if (Rs2Widget.isWidgetVisible(shape.widgetGroup, shape.widgetChild))
			{
				return shape;
			}
		}
		return null;
	}

	// ---- the Alchemists' Playground ------------------------------------------------------

	void alchemist()
	{
		AlchemyRoom room = core.getAlchemyRoom();
		AlchemyItem best = room.getBest();
		if (best != null && best != AlchemyItem.EMPTY && best != AlchemyItem.UNKNOWN)
		{
			Rs2ItemModel item = Rs2Inventory.getLast(best.getId());
			if (item != null)
			{
				sleep(50, 150);
				Rs2Magic.alch(item);
				return;
			}
		}
		Rs2Inventory.dropAll(MtaWorld.ALCHEMY_ITEMS);
		// Coins only become points in the Coin Collector.
		if (Rs2Inventory.itemQuantity(MtaWorld.COINS) >= COINS_PER_TRIP)
		{
			depositCoins();
			return;
		}
		AlchemyRoomTimer timer = (AlchemyRoomTimer) Microbot.getInfoBoxManager().getInfoBoxes().stream()
			.filter(x -> x instanceof AlchemyRoomTimer).findFirst().orElse(null);
		if (timer == null || secondsOf(timer.getText()) < 2)
		{
			// The items move between cupboards when the timer runs out: wait in the middle.
			Rs2Walker.walkTo(MtaWorld.ALCHEMY_CENTER.getX(), MtaWorld.ALCHEMY_CENTER.getY(),
				MtaWorld.ALCHEMY_CENTER.getPlane(), 2);
			return;
		}
		if (room.getSuggestion() == null)
		{
			Microbot.getClientThread().invoke(() -> Microbot.getRs2TileObjectCache().query().withName("Cupboard")
				.interact("Search"));
			sleep(300, 600);
			if (net.runelite.client.plugins.microbot.util.Global.sleepUntilTrue(Rs2Player::isMoving, 100, 1000))
			{
				sleepUntil(() -> !Rs2Player.isMoving());
			}
		}
		else
		{
			new Rs2TileObjectModel(room.getSuggestion().getGameObject()).click("Take-5");
			Rs2Inventory.waitForInventoryChanges(3000);
			sleep(300, 600);
		}
	}

	private static void depositCoins()
	{
		int coins = Rs2Inventory.itemQuantity(MtaWorld.COINS);
		if (Microbot.getRs2TileObjectCache().query().withName(MtaWorld.COIN_COLLECTOR).interact("Deposit"))
		{
			boolean done = sleepUntil(() -> !Rs2Inventory.contains(MtaWorld.COINS), 10000);
			log.info("alchemist: {} coins to the Coin Collector{}", coins, done ? "" : " (not taken yet)");
		}
	}

	/** "m:ss" of the room's timer, in seconds; 0 when it cannot be read. */
	private static int secondsOf(String text)
	{
		try
		{
			String[] parts = text.split(":");
			return Integer.parseInt(parts[0].trim()) * 60 + Integer.parseInt(parts[1].trim());
		}
		catch (RuntimeException ex)
		{
			return 0;
		}
	}

	// ---- the Creature Graveyard ----------------------------------------------------------

	/** {@code peaches}: the runner's plan casts Bones to Peaches (owned and Magic 60), else Bananas. */
	void graveyard(boolean peaches)
	{
		long boneGoal = 28L - Rs2Inventory.items().filter(x -> x.getName().equalsIgnoreCase(MtaWorld.BONES)).count();
		if (core.getGraveyardRoom().getCounter() != null && core.getGraveyardRoom().getCounter().getCount() >= boneGoal)
		{
			Rs2Magic.cast(peaches ? MagicAction.BONES_TO_PEACHES : MagicAction.BONES_TO_BANANAS);
			Rs2Player.waitForAnimation();
			if (!Rs2Tab.isCurrentTab(net.runelite.client.plugins.microbot.globval.enums.InterfaceTab.INVENTORY))
			{
				sleep(500, 1000);
				Rs2Tab.switchTo(net.runelite.client.plugins.microbot.globval.enums.InterfaceTab.INVENTORY);
				sleep(500, 1000);
			}
			return;
		}
		if (Rs2Inventory.contains(MtaWorld.BANANAS, MtaWorld.PEACHES))
		{
			int maxHp = Microbot.getClient().getRealSkillLevel(Skill.HITPOINTS);
			int hp = Microbot.getClient().getBoostedSkillLevel(Skill.HITPOINTS);
			if (hp * 100 / maxHp < nextHealBelow)
			{
				int fruit = peaches ? MtaWorld.PEACHES : MtaWorld.BANANAS;
				int heals = peaches ? 8 : 2;
				int eat = Math.max(1, (maxHp * HEAL_UP_TO / 100 - hp + heals - 1) / heals);
				eat = Math.min(eat, Rs2Inventory.count(fruit));
				nextHealBelow = Rs2Random.between(HEAL_BELOW_MIN, HEAL_BELOW_MAX);
				for (int i = 0; i < eat; i++)
				{
					Rs2Inventory.interact(fruit, "eat");
					if (i < eat - 1)
					{
						sleepGaussian(1400, 350);
					}
				}
			}
			if (Rs2Inventory.contains(MtaWorld.BANANAS, MtaWorld.PEACHES))
			{
				interactAt(MtaWorld.FRUIT_CHUTE, "Deposit");
				Rs2Inventory.waitForInventoryChanges(5000);
			}
			return;
		}
		if (core.getGraveyardRoom().getCounter() == null)
		{
			interactAt(MtaWorld.BONES_PILE, "Grab");
			sleepUntil(() -> !Rs2Player.isMoving() && Rs2Inventory.waitForInventoryChanges(2000));
		}
		while (core.getGraveyardRoom().getCounter() != null && core.getGraveyardRoom().getCounter().getCount() < boneGoal
			&& running.getAsBoolean())
		{
			interactAt(MtaWorld.BONES_PILE, "Grab");
			Rs2Inventory.waitForInventoryChanges(5000);
			boneGoal = Rs2Inventory.items().filter(x -> x.getName().equalsIgnoreCase(MtaWorld.BONES)).count()
				+ Rs2Inventory.emptySlotCount();
			sleepGaussian(400, 150);
		}
	}

	// ---- the Telekinetic Theatre ---------------------------------------------------------

	/**
	 * One step of the Telekinetic Theatre, polled fast by the runner. Off the side the next pull needs,
	 * walk there (also while the guardian still slides: the solver plans from where it stops); on it,
	 * have the spell selected and click the guardian in the tick it comes to rest. Returns false when
	 * the maze could not be read and the room was left to reset.
	 */
	boolean telekinetic(Runnable leave)
	{
		// Several reactions per runner step, so a pull lands in the tick the guardian rests without
		// the runner looping fast (see MtaRunner.LOOP_MS).
		long until = System.currentTimeMillis() + TELEKINETIC_STEP_MS;
		boolean ok = true;
		while (ok && running.getAsBoolean() && System.currentTimeMillis() < until
			&& !net.runelite.client.plugins.microbot.util.input.InputArbiter.isHuman())
		{
			ok = telekineticOnce(leave);
		}
		return ok;
	}

	private boolean telekineticOnce(Runnable leave)
	{
		MtaWorld.Maze maze = MtaWorld.Maze.at(Rs2Player.getWorldLocation());
		if (maze == null)
		{
			return false;
		}
		TelekineticRoom room = core.getTelekineticRoom();
		if (room.getGuardian() == null)
		{
			// A new maze: the guardian is only sent once the player is near it (the entrance is too far),
			// so head for the maze until it shows. Never leave the Theatre over it: leaving breaks the
			// five-maze streak and its bonus.
			walkTowardMaze(maze);
			if (!sleepUntil(() -> room.getGuardian() != null, 3000))
			{
				log.info("telekinetic: maze {} guardian not in sight yet", maze);
			}
			return true;
		}
		if (maze != badTilesMaze)
		{
			badTiles.clear();
			badTilesMaze = maze;
			lastWalk = null;
			lastWalkExact = false;
			inFlight = null;
			if (Rs2Camera.getZoom() < 40 || Rs2Camera.getZoom() > 60)
			{
				Rs2Camera.setZoom(Rs2Random.betweenInclusive(40, 60));
			}
			observed = false;
			observeTries = 0;
		}
		if (new Rs2NpcModel(room.getGuardian()).getWorldLocation().equals(room.getFinishLocation()))
		{
			sleepUntil(() -> room.getGuardian() != null && room.getGuardian().getId() == GUARDIAN_DONE);
			sleep(150, 300);
			new Rs2NpcModel(room.getGuardian()).click("New-maze");
			sleepUntil(() -> {
				WorldPoint p = Rs2Player.getWorldLocation();
				return p != null && maze.area.distanceTo(p) != 0;
			});
			return true;
		}
		if (!observed)
		{
			observe(room);
		}
		Direction next = planned(room, 0);
		if (next == null)
		{
			// No path yet (the guardian has not been seen moving or the walls are still coming in).
			sleepUntil(() -> !TelekineticRoom.getMoves().isEmpty(), 3000);
			return true;
		}
		Direction side = Microbot.getClientThread().invoke(room::getPosition);
		if (side != next)
		{
			walkToSide(maze, next, planned(room, 1));
			selectGrab();
			return true;
		}
		// On the right side: the spell selected, the pull is a single click when the guardian rests.
		selectGrab();
		// Standing still too: a pull cast while walking past the side can land with the player in a
		// corner ("The statue won't move - don't stand at the corners"), a law rune for nothing.
		if (!sleepUntil(() -> resting(room) && !Rs2Player.isMoving(), 4000))
		{
			log.debug("telekinetic: guardian not resting after 4s (id {})", room.getGuardian() == null ? null : room.getGuardian().getId());
			return true;
		}
		// Read the plan again after the wait: a direction read before it may already be spent, and pulling
		// on it again is a wasted cast on the same tile. The same once the guardian reached the finish.
		Direction now = planned(room, 0);
		if (now != next || Microbot.getClientThread().invoke(room::getPosition) != now
			|| new Rs2NpcModel(room.getGuardian()).getWorldLocation().equals(room.getFinishLocation()))
		{
			return true;
		}
		Direction after = planned(room, 1);
		Direction afterThat = planned(room, 2);
		log.debug("telekinetic: pull ({}), then {}", next, after);
		new Rs2NpcModel(room.getGuardian()).click();
		// Once the grab is cast (its projectile out, or the guardian already sliding) the walk does not
		// cancel it: head for the side of the move after right away, as the plan already says.
		if (sleepUntil(() -> room.getGuardian() == null || !resting(room), 2400))
		{
			inFlight = next;
			inFlightUntil = System.currentTimeMillis() + 3000;
			if (after != null)
			{
				walkToSide(maze, after, afterThat);
				selectGrab();
			}
		}
		return true;
	}

	/**
	 * The guardian's Observe: an overhead view centred on the maze, kept while walking and casting once
	 * the camera has got there (a click during its move cancels it, so nothing else happens until it
	 * rests). Offered only with no spell selected and the guardian at rest; after two misses the camera's
	 * own pitch stands in.
	 */
	private void observe(TelekineticRoom room)
	{
		if (room.getGuardian() == null || room.getGuardian().getId() == GUARDIAN_MOVING || spellSelected())
		{
			if (++observeTries > 20)
			{
				observed = true;
				Rs2Camera.adjustPitch(1.0f);
			}
			return;
		}
		int[] before = cameraNow();
		boolean ok = new Rs2NpcModel(room.getGuardian()).click("Observe");
		log.info("telekinetic: observe {}", ok ? "clicked" : "missed");
		if (ok)
		{
			// The view starts moving a tick or so after the click: first see it leave, then see it rest
			// (unchanged over several looks), and only then walk or cast, or the move is cancelled.
			boolean moved = sleepUntil(() -> !java.util.Arrays.equals(cameraNow(), before), 2500);
			int[] last = cameraNow();
			int still = 0;
			long until = System.currentTimeMillis() + 6000;
			while (moved && still < CAMERA_STILL_LOOKS && System.currentTimeMillis() < until)
			{
				sleep(110, 130);
				int[] now = cameraNow();
				still = java.util.Arrays.equals(now, last) ? still + 1 : 0;
				last = now;
			}
			log.info("telekinetic: observe {}", moved ? "settled" : "did not move the camera");
			observed = true;
		}
		else if (++observeTries >= 2)
		{
			observed = true;
			Rs2Camera.adjustPitch(1.0f);
		}
	}

	/** Camera position, pitch and yaw. */
	private static int[] cameraNow()
	{
		return Microbot.getClientThread().runOnClientThreadOptional(() -> {
			net.runelite.api.Client c = Microbot.getClient();
			return new int[]{c.getCameraX(), c.getCameraY(), c.getCameraZ(), c.getCameraPitch(), c.getCameraYaw()};
		}).orElse(new int[0]);
	}

	/**
	 * The plan's move {@code i} steps ahead (0: the next pull), or null. A pull just cast still sits on
	 * top of the solver's plan for a tick or two: it is skipped.
	 */
	private Direction planned(TelekineticRoom room, int i)
	{
		return Microbot.getClientThread().invoke(() -> {
			java.util.Stack<Direction> moves = TelekineticRoom.getMoves();
			int skip = 0;
			// The same move twice in a row cannot happen (the guardian slides until a wall), so the pull
			// just made on top is the solver not having rebuilt its plan yet (it does a tick after the
			// slide starts).
			if (inFlight != null && System.currentTimeMillis() < inFlightUntil && !moves.isEmpty()
				&& moves.peek() == inFlight)
			{
				skip = 1;
			}
			int k = i + skip;
			return moves.size() > k ? moves.get(moves.size() - 1 - k) : null;
		});
	}

	/** Telekinetic Grab selected, so pulling is one click on the guardian. */
	private static void selectGrab()
	{
		if (spellSelected())
		{
			return;
		}
		Rs2Magic.cast(MagicAction.TELEKINETIC_GRAB);
		sleepUntil(MtaRooms::spellSelected, 1200);
	}

	private static boolean spellSelected()
	{
		return Microbot.getClientThread().runOnClientThreadOptional(() -> Microbot.getClient().isWidgetSelected())
			.orElse(false);
	}

	/** The guardian stands still and no grab is in flight: it can be pulled. */
	private static boolean resting(TelekineticRoom room)
	{
		return Microbot.getClientThread().runOnClientThreadOptional(() -> {
			if (room.getGuardian() == null || room.getGuardian().getId() == GUARDIAN_MOVING)
			{
				return false;
			}
			return java.util.stream.StreamSupport.stream(Microbot.getClient().getProjectiles().spliterator(), false)
				.noneMatch(x -> x.getId() == GRAB_PROJECTILE);
		}).orElse(false);
	}

	/**
	 * Towards the middle of the maze's walls (loaded with the scene, unlike its guardian): the reachable
	 * tile closest to it. Before the walls are read, the maze's viewpoint.
	 */
	private void walkTowardMaze(MtaWorld.Maze maze)
	{
		if (Rs2Player.isMoving())
		{
			return;
		}
		Rectangle box = mazeBounds();
		if (box == null || box.width == 0)
		{
			Rs2Walker.walkTo(maze.maze, 6);
			return;
		}
		Object[] picked = Microbot.getClientThread().invoke(() -> {
			net.runelite.api.Client client = Microbot.getClient();
			net.runelite.api.WorldView wv = client.getTopLevelWorldView();
			net.runelite.api.coords.LocalPoint me = client.getLocalPlayer().getLocalLocation();
			MtaScene.Tile here = new MtaScene.Tile(me.getSceneX(), me.getSceneY());
			int[][] flags = wv.getCollisionMaps()[wv.getPlane()].getFlags();
			int[][] dist = MtaScene.steps(flags, here);
			MtaScene.Tile t = MtaScene.closest(dist, wv.getBaseX(), wv.getBaseY(), (int) box.getCenterX(),
				(int) box.getCenterY());
			return new Object[]{here, t, wv.getBaseX(), wv.getBaseY()};
		});
		MtaScene.Tile here = (MtaScene.Tile) picked[0];
		MtaScene.Tile tile = (MtaScene.Tile) picked[1];
		log.debug("telekinetic: maze {} no guardian in sight: box {} base {},{} at {} -> {}", maze, box, picked[2],
			picked[3], here, tile);
		if (tile == null || tile.equals(here) || tile.equals(lastApproach))
		{
			// Nothing nearer the maze to click (or the last click did not move the player): the walker
			// knows the way to the maze's viewpoint.
			lastApproach = null;
			Rs2Walker.walkTo(maze.maze, 6);
			return;
		}
		lastApproach = tile;
		walkScene(tile);
	}

	/** The last tile asked for on the way to a maze without its guardian in sight. */
	private MtaScene.Tile lastApproach;

	/** A walk to a scene tile: a click on it when drawn on screen, else the minimap's way there. */
	private boolean walkScene(MtaScene.Tile tile)
	{
		net.runelite.api.coords.LocalPoint target = net.runelite.api.coords.LocalPoint.fromScene(tile.x, tile.y,
			Microbot.getClient().getTopLevelWorldView());
		boolean exact = Rs2Camera.isTileOnScreen(target) && clickTile(target);
		if (!exact)
		{
			WorldPoint far = Microbot.getClientThread().invoke(
				() -> WorldPoint.fromLocalInstance(Microbot.getClient(), target));
			Rs2Walker.walkMiniMap(far);
		}
		sleepUntil(Rs2Player::isMoving, 900);
		return exact;
	}

	/**
	 * Walks to the best tile on {@code side} ({@link MtaScene#best}), worked out and clicked in scene
	 * coordinates. A tile asked for twice that the game stood the player beside is left out for the maze.
	 */
	private void walkToSide(MtaWorld.Maze maze, Direction side, Direction after)
	{
		Rectangle box = mazeBounds();
		if (box == null)
		{
			return;
		}
		Object[] picked = Microbot.getClientThread().invoke(() -> {
			net.runelite.api.Client client = Microbot.getClient();
			net.runelite.api.WorldView wv = client.getTopLevelWorldView();
			net.runelite.api.coords.LocalPoint me = client.getLocalPlayer().getLocalLocation();
			MtaScene.Tile here = new MtaScene.Tile(me.getSceneX(), me.getSceneY());
			int[][] flags = wv.getCollisionMaps()[wv.getPlane()].getFlags();
			int[] aim = MtaScene.highlight(wv.getBaseX() + here.x, wv.getBaseY() + here.y, box, side, after);
			int[][] dist = MtaScene.steps(flags, here);
			MtaScene.Tile tile = MtaScene.best(dist, wv.getBaseX(), wv.getBaseY(), box, side, aim, badTiles);
			if (tile != null && lastWalkExact && tile.equals(lastWalk) && !tile.equals(here) && !Rs2Player.isMoving())
			{
				// Asked for this tile last time and the game stood the player elsewhere: not standable.
				badTiles.add(tile);
				tile = MtaScene.best(dist, wv.getBaseX(), wv.getBaseY(), box, side, aim, badTiles);
			}
			return new Object[]{here, tile, new MtaScene.Tile(aim[0] - wv.getBaseX(), aim[1] - wv.getBaseY())};
		});
		MtaScene.Tile here = (MtaScene.Tile) picked[0];
		MtaScene.Tile tile = (MtaScene.Tile) picked[1];
		if (tile == null)
		{
			log.info("telekinetic: maze {} no reachable tile on the {} side", maze, side);
			sleep(600, 900);
			return;
		}
		if (tile.equals(lastWalk) && Rs2Player.isMoving())
		{
			return;
		}
		log.debug("telekinetic: maze {} next {} from {} walk to {} (highlight {})", maze, side, here, tile, picked[2]);
		lastWalk = tile;
		// A tile drawn on screen gets a click on it; a far one goes by the minimap, the exact click after.
		lastWalkExact = walkScene(tile);
	}

	/**
	 * "Walk here" on a scene tile: the menu entry at the tile's canvas point, clicked quick. Not
	 * Rs2Walker.walkFastLocal (a box at (1, 1), the client's fallback, sends the mouse to the canvas
	 * corner); and not the human-like mouse path, which never settles on a point the mouse cannot reach
	 * and holds the mouse lock, every script's clicks then waiting until the client restarts.
	 */
	private static boolean clickTile(net.runelite.api.coords.LocalPoint local)
	{
		net.runelite.api.Point p = Microbot.getClientThread().runOnClientThreadOptional(() -> {
			net.runelite.api.Client c = Microbot.getClient();
			net.runelite.api.Point at = net.runelite.api.Perspective.localToCanvas(c, local,
				c.getTopLevelWorldView().getPlane());
			// Only a point well inside the game's viewport: one outside it cannot be reached by the mouse.
			Rectangle view = new Rectangle(c.getViewportXOffset() + VIEW_MARGIN, c.getViewportYOffset() + VIEW_MARGIN,
				c.getViewportWidth() - 2 * VIEW_MARGIN, c.getViewportHeight() - 2 * VIEW_MARGIN);
			return at != null && view.contains(at.getX(), at.getY()) ? at : null;
		}).orElse(null);
		if (p == null)
		{
			return false;
		}
		net.runelite.client.plugins.microbot.util.menu.NewMenuEntry entry =
			new net.runelite.client.plugins.microbot.util.menu.NewMenuEntry()
				.param0(p.getX())
				.param1(p.getY())
				.type(net.runelite.api.MenuAction.WALK)
				.identifier(0)
				.itemId(0)
				.option("Walk here");
		// The quick click: no human-like mouse path to the point, which is where the client's mouse can
		// spin forever holding the mouse lock (seen twice in a session).
		Microbot.doInvokeQuick(entry, new Rectangle(p.getX() - 2, p.getY() - 2, 4, 4));
		return true;
	}

	/** The maze's walls' box, kept by the solver in a private field. */
	private static Rectangle mazeBounds()
	{
		try
		{
			java.lang.reflect.Field f = TelekineticRoom.class.getDeclaredField("bounds");
			f.setAccessible(true);
			return (Rectangle) f.get(null);
		}
		catch (ReflectiveOperationException | RuntimeException ex)
		{
			log.warn("telekinetic: cannot read the maze bounds", ex);
			return null;
		}
	}

	// ---- helpers -------------------------------------------------------------------------

	static boolean interactAt(WorldPoint where, String action)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		if (here == null)
		{
			return false;
		}
		Rs2TileObjectModel object = Microbot.getRs2TileObjectCache().query().fromWorldView()
			.where(o -> where.equals(o.getTileObject().getWorldLocation())).toList().stream()
			.min(Comparator.comparingInt(o -> o.getTileObject().getWorldLocation().distanceTo(here)))
			.orElse(null);
		return object != null && object.click(action);
	}

	private static boolean near(WorldPoint p, int distance)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		return here != null && here.getPlane() == p.getPlane() && here.distanceTo(p) <= distance;
	}

	private static boolean far(WorldPoint p, int distance)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		return here != null && here.distanceTo(p) > distance;
	}
}
