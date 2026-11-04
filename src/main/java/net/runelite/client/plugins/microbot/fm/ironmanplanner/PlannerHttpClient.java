package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import com.google.gson.Gson;
import com.google.gson.JsonParseException;
import java.io.IOException;
import java.util.concurrent.TimeUnit;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.ResponseBody;

/**
 * Blocking HTTP calls to the planner, run only on the plugin's own executor. The token goes in
 * the Authorization header and is never logged or put in a URL.
 */
final class PlannerHttpClient
{
	static final long TIMEOUT_SECONDS = 3;
	private static final MediaType JSON = MediaType.parse("application/json; charset=utf-8");
	private static final long MAX_BODY_BYTES = 64 * 1024;

	/** Outcome of one call: {@code status == 0} means timeout or I/O error. */
	static final class Result<T>
	{
		final int status;
		final T body;

		Result(int status, T body)
		{
			this.status = status;
			this.body = body;
		}
	}

	private final OkHttpClient http;
	private final Gson gson;

	PlannerHttpClient(OkHttpClient base, Gson gson)
	{
		this.http = base.newBuilder()
			.callTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.connectTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.readTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.writeTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
			.followRedirects(false)
			.build();
		this.gson = gson;
	}

	/** The configured server as a base URL, or null unless it is an http(s) URL. */
	static HttpUrl parseServer(String serverUrl)
	{
		if (serverUrl == null)
		{
			return null;
		}
		HttpUrl url = HttpUrl.parse(serverUrl.trim());
		if (url == null || url.query() != null || !url.username().isEmpty() || !url.password().isEmpty())
		{
			return null;
		}
		return url;
	}

	/** {@code base} plus {@code api/ingest/...}, keeping any path prefix (a reverse-proxy mount). */
	static HttpUrl endpoint(HttpUrl base, String... segments)
	{
		HttpUrl.Builder b = base.newBuilder();
		// drop the empty last segment of a trailing slash
		int last = base.pathSize() - 1;
		if (last >= 0 && base.pathSegments().get(last).isEmpty())
		{
			b.removePathSegment(last);
		}
		for (String s : segments)
		{
			b.addPathSegment(s);
		}
		return b.build();
	}

	Result<IngestContract.Manifest> fetchManifest(HttpUrl base, String token)
	{
		Request request = new Request.Builder()
			.url(endpoint(base, "api", "ingest", "manifest"))
			.header("Authorization", "Bearer " + token)
			.header("Accept", "application/json")
			.get()
			.build();
		return execute(request, IngestContract.Manifest.class);
	}

	Result<IngestContract.Response> push(HttpUrl base, String token, IngestContract.Push push)
	{
		Request request = new Request.Builder()
			.url(endpoint(base, "api", "ingest"))
			.header("Authorization", "Bearer " + token)
			.header("Accept", "application/json")
			.post(RequestBody.create(JSON, gson.toJson(push)))
			.build();
		return execute(request, IngestContract.Response.class);
	}

	private <T> Result<T> execute(Request request, Class<T> type)
	{
		try (Response response = http.newCall(request).execute())
		{
			ResponseBody body = response.body();
			T parsed = null;
			if (body != null && isJson(body))
			{
				try
				{
					parsed = gson.fromJson(response.peekBody(MAX_BODY_BYTES).string(), type);
				}
				catch (JsonParseException ex)
				{
					parsed = null;
				}
			}
			return new Result<>(response.code(), parsed);
		}
		catch (IOException ex)
		{
			return new Result<>(0, null);
		}
	}

	private static boolean isJson(ResponseBody body)
	{
		MediaType type = body.contentType();
		return type != null && "json".equals(type.subtype());
	}
}
