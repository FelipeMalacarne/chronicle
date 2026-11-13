package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpContext;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpPrincipal;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** The executor behind the bridge: only the agent's start runs a task, one at a time. */
class TaskBridgeTest
{
	private static final String TOKEN = "agent-token";

	@BeforeEach
	void token()
	{
		AgentHandler.setTokenSupplier(() -> TOKEN);
	}

	@AfterEach
	void noToken()
	{
		AgentHandler.setTokenSupplier(null);
	}

	/** A game whose walk blocks until released, so a task stays running. */
	static final class SlowGame extends TaskMachineTest.FakeGame
	{
		final CountDownLatch release = new CountDownLatch(1);
		// Counted down once the machine is walking, so a test cancels a task that has already
		// written its first events, not one that is still starting.
		final CountDownLatch walking = new CountDownLatch(1);

		SlowGame()
		{
			walks.add(new WorldPoint(3217, 3219, 0));
			onWalk = () -> {
				walking.countDown();
				try
				{
					release.await(5, TimeUnit.SECONDS);
				}
				catch (InterruptedException e)
				{
					Thread.currentThread().interrupt();
				}
			};
		}

		@Override
		public void stopWalking()
		{
			super.stopWalking();
			release.countDown();
		}
	}

	private final List<TaskMachine> ended = new CopyOnWriteArrayList<>();

	private TaskExecutor executor(TaskGame game)
	{
		return new TaskExecutor(game, Clock.systemUTC(), ended::add);
	}

	@Test
	void startReadCancelThroughTheBridge() throws Exception
	{
		SlowGame game = new SlowGame();
		TaskExecutor tasks = executor(game);
		AgentBridge bridge = new AgentBridge(new Gson(), () -> TOKEN, tasks);
		try
		{
			Exchange idle = call(bridge, "/bridge/task", "GET", null);
			assertEquals(200, idle.status);
			assertEquals("idle", json(idle).get("state").getAsString());

			JsonObject body = TaskSpecTest.start(TaskSpec.TRAVEL);
			Exchange started = call(bridge, "/bridge/task/start", "POST", body.toString());
			assertEquals(200, started.status, started.body());

			// One task at a time.
			JsonObject other = body.deepCopy();
			other.addProperty("id", "0123456789abcdef");
			assertEquals(409, call(bridge, "/bridge/task/start", "POST", other.toString()).status);

			JsonObject running = json(call(bridge, "/bridge/task", "GET", null));
			assertEquals("5f2a9c0d11e84b7a", running.get("id").getAsString());
			assertEquals("running", running.get("state").getAsString());

			assertTrue(game.walking.await(5, TimeUnit.SECONDS), "the machine never started walking");
			// A cancel for another id changes nothing; the right one stops the walk.
			assertFalse(json(call(bridge, "/bridge/task/cancel", "POST", "{\"id\":\"0123456789abcdef\"}")).get("success").getAsBoolean());
			assertTrue(json(call(bridge, "/bridge/task/cancel", "POST", "{\"id\":\"5f2a9c0d11e84b7a\"}")).get("success").getAsBoolean());
			tasks.awaitIdle(5_000);
			JsonObject canceled = json(call(bridge, "/bridge/task", "GET", null));
			assertEquals("canceled", canceled.get("state").getAsString());
			assertTrue(canceled.getAsJsonArray("events").size() >= 2);
			waitEnded(1);
			assertEquals("5f2a9c0d11e84b7a", ended.get(0).id);
		}
		finally
		{
			tasks.stop();
		}
	}

	@Test
	void theBridgeRefusesWhatIsNotATask()
	{
		TaskExecutor tasks = executor(new TaskMachineTest.FakeGame());
		AgentBridge bridge = new AgentBridge(new Gson(), () -> TOKEN, tasks);
		try
		{
			JsonObject talk = new JsonObject();
			talk.addProperty("id", "5f2a9c0d11e84b7a");
			talk.add("task", new JsonParser().parse("{\"type\":\"talk\"}"));
			Exchange refused = call(bridge, "/bridge/task/start", "POST", talk.toString());
			assertEquals(400, refused.status);
			assertTrue(refused.body().contains("not executed yet"), refused.body());

			JsonObject badID = TaskSpecTest.start(TaskSpec.BANK).deepCopy();
			badID.addProperty("id", "../../etc");
			assertEquals(400, call(bridge, "/bridge/task/start", "POST", badID.toString()).status);

			assertEquals(400, call(bridge, "/bridge/task/start", "GET", null).status, "start is a POST");
			assertEquals("idle", json(call(bridge, "/bridge/task", "GET", null)).get("state").getAsString());

			// Without the Agent Server's token, nothing answers.
			Exchange noToken = new Exchange("/bridge/task/start", "POST", TaskSpecTest.start(TaskSpec.BANK).toString(), "other");
			handle(bridge, noToken);
			assertEquals(404, noToken.status);
		}
		finally
		{
			tasks.stop();
		}
	}

	@Test
	void withoutTheExecutorTheRoutesSayItIsOff()
	{
		AgentBridge bridge = new AgentBridge(new Gson(), () -> TOKEN);
		Exchange start = call(bridge, "/bridge/task/start", "POST", TaskSpecTest.start(TaskSpec.BUY).toString());
		assertEquals(404, start.status);
		assertTrue(start.body().contains("off"));
		assertEquals(404, call(bridge, "/bridge/task", "GET", null).status);
	}

	@Test
	void theExecutorEndsATaskAndTellsThePlugin() throws Exception
	{
		TaskMachineTest.FakeGame game = new TaskMachineTest.FakeGame();
		game.walks.add(new WorldPoint(3217, 3219, 0));
		TaskExecutor tasks = executor(game);
		try
		{
			tasks.start("5f2a9c0d11e84b7a", TaskSpecTest.spec(TaskSpec.TRAVEL));
			waitEnded(1);
			assertEquals(TaskMachine.State.DONE, ended.get(0).state());
			assertEquals("done", tasks.view().get("state"));
			// The next task may start once the last one ended.
			game.pos = new WorldPoint(3103, 3265, 0);
			game.walked = 0;
			tasks.start("5f2a9c0d11e84b7b", TaskSpecTest.spec(TaskSpec.TRAVEL));
			waitEnded(2);
		}
		finally
		{
			tasks.stop();
		}
		assertThrows(IllegalStateException.class, () -> tasks.start("5f2a9c0d11e84b7c", TaskSpecTest.spec(TaskSpec.TRAVEL)));
		assertThrows(IllegalArgumentException.class, () -> executor(game).start("nope", TaskSpecTest.spec(TaskSpec.TRAVEL)));
	}

	private void waitEnded(int n) throws InterruptedException
	{
		long deadline = System.currentTimeMillis() + 5_000;
		while (ended.size() < n && System.currentTimeMillis() < deadline)
		{
			Thread.sleep(5);
		}
		assertEquals(n, ended.size());
	}

	private static JsonObject json(Exchange ex)
	{
		return new JsonParser().parse(ex.body()).getAsJsonObject();
	}

	private static Exchange call(AgentBridge bridge, String path, String method, String body)
	{
		Exchange ex = new Exchange(path, method, body, TOKEN);
		handle(bridge, ex);
		return ex;
	}

	private static void handle(AgentBridge bridge, Exchange ex)
	{
		try
		{
			bridge.handler(ex.getRequestURI().getPath()).handle(ex);
		}
		catch (IOException e)
		{
			throw new java.io.UncheckedIOException(e);
		}
	}

	/** One request to a bridge route, with its method and body. */
	static final class Exchange extends HttpExchange
	{
		private final URI uri;
		private final String method;
		private final byte[] in;
		private final Headers request = new Headers();
		private final Headers response = new Headers();
		private final ByteArrayOutputStream out = new ByteArrayOutputStream();
		int status = -1;

		Exchange(String path, String method, String body, String token)
		{
			this.uri = URI.create(path);
			this.method = method;
			this.in = body == null ? new byte[0] : body.getBytes(StandardCharsets.UTF_8);
			request.add("Host", "localhost");
			request.add("X-Agent-Token", token);
		}

		String body()
		{
			return out.toString(StandardCharsets.UTF_8);
		}

		@Override
		public Headers getRequestHeaders()
		{
			return request;
		}

		@Override
		public Headers getResponseHeaders()
		{
			return response;
		}

		@Override
		public URI getRequestURI()
		{
			return uri;
		}

		@Override
		public String getRequestMethod()
		{
			return method;
		}

		@Override
		public HttpContext getHttpContext()
		{
			return null;
		}

		@Override
		public void close()
		{
		}

		@Override
		public InputStream getRequestBody()
		{
			return new ByteArrayInputStream(in);
		}

		@Override
		public OutputStream getResponseBody()
		{
			return out;
		}

		@Override
		public void sendResponseHeaders(int rCode, long responseLength)
		{
			status = rCode;
		}

		@Override
		public InetSocketAddress getRemoteAddress()
		{
			return null;
		}

		@Override
		public int getResponseCode()
		{
			return status;
		}

		@Override
		public InetSocketAddress getLocalAddress()
		{
			return null;
		}

		@Override
		public String getProtocol()
		{
			return "HTTP/1.1";
		}

		@Override
		public Object getAttribute(String name)
		{
			return null;
		}

		@Override
		public void setAttribute(String name, Object value)
		{
		}

		@Override
		public void setStreams(InputStream i, OutputStream o)
		{
		}

		@Override
		public HttpPrincipal getPrincipal()
		{
			return null;
		}
	}
}
