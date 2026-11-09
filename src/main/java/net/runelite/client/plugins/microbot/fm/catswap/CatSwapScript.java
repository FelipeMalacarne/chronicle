package net.runelite.client.plugins.microbot.fm.catswap;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.NPC;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.gameval.ItemID;
import net.runelite.api.gameval.NpcID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.statemachine.StateMachineScript;
import net.runelite.client.plugins.microbot.statemachine.Transition;
import net.runelite.client.plugins.microbot.util.dialogues.Rs2Dialogue;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;
import net.runelite.client.plugins.microbot.util.walker.Rs2Walker;

/**
 * CHECK (where the cat is, coins) → PICK_UP (a following cat into the inventory) → SELL (walk to
 * the Civilians, use the cat on one) → BUY (walk to Gertrude, her "Kitten" option) → RELEASE (drop
 * the kitten so it follows) → DONE. Each onState does at most one game action and the next tick
 * re-derives from what the game shows; DONE and FAILED submit the outcome and stop the plugin, so
 * whoever started it sees it inactive with a result.
 */
@Slf4j
public class CatSwapScript extends StateMachineScript<CatSwapScript.State>
{
	enum State
	{
		CHECK,
		PICK_UP,
		SELL,
		BUY,
		RELEASE,
		DONE,
		FAILED
	}

	/** What follows the player. */
	enum Follower
	{
		NONE,
		CAT,
		KITTEN,
		OTHER
	}

	// West Ardougne, east of the Underground Pass, where the three Civilians walk (OSRS Wiki
	// "Civilian"); through the Ardougne wall door, which opens after Biohazard.
	static final WorldPoint CIVILIANS = new WorldPoint(2481, 3303, 0);
	static final int CIVILIANS_RADIUS = 18;
	// Gertrude's house, west of Varrock.
	static final WorldPoint GERTRUDE = new WorldPoint(3151, 3409, 0);
	static final int GERTRUDE_RADIUS = 6;
	static final int KITTEN_PRICE = 100;
	/** Interactions (pick up, sell, buy, drop) that may fail before the swap gives up. */
	static final int MAX_TRIES = 5;

	static final int[] CIVILIAN_NPCS = {NpcID.WANTCAT1, NpcID.WANTCAT2, NpcID.WANTCAT3};
	// Grown cats of every kind the Civilians take: adult, lazy, wily, overgrown and hell.
	static final int[] CAT_NPCS = {
		NpcID.GROWNCAT, NpcID.GROWNCAT_LIGHT, NpcID.GROWNCAT_BROWN, NpcID.GROWNCAT_BLACK,
		NpcID.GROWNCAT_BROWNGREY, NpcID.GROWNCAT_BLUEGREY, NpcID.GROWNCAT_HELL,
		NpcID.LAZYCAT_LIGHT, NpcID.LAZYCAT_BROWN, NpcID.LAZYCAT_BLACK, NpcID.LAZYCAT_BROWNGREY,
		NpcID.LAZYCAT_BLUEGREY, NpcID.LAZYCAT_HELL,
		NpcID.WILEYCAT_LIGHT, NpcID.WILEYCAT_BROWN, NpcID.WILEYCAT_BLACK, NpcID.WILEYCAT_BROWNGREY,
		NpcID.WILEYCAT_BLUEGREY, NpcID.WILEYCAT_HELL,
		NpcID.OVERGROWNCAT, NpcID.OVERGROWNCAT_LIGHT, NpcID.OVERGROWNCAT_BROWN, NpcID.OVERGROWNCAT_BLACK,
		NpcID.OVERGROWNCAT_BROWNGREY, NpcID.OVERGROWNCAT_BLUEGREY, NpcID.OVERGROWNCAT_HELL
	};
	static final int[] KITTEN_NPCS = {
		NpcID.KITTENPET1, NpcID.KITTENPET_LIGHT, NpcID.KITTENPET_BROWN, NpcID.KITTENPET_BLACK,
		NpcID.KITTENPET_BROWNGREY, NpcID.KITTENPET_BLUEGREY, NpcID.KITTENPET_HELL
	};
	static final int[] CAT_ITEMS = {
		ItemID.GROWNCATOBJECT, ItemID.GROWNCATOBJECT_LIGHT, ItemID.GROWNCATOBJECT_BROWN,
		ItemID.GROWNCATOBJECT_BLACK, ItemID.GROWNCATOBJECT_BROWNGREY, ItemID.GROWNCATOBJECT_BLUEGREY,
		ItemID.GROWNCATOBJECT_HELL,
		ItemID.OVERGROWNCATOBJECT, ItemID.OVERGROWNCATOBJECT_LIGHT, ItemID.OVERGROWNCATOBJECT_BROWN,
		ItemID.OVERGROWNCATOBJECT_BLACK, ItemID.OVERGROWNCATOBJECT_BROWNGREY, ItemID.OVERGROWNCATOBJECT_BLUEGREY,
		ItemID.OVERGROWNCATOBJECT_HELL,
		ItemID.LAZYCATOBJECT_BROWN, ItemID.LAZYCATOBJECT_BLACK, ItemID.LAZYCATOBJECT_BROWNGREY,
		ItemID.LAZYCATOBJECT_BLUEGREY, ItemID.LAZYCATOBJECT_HELL,
		ItemID.WILEYCATOBJECT, ItemID.WILEYCATOBJECT_LIGHT, ItemID.WILEYCATOBJECT_BROWN,
		ItemID.WILEYCATOBJECT_BLACK, ItemID.WILEYCATOBJECT_BROWNGREY, ItemID.WILEYCATOBJECT_BLUEGREY,
		ItemID.WILEYCATOBJECT_HELL
	};
	static final int[] KITTEN_ITEMS = {
		ItemID.KITTENOBJECT, ItemID.KITTENOBJECT_LIGHT, ItemID.KITTENOBJECT_BROWN, ItemID.KITTENOBJECT_BLACK,
		ItemID.KITTENOBJECT_BROWNGREY, ItemID.KITTENOBJECT_BLUEGREY, ItemID.KITTENOBJECT_HELL
	};

	private CatSwapConfig config;
	private volatile Follower follower = Follower.NONE;
	private boolean checked;
	private boolean sold;
	private String failReason;
	private int tries;
	private State triesState;
	private int deathRunesBefore;
	private boolean finished;

	/** The follower's kind from its NPC id (-1: nothing follows). */
	static Follower followerOf(int npcId)
	{
		if (npcId < 0)
		{
			return Follower.NONE;
		}
		if (contains(CAT_NPCS, npcId))
		{
			return Follower.CAT;
		}
		return contains(KITTEN_NPCS, npcId) ? Follower.KITTEN : Follower.OTHER;
	}

	static boolean contains(int[] ids, int id)
	{
		return Arrays.stream(ids).anyMatch(i -> i == id);
	}

	@Override
	protected State initialState()
	{
		return State.CHECK;
	}

	@Override
	protected List<Transition<State>> defineTransitions()
	{
		List<Transition<State>> t = new ArrayList<>();
		for (State s : new State[]{State.CHECK, State.PICK_UP, State.SELL, State.BUY, State.RELEASE})
		{
			t.add(Transition.<State>from(s)
				.when(() -> failReason != null, "failReason != null")
				.because("Cannot continue")
				.goTo(State.FAILED));
		}
		t.add(Transition.<State>from(State.CHECK)
			.when(() -> checked && catInInventory(), "checked && catInInventory()")
			.because("The cat is in the inventory")
			.goTo(State.SELL));
		t.add(Transition.<State>from(State.CHECK)
			.when(() -> checked && follower == Follower.CAT, "checked && follower == CAT")
			.because("A grown cat follows")
			.goTo(State.PICK_UP));
		t.add(Transition.<State>from(State.PICK_UP)
			.when(this::catInInventory, "catInInventory()")
			.because("Picked the cat up")
			.goTo(State.SELL));
		t.add(Transition.<State>from(State.SELL)
			.when(() -> sold && !config.buyKitten(), "sold && !buyKitten")
			.because("Cat sold; no kitten wanted")
			.goTo(State.DONE));
		t.add(Transition.<State>from(State.SELL)
			.when(() -> sold, "sold")
			.because("Cat sold for death runes")
			.goTo(State.BUY));
		t.add(Transition.<State>from(State.BUY)
			.when(() -> kittenInInventory() || follower == Follower.KITTEN, "kitten in inventory or following")
			.because("Gertrude gave a kitten")
			.goTo(State.RELEASE));
		t.add(Transition.<State>from(State.RELEASE)
			.when(() -> follower == Follower.KITTEN && !kittenInInventory(), "follower == KITTEN && !kittenInInventory()")
			.because("The kitten follows")
			.goTo(State.DONE));
		return t;
	}

	@Override
	protected void onTransition(State from, State to, String reason)
	{
		log.info("Cat Swap {} -> {}: {}", from, to, reason);
	}

	@Override
	protected void onState(State state)
	{
		follower = readFollower();
		// The Civilian and Gertrude talk after the trade; nothing else moves until it is closed.
		if (state != State.DONE && state != State.FAILED && Rs2Dialogue.hasContinue())
		{
			Rs2Dialogue.clickContinue();
			return;
		}
		switch (state)
		{
			case CHECK:
				check();
				break;
			case PICK_UP:
				pickUp();
				break;
			case SELL:
				sell();
				break;
			case BUY:
				buy();
				break;
			case RELEASE:
				release();
				break;
			case DONE:
				finish(true, null);
				break;
			case FAILED:
				finish(false, failReason);
				break;
		}
	}

	public boolean run(CatSwapConfig config)
	{
		this.config = config;
		checked = false;
		sold = false;
		failReason = null;
		tries = 0;
		triesState = null;
		finished = false;
		deathRunesBefore = -1;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				if (!Microbot.isLoggedIn()) return;
				if (deathRunesBefore < 0)
				{
					deathRunesBefore = Rs2Inventory.count(ItemID.DEATHRUNE);
				}
				step();
			}
			catch (Exception ex)
			{
				log.error("Cat Swap loop failed", ex);
			}
		}, 0, 600, TimeUnit.MILLISECONDS);
		return true;
	}

	private void check()
	{
		if (!catInInventory() && follower != Follower.CAT)
		{
			failReason = follower == Follower.KITTEN
				? "The kitten has not grown yet"
				: "No grown cat follows or is in the inventory";
			return;
		}
		if (config.buyKitten() && Rs2Inventory.count(ItemID.COINS) < KITTEN_PRICE)
		{
			failReason = "Needs " + KITTEN_PRICE + " coins in the inventory for Gertrude's kitten";
			return;
		}
		checked = true;
	}

	private void pickUp()
	{
		if (Rs2Player.isMoving() || !attempt(State.PICK_UP))
		{
			return;
		}
		Microbot.status = "Cat Swap: picking up the cat";
		NPC cat = Microbot.getClientThread().runOnClientThreadOptional(() -> Microbot.getClient().getFollower()).orElse(null);
		if (cat != null && new Rs2NpcModel(cat).click("Pick-up"))
		{
			sleepUntil(this::catInInventory, 3000);
		}
	}

	private void sell()
	{
		if (!catInInventory())
		{
			// Used on a Civilian: the cat left the inventory.
			sold = true;
			return;
		}
		if (Rs2Player.isMoving())
		{
			return;
		}
		if (!near(CIVILIANS, CIVILIANS_RADIUS))
		{
			Microbot.status = "Cat Swap: walking to the West Ardougne Civilians";
			Rs2Walker.walkTo(CIVILIANS, 4);
			return;
		}
		Rs2NpcModel civilian = Microbot.getRs2NpcCache().query()
			.withIds(CIVILIAN_NPCS)
			.nearestOnClientThread();
		if (civilian == null)
		{
			Microbot.status = "Cat Swap: looking for a Civilian";
			Rs2Walker.walkTo(CIVILIANS, 2);
			return;
		}
		if (!attempt(State.SELL))
		{
			return;
		}
		Microbot.status = "Cat Swap: selling the cat";
		int cat = Rs2Inventory.get(CAT_ITEMS).getId();
		if (Rs2Inventory.useItemOnNpc(cat, new net.runelite.client.plugins.microbot.util.npc.Rs2NpcModel(civilian.getNpc())))
		{
			sleepUntil(() -> !catInInventory(), 8000);
		}
	}

	private void buy()
	{
		if (Rs2Player.isMoving())
		{
			return;
		}
		if (!near(GERTRUDE, GERTRUDE_RADIUS))
		{
			Microbot.status = "Cat Swap: walking to Gertrude";
			Rs2Walker.walkTo(GERTRUDE, 2);
			return;
		}
		Rs2NpcModel gertrude = Microbot.getRs2NpcCache().query()
			.withIds(NpcID.GERTRUDE_POST)
			.nearestOnClientThread();
		if (gertrude == null || !attempt(State.BUY))
		{
			return;
		}
		Microbot.status = "Cat Swap: buying a kitten";
		// Her right-click "Kitten" sells one straight away, with no dialogue.
		if (gertrude.click("Kitten"))
		{
			sleepUntil(this::kittenInInventory, 5000);
		}
	}

	private void release()
	{
		if (!kittenInInventory() || !attempt(State.RELEASE))
		{
			return;
		}
		Microbot.status = "Cat Swap: letting the kitten follow";
		if (Rs2Inventory.interact(Rs2Inventory.get(KITTEN_ITEMS).getId(), "Drop"))
		{
			sleepUntil(() -> !kittenInInventory(), 3000);
		}
	}

	/** Counts one try of an interaction in this state; false (and a failure) past MAX_TRIES. */
	private boolean attempt(State state)
	{
		if (triesState != state)
		{
			triesState = state;
			tries = 0;
		}
		if (++tries > MAX_TRIES)
		{
			failReason = "Gave up after " + MAX_TRIES + " tries in " + state;
			return false;
		}
		return true;
	}

	private void finish(boolean ok, String reason)
	{
		if (finished)
		{
			return;
		}
		finished = true;
		Map<String, Object> result = new LinkedHashMap<>();
		result.put("outcome", ok ? "done" : "failed");
		if (reason != null)
		{
			result.put("reason", reason);
		}
		result.put("deathRunesGained", Math.max(0, Rs2Inventory.count(ItemID.DEATHRUNE) - Math.max(0, deathRunesBefore)));
		result.put("kittenFollows", follower == Follower.KITTEN);
		ScriptResultStore.submit(CatSwapPlugin.class.getName(), result);
		Microbot.status = "Cat Swap: " + (ok ? "done" : "failed: " + reason);
		log.info("Cat Swap finished: {}", result);
		// Off, so the agent and the plugin list see it inactive (it is queued on the Swing thread).
		Microbot.stopPlugin(CatSwapPlugin.class);
	}

	private boolean catInInventory()
	{
		return Rs2Inventory.hasItem(CAT_ITEMS);
	}

	private boolean kittenInInventory()
	{
		return Rs2Inventory.hasItem(KITTEN_ITEMS);
	}

	private static boolean near(WorldPoint target, int radius)
	{
		WorldPoint me = Rs2Player.getWorldLocation();
		return me != null && me.getPlane() == target.getPlane() && me.distanceTo(target) <= radius;
	}

	private static Follower readFollower()
	{
		return followerOf(Microbot.getClientThread()
			.runOnClientThreadOptional(() -> {
				NPC f = Microbot.getClient().getFollower();
				return f == null ? -1 : f.getId();
			})
			.orElse(-1));
	}
}
