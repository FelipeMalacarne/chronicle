package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.io.IOException;
import java.time.Instant;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
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

class LoginSessionTest
{
	private static final HttpUrl BASE = HttpUrl.parse("https://chronicle.example/");
	private static final String INSTALLATION = "3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c";
	private static final String CLIENT = "{\"auth_mode\":\"oidc\",\"issuer\":\"https://auth.example\",\"client_id\":\"native-1\","
		+ "\"scopes\":[\"openid\",\"offline_access\"]}";
	private static final String DISCOVERY = "{\"device_authorization_endpoint\":\"https://auth.example/oauth/v2/device_authorization\","
		+ "\"token_endpoint\":\"https://auth.example/oauth/v2/token\"}";
	private static final String DEVICE = "{\"device_code\":\"dc-1\",\"user_code\":\"ABCD-EFGH\",\"verification_uri\":\"https://auth.example/device\","
		+ "\"verification_uri_complete\":\"https://auth.example/device?user_code=ABCD-EFGH\",\"expires_in\":300,\"interval\":5}";

	/** A fake planner and issuer: answers by path from queued bodies, records the requests. */
	private static final class Issuer
	{
		final List<Request> requests = new ArrayList<>();
		final List<String> forms = new ArrayList<>();
		final Deque<String[]> tokenAnswers = new ArrayDeque<>();
		String client = CLIENT;
		String stored = "";

		LoginSession session()
		{
			OkHttpClient http = new OkHttpClient.Builder().addInterceptor(chain ->
			{
				Request r = chain.request();
				requests.add(r);
				forms.add(bodyOf(r));
				String path = r.url().encodedPath();
				int status = 200;
				String body;
				if (path.equals("/api/auth/client"))
				{
					body = client;
				}
				else if (path.equals("/.well-known/openid-configuration"))
				{
					body = DISCOVERY;
				}
				else if (path.endsWith("/device_authorization"))
				{
					body = DEVICE;
				}
				else
				{
					String[] next = tokenAnswers.removeFirst();
					status = Integer.parseInt(next[0]);
					body = next[1];
				}
				return new Response.Builder().request(r).protocol(Protocol.HTTP_1_1).code(status).message("x")
					.body(ResponseBody.create(MediaType.parse("application/json"), body)).build();
			}).build();
			return new LoginSession(new DeviceLogin(http, new Gson()), () -> stored, t -> stored = t);
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
				return null;
			}
		}
	}

	@Test
	void deviceFlowSignsInAndKeepsTheRefreshToken()
	{
		Issuer issuer = new Issuer();
		LoginSession s = issuer.session();
		Instant t0 = Instant.parse("2026-11-12T00:00:00Z");
		assertEquals(LoginSession.Kind.SIGNED_OUT, s.view().kind);
		assertNull(s.accessToken(BASE, t0));

		s.start(BASE, t0);
		LoginSession.View waiting = s.view();
		assertEquals(LoginSession.Kind.WAITING, waiting.kind);
		assertEquals("ABCD-EFGH", waiting.userCode);
		assertEquals("https://auth.example/device?user_code=ABCD-EFGH", waiting.url);
		String deviceForm = issuer.forms.get(issuer.forms.size() - 1);
		assertTrue(deviceForm.contains("client_id=native-1") && deviceForm.contains("scope=openid%20offline_access"), deviceForm);

		// not due yet, then pending, then slowed down, then confirmed
		assertFalse(s.poll(t0.plusSeconds(1)));
		issuer.tokenAnswers.add(new String[]{"400", "{\"error\":\"authorization_pending\"}"});
		assertFalse(s.poll(t0.plusSeconds(5)));
		issuer.tokenAnswers.add(new String[]{"400", "{\"error\":\"slow_down\"}"});
		assertFalse(s.poll(t0.plusSeconds(10)));
		assertFalse(s.poll(t0.plusSeconds(15)), "slow_down adds 5 s to the interval");
		issuer.tokenAnswers.add(new String[]{"200", "{\"access_token\":\"jwt-1\",\"refresh_token\":\"rt-1\",\"expires_in\":3600}"});
		assertTrue(s.poll(t0.plusSeconds(20)));
		assertTrue(issuer.forms.get(issuer.forms.size() - 1).contains("grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Adevice_code"));
		assertEquals(LoginSession.Kind.SIGNED_IN, s.view().kind);
		assertEquals("rt-1", issuer.stored);
		assertEquals("jwt-1", s.accessToken(BASE, t0.plusSeconds(30)));

		// close to expiry the refresh token renews it, rotated
		issuer.tokenAnswers.add(new String[]{"200", "{\"access_token\":\"jwt-2\",\"refresh_token\":\"rt-2\",\"expires_in\":3600}"});
		assertEquals("jwt-2", s.accessToken(BASE, t0.plusSeconds(3550)));
		assertTrue(issuer.forms.get(issuer.forms.size() - 1).contains("refresh_token=rt-1"));
		assertEquals("rt-2", issuer.stored);

		// a 401 from the planner renews on the next request
		s.dropAccess();
		issuer.tokenAnswers.add(new String[]{"200", "{\"access_token\":\"jwt-3\",\"expires_in\":3600}"});
		assertEquals("jwt-3", s.accessToken(BASE, t0.plusSeconds(3600)));
		assertEquals("rt-2", issuer.stored, "without a new refresh token the old one stays");

		// a revoked refresh token signs out
		s.dropAccess();
		issuer.tokenAnswers.add(new String[]{"400", "{\"error\":\"invalid_grant\"}"});
		assertNull(s.accessToken(BASE, t0.plusSeconds(3700)));
		assertEquals("", issuer.stored);
		LoginSession.View out = s.view();
		assertEquals(LoginSession.Kind.SIGNED_OUT, out.kind);
		assertEquals("o login expirou; entre de novo", out.message);
	}

	@Test
	void refusedExpiredAndDevPlanner()
	{
		Issuer issuer = new Issuer();
		LoginSession s = issuer.session();
		Instant t0 = Instant.parse("2026-11-12T00:00:00Z");
		s.start(BASE, t0);
		issuer.tokenAnswers.add(new String[]{"400", "{\"error\":\"access_denied\"}"});
		assertTrue(s.poll(t0.plusSeconds(5)));
		assertEquals(LoginSession.Kind.SIGNED_OUT, s.view().kind);
		assertEquals("login recusado", s.view().message);

		s.start(BASE, t0);
		assertTrue(s.poll(t0.plusSeconds(301)), "the code expires without asking the issuer");
		assertEquals("o código expirou; entre de novo", s.view().message);

		Issuer dev = new Issuer();
		dev.client = "{\"auth_mode\":\"dev\"}";
		LoginSession d = dev.session();
		d.start(BASE, t0);
		assertEquals(LoginSession.Kind.SIGNED_OUT, d.view().kind);
		assertEquals("o planner está em modo dev: use Credential None", d.view().message);

		d.start(null, t0);
		assertEquals("configure a URL do planner", d.view().message);
	}

	@Test
	void unreachableIssuerKeepsTheTokenAndRetriesLater()
	{
		Issuer issuer = new Issuer();
		issuer.stored = "rt-1";
		LoginSession s = issuer.session();
		Instant t0 = Instant.parse("2026-11-12T00:00:00Z");
		issuer.tokenAnswers.add(new String[]{"503", "<html>down</html>"});
		assertNull(s.accessToken(BASE, t0));
		assertEquals("rt-1", issuer.stored, "an outage never drops the login");
		int calls = issuer.requests.size();
		assertNull(s.accessToken(BASE, t0.plusSeconds(10)));
		assertEquals(calls, issuer.requests.size(), "no new call inside the retry window");
		issuer.tokenAnswers.add(new String[]{"200", "{\"access_token\":\"jwt-1\",\"expires_in\":3600}"});
		assertEquals("jwt-1", s.accessToken(BASE, t0.plusSeconds(31)));
	}

	@Test
	void loginCredentialCarriesTheInstallation()
	{
		Request.Builder b = new Request.Builder().url(BASE);
		LoginCredential c = new LoginCredential("jwt-1", INSTALLATION, "acc_x");
		c.apply(b);
		Request r = b.build();
		assertEquals("Bearer jwt-1", r.header("Authorization"));
		assertEquals(INSTALLATION, r.header(NoCredential.HEADER_INSTALLATION));
		assertEquals("acc_x", r.header(NoCredential.HEADER_ACCOUNT));
		assertTrue(c.ready());
		assertFalse(new LoginCredential(null, INSTALLATION, "").ready());
		assertEquals(c.fingerprint(), new LoginCredential("jwt-2", INSTALLATION, "acc_x").fingerprint(), "a renewal keeps the protocol state");
		assertEquals("LoginCredential", c.toString());
	}
}
