package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.Collections;
import java.util.Map;
import java.util.Objects;

/**
 * One container observation, immutable. {@code unavailable} carries no revision, epoch or
 * quantities ("could not read" is not "empty"); {@code restored} is a persisted mirror with no
 * live epoch; {@code live} comes from this login's client-thread reads or container events.
 */
final class ContainerSnapshot
{
	enum State
	{
		UNAVAILABLE("unavailable"),
		RESTORED("restored"),
		LIVE("live");

		final String wire;

		State(String wire)
		{
			this.wire = wire;
		}
	}

	final State state;
	final Instant observedAt;
	final long revision;
	final String liveEpoch;
	final Map<String, Long> quantities;

	private ContainerSnapshot(State state, Instant observedAt, long revision, String liveEpoch, Map<String, Long> quantities)
	{
		this.state = state;
		this.observedAt = Objects.requireNonNull(observedAt);
		this.revision = revision;
		this.liveEpoch = liveEpoch;
		this.quantities = quantities == null ? null : Collections.unmodifiableMap(ItemQuantities.copy(quantities));
	}

	static ContainerSnapshot unavailable(Instant observedAt)
	{
		return new ContainerSnapshot(State.UNAVAILABLE, observedAt, -1, null, null);
	}

	static ContainerSnapshot restored(Instant observedAt, long revision, Map<String, Long> quantities)
	{
		return new ContainerSnapshot(State.RESTORED, observedAt, Math.max(0, revision), null, Objects.requireNonNull(quantities));
	}

	static ContainerSnapshot live(Instant observedAt, long revision, String liveEpoch, Map<String, Long> quantities)
	{
		if (liveEpoch == null || liveEpoch.trim().isEmpty())
		{
			throw new IllegalArgumentException("live observation needs a live epoch");
		}
		return new ContainerSnapshot(State.LIVE, observedAt, Math.max(0, revision), liveEpoch, Objects.requireNonNull(quantities));
	}

	/** Whether {@code next} can be sent as a delta patch on top of this acknowledged observation. */
	boolean deltaCompatible(ContainerSnapshot next)
	{
		if (state != next.state)
		{
			return false;
		}
		switch (state)
		{
			case UNAVAILABLE:
				return true;
			case LIVE:
				return liveEpoch.equals(next.liveEpoch) && next.revision >= revision;
			case RESTORED:
			default:
				// restored is valid only in a full push, so any change to it needs a new full
				return revision == next.revision && quantities.equals(next.quantities);
		}
	}

	/** A live container whose revision and contents moved past the acknowledged ones. */
	boolean changedSince(ContainerSnapshot acked)
	{
		return state == State.LIVE && revision > acked.revision && !quantities.equals(acked.quantities);
	}
}
