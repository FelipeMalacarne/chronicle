package net.runelite.client.plugins.microbot.fm.processor;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.statemachine.StateMachineScript;
import net.runelite.client.plugins.microbot.statemachine.Transition;
import net.runelite.client.plugins.microbot.testing.TestResult;
import net.runelite.client.plugins.microbot.testing.TestResultWriter;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;
import net.runelite.client.plugins.microbot.util.widget.Rs2Widget;

/**
 * BANK (deposit what was made, withdraw one trip's inputs) → MAKE (station, make-X, wait while
 * the inputs last) → BANK ... until the bank runs out of an input or the trips are done. Each
 * onState does at most one game action and re-derives what to do from observed state on the
 * next tick.
 */
@Slf4j
public class ProcessorScript extends StateMachineScript<ProcessorScript.State>
{
	enum State
	{
		BANK,
		MAKE,
		STOPPED
	}

	/** Farther than this from the site's tile, walk there before clicking the station. */
	static final int NEAR_STATION = 6;
	/** Make-X misses in a row (the option is not there) before giving up. */
	static final int MAX_MAKE_MISSES = 3;

	private ProcessorConfig config;
	private Product product;
	private Site site;
	private TestResult testResult;
	private int startXp = -1;

	private boolean banked;
	private String stopReason;
	private int trips;
	private int made;
	private int makeMisses;
	// The batch's progress: the actions the inventory held last tick, and when that last went down.
	private int lastHeld = -1;
	private long lastProgressAt;

	@Override
	protected State initialState()
	{
		// Resume a trip in progress (a restart mid-trip) instead of banking the inputs.
		if (stopReason == null && Batch.canAct(product, Rs2Inventory::count) && onlyInputsAndTools())
		{
			return State.MAKE;
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
				.because("One trip's inputs in the inventory")
				.goTo(State.MAKE),
			Transition.<State>from(State.MAKE)
				.when(() -> stopReason != null, "stopReason != null")
				.because("Cannot continue")
				.goTo(State.STOPPED),
			Transition.<State>from(State.MAKE)
				.when(() -> !Batch.canAct(product, Rs2Inventory::count), "!canAct(inventory)")
				.because("Inputs used up")
				.goTo(State.BANK)
		);
	}

	@Override
	protected void onTransition(State from, State to, String reason)
	{
		log.info("Processor {} -> {}: {}", from, to, reason);
		if (to == State.BANK)
		{
			banked = false;
		}
		makeMisses = 0;
		lastHeld = -1;
		lastProgressAt = 0;
	}

	@Override
	protected void onState(State state)
	{
		switch (state)
		{
			case BANK:
				bank();
				break;
			case MAKE:
				make();
				break;
			case STOPPED:
				stop();
				break;
		}
	}

	public boolean run(ProcessorConfig config)
	{
		this.config = config;
		product = config.product();
		site = config.site();
		trips = 0;
		made = 0;
		stopReason = site.station == product.station ? null
			: product + " is not made at the " + site;
		testResult = Boolean.getBoolean("microbot.test.mode") ? new TestResult("Processor") : null;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				if (!Microbot.isLoggedIn()) return;
				if (startXp < 0)
				{
					startXp = xp();
				}
				step();
			}
			catch (Exception ex)
			{
				log.error("Processor loop failed", ex);
			}
		}, 0, 600, TimeUnit.MILLISECONDS);
		return true;
	}

	@Override
	public void shutdown()
	{
		super.shutdown();
		startXp = -1;
	}

	private void bank()
	{
		if (Rs2Bank.isBankPinWidgetVisible())
		{
			Microbot.status = "Processor: waiting for the bank PIN";
			return;
		}
		if (!Rs2Bank.isOpen())
		{
			Microbot.status = "Processor: walking to the bank";
			Rs2Bank.walkToBankAndUseBank(site.bank);
			return;
		}
		// The bank's items arrive a moment after it opens; an empty list is no proof of stock.
		if (Rs2Bank.bankItems().isEmpty())
		{
			return;
		}

		if (!onlyInputsAndTools())
		{
			int products = Rs2Inventory.count(product.itemId);
			if (!depositAllButTools())
			{
				return;
			}
			if (products > 0)
			{
				made += products;
				trips++;
				report();
			}
			return;
		}

		for (Product.Input tool : product.tools)
		{
			if (Rs2Inventory.hasItem(tool.itemId))
			{
				continue;
			}
			if (!Rs2Bank.hasItem(tool.itemId))
			{
				stopReason = "No " + tool.name + " in the inventory or the bank";
				return;
			}
			Rs2Bank.withdrawOne(tool.itemId);
			sleepUntil(() -> Rs2Inventory.hasItem(tool.itemId), 3000);
			return;
		}

		if (finishedTrips())
		{
			stopReason = "Completed " + trips + " trip(s)";
			return;
		}

		int actions = Batch.actions(product, id -> Rs2Bank.count(id) + Rs2Inventory.count(id));
		if (actions == 0)
		{
			Product.Input out = Batch.scarcest(product, id -> Rs2Bank.count(id) + Rs2Inventory.count(id));
			stopReason = "Out of " + out.name + " (made " + made + " " + product + ")";
			return;
		}
		for (Product.Input in : product.inputs)
		{
			int held = Rs2Inventory.count(in.itemId);
			int want = actions * in.perAction;
			if (held > want)
			{
				Rs2Bank.depositAll(in.itemId);
				sleepUntil(() -> Rs2Inventory.count(in.itemId) == 0, 3000);
				return;
			}
			if (held < want)
			{
				Rs2Bank.withdrawX(in.itemId, want - held);
				sleepUntil(() -> Rs2Inventory.count(in.itemId) > held, 3000);
				return;
			}
		}

		Rs2Bank.closeBank();
		banked = sleepUntil(() -> !Rs2Bank.isOpen(), 3000);
	}

	private void make()
	{
		if (Rs2Widget.isProductionWidgetOpen())
		{
			int before = actionsHeld();
			if (!Rs2Widget.handleProcessingInterface(product.option))
			{
				if (++makeMisses >= MAX_MAKE_MISSES)
				{
					stopReason = "The make-X has no \"" + product.option + "\" option";
				}
				return;
			}
			makeMisses = 0;
			lastProgressAt = System.currentTimeMillis();
			sleepUntil(() -> Rs2Player.isAnimating() || actionsHeld() < before, 3000);
			return;
		}
		// The batch is going while the inputs keep going down. The animation is no sign of it:
		// at the furnace it plays once and the player stands idle for the next few glass. A
		// level-up ends the batch early, and the next click on the station starts it again.
		int held = actionsHeld();
		long now = System.currentTimeMillis();
		if (held != lastHeld)
		{
			if (lastHeld >= 0)
			{
				lastProgressAt = now;
			}
			lastHeld = held;
		}
		if (Rs2Player.isMoving() || now - lastProgressAt < product.stalledAfterMs())
		{
			Microbot.status = "Processor: making " + product + " (" + held + " left)";
			return;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return;
		}
		if (me.getPlane() != site.stand.getPlane() || me.distanceTo(site.stand) > NEAR_STATION)
		{
			Microbot.status = "Processor: walking to the " + site;
			Rs2Walker.walkTo(site.stand, 2);
			return;
		}

		boolean clicked = Microbot.getRs2TileObjectCache().query()
			.interact(site.objectId, site.station.action);
		if (clicked)
		{
			sleepUntil(Rs2Widget::isProductionWidgetOpen, 5000);
		}
	}

	private void stop()
	{
		Microbot.status = "Processor stopped: " + stopReason;
		log.info("Processor stopped: {}", stopReason);
		if (testResult != null)
		{
			finishTest();
			return;
		}
		shutdown();
	}

	/** Deposits everything but the product's tools; true once the inventory holds only those. */
	private boolean depositAllButTools()
	{
		if (product.tools.length == 0)
		{
			return Rs2Bank.depositAll() && Rs2Inventory.isEmpty();
		}
		Integer[] keep = new Integer[product.tools.length];
		for (int i = 0; i < keep.length; i++)
		{
			keep[i] = product.tools[i].itemId;
		}
		Rs2Bank.depositAllExcept(keep);
		return sleepUntil(this::onlyInputsAndTools, 3000);
	}

	/** Whether every inventory slot holds an input or a tool of the product. */
	private boolean onlyInputsAndTools()
	{
		int known = 0;
		for (Product.Input in : product.inputs)
		{
			known += Rs2Inventory.count(in.itemId);
		}
		for (Product.Input tool : product.tools)
		{
			known += Rs2Inventory.count(tool.itemId);
		}
		return Rs2Inventory.count() == known;
	}

	private int actionsHeld()
	{
		int n = Integer.MAX_VALUE;
		for (Product.Input in : product.inputs)
		{
			n = Math.min(n, Rs2Inventory.count(in.itemId) / in.perAction);
		}
		return n;
	}

	private boolean finishedTrips()
	{
		int limit = testResult != null ? config.testTrips() : config.maxTrips();
		return limit > 0 && trips >= limit;
	}

	private void report()
	{
		Map<String, Object> progress = new LinkedHashMap<>();
		progress.put("product", product.name());
		progress.put("site", site.name());
		progress.put("trips", trips);
		progress.put("made", made);
		progress.put("xpGained", xp() - startXp);
		ScriptResultStore.submit(ProcessorPlugin.class.getName(), progress);
		log.info("Processor progress: {}", progress);
	}

	// Test mode protocol (docs/AGENTIC_TESTING_LOOP.md): record checks, write result.json, exit.
	private void finishTest()
	{
		int xpGained = xp() - startXp;
		int expectedTrips = config.testTrips();
		check("completed " + trips + "/" + expectedTrips + " trips", trips >= expectedTrips, stopReason);
		check("banked " + made + " " + product, made > 0, "nothing was made");
		check(product.skill.getName() + " xp gained: " + xpGained, xpGained > 0, "no XP gained");
		testResult.complete("completed");
		TestResultWriter.write(testResult);
		log.info("Processor test finished: {}/{} checks passed", testResult.passedCount(), testResult.checks.size());
		System.exit(testResult.exitCode);
	}

	private void check(String name, boolean passed, String failure)
	{
		testResult.addCheck(name, passed, passed ? null : failure);
	}

	private int xp()
	{
		return Microbot.getClientThread()
			.runOnClientThreadOptional(() -> Microbot.getClient().getSkillExperience(product.skill))
			.orElse(0);
	}
}
