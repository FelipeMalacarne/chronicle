package net.runelite.client.plugins.microbot.fm.tickminer;

import java.util.LinkedHashMap;
import java.util.Map;

/** One run's counters, read by the overlay on the client thread. */
final class Session
{
	final long startedAt = System.currentTimeMillis();
	final int startTick;
	final int startXp;
	volatile String stopReason;
	/** Why the run went on mining plainly (the teak log or the herb used up), null while 3-ticking. */
	volatile String delayLost;

	/** Cycles started (knife on teak, then a rock). */
	volatile int cycles;
	/** Cycles whose clicks all went out within the tick they were meant for. */
	volatile int onTime;
	/** Cycles whose delay click made it but the rock click landed a tick later. */
	volatile int rockNextTick;
	/** Cycles that stepped to the other row (two rolls on the rock). */
	volatile int steps;
	/** XP drops by the phase of the cycle they landed on; one phase takes them all when in step. */
	final int[] dropsByPhase = new int[3];
	/** XP drops: sandstone mined. */
	volatile int mined;
	/** Ticks spent at the rocks (cycling or plain), the base for sandstone per 100 ticks. */
	volatile int miningTicks;
	/** Trips to the grinder, the sandstone left there by size (1, 2, 5, 10 kg) and the buckets of sand it makes. */
	volatile int deposits;
	final int[] deposited = new int[4];
	volatile int sandStored;
	/** Teak stocks made: a cycle where the rock click did not land and the fletch completed. */
	volatile int stocksMade;
	/** Cycles skipped on purpose, plain-mining breathers, and re-anchors on an off-cycle drop. */
	volatile int slips;
	volatile int breathers;
	volatile int realigns;
	/** Walks to the spot: at the start and back from the grinder. */
	volatile int approaches;
	/** Phase-0 ticks with no rock of the spot up. */
	volatile int rockWaitTicks;
	/** Sips left in the waterskins, as last read. */
	volatile int waterSips;
	/** Trips to the cacti, sips cut into the waterskins, and failed cuts (no water, cactus still healthy). */
	volatile int refillTrips;
	volatile int sipsRefilled;
	volatile int cactusFails;

	Session(int startTick, int startXp)
	{
		this.startTick = startTick;
		this.startXp = startXp;
	}

	void stop(String reason)
	{
		if (stopReason == null)
		{
			stopReason = reason;
		}
	}

	/** Share of XP drops on the busiest phase, in percent; 100 when every roll is on the cycle. */
	int inStepPercent()
	{
		int total = dropsByPhase[0] + dropsByPhase[1] + dropsByPhase[2];
		if (total == 0)
		{
			return 0;
		}
		int max = Math.max(dropsByPhase[0], Math.max(dropsByPhase[1], dropsByPhase[2]));
		return 100 * max / total;
	}

	int onTimePercent()
	{
		return cycles == 0 ? 0 : 100 * onTime / cycles;
	}

	int depositedCount()
	{
		return deposited[0] + deposited[1] + deposited[2] + deposited[3];
	}

	Map<String, Object> counters()
	{
		Map<String, Object> counters = new LinkedHashMap<>();
		counters.put("cycles", cycles);
		counters.put("onTimePercent", onTimePercent());
		counters.put("rockNextTick", rockNextTick);
		counters.put("steps", steps);
		counters.put("dropsByPhase", dropsByPhase[0] + "/" + dropsByPhase[1] + "/" + dropsByPhase[2]);
		counters.put("inStepPercent", inStepPercent());
		counters.put("mined", mined);
		counters.put("miningTicks", miningTicks);
		counters.put("deposits", deposits);
		counters.put("deposited", deposited[0] + "/" + deposited[1] + "/" + deposited[2] + "/" + deposited[3]);
		counters.put("sandStored", sandStored);
		counters.put("stocksMade", stocksMade);
		counters.put("delayLost", delayLost);
		counters.put("slips", slips);
		counters.put("breathers", breathers);
		counters.put("realigns", realigns);
		counters.put("approaches", approaches);
		counters.put("rockWaitTicks", rockWaitTicks);
		counters.put("refillTrips", refillTrips);
		counters.put("sipsRefilled", sipsRefilled);
		counters.put("cactusFails", cactusFails);
		return counters;
	}
}
