package net.runelite.client.plugins.microbot.fm.tickfisher;

import java.util.LinkedHashMap;
import java.util.Map;

/** One run's counters, read by the overlay on the client thread. */
final class Session
{
	final long startedAt = System.currentTimeMillis();
	final int startTick;
	final int startXp;
	volatile String stopReason;

	/** Cycles started (knife on teak, then the spot). */
	volatile int cycles;
	/** Cycles whose clicks all went out within the tick they were meant for. */
	volatile int onTime;
	/** Cycles whose teak click made it but the spot click landed a tick later. */
	volatile int spotNextTick;
	/** XP drops by the phase of the cycle they landed on; one phase takes them all when in step. */
	final int[] dropsByPhase = new int[3];
	volatile int catches;
	/** Ticks spent fishing at a spot (cycling or plain), the base for catches per 100 ticks. */
	volatile int fishingTicks;
	volatile int fishDropped;
	volatile int bankTrips;
	/** Teak stocks made: a cycle where the spot click did not land and the fletch completed. */
	volatile int stocksMade;
	volatile int spotMoves;
	/** Cycles skipped on purpose, plain-fishing breathers, and re-anchors on an off-cycle drop. */
	volatile int slips;
	volatile int breathers;
	volatile int realigns;
	/** Walks to a spot: after a move, a bank trip, or not standing next to it. */
	volatile int approaches;
	/** Ticks with no spot in reach (after a move none may show for a while). */
	volatile int spotWaitTicks;

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

	Map<String, Object> counters()
	{
		Map<String, Object> counters = new LinkedHashMap<>();
		counters.put("cycles", cycles);
		counters.put("onTimePercent", onTimePercent());
		counters.put("spotNextTick", spotNextTick);
		counters.put("dropsByPhase", dropsByPhase[0] + "/" + dropsByPhase[1] + "/" + dropsByPhase[2]);
		counters.put("inStepPercent", inStepPercent());
		counters.put("catches", catches);
		counters.put("fishingTicks", fishingTicks);
		counters.put("fishDropped", fishDropped);
		counters.put("bankTrips", bankTrips);
		counters.put("stocksMade", stocksMade);
		counters.put("spotMoves", spotMoves);
		counters.put("slips", slips);
		counters.put("breathers", breathers);
		counters.put("realigns", realigns);
		counters.put("approaches", approaches);
		counters.put("spotWaitTicks", spotWaitTicks);
		return counters;
	}
}
