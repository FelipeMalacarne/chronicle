package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.T0;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.ack;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.manifest;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.quests;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.stale;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.states;
import static net.runelite.client.plugins.microbot.fm.ironmanplanner.Fixtures.vars;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import com.google.gson.JsonObject;
import java.time.Instant;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;
import net.runelite.api.Quest;
import net.runelite.api.QuestState;
import org.junit.jupiter.api.Test;

class QuestsAndVarsTest
{
	private static final Gson GSON = new Gson();
	private static final Instant SENT = T0.plusSeconds(5);
	private static final String WRITE = IngestContract.SCOPE_STATE_WRITE;

	private static JsonObject data(IngestContract.Push push)
	{
		return GSON.fromJson(GSON.toJson(push), JsonObject.class).getAsJsonObject("data");
	}

	private static IngestContract.Manifest withQuests(long version)
	{
		IngestContract.Manifest m = manifest(version, WRITE);
		m.quests = true;
		return m;
	}

	@Test
	void fullCarriesEveryQuest()
	{
		PlayerSnapshot s = quests(7, states("Cook's Assistant", "FINISHED", "Rune Mysteries", "IN_PROGRESS",
			"Recipe for Disaster - Mountain Dwarf", "NOT_STARTED"));
		JsonObject quest = data(PushBuilder.full(s, 1, SENT)).getAsJsonObject("quest");
		assertEquals(3, quest.size());
		assertEquals("FINISHED", quest.get("Cook's Assistant").getAsString());
		assertEquals("IN_PROGRESS", quest.get("Rune Mysteries").getAsString());
		assertEquals("NOT_STARTED", quest.get("Recipe for Disaster - Mountain Dwarf").getAsString());
	}

	@Test
	void emptyQuestsAreAnObjectNeverNull()
	{
		JsonObject data = data(PushBuilder.full(quests(7, Collections.emptyMap()), 1, SENT));
		assertTrue(data.get("quest").isJsonObject());
		assertEquals(0, data.getAsJsonObject("quest").size());
		assertTrue(GSON.toJson(PushBuilder.full(quests(7, Collections.emptyMap()), 1, SENT)).contains("\"quest\":{}"));
	}

	@Test
	void questsNotCollectedAreLeftOutOfFullAndDelta()
	{
		PlayerSnapshot a = quests(7, null);
		assertFalse(data(PushBuilder.full(a, 1, SENT)).has("quest"));
		PlayerSnapshot b = Fixtures.withQuests(7, vars("4458", 1), vars("101", 33), vars("attack", 40), null,
			Fixtures.allUnavailable(T0));
		assertFalse(PushBuilder.requiresFull(a, b));
		JsonObject delta = data(PushBuilder.delta(a, b, 2, SENT));
		assertFalse(delta.has("quest"));
		assertEquals(1, delta.getAsJsonObject("varb").get("4458").getAsLong());
	}

	@Test
	void deltaCarriesOnlyTheQuestsThatChanged()
	{
		Map<String, String> before = states("Cook's Assistant", "FINISHED", "Rune Mysteries", "IN_PROGRESS");
		Map<String, String> after = states("Cook's Assistant", "FINISHED", "Rune Mysteries", "FINISHED");
		PlayerSnapshot acked = quests(7, before);
		PlayerSnapshot current = Fixtures.withQuests(7, vars("4458", 0), vars("101", 34), vars("attack", 40), after,
			Fixtures.allUnavailable(T0));

		assertFalse(PushBuilder.requiresFull(acked, current));
		IngestContract.Push push = PushBuilder.delta(acked, current, 2, SENT);
		assertEquals(states("Rune Mysteries", "FINISHED"), push.data.quest);
		assertEquals(vars("101", 34), push.data.varp);
		assertNull(push.data.varb);

		assertNull(PushBuilder.delta(acked, quests(7, before), 3, SENT), "unchanged quests send nothing");
		assertFalse(data(PushBuilder.delta(quests(7, before), Fixtures.withQuests(7, vars("4458", 1), vars("101", 33),
			vars("attack", 40), before, Fixtures.allUnavailable(T0)), 3, SENT)).has("quest"),
			"unchanged quests are omitted, not sent empty");
	}

	@Test
	void questsAppearingOrDisappearingNeedAFull()
	{
		Map<String, String> some = states("Cook's Assistant", "FINISHED");
		assertTrue(PushBuilder.requiresFull(quests(7, null), quests(7, some)));
		assertTrue(PushBuilder.requiresFull(quests(7, some), quests(7, null)));
		assertTrue(PushBuilder.requiresFull(quests(7, some),
			quests(7, states("Cook's Assistant", "FINISHED", "Dragon Slayer I", "NOT_STARTED"))));
	}

	@Test
	void questsAreOnlyReadWhenTheManifestOffersThem()
	{
		AtomicInteger reads = new AtomicInteger();
		QuestTracker tracker = new QuestTracker(new Quest[]{Quest.COOKS_ASSISTANT}, q ->
		{
			reads.incrementAndGet();
			return QuestState.FINISHED;
		});

		IngestContract.Manifest absent = manifest(7, WRITE);
		assertNull(absent.quests);
		assertNull(StateCollector.readQuests(absent, tracker));
		IngestContract.Manifest off = manifest(7, WRITE);
		off.quests = false;
		assertNull(StateCollector.readQuests(off, tracker));
		assertEquals(0, reads.get(), "no client script runs for a server that does not want quests");

		assertEquals(states("Cook's Assistant", "FINISHED"), StateCollector.readQuests(withQuests(7), tracker));
		assertFalse(data(PushBuilder.full(quests(7, StateCollector.readQuests(off, tracker)), 1, SENT)).has("quest"));
	}

	@Test
	void trackerReadsEveryClientQuestAndCachesUntilAVarChanges()
	{
		Map<Quest, QuestState> game = new HashMap<>();
		AtomicInteger reads = new AtomicInteger();
		QuestTracker tracker = new QuestTracker(Quest.values(), q ->
		{
			reads.incrementAndGet();
			return game.getOrDefault(q, QuestState.NOT_STARTED);
		});
		game.put(Quest.COOKS_ASSISTANT, QuestState.FINISHED);

		Map<String, String> first = tracker.read();
		assertEquals(Quest.values().length, first.size());
		assertEquals("FINISHED", first.get(Quest.COOKS_ASSISTANT.getName()));
		assertEquals("NOT_STARTED", first.get(Quest.RUNE_MYSTERIES.getName()));
		int perRead = reads.get();

		game.put(Quest.RUNE_MYSTERIES, QuestState.IN_PROGRESS);
		assertSame(first, tracker.read(), "no var changed: cached");
		assertEquals(perRead, reads.get());

		tracker.invalidate();
		assertEquals("IN_PROGRESS", tracker.read().get(Quest.RUNE_MYSTERIES.getName()));
		assertEquals(2 * perRead, reads.get());

		tracker.reset();
		tracker.read();
		assertEquals(3 * perRead, reads.get(), "a new login reads again");
	}

	@Test
	void clientQuestNamesAreValidKeys()
	{
		for (Quest quest : Quest.values())
		{
			assertTrue(QuestTracker.validName(quest.getName()), quest.name());
		}
		assertFalse(QuestTracker.validName(" Padded"));
		assertFalse(QuestTracker.validName(""));
		assertFalse(QuestTracker.validName(String.join("", Collections.nCopies(121, "a"))));
		assertTrue(QuestTracker.validName(String.join("", Collections.nCopies(120, "a"))));
	}

	@Test
	void aFailedReadSendsNoQuestsRatherThanAPartialList()
	{
		AtomicInteger calls = new AtomicInteger();
		QuestTracker tracker = new QuestTracker(new Quest[]{Quest.COOKS_ASSISTANT, Quest.RUNE_MYSTERIES}, q ->
		{
			if (calls.incrementAndGet() == 2)
			{
				throw new IllegalStateException("script");
			}
			return QuestState.FINISHED;
		});
		assertNull(tracker.read());
		assertEquals(2, tracker.read().size(), "stays dirty and retries at the next capture");
	}

	@Test
	void varsAreReadGenericallyFromWhateverTheManifestLists()
	{
		IngestContract.Manifest m = GSON.fromJson("{\"contract_version\":1,\"manifest_version\":329934161,"
			+ "\"token_scopes\":[\"state:write\"],\"varbits\":[3578,4458,7928],\"varps\":[101],\"collections\":[],"
			+ "\"skills\":true,\"quests\":true,\"items\":{\"bank\":false,\"inventory\":false,\"equipment\":false},"
			+ "\"interval_seconds\":10,\"resync\":true}", IngestContract.Manifest.class);
		assertTrue(m.questsEnabled());

		Map<String, Long> varb = StateCollector.readVars(m.varbits, id -> id == 4458 ? 1 : 0);
		Map<String, Long> varp = StateCollector.readVars(m.varps, id -> id == 101 ? 33 : -1);
		assertEquals(vars("3578", 0, "4458", 1, "7928", 0), varb);
		assertEquals(vars("101", 33), varp);
		assertEquals(Collections.emptyMap(), StateCollector.readVars(null, id -> 1));
		assertEquals(vars("1", 7), StateCollector.readVars(Arrays.asList(1, null), id -> 7));

		JsonObject data = data(PushBuilder.full(Fixtures.withQuests(m.manifestVersion, varb, varp, vars(), null,
			Fixtures.allUnavailable(T0)), 1, SENT));
		assertEquals(33, data.getAsJsonObject("varp").get("101").getAsLong());
		assertEquals(3, data.getAsJsonObject("varb").size());
	}

	@Test
	void manifestChangeAfterTheServerDeployResyncsWithAFullThatCarriesQuests()
	{
		// a 1.0-era manifest without quests, acknowledged baseline
		SyncEngine engine = new SyncEngine();
		engine.next(T0);
		engine.onManifest(manifest(7, WRITE), T0);
		engine.prepare(quests(7, null), T0);
		engine.onPushResult(200, ack(1, 7), T0);

		// the server is redeployed: the next delta hits a new manifest_version
		Instant t = T0.plusSeconds(10);
		IngestContract.Push delta = engine.prepare(Fixtures.withQuests(7, vars("4458", 1), vars("101", 33),
			vars("attack", 40), null, Fixtures.allUnavailable(T0)), t);
		assertEquals("delta", delta.kind);
		engine.onPushResult(409, stale(1, 8), t);
		assertTrue(engine.needsFull());
		assertEquals(3, engine.nextSeq(), "renumbered past the conflicting seq");
		assertEquals(SyncEngine.Action.FETCH_MANIFEST, engine.next(t));

		engine.onManifest(withQuests(8), t);
		assertNull(engine.prepare(quests(7, null), t), "a capture against the old manifest is dropped");
		Map<String, String> all = states("Cook's Assistant", "FINISHED", "Rune Mysteries", "NOT_STARTED");
		IngestContract.Push full = engine.prepare(quests(8, all), t);
		assertEquals("full", full.kind);
		assertEquals(3, full.clientSeq);
		assertEquals(8, full.manifestVersion);
		assertEquals(all, full.data.quest);
		engine.onPushResult(200, ack(3, 8), t);
		assertEquals(SyncStatus.Kind.SYNCED, engine.status().kind);
		assertFalse(engine.needsFull());
	}
}
