package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.Gson;
import com.google.gson.JsonParseException;
import com.google.gson.annotations.SerializedName;
import java.io.IOException;
import java.util.List;
import java.util.concurrent.TimeUnit;
import okhttp3.FormBody;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.ResponseBody;

/**
 * The HTTP calls of the plugin's login in {@code AUTH_MODE=oidc} (docs/04 2.5,
 * planner item 15): the OAuth 2.0 Device Authorization Grant (RFC 8628) against the planner's
 * issuer (Zitadel), with the public native client the planner names at
 * {@code GET /api/auth/client}, and the refresh of the access token. No state: {@link LoginSession}
 * keeps it. Blocking; run only on the plugin's own executors. Tokens are never logged.
 */
final class DeviceLogin
{
	static final String DEVICE_GRANT = "urn:ietf:params:oauth:grant-type:device_code";
	private static final long TIMEOUT_SECONDS = 10;
	private static final long MAX_BODY_BYTES = 64 * 1024;

	/** {@code GET /api/auth/client}: how a native client logs in to this planner. */
	static final class ClientInfo
	{
		@SerializedName("auth_mode")
		String authMode;
		String issuer;
		@SerializedName("client_id")
		String clientId;
		List<String> scopes;

		boolean oidc()
		{
			return "oidc".equals(authMode) && issuer != null && !issuer.isEmpty() && clientId != null && !clientId.isEmpty();
		}
	}

	/** The issuer's endpoints, from its discovery document. */
	static final class Endpoints
	{
		@SerializedName("device_authorization_endpoint")
		String device;
		@SerializedName("token_endpoint")
		String token;
	}

	/** The device authorization answer: what the owner types or opens. */
	static final class DeviceCode
	{
		@SerializedName("device_code")
		String deviceCode;
		@SerializedName("user_code")
		String userCode;
		@SerializedName("verification_uri")
		String verificationUri;
		@SerializedName("verification_uri_complete")
		String verificationUriComplete;
		@SerializedName("expires_in")
		long expiresIn;
		long interval;

		/** The page to open: with the code filled in when the issuer offers it. */
		String url()
		{
			return verificationUriComplete != null && !verificationUriComplete.isEmpty() ? verificationUriComplete : verificationUri;
		}
	}

	/** A token answer, or an OAuth error ({@code authorization_pending}, {@code invalid_grant}...). */
	static final class Tokens
	{
		@SerializedName("access_token")
		String accessToken;
		@SerializedName("refresh_token")
		String refreshToken;
		@SerializedName("expires_in")
		long expiresIn;
		String error;

		boolean ok()
		{
			return error == null && accessToken != null && !accessToken.isEmpty();
		}
	}

	/** Thrown for an unreachable server or an answer that is not what the protocol says. */
	static final class LoginException extends Exception
	{
		LoginException(String message)
		{
			super(message);
		}
	}

	private final OkHttpClient http;
	private final Gson gson;

	DeviceLogin(OkHttpClient base, Gson gson)
	{
		this.http = base.newBuilder()
			.callTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.connectTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.readTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.followRedirects(false)
			.build();
		this.gson = gson;
	}

	/** How the planner at {@code base} takes a native login. */
	ClientInfo clientInfo(HttpUrl base) throws LoginException
	{
		Request request = new Request.Builder()
			.url(PlannerHttpClient.endpoint(base, "api", "auth", "client"))
			.header("Accept", "application/json")
			.get()
			.build();
		ClientInfo info = call(request, ClientInfo.class, false);
		if (info == null)
		{
			throw new LoginException("o planner não respondeu como entrar");
		}
		return info;
	}

	/** The issuer's device and token endpoints. */
	Endpoints discover(String issuer) throws LoginException
	{
		HttpUrl url = HttpUrl.parse(issuer.replaceAll("/+$", "") + "/.well-known/openid-configuration");
		if (url == null || !"https".equals(url.scheme()) && !url.host().equals("127.0.0.1") && !url.host().equals("localhost"))
		{
			throw new LoginException("issuer inválido");
		}
		Request request = new Request.Builder().url(url).header("Accept", "application/json").get().build();
		Endpoints e = call(request, Endpoints.class, false);
		if (e == null || HttpUrl.parse(nullToEmpty(e.device)) == null || HttpUrl.parse(nullToEmpty(e.token)) == null)
		{
			throw new LoginException("o issuer não oferece o device flow");
		}
		return e;
	}

	/** Starts a device authorization. */
	DeviceCode start(Endpoints endpoints, ClientInfo client) throws LoginException
	{
		FormBody.Builder form = new FormBody.Builder().add("client_id", client.clientId);
		if (client.scopes != null && !client.scopes.isEmpty())
		{
			form.add("scope", String.join(" ", client.scopes));
		}
		Request request = new Request.Builder().url(endpoints.device).header("Accept", "application/json").post(form.build()).build();
		DeviceCode code = call(request, DeviceCode.class, false);
		if (code == null || code.deviceCode == null || code.userCode == null || code.url() == null)
		{
			throw new LoginException("o issuer recusou o pedido de login");
		}
		return code;
	}

	/** One poll of the token endpoint; an OAuth error comes back in {@link Tokens#error}. */
	Tokens poll(Endpoints endpoints, ClientInfo client, String deviceCode) throws LoginException
	{
		return token(endpoints, new FormBody.Builder()
			.add("grant_type", DEVICE_GRANT)
			.add("device_code", deviceCode)
			.add("client_id", client.clientId)
			.build());
	}

	/** Trades the refresh token for a new access token (and, rotated, a new refresh token). */
	Tokens refresh(Endpoints endpoints, ClientInfo client, String refreshToken) throws LoginException
	{
		return token(endpoints, new FormBody.Builder()
			.add("grant_type", "refresh_token")
			.add("refresh_token", refreshToken)
			.add("client_id", client.clientId)
			.build());
	}

	private Tokens token(Endpoints endpoints, FormBody form) throws LoginException
	{
		Request request = new Request.Builder().url(endpoints.token).header("Accept", "application/json").post(form).build();
		Tokens t = call(request, Tokens.class, true);
		if (t == null || !t.ok() && t.error == null)
		{
			throw new LoginException("resposta inválida do issuer");
		}
		return t;
	}

	/**
	 * The parsed JSON body of a 2xx answer, or of a 4xx when {@code errors} (the token endpoint
	 * answers OAuth errors with 400); null otherwise.
	 */
	private <T> T call(Request request, Class<T> type, boolean errors) throws LoginException
	{
		try (Response response = http.newCall(request).execute())
		{
			ResponseBody body = response.body();
			boolean accepted = response.isSuccessful() || errors && response.code() >= 400 && response.code() < 500;
			if (!accepted || body == null || !isJson(body))
			{
				return null;
			}
			return gson.fromJson(response.peekBody(MAX_BODY_BYTES).string(), type);
		}
		catch (IOException ex)
		{
			throw new LoginException("sem conexão");
		}
		catch (JsonParseException ex)
		{
			return null;
		}
	}

	private static boolean isJson(ResponseBody body)
	{
		MediaType type = body.contentType();
		return type != null && "json".equals(type.subtype());
	}

	private static String nullToEmpty(String s)
	{
		return s == null ? "" : s;
	}
}
