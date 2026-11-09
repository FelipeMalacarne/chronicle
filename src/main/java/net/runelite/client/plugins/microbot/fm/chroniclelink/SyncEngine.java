package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Duration;
import java.time.Instant;

/**
 * The ingest protocol as a pure state machine; the plugin feeds it the clock, snapshots and
 * HTTP outcomes. One push is in flight at a time. The acknowledged baseline, the delta base,
 * only advances when the server accepts exactly the sequence that was sent; a timeout keeps
 * the same {@code client_seq} for the retry, so a push that did land is caught by the
 * server's stale-sequence answer, which in turn renumbers past it and sends a new full.
 * Not thread-safe: the plugin drives it from a single executor thread.
 */
final class SyncEngine
{
	enum Action
	{
		/** Nothing to do now. */
		WAIT,
		/** GET the manifest, then {@link #onManifest} or {@link #onManifestError}. */
		FETCH_MANIFEST,
		/** Capture a snapshot and pass it to {@link #prepare}. */
		CAPTURE
	}

	static final Duration MANIFEST_TTL = Duration.ofMinutes(20);
	static final Duration BACKOFF_BASE = Duration.ofSeconds(5);
	static final Duration BACKOFF_CAP = Duration.ofMinutes(5);
	/** How soon to look again when a full is due but the player is not readable yet. */
	static final Duration CAPTURE_RETRY = Duration.ofSeconds(2);

	private static final class InFlight
	{
		final long seq;
		final PlayerSnapshot snapshot;

		InFlight(long seq, PlayerSnapshot snapshot)
		{
			this.seq = seq;
			this.snapshot = snapshot;
		}
	}

	private IngestContract.Manifest manifest;
	private Instant manifestFetchedAt = Instant.EPOCH;
	private boolean manifestStale = true;
	private PlayerSnapshot acked;
	private long nextSeq = 1;
	private boolean needFull = true;
	private InFlight inFlight;
	private int failures;
	private int conflicts;
	private Instant retryAt = Instant.EPOCH;
	private Instant nextCheckAt = Instant.EPOCH;
	private boolean halted;
	private SyncStatus.Kind kind = SyncStatus.Kind.CONNECTING;
	private Instant lastAckAt;
	private long lastAcceptedSeq;

	Action next(Instant now)
	{
		if (halted || inFlight != null || now.isBefore(retryAt))
		{
			return Action.WAIT;
		}
		if (manifest == null || manifestStale || !now.isBefore(manifestFetchedAt.plus(MANIFEST_TTL)))
		{
			return Action.FETCH_MANIFEST;
		}
		return now.isBefore(nextCheckAt) ? Action.WAIT : Action.CAPTURE;
	}

	void onManifest(IngestContract.Manifest m, Instant now)
	{
		if (m == null || m.contractVersion != IngestContract.CONTRACT_VERSION || m.manifestVersion < 1
			|| m.tokenScopes == null || !m.tokenScopes.contains(IngestContract.SCOPE_STATE_WRITE))
		{
			halted = true;
			kind = SyncStatus.Kind.INCOMPATIBLE;
			return;
		}
		if (manifest == null || manifest.manifestVersion != m.manifestVersion || m.resync)
		{
			needFull = true;
			nextCheckAt = now;
		}
		manifest = m;
		manifestFetchedAt = now;
		manifestStale = false;
		retryAt = Instant.EPOCH;
		if (kind != SyncStatus.Kind.SYNCED && kind != SyncStatus.Kind.WAITING_LOGIN)
		{
			kind = SyncStatus.Kind.CONNECTING;
		}
	}

	void onManifestError(int httpStatus, Instant now)
	{
		fail(httpStatus, now);
	}

	/**
	 * Turns a snapshot into the next push and marks it in flight, or returns {@code null} when
	 * there is nothing to send ({@code snapshot == null} means the player is not readable).
	 */
	IngestContract.Push prepare(PlayerSnapshot snapshot, Instant now)
	{
		if (manifest == null || inFlight != null)
		{
			return null;
		}
		if (snapshot == null)
		{
			if (kind == SyncStatus.Kind.SYNCED || kind == SyncStatus.Kind.CONNECTING)
			{
				kind = SyncStatus.Kind.WAITING_LOGIN;
			}
			nextCheckAt = now.plus(needFull ? CAPTURE_RETRY : interval());
			return null;
		}
		if (snapshot.manifestVersion != manifest.manifestVersion)
		{
			// captured against a manifest that has since been replaced
			nextCheckAt = now;
			return null;
		}

		long seq = nextSeq;
		IngestContract.Push push;
		if (needFull || PushBuilder.requiresFull(acked, snapshot))
		{
			push = PushBuilder.full(snapshot, seq, now);
		}
		else
		{
			push = PushBuilder.delta(acked, snapshot, seq, now);
			if (push == null)
			{
				nextCheckAt = now.plus(interval());
				return null;
			}
		}
		inFlight = new InFlight(seq, snapshot);
		return push;
	}

	/** An HTTP answer arrived ({@code body} may be null when it was not a contract response). */
	void onPushResult(int httpStatus, IngestContract.Response body, Instant now)
	{
		InFlight sent = inFlight;
		inFlight = null;
		if (sent == null)
		{
			return;
		}
		boolean contractBody = body != null && body.contractVersion == IngestContract.CONTRACT_VERSION;
		if (httpStatus == 200 && contractBody && body.ok && body.acceptedSeq == sent.seq)
		{
			acked = sent.snapshot;
			nextSeq = sent.seq + 1;
			needFull = false;
			failures = 0;
			conflicts = 0;
			retryAt = Instant.EPOCH;
			lastAckAt = now;
			lastAcceptedSeq = sent.seq;
			kind = SyncStatus.Kind.SYNCED;
			nextCheckAt = now.plus(interval());
			if (body.resync || body.manifestVersion != manifest.manifestVersion)
			{
				needFull = true;
				manifestStale = true;
				nextCheckAt = now;
			}
			return;
		}
		if (httpStatus == 409 || (httpStatus == 200 && contractBody))
		{
			// stale sequence or manifest: renumber past the server's last accepted seq, then a full
			long accepted = contractBody ? body.acceptedSeq : sent.seq;
			nextSeq = Math.max(sent.seq, accepted) + 1;
			acked = null;
			needFull = true;
			manifestStale = true;
			conflicts++;
			retryAt = conflicts <= 1 ? now : now.plus(backoff(conflicts - 1));
			nextCheckAt = now;
			return;
		}
		fail(httpStatus, now);
	}

	/** Timeout or I/O error: unknown whether it landed, so the retry reuses the same seq. */
	void onPushError(Instant now)
	{
		if (inFlight == null)
		{
			return;
		}
		inFlight = null;
		fail(0, now);
	}

	/** Login, logout or a toggle change: the next push describes everything again. */
	void requestFull(Instant now)
	{
		needFull = true;
		nextCheckAt = now;
	}

	/**
	 * A game event asks for a capture now instead of at the next periodic check. It only moves
	 * that check forward: a push in flight, a backoff or a halt still hold it back, and the
	 * capture is a delta unless a full was already due. Whatever the capture finds, the next
	 * periodic check is again one interval later, so the safety net skips what events covered.
	 */
	void requestCapture(Instant now)
	{
		if (now.isBefore(nextCheckAt))
		{
			nextCheckAt = now;
		}
	}

	IngestContract.Manifest manifest()
	{
		return manifest;
	}

	boolean isAcknowledged(PlayerSnapshot snapshot)
	{
		return snapshot != null && acked == snapshot;
	}

	long nextSeq()
	{
		return nextSeq;
	}

	boolean needsFull()
	{
		return needFull;
	}

	SyncStatus status()
	{
		return new SyncStatus(kind, lastAckAt, lastAcceptedSeq, Instant.EPOCH.equals(retryAt) ? null : retryAt);
	}

	static Duration backoff(int failures)
	{
		long n = Math.min(Math.max(1, failures), 100);
		return Duration.ofSeconds(Math.min(BACKOFF_BASE.getSeconds() * n * n, BACKOFF_CAP.getSeconds()));
	}

	private Duration interval()
	{
		return Duration.ofSeconds(manifest == null ? 10 : manifest.intervalSecondsOrDefault());
	}

	private void fail(int httpStatus, Instant now)
	{
		failures++;
		Duration delay = backoff(failures);
		switch (httpStatus)
		{
			case 401:
				kind = SyncStatus.Kind.UNAUTHORIZED;
				manifestStale = true;
				delay = BACKOFF_CAP;
				break;
			case 403:
				// scope denied: the token's scopes changed under us
				kind = SyncStatus.Kind.REJECTED;
				manifestStale = true;
				needFull = true;
				break;
			case 400:
			case 404:
			case 413:
			case 422:
				kind = SyncStatus.Kind.REJECTED;
				needFull = true;
				break;
			default:
				kind = SyncStatus.Kind.RETRYING;
				break;
		}
		retryAt = now.plus(delay);
	}
}
