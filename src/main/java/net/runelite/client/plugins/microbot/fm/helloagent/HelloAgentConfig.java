package net.runelite.client.plugins.microbot.fm.helloagent;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;

@ConfigGroup(HelloAgentConfig.GROUP)
public interface HelloAgentConfig extends Config
{
	String GROUP = "fm-helloagent";

	@ConfigItem(
		keyName = "npcRadius",
		name = "NPC radius",
		description = "Count NPCs within this many tiles of the player",
		position = 0
	)
	default int npcRadius()
	{
		return 10;
	}

	@ConfigItem(
		keyName = "testLoops",
		name = "Test loops",
		description = "In test mode, loops to observe before writing the result and exiting",
		position = 1
	)
	default int testLoops()
	{
		return 5;
	}
}
