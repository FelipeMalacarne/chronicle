package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.util.List;
import net.runelite.client.plugins.microbot.agentserver.handler.AgentHandler;
import net.runelite.client.plugins.microbot.util.magic.Rs2CombatSpells;
import net.runelite.client.plugins.microbot.util.magic.Rs2Spells;
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

	@Test
	void spellsKnowWhatTheyAreCastOn()
	{
		assertEquals("item", AgentBridge.targetOf("HIGH_LEVEL_ALCHEMY"));
		assertEquals("item", AgentBridge.targetOf("SUPERHEAT_ITEM"));
		assertEquals("npc", AgentBridge.targetOf("WIND_STRIKE"));
		assertEquals("npc", AgentBridge.targetOf("CONFUSE"));
		assertEquals("none", AgentBridge.targetOf("VARROCK_TELEPORT"));
		// Enchanting has no way to be cast on an item in the client yet: never a plain cast.
		assertEquals("unsupported", AgentBridge.targetOf("ENCHANT_SAPPHIRE_JEWELLERY"));
		assertEquals("unsupported", AgentBridge.targetOf("TELEKINETIC_GRAB"));
		assertEquals("none", AgentBridge.targetOf("BONES_TO_BANANAS"));
		assertEquals(Rs2CombatSpells.FIRE_BOLT, AgentBridge.spellOf("FIRE_BOLT"));
		assertEquals(Rs2Spells.HIGH_LEVEL_ALCHEMY, AgentBridge.spellOf("HIGH_LEVEL_ALCHEMY"));
		assertThrows(AgentHandler.BadRequestException.class, () -> AgentBridge.spellOf("SUMMON_DRAGON"));
	}
}
