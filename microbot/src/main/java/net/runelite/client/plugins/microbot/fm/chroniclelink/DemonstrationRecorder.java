package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Deque;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.TreeMap;
import java.util.TreeSet;
import java.util.function.Consumer;

/**
 * The demonstration recording (planner docs/10 section 11; decision E8: opt-in): with "Record
 * demonstration" on, the owner's own interactions become typed events with the game's ids, which
 * the planner turns into proposed tasks for a route step. It only records; it never acts.
 *
 * <p>What it records, all from what the owner did: a coarse position (the first of a recording,
 * another world, plane or map region, and where the character stopped after walking at least
 * {@link #STOP_TILES} tiles), menu options on NPCs and objects, dialogue options chosen, items
 * used on something and inventory item options, the bank opening and what left or entered it,
 * and what entered the inventory while a shop was open. Never chat, other players, keys typed or
 * walking tile by tile.
 *
 * <p>Pure apart from the lock: the client thread feeds it what it read, the sync thread copies
 * the pending events into each capture and drops them once a push carrying them is
 * acknowledged. Pending events survive a logout and go to the same character's account only.
 */
final class DemonstrationRecorder
{
	/** Events kept while the planner is unreachable; past it new events are dropped (counted). */
	static final int MAX_PENDING = 5000;
	/** Events per push (the planner takes up to 500). */
	static final int MAX_PER_PUSH = 200;
	/** A stop is recorded only this far (Chebyshev, in tiles) from the last position recorded. */
	static final int STOP_TILES = 5;
	/** Game ticks on the same tile that make a stop. */
	static final int STOP_TICKS = 2;
	/** Coins: what a shop takes, never a purchase. */
	static final int COINS = 995;

	static final String MOVE = "move";
	static final String NPC = "npc";
	static final String OBJECT = "object";
	static final String DIALOGUE = "dialogue";
	static final String SHOP_BUY = "shop_buy";
	static final String BANK_OPEN = "bank_open";
	static final String BANK_WITHDRAW = "bank_withdraw";
	static final String BANK_DEPOSIT = "bank_deposit";
	static final String ITEM_USE = "item_use";
	static final String ITEM_OP = "item_op";

	/** A tile and the world. */
	static final class Tile
	{
		final int world;
		final int x;
		final int y;
		final int plane;

		Tile(int world, int x, int y, int plane)
		{
			this.world = world;
			this.x = x;
			this.y = y;
			this.plane = plane;
		}

		int region()
		{
			return (x >> 6) << 8 | y >> 6;
		}

		boolean sameTile(Tile o)
		{
			return o != null && world == o.world && x == o.x && y == o.y && plane == o.plane;
		}

		int distance(Tile o)
		{
			return Math.max(Math.abs(x - o.x), Math.abs(y - o.y));
		}
	}

	/** The interface whose item changes are recorded. */
	enum Open
	{
		NONE,
		BANK,
		SHOP
	}

	private final Deque<IngestContract.RecordedEvent> pending = new ArrayDeque<>();
	/** The character the pending events belong to; null before the first recording. */
	private String owner;
	/** The current recording's id; null while not recording. */
	private String recording;
	private long seq;
	private int dropped;

	private Tile lastSample;
	private Tile lastTick;
	private int stillTicks;

	/** The NPC or object of the last click, the context of a dialogue, a shop or the bank. */
	private Integer lastNpc;
	private Integer lastObject;
	/** A dialogue option was recorded for the options on screen. */
	private boolean dialogueChosen;

	private Open open = Open.NONE;
	private Map<Integer, Long> carried;
	private Integer shopkeeper;

	/** Whether a recording is on. */
	synchronized boolean recording()
	{
		return recording != null;
	}

	/** Whether a recording is on for this character. */
	synchronized boolean recordingFor(String username)
	{
		return recording != null && username != null && username.equals(owner);
	}

	/**
	 * Starts a recording for a character. Events still pending from another character are
	 * dropped: they could only ever reach that character's account.
	 */
	synchronized void start(String username, String recordingId)
	{
		if (owner != null && !owner.equals(username))
		{
			pending.clear();
		}
		owner = username;
		recording = recordingId;
		seq = 0;
		resetContext();
	}

	/** Stops recording (toggle off, logout); the pending events are still sent. */
	synchronized void stop()
	{
		recording = null;
		resetContext();
	}

	private void resetContext()
	{
		lastSample = null;
		lastTick = null;
		stillTicks = 0;
		lastNpc = null;
		lastObject = null;
		dialogueChosen = false;
		open = Open.NONE;
		carried = null;
		shopkeeper = null;
	}

	/** One game tick: records a position sample when it is worth one. */
	synchronized boolean onTick(Instant now, Tile at)
	{
		if (recording == null || at == null)
		{
			return false;
		}
		if (lastSample == null || at.world != lastSample.world || at.plane != lastSample.plane || at.region() != lastSample.region())
		{
			return sample(now, at);
		}
		stillTicks = at.sameTile(lastTick) ? stillTicks + 1 : 0;
		lastTick = at;
		if (stillTicks == STOP_TICKS && at.distance(lastSample) >= STOP_TILES)
		{
			return sample(now, at);
		}
		return false;
	}

	private boolean sample(Instant now, Tile at)
	{
		lastSample = at;
		lastTick = at;
		stillTicks = 0;
		return add(now, at, MOVE, e ->
		{
		});
	}

	/** A menu option clicked on an NPC. */
	synchronized boolean onNpc(Instant now, Tile at, int npcId, String option, String name)
	{
		boolean added = add(now, at, NPC, e ->
		{
			e.npcId = npcId;
			e.option = option;
			e.name = name;
		});
		if (added)
		{
			lastNpc = npcId;
			lastObject = null;
		}
		return added;
	}

	/** A menu option clicked on a game object; the object's tile when known. */
	synchronized boolean onObject(Instant now, Tile at, int objectId, String option, String name, Tile object)
	{
		boolean added = add(now, at, OBJECT, e ->
		{
			e.objectId = objectId;
			e.option = option;
			e.name = name;
			if (object != null)
			{
				e.targetX = object.x;
				e.targetY = object.y;
			}
		});
		if (added)
		{
			lastObject = objectId;
			lastNpc = null;
		}
		return added;
	}

	/** The dialogue options interface (re)loaded: a new choice can be recorded. */
	synchronized void onDialogueOptions()
	{
		dialogueChosen = false;
	}

	/** A dialogue option chosen, once per options shown (a click and a key press are the same choice). */
	synchronized boolean onDialogue(Instant now, Tile at, String text)
	{
		if (dialogueChosen || text == null || text.isEmpty())
		{
			return false;
		}
		Integer npc = lastNpc;
		boolean added = add(now, at, DIALOGUE, e ->
		{
			e.option = text;
			e.npcId = npc;
		});
		dialogueChosen = added;
		return added;
	}

	/** An item used on an NPC, an object or another item (exactly one target). */
	synchronized boolean onItemUse(Instant now, Tile at, int itemId, Integer npcId, Integer objectId, Integer targetItemId, String name)
	{
		int targets = (npcId != null ? 1 : 0) + (objectId != null ? 1 : 0) + (targetItemId != null ? 1 : 0);
		if (targets != 1)
		{
			return false;
		}
		return add(now, at, ITEM_USE, e ->
		{
			e.itemId = itemId;
			e.npcId = npcId;
			e.objectId = objectId;
			e.targetItemId = targetItemId;
			e.option = "Use";
			e.name = name;
		});
	}

	/** An inventory item's own option ("Rub", "Break"). */
	synchronized boolean onItemOp(Instant now, Tile at, int itemId, String option, String name)
	{
		return add(now, at, ITEM_OP, e ->
		{
			e.itemId = itemId;
			e.option = option;
			e.name = name;
		});
	}

	/** The bank opened; carried is the inventory and equipment together, the baseline of the changes. */
	synchronized boolean onBankOpened(Instant now, Tile at, Map<Integer, Long> carried)
	{
		if (recording == null)
		{
			return false;
		}
		open = Open.BANK;
		this.carried = new TreeMap<>(carried);
		Integer npc = lastNpc;
		Integer object = lastObject;
		return add(now, at, BANK_OPEN, e ->
		{
			e.npcId = npc;
			e.objectId = npc == null ? object : null;
		});
	}

	/** A shop opened; inventory is the baseline of the purchases. */
	synchronized void onShopOpened(Map<Integer, Long> inventory)
	{
		if (recording == null)
		{
			return;
		}
		open = Open.SHOP;
		carried = new TreeMap<>(inventory);
		shopkeeper = lastNpc;
	}

	/** The bank or shop closed. */
	synchronized void onClosed(Open which)
	{
		if (open == which)
		{
			open = Open.NONE;
			carried = null;
			shopkeeper = null;
		}
	}

	/**
	 * The inventory or the equipment changed. With the bank open, what the two hold together
	 * grew (withdrawn) or shrank (deposited), so equipping an item is neither; with a shop open,
	 * what entered the inventory, other than coins, was bought. Returns how many events it added.
	 */
	synchronized int onCarried(Instant now, Tile at, Map<Integer, Long> inventory, Map<Integer, Long> worn)
	{
		if (recording == null || open == Open.NONE || carried == null)
		{
			return 0;
		}
		Map<Integer, Long> now2 = new TreeMap<>(inventory);
		if (open == Open.BANK)
		{
			worn.forEach((id, qty) -> now2.merge(id, qty, Long::sum));
		}
		Set<Integer> ids = new TreeSet<>(carried.keySet());
		ids.addAll(now2.keySet());
		int added = 0;
		for (int id : ids)
		{
			long delta = now2.getOrDefault(id, 0L) - carried.getOrDefault(id, 0L);
			if (delta == 0 || delta > Integer.MAX_VALUE || delta < -Integer.MAX_VALUE)
			{
				continue;
			}
			int qty = (int) Math.abs(delta);
			if (open == Open.BANK)
			{
				if (add(now, at, delta > 0 ? BANK_WITHDRAW : BANK_DEPOSIT, e ->
				{
					e.itemId = id;
					e.qty = qty;
				}))
				{
					added++;
				}
			}
			else if (delta > 0 && id != COINS)
			{
				Integer npc = shopkeeper;
				if (add(now, at, SHOP_BUY, e ->
				{
					e.itemId = id;
					e.qty = qty;
					e.npcId = npc;
				}))
				{
					added++;
				}
			}
		}
		carried = now2;
		return added;
	}

	private boolean add(Instant now, Tile at, String kind, Consumer<IngestContract.RecordedEvent> fill)
	{
		if (recording == null || at == null)
		{
			return false;
		}
		if (pending.size() >= MAX_PENDING)
		{
			dropped++;
			return false;
		}
		IngestContract.RecordedEvent e = new IngestContract.RecordedEvent();
		e.recording = recording;
		e.seq = ++seq;
		e.at = PushBuilder.timestamp(now);
		e.kind = kind;
		e.world = at.world;
		e.x = at.x;
		e.y = at.y;
		e.plane = at.plane;
		fill.accept(e);
		e.option = text(e.option);
		e.name = text(e.name);
		pending.addLast(e);
		return true;
	}

	/** The planner's text rules: trimmed, no tags, at most 120 bytes; empty is absent. */
	static String text(String s)
	{
		if (s == null)
		{
			return null;
		}
		String t = s.replaceAll("<[^>]*>", " ").replaceAll("\\p{Cntrl}", " ").replaceAll("[<>]", "").replaceAll("\\s+", " ").trim();
		while (t.getBytes(java.nio.charset.StandardCharsets.UTF_8).length > 120)
		{
			t = t.substring(0, t.length() - 1).trim();
		}
		return t.isEmpty() ? null : t;
	}

	/** The oldest pending events of a character, at most max of them, to go out in a push. */
	synchronized List<IngestContract.RecordedEvent> pending(String username, int max)
	{
		if (owner == null || !owner.equals(username) || pending.isEmpty())
		{
			return Collections.emptyList();
		}
		List<IngestContract.RecordedEvent> out = new ArrayList<>(Math.min(max, pending.size()));
		for (IngestContract.RecordedEvent e : pending)
		{
			if (out.size() == max)
			{
				break;
			}
			out.add(e);
		}
		return out;
	}

	/** A push carrying these events was acknowledged: they are stored, drop them. */
	synchronized void acknowledge(List<IngestContract.RecordedEvent> sent)
	{
		if (sent == null || sent.isEmpty())
		{
			return;
		}
		Set<IngestContract.RecordedEvent> done = Collections.newSetFromMap(new IdentityHashMap<>());
		done.addAll(sent);
		pending.removeIf(done::contains);
	}

	synchronized int pendingCount()
	{
		return pending.size();
	}

	synchronized int dropped()
	{
		return dropped;
	}
}
