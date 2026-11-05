package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Protocol;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.ResponseBody;
import okio.Buffer;
import org.junit.jupiter.api.Test;

class PlannerHttpClientTest
{
	private static final String INSTALLATION = "3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c";
	private static final HttpUrl BASE = HttpUrl.parse("http://127.0.0.1:8080/");

	/** Answers every call with {@code body} and records the requests; no network. */
	private static final class Recorder
	{
		final List<Request> requests = new ArrayList<>();
		final List<String> bodies = new ArrayList<>();
		final PlannerHttpClient client;

		Recorder(int status, String body)
		{
			OkHttpClient http = new OkHttpClient.Builder().addInterceptor(chain ->
			{
				Request r = chain.request();
				requests.add(r);
				bodies.add(bodyOf(r));
				return new Response.Builder().request(r).protocol(Protocol.HTTP_1_1).code(status).message("x")
					.body(ResponseBody.create(MediaType.parse("application/json; charset=utf-8"), body)).build();
			}).build();
			client = new PlannerHttpClient(http, new Gson());
		}

		private static String bodyOf(Request r)
		{
			if (r.body() == null)
			{
				return null;
			}
			try
			{
				Buffer b = new Buffer();
				r.body().writeTo(b);
				return b.readUtf8();
			}
			catch (IOException ex)
			{
				throw new IllegalStateException(ex);
			}
		}
	}

	@Test
	void routeWindowIsFetchedWithTheDevCredential()
	{
		Recorder rec = new Recorder(200, RouteWindowFixtures.WINDOW_JSON);
		PlannerHttpClient.Result<RouteWindow> r = rec.client.fetchRoute(BASE, new NoCredential(INSTALLATION, "acc_main"), 15);
		assertEquals(200, r.status);
		assertEquals("1.1.7", r.body.cursorStepId());
		Request req = rec.requests.get(0);
		assertEquals("GET", req.method());
		assertEquals("http://127.0.0.1:8080/api/plugin/route?limit=15", req.url().toString());
		assertNull(req.header("Authorization"), "no token in dev mode");
		assertEquals(INSTALLATION, req.header("X-Ironman-Installation"));
		assertEquals("acc_main", req.header("X-Ironman-Account"));
	}

	@Test
	void feitoPularAndDesfazerAreTheMarkCalls()
	{
		Recorder rec = new Recorder(200, RouteWindowFixtures.WINDOW_JSON);
		PlannerCredential token = new TokenCredential("ipl_secret");
		rec.client.mark(BASE, token, "oziris-main", "1.1.7", "manual_done", 15);
		rec.client.mark(BASE, token, "bruhsailer-main", "a/b c", "skipped", 15);
		rec.client.restore(BASE, token, "oziris-main", "1.1.10", new RouteWindow.Mark("implied", "1.1.12"), 15);

		Request feito = rec.requests.get(0);
		assertEquals("PATCH", feito.method());
		assertEquals("/api/plugin/guides/oziris-main/steps/1.1.7/mark", feito.url().encodedPath());
		assertEquals("15", feito.url().queryParameter("limit"));
		assertEquals("Bearer ipl_secret", feito.header("Authorization"));
		assertEquals("{\"kind\":\"manual_done\"}", rec.bodies.get(0));

		Request pular = rec.requests.get(1);
		assertEquals("/api/plugin/guides/bruhsailer-main/steps/a%2Fb%20c/mark", pular.url().encodedPath(), "step ids are one path segment");
		assertEquals("{\"kind\":\"skipped\"}", rec.bodies.get(1));

		Request undo = rec.requests.get(2);
		assertEquals("POST", undo.method());
		assertEquals("/api/plugin/guides/oziris-main/marks", undo.url().encodedPath());
		assertEquals("{\"restore\":[{\"step_id\":\"1.1.10\",\"mark\":{\"kind\":\"implied\",\"implied_by_step_id\":\"1.1.12\"}}]}", rec.bodies.get(2));
	}

	@Test
	void ingestCallsCarryTheCredentialAndTheRouteOfTheAnswer()
	{
		Recorder rec = new Recorder(200, "{\"contract_version\":1,\"ok\":true,\"accepted_seq\":1,\"resync\":false,\"manifest_version\":7,"
			+ "\"route\":{\"guide\":\"oziris-main\",\"cursor\":\"1.1.8\",\"changed\":[\"1.1.7\"]}}");
		PlannerHttpClient.Result<IngestContract.Response> r = rec.client.push(BASE, new NoCredential(INSTALLATION, ""), new IngestContract.Push());
		assertEquals("1.1.8", r.body.route.cursor);
		assertEquals(INSTALLATION, rec.requests.get(0).header("X-Ironman-Installation"));
		assertNull(rec.requests.get(0).header("Authorization"));
		rec.client.fetchManifest(BASE, new TokenCredential("ipl_secret"));
		assertEquals("Bearer ipl_secret", rec.requests.get(1).header("Authorization"));
		assertTrue(rec.requests.get(1).url().toString().endsWith("/api/ingest/manifest"));
		assertNull(rec.requests.get(1).header("X-Ironman-Installation"));
	}

	@Test
	void aRefusedMarkHasItsStatusAndNoWindow()
	{
		Recorder rec = new Recorder(403, "{\"error\":\"the plugin token lacks the marks:write scope\",\"status\":403}");
		PlannerHttpClient.Result<RouteWindow> r = rec.client.mark(BASE, new TokenCredential("ipl_x"), "oziris-main", "1.1.7", "manual_done", 15);
		assertEquals(403, r.status);
		assertNull(r.body == null ? null : r.body.guide);
	}
	@Test
	void endpointKeepsPathPrefixAndIgnoresTrailingSlash()
	{
		assertEquals("https://planner.example/api/ingest/manifest",
			PlannerHttpClient.endpoint(PlannerHttpClient.parseServer("https://planner.example"), "api", "ingest", "manifest").toString());
		assertEquals("https://planner.example/api/ingest",
			PlannerHttpClient.endpoint(PlannerHttpClient.parseServer("https://planner.example/"), "api", "ingest").toString());
		assertEquals("http://127.0.0.1:8080/planner/api/ingest",
			PlannerHttpClient.endpoint(PlannerHttpClient.parseServer(" http://127.0.0.1:8080/planner/ "), "api", "ingest").toString());
	}

	@Test
	void onlyPlainHttpUrlsAreAccepted()
	{
		assertNull(PlannerHttpClient.parseServer(""));
		assertNull(PlannerHttpClient.parseServer(null));
		assertNull(PlannerHttpClient.parseServer("planner.example"));
		assertNull(PlannerHttpClient.parseServer("ftp://planner.example"));
		assertNull(PlannerHttpClient.parseServer("https://user:pw@planner.example"));
		assertNull(PlannerHttpClient.parseServer("https://planner.example/?token=x"));
		HttpUrl ok = PlannerHttpClient.parseServer("https://planner.example");
		assertEquals("planner.example", ok.host());
	}
}
