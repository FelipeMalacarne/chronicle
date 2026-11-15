package net.runelite.client.plugins.microbot.fm.mtaarena;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaCalculator.Points;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaPlanner.Kind;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaPlanner.Room;
import net.runelite.client.plugins.microbot.fm.mtaarena.MtaPlanner.Step;
import org.junit.jupiter.api.Test;

class MtaPlannerTest
{
	private static final List<String> NONE = Collections.emptyList();

	private static Step next(List<String> wanted, List<String> owned, Points held, int magic) throws Exception
	{
		return MtaPlanner.next(wanted, owned, held, magic, 0);
	}

	@Test
	void everythingOwnedIsDone() throws Exception
	{
		assertEquals(Kind.DONE, next(Arrays.asList("rune_pouch"), Arrays.asList("rune_pouch"), new Points(0, 0, 0, 0), 44).kind);
	}

	@Test
	void peachesFirst_earnsTheLeastEnchantingRoomLast() throws Exception
	{
		// Peaches cost 200/200/2000/300 and go first from Magic 60; the Enchanting Chamber is last in
		// the planner's preference.
		Step s = next(Arrays.asList("infinity_boots", "bones_to_peaches"), NONE, new Points(0, 0, 0, 0), 60);
		assertEquals(Kind.EARN, s.kind);
		assertEquals("bones_to_peaches", s.reward);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(200, s.points);
	}

	@Test
	void belowMagic60ThePeachesKeepTheirPlace() throws Exception
	{
		Step s = next(Arrays.asList("infinity_boots", "bones_to_peaches"), NONE, new Points(0, 0, 0, 0), 44);
		assertEquals("infinity_boots", s.reward);
	}

	@Test
	void roomsOtherThanEnchantingBeforeEnchanting() throws Exception
	{
		Step s = next(Arrays.asList("bones_to_peaches"), NONE, new Points(200, 200, 0, 300), 44);
		assertEquals(Kind.EARN, s.kind);
		assertEquals(Room.ENCHANTMENT, s.room);
		assertEquals(2000, s.points);
	}

	@Test
	void enoughPointsBuys() throws Exception
	{
		Step s = next(Arrays.asList("bones_to_peaches"), NONE, new Points(200, 200, 2000, 300), 44);
		assertEquals(Kind.BUY, s.kind);
		assertEquals("bones_to_peaches", s.reward);
	}

	@Test
	void afterThePurchaseTheNextRewardIsTheTarget() throws Exception
	{
		// Peaches bought (owned, points spent): the boots (120/120/1200/120) are next.
		Step s = next(Arrays.asList("infinity_boots", "bones_to_peaches"), Arrays.asList("bones_to_peaches"),
			new Points(0, 0, 0, 0), 44);
		assertEquals("infinity_boots", s.reward);
		assertEquals(120, s.points);
	}

	@Test
	void wandUpgradeNeedsTheTierBelowFirst() throws Exception
	{
		Step s = next(Arrays.asList("apprentice_wand"), NONE, new Points(1000, 1000, 10000, 1000), 44);
		assertEquals(Kind.BUY, s.kind);
		assertEquals("beginner_wand", s.reward);
	}

	@Test
	void noEnchantCastableBlocks() throws Exception
	{
		Step s = next(Arrays.asList("bones_to_peaches"), NONE, new Points(200, 200, 0, 300), 5);
		assertEquals(Kind.BLOCKED, s.kind);
		assertEquals(Room.ENCHANTMENT, s.room);
	}

	@Test
	void roomsOutsideTheAllowedSetAreLeftToTheOwner() throws Exception
	{
		// Peaches (200/200/2000/300) with only the Enchanting Chamber allowed: earn there first,
		// then wait for the rest by hand.
		java.util.Set<Room> only = java.util.EnumSet.of(Room.ENCHANTMENT);
		Step s = MtaPlanner.next(Arrays.asList("bones_to_peaches"), NONE, new Points(0, 0, 0, 0), 44, 0, only);
		assertEquals(Kind.EARN, s.kind);
		assertEquals(Room.ENCHANTMENT, s.room);
		s = MtaPlanner.next(Arrays.asList("bones_to_peaches"), NONE, new Points(0, 0, 2000, 0), 44, 0, only);
		assertEquals(Kind.MANUAL, s.kind);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(200, s.points);
	}

	@Test
	void whileTheFirstRewardWaitsForTheOwnerThePluginWorksAhead()
	{
		// Boots (120/120/1200/120) then peaches (200/200/2000/300), Magic 44, only the Enchanting
		// Chamber played: at 1200 the boots wait for the owner's rooms, the Chamber goes on to 3200.
		java.util.Set<Room> only = java.util.EnumSet.of(Room.ENCHANTMENT);
		Step s = assertDoesNotThrowNext(Arrays.asList("infinity_boots", "bones_to_peaches"), new Points(0, 0, 1200, 0), only);
		assertEquals(Kind.EARN, s.kind);
		assertEquals(Room.ENCHANTMENT, s.room);
		assertEquals(2000, s.points);
		assertEquals("infinity_boots", s.reward);
		assertEquals(true, s.ahead);
		// Everything the Chamber can give is in: now it is the owner's turn.
		s = assertDoesNotThrowNext(Arrays.asList("infinity_boots", "bones_to_peaches"), new Points(0, 0, 3200, 0), only);
		assertEquals(Kind.MANUAL, s.kind);
		assertEquals(Room.TELEKINETIC, s.room);
	}

	private static Step assertDoesNotThrowNext(List<String> wanted, Points held, java.util.Set<Room> allowed)
	{
		try
		{
			return MtaPlanner.next(wanted, NONE, held, 44, 0, allowed);
		}
		catch (Exception ex)
		{
			throw new AssertionError(ex);
		}
	}
}
