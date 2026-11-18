package net.runelite.client.plugins.microbot.fm.tickminer;

import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.Skill;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.gameval.ItemID;
import net.runelite.api.gameval.ObjectID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.Script;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.api.tileobject.models.Rs2TileObjectModel;
import net.runelite.client.plugins.microbot.testing.TestResult;
import net.runelite.client.plugins.microbot.testing.TestResultWriter;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.antiban.Rs2Antiban;
import net.runelite.client.plugins.microbot.util.antiban.Rs2AntibanSettings;
import net.runelite.client.plugins.microbot.util.antiban.enums.Activity;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * 3-tick sandstone mining at the Desert Quarry (3t4s). The loop runs once per game tick, one phase
 * of the cycle (wiki, Tick manipulation): on phase 0 knife on teak (or a herb on swamp tar) sets the
 * skilling timer to 3, and a click on a rock of the spot's other row steps there and turns it into a
 * mining roll 3 ticks later, two rolls since the player moves up to the rock ({@link Rocks}). Phases
 * 1 and 2 select the knife and bring the cursor onto the teak log, so phase 0 is two clicks. The
 * teak log is never used up: the rock click cancels the fletch. A full inventory goes into Drew's
 * Sandstorm grinder, a dozen tiles west, as buckets of sand claimed from Drew later.
 */
@Slf4j
public class TickMinerScript extends Script
{
	/** Farther than this from the spot at start: not at the quarry. */
	private static final int START_RADIUS = 30;
	/**
	 * Farther than this the walker goes there; closer, a click on a rock does (the grinder is 15
	 * tiles away, the cacti 28: the walker took 15 s back from them, deciding between clicks).
	 */
	private static final int CLICK_RADIUS = 32;
	private static final int MINING_LEVEL = 35;
	private static final int SANDSTONE_ROCKS = ObjectID.ENAKH_SANDSTONE_ROCKS;
	private static final int GRINDER = ObjectID.GRINDER_MACHINE;
	/** 1, 2, 5 and 10 kg, and the buckets of sand the grinder makes of each (wiki, Sandstorm). */
	private static final int[] SANDSTONE = {ItemID.ENAKH_SANDSTONE_TINY, ItemID.ENAKH_SANDSTONE_SMALL,
		ItemID.ENAKH_SANDSTONE_MEDIUM, ItemID.ENAKH_SANDSTONE_LARGE};
	private static final int[] SAND_PER_SANDSTONE = {1, 2, 4, 8};
	/** Waterskins with water, by sips left (1 to 4). */
	private static final int[] WATERSKINS = {ItemID.WATER_SKIN1, ItemID.WATER_SKIN2, ItemID.WATER_SKIN3, ItemID.WATER_SKIN4};
	/** Clean herbs that make a tar, and the Herblore level each needs. */
	private static final int[] TAR_HERBS = {ItemID.GUAM_LEAF, ItemID.MARENTILL, ItemID.TARROMIN, ItemID.HARRALANDER};
	private static final int[] TAR_HERB_LEVELS = {19, 31, 39, 44};
	/** Making a tar takes 15 swamp tar; with fewer the action fails and sets no timer. */
	private static final int MIN_SWAMP_TAR = 15;
	/** Grinder clicks in a row that left the sandstone in the inventory before giving up. */
	private static final int MAX_DEPOSIT_FAILURES = 5;
	private static final int CACTUS = ObjectID.DESERT_CACTUS_FULL;
	/** All waterskins, empty to full: four sips each. */
	private static final int[] ALL_WATERSKINS = {ItemID.WATER_SKIN0, ItemID.WATER_SKIN1, ItemID.WATER_SKIN2,
		ItemID.WATER_SKIN3, ItemID.WATER_SKIN4};
	/** A dry cactus is healthy again a minute after its cut. */
	private static final long CACTUS_REGROW_MS = 60_000;
	/** Cut clicks in a row that came to nothing (no sip, no failed-cut message) before leaving the cacti. */
	private static final int MAX_CUT_FAILURES = 4;

	private final MiningEvents events = new MiningEvents();
	private Clicks clicks;
	private TickMinerConfig config;
	private DelayMethod delay;
	private int herbloreLevel;
	// Read by the overlay on the client thread.
	private volatile Session session;
	private TestResult testResult;
	private long lastReport;

	/** The tick phase 0 counts from, the last tick the loop acted on, the last XP drop counted. */
	private int anchorTick = -1;
	private int handledTick = -1;
	private int countedDropTick = -1;
	/** The rock clicked last, and when (plain mining waits for the swing to start before clicking again). */
	private WorldPoint lastRock;
	private int lastRockTick = -1;
	/** Sandstone still in the inventory after a deposit started, and failed grinder clicks in a row. */
	private boolean depositing;
	private int depositFailures;
	/** At the cacti until they are dry or the skins full; no trip before the cacti regrow. */
	private boolean refilling;
	private long refillCooldownUntil;
	private int cutFailures;
	/** When each cactus was cut (or found dry up close): from afar only that says when it is back. */
	private final Map<WorldPoint, Long> cactusCutAt = new HashMap<>();
	/** The last cut of ours, success or not: waiting at the cacti for a regrowth counts from it. */
	private long lastCactusAt;
	/** Paused by a break handler or logged out since the last tick acted on. */
	private volatile boolean interrupted;
	/** The varied rhythm: cycles until the next hesitation and how many it still skips; breathers. */
	private int cyclesToSlip;
	private int slipCycles;
	private long breatherUntil;
	private long nextBreatherAt;

	MiningEvents events()
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
		return s == null || events.miningXp() == 0 ? 0 : events.miningXp() - s.startXp;
	}

	public boolean run(TickMinerConfig config)
	{
		this.config = config;
		delay = config.delay();
		// The client's antiban (its panel, its natural mouse profile, micro breaks): tell it what we
		// do. Its actionCooldown is never called: any pause mid-cycle loses the cycle.
		Rs2Antiban.setActivity(Activity.GENERAL_MINING);
		session = null;
		anchorTick = -1;
		handledTick = -1;
		countedDropTick = -1;
		lastRock = null;
		lastRockTick = -1;
		depositing = false;
		depositFailures = 0;
		refilling = false;
		refillCooldownUntil = 0;
		cutFailures = 0;
		cactusCutAt.clear();
		lastCactusAt = 0;
		clicks = new Clicks(config.mouse());
		cyclesToSlip = nextSlip();
		slipCycles = 0;
		breatherUntil = 0;
		nextBreatherAt = System.currentTimeMillis() + nextBreatherGap();
		testResult = Boolean.getBoolean("microbot.test.mode") ? new TestResult("Tick Miner") : null;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				// A break handler pauses scripts (and may log out): afterwards the cycle is stale.
				if (!Microbot.isLoggedIn() || !super.run())
				{
					interrupted = true;
					return;
				}
				if (interrupted)
				{
					interrupted = false;
					anchorTick = -1;
					lastRock = null;
					log.info("Tick Miner: resuming after a pause or logout");
				}
				// Once per tick, as early in it as we can. A step that ran into the next tick (a
				// slow natural movement) does not wait for another: that tick is acted on now.
				if (events.tick() == handledTick && !Global.sleepUntilNextTick(2000)) return;
				handledTick = events.tick();
				loop();
				report(false);
			}
			catch (Exception ex)
			{
				log.error("Tick Miner loop failed", ex);
			}
		}, 0, 1, TimeUnit.MILLISECONDS);
		return true;
	}

	@Override
	public void shutdown()
	{
		super.shutdown();
		Rs2Antiban.resetAntibanSettings();
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
		if (refilling)
		{
			// A cut needs no free slot: a full inventory waits for the trip to end (0.2.3 left the
			// cacti for the grinder, 30 tiles west, and walked back).
			refill(s);
			return;
		}
		if (depositing || (Rs2Inventory.isFull() && sandstoneCount() > 0))
		{
			deposit(s);
			return;
		}
		if (Rs2Inventory.isFull())
		{
			s.stop("Inventory full of something else than sandstone");
			return;
		}
		if (needsWater())
		{
			refill(s);
			return;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return;
		}
		Map<WorldPoint, Rs2TileObjectModel> up = rocksUp();
		if (!Rocks.inCorridor(me))
		{
			approach(s, me);
			return;
		}
		s.miningTicks++;
		if (config.threeTick() && delayReady(s) && !breather(s, me, up))
		{
			cycle(s, me, up);
		}
		else if (!config.threeTick() || s.delayLost != null)
		{
			Microbot.status = s.delayLost != null ? "Tick Miner: mining plainly, " + s.delayLost : "Tick Miner: mining";
			plain(s, me, up);
		}
	}

	/** Checks level, place and gear, then starts the run; false when it cannot run. */
	private boolean start()
	{
		int level = Rs2Player.getRealSkillLevel(Skill.MINING);
		if (level <= 0)
		{
			// Just logged in: the skills are not loaded yet.
			return false;
		}
		if (level < MINING_LEVEL)
		{
			stop("Sandstone needs " + MINING_LEVEL + " Mining (have " + level + ")");
			return false;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return false;
		}
		if (me.distanceTo(Rocks.SOUTH) > START_RADIUS)
		{
			stop("Start at the Desert Quarry, by Drew's grinder");
			return false;
		}
		if (!hasPickaxe())
		{
			stop("No pickaxe worn or carried");
			return false;
		}
		if (!heatSafe())
		{
			stop("No water: bring waterskins (or wear a Desert amulet 4 or a charged circlet of water)");
			return false;
		}
		herbloreLevel = Rs2Player.getRealSkillLevel(Skill.HERBLORE);
		if (config.threeTick() && missingDelay() != null)
		{
			// At the start a missing delay item is a setup to fix, not a run to mine plainly.
			stop("3-tick needs " + missingDelay());
			return false;
		}
		int xp = skillXp(Skill.MINING);
		events.onMining(level, xp);
		int[] hp = Microbot.getClientThread().runOnClientThreadOptional(() -> new int[]{
			Microbot.getClient().getBoostedSkillLevel(Skill.HITPOINTS), Microbot.getClient().getRealSkillLevel(Skill.HITPOINTS)})
			.orElse(new int[]{0, 0});
		events.onHitpoints(hp[0], hp[1]);
		session = new Session(events.tick(), xp);
		log.info("Tick Miner {}: Mining {}, HP {}/{}, water {} sips, 3-tick {} ({}), mouse {}", TickMinerPlugin.VERSION,
			level, hp[0], hp[1], sips(), config.threeTick(), delay, config.mouse());
		return true;
	}

	private void checkLimits(Session s)
	{
		int level = config.stopAtGoal() ? config.stopAtLevel() : 0;
		if (level > 0 && events.miningLevel() >= level)
		{
			s.stop("Reached Mining " + events.miningLevel());
		}
		int minutes = config.stopAfterMinutes();
		if (minutes > 0 && System.currentTimeMillis() - s.startedAt >= minutes * 60_000L)
		{
			s.stop("Ran for " + minutes + " minutes");
		}
		int hp = events.hitpoints();
		int max = events.maxHitpoints();
		if (max > 0 && hp > 0 && hp * 100 < max * config.minHitpointsPercent())
		{
			s.stop("Hitpoints " + hp + "/" + max);
		}
		s.waterSips = sips();
		if (!heatSafe())
		{
			s.stop("Out of water: the waterskins are empty");
		}
		if (!hasPickaxe())
		{
			s.stop("No pickaxe worn or carried");
		}
		if (testResult != null && s.mined >= config.testMines())
		{
			s.stop("Test: " + s.mined + " sandstone mined");
		}
	}

	// ----- the cycle ---------------------------------------------------------------------------

	/** One tick of the 3-tick cycle. Phase 0 is the tick the previous roll lands on. */
	private void cycle(Session s, WorldPoint me, Map<WorldPoint, Rs2TileObjectModel> up)
	{
		int tick = events.tick();
		if (anchorTick < 0)
		{
			anchorTick = tick;
		}
		countDrop(s, tick, true);
		int phase = Math.floorMod(tick - anchorTick, 3);
		// The delay: the tool (knife or herb) used on the target (teak log or swamp tar).
		Rs2ItemModel tool = delayTool();
		Rs2ItemModel delayOn = Rs2Inventory.get(delay == DelayMethod.TEAK ? ItemID.TEAK_LOGS : ItemID.SWAMP_TAR);
		if (tool == null || delayOn == null)
		{
			return;
		}
		switch (phase)
		{
			case 0:
				if (hesitate(s))
				{
					Microbot.status = "Tick Miner: hesitating";
					break;
				}
				WorldPoint rock = Rocks.next(me, up.keySet(), lastRock, coin());
				if (rock == null)
				{
					// All four down (another miner, or slow respawns): this tick becomes phase 0
					// again, so the cycle starts the moment one is back.
					Microbot.status = "Tick Miner: waiting for a rock";
					s.rockWaitTicks++;
					anchorTick = -1;
					break;
				}
				Microbot.status = "Tick Miner: 3-tick mining";
				clicks.burst(true);
				selectTool(tool);
				clicks.useOn(delayOn);
				boolean delayInTime = events.tick() == tick;
				clicks.object(up.get(rock), "Mine");
				clicks.burst(false);
				int late = events.tick() - tick;
				s.cycles++;
				if (!Rocks.standTile(rock).equals(me))
				{
					s.steps++;
				}
				lastRock = rock;
				lastRockTick = tick;
				if (late == 0)
				{
					s.onTime++;
				}
				else if (delayInTime && late == 1)
				{
					// Still in time: the rock click may land on the tick after the delay.
					s.rockNextTick++;
				}
				break;
			case 1:
				// Selecting is client-side only: nothing reaches the server until the roll.
				selectTool(tool);
				break;
			default:
				selectTool(tool);
				clicks.hover(delayOn);
				break;
		}
	}

	/**
	 * Counts a new XP drop. While cycling, a drop off phase 0 on the tick it landed (after a
	 * hesitation, a breather or a lost click) moves the anchor to it: the skilling timer is at -1 on
	 * the tick after a roll, so the drop tick is phase 0.
	 */
	private void countDrop(Session s, int tick, boolean cycling)
	{
		int drop = events.lastDropTick();
		if (drop <= countedDropTick)
		{
			return;
		}
		countedDropTick = drop;
		s.mined++;
		if (!cycling)
		{
			return;
		}
		int dropPhase = Math.floorMod(drop - anchorTick, 3);
		if (dropPhase != 0 && drop == tick)
		{
			log.info("Tick Miner: XP drop on phase {} at tick {}, re-anchoring", dropPhase, tick);
			s.realigns++;
		}
		else
		{
			s.dropsByPhase[dropPhase]++;
		}
		if (drop == tick)
		{
			anchorTick = drop;
		}
	}

	/**
	 * Plain mining: click a rock (the other row's first, for the second roll) when the one clicked
	 * last went down, or nothing has swung since a click two ticks ago.
	 */
	private void plain(Session s, WorldPoint me, Map<WorldPoint, Rs2TileObjectModel> up)
	{
		int tick = events.tick();
		countDrop(s, tick, false);
		anchorTick = -1;
		boolean lastUp = lastRock != null && up.containsKey(lastRock);
		if (lastUp && (tick - lastRockTick < 3 || Rs2Player.isAnimating()))
		{
			return;
		}
		WorldPoint rock = Rocks.next(me, up.keySet(), lastRock, coin());
		if (rock == null)
		{
			s.rockWaitTicks++;
			return;
		}
		deselect();
		clicks.object(up.get(rock), "Mine");
		lastRock = rock;
		lastRockTick = tick;
	}

	/**
	 * A cycle skipped on purpose now and then: the roll still comes, on the pickaxe's timer, and
	 * the cycle finds its step again on the next off-cycle drop.
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

	/** Every few minutes, mine plainly for a while; true while it lasts. */
	private boolean breather(Session s, WorldPoint me, Map<WorldPoint, Rs2TileObjectModel> up)
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
			// A pause in the cycle anyway: the panel's micro break (the break handler pauses us)
			// and mouse off screen, when turned on there.
			if (!Rs2Antiban.takeMicroBreakByChance() && Rs2AntibanSettings.moveMouseOffScreen)
			{
				Rs2Antiban.moveMouseOffScreen(Rs2AntibanSettings.moveMouseOffScreenChance);
			}
		}
		if (now >= breatherUntil)
		{
			return false;
		}
		Microbot.status = "Tick Miner: mining plainly for a bit";
		plain(s, me, up);
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

	private static boolean coin()
	{
		return Rs2Random.betweenInclusive(0, 1) == 1;
	}

	/** The spot's sandstone rocks that are up (a mined one turns into plain rocks until it respawns). */
	private static Map<WorldPoint, Rs2TileObjectModel> rocksUp()
	{
		Map<WorldPoint, Rs2TileObjectModel> up = new HashMap<>();
		for (Rs2TileObjectModel rock : Microbot.getRs2TileObjectCache().query()
			.withId(SANDSTONE_ROCKS)
			.where(o -> Rocks.isSpotRock(o.getWorldLocation()))
			.toListOnClientThread())
		{
			up.put(rock.getWorldLocation(), rock);
		}
		return up;
	}

	// ----- the delay ---------------------------------------------------------------------------

	/**
	 * The delay items are there. Once one is used up mid-run (a rock click that did not land and
	 * the fletch or tar completed), the run goes on mining plainly: the sandstone still comes.
	 */
	private boolean delayReady(Session s)
	{
		if (s.delayLost != null)
		{
			return false;
		}
		String missing = missingDelay();
		if (missing == null)
		{
			return true;
		}
		s.stocksMade += Rs2Inventory.count(ItemID.XBOWS_CROSSBOW_STOCK_TEAK);
		s.delayLost = "no " + missing;
		log.warn("Tick Miner: 3-tick delay lost ({}), mining plainly", missing);
		deselect();
		return false;
	}

	/** What the 3-tick delay lacks, null when nothing. */
	private String missingDelay()
	{
		if (delay == DelayMethod.TEAK)
		{
			if (!Rs2Inventory.hasItem(ItemID.KNIFE))
			{
				return "a knife";
			}
			// Exactly one log: with more, knife on teak opens a Make-X and the tick is lost.
			int logs = Rs2Inventory.count(ItemID.TEAK_LOGS);
			return logs == 1 ? null : logs == 0 ? "a teak log" : "exactly one teak log (" + logs + " open the Make-X)";
		}
		if (!Rs2Inventory.hasItem(ItemID.PESTLE_AND_MORTAR))
		{
			return "a pestle and mortar";
		}
		if (Rs2Inventory.itemQuantity(ItemID.SWAMP_TAR) < MIN_SWAMP_TAR)
		{
			return MIN_SWAMP_TAR + " swamp tar";
		}
		if (tarHerbCount() != 1 || delayTool() == null)
		{
			return "exactly one clean guam leaf (or a marrentill, tarromin, harralander your Herblore can use)";
		}
		return null;
	}

	/** The knife, or the one clean tar herb (several open a Make-X). */
	private Rs2ItemModel delayTool()
	{
		if (delay == DelayMethod.TEAK)
		{
			return Rs2Inventory.get(ItemID.KNIFE);
		}
		for (int i = 0; i < TAR_HERBS.length; i++)
		{
			if (herbloreLevel >= TAR_HERB_LEVELS[i] && Rs2Inventory.hasItem(TAR_HERBS[i]))
			{
				return Rs2Inventory.get(TAR_HERBS[i]);
			}
		}
		return null;
	}

	private int tarHerbCount()
	{
		int count = 0;
		for (int id : TAR_HERBS)
		{
			count += Rs2Inventory.count(id);
		}
		return count;
	}

	private void selectTool(Rs2ItemModel item)
	{
		if (!(Rs2Inventory.isItemSelected() && Rs2Inventory.getSelectedItemId() == item.getId()))
		{
			clicks.select(item);
		}
	}

	/**
	 * Drops the item selection. Client-side only, as the selection itself: a "Use" on the selected
	 * knife again (Rs2Inventory.deselect) kept it selected, and the grinder click went out as knife
	 * on the grinder ("Nothing interesting happens", 0.1.0).
	 */
	private static void deselect()
	{
		if (Rs2Inventory.isItemSelected())
		{
			Microbot.getClientThread().runOnClientThreadOptional(() -> {
				Microbot.getClient().setWidgetSelected(false);
				return true;
			});
		}
	}

	// ----- the spot and the grinder ------------------------------------------------------------

	/**
	 * Walks into the corridor: from afar with the walker, from the grinder or the cacti with a walk
	 * click on the corridor tile, then starts a fresh cycle.
	 */
	private void approach(Session s, WorldPoint me)
	{
		Microbot.status = "Tick Miner: walking to the rocks";
		anchorTick = -1;
		deselect();
		if (me.distanceTo(Rocks.SOUTH) > CLICK_RADIUS)
		{
			Rs2Walker.walkTo(Rocks.SOUTH, 0);
			return;
		}
		// Into the corridor itself, at the end nearer to us (it opens north and south). A click on a
		// rock from the grinder's side stopped on the tile north of the north-west rock, out of the
		// corridor, and 0.3.0 clicked that rock again every 25 s.
		WorldPoint target = me.getY() >= Rocks.NORTH.getY() ? Rocks.NORTH : Rocks.SOUTH;
		s.approaches++;
		if (!Rs2Walker.walkFastCanvas(target))
		{
			Rs2Walker.walkTo(target, 0);
			return;
		}
		// 32 tiles from the cacti: about 10 s running, 20 s walking. Standing still out of the
		// corridor for two ticks: the walk ended short, the next loop walks again.
		WorldPoint[] last = {me};
		long[] since = {System.currentTimeMillis()};
		Global.sleepUntil(() ->
		{
			WorldPoint at = Rs2Player.getWorldLocation();
			if (Rocks.inCorridor(at))
			{
				return true;
			}
			long now = System.currentTimeMillis();
			if (at == null || !at.equals(last[0]))
			{
				last[0] = at;
				since[0] = now;
				return false;
			}
			return now - since[0] >= 1200;
		}, 25_000);
	}

	/**
	 * Puts the sandstone into the Sandstorm. The grinder keeps it as buckets of sand (1, 2, 4 or 8
	 * by size), claimed from Drew later with empty buckets and 50 coins each.
	 */
	private void deposit(Session s)
	{
		Microbot.status = "Tick Miner: depositing sandstone";
		anchorTick = -1;
		int[] before = sandstoneCounts();
		int total = sum(before);
		if (total == 0)
		{
			depositing = false;
			return;
		}
		if (!depositing)
		{
			// The trip is a natural pause: the panel's micro break, when turned on there.
			Rs2Antiban.takeMicroBreakByChance();
		}
		depositing = true;
		deselect();
		Rs2TileObjectModel grinder = Microbot.getRs2TileObjectCache().query().withId(GRINDER).nearestOnClientThread();
		if (grinder == null || !grinder.click("Deposit")
			|| !Global.sleepUntil(() -> sandstoneCount() < total, 15_000))
		{
			log.info("Tick Miner: the grinder took no sandstone ({} in the inventory)", total);
			if (++depositFailures >= MAX_DEPOSIT_FAILURES)
			{
				s.stop("Could not deposit the sandstone in the grinder");
			}
			return;
		}
		// The rest of it, should the grinder take one piece at a time.
		Global.sleepUntil(() -> sandstoneCount() == 0, 3000);
		int[] after = sandstoneCounts();
		for (int i = 0; i < SANDSTONE.length; i++)
		{
			int left = Math.max(0, before[i] - after[i]);
			s.deposited[i] += left;
			s.sandStored += left * SAND_PER_SANDSTONE[i];
		}
		depositFailures = 0;
		if (sum(after) == 0)
		{
			depositing = false;
			s.deposits++;
			log.info("Tick Miner: deposited {} sandstone, {} buckets of sand stored this run", total, s.sandStored);
		}
	}

	private static int[] sandstoneCounts()
	{
		int[] counts = new int[SANDSTONE.length];
		for (int i = 0; i < SANDSTONE.length; i++)
		{
			counts[i] = Rs2Inventory.count(SANDSTONE[i]);
		}
		return counts;
	}

	private static int sandstoneCount()
	{
		return sum(sandstoneCounts());
	}

	private static int sum(int[] values)
	{
		int total = 0;
		for (int v : values)
		{
			total += v;
		}
		return total;
	}

	private static boolean hasPickaxe()
	{
		return Rs2Equipment.isWearing("pickaxe") || Rs2Inventory.hasItem("pickaxe");
	}

	/** Waterskins with water, or an item that keeps the desert heat away. */
	private static boolean heatSafe()
	{
		return sips() > 0 || heatImmune();
	}

	private static boolean heatImmune()
	{
		return Rs2Equipment.isWearing(ItemID.DESERT_AMULET_ELITE, ItemID.WATER_CIRCLET_CHARGED);
	}

	/** Sips all the waterskins carried hold when full. */
	private static int waterCapacity()
	{
		int skins = 0;
		for (int id : ALL_WATERSKINS)
		{
			skins += Rs2Inventory.count(id);
		}
		return 4 * skins;
	}

	/** Time for a trip to the cacti ({@link Water#worthATrip}), when refilling is on. */
	private boolean needsWater()
	{
		if (!config.refillWater() || heatImmune() || System.currentTimeMillis() < refillCooldownUntil)
		{
			return false;
		}
		int sips = sips();
		if (sips > Water.LOW_SIPS)
		{
			// Not worth a trip whatever the cacti: no scene scan for them.
			return false;
		}
		return Water.worthATrip(sips, waterCapacity(), cactiUp(Rs2Player.getWorldLocation(), healthyCacti()));
	}

	/**
	 * Cacti that can be cut: in sight, the ones the client shows healthy; from afar, where its view
	 * is stale, the ones not cut (or found dry) within the last minute.
	 */
	private int cactiUp(WorldPoint me, Map<WorldPoint, Rs2TileObjectModel> healthy)
	{
		long now = System.currentTimeMillis();
		int up = 0;
		for (WorldPoint cactus : Water.CACTI)
		{
			if (me != null && me.distanceTo(cactus) <= Water.SIGHT)
			{
				up += healthy.containsKey(cactus) ? 1 : 0;
				continue;
			}
			Long cut = cactusCutAt.get(cactus);
			up += cut == null || now - cut >= CACTUS_REGROW_MS ? 1 : 0;
		}
		return up;
	}

	/**
	 * One step of a trip to the cacti east of the quarry: walk up to them (from afar the client's
	 * view of them is stale), cut the nearest healthy one and wait for its sip or its failed-cut
	 * message; either dries it. The trip ends when the skins are full or no cactus in sight is
	 * healthy; the loop then walks back to the rocks.
	 */
	private void refill(Session s)
	{
		if (!refilling)
		{
			refilling = true;
			cutFailures = 0;
			s.refillTrips++;
			log.info("Tick Miner: refilling the waterskins ({} of {} sips)", sips(), waterCapacity());
		}
		Microbot.status = "Tick Miner: refilling the waterskins at the cacti";
		anchorTick = -1;
		lastRock = null;
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return;
		}
		if (me.distanceTo(Water.STAND) > Water.SIGHT / 2)
		{
			deselect();
			Rs2Walker.walkTo(Water.STAND, 2);
			return;
		}
		Map<WorldPoint, Rs2TileObjectModel> healthy = healthyCacti();
		long now = System.currentTimeMillis();
		for (WorldPoint cactus : Water.CACTI)
		{
			Long cut = cactusCutAt.get(cactus);
			if (!healthy.containsKey(cactus) && (cut == null || now - cut >= CACTUS_REGROW_MS))
			{
				// Dry in sight and not by our cut: someone else's, a minute at most from now.
				cactusCutAt.put(cactus, now);
			}
		}
		if (healthy.isEmpty() && sips() <= Water.LOW_SIPS && sips() < waterCapacity() && cutFailures < MAX_CUT_FAILURES
			&& now - lastCactusAt < CACTUS_REGROW_MS + 15_000)
		{
			// Still under the reserve: about one cut in three gives a sip, and walking back dry makes
			// another trip soon. The cactus cut last is back within the minute: wait for it here.
			Microbot.status = "Tick Miner: waiting at the cacti for one to regrow";
			return;
		}
		if (sips() >= waterCapacity() || healthy.isEmpty() || cutFailures >= MAX_CUT_FAILURES)
		{
			refilling = false;
			refillCooldownUntil = System.currentTimeMillis() + CACTUS_REGROW_MS;
			log.info("Tick Miner: waterskins at {} of {} sips ({} cut this run)", sips(), waterCapacity(), s.sipsRefilled);
			return;
		}
		WorldPoint at = null;
		for (WorldPoint cactus : healthy.keySet())
		{
			if (at == null || me.distanceTo(cactus) < me.distanceTo(at))
			{
				at = cactus;
			}
		}
		deselect();
		int topUpsBefore = events.cactusTopUps();
		int failsBefore = events.cactusFails();
		if (!healthy.get(at).click("Cut"))
		{
			cutFailures++;
			return;
		}
		// The walk there first: 32 tiles from the rocks, longer without run. The chat says how the
		// cut went: the cactus's object can drop out of a query for a moment (0.2.0 counted a
		// failed cut as a sip that way).
		// A few tiles away now: a cut answers within 2-3 ticks. One clicked as the walk ended got
		// no answer at all (0.2.2 then waited 10 s).
		Global.sleepUntil(() -> events.cactusTopUps() > topUpsBefore || events.cactusFails() > failsBefore, 6000);
		if (events.cactusTopUps() > topUpsBefore || events.cactusFails() > failsBefore)
		{
			s.sipsRefilled += events.cactusTopUps() - topUpsBefore;
			s.cactusFails += events.cactusFails() - failsBefore;
			lastCactusAt = System.currentTimeMillis();
			cactusCutAt.put(at, lastCactusAt);
			cutFailures = 0;
		}
		else
		{
			cutFailures++;
		}
	}

	/** The cacti east of the quarry that can be cut now. */
	private static Map<WorldPoint, Rs2TileObjectModel> healthyCacti()
	{
		Map<WorldPoint, Rs2TileObjectModel> healthy = new HashMap<>();
		for (Rs2TileObjectModel cactus : Microbot.getRs2TileObjectCache().query()
			.withId(CACTUS)
			.where(o -> Water.isCactus(o.getWorldLocation()))
			.toListOnClientThread())
		{
			healthy.put(cactus.getWorldLocation(), cactus);
		}
		return healthy;
	}

	/** Sips left in the waterskins (the game drinks one by itself at each heat tick). */
	private static int sips()
	{
		int sips = 0;
		for (int i = 0; i < WATERSKINS.length; i++)
		{
			sips += (i + 1) * Rs2Inventory.count(WATERSKINS[i]);
		}
		return sips;
	}

	// ----- stopping and reporting --------------------------------------------------------------

	private void stop(String reason)
	{
		Session s = session;
		if (config.threeTick() && s != null && s.delayLost == null && Microbot.isLoggedIn())
		{
			// A knife-on-teak or a herb tar may still be running: a rock click cancels it, or it
			// completes and takes the log (or 15 tar and the herb).
			WorldPoint me = Rs2Player.getWorldLocation();
			Map<WorldPoint, Rs2TileObjectModel> up = rocksUp();
			WorldPoint rock = me == null ? null : Rocks.next(me, up.keySet(), null, coin());
			if (rock != null && me.distanceTo(rock) <= 2)
			{
				deselect();
				clicks.object(up.get(rock), "Mine");
			}
		}
		Microbot.status = "Tick Miner stopped: " + reason;
		log.info("Tick Miner stopped: {}", reason);
		report(true, reason);
		if (testResult != null)
		{
			finishTest(reason);
			return;
		}
		if (config.logoutOnStop() && Microbot.isLoggedIn())
		{
			// Left logged in, the desert heat hurts once the water is gone, and a break handler logs
			// straight back in after an idle logout. This logout tells them it was meant.
			Global.sleepUntilNextTick(1200);
			log.info("Tick Miner: logging out ({})", Rs2Player.logoutWithoutAutoLogin());
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
		progress.put("delay", delay == null ? null : delay.name());
		progress.put("mouse", config.mouse().name());
		progress.put("miningLevel", events.miningLevel());
		progress.put("miningXpGained", xpGained());
		progress.put("hitpoints", events.hitpoints());
		progress.put("waterSips", sips());
		if (s != null)
		{
			progress.putAll(s.counters());
			progress.put("ticks", events.tick() - s.startTick);
		}
		progress.put("stopReason", stopReason);
		ScriptResultStore.submit(TickMinerPlugin.class.getName(), progress);
	}

	// Test mode protocol (docs/AGENTIC_TESTING_LOOP.md): record checks, write result.json, exit.
	private void finishTest(String reason)
	{
		Session s = session;
		int mined = s == null ? 0 : s.mined;
		int xp = xpGained();
		check("sandstone mined: " + mined + "/" + config.testMines(), mined >= config.testMines(), reason);
		check("mining xp gained: " + xp, xp > 0, "no Mining XP gained");
		if (config.threeTick())
		{
			int onTime = s == null ? 0 : s.onTimePercent();
			int inStep = s == null ? 0 : s.inStepPercent();
			check("cycles on time: " + onTime + "%", onTime >= 90, "clicks spilled into the next tick");
			check("xp drops in step: " + inStep + "%", inStep >= 90, "rolls not on the 3-tick cycle");
		}
		testResult.complete("completed");
		TestResultWriter.write(testResult);
		log.info("Tick Miner test finished: {}/{} checks passed", testResult.passedCount(), testResult.checks.size());
		System.exit(testResult.exitCode);
	}

	private void check(String name, boolean passed, String failure)
	{
		testResult.addCheck(name, passed, passed ? null : failure);
	}

	private static int skillXp(Skill skill)
	{
		return Microbot.getClientThread()
			.runOnClientThreadOptional(() -> Microbot.getClient().getSkillExperience(skill))
			.orElse(0);
	}
}
