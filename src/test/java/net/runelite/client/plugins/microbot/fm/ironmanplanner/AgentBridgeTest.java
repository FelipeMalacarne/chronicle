package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.util.List;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import net.runelite.client.plugins.microbot.util.prayer.Rs2PrayerEnum;
import org.junit.jupiter.api.Test;

class AgentBridgeTest
{
	@Test
	void actionsKeepTheGameOrderWithoutEmptySlots()
	{
		assertEquals(List.of("Talk-to", "Bank", "Collect"), AgentBridge.actions(new String[]{"Talk-to", null, "Bank", "", "Collect"}));
		assertEquals(List.of(), AgentBridge.actions(null));
	}

	@Test
	void equipmentSlotsByIndex()
	{
		assertEquals("WEAPON", AgentBridge.slotName(3));
		assertEquals("AMULET", AgentBridge.slotName(2));
		assertEquals("99", AgentBridge.slotName(99));
	}

	@Test
	void limitsAndKeys()
	{
		assertEquals(30, AgentBridge.clamp(500, 1, 30));
		assertEquals(1, AgentBridge.clamp(-4, 1, 30));
		assertEquals(Rs2PrayerEnum.PROTECT_ITEM, AgentBridge.enumOf(Rs2PrayerEnum.class, "PROTECT_ITEM"));
		assertThrows(AgentHandler.BadRequestException.class, () -> AgentBridge.enumOf(Rs2PrayerEnum.class, "SMITE_EVERYONE"));
	}
}
