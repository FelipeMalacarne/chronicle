package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;

@ConfigGroup(IronmanPlannerConfig.GROUP)
public interface IronmanPlannerConfig extends Config
{
	String GROUP = "fm-ironmanplanner";

	@ConfigItem(
		keyName = "serverUrl",
		name = "Server URL",
		description = "Base URL of the Ironman Planner server, e.g. https://planner.example. Nothing is sent while empty",
		position = 0
	)
	default String serverUrl()
	{
		return "";
	}

	@ConfigItem(
		keyName = "ingestToken",
		name = "Ingest token",
		description = "Per-account ipl_ token from the planner's /account/tokens page",
		position = 1,
		secret = true
	)
	default String ingestToken()
	{
		return "";
	}

	@ConfigItem(
		keyName = "sendBank",
		name = "Send bank",
		description = "Share the bank mirror. Also needs the token's items:bank scope",
		position = 2
	)
	default boolean sendBank()
	{
		return false;
	}

	@ConfigItem(
		keyName = "sendInventory",
		name = "Send inventory",
		description = "Share the inventory. Also needs the token's items:inventory scope",
		position = 3
	)
	default boolean sendInventory()
	{
		return false;
	}

	@ConfigItem(
		keyName = "sendEquipment",
		name = "Send equipment",
		description = "Share worn equipment. Also needs the token's items:equipment scope",
		position = 4
	)
	default boolean sendEquipment()
	{
		return false;
	}

	@ConfigItem(
		keyName = "showStatus",
		name = "Show status",
		description = "Show the sync status indicator overlay",
		position = 5
	)
	default boolean showStatus()
	{
		return true;
	}
}
