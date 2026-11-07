package net.runelite.client.plugins.microbot.fm.tickfisher;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Skill;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.gameval.ItemID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.Script;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.testing.TestResult;
import net.runelite.client.plugins.microbot.testing.TestResultWriter;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.bank.Rs2Bank;
import net.runelite.client.plugins.microbot.util.bank.enums.BankLocation;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * 3-tick fly fishing at Shilo Village. The loop runs once per game tick, one phase of the cycle
 * (wiki, Tick manipulation): on phase 0 knife on teak sets the skilling timer to 3 and the spot
 * click turns it into a fishing roll 3 ticks later, instead of 5. Phase 1 drops a fish and
 * selects the knife, phase 2 brings the cursor onto the teak log, so phase 0 starts with a click
 * where the cursor already is. The teak log is never used up: the spot click cancels the fletch.
 */
@Slf4j
public class TickFisherScript extends Script
{
	static final String SPOT_NAME = "Rod Fishing spot";
	private static final String SPOT_ACTION = "Lure";
	/** Between the spots of the Shilo river, east and west of the bank path. */
	private static final WorldPoint FISHING_AREA = new WorldPoint(2846, 2975, 0);
	private static final int FISHING_AREA_RADIUS = 16;
	/** On the bank path by the river: where to walk when far from it (the area's centre is water). */
	private static final WorldPoint RIVER_BANK = new WorldPoint(2842, 2969, 0);
	/** Farther than this from the river at start: not at Shilo. */
	private static final int START_RADIUS = 40;
	private static final int FLY_FISHING_LEVEL = 20;

	private static final int[] RODS = {ItemID.FLY_FISHING_ROD, ItemID.FISHINGROD_PEARL_FLY};
	private static final int[] FISH = {ItemID.RAW_TROUT, ItemID.RAW_SALMON};

	private final FishingEvents events = new FishingEvents();
	private Clicks clicks;
	private TickFisherConfig config;
	// Read by the overlay on the client thread.
	private volatile Session session;
	private TestResult testResult;
	private long lastReport;

	/** The spot being fished, its tile when picked, and the tick phase 0 counts from. */
	private Rs2NpcModel spot;
	private WorldPoint spotTile;
	private int anchorTick = -1;
	/** The last tick the loop acted on, and the last XP drop tick counted. */
	private int handledTick = -1;
	private int countedDropTick = -1;
	private int bankFailures;
	/**
	 * The varied rhythm: cycles until the next hesitation and how many cycles it still skips;
	 * when the current plain-fishing breather ends and the next one starts.
	 */
	private int cyclesToSlip;
	private int slipCycles;
	private long breatherUntil;
	private long nextBreatherAt;

	FishingEvents events()
	{
		return events;
	}

	/** The run in progress, or null before it starts and after it stops. */
	Session session()
	{
		return session;
	}

	int xpGained()
	{
		Session s = session;
		return s == null || events.fishingXp() == 0 ? 0 : events.fishingXp() - s.startXp;
	}

	public boolean run(TickFisherConfig config)
	{
		this.config = config;
		session = null;
		spot = null;
		anchorTick = -1;
		handledTick = -1;
		countedDropTick = -1;
		bankFailures = 0;
		clicks = new Clicks(config.mouse());
		cyclesToSlip = nextSlip();
		slipCycles = 0;
		breatherUntil = 0;
		nextBreatherAt = System.currentTimeMillis() + nextBreatherGap();
		testResult = Boolean.getBoolean("microbot.test.mode") ? new TestResult("Tick Fisher") : null;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				if (!Microbot.isLoggedIn()) return;
				if (!super.run()) return;
				// Once per tick, as early in it as we can. A step that ran into the next tick (a
				// slow natural movement) does not wait for another: that tick is acted on now.
				if (events.tick() == handledTick && !Global.sleepUntilNextTick(2000)) return;
				handledTick = events.tick();
				loop();
				report(false);
			}
			catch (Exception ex)
			{
				log.error("Tick Fisher loop failed", ex);
			}
		}, 0, 1, TimeUnit.MILLISECONDS);
		return true;
	}

	@Override
	public void shutdown()
	{
		super.shutdown();
		session = null;
	}

	private void loop()
	{
		if (session == null && !start())
		{
			return;
		}
		Session s = session;
		checkLimits(s);
		if (s.stopReason != null)
		{
			stop(s.stopReason);
			return;
		}
		if (!hasGear() || (Rs2Bank.isOpen()))
		{
			gearUp(s);
			return;
		}
		if (Rs2Inventory.isFull())
		{
			if (config.fishHandling() == FishHandling.BANK)
			{
				bankFish(s);
			}
			else
			{
				dropAllFish(s);
			}
			return;
		}
		Rs2NpcModel target = currentSpot(s);
		if (target == null)
		{
			// Shilo has few spots, and after a move none may show for up to a minute: wait by the
			// river rather than walk.
			WorldPoint me = Rs2Player.getWorldLocation();
			if (me != null && me.distanceTo(RIVER_BANK) > FISHING_AREA_RADIUS)
			{
				Microbot.status = "Tick Fisher: walking to the river";
				Rs2Walker.walkTo(RIVER_BANK, 2);
			}
			else
			{
				Microbot.status = "Tick Fisher: waiting for a fishing spot";
				s.spotWaitTicks++;
			}
			return;
		}
		if (!nextTo(target))
		{
			approach(target);
			return;
		}
		s.fishingTicks++;
		if (config.threeTick() && !breather(s))
		{
			cycle(s, target);
		}
		else
		{
			plain(s, target);
		}
	}

	/** Checks level and place, then starts the run; false when it cannot run. */
	private boolean start()
	{
		int level = Rs2Player.getRealSkillLevel(Skill.FISHING);
		if (level < FLY_FISHING_LEVEL)
		{
			stop("Fly fishing needs " + FLY_FISHING_LEVEL + " Fishing (have " + level + ")");
			return false;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return false;
		}
		if (me.distanceTo(FISHING_AREA) > START_RADIUS)
		{
			stop("Start at Shilo Village (the river by the bank)");
			return false;
		}
		int xp = fishingXp();
		events.onFishing(level, xp);
		session = new Session(events.tick(), xp);
		log.info("Tick Fisher {}: Fishing {}, 3-tick {}, fish {}, mouse {}", TickFisherPlugin.VERSION, level, config.threeTick(), config.fishHandling(), config.mouse());
		return true;
	}

	private void checkLimits(Session s)
	{
		int level = config.stopAtLevel();
		if (level > 0 && events.fishingLevel() >= level)
		{
			s.stop("Reached Fishing " + events.fishingLevel());
		}
		int minutes = config.stopAfterMinutes();
		if (minutes > 0 && System.currentTimeMillis() - s.startedAt >= minutes * 60_000L)
		{
			s.stop("Ran for " + minutes + " minutes");
		}
		if (testResult != null && s.catches >= config.testCatches())
		{
			s.stop("Test: " + s.catches + " catches");
		}
	}

	// ----- the cycle ---------------------------------------------------------------------------

	/** One tick of the 3-tick cycle. Phase 0 is the tick the previous roll lands on. */
	private void cycle(Session s, Rs2NpcModel target)
	{
		int tick = events.tick();
		if (anchorTick < 0)
		{
			anchorTick = tick;
		}
		int drop = events.lastDropTick();
		if (drop > countedDropTick)
		{
			countedDropTick = drop;
			s.catches++;
			int dropPhase = Math.floorMod(drop - anchorTick, 3);
			if (dropPhase != 0 && drop == tick)
			{
				// Off the cycle (after a hesitation, a breather or a lost click): the skilling
				// timer is at zero on a drop tick, so a cycle started now is in step again.
				log.info("Tick Fisher: XP drop on phase {} at tick {}, re-anchoring", dropPhase, tick);
				anchorTick = tick;
				s.realigns++;
			}
			else
			{
				s.dropsByPhase[dropPhase]++;
			}
		}
		int phase = Math.floorMod(tick - anchorTick, 3);
		Rs2ItemModel knife = Rs2Inventory.get(ItemID.KNIFE);
		Rs2ItemModel teak = Rs2Inventory.get(ItemID.TEAK_LOGS);
		if (knife == null || teak == null)
		{
			return;
		}
		switch (phase)
		{
			case 0:
				if (hesitate(s))
				{
					Microbot.status = "Tick Fisher: hesitating";
					break;
				}
				Microbot.status = "Tick Fisher: 3-tick fishing";
				if (!knifeSelected())
				{
					clicks.select(knife);
				}
				clicks.useOn(teak);
				boolean teakInTime = events.tick() == tick;
				clicks.npc(target, SPOT_ACTION);
				int late = events.tick() - tick;
				s.cycles++;
				if (late == 0)
				{
					s.onTime++;
				}
				else if (teakInTime && late == 1)
				{
					s.spotNextTick++;
				}
				break;
			case 1:
				if (config.fishHandling() == FishHandling.DROP)
				{
					Rs2ItemModel fish = Rs2Inventory.get(FISH);
					if (fish != null)
					{
						clicks.drop(fish);
						s.fishDropped++;
					}
				}
				if (!knifeSelected())
				{
					clicks.select(knife);
				}
				break;
			default:
				if (!knifeSelected())
				{
					clicks.select(knife);
				}
				clicks.hover(teak);
				break;
		}
	}

	/**
	 * A cycle skipped on purpose now and then: the roll still comes, on the 5-tick timer, and the
	 * cycle finds its step again on the next off-cycle drop.
	 */
	private boolean hesitate(Session s)
	{
		if (!config.varyRhythm())
		{
			return false;
		}
		if (slipCycles > 0)
		{
			slipCycles--;
			return true;
		}
		if (--cyclesToSlip > 0)
		{
			return false;
		}
		cyclesToSlip = nextSlip();
		slipCycles = Rs2Random.betweenInclusive(0, 1);
		s.slips++;
		return true;
	}

	/** Every few minutes, fish plainly for a while; true while it lasts. */
	private boolean breather(Session s)
	{
		if (!config.varyRhythm())
		{
			return false;
		}
		long now = System.currentTimeMillis();
		if (now >= nextBreatherAt)
		{
			breatherUntil = now + Rs2Random.betweenInclusive(10_000, 40_000);
			nextBreatherAt = breatherUntil + nextBreatherGap();
			s.breathers++;
		}
		if (now >= breatherUntil)
		{
			return false;
		}
		Microbot.status = "Tick Fisher: fishing plainly for a bit";
		plain(s, spot);
		anchorTick = -1;
		return true;
	}

	private static int nextSlip()
	{
		return Rs2Random.betweenInclusive(25, 70);
	}

	private static long nextBreatherGap()
	{
		return Rs2Random.betweenInclusive(4 * 60_000, 9 * 60_000);
	}

	/** Plain fishing: click the spot whenever we are not already on it. */
	private void plain(Session s, Rs2NpcModel target)
	{
		int drop = events.lastDropTick();
		if (drop > countedDropTick)
		{
			countedDropTick = drop;
			s.catches++;
		}
		Microbot.status = "Tick Fisher: fishing";
		if (!Rs2Player.isInteracting() && !Rs2Player.isAnimating())
		{
			clicks.npc(target, SPOT_ACTION);
		}
	}

	private static boolean knifeSelected()
	{
		return Rs2Inventory.isItemSelected() && Rs2Inventory.getSelectedItemId() == ItemID.KNIFE;
	}

	// ----- the spot ----------------------------------------------------------------------------

	/**
	 * The spot we fish, or the nearest one when it despawned or moved. A move breaks the cycle:
	 * the player stops fishing, and the next cycle starts once we stand next to the new spot.
	 */
	private Rs2NpcModel currentSpot(Session s)
	{
		if (spot != null)
		{
			boolean gone = events.despawnedIndex() == spot.getIndex()
				|| !spotTile.equals(spot.getWorldLocation());
			if (!gone)
			{
				return spot;
			}
			s.spotMoves++;
			spot = null;
			anchorTick = -1;
		}
		Rs2NpcModel nearest = Microbot.getRs2NpcCache().query()
			.where(npc -> SPOT_NAME.equals(npc.getName()))
			.where(npc -> npc.getWorldLocation().distanceTo(FISHING_AREA) <= FISHING_AREA_RADIUS)
			.nearestOnClientThread();
		if (nearest != null)
		{
			spot = nearest;
			spotTile = nearest.getWorldLocation();
			events.clearDespawned();
		}
		return nearest;
	}

	/**
	 * Next to the spot. Distance only: Rs2Player.isMoving compares the pose animation with the
	 * idle one and is true while turning or fishing, which read as walking threw the cycle away
	 * and waited up to 8 s (0.3.1: about half the XP drops off the cycle).
	 */
	private static boolean nextTo(Rs2NpcModel target)
	{
		WorldPoint me = Rs2Player.getWorldLocation();
		return me != null && me.distanceTo(target.getWorldLocation()) <= 1;
	}

	/** Walks to the spot by clicking it, then starts a fresh cycle on the next tick. */
	private void approach(Rs2NpcModel target)
	{
		Microbot.status = "Tick Fisher: walking to the spot";
		session.approaches++;
		log.info("Tick Fisher: walking to the spot at {} from {}", target.getWorldLocation(), Rs2Player.getWorldLocation());
		anchorTick = -1;
		if (!clicks.npc(target, SPOT_ACTION))
		{
			Rs2Walker.walkTo(target.getWorldLocation(), 1);
			return;
		}
		Global.sleepUntil(() -> nextTo(target), 8000);
	}

	// ----- inventory, bank ---------------------------------------------------------------------

	private boolean hasGear()
	{
		if (!Rs2Inventory.hasItem(RODS) || !Rs2Inventory.hasItem(ItemID.FEATHER))
		{
			return false;
		}
		// Exactly one log: with more, knife on teak opens a Make-X window and loses the tick.
		return !config.threeTick()
			|| (Rs2Inventory.hasItem(ItemID.KNIFE) && Rs2Inventory.count(ItemID.TEAK_LOGS) == 1);
	}

	/** At the Shilo bank: keeps rod, feathers, knife and coins, and takes exactly one teak log. */
	private void gearUp(Session s)
	{
		Microbot.status = "Tick Fisher: getting rod, feathers, knife and a teak log";
		if (!openBank())
		{
			return;
		}
		if (Rs2Inventory.hasItem(ItemID.XBOWS_CROSSBOW_STOCK_TEAK))
		{
			// The fletch finished and used the log: a spot click that did not land in time.
			s.stocksMade += Rs2Inventory.count(ItemID.XBOWS_CROSSBOW_STOCK_TEAK);
		}
		Rs2Bank.depositAllExcept(ItemID.FLY_FISHING_ROD, ItemID.FISHINGROD_PEARL_FLY, ItemID.FEATHER,
			ItemID.KNIFE, ItemID.TEAK_LOGS, ItemID.COINS);
		if (!Rs2Inventory.hasItem(RODS) && !withdraw(ItemID.FLY_FISHING_ROD, 1, "fly fishing rod"))
		{
			return;
		}
		if (!Rs2Inventory.hasItem(ItemID.FEATHER) && !withdraw(ItemID.FEATHER, -1, "feathers"))
		{
			return;
		}
		if (config.threeTick())
		{
			if (!Rs2Inventory.hasItem(ItemID.KNIFE) && !withdraw(ItemID.KNIFE, 1, "knife"))
			{
				return;
			}
			if (Rs2Inventory.count(ItemID.TEAK_LOGS) > 1)
			{
				Rs2Bank.depositAll(ItemID.TEAK_LOGS);
				Global.sleepUntil(() -> !Rs2Inventory.hasItem(ItemID.TEAK_LOGS), 2400);
			}
			if (!Rs2Inventory.hasItem(ItemID.TEAK_LOGS) && !withdraw(ItemID.TEAK_LOGS, 1, "teak logs"))
			{
				return;
			}
		}
		Rs2Bank.closeBank();
		Global.sleepUntil(() -> !Rs2Bank.isOpen(), 2400);
		spot = null;
	}

	private void bankFish(Session s)
	{
		Microbot.status = "Tick Fisher: banking fish";
		if (!openBank())
		{
			return;
		}
		Rs2Bank.depositAll(item -> item.getId() == ItemID.RAW_TROUT || item.getId() == ItemID.RAW_SALMON);
		if (!Global.sleepUntil(() -> !Rs2Inventory.hasItem(FISH), 3000))
		{
			return;
		}
		s.bankTrips++;
		Rs2Bank.closeBank();
		Global.sleepUntil(() -> !Rs2Bank.isOpen(), 2400);
		spot = null;
	}

	private boolean openBank()
	{
		if (Rs2Bank.isOpen())
		{
			return true;
		}
		if (Rs2Bank.walkToBankAndUseBank(BankLocation.SHILO_VILLAGE) && Global.sleepUntil(Rs2Bank::isOpen, 5000))
		{
			bankFailures = 0;
			return true;
		}
		if (++bankFailures >= 5)
		{
			session.stop("Could not open the Shilo bank");
		}
		return false;
	}

	/** Withdraws an item (amount -1: all); stops the run when the bank has none. */
	private boolean withdraw(int id, int amount, String name)
	{
		if (!Rs2Bank.hasItem(id))
		{
			session.stop("No " + name + " in the bank");
			return false;
		}
		boolean clicked = amount < 0 ? Rs2Bank.withdrawAll(id) : amount == 1 ? Rs2Bank.withdrawOne(id) : Rs2Bank.withdrawX(id, amount);
		return clicked && Global.sleepUntil(() -> Rs2Inventory.hasItem(id), 3000);
	}

	/** Full inventory while dropping: only when the drops between rolls fell behind. */
	private void dropAllFish(Session s)
	{
		Microbot.status = "Tick Fisher: dropping fish";
		int before = Rs2Inventory.count(ItemID.RAW_TROUT) + Rs2Inventory.count(ItemID.RAW_SALMON);
		Rs2Inventory.dropAll(FISH);
		Global.sleepUntil(() -> !Rs2Inventory.hasItem(FISH), 6000);
		s.fishDropped += before - Rs2Inventory.count(ItemID.RAW_TROUT) - Rs2Inventory.count(ItemID.RAW_SALMON);
		anchorTick = -1;
	}

	// ----- stopping and reporting --------------------------------------------------------------

	private void stop(String reason)
	{
		Rs2NpcModel last = spot;
		if (config.threeTick() && last != null && Rs2Inventory.hasItem(ItemID.TEAK_LOGS))
		{
			// A knife-on-teak may still be running: one more spot click cancels it, or the fletch
			// completes and takes the log.
			clicks.npc(last, SPOT_ACTION);
		}
		Microbot.status = "Tick Fisher stopped: " + reason;
		log.info("Tick Fisher stopped: {}", reason);
		report(true, reason);
		if (testResult != null)
		{
			finishTest(reason);
			return;
		}
		shutdown();
	}

	private void report(boolean force)
	{
		Session s = session;
		report(force, s == null ? null : s.stopReason);
	}

	private void report(boolean force, String stopReason)
	{
		long now = System.currentTimeMillis();
		if (!force && now - lastReport < 5000)
		{
			return;
		}
		lastReport = now;
		Session s = session;
		Map<String, Object> progress = new LinkedHashMap<>();
		progress.put("threeTick", config.threeTick());
		progress.put("fishHandling", config.fishHandling().name());
		progress.put("mouse", config.mouse().name());
		progress.put("fishingLevel", events.fishingLevel());
		progress.put("fishingXpGained", xpGained());
		if (s != null)
		{
			progress.putAll(s.counters());
			progress.put("ticks", events.tick() - s.startTick);
		}
		progress.put("stopReason", stopReason);
		ScriptResultStore.submit(TickFisherPlugin.class.getName(), progress);
	}

	// Test mode protocol (docs/AGENTIC_TESTING_LOOP.md): record checks, write result.json, exit.
	private void finishTest(String reason)
	{
		Session s = session;
		int catches = s == null ? 0 : s.catches;
		int xp = xpGained();
		check("catches: " + catches + "/" + config.testCatches(), catches >= config.testCatches(), reason);
		check("fishing xp gained: " + xp, xp > 0, "no Fishing XP gained");
		if (config.threeTick())
		{
			int onTime = s == null ? 0 : s.onTimePercent();
			int inStep = s == null ? 0 : s.inStepPercent();
			check("cycles on time: " + onTime + "%", onTime >= 90, "clicks spilled into the next tick");
			check("xp drops in step: " + inStep + "%", inStep >= 90, "rolls not on the 3-tick cycle");
		}
		testResult.complete("completed");
		TestResultWriter.write(testResult);
		log.info("Tick Fisher test finished: {}/{} checks passed", testResult.passedCount(), testResult.checks.size());
		System.exit(testResult.exitCode);
	}

	private void check(String name, boolean passed, String failure)
	{
		testResult.addCheck(name, passed, passed ? null : failure);
	}

	private static int fishingXp()
	{
		return Microbot.getClientThread()
			.runOnClientThreadOptional(() -> Microbot.getClient().getSkillExperience(Skill.FISHING))
			.orElse(0);
	}
}
