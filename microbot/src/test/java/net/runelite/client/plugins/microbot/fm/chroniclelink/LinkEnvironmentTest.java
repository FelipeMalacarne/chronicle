package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.HashMap;
import java.util.Map;
import okhttp3.Request;
import org.junit.jupiter.api.Test;

class LinkEnvironmentTest
{
	private static LinkEnvironment read(String url, String token)
	{
		Map<String, String> env = new HashMap<>();
		if (url != null)
		{
			env.put(LinkEnvironment.SERVER_URL, url);
		}
		if (token != null)
		{
			env.put(LinkEnvironment.TOKEN, token);
		}
		return LinkEnvironment.read(env::get);
	}

	@Test
	void bothVariablesAreNeeded()
	{
		assertNull(read(null, null), "no variables: the config decides");
		assertNull(read("http://chronicle.chronicle.svc", null));
		assertNull(read(null, "ipl_secret"));
		assertNull(read("", "ipl_secret"), "empty counts as unset");
		assertNull(read("http://chronicle.chronicle.svc", "  "), "blank counts as unset");
		assertNotNull(read("http://chronicle.chronicle.svc", "ipl_secret"));
	}

	@Test
	void theEnvironmentReplacesTheConfigUrlAndCredential()
	{
		LinkEnvironment env = read(" http://chronicle.chronicle.svc ", " ipl_secret ");
		assertEquals("http://chronicle.chronicle.svc", LinkEnvironment.serverUrl(env, "https://chronicle.saradomin.ftm.dev.br"));
		assertEquals(ChronicleLinkConfig.CredentialMode.TOKEN, LinkEnvironment.mode(env, ChronicleLinkConfig.CredentialMode.LOGIN));
		assertEquals(ChronicleLinkConfig.CredentialMode.TOKEN, LinkEnvironment.mode(env, ChronicleLinkConfig.CredentialMode.NONE));

		PlannerCredential c = env.credential();
		assertTrue(c instanceof TokenCredential);
		assertTrue(c.ready());
		Request.Builder b = new Request.Builder().url("http://chronicle.chronicle.svc/api/ingest");
		c.apply(b);
		Request r = b.build();
		assertEquals("Bearer ipl_secret", r.header("Authorization"));
		assertNull(r.header(NoCredential.HEADER_INSTALLATION), "token mode sends no installation id");
		assertEquals(new TokenCredential("ipl_secret").fingerprint(), c.fingerprint());
	}

	@Test
	void withoutTheEnvironmentTheConfigDecides()
	{
		assertEquals("https://chronicle.saradomin.ftm.dev.br", LinkEnvironment.serverUrl(null, "https://chronicle.saradomin.ftm.dev.br"));
		assertEquals(ChronicleLinkConfig.CredentialMode.LOGIN, LinkEnvironment.mode(null, ChronicleLinkConfig.CredentialMode.LOGIN));
		assertEquals(ChronicleLinkConfig.CredentialMode.NONE, LinkEnvironment.mode(null, ChronicleLinkConfig.CredentialMode.NONE));
	}

	@Test
	void theTokenStaysOutOfToStringAndThePanelTexts()
	{
		LinkEnvironment env = read("http://chronicle.chronicle.svc", "ipl_secret");
		assertFalse(env.toString().contains("ipl_secret"));
		assertFalse(env.credential().toString().contains("ipl_secret"));
		assertTrue(PanelText.ENVIRONMENT_SOURCE_TOOLTIP.contains(LinkEnvironment.SERVER_URL));
		assertTrue(PanelText.ENVIRONMENT_SOURCE_TOOLTIP.contains(LinkEnvironment.TOKEN));
	}
}
