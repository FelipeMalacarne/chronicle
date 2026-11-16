package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.Map;
import java.util.TreeMap;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import net.runelite.api.Client;
import net.runelite.api.gameval.InterfaceID;
import net.runelite.api.gameval.VarbitID;
import net.runelite.api.widgets.Widget;

/**
 * Counters a minigame shows only in its interface, for {@code data.minigame} (planner
 * 2026-11-16): the Mage Training Arena's pizazz points, from the arena's panel in the lobby or a
 * room's own counter inside it, and whether Bones to Peaches is unlocked (a varbit, readable
 * anywhere). The value is the game's: the Alchemists' Playground coins not handed in yet are not
 * points. A counter is kept from the last time it was on screen in this login, and never sent
 * before: the planner keeps the last value it had. Client thread only.
 */
final class MinigameCounters
{
	private static final Pattern TAGS = Pattern.compile("<[^>]*>");
	private static final Pattern NUMBER = Pattern.compile("\\d[\\d,.]*");
	/** "mta.<room>", the lobby panel's widget, and the room's counter (group, child) inside it. */
	private static final Object[][] MTA_ROOMS = {
		{"mta.telekinetic", InterfaceID.MagictrainingMain.MAGICTRAIN_TELE_POINTS, 198},
		{"mta.alchemist", InterfaceID.MagictrainingMain.MAGICTRAIN_ALCH_POINTS, 194},
		{"mta.enchantment", InterfaceID.MagictrainingMain.MAGICTRAIN_ENCH_POINTS, 195},
		{"mta.graveyard", InterfaceID.MagictrainingMain.MAGICTRAIN_GRAVE_POINTS, 196},
	};
	private static final int MTA_ROOM_POINTS_CHILD = 6;

	private final Map<String, Long> last = new TreeMap<>();

	void reset()
	{
		last.clear();
	}

	/** Reads what the client shows now. */
	void read(Client client)
	{
		for (Object[] room : MTA_ROOMS)
		{
			int n = number(client.getWidget((int) room[1]));
			if (n < 0)
			{
				n = number(client.getWidget((int) room[2], MTA_ROOM_POINTS_CHILD));
			}
			if (n >= 0)
			{
				last.put((String) room[0], (long) n);
			}
		}
		last.put("mta.bones_to_peaches", client.getVarbitValue(VarbitID.MAGICTRAINING_BONESPEACHES) > 0 ? 1L : 0L);
	}

	/** The counters read in this login, or {@code null} (not sent) unless the manifest takes them. */
	Map<String, Long> forIngest(IngestContract.Manifest manifest)
	{
		return manifest.minigameEnabled() ? new TreeMap<>(last) : null;
	}

	private static int number(Widget w)
	{
		return w == null || w.isHidden() ? -1 : lastNumber(w.getText());
	}

	/** The last number in a widget's text ("Telekinetic: <col=...>1,342</col>" -> 1342), or -1. */
	static int lastNumber(String text)
	{
		if (text == null)
		{
			return -1;
		}
		Matcher m = NUMBER.matcher(TAGS.matcher(text).replaceAll(" "));
		String found = null;
		while (m.find())
		{
			found = m.group();
		}
		if (found == null)
		{
			return -1;
		}
		try
		{
			long v = Long.parseLong(found.replace(",", "").replace(".", ""));
			return v > Integer.MAX_VALUE ? -1 : (int) v;
		}
		catch (NumberFormatException e)
		{
			return -1;
		}
	}
}
