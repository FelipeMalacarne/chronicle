package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.util.UUID;
import java.util.function.Consumer;
import java.util.function.Supplier;
import java.util.regex.Pattern;

/**
 * The stable id of this plugin installation: a random UUID generated once and kept in the
 * plugin config. Without a token, the planner keys this installation's protocol state by it.
 * It identifies the installation, not the player, and carries no secret.
 */
final class InstallationId
{
	static final String CONFIG_KEY = "installationId";
	private static final Pattern UUID_FORMAT =
		Pattern.compile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$");

	private InstallationId()
	{
	}

	static boolean isValid(String id)
	{
		return id != null && UUID_FORMAT.matcher(id).matches();
	}

	/**
	 * Returns the stored id when it is a valid UUID; otherwise generates one, saves it and
	 * returns it. Saving happens at most once per missing or broken value.
	 */
	static String ensure(String stored, Consumer<String> save, Supplier<UUID> generator)
	{
		String trimmed = stored == null ? "" : stored.trim();
		if (isValid(trimmed))
		{
			return trimmed;
		}
		String id = generator.get().toString().toLowerCase();
		save.accept(id);
		return id;
	}
}
