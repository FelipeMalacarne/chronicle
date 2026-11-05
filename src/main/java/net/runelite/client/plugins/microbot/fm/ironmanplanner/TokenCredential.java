package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import okhttp3.Request;

/** A per-account {@code ipl_} token in the Authorization header; never logged or put in a URL. */
final class TokenCredential implements PlannerCredential
{
	private final String token;

	TokenCredential(String token)
	{
		this.token = token == null ? "" : token.trim();
	}

	@Override
	public void apply(Request.Builder request)
	{
		request.header("Authorization", "Bearer " + token);
	}

	@Override
	public boolean ready()
	{
		return !token.isEmpty();
	}

	@Override
	public String fingerprint()
	{
		try
		{
			byte[] hash = MessageDigest.getInstance("SHA-256").digest(token.getBytes(StandardCharsets.UTF_8));
			StringBuilder sb = new StringBuilder("token\n");
			for (byte b : hash)
			{
				sb.append(String.format("%02x", b));
			}
			return sb.toString();
		}
		catch (NoSuchAlgorithmException ex)
		{
			throw new IllegalStateException(ex);
		}
	}

	@Override
	public String toString()
	{
		return "TokenCredential";
	}
}
