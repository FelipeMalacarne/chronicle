package net.runelite.client.plugins.microbot.fm.chroniclelink;

import okhttp3.Request;

/**
 * The {@code AUTH_MODE=dev} credential: no secret at all. The planner accepts it only from a
 * loopback connection; the installation id keys this installation's protocol state
 * ({@code client_seq}, manifest version) and the optional account id picks the account when
 * the planner has several.
 */
final class NoCredential implements PlannerCredential
{
	static final String HEADER_INSTALLATION = "X-Ironman-Installation";
	static final String HEADER_ACCOUNT = "X-Ironman-Account";

	private final String installationId;
	private final String account;

	NoCredential(String installationId, String account)
	{
		this.installationId = installationId == null ? "" : installationId.trim();
		this.account = account == null ? "" : account.trim();
	}

	@Override
	public void apply(Request.Builder request)
	{
		request.header(HEADER_INSTALLATION, installationId);
		if (!account.isEmpty())
		{
			request.header(HEADER_ACCOUNT, account);
		}
	}

	@Override
	public boolean ready()
	{
		return InstallationId.isValid(installationId);
	}

	@Override
	public String fingerprint()
	{
		return "none\n" + installationId + "\n" + account;
	}

	@Override
	public String toString()
	{
		return "NoCredential";
	}
}
