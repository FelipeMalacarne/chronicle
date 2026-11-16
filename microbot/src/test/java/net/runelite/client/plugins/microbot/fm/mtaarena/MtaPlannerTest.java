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

	private static final java.util.Set<Room> ALL = java.util.EnumSet.allOf(Room.class);

	/** Gathering only (no buying but the peaches), every room played. */
	private static Step gather(List<String> wanted, List<String> owned, Points held, int magic) throws Exception
	{
		return MtaPlanner.next(wanted, owned, held, magic, 0, ALL, false);
	}

	@Test
	void everythingOwnedIsDone() throws Exception
	{
		assertEquals(Kind.DONE, next(Arrays.asList("rune_pouch"), Arrays.asList("rune_pouch"), new Points(0, 0, 0, 0), 44).kind);
	}

	@Test
	void thePeachesComeFirstEvenBelowMagic60() throws Exception
	{
		// Peaches cost 200/200/2000/300; the Enchanting Chamber is last in the planner's preference.
		Step s = gather(Arrays.asList("infinity_boots", "bones_to_peaches"), NONE, new Points(0, 0, 0, 0), 44);
		assertEquals(Kind.EARN, s.kind);
		assertEquals("bones_to_peaches", s.reward);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(200, s.points);
	}

	@Test
	void thePeachesAreBoughtWhenTheirPointsAreIn() throws Exception
	{
		Step s = gather(Arrays.asList("infinity_boots", "bones_to_peaches"), NONE, new Points(200, 200, 2000, 300), 44);
		assertEquals(Kind.BUY, s.kind);
		assertEquals("bones_to_peaches", s.reward);
	}

	@Test
	void otherRewardsAreNotBoughtUnlessAsked() throws Exception
	{
		Points enough = new Points(120, 120, 1200, 120);
		assertEquals(Kind.READY, gather(Arrays.asList("infinity_boots"), NONE, enough, 44).kind);
		Step s = MtaPlanner.next(Arrays.asList("infinity_boots"), NONE, enough, 44, 0, ALL, true);
		assertEquals(Kind.BUY, s.kind);
		assertEquals("infinity_boots", s.reward);
	}

	@Test
	void pointsGoRewardByRewardTheRoomsTakingTurns() throws Exception
	{
		// Boots (120/120/1200/120) then the beginner wand (30/30/300/30), peaches owned, Magic 60.
		List<String> wanted = Arrays.asList("infinity_boots", "beginner_wand");
		List<String> peaches = Arrays.asList("bones_to_peaches");
		// The boots' telekinetic points first...
		Step s = gather(wanted, peaches, new Points(0, 0, 0, 0), 60);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(120, s.points);
		assertEquals("infinity_boots", s.reward);
		// ...then the boots' alchemist points, not the wand's telekinetic ones.
		s = gather(wanted, peaches, new Points(120, 0, 0, 0), 60);
		assertEquals(Room.ALCHEMIST, s.room);
		assertEquals(120, s.points);
		// Boots covered: on to the wand, on top of them.
		s = gather(wanted, peaches, new Points(120, 120, 1200, 120), 60);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(30, s.points);
		assertEquals("beginner_wand", s.reward);
		assertEquals(true, s.ahead);
		// All of it in: bought by hand.
		assertEquals(Kind.READY, gather(wanted, peaches, new Points(150, 150, 1500, 150), 60).kind);
	}

	@Test
	void withThePeachesOwnedTheGraveyardWaitsForMagic60() throws Exception
	{
		List<String> wanted = Arrays.asList("infinity_boots");
		List<String> peaches = Arrays.asList("bones_to_peaches");
		// Magic 49: the boots' graveyard points wait, the other rooms go on.
		Step s = gather(wanted, peaches, new Points(120, 0, 0, 0), 49);
		assertEquals(Room.ALCHEMIST, s.room);
		// Points(telekinetic, graveyard, enchantment, alchemist).
		s = gather(wanted, peaches, new Points(120, 0, 1200, 120), 49);
		// Nothing else left: the Graveyard with bananas rather than standing still.
		assertEquals(Kind.EARN, s.kind);
		assertEquals(Room.GRAVEYARD, s.room);
		// Magic 60: the Graveyard in its turn.
		s = gather(wanted, peaches, new Points(120, 0, 0, 120), 60);
		assertEquals(Room.GRAVEYARD, s.room);
	}

	@Test
	void theGraveyardIsPlayedForThePeachesThemselves() throws Exception
	{
		Step s = gather(Arrays.asList("bones_to_peaches"), NONE, new Points(200, 0, 0, 300), 44);
		assertEquals(Room.GRAVEYARD, s.room);
		assertEquals(200, s.points);
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
	void roomsOutsideTheAllowedSetAreLeftToTheOwnerAfterTheOthers() throws Exception
	{
		// Peaches with only the Enchanting Chamber played: earn there, then wait for the rest by hand.
		java.util.Set<Room> only = java.util.EnumSet.of(Room.ENCHANTMENT);
		Step s = MtaPlanner.next(Arrays.asList("bones_to_peaches"), NONE, new Points(0, 0, 0, 0), 44, 0, only, false);
		assertEquals(Kind.EARN, s.kind);
		assertEquals(Room.ENCHANTMENT, s.room);
		s = MtaPlanner.next(Arrays.asList("bones_to_peaches"), NONE, new Points(0, 0, 2000, 0), 44, 0, only, false);
		assertEquals(Kind.MANUAL, s.kind);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(200, s.points);
	}

	@Test
	void theRoomThePlayerIsInIsKeptForTheNextRewards() throws Exception
	{
		// Boots (120/120/1200/120), beginner wand (30/30/300/30), apprentice wand (60/60/600/60).
		// Points(telekinetic, graveyard, enchantment, alchemist).
		List<String> wanted = Arrays.asList("infinity_boots", "apprentice_wand");
		List<String> peaches = Arrays.asList("bones_to_peaches");
		Points bootsTele = new Points(120, 0, 0, 0);
		// Without a current room: the boots' alchemist points come next.
		assertEquals(Room.ALCHEMIST, MtaPlanner.next(wanted, peaches, bootsTele, 60, 0, ALL, false, null).room);
		// In the Telekinetic Theatre: stay for the wands' telekinetic points first.
		Step s = MtaPlanner.next(wanted, peaches, bootsTele, 60, 0, ALL, false, Room.TELEKINETIC);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(30, s.points);
		assertEquals("beginner_wand", s.reward);
		// Its share of the lookahead done: on to the room in turn.
		s = MtaPlanner.next(wanted, peaches, new Points(210, 0, 0, 0), 60, 0, ALL, false, Room.TELEKINETIC);
		assertEquals(Room.ALCHEMIST, s.room);
	}

	@Test
	void theRoomThePlayerIsInIsKeptOnlyForASmallBatch() throws Exception
	{
		// The account on 2026-11-16: peaches owned, boots and the beginner wand covered; the apprentice
		// wand still needs 60 Alchemist and 58 Graveyard points. In the Theatre, the teacher and master
		// wands' 375 telekinetic points are hours there: go to the Playground.
		List<String> wanted = Arrays.asList("infinity_boots", "beginner_wand", "apprentice_wand", "teacher_wand",
			"master_wand", "rune_pouch");
		List<String> peaches = Arrays.asList("bones_to_peaches");
		Points held = new Points(225, 152, 2428, 150);
		Step s = MtaPlanner.next(wanted, peaches, held, 62, 0, ALL, false, Room.TELEKINETIC);
		assertEquals(Room.ALCHEMIST, s.room);
		assertEquals("apprentice_wand", s.reward);
		assertEquals(60, s.points);
		// Twenty points short of the teacher wand's share there: a batch, kept.
		s = MtaPlanner.next(wanted, peaches, new Points(340, 152, 2428, 150), 62, 0, ALL, false, Room.TELEKINETIC);
		assertEquals(Room.TELEKINETIC, s.room);
		assertEquals(20, s.points);
	}
}
