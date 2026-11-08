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
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * 3-tick rod fishing, fly at Shilo Village or barbarian at Otto's Grotto. The loop runs once per
 * game tick, one phase of the cycle (wiki, Tick manipulation): on phase 0 knife on teak (or eating
 * roe or caviar) sets the skilling timer to 3 and the spot click turns it into a fishing roll 3
 * ticks later, instead of 5. Phase 1 drops or cuts a fish, phase 2 brings the cursor onto the
 * teak log or the roe, so phase 0 starts with a click where the cursor already is. The teak log
 * is never used up: the spot click cancels the fletch.
 */
@Slf4j
public class TickFisherScript extends Script
{
	/** Farther than this from the place's centre at start: not there. */
	private static final int START_RADIUS = 40;
	/** Eaten as the 3-tick delay in place of knife on teak (food: the timer goes to 3). */
	private static final int[] ROE = {ItemID.BRUT_ROE, ItemID.BRUT_CAVIAR};
	/** Cuts in a row that left the fish in place: cutting does not work here (Cooking level?). */
	private static final int MAX_CUT_FAILURES = 3;
	/** Clean herbs that make a tar, and the Herblore level each needs. */
	private static final int[] TAR_HERBS = {ItemID.GUAM_LEAF, ItemID.MARENTILL, ItemID.TARROMIN, ItemID.HARRALANDER};
	private static final int[] TAR_HERB_LEVELS = {19, 31, 39, 44};
	/** Making a tar takes 15 swamp tar; with fewer the action fails and sets no timer. */
	private static final int MIN_SWAMP_TAR = 15;

	private final FishingEvents events = new FishingEvents();
	private Clicks clicks;
	private TickFisherConfig config;
	private FishingPlace place;
	private DelayMethod delay;
	private int herbloreLevel;
	// Read by the overlay on the client thread.
	private volatile Session session;
	private TestResult testResult;
	private long lastReport;

	/** The spot being fished, its tile when picked, and the tick phase 0 counts from. */
	private Rs2NpcModel spot;
	private WorldPoint spotTile;
	/**
	 * Where the game put us to fish the spot when that is not next to it (a spot off the shore is
	 * fished from two tiles away), so standing there counts as next to it.
	 */
	private WorldPoint reachTile;
	private int anchorTick = -1;
	/** The last tick the loop acted on, and the last XP drop tick counted. */
	private int handledTick = -1;
	private int countedDropTick = -1;
	private int bankFailures;
	private int missingTicks;
	/** Paused by a break handler or logged out since the last tick acted on. */
	private volatile boolean interrupted;
	/** Fish count before the knife went on a fish (-1: no cut pending), misses in a row, given up. */
	private int fishBeforeCut = -1;
	private int cutFailures;
	private boolean cuttingFailed;
	/** The lookout being walked to or watched from (-1: none yet), and when to try the other. */
	private int lookout = -1;
	private long lookoutUntil;
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
		place = config.place();
		delay = config.delay();
		session = null;
		spot = null;
		anchorTick = -1;
		handledTick = -1;
		countedDropTick = -1;
		bankFailures = 0;
		lookout = -1;
		fishBeforeCut = -1;
		cutFailures = 0;
		cuttingFailed = false;
		clicks = new Clicks(config.mouse());
		cyclesToSlip = nextSlip();
		slipCycles = 0;
		breatherUntil = 0;
		nextBreatherAt = System.currentTimeMillis() + nextBreatherGap();
		testResult = Boolean.getBoolean("microbot.test.mode") ? new TestResult("Tick Fisher") : null;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				// A break handler pauses scripts (and may log out): afterwards the spot and the
				// cycle are stale (after a logout the NPCs are new objects), so start them over.
				if (!Microbot.isLoggedIn() || !super.run())
				{
					interrupted = true;
					return;
				}
				if (interrupted)
				{
					interrupted = false;
					spot = null;
					anchorTick = -1;
					lookout = -1;
					log.info("Tick Fisher: resuming after a pause or logout");
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
		if (!hasGear() || (place.bank != null && Rs2Bank.isOpen()))
		{
			if (place.bank == null)
			{
				// Confirmed over a few ticks: read mid-batch the inventory once lacked the teak
				// log it held (0.8.1 stopped with the log in place).
				if (++missingTicks >= 3)
				{
					restock(s);
				}
				return;
			}
			gearUp(s);
			return;
		}
		missingTicks = 0;
		if (Rs2Inventory.isFull())
		{
			if (config.fishHandling() == FishHandling.BANK)
			{
				bankFish(s);
			}
			else if (cutting())
			{
				cutAllFish(s);
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
			waitForSpot(s);
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
		if (level < place.fishingLevel)
		{
			stop(place + " needs " + place.fishingLevel + " Fishing (have " + level + ")");
			return false;
		}
		int agility = Rs2Player.getRealSkillLevel(Skill.AGILITY);
		int strength = Rs2Player.getRealSkillLevel(Skill.STRENGTH);
		if (agility < place.agilityStrengthLevel || strength < place.agilityStrengthLevel)
		{
			stop(place + " needs " + place.agilityStrengthLevel + " Agility and Strength");
			return false;
		}
		if (config.fishHandling() == FishHandling.BANK && place.bank == null)
		{
			stop("No bank at " + place + ": set Fish to Drop or Cut");
			return false;
		}
		if (config.fishHandling() == FishHandling.CUT && place.bank != null)
		{
			stop("Cut is for barbarian fish: set Fish to Drop or Bank");
			return false;
		}
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return false;
		}
		if (me.distanceTo(place.centre) > START_RADIUS)
		{
			stop("Start at " + place);
			return false;
		}
		herbloreLevel = Rs2Player.getRealSkillLevel(Skill.HERBLORE);
		int xp = fishingXp();
		events.onFishing(level, xp);
		events.onAgility(agility);
		session = new Session(events.tick(), xp);
		log.info("Tick Fisher {}: {}, Fishing {}, Agility {}, 3-tick {} ({}), fish {}, mouse {}", TickFisherPlugin.VERSION, place, level, agility, config.threeTick(), delay, config.fishHandling(), config.mouse());
		return true;
	}

	private void checkLimits(Session s)
	{
		int level = config.stopAtLevel();
		if (level > 0 && events.fishingLevel() >= level)
		{
			s.stop("Reached Fishing " + events.fishingLevel());
		}
		int agility = config.stopAtAgilityLevel();
		if (agility > 0 && events.agilityLevel() >= agility)
		{
			s.stop("Reached Agility " + events.agilityLevel());
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
				// Off the cycle: after a hesitation, a breather or a lost click.
				log.info("Tick Fisher: XP drop on phase {} at tick {}, re-anchoring", dropPhase, tick);
				s.realigns++;
			}
			else
			{
				s.dropsByPhase[dropPhase]++;
			}
			if (drop == tick)
			{
				// The skilling timer is at -1 on the tick after a roll: phase 0 is the drop tick.
				anchorTick = drop;
			}
		}
		int phase = Math.floorMod(tick - anchorTick, 3);
		// The delay: the tool (knife or herb) used on the target (teak log or swamp tar).
		Rs2ItemModel tool = delayTool();
		Rs2ItemModel delayOn = Rs2Inventory.get(delay == DelayMethod.TEAK ? ItemID.TEAK_LOGS : ItemID.SWAMP_TAR);
		Rs2ItemModel knife = Rs2Inventory.get(ItemID.KNIFE);
		if (tool == null || delayOn == null)
		{
			return;
		}
		// Cut-eat: roe or caviar from a cut fish is the delay while there is one.
		Rs2ItemModel roe = cutting() ? Rs2Inventory.get(ROE) : null;
		switch (phase)
		{
			case 0:
				if (hesitate(s))
				{
					Microbot.status = "Tick Fisher: hesitating";
					break;
				}
				Microbot.status = "Tick Fisher: 3-tick fishing";
				// Dropping or cutting a fish removes the interaction with the spot (guide, 2t teaks:
				// "log dropping"). Done on the tick after the spot click it lost the roll after each
				// catch (0.4-0.7: never two catches 3 ticks apart in 400+, ~47-52k XP/h at 58). A
				// 3-tick player does it on this tick in the order delay, knife on the fish, spot
				// (cut-eat video and the guide): 3-4 clicks in one tick (measured: ~74k at 60).
				clicks.burst(true);
				if (roe != null)
				{
					deselect();
					clicks.eat(roe);
					s.roeEaten++;
				}
				if (roe == null)
				{
					selectTool(tool);
					clicks.useOn(delayOn);
				}
				Rs2ItemModel fish = config.fishHandling() != FishHandling.BANK ? Rs2Inventory.get(place.fish) : null;
				if (fish != null && cutting() && knife != null)
				{
					fishBeforeCut = fishCount();
					clicks.select(knife);
					clicks.useOn(fish);
				}
				else if (fish != null)
				{
					clicks.drop(fish);
					s.fishDropped++;
				}
				boolean delayInTime = events.tick() == tick;
				clicks.npc(target, place.spotAction);
				clicks.burst(false);
				int late = events.tick() - tick;
				s.cycles++;
				if (late == 0)
				{
					s.onTime++;
				}
				else if (delayInTime && late == 1)
				{
					// Still in time: the spot click may land on the tick after the delay.
					s.spotNextTick++;
				}
				break;
			case 1:
				// Nothing that touches the inventory until the roll: selecting is client-side only.
				checkCut(s);
				if (roe == null)
				{
					selectTool(tool);
				}
				break;
			default:
				if (roe != null)
				{
					// Clicking the roe with an item selected would use that item on it.
					deselect();
					clicks.hover(roe);
					break;
				}
				selectTool(tool);
				clicks.hover(delayOn);
				break;
		}
	}

	/** Cut-eat, unless cutting turned out not to work. */
	private boolean cutting()
	{
		return config.fishHandling() == FishHandling.CUT && !cuttingFailed;
	}

	/** After a cut on phase 1: one fish fewer now, or the cut did nothing (dropping instead after a few). */
	private void checkCut(Session s)
	{
		if (fishBeforeCut < 0)
		{
			return;
		}
		if (fishCount() < fishBeforeCut)
		{
			s.fishCut++;
			cutFailures = 0;
		}
		else if (++cutFailures >= MAX_CUT_FAILURES)
		{
			cuttingFailed = true;
			log.warn("Tick Fisher: {} cuts in a row left the fish, dropping them instead (Cooking level?)", cutFailures);
		}
		fishBeforeCut = -1;
	}

	private int fishCount()
	{
		int count = 0;
		for (int id : place.fish)
		{
			count += Rs2Inventory.count(id);
		}
		return count;
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
			clicks.npc(target, place.spotAction);
		}
	}

	/** The knife, or the one clean tar herb (the first usable one; several open a Make-X). */
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

	private void selectTool(Rs2ItemModel item)
	{
		if (!(Rs2Inventory.isItemSelected() && Rs2Inventory.getSelectedItemId() == item.getId()))
		{
			clicks.select(item);
		}
	}

	/** "Use" on the selected item again drops the selection (what Rs2Inventory.deselect does). */
	private void deselect()
	{
		if (Rs2Inventory.isItemSelected())
		{
			Rs2ItemModel selected = Rs2Inventory.get(Rs2Inventory.getSelectedItemId());
			if (selected != null)
			{
				clicks.select(selected);
			}
		}
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
			.where(npc -> place.spotName.equals(npc.getName()))
			.where(npc -> npc.getWorldLocation().distanceTo(place.centre) <= place.radius)
			.nearestOnClientThread();
		if (nearest != null)
		{
			lookout = -1;
			spot = nearest;
			spotTile = nearest.getWorldLocation();
			reachTile = null;
			events.clearDespawned();
		}
		return nearest;
	}

	/**
	 * Shilo has few spots, and after a move none may show for up to a minute: watch from a
	 * lookout for a while, then from the next one (from the bank itself, or from one end of the
	 * river, the spots elsewhere are out of sight).
	 */
	private void waitForSpot(Session s)
	{
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me == null)
		{
			return;
		}
		long now = System.currentTimeMillis();
		WorldPoint[] lookouts = place.lookouts;
		if (lookout < 0)
		{
			lookout = 0;
			for (int i = 1; i < lookouts.length; i++)
			{
				if (me.distanceTo(lookouts[i]) < me.distanceTo(lookouts[lookout]))
				{
					lookout = i;
				}
			}
			lookoutUntil = 0;
		}
		else if (lookoutUntil > 0 && now >= lookoutUntil && lookouts.length > 1)
		{
			lookout = (lookout + 1) % lookouts.length;
			lookoutUntil = 0;
			log.info("Tick Fisher: no spot in view, looking from {}", lookouts[lookout]);
		}
		WorldPoint at = lookouts[lookout];
		if (me.distanceTo(at) > 2)
		{
			Microbot.status = "Tick Fisher: walking to look for a spot";
			Rs2Walker.walkTo(at, 2);
			return;
		}
		if (lookoutUntil == 0)
		{
			lookoutUntil = now + Rs2Random.betweenInclusive(8_000, 20_000);
		}
		Microbot.status = "Tick Fisher: waiting for a fishing spot";
		s.spotWaitTicks++;
	}

	/**
	 * Next to the spot. Distance only: Rs2Player.isMoving compares the pose animation with the
	 * idle one and is true while turning or fishing, which read as walking threw the cycle away
	 * and waited up to 8 s (0.3.1: about half the XP drops off the cycle).
	 */
	private boolean nextTo(Rs2NpcModel target)
	{
		WorldPoint me = Rs2Player.getWorldLocation();
		return me != null && (me.distanceTo(target.getWorldLocation()) <= 1 || me.equals(reachTile));
	}

	/** Walks to the spot by clicking it, then starts a fresh cycle on the next tick. */
	private void approach(Rs2NpcModel target)
	{
		Microbot.status = "Tick Fisher: walking to the spot";
		session.approaches++;
		log.info("Tick Fisher: walking to the spot at {} from {}", target.getWorldLocation(), Rs2Player.getWorldLocation());
		anchorTick = -1;
		if (!clicks.npc(target, place.spotAction))
		{
			Rs2Walker.walkTo(target.getWorldLocation(), 1);
			return;
		}
		// Arrived where the click put us: next to the spot, or standing still close to it (some
		// spots are fished from two tiles away; 0.3.5 re-clicked such a spot every tick).
		WorldPoint[] last = {Rs2Player.getWorldLocation()};
		long[] since = {System.currentTimeMillis()};
		Global.sleepUntil(() ->
		{
			if (nextTo(target))
			{
				return true;
			}
			WorldPoint me = Rs2Player.getWorldLocation();
			long now = System.currentTimeMillis();
			if (me == null || !me.equals(last[0]))
			{
				last[0] = me;
				since[0] = now;
				return false;
			}
			return me.distanceTo(target.getWorldLocation()) <= 2 && now - since[0] >= 1200;
		}, 8000);
		WorldPoint me = Rs2Player.getWorldLocation();
		if (me != null && !nextTo(target) && me.distanceTo(target.getWorldLocation()) <= 2)
		{
			reachTile = me;
			log.info("Tick Fisher: fishing the spot at {} from {}", target.getWorldLocation(), me);
		}
	}

	// ----- inventory, bank ---------------------------------------------------------------------

	private boolean hasGear()
	{
		return missingGear() == null;
	}

	/** What the inventory lacks for this place and mode, null when nothing. */
	private String missingGear()
	{
		if (!Rs2Inventory.hasItem(place.rods))
		{
			return place.bank == null ? "a barbarian rod" : "a fly fishing rod";
		}
		if (!Rs2Inventory.hasItem(place.bait))
		{
			return place.bank == null ? "feathers or fish offcuts" : "feathers";
		}
		boolean teak = config.threeTick() && delay == DelayMethod.TEAK;
		if ((teak || config.fishHandling() == FishHandling.CUT) && !Rs2Inventory.hasItem(ItemID.KNIFE))
		{
			return "a knife";
		}
		// Exactly one log or herb: with more, the delay opens a Make-X window and loses the tick.
		if (teak && Rs2Inventory.count(ItemID.TEAK_LOGS) != 1)
		{
			return "exactly one teak log";
		}
		if (config.threeTick() && delay == DelayMethod.HERB_TAR)
		{
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
		}
		return null;
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
			ItemID.KNIFE, ItemID.TEAK_LOGS, ItemID.COINS, ItemID.PESTLE_AND_MORTAR, ItemID.SWAMP_TAR,
			ItemID.GUAM_LEAF, ItemID.MARENTILL, ItemID.TARROMIN, ItemID.HARRALANDER);
		if (!Rs2Inventory.hasItem(place.rods) && !withdraw(ItemID.FLY_FISHING_ROD, 1, "fly fishing rod"))
		{
			return;
		}
		if (!Rs2Inventory.hasItem(ItemID.FEATHER) && !withdraw(ItemID.FEATHER, -1, "feathers"))
		{
			return;
		}
		if (config.threeTick() && delay == DelayMethod.HERB_TAR && missingGear() != null)
		{
			// The herb tar kit is not taken from the bank: bring it.
			session.stop("Missing " + missingGear());
			return;
		}
		if (config.threeTick() && delay == DelayMethod.TEAK)
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
		Rs2Bank.depositAll(item -> isFish(item.getId()));
		if (!Global.sleepUntil(() -> !Rs2Inventory.hasItem(place.fish), 3000))
		{
			return;
		}
		s.bankTrips++;
		Rs2Bank.closeBank();
		Global.sleepUntil(() -> !Rs2Bank.isOpen(), 2400);
		spot = null;
	}

	/**
	 * No bank at the spot: fetch what ran out (a teak log used up by a slipped cycle, a knife,
	 * feathers) from the nearest bank, deposit the teak stock and the fish, and walk back. Herb tar
	 * kits are not restocked: missing one still stops the run.
	 */
	private void restock(Session s)
	{
		String missing = missingGear();
		boolean herbTar = config.threeTick() && delay == DelayMethod.HERB_TAR;
		boolean herbKit = herbTar && missing != null
			&& (missing.contains("pestle") || missing.contains("swamp tar") || missing.contains("guam"));
		if (place.restock == null || herbKit)
		{
			s.stop("Missing " + missing);
			return;
		}
		Microbot.status = "Tick Fisher: fetching " + missing;
		log.info("Tick Fisher: missing {}, restocking at {}", missing, place.restock);
		if (!openBank())
		{
			return;
		}
		Rs2Bank.depositAll(item -> isFish(item.getId()) || item.getId() == ItemID.XBOWS_CROSSBOW_STOCK_TEAK);
		Global.sleepUntil(() -> !Rs2Inventory.hasItem(ItemID.XBOWS_CROSSBOW_STOCK_TEAK), 2400);
		if (!Rs2Inventory.hasItem(place.rods) && !withdraw(place.rods[0], 1, "barbarian rod"))
		{
			return;
		}
		if (!Rs2Inventory.hasItem(place.bait) && !withdraw(ItemID.FEATHER, -1, "feathers"))
		{
			return;
		}
		boolean teak = config.threeTick() && delay == DelayMethod.TEAK;
		if ((teak || config.fishHandling() == FishHandling.CUT) && !Rs2Inventory.hasItem(ItemID.KNIFE) && !withdraw(ItemID.KNIFE, 1, "knife"))
		{
			return;
		}
		if (teak)
		{
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
		s.bankTrips++;
		missingTicks = 0;
		spot = null;
		lookout = -1;
		anchorTick = -1;
	}

	private boolean openBank()
	{
		if (Rs2Bank.isOpen())
		{
			return true;
		}
		if (Rs2Bank.walkToBankAndUseBank(place.bank != null ? place.bank : place.restock) && Global.sleepUntil(Rs2Bank::isOpen, 5000))
		{
			bankFailures = 0;
			return true;
		}
		if (++bankFailures >= 5)
		{
			session.stop("Could not open the bank");
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

	private boolean isFish(int id)
	{
		for (int fish : place.fish)
		{
			if (fish == id)
			{
				return true;
			}
		}
		return false;
	}

	/** Full inventory while dropping: only when the drops between rolls fell behind. */
	private void dropAllFish(Session s)
	{
		Microbot.status = "Tick Fisher: dropping fish";
		int before = fishCount();
		Rs2Inventory.dropAll(place.fish);
		Global.sleepUntil(() -> !Rs2Inventory.hasItem(place.fish), 6000);
		s.fishDropped += before - fishCount();
		anchorTick = -1;
	}

	/** Full inventory while cutting (plain fishing, or cuts that fell behind): cut them all, a tick each. */
	private void cutAllFish(Session s)
	{
		Microbot.status = "Tick Fisher: cutting fish";
		anchorTick = -1;
		for (int i = 0; i < 28 && isRunning(); i++)
		{
			Rs2ItemModel knife = Rs2Inventory.get(ItemID.KNIFE);
			Rs2ItemModel fish = Rs2Inventory.get(place.fish);
			if (knife == null || fish == null)
			{
				return;
			}
			int before = fishCount();
			selectTool(knife);
			clicks.useOn(fish);
			if (!Global.sleepUntil(() -> fishCount() < before, 1800))
			{
				cuttingFailed = true;
				log.warn("Tick Fisher: knife on a fish did nothing, dropping them instead (Cooking level?)");
				return;
			}
			s.fishCut++;
			Rs2ItemModel roe = Rs2Inventory.get(ROE);
			if (roe != null && Rs2Inventory.isFull())
			{
				clicks.eat(roe);
				s.roeEaten++;
			}
			Global.sleepUntilNextTick(1200);
		}
	}

	// ----- stopping and reporting --------------------------------------------------------------

	private void stop(String reason)
	{
		Rs2NpcModel last = spot;
		if (config.threeTick() && last != null && Microbot.isLoggedIn())
		{
			// A knife-on-teak or a herb tar may still be running: one more spot click cancels it,
			// or it completes and takes the log (or 15 tar and the herb).
			clicks.npc(last, place.spotAction);
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
		progress.put("place", place == null ? null : place.name());
		progress.put("agilityLevel", events.agilityLevel());
		progress.put("threeTick", config.threeTick());
		progress.put("delay", delay == null ? null : delay.name());
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
