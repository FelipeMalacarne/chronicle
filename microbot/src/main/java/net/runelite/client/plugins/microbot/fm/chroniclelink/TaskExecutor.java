package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Clock;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import java.util.function.Consumer;
import java.util.regex.Pattern;

/**
 * Chronicle Link's task executor (planner docs/10 phase 3): one {@link TaskMachine} at a time on
 * its own daemon thread, started, read and canceled only through the control bridge
 * (/bridge/task/start, /bridge/task, /bridge/task/cancel), so only the planner's agent starts it,
 * and only in the dev harness client (where the bridge exists). Nothing here runs by itself.
 */
final class TaskExecutor
{
	private static final Pattern ID = Pattern.compile("^[0-9a-f]{8,64}$");

	private final TaskGame game;
	private final Clock clock;
	/** Called with the machine when it ends (the plugin asks the ingest for a capture). */
	private final Consumer<TaskMachine> onEnd;
	private ExecutorService thread;
	private TaskMachine current;
	private boolean stopped;

	TaskExecutor(TaskGame game, Clock clock, Consumer<TaskMachine> onEnd)
	{
		this.game = game;
		this.clock = clock;
		this.onEnd = onEnd;
	}

	/** Thrown for a start the executor refuses while another task runs. */
	static final class BusyException extends RuntimeException
	{
		BusyException(String message)
		{
			super(message);
		}
	}

	/**
	 * Starts a task; IllegalArgumentException for a bad id or task, BusyException while another
	 * runs, IllegalStateException once stopped.
	 */
	synchronized TaskMachine start(String id, TaskSpec spec)
	{
		if (id == null || !ID.matcher(id).matches())
		{
			throw new IllegalArgumentException("task: id must be 8 to 64 hex characters");
		}
		spec.check();
		if (stopped)
		{
			throw new IllegalStateException("the executor is stopped");
		}
		if (current != null && current.state() == TaskMachine.State.RUNNING)
		{
			throw new BusyException("another task is running (" + current.id + ")");
		}
		if (thread == null)
		{
			thread = Executors.newSingleThreadExecutor(r -> {
				Thread t = new Thread(r, "chronicle-link-task");
				t.setDaemon(true);
				return t;
			});
		}
		TaskMachine m = new TaskMachine(id, spec, game, clock);
		current = m;
		try
		{
			thread.execute(() -> {
				m.run();
				onEnd.accept(m);
			});
		}
		catch (RejectedExecutionException e)
		{
			current = null;
			throw new IllegalStateException("the executor is stopped");
		}
		return m;
	}

	/** The current or last task, or {"state": "idle"}. */
	synchronized Map<String, Object> view()
	{
		if (current == null)
		{
			Map<String, Object> idle = new LinkedHashMap<>();
			idle.put("state", "idle");
			return Collections.unmodifiableMap(idle);
		}
		return current.view();
	}

	/** Cancels the task with this id when it is the current one; true when it was. */
	synchronized boolean cancel(String id)
	{
		if (current == null || !current.id.equals(id))
		{
			return false;
		}
		current.cancel();
		return true;
	}

	/** Cancels whatever runs and stops the thread (the plugin shuts down). */
	synchronized void stop()
	{
		stopped = true;
		if (current != null)
		{
			current.cancel();
		}
		if (thread != null)
		{
			thread.shutdownNow();
			thread = null;
		}
	}

	/** Waits for the current task to end (tests). */
	void awaitIdle(long timeoutMs) throws InterruptedException
	{
		long deadline = System.currentTimeMillis() + timeoutMs;
		while (System.currentTimeMillis() < deadline)
		{
			TaskMachine m;
			synchronized (this)
			{
				m = current;
			}
			if (m == null || m.state() != TaskMachine.State.RUNNING)
			{
				return;
			}
			Thread.sleep(5);
		}
	}
}
