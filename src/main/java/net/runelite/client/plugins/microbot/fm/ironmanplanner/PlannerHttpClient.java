package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import com.google.gson.Gson;
import com.google.gson.JsonParseException;
import java.io.IOException;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.TimeUnit;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.ResponseBody;

/**
 * Blocking HTTP calls to the planner, run only on the plugin's own executors (never the client
 * thread or the Swing thread). The credential decides the headers; a token goes in the
 * Authorization header and is never logged or put in a URL.
 */
final class PlannerHttpClient implements PanelApi
{
	static final long TIMEOUT_SECONDS = 3;
	private static final MediaType JSON = MediaType.parse("application/json; charset=utf-8");
	private static final long MAX_BODY_BYTES = 64 * 1024;
	/** A route window carries full step texts (BRUHsailer steps are long paragraphs). */
	private static final long MAX_WINDOW_BYTES = 1024 * 1024;

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

	Result<IngestContract.Manifest> fetchManifest(HttpUrl base, PlannerCredential credential)
	{
		Request.Builder request = new Request.Builder()
			.url(endpoint(base, "api", "ingest", "manifest"))
			.header("Accept", "application/json")
			.get();
		credential.apply(request);
		return execute(request.build(), IngestContract.Manifest.class, MAX_BODY_BYTES);
	}

	Result<IngestContract.Response> push(HttpUrl base, PlannerCredential credential, IngestContract.Push push)
	{
		Request.Builder request = new Request.Builder()
			.url(endpoint(base, "api", "ingest"))
			.header("Accept", "application/json")
			.post(RequestBody.create(JSON, gson.toJson(push)));
		credential.apply(request);
		return execute(request.build(), IngestContract.Response.class, MAX_BODY_BYTES);
	}

	@Override
	public Result<RouteWindow> fetchRoute(HttpUrl base, PlannerCredential credential, int limit)
	{
		HttpUrl url = endpoint(base, "api", "plugin", "route").newBuilder()
			.addQueryParameter("limit", Integer.toString(limit))
			.build();
		Request.Builder request = new Request.Builder().url(url).header("Accept", "application/json").get();
		credential.apply(request);
		return execute(request.build(), RouteWindow.class, MAX_WINDOW_BYTES);
	}

	/** Body of PATCH .../mark: the web's mark action. */
	static final class MarkRequest
	{
		final String kind;

		MarkRequest(String kind)
		{
			this.kind = kind;
		}
	}

	/** Body of POST .../marks: the web's batch undo, one step. */
	static final class RestoreRequest
	{
		static final class Entry
		{
			@com.google.gson.annotations.SerializedName("step_id")
			final String stepId;
			// serialized as null when the step had no mark
			final RouteWindow.Mark mark;

			Entry(String stepId, RouteWindow.Mark mark)
			{
				this.stepId = stepId;
				this.mark = mark;
			}
		}

		final List<Entry> restore;

		RestoreRequest(String stepId, RouteWindow.Mark mark)
		{
			this.restore = Collections.singletonList(new Entry(stepId, mark));
		}
	}

	@Override
	public Result<RouteWindow> mark(HttpUrl base, PlannerCredential credential, String guide, String stepId, String kind, int limit)
	{
		HttpUrl url = endpoint(base, "api", "plugin", "guides", guide, "steps", stepId, "mark").newBuilder()
			.addQueryParameter("limit", Integer.toString(limit))
			.build();
		Request.Builder request = new Request.Builder()
			.url(url)
			.header("Accept", "application/json")
			.patch(RequestBody.create(JSON, gson.toJson(new MarkRequest(kind))));
		credential.apply(request);
		return execute(request.build(), RouteWindow.class, MAX_WINDOW_BYTES);
	}

	@Override
	public Result<RouteWindow> restore(HttpUrl base, PlannerCredential credential, String guide, String stepId, RouteWindow.Mark mark, int limit)
	{
		HttpUrl url = endpoint(base, "api", "plugin", "guides", guide, "marks").newBuilder()
			.addQueryParameter("limit", Integer.toString(limit))
			.build();
		Request.Builder request = new Request.Builder()
			.url(url)
			.header("Accept", "application/json")
			.post(RequestBody.create(JSON, gson.toJson(new RestoreRequest(stepId, mark))));
		credential.apply(request);
		return execute(request.build(), RouteWindow.class, MAX_WINDOW_BYTES);
	}

	private <T> Result<T> execute(Request request, Class<T> type, long maxBytes)
	{
		try (Response response = http.newCall(request).execute())
		{
			ResponseBody body = response.body();
			T parsed = null;
			if (body != null && isJson(body))
			{
				try
				{
					parsed = gson.fromJson(response.peekBody(maxBytes).string(), type);
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
