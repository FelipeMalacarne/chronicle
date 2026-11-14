package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** Reads the numbers the Magic Training Arena interfaces show as text. Pure. */
final class MtaText
{
	private static final Pattern TAGS = Pattern.compile("<[^>]*>");
	private static final Pattern NUMBER = Pattern.compile("\\d[\\d,.]*");

	private MtaText()
	{
	}

	/**
	 * The last number in a widget's text, e.g. "Telekinetic: <col=ffffff>1,234</col>" gives 1234;
	 * -1 when there is none (an empty or non-numeric label must never read as zero points).
	 */
	static int lastNumber(String text)
	{
		if (text == null)
		{
			return -1;
		}
		Matcher m = NUMBER.matcher(TAGS.matcher(text).replaceAll(" "));
		String last = null;
		while (m.find())
		{
			last = m.group();
		}
		if (last == null)
		{
			return -1;
		}
		try
		{
			long v = Long.parseLong(last.replace(",", "").replace(".", ""));
			return v > Integer.MAX_VALUE ? -1 : (int) v;
		}
		catch (NumberFormatException e)
		{
			return -1;
		}
	}
}
