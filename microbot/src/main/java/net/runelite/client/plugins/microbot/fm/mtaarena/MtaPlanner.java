package net.runelite.client.plugins.microbot.fm.mtaarena;

import java.util.Collection;
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
		BLOCKED
	}

	static final class Step
	{
		final Kind kind;
		final String reward;
		final Room room;
		final int points;

		Step(Kind kind, String reward, Room room, int points)
		{
			this.kind = kind;
			this.reward = reward;
			this.room = room;
			this.points = points;
		}

		@Override
		public String toString()
		{
			return kind + (reward == null ? "" : " " + reward) + (room == null ? "" : " " + room + " " + points);
		}
	}

	static Step next(Collection<String> wanted, Collection<String> owned, Points held, int magic,
		int enchantLevel) throws MtaException
	{
		List<String> order = MtaCalculator.buyOrder(wanted, owned);
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
		for (Room room : Room.values())
		{
			int n = lack[room.ordinal()];
			if (n == 0)
			{
				continue;
			}
			if (room == Room.ENCHANTMENT && MtaCalculator.enchantLevelFor(magic, enchantLevel) == 0)
			{
				return new Step(Kind.BLOCKED, r.id, room, n);
			}
			return new Step(Kind.EARN, r.id, room, n);
		}
		return new Step(Kind.BUY, r.id, null, 0);
	}
}
