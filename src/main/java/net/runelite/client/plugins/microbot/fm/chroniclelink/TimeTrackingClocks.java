package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;
import net.runelite.client.plugins.Plugin;
import net.runelite.client.plugins.PluginManager;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.timetracking.SummaryState;
import net.runelite.client.plugins.timetracking.Tab;
import net.runelite.client.plugins.timetracking.TimeTrackingPlugin;
import net.runelite.client.plugins.timetracking.farming.FarmingContractManager;
import net.runelite.client.plugins.timetracking.farming.FarmingTracker;
import net.runelite.client.plugins.timetracking.hunter.BirdHouseTracker;

/**
 * The Time Tracking panel's clocks, read from the plugin's own trackers (singletons of its
 * injector), so they are the numbers the client shows. Farming patches are only seen near them;
 * the tracker predicts from the last visit. The control bridge serves them (/bridge/timers) and
 * the ingest sends them ({@code data.timers}, the planner's recurring tasks).
 */
final class TimeTrackingClocks
{
	static final String FARMING_CONTRACT = "FARMING_CONTRACT";

	private TimeTrackingClocks()
	{
	}

	/** One clock as read: the tab, its label, its summary and when it is ready (or -1). */
	static final class Clock
	{
		final String key;
		final String label;
		final SummaryState state;
		final long completion;

		Clock(String key, String label, SummaryState state, long completion)
		{
			this.key = key;
			this.label = label;
			this.state = state;
			this.completion = completion;
		}
	}

	/** The active Time Tracking plugin, or null when it is off. */
	static TimeTrackingPlugin plugin()
	{
		PluginManager pm = Microbot.getPluginManager();
		if (pm == null)
		{
			return null;
		}
		for (Plugin p : pm.getPlugins())
		{
			if (p instanceof TimeTrackingPlugin && pm.isPluginActive(p) && p.getInjector() != null)
			{
				return (TimeTrackingPlugin) p;
			}
		}
		return null;
	}

	/** Every clock of the plugin: the birdhouses, each farming tab, the contract when there is one. */
	static List<Clock> read(TimeTrackingPlugin plugin)
	{
		List<Clock> out = new ArrayList<>();
		BirdHouseTracker birds = plugin.getInjector().getInstance(BirdHouseTracker.class);
		out.add(new Clock(Tab.BIRD_HOUSE.name(), Tab.BIRD_HOUSE.getName(), birds.getSummary(), birds.getCompletionTime()));
		FarmingTracker farming = plugin.getInjector().getInstance(FarmingTracker.class);
		for (Tab tab : Tab.FARMING_TABS)
		{
			out.add(new Clock(tab.name(), tab.getName(), farming.getSummary(tab), farming.getCompletionTime(tab)));
		}
		FarmingContractManager contract = plugin.getInjector().getInstance(FarmingContractManager.class);
		if (contract.hasContract())
		{
			out.add(new Clock(FARMING_CONTRACT, "Farming contract: " + contract.getContractName(),
				contract.getSummary(), contract.getCompletionTime()));
		}
		return out;
	}

	/**
	 * The ingest facet: tab -> clock, or {@code null} (not sent) when the manifest does not
	 * accept it or the Time Tracking plugin is off. Client thread.
	 */
	static Map<String, IngestContract.Timer> forIngest(IngestContract.Manifest manifest)
	{
		if (!manifest.timersEnabled())
		{
			return null;
		}
		TimeTrackingPlugin plugin = plugin();
		if (plugin == null)
		{
			return null;
		}
		Map<String, IngestContract.Timer> out = new TreeMap<>();
		for (Clock c : read(plugin))
		{
			out.put(c.key, IngestContract.Timer.of(c.label, c.state, c.completion));
		}
		return out;
	}

	/** One clock as the bridge serves it. */
	static Map<String, Object> row(String key, String label, SummaryState state, long completion)
	{
		IngestContract.Timer t = IngestContract.Timer.of(label, state, completion);
		Map<String, Object> m = new LinkedHashMap<>();
		m.put("key", key);
		m.put("label", label);
		m.put("state", t.state);
		if (t.readyAt != null)
		{
			m.put("readyAt", t.readyAt);
		}
		return m;
	}
}
