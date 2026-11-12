package net.runelite.client.plugins.microbot.fm.chroniclelink;

import okhttp3.Request;

/**
 * The {@code AUTH_MODE=oidc} credential (planner item 15): the access token of the plugin's
 * device-flow login ({@link LoginSession}) as a bearer, with the installation id that keys this
 * installation's protocol state and the optional account id. The planner acts for the logged-in
 * user, on that user's accounts only. The token is never logged or put in a URL.
 */
final class LoginCredential implements PlannerCredential
{
	private final String accessToken;
	private final String installationId;
	private final String account;

	LoginCredential(String accessToken, String installationId, String account)
	{
		this.accessToken = accessToken == null ? "" : accessToken;
		this.installationId = installationId == null ? "" : installationId.trim();
		this.account = account == null ? "" : account.trim();
	}

	@Override
	public void apply(Request.Builder request)
	{
		request.header("Authorization", "Bearer " + accessToken);
		request.header(NoCredential.HEADER_INSTALLATION, installationId);
		if (!account.isEmpty())
		{
			request.header(NoCredential.HEADER_ACCOUNT, account);
		}
	}

	@Override
	public boolean ready()
	{
		return !accessToken.isEmpty() && InstallationId.isValid(installationId);
	}

	/** Stable across token renewals: the protocol state belongs to the installation. */
	@Override
	public String fingerprint()
	{
		return "login\n" + installationId + "\n" + account;
	}

	@Override
	public String toString()
	{
		return "LoginCredential";
	}
}
