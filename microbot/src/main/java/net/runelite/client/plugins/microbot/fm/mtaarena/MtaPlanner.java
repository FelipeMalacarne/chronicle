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
		TELEKINETIC, ALCHEMIST, GRAVEYARD, ENCHANTMENT
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
		MANUAL
	}

	static final class Step
	{
		final Kind kind;
		final String reward;
		final Room room;
		final int points;
		/**
		 * The points are for the rewards after {@link #reward}, which waits for a room the owner plays:
		 * the plugin works ahead in the rooms it plays meanwhile.
		 */
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
		return next(wanted, owned, held, magic, enchantLevel, EnumSet.allOf(Room.class));
	}

	/**
	 * {@code allowed} are the rooms the plugin may play: a room outside it with points missing is
	 * left to the owner ({@link Kind#MANUAL}), after the allowed rooms with points missing.
	 */
	static Step next(Collection<String> wanted, Collection<String> owned, Points held, int magic,
		int enchantLevel, Set<Room> allowed) throws MtaException
	{
		List<String> order = MtaCalculator.buyOrder(wanted, owned, magic);
		if (order.isEmpty())
		{
			return new Step(Kind.DONE, null, null, 0);
		}
		MtaReward r = MtaCalculator.reward(order.get(0));
		int[] lack = {
			Math.max(0, r.telekinetic - held.telekinetic),
			Math.max(0, r.alchemist - held.alchemist),
			Math.max(0, r.graveyard - held.graveyard),
			Math.max(0, r.enchantment - held.enchantment),
		};
		Step manual = null;
		for (Room room : Room.values())
		{
			int n = lack[room.ordinal()];
			if (n == 0)
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
			if (room == Room.ENCHANTMENT && MtaCalculator.enchantLevelFor(magic, enchantLevel) == 0)
			{
				return new Step(Kind.BLOCKED, r.id, room, n);
			}
			return new Step(Kind.EARN, r.id, room, n);
		}
		if (manual == null)
		{
			return new Step(Kind.BUY, r.id, null, 0);
		}
		// The first reward waits for the owner: meanwhile earn, in the rooms the plugin plays, what the
		// rest of the plan needs, so the run does not stop at the first reward.
		int[] total = new int[4];
		for (String id : order)
		{
			MtaReward x = MtaCalculator.reward(id);
			total[Room.TELEKINETIC.ordinal()] += x.telekinetic;
			total[Room.ALCHEMIST.ordinal()] += x.alchemist;
			total[Room.GRAVEYARD.ordinal()] += x.graveyard;
			total[Room.ENCHANTMENT.ordinal()] += x.enchantment;
		}
		int[] held2 = {held.telekinetic, held.alchemist, held.graveyard, held.enchantment};
		for (Room room : Room.values())
		{
			int n = Math.max(0, total[room.ordinal()] - held2[room.ordinal()]);
			if (n == 0 || !allowed.contains(room))
			{
				continue;
			}
			if (room == Room.ENCHANTMENT && MtaCalculator.enchantLevelFor(magic, enchantLevel) == 0)
			{
				continue;
			}
			return new Step(Kind.EARN, r.id, room, n, true);
		}
		return manual;
	}
}
