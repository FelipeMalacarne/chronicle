package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.time.Duration;
import java.time.Instant;

/**
 * Event-driven capture cadence: game events ask for a capture, which runs after a trailing
 * debounce so a burst (a bank deposit, a quest's var cascade) becomes one capture. A hard event
 * also starts a max-wait cap, so a steady stream of them (skilling with the inventory shared)
 * still captures every {@link #MAX_WAIT}. A soft event only debounces: a steady stream of them
 * (vars that churn in combat) waits for the periodic capture, which stays as the safety net.
 *
 * <p>The trigger only moves the engine's next check forward; the engine still decides whether a
 * capture may run (one push in flight, backoff, manifest), so a request made while a push is in
 * flight stays pending and becomes exactly one capture once the push is answered. Any capture,
 * event-driven or periodic, consumes every request made before it started: events carry no
 * data, the capture reads the current state.
 *
 * <p>Pure apart from the lock: game events arrive on the client thread, {@link #next} runs on
 * the sync thread, and nothing here touches client APIs.
 */
final class CaptureTrigger
{
	/** Quiet time after the last event before capturing. */
	static final Duration DEBOUNCE = Duration.ofSeconds(1);
	/** Longest a hard event waits while more events keep extending the debounce. */
	static final Duration MAX_WAIT = Duration.ofSeconds(3);

	enum Strength
	{
		/** Not something the plugin sends: ignored. */
		NONE,
		/** May have changed what is sent: debounced, no max-wait cap. */
		SOFT,
		/** Changed what is sent: debounced and capped by {@link #MAX_WAIT}. */
		HARD
	}

	/** Last event of either strength since the last capture started, or null. */
	private Instant lastEventAt;
	/** First hard event since the last capture started, or null. */
	private Instant firstHardAt;

	synchronized void onEvent(Strength strength, Instant now)
	{
		if (strength == Strength.NONE)
		{
			return;
		}
		lastEventAt = now;
		if (strength == Strength.HARD && firstHardAt == null)
		{
			firstHardAt = now;
		}
	}

	synchronized boolean pending()
	{
		return lastEventAt != null;
	}

	synchronized boolean due(Instant now)
	{
		if (lastEventAt == null)
		{
			return false;
		}
		return !now.isBefore(lastEventAt.plus(DEBOUNCE))
			|| (firstHardAt != null && !now.isBefore(firstHardAt.plus(MAX_WAIT)));
	}

	/**
	 * The engine's next action, after moving its next check to now when a requested capture is
	 * due. A {@code CAPTURE} answer consumes the pending requests, whoever asked for it.
	 */
	synchronized SyncEngine.Action next(SyncEngine engine, Instant now)
	{
		if (due(now))
		{
			engine.requestCapture(now);
		}
		SyncEngine.Action action = engine.next(now);
		if (action == SyncEngine.Action.CAPTURE)
		{
			clear();
		}
		return action;
	}

	/** Login or logout: what was pending belonged to the previous session. */
	synchronized void clear()
	{
		lastEventAt = null;
		firstHardAt = null;
	}
}
