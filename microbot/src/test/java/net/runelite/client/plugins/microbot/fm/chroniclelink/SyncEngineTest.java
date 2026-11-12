package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.ack;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.manifest;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.simple;
import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.stale;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Duration;
import java.time.Instant;
import org.junit.jupiter.api.Test;

class SyncEngineTest
{
	private static final String WRITE = IngestContract.SCOPE_STATE_WRITE;

	private static SyncEngine ready(Instant now)
	{
		SyncEngine engine = new SyncEngine();
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(now));
		engine.onManifest(manifest(7, WRITE), now);
		assertEquals(SyncEngine.Action.CAPTURE, engine.next(now));
		return engine;
	}

	@Test
	void baselineThenAcknowledgedDeltas()
	{
		SyncEngine engine = ready(T0);
		PlayerSnapshot first = simple(7, 40);
		IngestContract.Push full = engine.prepare(first, T0);
		assertEquals("full", full.kind);
		assertEquals(1, full.clientSeq);
		assertEquals(SyncEngine.Action.WAIT, engine.next(T0), "one push in flight at a time");

		engine.onPushResult(200, ack(1, 7), T0);
		assertTrue(engine.isAcknowledged(first));
		assertEquals(SyncStatus.Kind.SYNCED, engine.status().kind);
		assertEquals(SyncEngine.Action.WAIT, engine.next(T0.plusSeconds(5)), "waits interval_seconds");

		Instant t = T0.plusSeconds(10);
		assertEquals(SyncEngine.Action.CAPTURE, engine.next(t));
		assertNull(engine.prepare(simple(7, 40), t), "unchanged state sends nothing");

		t = T0.plusSeconds(20);
		PlayerSnapshot second = simple(7, 41);
		IngestContract.Push delta = engine.prepare(second, t);
		assertEquals("delta", delta.kind);
		assertEquals(2, delta.clientSeq);
		assertEquals(41L, delta.data.level.get("attack"));
		engine.onPushResult(200, ack(2, 7), t);
		assertTrue(engine.isAcknowledged(second));
		assertEquals(3, engine.nextSeq());
	}

	@Test
	void timeoutBacksOffAndRetriesTheSameSeqAgainstTheOldBaseline()
	{
		SyncEngine engine = ready(T0);
		PlayerSnapshot base = simple(7, 40);
		engine.prepare(base, T0);
		engine.onPushResult(200, ack(1, 7), T0);

		Instant t = T0.plusSeconds(10);
		IngestContract.Push lost = engine.prepare(simple(7, 41), t);
		engine.onPushError(t);
		assertTrue(engine.isAcknowledged(base), "an unanswered delta does not move the baseline");
		assertEquals(SyncStatus.Kind.RETRYING, engine.status().kind);
		assertEquals(SyncEngine.Action.WAIT, engine.next(t.plusSeconds(4)));

		Instant retry = t.plus(SyncEngine.backoff(1));
		assertEquals(SyncEngine.Action.CAPTURE, engine.next(retry));
		IngestContract.Push again = engine.prepare(simple(7, 42), retry);
		assertEquals(lost.clientSeq, again.clientSeq);
		assertEquals("delta", again.kind);
		assertEquals(42L, again.data.level.get("attack"), "rebuilt against the acknowledged baseline");
	}

	@Test
	void staleSequenceRenumbersPastTheServerAndSendsAFull()
	{
		SyncEngine engine = ready(T0);
		engine.prepare(simple(7, 40), T0);
		// a previous run already used seqs up to 57
		engine.onPushResult(409, stale(57, 7), T0);
		assertNull(engine.status().lastAckAt);
		assertEquals(58, engine.nextSeq());
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(T0), "first conflict retries at once");
		engine.onManifest(manifest(7, WRITE), T0);

		IngestContract.Push full = engine.prepare(simple(7, 40), T0);
		assertEquals("full", full.kind);
		assertEquals(58, full.clientSeq);
		engine.onPushResult(200, ack(58, 7), T0);
		assertEquals(SyncStatus.Kind.SYNCED, engine.status().kind);
	}

	@Test
	void repeatedConflictsBackOff()
	{
		SyncEngine engine = ready(T0);
		engine.prepare(simple(7, 40), T0);
		engine.onPushResult(409, stale(3, 7), T0);
		engine.onManifest(manifest(7, WRITE), T0);
		engine.prepare(simple(7, 40), T0);
		engine.onPushResult(409, stale(9, 7), T0);
		assertEquals(SyncEngine.Action.WAIT, engine.next(T0.plusSeconds(1)));
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(T0.plus(SyncEngine.backoff(1))));
	}

	@Test
	void ackForAnotherSeqIsNotAnAck()
	{
		SyncEngine engine = ready(T0);
		PlayerSnapshot s = simple(7, 40);
		engine.prepare(s, T0);
		engine.onPushResult(200, ack(4, 7), T0);
		assertFalse(engine.isAcknowledged(s));
		assertTrue(engine.needsFull());
		assertEquals(5, engine.nextSeq());
	}

	@Test
	void resyncInAResponseRefreshesTheManifestAndSendsAFull()
	{
		SyncEngine engine = ready(T0);
		engine.prepare(simple(7, 40), T0);
		IngestContract.Response r = ack(1, 8);
		r.resync = true;
		engine.onPushResult(200, r, T0);
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(T0));
		engine.onManifest(manifest(8, WRITE), T0);
		IngestContract.Push full = engine.prepare(simple(8, 40), T0);
		assertEquals("full", full.kind);
		assertEquals(2, full.clientSeq);
		assertEquals(8, full.manifestVersion);
	}

	@Test
	void snapshotOfAReplacedManifestIsNotSent()
	{
		SyncEngine engine = ready(T0);
		assertNull(engine.prepare(simple(6, 40), T0));
	}

	@Test
	void manifestIsRefreshedAfterItsTtl()
	{
		SyncEngine engine = ready(T0);
		engine.prepare(simple(7, 40), T0);
		engine.onPushResult(200, ack(1, 7), T0);
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(T0.plus(SyncEngine.MANIFEST_TTL)));
		engine.onManifest(manifest(7, WRITE), T0.plus(SyncEngine.MANIFEST_TTL));
		assertFalse(engine.needsFull(), "same manifest version keeps the baseline");
		engine.onManifest(manifest(9, WRITE), T0.plus(SyncEngine.MANIFEST_TTL));
		assertTrue(engine.needsFull(), "a new manifest version needs a new full");
	}

	@Test
	void loginAndLogoutRequestAFull()
	{
		SyncEngine engine = ready(T0);
		engine.prepare(simple(7, 40), T0);
		engine.onPushResult(200, ack(1, 7), T0);
		engine.requestFull(T0.plusSeconds(1));
		assertEquals(SyncEngine.Action.CAPTURE, engine.next(T0.plusSeconds(1)));
		assertEquals("full", engine.prepare(simple(7, 40), T0.plusSeconds(1)).kind);
	}

	@Test
	void loggedOutWaitsWithoutSending()
	{
		SyncEngine engine = ready(T0);
		assertNull(engine.prepare(null, T0));
		assertEquals(SyncStatus.Kind.WAITING_LOGIN, engine.status().kind);
		assertEquals(SyncEngine.Action.WAIT, engine.next(T0.plusSeconds(1)));
		assertEquals(SyncEngine.Action.CAPTURE, engine.next(T0.plus(SyncEngine.CAPTURE_RETRY)));
	}

	@Test
	void rejectedTokenWaitsTheLongestBackoff()
	{
		SyncEngine engine = new SyncEngine();
		engine.onManifestError(401, T0);
		assertEquals(SyncStatus.Kind.UNAUTHORIZED, engine.status().kind);
		assertEquals(SyncEngine.Action.WAIT, engine.next(T0.plus(SyncEngine.BACKOFF_CAP).minusSeconds(1)));
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(T0.plus(SyncEngine.BACKOFF_CAP)));
	}

	@Test
	void unsupportedContractHalts()
	{
		SyncEngine engine = new SyncEngine();
		IngestContract.Manifest m = manifest(7, WRITE);
		m.contractVersion = 2;
		engine.onManifest(m, T0);
		assertEquals(SyncStatus.Kind.INCOMPATIBLE, engine.status().kind);
		assertEquals(SyncEngine.Action.WAIT, engine.next(T0.plusSeconds(3600)));

		SyncEngine noWrite = new SyncEngine();
		noWrite.onManifest(manifest(7, IngestContract.SCOPE_BANK), T0);
		assertEquals(SyncStatus.Kind.INCOMPATIBLE, noWrite.status().kind);
	}

	@Test
	void backoffIsQuadraticAndCapped()
	{
		assertEquals(Duration.ofSeconds(5), SyncEngine.backoff(1));
		assertEquals(Duration.ofSeconds(20), SyncEngine.backoff(2));
		assertEquals(Duration.ofSeconds(45), SyncEngine.backoff(3));
		assertEquals(SyncEngine.BACKOFF_CAP, SyncEngine.backoff(50));
		assertEquals(SyncEngine.BACKOFF_CAP, SyncEngine.backoff(Integer.MAX_VALUE));
	}

	@Test
	void itemPermissionNeedsBothManifestFlagAndScope()
	{
		IngestContract.Manifest m = manifest(7, WRITE, IngestContract.SCOPE_BANK);
		assertTrue(m.allows(ContainerKind.BANK));
		assertFalse(m.allows(ContainerKind.INVENTORY));
		m.items.bank = false;
		assertFalse(m.allows(ContainerKind.BANK));
		m.items.equipment = true;
		assertFalse(m.allows(ContainerKind.EQUIPMENT), "flag without scope");
	}

	@Test
	void statusLineHasNoIdentity()
	{
		SyncEngine engine = ready(T0);
		engine.prepare(simple(7, 40), T0);
		engine.onPushResult(200, ack(1, 7), T0);
		String line = engine.status().describe(T0.plusSeconds(12));
		assertEquals("Synced 12s ago (#1)", line);
		assertFalse(line.contains("Iron Test"));
		assertNotNull(engine.status().lastAckAt);
	}
}
