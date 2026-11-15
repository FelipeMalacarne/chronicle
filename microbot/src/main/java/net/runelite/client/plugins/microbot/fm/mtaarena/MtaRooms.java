package net.runelite.client.plugins.microbot.fm.mtaarena;

import static net.runelite.client.plugins.microbot.util.Global.sleep;
import static net.runelite.client.plugins.microbot.util.Global.sleepGaussian;
import static net.runelite.client.plugins.microbot.util.Global.sleepUntil;

import java.util.Comparator;
import java.util.function.BooleanSupplier;
import net.runelite.api.Actor;
import net.runelite.api.Skill;
import net.runelite.api.coords.LocalPoint;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.util.camera.Rs2Camera;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.magic.Rs2Magic;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
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
 * maze path), so that plugin has to be on; {@link MtaRunner} makes sure it is.
 */
final class MtaRooms
{
	private static final Rs2Spells[] ENCHANT_SPELLS = {
		Rs2Spells.ENCHANT_SAPPHIRE_JEWELLERY, Rs2Spells.ENCHANT_EMERALD_JEWELLERY,
		Rs2Spells.ENCHANT_RUBY_JEWELLERY, Rs2Spells.ENCHANT_DIAMOND_JEWELLERY,
		Rs2Spells.ENCHANT_DRAGONSTONE_JEWELLERY, Rs2Spells.ENCHANT_ONYX_JEWELLERY,
	};
	private static final MagicAction[] ENCHANT = {
		MagicAction.ENCHANT_SAPPHIRE_JEWELLERY, MagicAction.ENCHANT_EMERALD_JEWELLERY,
		MagicAction.ENCHANT_RUBY_JEWELLERY, MagicAction.ENCHANT_DIAMOND_JEWELLERY,
		MagicAction.ENCHANT_DRAGONSTONE_JEWELLERY, MagicAction.ENCHANT_ONYX_JEWELLERY,
	};
	private static final int HEAL_BELOW_MIN = 40;
	private static final int HEAL_BELOW_MAX = 60;

	private final MtaArenaConfig config;
	private final BooleanSupplier running;
	private MTAPlugin core;
	private int shapesToPick = 3;
	private int nextHealBelow = Rs2Random.between(HEAL_BELOW_MIN, HEAL_BELOW_MAX);
	private Boolean peaches;

	MtaRooms(MtaArenaConfig config, BooleanSupplier running)
	{
		this.config = config;
		this.running = running;
	}

	void core(MTAPlugin core)
	{
		this.core = core;
	}

	// ---- can the room be played with what is carried? ------------------------------------

	/** Wields the best staff and says whether the room's spell can be cast. */
	boolean canPlay(MtaWorld.Room room)
	{
		switch (room)
		{
			case TELEKINETIC:
				return MtaStaves.equipFor(Rs2Spells.TELEKINETIC_GRAB);
			case ALCHEMIST:
				return MtaStaves.equipFor(Rs2Spells.HIGH_LEVEL_ALCHEMY) || MtaStaves.equipFor(Rs2Spells.LOW_LEVEL_ALCHEMY);
			case ENCHANTMENT:
				return enchantLevel() > 0 && MtaStaves.equipFor(ENCHANT_SPELLS[enchantLevel() - 1]);
			default:
				return MtaStaves.equipFor(Rs2Spells.BONES_TO_BANANAS) || MtaStaves.equipFor(Rs2Spells.BONES_TO_PEACHES);
		}
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
		Rs2ItemModel item = Rs2Inventory.getLast(best.getId());
		if (item != null)
		{
			sleep(50, 150);
			Rs2Magic.alch(item);
			return;
		}
		Rs2Inventory.dropAll(MtaWorld.ALCHEMY_ITEMS);
		AlchemyRoomTimer timer = (AlchemyRoomTimer) Microbot.getInfoBoxManager().getInfoBoxes().stream()
			.filter(x -> x instanceof AlchemyRoomTimer).findFirst().orElse(null);
		if (timer == null || Integer.parseInt(timer.getText().split(":")[1]) < 2)
		{
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

	// ---- the Creature Graveyard ----------------------------------------------------------

	void graveyard()
	{
		if (peaches == null)
		{
			peaches = Rs2Magic.canCast(MagicAction.BONES_TO_PEACHES);
		}
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
				int most = (maxHp - hp) / (peaches ? 8 : 2);
				int eat = Rs2Random.between(Math.min(2, most), Math.min(6, most));
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

	/** Returns false when the maze could not be read and the room was left to reset. */
	boolean telekinetic(Runnable leave)
	{
		MtaWorld.Maze maze = MtaWorld.Maze.at(Rs2Player.getWorldLocation());
		if (maze == null)
		{
			return true;
		}
		TelekineticRoom room = core.getTelekineticRoom();
		WorldPoint target = room.getTarget();
		if (target == null)
		{
			Rs2Walker.walkTo(maze.maze, 4);
			sleepUntil(() -> room.getTarget() != null, 10000);
			target = room.getTarget();
			if (target == null)
			{
				leave.run();
				return false;
			}
			sleep(400, 600);
		}
		final WorldPoint wanted = target;
		LocalPoint localTarget = Microbot.getClientThread().invoke(() -> LocalPoint.fromWorld(
			Microbot.getClient().getTopLevelWorldView(), wanted));
		WorldPoint targetConverted = Microbot.getClientThread().invoke(() -> WorldPoint.fromLocalInstance(
			Microbot.getClient(), localTarget));
		if (Rs2Camera.getZoom() < 40 || Rs2Camera.getZoom() > 60)
		{
			Rs2Camera.setZoom(Rs2Random.betweenInclusive(40, 60));
		}
		Rs2NpcModel guardian = new Rs2NpcModel(room.getGuardian());
		if (guardian.getWorldLocation().equals(room.getFinishLocation()))
		{
			sleepUntil(() -> room.getGuardian().getId() == 6779);
			sleep(200, 400);
			new Rs2NpcModel(room.getGuardian()).click("New-maze");
			sleepUntil(() -> {
				WorldPoint p = Rs2Player.getWorldLocation();
				return p != null && maze.area.distanceTo(p) != 0;
			});
			return true;
		}
		while (away(targetConverted) && running.getAsBoolean()
			&& !java.util.Objects.equals(Microbot.getClientThread().invoke(
				() -> Microbot.getClient().getLocalDestinationLocation()), localTarget))
		{
			if (Rs2Camera.isTileOnScreen(localTarget))
			{
				Rs2Walker.walkFastCanvas(targetConverted);
				sleepGaussian(600, 150);
			}
			else
			{
				Rs2Walker.walkTo(targetConverted);
			}
			sleepUntil(() -> !Rs2Player.isMoving());
		}
		if (Rs2Player.getWorldLocation() == null)
		{
			return true;
		}
		boolean noProjectile = Microbot.getClientThread().runOnClientThreadOptional(
			() -> java.util.stream.StreamSupport.stream(Microbot.getClient().getProjectiles().spliterator(), false)
				.noneMatch(x -> x.getId() == 143)).orElse(true);
		net.runelite.api.coords.Direction position = Microbot.getClientThread().invoke(room::getPosition);
		if (!(Rs2Player.isAnimating() || Rs2Player.isMoving() || !noProjectile || TelekineticRoom.getMoves().isEmpty()
			|| TelekineticRoom.getMoves().peek() != position || room.getGuardian().getId() == 6778
			|| room.getGuardian().getLocalLocation().equals(room.getDestination())))
		{
			Rs2Magic.cast(MagicAction.TELEKINETIC_GRAB);
			sleepGaussian(600, 150);
			if (Rs2Random.dicePercentage(50.0))
			{
				Rs2Camera.turnTo((Actor) room.getGuardian());
			}
			new Rs2NpcModel(room.getGuardian()).click();
			sleepUntil(() -> room.getGuardian().getId() != 6778);
		}
		return true;
	}

	private static boolean away(WorldPoint target)
	{
		WorldPoint here = Rs2Player.getWorldLocation();
		return here != null && !here.equals(target);
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
