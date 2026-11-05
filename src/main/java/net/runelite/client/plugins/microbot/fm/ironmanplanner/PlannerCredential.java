package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import okhttp3.Request;

/**
 * How the plugin authenticates to the planner (ironman-planner docs/04-arquitetura.md 2.5,
 * decision 28 of docs/01). The planner stays the source of truth; a credential only decides
 * what a request carries. Implementations never expose a secret through {@code toString},
 * logs or URLs.
 *
 * <ul>
 * <li>{@link NoCredential}: {@code AUTH_MODE=dev}. No secret; the server accepts it only over
 * loopback and keys the protocol state by the installation id.</li>
 * <li>{@link TokenCredential}: a per-account {@code ipl_} token, accepted in every mode until the
 * device flow replaces it.</li>
 * <li>Seam for {@code AUTH_MODE=oidc} (planner item 15, not implemented): a device-flow credential
 * that discovers issuer and client id at {@code GET /api/auth/plugin}, shows the user code, keeps
 * the refresh token in the RuneLite config, renews the access token off the client thread and
 * applies {@code Authorization: Bearer <jwt>}. It only needs to implement this interface and be
 * returned by {@link #of}.</li>
 * </ul>
 */
interface PlannerCredential
{
	/** Adds the credential to a request. */
	void apply(Request.Builder request);

	/** Whether requests can be sent with it (a token is set...). */
	boolean ready();

	/**
	 * Opaque value that changes whenever the credential changes, so the plugin can drop its
	 * protocol state. Never logged and never sent.
	 */
	String fingerprint();

	/** The credential the config selects. */
	static PlannerCredential of(IronmanPlannerConfig.CredentialMode mode, String token, String installationId, String account)
	{
		if (mode == IronmanPlannerConfig.CredentialMode.TOKEN)
		{
			return new TokenCredential(token);
		}
		return new NoCredential(installationId, account);
	}
}
