package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Duration;
import java.time.Instant;
import java.util.function.Consumer;
import java.util.function.Supplier;
import okhttp3.HttpUrl;

/**
 * The plugin's login in {@code AUTH_MODE=oidc} (planner item 15): a pending device
 * authorization, the access token in memory and the refresh token in the RuneLite config, read
 * and written through {@code loadRefresh}/{@code storeRefresh}. Thread-safe: the panel thread
 * starts and polls a login, the sync and panel threads ask for the access token, and the
 * blocking calls run under the lock (never on the client or Swing thread). Tokens are never
 * logged or shown.
 */
final class LoginSession
{
	/** Renew the access token this long before it expires. */
	static final Duration RENEW_BEFORE = Duration.ofMinutes(2);
	/** After a failed call to the planner or the issuer, wait this long before trying again. */
	static final Duration RETRY = Duration.ofSeconds(30);

	enum Kind
	{
		/** No refresh token: the panel offers "Entrar". */
		SIGNED_OUT,
		/** A device code is waiting for the owner to confirm it in the browser. */
		WAITING,
		/** A refresh token is kept: requests carry its access token. */
		SIGNED_IN
	}

	/** What the panel shows of the login. */
	static final class View
	{
		final Kind kind;
		/** The code to confirm and the page to open, while WAITING. */
		final String userCode;
		final String url;
		/** What went wrong last (PT-BR), or null. */
		final String message;

		View(Kind kind, String userCode, String url, String message)
		{
			this.kind = kind;
			this.userCode = userCode;
			this.url = url;
			this.message = message;
		}
	}

	private final DeviceLogin api;
	private final Supplier<String> loadRefresh;
	private final Consumer<String> storeRefresh;

	// guarded by this
	private HttpUrl base;
	private DeviceLogin.ClientInfo client;
	private DeviceLogin.Endpoints endpoints;
	private DeviceLogin.DeviceCode pending;
	private Instant pendingUntil;
	private Instant nextPoll;
	private long interval;
	private String access;
	private Instant accessUntil;
	private Instant retryAfter;
	private String message;

	LoginSession(DeviceLogin api, Supplier<String> loadRefresh, Consumer<String> storeRefresh)
	{
		this.api = api;
		this.loadRefresh = loadRefresh;
		this.storeRefresh = storeRefresh;
	}

	synchronized View view()
	{
		if (pending != null)
		{
			return new View(Kind.WAITING, pending.userCode, pending.url(), message);
		}
		return new View(refreshToken().isEmpty() ? Kind.SIGNED_OUT : Kind.SIGNED_IN, null, null, message);
	}

	/** Starts a device authorization against the planner at {@code server}. */
	synchronized void start(HttpUrl server, Instant now)
	{
		pending = null;
		message = null;
		if (server == null)
		{
			message = "configure a URL do planner";
			return;
		}
		try
		{
			issuer(server);
			DeviceLogin.DeviceCode code = api.start(endpoints, client);
			pending = code;
			interval = Math.max(1, code.interval == 0 ? 5 : code.interval);
			pendingUntil = now.plusSeconds(code.expiresIn > 0 ? code.expiresIn : 600);
			nextPoll = now.plusSeconds(interval);
		}
		catch (DeviceLogin.LoginException ex)
		{
			message = ex.getMessage();
		}
	}

	/** Cancels a pending login. */
	synchronized void cancel()
	{
		pending = null;
		message = null;
	}

	/** Forgets the login: the refresh token leaves the config. */
	synchronized void signOut()
	{
		pending = null;
		access = null;
		accessUntil = null;
		message = null;
		storeRefresh.accept("");
	}

	/** The planner refused the access token (401): renew it on the next request. */
	synchronized void dropAccess()
	{
		access = null;
		accessUntil = null;
	}

	/**
	 * Polls a pending login when its interval is due. Returns true when the view changed (the
	 * owner confirmed, refused, or the code expired).
	 */
	synchronized boolean poll(Instant now)
	{
		if (pending == null || now.isBefore(nextPoll))
		{
			return false;
		}
		if (!now.isBefore(pendingUntil))
		{
			pending = null;
			message = "o código expirou; entre de novo";
			return true;
		}
		DeviceLogin.Tokens t;
		try
		{
			t = api.poll(endpoints, client, pending.deviceCode);
		}
		catch (DeviceLogin.LoginException ex)
		{
			nextPoll = now.plusSeconds(interval);
			return false;
		}
		if (t.ok())
		{
			pending = null;
			message = null;
			take(t, now);
			return true;
		}
		switch (t.error)
		{
			case "authorization_pending":
				nextPoll = now.plusSeconds(interval);
				return false;
			case "slow_down":
				interval += 5;
				nextPoll = now.plusSeconds(interval);
				return false;
			case "access_denied":
				message = "login recusado";
				break;
			case "expired_token":
				message = "o código expirou; entre de novo";
				break;
			default:
				message = "o issuer recusou o login";
		}
		pending = null;
		return true;
	}

	/**
	 * A valid access token for the planner at {@code server}, renewed with the refresh token
	 * when close to expiry; null when signed out, or while the planner or the issuer cannot be
	 * reached (retried after {@link #RETRY}).
	 */
	synchronized String accessToken(HttpUrl server, Instant now)
	{
		if (server == null || !server.equals(base))
		{
			access = null;
			accessUntil = null;
		}
		if (access != null && now.isBefore(accessUntil.minus(RENEW_BEFORE)))
		{
			return access;
		}
		String refresh = refreshToken();
		if (refresh.isEmpty() || server == null || retryAfter != null && now.isBefore(retryAfter))
		{
			return validAccess(now);
		}
		try
		{
			issuer(server);
			DeviceLogin.Tokens t = api.refresh(endpoints, client, refresh);
			if (t.ok())
			{
				retryAfter = null;
				message = null;
				take(t, now);
				return access;
			}
			// invalid_grant: revoked or expired; the owner signs in again
			access = null;
			accessUntil = null;
			message = "o login expirou; entre de novo";
			storeRefresh.accept("");
			return null;
		}
		catch (DeviceLogin.LoginException ex)
		{
			retryAfter = now.plus(RETRY);
			return validAccess(now);
		}
	}

	/** The current access token while it has not expired yet. */
	private String validAccess(Instant now)
	{
		return access != null && now.isBefore(accessUntil) ? access : null;
	}

	private void take(DeviceLogin.Tokens t, Instant now)
	{
		access = t.accessToken;
		accessUntil = now.plusSeconds(t.expiresIn > 0 ? t.expiresIn : 300);
		if (t.refreshToken != null && !t.refreshToken.isEmpty())
		{
			storeRefresh.accept(t.refreshToken);
		}
	}

	/** Client info and the issuer's endpoints of {@code server}, fetched once per server. */
	private void issuer(HttpUrl server) throws DeviceLogin.LoginException
	{
		if (server.equals(base) && client != null && endpoints != null)
		{
			return;
		}
		DeviceLogin.ClientInfo info = api.clientInfo(server);
		if (!info.oidc())
		{
			throw new DeviceLogin.LoginException("dev".equals(info.authMode)
				? "o planner está em modo dev: use Credential None"
				: "o planner não aceita login de clientes");
		}
		DeviceLogin.Endpoints e = api.discover(info.issuer);
		base = server;
		client = info;
		endpoints = e;
	}

	private String refreshToken()
	{
		String s = loadRefresh.get();
		return s == null ? "" : s.trim();
	}
}
