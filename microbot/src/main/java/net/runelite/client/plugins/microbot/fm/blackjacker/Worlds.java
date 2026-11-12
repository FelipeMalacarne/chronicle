package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.List;
import java.util.stream.Collectors;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.globval.enums.InterfaceTab;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.math.Rs2Random;
import net.runelite.client.plugins.microbot.util.tabs.Rs2Tab;
import net.runelite.client.plugins.microbot.util.world.Rs2WorldUtil;
import net.runelite.http.api.worlds.World;
import net.runelite.http.api.worlds.WorldResult;
import net.runelite.http.api.worlds.WorldType;

/** World hops, for wine stock and for a room someone else can see into. */
final class Worlds
{
	private Worlds()
	{
	}

	/**
	 * A random members world other than {@code current} that the account can use (no PvP,
	 * high-risk, skill-total, Deadman, speedrun or beta worlds); -1 when the list is not loaded.
	 * Pollnivneach is members-only, so a free world would refuse the hop anyway.
	 */
	static int otherMembersWorld(int current)
	{
		WorldResult result = Microbot.getWorldService().getWorlds();
		if (result == null)
		{
			return -1;
		}
		boolean seasonal = Microbot.getClient().getWorldType().contains(net.runelite.api.WorldType.SEASONAL);
		List<World> worlds = result.getWorlds().stream()
			.filter(w -> w.getId() != current && w.getTypes().contains(WorldType.MEMBERS))
			.filter(w -> Rs2WorldUtil.isWorldAccessible(w, true, seasonal))
			.collect(Collectors.toList());
		return worlds.isEmpty() ? -1 : worlds.get(Rs2Random.between(0, worlds.size() - 1)).getId();
	}

	/** Hops to {@code world} and waits until logged in there. */
	static void hop(int world)
	{
		if (Microbot.hopToWorld(world))
		{
			Global.sleepUntil(() -> Microbot.getClient().getWorld() == world && Microbot.isLoggedIn(), 15000);
			// The hop leaves the world list open; the next drink should not wait on a tab switch.
			Rs2Tab.switchTo(InterfaceTab.INVENTORY);
		}
		else
		{
			Global.sleep(1500, 2500);
		}
	}
}
