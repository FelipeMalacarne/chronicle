package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.Set;
import java.util.function.BooleanSupplier;
import net.runelite.api.coords.WorldPoint;

/**
 * The Microbot primitives the task executor uses ({@link TaskMachine}), behind an interface so the
 * machine is tested with fakes; {@link MicrobotTaskGame} is the client's. Every call runs on the
 * executor's own thread, never the client thread, and blocks for at most a few seconds except
 * {@link #walk}.
 */
interface TaskGame
{
	/** Logged in, out of the loading screen. */
	boolean loggedIn();

	/** The local player's tile; null when unknown. */
	WorldPoint position();

	/** Microbot's scripts are paused (the owner, or the client's own stuck-interaction guard). */
	boolean scriptsPaused();

	/** The web walker is configured (its Shortest Path config is loaded). */
	boolean walkerReady();

	/** The web walker's "Avoid wilderness" option. */
	boolean walkerAvoidsWilderness();

	/**
	 * Walks toward target with the web walker until it arrives, gives up, or stop is true (polled at
	 * the walker's checkpoints). True when it arrived or stop came true.
	 */
	boolean walk(WorldPoint target, BooleanSupplier stop);

	/** Clears the walker's route. */
	void stopWalking();

	/** The bank interface is open. */
	boolean bankOpen();

	/** Opens the nearest bank booth, chest or banker, only when one is near (no walking). */
	boolean openBank();

	void closeBank();

	/** Deposits every inventory item but the ones whose unnoted id is in keep. */
	boolean depositAllExcept(Set<Integer> keep);

	/** Quantity of an item in the inventory, its noted form included. */
	int inventoryCount(int unnotedId);

	/** Quantity of an item in the open bank. */
	int bankCount(int unnotedId);

	boolean wearing(int id);

	/** Withdraws qty of an item from the open bank, noted or not. */
	boolean withdraw(int unnotedId, int qty, boolean noted);

	/** Wears an inventory item. */
	boolean wear(int id);

	/** Trades with the nearest of these NPCs that has "Trade", only when one is near. */
	boolean openShop(int[] npcIds);

	boolean shopOpen();

	/** The open shop's stock of an item, or -1 when it does not list it. */
	int shopStock(int itemId);

	/** One "Buy N" of an item (N is 1, 5, 10 or 50). */
	boolean buy(int itemId, int qty);

	void closeShop();

	/** Waits up to timeoutMs for cond; true when it held. */
	boolean waitUntil(BooleanSupplier cond, long timeoutMs) throws InterruptedException;
}
