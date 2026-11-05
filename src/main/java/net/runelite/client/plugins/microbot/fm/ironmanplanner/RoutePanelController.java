package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.time.Duration;
import java.time.Instant;
import java.util.Objects;
import java.util.function.Consumer;
import java.util.function.Supplier;
import okhttp3.HttpUrl;

/**
 * State and actions of the route panel. The planner decides everything (verdicts, status,
 * cursor); this class only fetches the window, keeps the last one to show while the planner is
 * unreachable, and turns Feito/Pular/Desfazer into the planner's mark calls. Not thread-safe:
 * every method runs on the panel's own executor, never on the client or Swing thread. Each
 * change is handed to {@code publish} as an immutable {@link PanelState}.
 */
final class RoutePanelController
{
	/** Steps after the cursor in the window. */
	static final int WINDOW_STEPS = 15;
	/** Fallback poll while the panel is visible. */
	static final Duration POLL = Duration.ofSeconds(30);

	static final String MANUAL_DONE = "manual_done";
	static final String SKIPPED = "skipped";
	static final String CLEAR = "clear";
	static final String IMPLIED = "implied";

	/** Where and how to reach the planner; null while not configured. */
	static final class Endpoint
	{
		final HttpUrl base;
		final PlannerCredential credential;

		Endpoint(HttpUrl base, PlannerCredential credential)
		{
			this.base = base;
			this.credential = credential;
		}

		String key()
		{
			return base + "\n" + credential.fingerprint();
		}
	}

	/**
	 * The last mark the panel wrote and what the step had before, so Desfazer can put it back.
	 */
	static final class UndoAction
	{
		final String guide;
		final String stepId;
		final String ref;
		/** manual_done or skipped. */
		final String applied;
		/** The step's mark before, or null. */
		final RouteWindow.Mark previous;

		UndoAction(String guide, String stepId, String ref, String applied, RouteWindow.Mark previous)
		{
			this.guide = guide;
			this.stepId = stepId;
			this.ref = ref;
			this.applied = applied;
			this.previous = previous;
		}

		/**
		 * The planner call that undoes the action. An implied mark can only come back through
		 * the batch restore, which accepts it over a manual_done; after Pular on an implied step
		 * the mark is cleared instead (the planner's restore would refuse it).
		 */
		UndoRequest request()
		{
			if (previous == null || previous.kind == null)
			{
				return UndoRequest.mark(CLEAR);
			}
			if (IMPLIED.equals(previous.kind))
			{
				return MANUAL_DONE.equals(applied) && previous.impliedBy != null
					? UndoRequest.restore(new RouteWindow.Mark(IMPLIED, previous.impliedBy))
					: UndoRequest.mark(CLEAR);
			}
			return UndoRequest.mark(previous.kind);
		}
	}

	/** Either a PATCH mark with {@code kind} or a POST restore with {@code mark}. */
	static final class UndoRequest
	{
		final String kind;
		final RouteWindow.Mark restore;

		private UndoRequest(String kind, RouteWindow.Mark restore)
		{
			this.kind = kind;
			this.restore = restore;
		}

		static UndoRequest mark(String kind)
		{
			return new UndoRequest(kind, null);
		}

		static UndoRequest restore(RouteWindow.Mark mark)
		{
			return new UndoRequest(null, mark);
		}
	}

	private final PanelApi api;
	private final Supplier<Endpoint> endpoint;
	private final Consumer<PanelState> publish;

	private String endpointKey;
	private RouteWindow window;
	private Instant fetchedAt;
	private Instant lastAttemptAt;
	private PanelState.Connection connection = PanelState.Connection.NOT_CONFIGURED;
	private String message;
	private UndoAction undo;

	RoutePanelController(PanelApi api, Supplier<Endpoint> endpoint, Consumer<PanelState> publish)
	{
		this.api = api;
		this.endpoint = endpoint;
		this.publish = publish;
	}

	/** Fetches the window now (login, a changed push, the Atualizar button). */
	void refresh(Instant now)
	{
		Endpoint ep = current();
		if (ep == null)
		{
			emit();
			return;
		}
		lastAttemptAt = now;
		apply(api.fetchRoute(ep.base, ep.credential, WINDOW_STEPS), now, false);
		emit();
	}

	/**
	 * An acknowledged push: refetch only when it changed the route (the response's cursor and
	 * changed steps), or when nothing is cached yet.
	 */
	void onPushAck(IngestContract.Response response, Instant now)
	{
		if (current() == null)
		{
			emit();
			return;
		}
		if (shouldRefresh(window, response))
		{
			refresh(now);
		}
	}

	/** Whether an acknowledged push makes the cached window outdated. */
	static boolean shouldRefresh(RouteWindow cached, IngestContract.Response response)
	{
		if (cached == null)
		{
			return true;
		}
		if (response == null)
		{
			return false;
		}
		IngestContract.RouteUpdate route = response.route;
		if (route == null)
		{
			// older planner: only the step count tells
			return response.stepsChanged != null && response.stepsChanged > 0;
		}
		return !Objects.equals(route.guide, cached.guide)
			|| !Objects.equals(route.cursor, cached.cursorStepId())
			|| (route.changed != null && !route.changed.isEmpty());
	}

	/** The slow fallback poll: due while the panel is visible and the window is older than POLL. */
	boolean pollDue(Instant now, boolean visible)
	{
		if (!visible || current() == null)
		{
			return false;
		}
		Instant last = lastAttemptAt;
		return last == null || !now.isBefore(last.plus(POLL));
	}

	/** Feito or Pular on a step of the window. */
	void mark(String stepId, String kind, Instant now)
	{
		Endpoint ep = current();
		RouteWindow shown = window;
		if (ep == null || shown == null || shown.guide == null)
		{
			emit();
			return;
		}
		RouteWindow.Step step = shown.step(stepId);
		RouteWindow.Mark previous = step == null ? null : step.mark;
		String ref = step == null ? stepId : step.ref;
		lastAttemptAt = now;
		if (apply(api.mark(ep.base, ep.credential, shown.guide, stepId, kind, WINDOW_STEPS), now, true))
		{
			undo = new UndoAction(shown.guide, stepId, ref, kind, previous);
		}
		emit();
	}

	/** Desfazer: puts back what the last Feito/Pular replaced. */
	void undo(Instant now)
	{
		Endpoint ep = current();
		UndoAction action = undo;
		if (ep == null || action == null)
		{
			emit();
			return;
		}
		UndoRequest req = action.request();
		lastAttemptAt = now;
		PlannerHttpClient.Result<RouteWindow> result = req.restore != null
			? api.restore(ep.base, ep.credential, action.guide, action.stepId, req.restore, WINDOW_STEPS)
			: api.mark(ep.base, ep.credential, action.guide, action.stepId, req.kind, WINDOW_STEPS);
		if (apply(result, now, true))
		{
			undo = null;
		}
		emit();
	}

	/** The last published state (for tests and the first render). */
	PanelState state()
	{
		return new PanelState(connection, window, fetchedAt, message, undo == null ? null : undo.ref);
	}

	/** The endpoint now; a changed URL or credential drops the cache and the undo. */
	private Endpoint current()
	{
		Endpoint ep = endpoint.get();
		String key = ep == null ? null : ep.key();
		if (!Objects.equals(key, endpointKey))
		{
			endpointKey = key;
			window = null;
			fetchedAt = null;
			lastAttemptAt = null;
			undo = null;
			message = null;
			connection = ep == null ? PanelState.Connection.NOT_CONFIGURED : PanelState.Connection.CONNECTING;
		}
		return ep;
	}

	/** Takes a window answer; on failure the cached window stays, marked outdated. */
	private boolean apply(PlannerHttpClient.Result<RouteWindow> result, Instant now, boolean write)
	{
		if (result != null && result.status == 200 && result.body != null && result.body.guide != null)
		{
			window = result.body;
			fetchedAt = now;
			connection = PanelState.Connection.ONLINE;
			message = null;
			return true;
		}
		int status = result == null ? 0 : result.status;
		connection = window == null ? PanelState.Connection.OFFLINE : PanelState.Connection.STALE;
		message = PanelText.error(status, write);
		return false;
	}

	private void emit()
	{
		publish.accept(state());
	}
}
