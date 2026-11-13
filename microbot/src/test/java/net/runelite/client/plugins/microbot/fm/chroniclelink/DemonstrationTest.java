package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.time.Instant;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;
import net.runelite.api.MenuAction;
import net.runelite.api.gameval.InterfaceID;
import org.junit.jupiter.api.Test;

/** The demonstration recording (planner docs/10 section 11): what is recorded, how, and the queue. */
class DemonstrationTest
{
	private static final Gson GSON = new Gson();
	private static final Instant T = Instant.parse("2026-11-13T12:00:00Z");
	private static final DemonstrationRecorder.Tile LUMBRIDGE = tile(3222, 3218);

	private static DemonstrationRecorder.Tile tile(int x, int y)
	{
		return new DemonstrationRecorder.Tile(330, x, y, 0);
	}

	private static DemonstrationRecorder recording()
	{
		DemonstrationRecorder r = new DemonstrationRecorder();
		r.start("Iron Test", "rec-5b1c");
		return r;
	}

	private static List<IngestContract.RecordedEvent> all(DemonstrationRecorder r)
	{
		return r.pending("Iron Test", Integer.MAX_VALUE);
	}

	private static Map<Integer, Long> items(long... idQty)
	{
		Map<Integer, Long> out = new TreeMap<>();
		for (int i = 0; i < idQty.length; i += 2)
		{
			out.put((int) idQty[i], idQty[i + 1]);
		}
		return out;
	}

	@Test
	void nothingIsRecordedUntilStarted()
	{
		DemonstrationRecorder r = new DemonstrationRecorder();
		assertFalse(r.onTick(T, LUMBRIDGE));
		assertFalse(r.onNpc(T, LUMBRIDGE, 3080, "Talk-to", "Duke Horacio"));
		assertFalse(r.onBankOpened(T, LUMBRIDGE, items()));
		assertEquals(0, r.pendingCount());
		assertTrue(r.pending("Iron Test", 10).isEmpty());
	}

	@Test
	void positionIsSampledCoarsely()
	{
		DemonstrationRecorder r = recording();
		assertTrue(r.onTick(T, LUMBRIDGE), "the first tick of a recording");
		// walking within the region: no sample per tile
		assertFalse(r.onTick(T, tile(3223, 3218)));
		assertFalse(r.onTick(T, tile(3225, 3218)));
		// standing still near the last sample is not a stop worth recording
		assertFalse(r.onTick(T, tile(3225, 3218)));
		assertFalse(r.onTick(T, tile(3225, 3218)));
		// walking on and stopping five tiles or more away is
		assertFalse(r.onTick(T, tile(3228, 3218)));
		assertFalse(r.onTick(T, tile(3228, 3218)));
		assertTrue(r.onTick(T, tile(3228, 3218)));
		assertFalse(r.onTick(T, tile(3228, 3218)), "one sample per stop");
		// another map region, plane or world
		assertTrue(r.onTick(T, tile(3100, 3218)));
		assertTrue(r.onTick(T, new DemonstrationRecorder.Tile(330, 3100, 3218, 1)));
		assertTrue(r.onTick(T, new DemonstrationRecorder.Tile(331, 3100, 3218, 1)));
		List<IngestContract.RecordedEvent> events = all(r);
		assertEquals(5, events.size());
		assertEquals("move", events.get(1).kind);
		assertEquals(3228, events.get(1).x);
		for (int i = 0; i < events.size(); i++)
		{
			assertEquals("rec-5b1c", events.get(i).recording);
			assertEquals(i + 1, events.get(i).seq);
		}
	}

	@Test
	void interactionsCarryTheGameIds()
	{
		DemonstrationRecorder r = recording();
		assertTrue(r.onNpc(T, LUMBRIDGE, 3080, "Talk-to", "<col=ffff00>Duke Horacio</col>"));
		r.onDialogueOptions();
		assertTrue(r.onDialogue(T, LUMBRIDGE, "Have you any quests for me?"));
		assertFalse(r.onDialogue(T, LUMBRIDGE, "Have you any quests for me?"), "a click and its key are one choice");
		r.onDialogueOptions();
		assertTrue(r.onDialogue(T, LUMBRIDGE, "Sure, no problem."));
		assertTrue(r.onObject(T, LUMBRIDGE, 10583, "Bank", "Bank booth", tile(3208, 3220)));
		assertTrue(r.onItemUse(T, LUMBRIDGE, 1925, null, 14868, null, "Sink"));
		assertFalse(r.onItemUse(T, LUMBRIDGE, 1925, null, null, null, "nothing"), "a use needs one target");
		assertTrue(r.onItemOp(T, LUMBRIDGE, 8007, "Break", "Varrock teleport"));

		List<IngestContract.RecordedEvent> e = all(r);
		assertEquals(Arrays.asList("npc", "dialogue", "dialogue", "object", "item_use", "item_op"),
			Arrays.asList(e.get(0).kind, e.get(1).kind, e.get(2).kind, e.get(3).kind, e.get(4).kind, e.get(5).kind));
		assertEquals("Duke Horacio", e.get(0).name, "tags stripped");
		assertEquals(Integer.valueOf(3080), e.get(1).npcId, "a dialogue names the NPC talked to");
		assertEquals(Integer.valueOf(3208), e.get(3).targetX);
		assertEquals(Integer.valueOf(14868), e.get(4).objectId);
		assertEquals("Use", e.get(4).option);
		assertEquals(Integer.valueOf(8007), e.get(5).itemId);
	}

	@Test
	void bankChangesAreWhatLeftOrEnteredTheBank()
	{
		DemonstrationRecorder r = recording();
		r.onObject(T, LUMBRIDGE, 10583, "Bank", "Bank booth", null);
		// carrying a bronze axe (inventory) and a bronze sword (worn)
		assertTrue(r.onBankOpened(T, LUMBRIDGE, items(1351, 1, 1277, 1)));
		// withdraw 250 coins and 5 noted logs, deposit the axe
		assertEquals(3, r.onCarried(T, LUMBRIDGE, items(995, 250, 1512, 5), items(1277, 1)));
		// wearing the axe from the inventory changes nothing in the bank
		assertEquals(0, r.onCarried(T, LUMBRIDGE, items(995, 250, 1512, 5), items(1277, 1)));
		// deposit the worn sword
		assertEquals(1, r.onCarried(T, LUMBRIDGE, items(995, 250, 1512, 5), items()));
		r.onClosed(DemonstrationRecorder.Open.BANK);
		assertEquals(0, r.onCarried(T, LUMBRIDGE, items(), items()), "nothing after the bank closed");

		List<IngestContract.RecordedEvent> e = all(r);
		assertEquals("bank_open", e.get(1).kind);
		assertEquals(Integer.valueOf(10583), e.get(1).objectId, "the booth clicked");
		assertEquals("bank_withdraw", e.get(2).kind);
		assertEquals(Integer.valueOf(995), e.get(2).itemId);
		assertEquals(Integer.valueOf(250), e.get(2).qty);
		assertEquals("bank_deposit", e.get(3).kind);
		assertEquals(Integer.valueOf(1351), e.get(3).itemId);
		assertEquals("bank_withdraw", e.get(4).kind);
		assertEquals(Integer.valueOf(1512), e.get(4).itemId);
		assertEquals("bank_deposit", e.get(5).kind);
		assertEquals(Integer.valueOf(1277), e.get(5).itemId);
		assertEquals(6, e.size());
	}

	@Test
	void shopPurchasesAreWhatEnteredTheInventory()
	{
		DemonstrationRecorder r = recording();
		r.onNpc(T, LUMBRIDGE, 2813, "Trade", "Bob");
		r.onShopOpened(items(995, 100, 1511, 3));
		// bought an axe with coins, sold the logs
		assertEquals(1, r.onCarried(T, LUMBRIDGE, items(995, 84, 1351, 1), items()));
		r.onClosed(DemonstrationRecorder.Open.SHOP);
		List<IngestContract.RecordedEvent> e = all(r);
		assertEquals(2, e.size());
		assertEquals("shop_buy", e.get(1).kind);
		assertEquals(Integer.valueOf(1351), e.get(1).itemId);
		assertEquals(Integer.valueOf(1), e.get(1).qty);
		assertEquals(Integer.valueOf(2813), e.get(1).npcId, "the shopkeeper");
	}

	@Test
	void theQueueGoesToTheSameCharacterAndShrinksOnAck()
	{
		DemonstrationRecorder r = recording();
		r.onNpc(T, LUMBRIDGE, 1, "Talk-to", "Hans");
		r.onNpc(T, LUMBRIDGE, 2, "Talk-to", "Cook");
		r.onNpc(T, LUMBRIDGE, 3, "Talk-to", "Bob");
		assertTrue(r.pending("Someone Else", 10).isEmpty(), "never to another character");
		List<IngestContract.RecordedEvent> first = r.pending("Iron Test", 2);
		assertEquals(2, first.size());
		r.acknowledge(first);
		assertEquals(1, r.pendingCount());
		assertEquals(3, r.pending("Iron Test", 10).get(0).seq);

		// a logout stops recording; what is pending still goes out
		r.stop();
		assertFalse(r.recording());
		assertFalse(r.onNpc(T, LUMBRIDGE, 4, "Talk-to", "Duke"));
		assertEquals(1, r.pending("Iron Test", 10).size());

		// the same character's next login starts a new recording from seq 1
		r.start("Iron Test", "rec-next");
		r.onNpc(T, LUMBRIDGE, 4, "Talk-to", "Duke");
		List<IngestContract.RecordedEvent> e = r.pending("Iron Test", 10);
		assertEquals(2, e.size());
		assertEquals("rec-next", e.get(1).recording);
		assertEquals(1, e.get(1).seq);

		// another character's recording drops the first one's pending events
		r.start("Someone Else", "rec-other");
		assertEquals(0, r.pendingCount());
		assertTrue(r.recordingFor("Someone Else"));
	}

	@Test
	void aFullQueueDropsNewEvents()
	{
		DemonstrationRecorder r = recording();
		for (int i = 0; i < DemonstrationRecorder.MAX_PENDING; i++)
		{
			assertTrue(r.onItemOp(T, LUMBRIDGE, 1, "Eat", null));
		}
		assertFalse(r.onItemOp(T, LUMBRIDGE, 1, "Eat", null));
		assertEquals(1, r.dropped());
		assertEquals(DemonstrationRecorder.MAX_PER_PUSH, r.pending("Iron Test", DemonstrationRecorder.MAX_PER_PUSH).size());
	}

	@Test
	void textFollowsThePlannerRules()
	{
		assertEquals("Bank booth", DemonstrationRecorder.text("  <col=ffff>Bank booth</col> "));
		assertEquals("Yes please", DemonstrationRecorder.text("Yes\nplease"));
		assertNull(DemonstrationRecorder.text("<col=ff0000></col>"));
		assertNull(DemonstrationRecorder.text(null));
		StringBuilder long1 = new StringBuilder();
		for (int i = 0; i < 200; i++)
		{
			long1.append('x');
		}
		assertEquals(120, DemonstrationRecorder.text(long1.toString()).length());
	}

	@Test
	void theMenuClassifierRecordsOnlyTheOwnersInteractions()
	{
		assertEquals(DemonstrationMenu.Click.NPC, DemonstrationMenu.classify(MenuAction.NPC_FIRST_OPTION, 0, false, "Talk-to"));
		assertEquals(DemonstrationMenu.Click.NPC, DemonstrationMenu.classify(MenuAction.NPC_THIRD_OPTION, 0, false, "Trade"));
		assertEquals(DemonstrationMenu.Click.OBJECT, DemonstrationMenu.classify(MenuAction.GAME_OBJECT_FIFTH_OPTION, 0, false, "Open"));
		assertEquals(DemonstrationMenu.Click.USE_ON_NPC, DemonstrationMenu.classify(MenuAction.WIDGET_TARGET_ON_NPC, 0, false, "Use"));
		assertEquals(DemonstrationMenu.Click.USE_ON_OBJECT, DemonstrationMenu.classify(MenuAction.WIDGET_TARGET_ON_GAME_OBJECT, 0, false, "Use"));
		assertEquals(DemonstrationMenu.Click.USE_ON_ITEM, DemonstrationMenu.classify(MenuAction.WIDGET_TARGET_ON_WIDGET, InterfaceID.Inventory.ITEMS, false, "Use"));
		assertEquals(DemonstrationMenu.Click.DIALOGUE, DemonstrationMenu.classify(MenuAction.WIDGET_CONTINUE, InterfaceID.Chatmenu.OPTIONS, false, ""));
		assertEquals(DemonstrationMenu.Click.ITEM_OP, DemonstrationMenu.classify(MenuAction.CC_OP, InterfaceID.Inventory.ITEMS, true, "Break"));

		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.CC_OP, InterfaceID.Inventory.ITEMS, true, "Drop"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.CC_OP, InterfaceID.Inventory.ITEMS, false, "Use"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.CC_OP, InterfaceID.Bankmain.ITEMS, true, "Withdraw-1"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.WIDGET_CONTINUE, 0, false, "Continue"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.WALK, 0, false, "Walk here"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.EXAMINE_NPC, 0, false, "Examine"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(MenuAction.WIDGET_TARGET_ON_PLAYER, 0, false, "Use"));
		assertEquals(DemonstrationMenu.Click.NONE, DemonstrationMenu.classify(null, 0, false, null));

		assertEquals(1, DemonstrationMenu.keyOption('1'));
		assertEquals(5, DemonstrationMenu.keyOption('5'));
		assertEquals(-1, DemonstrationMenu.keyOption(' '));
		assertEquals(-1, DemonstrationMenu.keyOption('0'));
		assertTrue(DemonstrationMenu.dialogueWidget(InterfaceID.Chatmenu.OPTIONS));
		assertFalse(DemonstrationMenu.dialogueWidget(InterfaceID.Inventory.ITEMS));
		assertEquals(" Sink", DemonstrationSource.objectName("<col=ff9040>Bucket</col> -> Sink"));
	}

	/** The same JSON as the planner's recording.TestWireShape: both sides of the contract agree. */
	@Test
	void wireShape()
	{
		DemonstrationRecorder r = new DemonstrationRecorder();
		r.start("Iron Test", "rec-5b1c");
		r.onNpc(T, LUMBRIDGE, 1, "Talk-to", "Hans");
		r.onNpc(T, LUMBRIDGE, 2, "Talk-to", "Cook");
		r.onObject(T, LUMBRIDGE, 10583, "Bank", "Bank booth", tile(3208, 3220));
		IngestContract.RecordedEvent object = all(r).get(2);
		assertEquals("{\"recording\":\"rec-5b1c\",\"seq\":3,\"at\":\"2026-11-13T12:00:00Z\",\"kind\":\"object\",\"world\":330,\"x\":3222,\"y\":3218,\"plane\":0,"
			+ "\"object_id\":10583,\"target_x\":3208,\"target_y\":3220,\"option\":\"Bank\",\"name\":\"Bank booth\"}", GSON.toJson(object));
	}

	@Test
	void pushesCarryThePendingEvents()
	{
		DemonstrationRecorder r = recording();
		r.onNpc(T, LUMBRIDGE, 3080, "Talk-to", "Duke Horacio");
		PlayerSnapshot acked = Fixtures.simple(7, 40);
		PlayerSnapshot same = Fixtures.simple(7, 40);
		assertNull(PushBuilder.delta(acked, same, 2, T), "nothing changed, nothing pending");

		PlayerSnapshot withEvents = same.withRecording(r.pending("Iron Test", DemonstrationRecorder.MAX_PER_PUSH));
		IngestContract.Push delta = PushBuilder.delta(acked, withEvents, 2, T);
		assertEquals(1, delta.data.recording.size(), "pending events alone are worth a delta");
		assertEquals("npc", delta.data.recording.get(0).kind);
		assertTrue(GSON.toJson(PushBuilder.full(withEvents, 1, T).data).contains("\"recording\":[{\"recording\":\"rec-5b1c\""));
		assertFalse(GSON.toJson(PushBuilder.full(same, 1, T).data).contains("recording"));
		assertNull(same.withRecording(Collections.emptyList()).recording, "no events: the field stays out");
		assertFalse(PushBuilder.requiresFull(acked, withEvents), "events never force a full");
	}

	@Test
	void theManifestFlagGatesTheRecording()
	{
		IngestContract.Manifest m = GSON.fromJson("{\"contract_version\":1,\"manifest_version\":7,\"token_scopes\":[\"state:write\"],\"recording\":true}",
			IngestContract.Manifest.class);
		assertTrue(m.recordingEnabled());
		assertTrue(EventFilter.of(m).recording());
		assertEquals(CaptureTrigger.Strength.SOFT, EventFilter.of(m).recorded());
		IngestContract.Manifest old = GSON.fromJson("{\"contract_version\":1,\"manifest_version\":7,\"token_scopes\":[\"state:write\"]}",
			IngestContract.Manifest.class);
		assertFalse(old.recordingEnabled(), "an older planner does not take data.recording");
		assertEquals(CaptureTrigger.Strength.NONE, EventFilter.of(old).recorded());
	}
}
