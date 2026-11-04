package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.time.Instant;
import java.util.Map;

/**
 * Revision counter of one live container: it restarts at zero with each live epoch and grows
 * only when the contents change within that epoch. Pure; touched from one thread at a time.
 */
final class ContainerTracker
{
	private String epoch;
	private long revision;
	private Map<String, Long> last;

	ContainerSnapshot live(String liveEpoch, Map<String, Long> quantities, Instant observedAt)
	{
		if (!liveEpoch.equals(epoch))
		{
			epoch = liveEpoch;
			revision = 0;
			last = ItemQuantities.copy(quantities);
		}
		else if (!last.equals(quantities))
		{
			revision++;
			last = ItemQuantities.copy(quantities);
		}
		return ContainerSnapshot.live(observedAt, revision, epoch, last);
	}

	void reset()
	{
		epoch = null;
		revision = 0;
		last = null;
	}
}
