package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.Locale;
import java.util.concurrent.ConcurrentLinkedQueue;
import net.runelite.api.ChatMessageType;

/**
 * What the client thread tells the script loop: blackjack outcomes from chat, hits on the player
 * and Hitpoints. Written from the plugin's event handlers, read by the steps; HP and hits come
 * from here so the safety checks never wait on the client thread.
 */
final class GameEvents
{
	enum Outcome
	{
		KNOCKED_OUT,
		KNOCKOUT_FAILED,
		ALREADY_UNCONSCIOUS,
		PICKED,
		PICK_FAILED,
		SEEN,
		POUCHES_FULL,
		CANT_REACH
	}

	// Targets attack every 4 ticks; no hit for longer than that means the fight is over.
	private static final long IN_COMBAT_MS = 3000;

	private final ConcurrentLinkedQueue<Outcome> outcomes = new ConcurrentLinkedQueue<>();
	private volatile long lastHitAt;
	/** A failed pickpocket stuns with a hit of its own; that one is not an attack. */
	private volatile long stunHitUntil;
	/** "You can't knock-out the bandit during combat.": knocking out again no longer works. */
	private volatile long refusedAt;
	/** When the last knock-out landed; the target is down for a few ticks from then. */
	private volatile long knockedOutAt;
	private volatile int hits;
	private volatile int hp;
	private volatile int maxHp;
	private volatile int thievingLevel;
	private volatile int thievingXp;

	void reset()
	{
		outcomes.clear();
		lastHitAt = 0;
		stunHitUntil = 0;
		refusedAt = 0;
		knockedOutAt = 0;
		hits = 0;
		hp = 0;
		maxHp = 0;
		thievingLevel = 0;
		thievingXp = 0;
	}

	/** A hitsplat landed on the local player: the only reliable sign we are being attacked. */
	void onPlayerHit()
	{
		long now = System.currentTimeMillis();
		if (now >= stunHitUntil)
		{
			lastHitAt = now;
			hits++;
		}
	}

	void onHitpoints(int boosted, int real)
	{
		hp = boosted;
		maxHp = real;
	}

	void onThieving(int level, int xp)
	{
		thievingLevel = level;
		thievingXp = xp;
	}

	void onChat(ChatMessageType type, String raw)
	{
		if (type != ChatMessageType.GAMEMESSAGE && type != ChatMessageType.SPAM && type != ChatMessageType.ENGINE)
		{
			return;
		}
		String message = raw.toLowerCase(Locale.ROOT);
		if (message.contains("render them unconscious"))
		{
			// A hit that landed as it went down is over; don't climb for it.
			lastHitAt = 0;
			knockedOutAt = System.currentTimeMillis();
			outcomes.add(Outcome.KNOCKED_OUT);
		}
		else if (message.contains("blow only glances"))
		{
			outcomes.add(Outcome.KNOCKOUT_FAILED);
		}
		else if (message.contains("they're unconscious"))
		{
			outcomes.add(Outcome.ALREADY_UNCONSCIOUS);
		}
		else if (message.startsWith("you pick the") && message.contains("pocket"))
		{
			outcomes.add(Outcome.PICKED);
		}
		else if (message.startsWith("you fail to pick"))
		{
			// The stun hitsplat can land just before or after this message.
			long now = System.currentTimeMillis();
			stunHitUntil = now + 1800;
			if (now - lastHitAt < 1200)
			{
				lastHitAt = 0;
			}
			outcomes.add(Outcome.PICK_FAILED);
		}
		else if (message.startsWith("perhaps i shouldn't"))
		{
			outcomes.add(Outcome.SEEN);
		}
		else if (message.contains("empty your coin pouch"))
		{
			outcomes.add(Outcome.POUCHES_FULL);
		}
		else if (message.contains("can't reach that"))
		{
			outcomes.add(Outcome.CANT_REACH);
		}
		else if (message.contains("during combat"))
		{
			// "You can't knock-out/pickpocket the bandit during combat."
			long now = System.currentTimeMillis();
			refusedAt = now;
			lastHitAt = now;
		}
	}

	Outcome poll()
	{
		return outcomes.poll();
	}

	/** A fight just ended: drop what it left behind. */
	void forgetFight()
	{
		outcomes.clear();
		lastHitAt = 0;
		refusedAt = 0;
	}

	boolean beingAttacked(long now)
	{
		return now - lastHitAt < IN_COMBAT_MS;
	}

	long knockedOutAt()
	{
		return knockedOutAt;
	}

	/** The game just refused a knock-out or pickpocket because we are in combat. */
	boolean refused(long now)
	{
		return now - refusedAt < IN_COMBAT_MS;
	}

	long lastHitAt()
	{
		return lastHitAt;
	}

	/** Non-stun hits taken since the run started; differences count hits over a stretch. */
	int hits()
	{
		return hits;
	}

	int hp()
	{
		return hp;
	}

	int maxHp()
	{
		return maxHp;
	}

	int thievingLevel()
	{
		return thievingLevel;
	}

	int thievingXp()
	{
		return thievingXp;
	}
}
