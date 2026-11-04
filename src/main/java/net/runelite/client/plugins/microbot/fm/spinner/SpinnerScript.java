package net.runelite.client.plugins.microbot.fm.spinner;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Skill;
import net.runelite.api.coords.WorldArea;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.gameval.ItemID;
import net.runelite.api.gameval.NpcID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.statemachine.StateMachineScript;
import net.runelite.client.plugins.microbot.statemachine.Transition;
import net.runelite.client.plugins.microbot.testing.TestResult;
import net.runelite.client.plugins.microbot.testing.TestResultWriter;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.bank.enums.BankLocation;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;
import net.runelite.client.plugins.microbot.util.widget.Rs2Widget;

/**
 * BANK (clean inventory, make sure we hold shears) → SHEAR until full → SPIN until no wool
 * → BANK (deposit, count the trip) → SHEAR ... Each onState does at most one game action
 * and re-derives what to do from observed state on the next tick.
 */
@Slf4j
public class SpinnerScript extends StateMachineScript<SpinnerScript.State>
{
	enum State
	{
		BANK,
		SHEAR,
		SPIN,
		STOPPED
	}

	// Sheep field north of Lumbridge (Sheep Shearer quest helper).
	static final WorldPoint SHEEP_FIELD = new WorldPoint(3201, 3268, 0);
	static final int SHEEP_FIELD_RADIUS = 12;
	// The 1st-floor wheel (3209,3212) sits in a small room that only opens north, onto the
	// corridor with doors at y=3214. Distance alone is wrong here: from 3209,3211 the wheel is
	// one tile away through a wall ("I can't reach that!"). Stand inside the room instead.
	static final WorldArea WHEEL_ROOM = new WorldArea(3208, 3212, 3, 2, 1);
	static final WorldPoint WHEEL_STAND = new WorldPoint(3209, 3213, 1);
	static final int[] UNSHEARED_SHEEP = {
		NpcID.SHEEPUNSHEERED, NpcID.SHEEPUNSHEEREDG, NpcID.SHEEPUNSHEEREDW,
		NpcID.SHEEPUNSHEERED2, NpcID.SHEEPUNSHEERED2G, NpcID.SHEEPUNSHEERED2W,
		NpcID.SHEEPUNSHEERED3, NpcID.SHEEPUNSHEERED3G, NpcID.SHEEPUNSHEERED3W,
		NpcID.SHEEPUNSHEEREDSHAGGY, NpcID.SHEEPUNSHEEREDSHAGGY2
	};

	private SpinnerConfig config;
	private TestResult testResult;
	private int startCraftingXp = -1;

	private boolean banked;
	private String stopReason;
	private int trips;
	private int ballsBanked;

	@Override
	protected State initialState()
	{
		// Resume a trip in progress instead of banking a full inventory of unspun wool.
		if (Rs2Inventory.hasItem(ItemID.SHEARS) && Rs2Inventory.isFull() && Rs2Inventory.hasItem(ItemID.WOOL))
		{
			return State.SPIN;
		}
		return State.BANK;
	}

	@Override
	protected List<Transition<State>> defineTransitions()
	{
		return List.of(
			Transition.<State>from(State.BANK)
				.when(() -> stopReason != null, "stopReason != null")
				.because("Cannot continue")
				.goTo(State.STOPPED),
			Transition.<State>from(State.BANK)
				.when(() -> banked, "banked")
				.because("Inventory clean and shears in hand")
				.goTo(State.SHEAR),
			Transition.<State>from(State.SHEAR)
				.when(() -> !Rs2Inventory.hasItem(ItemID.SHEARS), "!hasItem(SHEARS)")
				.because("No shears in the inventory")
				.goTo(State.BANK),
			Transition.<State>from(State.SHEAR)
				.when(Rs2Inventory::isFull, "isFull()")
				.because("Inventory full of wool")
				.goTo(State.SPIN),
			Transition.<State>from(State.SPIN)
				.when(() -> !Rs2Inventory.hasItem(ItemID.WOOL), "!hasItem(WOOL)")
				.because("All wool spun")
				.goTo(State.BANK)
		);
	}

	@Override
	protected void onTransition(State from, State to, String reason)
	{
		log.info("Spinner {} -> {}: {}", from, to, reason);
		if (to == State.BANK)
		{
			banked = false;
		}
	}

	@Override
	protected void onState(State state)
	{
		switch (state)
		{
			case BANK:
				bank();
				break;
			case SHEAR:
				shear();
				break;
			case SPIN:
				spin();
				break;
			case STOPPED:
				stop();
				break;
		}
	}

	public boolean run(SpinnerConfig config)
	{
		this.config = config;
		trips = 0;
		ballsBanked = 0;
		stopReason = null;
		testResult = Boolean.getBoolean("microbot.test.mode") ? new TestResult("Lumbridge Spinner") : null;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				if (!Microbot.isLoggedIn()) return;
				if (startCraftingXp < 0)
				{
					startCraftingXp = craftingXp();
				}
				step();
			}
			catch (Exception ex)
			{
				log.error("Spinner loop failed", ex);
			}
		}, 0, 600, TimeUnit.MILLISECONDS);
		return true;
	}

	@Override
	public void shutdown()
	{
		super.shutdown();
		startCraftingXp = -1;
	}

	private void bank()
	{
		if (!Rs2Bank.isOpen())
		{
			Microbot.status = "Spinner: walking to the bank";
			Rs2Bank.walkToBankAndUseBank(BankLocation.LUMBRIDGE_TOP);
			return;
		}

		int balls = Rs2Inventory.count(ItemID.BALL_OF_WOOL);
		if (Rs2Inventory.count() > (Rs2Inventory.hasItem(ItemID.SHEARS) ? 1 : 0))
		{
			Rs2Bank.depositAllExcept("Shears");
			if (!sleepUntil(() -> !Rs2Inventory.hasItem(ItemID.BALL_OF_WOOL), 3000))
			{
				return;
			}
			if (balls > 0)
			{
				ballsBanked += balls;
				trips++;
				report();
			}
		}

		if (!Rs2Inventory.hasItem(ItemID.SHEARS))
		{
			if (!Rs2Bank.hasItem(ItemID.SHEARS))
			{
				stopReason = "No shears in the inventory or bank (buy them at the Lumbridge general store)";
				return;
			}
			Rs2Bank.withdrawOne("Shears");
			sleepUntil(() -> Rs2Inventory.hasItem(ItemID.SHEARS), 3000);
			return;
		}

		if (finishedTrips())
		{
			stopReason = "Completed " + trips + " trip(s)";
			return;
		}

		Rs2Bank.closeBank();
		banked = sleepUntil(() -> !Rs2Bank.isOpen(), 3000);
	}

	private void shear()
	{
		if (Rs2Player.isMoving() || Rs2Player.isAnimating())
		{
			return;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return;
		}
		if (me.getPlane() != SHEEP_FIELD.getPlane() || me.distanceTo(SHEEP_FIELD) > SHEEP_FIELD_RADIUS)
		{
			Microbot.status = "Spinner: walking to the sheep";
			Rs2Walker.walkTo(SHEEP_FIELD, 4);
			return;
		}

		Microbot.status = "Spinner: shearing (" + Rs2Inventory.count(ItemID.WOOL) + " wool)";
		int wool = Rs2Inventory.count(ItemID.WOOL);
		boolean clicked = Microbot.getRs2NpcCache().query()
			.withIds(UNSHEARED_SHEEP)
			.within(SHEEP_FIELD, SHEEP_FIELD_RADIUS + 4)
			.interact("Shear");
		if (clicked)
		{
			sleepUntil(() -> Rs2Inventory.count(ItemID.WOOL) > wool, 6000);
		}
	}

	private void spin()
	{
		if (Rs2Widget.isProductionWidgetOpen())
		{
			Rs2Widget.handleProcessingInterface("Ball of wool");
			return;
		}
		// Spinning animates per item with short gaps; don't re-click mid-batch.
		if (Rs2Player.isMoving() || Rs2Player.isAnimating(2400))
		{
			Microbot.status = "Spinner: spinning (" + Rs2Inventory.count(ItemID.WOOL) + " wool left)";
			return;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return;
		}
		if (!WHEEL_ROOM.contains(me))
		{
			Microbot.status = "Spinner: walking to the spinning wheel";
			Rs2Walker.walkTo(WHEEL_STAND, 0);
			return;
		}

		boolean clicked = Microbot.getRs2TileObjectCache().query()
			.withName("Spinning wheel")
			.withAction("Spin")
			.interact("Spin");
		if (clicked)
		{
			sleepUntil(Rs2Widget::isProductionWidgetOpen, 5000);
		}
	}

	private void stop()
	{
		Microbot.status = "Spinner stopped: " + stopReason;
		log.info("Spinner stopped: {}", stopReason);
		if (testResult != null)
		{
			finishTest();
			return;
		}
		shutdown();
	}

	private boolean finishedTrips()
	{
		int limit = testResult != null ? config.testTrips() : config.maxTrips();
		return limit > 0 && trips >= limit;
	}

	private void report()
	{
		Map<String, Object> progress = new LinkedHashMap<>();
		progress.put("trips", trips);
		progress.put("ballsBanked", ballsBanked);
		progress.put("craftingXpGained", craftingXp() - startCraftingXp);
		ScriptResultStore.submit(SpinnerPlugin.class.getName(), progress);
		log.info("Spinner progress: {}", progress);
	}

	// Test mode protocol (docs/AGENTIC_TESTING_LOOP.md): record checks, write result.json, exit.
	private void finishTest()
	{
		int xpGained = craftingXp() - startCraftingXp;
		int expectedTrips = config.testTrips();
		check("completed " + trips + "/" + expectedTrips + " trips", trips >= expectedTrips, stopReason);
		check("banked " + ballsBanked + " balls of wool", ballsBanked > 0, "no balls of wool were banked");
		check("crafting xp gained: " + xpGained, xpGained > 0, "no Crafting XP gained");
		testResult.complete("completed");
		TestResultWriter.write(testResult);
		log.info("Spinner test finished: {}/{} checks passed", testResult.passedCount(), testResult.checks.size());
		System.exit(testResult.exitCode);
	}

	private void check(String name, boolean passed, String failure)
	{
		testResult.addCheck(name, passed, passed ? null : failure);
	}

	private static int craftingXp()
	{
		return Microbot.getClientThread()
			.runOnClientThreadOptional(() -> Microbot.getClient().getSkillExperience(Skill.CRAFTING))
			.orElse(0);
	}
}
