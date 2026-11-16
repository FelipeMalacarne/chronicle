package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.Collection;
import java.util.EnumSet;
import java.util.Set;
import java.util.List;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaCalculator.MtaException;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaCalculator.Points;

/**
 * What to do next to get the chosen rewards: buy the first one in the plan when the points held
 * cover it, else earn the points it lacks, in one room. Pure: the executor runs the step, reads the
 * points again and asks again, so a purchase (which spends points) or a level-up needs no special case.
 */
final class MtaPlanner
{
	private MtaPlanner()
	{
	}

	/** The rooms, in the order the planner prefers them: the Enchanting Chamber last, so Magic grows first. */
	enum Room
	{
		// The batch: about ten minutes of the room at the rates seen in game (2026-11-15: the Theatre
		// ~3 points a minute, the Playground ~5, the Graveyard with peaches ~6, the Chamber ~60).
		TELEKINETIC(30), ALCHEMIST(50), GRAVEYARD(60), ENCHANTMENT(600);

		/** The most points the player stays on for in this room past what the reward in turn needs. */
		final int batch;

		Room(int batch)
		{
			this.batch = batch;
		}
	}

	enum Kind
	{
		/** Every chosen reward is owned. */
		DONE,
		/** Buy {@link Step#reward} at the Rewards Guardian. */
		BUY,
		/** Earn {@link Step#points} more points in {@link Step#room}. */
		EARN,
		/** The next reward needs points in the Enchanting Chamber and no Enchant is castable. */
		BLOCKED,
		/** The points are missing in a room the owner plays by hand: the plugin waits for them. */
		MANUAL,
		/** Every chosen reward's points are in; they are bought by hand (or with "Buy rewards" on). */
		READY
	}

	static final class Step
	{
		final Kind kind;
		final String reward;
		final Room room;
		final int points;
		/** The points are for {@link #reward} after the ones before it in the plan, already covered. */
		final boolean ahead;

		Step(Kind kind, String reward, Room room, int points)
		{
			this(kind, reward, room, points, false);
		}

		Step(Kind kind, String reward, Room room, int points, boolean ahead)
		{
			this.kind = kind;
			this.reward = reward;
			this.room = room;
			this.points = points;
			this.ahead = ahead;
		}

		@Override
		public String toString()
		{
			return kind + (reward == null ? "" : " " + reward) + (room == null ? "" : " " + room + " " + points)
				+ (ahead ? " ahead" : "");
		}
	}

	static Step next(Collection<String> wanted, Collection<String> owned, Points held, int magic,
		int enchantLevel) throws MtaException
	{
		return next(wanted, owned, held, magic, enchantLevel, EnumSet.allOf(Room.class), true);
	}

	/**
	 * {@code allowed}: the rooms the plugin may play (one outside it with points missing is left to the
	 * owner, {@link Kind#MANUAL}). {@code buyAll}: buy every reward once its points are in; without it
	 * only Bones to Peaches is bought (it changes how the Graveyard is played) and the rest of the points
	 * are only gathered.
	 *
	 * <p>The points are gathered reward by reward in purchase order: the target is what the rewards up
	 * to the first not yet covered cost together, so at any time as many rewards as possible are
	 * covered, the rooms taking turns instead of one room filling up for every reward. Once Bones to
	 * Peaches is owned, Graveyard points wait for Magic 60, when the spell halves their trips; the
	 * Graveyard with bananas comes only when nothing else is left.
	 */
	static Step next(Collection<String> wanted, Collection<String> owned, Points held, int magic,
		int enchantLevel, Set<Room> allowed, boolean buyAll) throws MtaException
	{
		return next(wanted, owned, held, magic, enchantLevel, allowed, buyAll, null);
	}

	/**
	 * As above; {@code current} is the room the player is in (or null). It is kept for the next
	 * {@link #LOOKAHEAD} rewards after the one in turn while what they still need from it is a small
	 * batch ({@link Room#batch}), so the runner does not walk to another room and the bank chest for a
	 * handful of points, and never stays for hours in a room the reward in turn no longer needs.
	 */
	static Step next(Collection<String> wanted, Collection<String> owned, Points held, int magic,
		int enchantLevel, Set<Room> allowed, boolean buyAll, Room current) throws MtaException
	{
		List<String> order = MtaCalculator.buyOrder(wanted, owned);
		if (order.isEmpty())
		{
			return new Step(Kind.DONE, null, null, 0);
		}
		boolean peachesOwned = owned.contains(MtaCalculator.PEACHES);
		boolean waitGraveyard = peachesOwned && magic < MtaCalculator.PEACHES_MAGIC;
		int[] have = {held.telekinetic, held.alchemist, held.graveyard, held.enchantment};
		int[][] cumulative = new int[order.size()][4];
		int[] running = new int[4];
		for (int k = 0; k < order.size(); k++)
		{
			MtaReward x = MtaCalculator.reward(order.get(k));
			running[Room.TELEKINETIC.ordinal()] += x.telekinetic;
			running[Room.ALCHEMIST.ordinal()] += x.alchemist;
			running[Room.GRAVEYARD.ordinal()] += x.graveyard;
			running[Room.ENCHANTMENT.ordinal()] += x.enchantment;
			cumulative[k] = running.clone();
		}
		boolean enchantable = MtaCalculator.enchantLevelFor(magic, enchantLevel) > 0;
		boolean stayable = current != null && allowed.contains(current)
			&& !(current == Room.GRAVEYARD && waitGraveyard) && !(current == Room.ENCHANTMENT && !enchantable);
		int[] total = new int[4];
		Step manual = null;
		Step graveyardLater = null;
		for (int k = 0; k < order.size(); k++)
		{
			MtaReward r = MtaCalculator.reward(order.get(k));
			total[Room.TELEKINETIC.ordinal()] += r.telekinetic;
			total[Room.ALCHEMIST.ordinal()] += r.alchemist;
			total[Room.GRAVEYARD.ordinal()] += r.graveyard;
			total[Room.ENCHANTMENT.ordinal()] += r.enchantment;
			if (k == 0 && (buyAll || r.id.equals(MtaCalculator.PEACHES)) && covers(have, total))
			{
				return new Step(Kind.BUY, r.id, null, 0);
			}
			for (Room room : Room.values())
			{
				int n = total[room.ordinal()] - have[room.ordinal()];
				if (n <= 0)
				{
					continue;
				}
				if (!allowed.contains(room))
				{
					if (manual == null)
					{
						manual = new Step(Kind.MANUAL, r.id, room, n);
					}
					continue;
				}
				if (room == Room.GRAVEYARD && waitGraveyard)
				{
					if (graveyardLater == null)
					{
						graveyardLater = new Step(Kind.EARN, r.id, room, n, k > 0);
					}
					continue;
				}
				if (room == Room.ENCHANTMENT && !enchantable)
				{
					return new Step(Kind.BLOCKED, r.id, room, n);
				}
				if (stayable && current != room)
				{
					for (int j = k; j < Math.min(order.size(), k + 1 + LOOKAHEAD); j++)
					{
						int m = cumulative[j][current.ordinal()] - have[current.ordinal()];
						if (m > current.batch)
						{
							break;
						}
						if (m > 0)
						{
							return new Step(Kind.EARN, order.get(j), current, m, j > 0);
						}
					}
				}
				return new Step(Kind.EARN, r.id, room, n, k > 0);
			}
		}
		if (graveyardLater != null)
		{
			// Everything else is in: the Graveyard with bananas, rather than standing still.
			return graveyardLater;
		}
		if (manual != null)
		{
			return manual;
		}
		return new Step(Kind.READY, order.get(0), null, 0);
	}

	/** Rewards past the one in turn the room the player is in keeps gathering for. */
	static final int LOOKAHEAD = 2;

	private static boolean covers(int[] have, int[] cost)
	{
		for (int i = 0; i < have.length; i++)
		{
			if (have[i] < cost[i])
			{
				return false;
			}
		}
		return true;
	}
}
