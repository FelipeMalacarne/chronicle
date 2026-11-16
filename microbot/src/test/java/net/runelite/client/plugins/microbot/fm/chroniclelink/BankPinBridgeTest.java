package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.google.gson.Gson;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** /bridge/bank-pin: a managed worker's PIN into the client's active profile (Chronicle's docs/11). */
class BankPinBridgeTest
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

	@Test
	void writesAFourDigitPinAndAnswersWithoutIt() throws Exception
	{
		List<String> written = new CopyOnWriteArrayList<>();
		AgentBridge bridge = new AgentBridge(new Gson(), () -> TOKEN).withBankPinWriter(written::add);
		TaskBridgeTest.Exchange ok = TaskBridgeTest.call(bridge, "/bridge/bank-pin", "POST", "{\"pin\":\"0420\"}");
		assertEquals(200, ok.status, ok.body());
		assertEquals(List.of("0420"), written);
		assertFalse(ok.body().contains("0420"), ok.body());
	}

	@Test
	void refusesAnythingButFourDigits() throws Exception
	{
		List<String> written = new CopyOnWriteArrayList<>();
		AgentBridge bridge = new AgentBridge(new Gson(), () -> TOKEN).withBankPinWriter(written::add);
		for (String body : new String[]{"{\"pin\":\"42\"}", "{\"pin\":\"04a0\"}", "{\"pin\":\"12345\"}", "{}"})
		{
			TaskBridgeTest.Exchange bad = TaskBridgeTest.call(bridge, "/bridge/bank-pin", "POST", body);
			assertEquals(400, bad.status, body + " -> " + bad.body());
		}
		// Only a POST writes it (the bridge's routes answer a wrong method with a 400).
		assertEquals(400, TaskBridgeTest.call(bridge, "/bridge/bank-pin", "GET", null).status);
		assertTrue(written.isEmpty());
	}
}
