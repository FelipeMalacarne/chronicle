package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.function.Function;

/**
 * The planner URL and {@code ipl_} token a Chronicle-managed worker hands the plugin through the
 * process environment (planner docs/11 section 13.2). When both variables are set and not blank
 * they replace the config's Server URL and credential: token mode with that token. Nothing of it
 * is written to the profile's config, so a worker's profile never keeps the token. Without them
 * the config decides, as before 1.11.0. The token is never logged or exposed by {@link #toString}.
 */
final class LinkEnvironment
{
	static final String SERVER_URL = "CHRONICLE_LINK_SERVER_URL";
	static final String TOKEN = "CHRONICLE_LINK_TOKEN";

	final String serverUrl;
	private final String token;

	private LinkEnvironment(String serverUrl, String token)
	{
		this.serverUrl = serverUrl;
		this.token = token;
	}

	/** The environment's link, or null unless both variables are set and not blank. */
	static LinkEnvironment read(Function<String, String> env)
	{
		String url = trimmed(env.apply(SERVER_URL));
		String token = trimmed(env.apply(TOKEN));
		if (url.isEmpty() || token.isEmpty())
		{
			return null;
		}
		return new LinkEnvironment(url, token);
	}

	/** The server URL in effect: the environment's, else the config's. */
	static String serverUrl(LinkEnvironment env, String configured)
	{
		return env != null ? env.serverUrl : configured;
	}

	/** The credential mode in effect: token with the environment, else the config's. */
	static ChronicleLinkConfig.CredentialMode mode(LinkEnvironment env, ChronicleLinkConfig.CredentialMode configured)
	{
		return env != null ? ChronicleLinkConfig.CredentialMode.TOKEN : configured;
	}

	PlannerCredential credential()
	{
		return new TokenCredential(token);
	}

	private static String trimmed(String value)
	{
		return value == null ? "" : value.trim();
	}

	@Override
	public String toString()
	{
		return "LinkEnvironment";
	}
}
