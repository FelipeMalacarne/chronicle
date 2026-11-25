package net.runelite.client.plugins.microbot.fm.chroniclelink;

import static org.junit.jupiter.api.Assertions.assertEquals;

import javax.swing.JLabel;
import org.junit.jupiter.api.Test;

class ClientMessagesTest
{
	@Test
	void theTextOfAMessage()
	{
		assertEquals("Your bot tried to interact with an NPC for 3 times but failed.",
			ClientMessages.text(" Your bot tried to interact with an NPC for 3 times but failed. "));
		assertEquals("first second", ClientMessages.text(new Object[]{"first", null, new Object[]{"second"}}));
		assertEquals("", ClientMessages.text(null));
	}

	@Test
	void aComponentHasNoText()
	{
		assertEquals("", ClientMessages.text(new JLabel("custom")));
		assertEquals("kept", ClientMessages.text(new Object[]{new JLabel("custom"), "kept"}));
	}
}
