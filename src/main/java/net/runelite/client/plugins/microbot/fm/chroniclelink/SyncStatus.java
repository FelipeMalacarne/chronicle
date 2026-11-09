package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Duration;
import java.time.Instant;

/** Immutable status for the indicator. Carries no username, token or URL. */
final class SyncStatus
{
	enum Kind
	{
		NOT_CONFIGURED("Not configured"),
		CONNECTING("Connecting"),
		WAITING_LOGIN("Waiting for login"),
		SYNCED("Synced"),
		RETRYING("Offline, retrying"),
		UNAUTHORIZED("Credential rejected"),
		REJECTED("Push rejected"),
		INCOMPATIBLE("Server contract unsupported");

		final String label;

		Kind(String label)
		{
			this.label = label;
		}
	}

	final Kind kind;
	/** Last acknowledged push, or null. */
	final Instant lastAckAt;
	final long lastAcceptedSeq;
	/** When the next attempt is allowed after a failure, or null. */
	final Instant retryAt;

	SyncStatus(Kind kind, Instant lastAckAt, long lastAcceptedSeq, Instant retryAt)
	{
		this.kind = kind;
		this.lastAckAt = lastAckAt;
		this.lastAcceptedSeq = lastAcceptedSeq;
		this.retryAt = retryAt;
	}

	/** One line for the overlay, e.g. "Synced 12s ago (#42)". */
	String describe(Instant now)
	{
		StringBuilder sb = new StringBuilder(kind.label);
		if (retryAt != null && retryAt.isAfter(now))
		{
			sb.append(", next try in ").append(ago(Duration.between(now, retryAt)));
		}
		if (lastAckAt != null)
		{
			sb.append(kind == Kind.SYNCED ? " " : "; last sync ")
				.append(ago(Duration.between(lastAckAt, now)))
				.append(" ago (#").append(lastAcceptedSeq).append(')');
		}
		return sb.toString();
	}

	private static String ago(Duration d)
	{
		long s = Math.max(0, d.getSeconds());
		if (s < 120)
		{
			return s + "s";
		}
		if (s < 7200)
		{
			return (s / 60) + "m";
		}
		return (s / 3600) + "h";
	}
}
