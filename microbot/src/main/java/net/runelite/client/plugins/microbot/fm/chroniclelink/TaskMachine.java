package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Clock;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import net.runelite.api.coords.WorldPoint;

/**
 * The executor's local state machine for one typed task (planner docs/10 phase 3): travel walks
 * to the location's area with the web walker, bank leaves the inventory and the equipment with
 * the task's items, buy trades with a shop keeper nearby. It runs on the executor's thread, only
 * because the planner's agent asked through the bridge, and stops at the first thing it cannot
 * do, saying why; it never retries a click blindly or guesses. Its DONE only says the machine
 * finished: the planner proves the task's termination from the ingest (the position in the area,
 * the item containers), never from this.
 *
 * <p>Between steps it checks the cancel, Microbot's pause (the owner's or the client's own guard)
 * and the login. A travel whose destination is not in the wilderness never walks into it: the
 * walker must avoid it, and a walk that enters it anyway is stopped.
 */
final class TaskMachine
{
	enum State
	{
		RUNNING, DONE, FAILED, CANCELED
	}

	static final int MAX_EVENTS = 100;
	static final int WALK_TRIES = 3;
	static final long OPEN_WAIT_MS = 5_000;
	static final long CHANGE_WAIT_MS = 3_000;
	static final int BUY_MISSES = 3;
	private static final int[] BUY_BATCHES = {50, 10, 5, 1};

	final String id;
	final TaskSpec spec;
	private final TaskGame game;
	private final Clock clock;
	private volatile boolean cancel;

	private State state = State.RUNNING;
	private String reason = "";
	private final List<Map<String, Object>> events = new ArrayList<>();
	private int seq;
	private final Instant startedAt;
	private Instant finishedAt;

	TaskMachine(String id, TaskSpec spec, TaskGame game, Clock clock)
	{
		this.id = id;
		this.spec = spec;
		this.game = game;
		this.clock = clock;
		this.startedAt = clock.instant();
	}

	/** Runs the task to its end; the caller's thread is the executor's. */
	void run()
	{
		try
		{
			switch (spec.type)
			{
				case TaskSpec.TRAVEL:
					travel(spec.travel);
					break;
				case TaskSpec.BANK:
					bank(spec.bank);
					break;
				case TaskSpec.BUY:
					buy(spec.buy);
					break;
				default:
					throw new End(State.FAILED, "o executor não roda " + spec.type);
			}
		}
		catch (End e)
		{
			finish(e.state, e.getMessage());
		}
		catch (InterruptedException e)
		{
			Thread.currentThread().interrupt();
			finish(State.CANCELED, "o executor foi interrompido");
		}
		catch (RuntimeException e)
		{
			// The class name only: a message can carry client details.
			finish(State.FAILED, "erro no executor: " + e.getClass().getSimpleName());
		}
	}

	/** Asks the machine to stop at its next checkpoint (the walk polls it too). */
	void cancel()
	{
		cancel = true;
		game.stopWalking();
	}

	synchronized State state()
	{
		return state;
	}

	/** The answer of GET /bridge/task. */
	synchronized Map<String, Object> view()
	{
		Map<String, Object> m = new LinkedHashMap<>();
		m.put("id", id);
		m.put("type", spec.type);
		m.put("state", state.name().toLowerCase());
		m.put("reason", reason);
		m.put("startedAt", startedAt.toEpochMilli());
		if (finishedAt != null)
		{
			m.put("finishedAt", finishedAt.toEpochMilli());
		}
		m.put("events", new ArrayList<>(events));
		return m;
	}

	synchronized List<String> trail()
	{
		List<String> out = new ArrayList<>();
		for (Map<String, Object> e : events)
		{
			out.add((String) e.get("text"));
		}
		return Collections.unmodifiableList(out);
	}

	synchronized String reason()
	{
		return reason;
	}

	// --- travel ---

	private void travel(TaskSpec.Travel t) throws InterruptedException
	{
		checkpoint();
		WorldPoint at = here();
		if (inArea(t, at))
		{
			throw done("já está em " + t.name);
		}
		if (!game.walkerReady())
		{
			throw new End(State.FAILED, "o webwalker do Microbot não está pronto (Shortest Path sem configuração)");
		}
		boolean startedInWild = TaskSpec.inWilderness(at.getX(), at.getY());
		boolean mayEnterWild = t.wilderness || startedInWild;
		if (!mayEnterWild && !game.walkerAvoidsWilderness())
		{
			throw new End(State.FAILED, "o webwalker não evita a wilderness (opção \"Avoid wilderness\" desligada): ligue-a ou ande à mão");
		}
		WorldPoint target = new WorldPoint(t.target.x, t.target.y, t.target.plane);
		for (int attempt = 1; attempt <= WALK_TRIES; attempt++)
		{
			note(attempt == 1 ? "andando até " + t.name : "andando de novo até " + t.name + " (tentativa " + attempt + ")");
			game.walk(target, () -> {
				if (cancel || game.scriptsPaused())
				{
					return true;
				}
				WorldPoint p = game.position();
				return p != null && (inArea(t, p) || (!mayEnterWild && TaskSpec.inWilderness(p.getX(), p.getY())));
			});
			checkpoint();
			WorldPoint now = here();
			if (!mayEnterWild && TaskSpec.inWilderness(now.getX(), now.getY()))
			{
				game.stopWalking();
				throw new End(State.FAILED, "o caminho entrou na wilderness, que a tarefa não pede: parado em " + tile(now));
			}
			if (inArea(t, now))
			{
				throw done("na área de " + t.name + " (" + tile(now) + ")");
			}
			note("o webwalker parou antes, em " + tile(now));
		}
		throw new End(State.FAILED, "não chegou em " + t.name + " em " + WALK_TRIES + " tentativas");
	}

	private static boolean inArea(TaskSpec.Travel t, WorldPoint p)
	{
		return p != null && t.area.contains(p.getX(), p.getY(), p.getPlane());
	}

	// --- bank ---

	private void bank(TaskSpec.Bank b) throws InterruptedException
	{
		checkpoint();
		if (!game.bankOpen())
		{
			note("abrindo o banco mais perto");
			if (!game.openBank() || !game.waitUntil(game::bankOpen, OPEN_WAIT_MS))
			{
				throw new End(State.FAILED, "nenhum banco por perto: falta um travel até um banco antes");
			}
		}
		Set<Integer> keep = new HashSet<>();
		for (TaskSpec.BankItem it : b.items)
		{
			for (int id : it.ids)
			{
				keep.add(id);
			}
		}
		note("depositando o que não é do setup");
		game.depositAllExcept(keep);
		for (TaskSpec.BankItem it : b.items)
		{
			checkpoint();
			if (it.equip)
			{
				wearItem(it);
			}
			else
			{
				carry(it);
			}
		}
		// Gear a swap took off goes back to the bank.
		game.depositAllExcept(keep);
		game.closeBank();
		throw done("banco pronto");
	}

	private void carry(TaskSpec.BankItem it) throws InterruptedException
	{
		int have = carried(it);
		if (have >= it.qty)
		{
			note("já tem " + have + " " + it.name);
			return;
		}
		int need = it.qty - have;
		Integer pick = null;
		for (int id : it.ids)
		{
			int n = game.bankCount(id);
			if (n >= need)
			{
				pick = id;
				break;
			}
			if (n > 0 && pick == null)
			{
				pick = id;
			}
		}
		if (pick == null)
		{
			missing(it, "falta " + it.name + " no banco");
			return;
		}
		final int id = pick;
		int take = Math.min(need, game.bankCount(id));
		int before = game.inventoryCount(id);
		if (!game.withdraw(id, take, it.noted) || !game.waitUntil(() -> game.inventoryCount(id) > before, CHANGE_WAIT_MS))
		{
			missing(it, "o saque de " + it.name + " não aconteceu (inventário cheio?)");
			return;
		}
		int now = carried(it);
		if (now < it.qty)
		{
			missing(it, "só " + now + " de " + it.qty + " " + it.name + " no banco");
			return;
		}
		note("sacado: " + take + " " + it.name + (it.noted ? " (noted)" : ""));
	}

	private void wearItem(TaskSpec.BankItem it) throws InterruptedException
	{
		for (int id : it.ids)
		{
			if (game.wearing(id))
			{
				note("já vestido: " + it.name);
				return;
			}
		}
		Integer carried = null;
		Integer banked = null;
		for (int id : it.ids)
		{
			if (carried == null && game.inventoryCount(id) > 0)
			{
				carried = id;
			}
			if (banked == null && game.bankCount(id) > 0)
			{
				banked = id;
			}
		}
		if (carried == null && banked == null)
		{
			missing(it, "falta " + it.name + " no banco");
			return;
		}
		final int id = carried != null ? carried : banked;
		if (carried == null && (!game.withdraw(id, 1, false) || !game.waitUntil(() -> game.inventoryCount(id) > 0, CHANGE_WAIT_MS)))
		{
			missing(it, "o saque de " + it.name + " não aconteceu (inventário cheio?)");
			return;
		}
		if (!game.wear(id) || !game.waitUntil(() -> game.wearing(id), CHANGE_WAIT_MS))
		{
			missing(it, "não vestiu " + it.name);
			return;
		}
		note("vestido: " + it.name);
	}

	private int carried(TaskSpec.BankItem it)
	{
		int n = 0;
		for (int id : it.ids)
		{
			n += game.inventoryCount(id);
		}
		return n;
	}

	/** An optional item is noted and skipped; a required one ends the task. */
	private void missing(TaskSpec.BankItem it, String why)
	{
		if (!it.optional)
		{
			throw new End(State.FAILED, why);
		}
		note(why + " (opcional, seguindo)");
	}

	// --- buy ---

	private void buy(TaskSpec.Buy b) throws InterruptedException
	{
		checkpoint();
		int start = game.inventoryCount(b.item);
		if (!game.shopOpen())
		{
			note("abrindo " + b.shopName);
			if (!game.openShop(b.npcs) || !game.waitUntil(game::shopOpen, OPEN_WAIT_MS))
			{
				throw new End(State.FAILED, "o lojista de " + b.shopName + " não está por perto: falta um travel antes");
			}
		}
		int bought = 0;
		int misses = 0;
		while (bought < b.qty)
		{
			checkpoint();
			if (game.shopStock(b.item) == 0)
			{
				throw new End(State.FAILED, b.shopName + " está sem " + b.itemName + " (comprados " + bought + " de " + b.qty + ")");
			}
			int batch = batch(b.qty - bought);
			int before = game.inventoryCount(b.item);
			if (!game.buy(b.item, batch) || !game.waitUntil(() -> game.inventoryCount(b.item) > before, CHANGE_WAIT_MS))
			{
				if (++misses >= BUY_MISSES)
				{
					throw new End(State.FAILED, "a compra de " + b.itemName + " não aconteceu (coins? inventário cheio?): comprados " + bought + " de " + b.qty);
				}
				continue;
			}
			misses = 0;
			bought = Math.max(0, game.inventoryCount(b.item) - start);
			note("comprados " + Math.min(bought, b.qty) + " de " + b.qty + " " + b.itemName);
		}
		game.closeShop();
		throw done("compra feita");
	}

	/** The largest of the shop's buy options that does not pass what is left. */
	static int batch(int left)
	{
		for (int n : BUY_BATCHES)
		{
			if (n <= left)
			{
				return n;
			}
		}
		return 1;
	}

	// --- steps ---

	private void checkpoint()
	{
		if (cancel)
		{
			throw new End(State.CANCELED, "cancelada pelo agent");
		}
		if (game.scriptsPaused())
		{
			throw new End(State.FAILED, "os scripts do Microbot estão pausados");
		}
		if (!game.loggedIn())
		{
			throw new End(State.FAILED, "o personagem não está no jogo");
		}
	}

	private WorldPoint here()
	{
		WorldPoint p = game.position();
		if (p == null)
		{
			throw new End(State.FAILED, "sem a posição do personagem");
		}
		return p;
	}

	private End done(String text)
	{
		return new End(State.DONE, text);
	}

	private synchronized void note(String text)
	{
		if (events.size() >= MAX_EVENTS)
		{
			return;
		}
		Map<String, Object> e = new LinkedHashMap<>();
		e.put("seq", ++seq);
		e.put("at", clock.instant().toEpochMilli());
		e.put("text", text);
		events.add(e);
	}

	private void finish(State end, String text)
	{
		// The last line says how it ended; the cap leaves room for it.
		synchronized (this)
		{
			if (events.size() >= MAX_EVENTS)
			{
				events.remove(events.size() - 1);
			}
		}
		note(text);
		synchronized (this)
		{
			state = end;
			reason = end == State.DONE ? "" : text;
			finishedAt = clock.instant();
		}
	}

	private static String tile(WorldPoint p)
	{
		return p.getX() + ", " + p.getY() + ", " + p.getPlane();
	}

	/** Unwinds the machine to its end. */
	private static final class End extends RuntimeException
	{
		final State state;

		End(State state, String message)
		{
			super(message, null, false, false);
			this.state = state;
		}
	}
}
