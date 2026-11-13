package net.runelite.client.plugins.microbot.fm.blackjacker;

import net.runelite.api.coords.WorldArea;
import net.runelite.api.coords.WorldPoint;

/**
 * A room a target is lured into and blackjacked in. With its curtain closed the lured NPC is
 * stuck inside and other NPCs outside cannot see the knock-outs.
 */
enum Room
{
	// Ali (Rana) the Dyer's house, the most north-eastern one in Pollnivneach: the wiki's spot for
	// bandits. Five tiles behind a curtain on its south side, with a ladder at the north end;
	// climbing it ends any fight because the bandit cannot follow.
	ALI_THE_DYER(
		new WorldArea(3363, 3000, 2, 3, 0),
		new WorldPoint(3364, 2999, 0),
		new WorldPoint(3364, 3000, 0),
		new WorldPoint(3364, 2998, 0),
		new WorldPoint(3364, 3002, 0),
		new WorldPoint(3364, 3003, 0),
		null),
	// Southern Pollnivneach, west of the Asp & Snake bar: four by four tiles (crates, a stool and
	// a table take the south-west and east), curtains on the north and south walls. The south one
	// was found open with NPCs walking through the room, so it is kept closed. Mapped in game on
	// 2026-11-12. No ladder: a fight is ended with the unarmed knock-out.
	THUG_TENT(
		new WorldArea(3348, 2953, 4, 4, 0),
		new WorldPoint(3350, 2957, 0),
		new WorldPoint(3350, 2956, 0),
		new WorldPoint(3350, 2958, 0),
		new WorldPoint(3351, 2953, 0),
		null,
		new WorldPoint(3350, 2952, 0));

	final WorldArea area;
	/** Tile the curtain wall object reports. */
	final WorldPoint curtain;
	/** Floor tiles on either side of the curtain. */
	final WorldPoint inside;
	final WorldPoint outside;
	/** Where the player ends a lead: the far end, so the NPC following stops inside. */
	final WorldPoint leadGoal;
	/** Ladder object inside the room (same x/y upstairs), or null when the room has none. */
	final WorldPoint ladder;
	/** Tile a second curtain reports, kept closed; null when the room has one curtain. */
	final WorldPoint backCurtain;

	Room(WorldArea area, WorldPoint curtain, WorldPoint inside, WorldPoint outside, WorldPoint leadGoal, WorldPoint ladder,
		WorldPoint backCurtain)
	{
		this.area = area;
		this.curtain = curtain;
		this.inside = inside;
		this.outside = outside;
		this.leadGoal = leadGoal;
		this.ladder = ladder;
		this.backCurtain = backCurtain;
	}

	boolean hasLadder()
	{
		return ladder != null;
	}

	/** Upstairs by the ladder, where a fight cannot follow. */
	boolean upstairs(WorldPoint me)
	{
		return ladder != null && me.getPlane() == 1 && me.distanceTo2D(ladder) <= 4;
	}

	/** In the room or in its doorway. */
	boolean inOrAt(WorldPoint me)
	{
		return area.contains(me) || me.distanceTo(curtain) <= 1;
	}
}
