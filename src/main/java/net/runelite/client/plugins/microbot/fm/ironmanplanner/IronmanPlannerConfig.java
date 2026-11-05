package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import net.runelite.client.config.Config;
import net.runelite.client.config.ConfigGroup;
import net.runelite.client.config.ConfigItem;

@ConfigGroup(IronmanPlannerConfig.GROUP)
public interface IronmanPlannerConfig extends Config
{
	String GROUP = "fm-ironmanplanner";

	/** How requests authenticate (ironman-planner docs/04-arquitetura.md 2.5). */
	enum CredentialMode
	{
		/** AUTH_MODE=dev: only the URL; the planner accepts loopback requests without a token. */
		NONE("None (dev, URL only)"),
		/** A per-account ipl_ token. */
		TOKEN("ipl_ token");

		private final String label;

		CredentialMode(String label)
		{
			this.label = label;
		}

		@Override
		public String toString()
		{
			return label;
		}
	}

	@ConfigItem(
		keyName = "serverUrl",
		name = "Server URL",
		description = "Base URL of the Ironman Planner server, e.g. http://127.0.0.1:8080. Nothing is sent while empty",
		position = 0
	)
	default String serverUrl()
	{
		return "";
	}

	@ConfigItem(
		keyName = "credentialMode",
		name = "Credential",
		description = "None: a planner in dev mode on this machine accepts the plugin with only the URL. Token: send the ipl_ token below",
		position = 1
	)
	default CredentialMode credentialMode()
	{
		return CredentialMode.NONE;
	}

	@ConfigItem(
		keyName = "ingestToken",
		name = "Ingest token",
		description = "Per-account ipl_ token, used when Credential is Token. Panel marks need its marks:write scope",
		position = 2,
		secret = true
	)
	default String ingestToken()
	{
		return "";
	}

	@ConfigItem(
		keyName = "account",
		name = "Conta",
		description = "Only without a token and with several planner accounts: the account id (acc_...). Empty: the logged-in character's account, or the only one",
		position = 3
	)
	default String account()
	{
		return "";
	}

	@ConfigItem(
		keyName = InstallationId.CONFIG_KEY,
		name = "Installation id",
		description = "Generated once; identifies this installation to the planner",
		hidden = true
	)
	default String installationId()
	{
		return "";
	}

	@ConfigItem(
		keyName = "sendBank",
		name = "Send bank",
		description = "Share the bank mirror. With a token, it also needs its items:bank scope",
		position = 4
	)
	default boolean sendBank()
	{
		return false;
	}

	@ConfigItem(
		keyName = "sendInventory",
		name = "Send inventory",
		description = "Share the inventory. With a token, it also needs its items:inventory scope",
		position = 5
	)
	default boolean sendInventory()
	{
		return false;
	}

	@ConfigItem(
		keyName = "sendEquipment",
		name = "Send equipment",
		description = "Share worn equipment. With a token, it also needs its items:equipment scope",
		position = 6
	)
	default boolean sendEquipment()
	{
		return false;
	}

	@ConfigItem(
		keyName = "showStatus",
		name = "Show status",
		description = "Show the sync status indicator overlay",
		position = 7
	)
	default boolean showStatus()
	{
		return true;
	}
}
