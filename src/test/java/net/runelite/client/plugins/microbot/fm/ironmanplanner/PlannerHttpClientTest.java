package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

import okhttp3.HttpUrl;
import org.junit.jupiter.api.Test;

class PlannerHttpClientTest
{
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
