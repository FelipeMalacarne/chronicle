package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static net.runelite.client.plugins.microbot.fm.chroniclelink.Fixtures.T0;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicReference;
import okhttp3.HttpUrl;
import org.junit.jupiter.api.Test;

class RoutePanelControllerTest
{
	private static final HttpUrl BASE = HttpUrl.parse("http://127.0.0.1:8080");
	private static final String INSTALLATION = "3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c";

	/** Records every call and answers with the queued results (default: the fixture window). */
	static final class FakeApi implements PanelApi
	{
		final List<String> calls = new ArrayList<>();
		final List<PlannerHttpClient.Result<RouteWindow>> answers = new ArrayList<>();
		RouteWindow.Mark lastRestore;

		private PlannerHttpClient.Result<RouteWindow> next()
		{
			return answers.isEmpty() ? new PlannerHttpClient.Result<>(200, RouteWindowFixtures.window()) : answers.remove(0);
		}

		@Override
		public PlannerHttpClient.Result<RouteWindow> fetchRoute(HttpUrl base, PlannerCredential credential, int limit)
		{
			calls.add("GET route " + limit);
			return next();
		}

		@Override
		public PlannerHttpClient.Result<RouteWindow> mark(HttpUrl base, PlannerCredential credential, String guide, String stepId, String kind, int limit)
		{
			calls.add("PATCH " + guide + "/" + stepId + " " + kind);
			return next();
		}

		@Override
		public PlannerHttpClient.Result<RouteWindow> restore(HttpUrl base, PlannerCredential credential, String guide, String stepId, RouteWindow.Mark mark, int limit)
		{
			calls.add("POST restore " + guide + "/" + stepId + " " + mark.kind + ":" + mark.impliedBy);
			lastRestore = mark;
			return next();
		}
	}

	private final FakeApi api = new FakeApi();
	private final List<PanelState> published = new ArrayList<>();
	private final AtomicReference<RoutePanelController.Endpoint> endpoint =
		new AtomicReference<>(new RoutePanelController.Endpoint(BASE, new NoCredential(INSTALLATION, "")));
	private final RoutePanelController controller = new RoutePanelController(api, endpoint::get, published::add);

	private static PlannerHttpClient.Result<RouteWindow> fail(int status)
	{
		return new PlannerHttpClient.Result<>(status, null);
	}

	@Test
	void refreshCachesTheWindowAndKeepsItWhenThePlannerIsDown()
	{
		controller.refresh(T0);
		PanelState online = controller.state();
		assertEquals(PanelState.Connection.ONLINE, online.connection);
		assertEquals("1.1.7", online.window.cursorStepId());
		assertEquals(T0, online.fetchedAt);
		assertEquals(List.of("GET route " + RoutePanelController.WINDOW_STEPS), api.calls);

		api.answers.add(fail(0));
		controller.refresh(T0.plusSeconds(40));
		PanelState stale = controller.state();
		assertEquals(PanelState.Connection.STALE, stale.connection, "the cached window stays, marked outdated");
		assertTrue(stale.stale());
		assertSame(online.window, stale.window);
		assertEquals(T0, stale.fetchedAt, "the age is the cached window's");
		assertEquals("servidor fora do ar", stale.message);
		assertTrue(PanelText.connection(stale, T0.plusSeconds(100)).startsWith("Desatualizado"));

		controller.refresh(T0.plusSeconds(80));
		assertEquals(PanelState.Connection.ONLINE, controller.state().connection);
		assertNull(controller.state().message);
		assertEquals(3, published.size(), "every change is published");
	}

	@Test
	void nothingCachedAndOfflineIsOffline()
	{
		api.answers.add(fail(401));
		controller.refresh(T0);
		assertEquals(PanelState.Connection.OFFLINE, controller.state().connection);
		assertNull(controller.state().window);
		assertEquals("credencial recusada", controller.state().message);
	}

	@Test
	void notConfiguredSendsNothing()
	{
		endpoint.set(null);
		controller.refresh(T0);
		controller.mark("1.1.7", RoutePanelController.MANUAL_DONE, T0);
		assertTrue(api.calls.isEmpty());
		assertEquals(PanelState.Connection.NOT_CONFIGURED, controller.state().connection);
		assertFalse(controller.pollDue(T0, true));
	}

	@Test
	void aChangedEndpointDropsTheCacheAndTheUndo()
	{
		controller.refresh(T0);
		controller.mark("1.1.7", RoutePanelController.MANUAL_DONE, T0);
		assertEquals("1.1.7", controller.state().undoRef);
		endpoint.set(new RoutePanelController.Endpoint(BASE, new TokenCredential("ipl_other")));
		controller.onPushAck(Fixtures.ack(1, 7), T0);
		assertEquals(3, api.calls.size(), "nothing cached for the new credential: refetch");
		api.answers.add(fail(0));
		endpoint.set(new RoutePanelController.Endpoint(BASE, new TokenCredential("ipl_third")));
		controller.refresh(T0);
		assertNull(controller.state().window, "another credential never shows the previous cache");
		assertNull(controller.state().undoRef);
	}

	@Test
	void pushAckRefreshesOnlyWhenTheRouteChanged()
	{
		RouteWindow cached = RouteWindowFixtures.window();
		assertTrue(RoutePanelController.shouldRefresh(null, Fixtures.ack(1, 7)), "nothing cached yet");
		assertFalse(RoutePanelController.shouldRefresh(cached, RouteWindowFixtures.ackWithRoute(2, "oziris-main", "1.1.7")),
			"same cursor, nothing changed");
		assertTrue(RoutePanelController.shouldRefresh(cached, RouteWindowFixtures.ackWithRoute(2, "oziris-main", "1.1.8")),
			"the cursor moved");
		assertTrue(RoutePanelController.shouldRefresh(cached, RouteWindowFixtures.ackWithRoute(2, "oziris-main", "1.1.7", "1.1.9")),
			"a step of the route changed");
		assertTrue(RoutePanelController.shouldRefresh(cached, RouteWindowFixtures.ackWithRoute(2, "bruhsailer-main", "1.1.7")),
			"another active route");
		assertTrue(RoutePanelController.shouldRefresh(cached, RouteWindowFixtures.ackWithRoute(2, "oziris-main", null)),
			"the route was finished");

		IngestContract.Response older = Fixtures.ack(2, 7);
		older.stepsChanged = 0L;
		assertFalse(RoutePanelController.shouldRefresh(cached, older), "older planner: no steps changed");
		older.stepsChanged = 3L;
		assertTrue(RoutePanelController.shouldRefresh(cached, older), "older planner: steps changed");

		controller.refresh(T0);
		controller.onPushAck(RouteWindowFixtures.ackWithRoute(2, "oziris-main", "1.1.7"), T0.plusSeconds(1));
		assertEquals(1, api.calls.size(), "an unchanged route is not refetched");
		controller.onPushAck(RouteWindowFixtures.ackWithRoute(3, "oziris-main", "1.1.8", "1.1.7"), T0.plusSeconds(2));
		assertEquals(2, api.calls.size());
	}

	@Test
	void slowPollOnlyWhileVisible()
	{
		assertTrue(controller.pollDue(T0, true), "never fetched");
		assertFalse(controller.pollDue(T0, false), "hidden panels do not poll");
		controller.refresh(T0);
		assertFalse(controller.pollDue(T0.plusSeconds(29), true));
		assertTrue(controller.pollDue(T0.plusSeconds(30), true));
		assertFalse(controller.pollDue(T0.plusSeconds(30), false));
		// a failed attempt also waits the interval: no hammering of a server that is down
		api.answers.add(fail(0));
		controller.refresh(T0.plusSeconds(30));
		assertFalse(controller.pollDue(T0.plusSeconds(40), true));
	}

	@Test
	void feitoAndPularCallTheMarkApiAndShowTheNewWindow()
	{
		controller.refresh(T0);
		RouteWindow after = RouteWindowFixtures.windowAt("1.1.8");
		api.answers.add(new PlannerHttpClient.Result<>(200, after));
		controller.mark("1.1.7", RoutePanelController.MANUAL_DONE, T0.plusSeconds(1));
		assertEquals("PATCH oziris-main/1.1.7 manual_done", api.calls.get(1));
		assertSame(after, controller.state().window, "the mark answers with the new window");
		assertEquals("1.1.7", controller.state().undoRef);

		controller.mark("1.1.9", RoutePanelController.SKIPPED, T0.plusSeconds(2));
		assertEquals("PATCH oziris-main/1.1.9 skipped", api.calls.get(2));
		assertEquals("1.1.9", controller.state().undoRef, "Desfazer undoes the last action");
	}

	@Test
	void aRefusedMarkKeepsTheWindowAndTheUndo()
	{
		controller.refresh(T0);
		controller.mark("1.1.7", RoutePanelController.MANUAL_DONE, T0);
		api.answers.add(fail(403));
		controller.mark("1.1.8", RoutePanelController.SKIPPED, T0);
		assertEquals("1.1.7", controller.state().undoRef, "the refused mark is not undoable");
		assertEquals("o token n\u00e3o tem marks:write", controller.state().message);
		assertEquals(PanelState.Connection.STALE, controller.state().connection);
	}

	@Test
	void desfazerClearsAStepThatHadNoMark()
	{
		controller.refresh(T0);
		controller.mark("1.1.7", RoutePanelController.MANUAL_DONE, T0);
		controller.undo(T0);
		assertEquals("PATCH oziris-main/1.1.7 clear", api.calls.get(2));
		assertNull(controller.state().undoRef);
		controller.undo(T0);
		assertEquals(3, api.calls.size(), "nothing left to undo");
	}

	@Test
	void desfazerPutsBackThePreviousMark()
	{
		controller.refresh(T0);
		// 1.1.9 was skipped; Feito on it, then Desfazer: skipped again
		controller.mark("1.1.9", RoutePanelController.MANUAL_DONE, T0);
		controller.undo(T0);
		assertEquals("PATCH oziris-main/1.1.9 skipped", api.calls.get(2));
	}

	@Test
	void desfazerRestoresAnImpliedMarkThroughTheBatchUndo()
	{
		controller.refresh(T0);
		controller.mark("1.1.10", RoutePanelController.MANUAL_DONE, T0);
		controller.undo(T0);
		assertEquals("POST restore oziris-main/1.1.10 implied:1.1.12", api.calls.get(2));

		controller.mark("1.1.10", RoutePanelController.SKIPPED, T0);
		controller.undo(T0);
		assertEquals("PATCH oziris-main/1.1.10 clear", api.calls.get(4),
			"the batch undo only restores over manual_done, so Pular is undone by clearing");
	}

	@Test
	void aFailedUndoCanBeRetried()
	{
		controller.refresh(T0);
		controller.mark("1.1.7", RoutePanelController.MANUAL_DONE, T0);
		api.answers.add(fail(0));
		controller.undo(T0);
		assertEquals("1.1.7", controller.state().undoRef);
		controller.undo(T0);
		assertNull(controller.state().undoRef);
	}
}
