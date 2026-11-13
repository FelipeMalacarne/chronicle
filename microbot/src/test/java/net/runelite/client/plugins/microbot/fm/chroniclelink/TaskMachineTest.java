package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.BooleanSupplier;
import net.runelite.api.coords.WorldPoint;
import org.junit.jupiter.api.Test;

/** The executor's state machine over a fake game: what it does, and where it stops. */
class TaskMachineTest
{
	private static final Clock CLOCK = Clock.fixed(Instant.parse("2026-11-13T12:00:00Z"), ZoneOffset.UTC);

	/** A game that answers at once and records what the machine asked. */
	static class FakeGame implements TaskGame
	{
		boolean loggedIn = true;
		boolean paused;
		boolean avoidsWilderness = true;
		boolean walkerReady = true;
		WorldPoint pos = new WorldPoint(3103, 3265, 0);
		/** Where each walk ends; past the list, the walk gets nowhere. */
		List<WorldPoint> walks = new ArrayList<>();
		/** Tiles the walk passes before its end, checked against its stop. */
		List<WorldPoint> path = new ArrayList<>();
		int walked;
		boolean stoppedWalking;
		boolean bankNear = true;
		boolean bankOpen;
		boolean shopNear = true;
		boolean shopOpen;
		boolean withdrawWorks = true;
		boolean buyWorks = true;
		boolean closedBank;
		boolean closedShop;
		Map<Integer, Integer> inventory = new HashMap<>();
		Map<Integer, Integer> bank = new HashMap<>();
		Map<Integer, Integer> stock = new HashMap<>();
		Set<Integer> worn = new HashSet<>();
		List<String> calls = new ArrayList<>();
		Runnable onWalk = () -> {
		};

		@Override
		public boolean loggedIn()
		{
			return loggedIn;
		}

		@Override
		public WorldPoint position()
		{
			return loggedIn ? pos : null;
		}

		@Override
		public boolean scriptsPaused()
		{
			return paused;
		}

		@Override
		public boolean walkerReady()
		{
			return walkerReady;
		}

		@Override
		public boolean walkerAvoidsWilderness()
		{
			return avoidsWilderness;
		}

		@Override
		public boolean walk(WorldPoint target, BooleanSupplier stop)
		{
			calls.add("walk " + target.getX() + "," + target.getY());
			onWalk.run();
			for (WorldPoint p : path)
			{
				pos = p;
				if (stop.getAsBoolean())
				{
					return true;
				}
			}
			if (walked < walks.size())
			{
				pos = walks.get(walked++);
			}
			return stop.getAsBoolean();
		}

		@Override
		public void stopWalking()
		{
			stoppedWalking = true;
		}

		@Override
		public boolean bankOpen()
		{
			return bankOpen;
		}

		@Override
		public boolean openBank()
		{
			calls.add("open bank");
			bankOpen = bankNear;
			return bankNear;
		}

		@Override
		public void closeBank()
		{
			calls.add("close bank");
			closedBank = true;
			bankOpen = false;
		}

		@Override
		public boolean depositAllExcept(Set<Integer> keep)
		{
			calls.add("deposit except " + new java.util.TreeSet<>(keep));
			inventory.keySet().removeIf(id -> {
				if (keep.contains(id))
				{
					return false;
				}
				bank.merge(id, inventory.get(id), Integer::sum);
				return true;
			});
			return true;
		}

		@Override
		public int inventoryCount(int unnotedId)
		{
			return inventory.getOrDefault(unnotedId, 0);
		}

		@Override
		public int bankCount(int unnotedId)
		{
			return bankOpen ? bank.getOrDefault(unnotedId, 0) : 0;
		}

		@Override
		public boolean wearing(int id)
		{
			return worn.contains(id);
		}

		@Override
		public boolean withdraw(int unnotedId, int qty, boolean noted)
		{
			calls.add("withdraw " + qty + " " + unnotedId + (noted ? " noted" : ""));
			if (!withdrawWorks)
			{
				return true; // the click went, the item never came
			}
			int n = Math.min(qty, bank.getOrDefault(unnotedId, 0));
			bank.merge(unnotedId, -n, Integer::sum);
			inventory.merge(unnotedId, n, Integer::sum);
			return true;
		}

		@Override
		public boolean wear(int id)
		{
			calls.add("wear " + id);
			if (inventory.getOrDefault(id, 0) < 1)
			{
				return false;
			}
			inventory.merge(id, -1, Integer::sum);
			worn.add(id);
			return true;
		}

		@Override
		public boolean openShop(int[] npcIds)
		{
			calls.add("trade " + Arrays.toString(npcIds));
			shopOpen = shopNear;
			return shopNear;
		}

		@Override
		public boolean shopOpen()
		{
			return shopOpen;
		}

		@Override
		public int shopStock(int itemId)
		{
			return stock.getOrDefault(itemId, -1);
		}

		@Override
		public boolean buy(int itemId, int qty)
		{
			calls.add("buy " + qty);
			if (buyWorks)
			{
				int n = Math.min(qty, Math.max(0, stock.getOrDefault(itemId, 1000)));
				stock.computeIfPresent(itemId, (k, v) -> v - n);
				inventory.merge(itemId, n, Integer::sum);
			}
			return true;
		}

		@Override
		public void closeShop()
		{
			calls.add("close shop");
			closedShop = true;
		}

		@Override
		public boolean waitUntil(BooleanSupplier cond, long timeoutMs)
		{
			return cond.getAsBoolean();
		}
	}

	static TaskMachine run(TaskSpec spec, FakeGame game)
	{
		TaskMachine m = new TaskMachine("5f2a9c0d11e84b7a", spec, game, CLOCK);
		m.run();
		return m;
	}

	static TaskSpec castle()
	{
		return TaskSpecTest.spec(TaskSpec.TRAVEL);
	}

	static TaskSpec bankSpec()
	{
		return TaskSpecTest.spec(TaskSpec.BANK);
	}

	static TaskSpec buySpec(int qty)
	{
		TaskSpec t = TaskSpecTest.spec(TaskSpec.BUY);
		t.buy.qty = qty;
		return t;
	}

	static String trail(TaskMachine m)
	{
		return String.join(" | ", m.trail());
	}

	// --- travel ---

	@Test
	void travelWalksIntoTheArea()
	{
		FakeGame g = new FakeGame();
		g.walks.add(new WorldPoint(3217, 3219, 0));
		TaskMachine m = run(castle(), g);
		assertEquals(TaskMachine.State.DONE, m.state(), trail(m));
		assertEquals(Arrays.asList("walk 3217,3219"), g.calls);
		assertTrue(trail(m).contains("andando até Lumbridge Castle"));
		assertTrue(trail(m).contains("na área de Lumbridge Castle"));
	}

	@Test
	void travelAlreadyThereDoesNothing()
	{
		FakeGame g = new FakeGame();
		g.pos = new WorldPoint(3210, 3220, 0);
		TaskMachine m = run(castle(), g);
		assertEquals(TaskMachine.State.DONE, m.state());
		assertTrue(g.calls.isEmpty());
		assertTrue(trail(m).contains("já está em"));
	}

	@Test
	void travelTriesThreeTimesThenFails()
	{
		FakeGame g = new FakeGame();
		g.walks.add(new WorldPoint(3150, 3240, 0));
		TaskMachine m = run(castle(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertEquals(TaskMachine.WALK_TRIES, g.calls.size());
		assertTrue(m.reason().contains("não chegou"), m.reason());
	}

	@Test
	void travelStopsAWalkThatEntersTheWilderness()
	{
		FakeGame g = new FakeGame();
		g.path.add(new WorldPoint(3100, 3500, 0));
		g.path.add(new WorldPoint(3100, 3530, 0)); // past the ditch
		g.path.add(new WorldPoint(3217, 3219, 0));
		TaskMachine m = run(castle(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("entrou na wilderness"), m.reason());
		assertTrue(g.stoppedWalking);
	}

	@Test
	void travelRefusesAWalkerThatDoesNotAvoidTheWilderness()
	{
		FakeGame g = new FakeGame();
		g.avoidsWilderness = false;
		TaskMachine m = run(castle(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("Avoid wilderness"), m.reason());
		assertTrue(g.calls.isEmpty(), "never walked");
	}

	@Test
	void travelNeedsTheWalker()
	{
		FakeGame g = new FakeGame();
		g.walkerReady = false;
		TaskMachine m = run(castle(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("webwalker do Microbot"), m.reason());
		assertTrue(g.calls.isEmpty(), "never walked");
	}

	@Test
	void travelIntoTheWildernessIsTheTasksOwnRisk()
	{
		TaskSpec mageBank = null;
		for (com.google.gson.JsonElement e : TaskSpecTest.wire().getAsJsonArray("starts"))
		{
			TaskSpec t = TaskSpec.parse(new com.google.gson.Gson(), e.getAsJsonObject().get("task"));
			if (t.travel != null && t.travel.wilderness)
			{
				mageBank = t;
			}
		}
		FakeGame g = new FakeGame();
		g.avoidsWilderness = false;
		g.path.add(new WorldPoint(3090, 3700, 0));
		g.walks.add(new WorldPoint(3090, 3956, 0));
		TaskMachine m = run(mageBank, g);
		assertEquals(TaskMachine.State.DONE, m.state(), trail(m));
	}

	@Test
	void cancelStopsTheWalk()
	{
		FakeGame g = new FakeGame();
		TaskMachine[] self = new TaskMachine[1];
		g.onWalk = () -> self[0].cancel();
		self[0] = new TaskMachine("5f2a9c0d11e84b7a", castle(), g, CLOCK);
		self[0].run();
		assertEquals(TaskMachine.State.CANCELED, self[0].state());
		assertTrue(g.stoppedWalking);
		assertEquals(1, g.calls.size());
	}

	@Test
	void aPauseOrALogoutStopsTheMachine()
	{
		FakeGame paused = new FakeGame();
		paused.paused = true;
		TaskMachine p = run(castle(), paused);
		assertEquals(TaskMachine.State.FAILED, p.state());
		assertTrue(p.reason().contains("pausados"));

		FakeGame out = new FakeGame();
		out.loggedIn = false;
		TaskMachine o = run(bankSpec(), out);
		assertEquals(TaskMachine.State.FAILED, o.state());
		assertTrue(o.reason().contains("não está no jogo"));
	}

	// --- bank ---

	@Test
	void bankLeavesTheInventoryWithTheSetup()
	{
		FakeGame g = new FakeGame();
		g.inventory.put(1511, 5); // logs: not of the setup
		g.inventory.put(995, 1000); // coins: some already carried
		g.bank.put(2347, 1);
		g.bank.put(995, 9000);
		g.bank.put(317, 200);
		g.bank.put(1351, 1); // the bronze axe: the last candidate of the axe
		g.worn.add(1205); // a dagger stays worn
		TaskMachine m = run(bankSpec(), g);
		assertEquals(TaskMachine.State.DONE, m.state(), trail(m));
		assertEquals(Arrays.asList(
			"deposit except [139, 141, 143, 317, 995, 1349, 1351, 1353, 1355, 1357, 1359, 2347, 2434]",
			"withdraw 1 2347",
			"withdraw 1500 995",
			"withdraw 50 317 noted",
			"withdraw 1 1351",
			"wear 1351",
			"deposit except [139, 141, 143, 317, 995, 1349, 1351, 1353, 1355, 1357, 1359, 2347, 2434]",
			"close bank"), g.calls.subList(1, g.calls.size()));
		assertEquals("open bank", g.calls.get(0));
		assertEquals(5, (int) g.bank.get(1511));
		assertEquals(2500, (int) g.inventory.get(995));
		assertTrue(g.worn.contains(1351));
		assertTrue(trail(m).contains("Prayer potion"), "the optional potion is noted: " + trail(m));
	}

	@Test
	void bankWithoutABankNearFails()
	{
		FakeGame g = new FakeGame();
		g.bankNear = false;
		TaskMachine m = run(bankSpec(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("nenhum banco por perto"));
	}

	@Test
	void bankMissingARequiredItemFails()
	{
		FakeGame g = new FakeGame();
		g.bank.put(995, 9000);
		TaskMachine m = run(bankSpec(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("falta Hammer no banco"), m.reason());
		assertFalse(g.closedBank, "it stops where it is");
	}

	@Test
	void bankWhoseWithdrawNeverComesFails()
	{
		FakeGame g = new FakeGame();
		g.bank.put(2347, 1);
		g.withdrawWorks = false;
		TaskMachine m = run(bankSpec(), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("não aconteceu"), m.reason());
	}

	// --- buy ---

	@Test
	void buyInTheShopsBatchesWithoutBuyingMore()
	{
		FakeGame g = new FakeGame();
		g.stock.put(1351, 100);
		g.inventory.put(1351, 2);
		TaskMachine m = run(buySpec(66), g);
		assertEquals(TaskMachine.State.DONE, m.state(), trail(m));
		assertEquals(Arrays.asList("trade [2812, 10619]", "buy 50", "buy 10", "buy 5", "buy 1", "close shop"), g.calls);
		assertEquals(68, (int) g.inventory.get(1351));
	}

	@Test
	void buyStopsWhenTheShopRunsOut()
	{
		FakeGame g = new FakeGame();
		g.stock.put(1351, 3);
		TaskMachine m = run(buySpec(5), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("está sem Bronze axe"), m.reason());
		assertTrue(m.reason().contains("comprados 3 de 5"), m.reason());
	}

	@Test
	void buyThatNeverHappensFails()
	{
		FakeGame g = new FakeGame();
		g.buyWorks = false;
		TaskMachine m = run(buySpec(1), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertEquals(TaskMachine.BUY_MISSES, g.calls.stream().filter(c -> c.startsWith("buy")).count());
		assertTrue(m.reason().contains("não aconteceu"));
	}

	@Test
	void buyWithoutTheKeeperNearFails()
	{
		FakeGame g = new FakeGame();
		g.shopNear = false;
		TaskMachine m = run(buySpec(1), g);
		assertEquals(TaskMachine.State.FAILED, m.state());
		assertTrue(m.reason().contains("não está por perto"));
	}

	@Test
	void batchesAreTheShopsOptions()
	{
		assertEquals(50, TaskMachine.batch(66));
		assertEquals(10, TaskMachine.batch(16));
		assertEquals(5, TaskMachine.batch(9));
		assertEquals(1, TaskMachine.batch(4));
		assertEquals(1, TaskMachine.batch(1));
	}

	@Test
	void theViewCarriesTheLinesAndTheEnd()
	{
		FakeGame g = new FakeGame();
		g.shopNear = false;
		Map<String, Object> v = run(buySpec(1), g).view();
		assertEquals("5f2a9c0d11e84b7a", v.get("id"));
		assertEquals("buy", v.get("type"));
		assertEquals("failed", v.get("state"));
		List<?> events = (List<?>) v.get("events");
		assertEquals(2, events.size());
		assertEquals(2, ((Map<?, ?>) events.get(1)).get("seq"));
		assertTrue(v.containsKey("finishedAt"));
	}
}
