package net.runelite.client.plugins.microbot.fm.helloagent;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import javax.inject.Inject;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.coords.WorldPoint;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.Script;
import net.runelite.client.plugins.microbot.agentserver.handler.ScriptResultStore;
import net.runelite.client.plugins.microbot.testing.TestResult;
import net.runelite.client.plugins.microbot.testing.TestResultWriter;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;

@Slf4j
public class HelloAgentScript extends Script
{
	private final HelloAgentConfig config;
	private int loops;

	@Inject
	HelloAgentScript(HelloAgentConfig config)
	{
		this.config = config;
	}

	public boolean run()
	{
		loops = 0;
		mainScheduledFuture = scheduledExecutorService.scheduleWithFixedDelay(() -> {
			try
			{
				if (!Microbot.isLoggedIn()) return;
				if (!super.run()) return;

				WorldPoint position = Rs2Player.getWorldLocation();
				int npcs = Microbot.getRs2NpcCache().query().within(config.npcRadius()).count();
				loops++;
				Microbot.status = "Hello Agent: " + npcs + " NPCs near " + position;

				Map<String, Object> observation = new LinkedHashMap<>();
				observation.put("loop", loops);
				observation.put("position", position == null ? null : position.getX() + "," + position.getY() + "," + position.getPlane());
				observation.put("npcsNearby", npcs);
				ScriptResultStore.submit(HelloAgentPlugin.class.getName(), observation);

				if (isTestMode() && loops >= config.testLoops())
				{
					finishTest(position, npcs);
				}
			}
			catch (Exception ex)
			{
				log.error("Hello Agent loop failed", ex);
			}
		}, 0, 1000, TimeUnit.MILLISECONDS);
		return true;
	}

	// Test mode protocol (docs/AGENTIC_TESTING_LOOP.md): the target plugin records its own
	// checks, writes result.json and exits; the runner only handles login and timeouts.
	private void finishTest(WorldPoint position, int npcs)
	{
		TestResult result = new TestResult("Hello Agent");
		result.addCheck("logged in", Microbot.isLoggedIn(), null);
		result.addCheck("player position known", position != null, "Rs2Player.getWorldLocation() returned null");
		result.addCheck("npc query ran", npcs >= 0, "negative NPC count");
		result.complete("completed");
		TestResultWriter.write(result);
		log.info("Hello Agent test finished: {}/{} checks passed", result.passedCount(), result.checks.size());
		System.exit(result.exitCode);
	}

	private static boolean isTestMode()
	{
		return Boolean.getBoolean("microbot.test.mode");
	}
}
