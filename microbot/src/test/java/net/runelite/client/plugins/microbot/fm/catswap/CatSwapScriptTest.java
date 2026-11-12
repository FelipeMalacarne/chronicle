package net.runelite.client.plugins.microbot.fm.catswap;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import net.runelite.api.gameval.ItemID;
import net.runelite.api.gameval.NpcID;
import org.junit.jupiter.api.Test;

class CatSwapScriptTest
{
	@Test
	void followersByKind()
	{
		assertEquals(CatSwapScript.Follower.NONE, CatSwapScript.followerOf(-1));
		assertEquals(CatSwapScript.Follower.CAT, CatSwapScript.followerOf(NpcID.GROWNCAT_BLACK));
		assertEquals(CatSwapScript.Follower.CAT, CatSwapScript.followerOf(NpcID.OVERGROWNCAT_LIGHT));
		assertEquals(CatSwapScript.Follower.CAT, CatSwapScript.followerOf(NpcID.WILEYCAT_HELL));
		assertEquals(CatSwapScript.Follower.KITTEN, CatSwapScript.followerOf(NpcID.KITTENPET_BROWN));
		assertEquals(CatSwapScript.Follower.OTHER, CatSwapScript.followerOf(NpcID.GERTRUDE_POST));
	}

	@Test
	void kittensAreNeverSold()
	{
		// The Civilians refuse kittens: no kitten item may count as a cat.
		for (int kitten : CatSwapScript.KITTEN_ITEMS)
		{
			assertFalse(CatSwapScript.contains(CatSwapScript.CAT_ITEMS, kitten), "kitten " + kitten);
		}
		assertTrue(CatSwapScript.contains(CatSwapScript.CAT_ITEMS, ItemID.GROWNCATOBJECT_BROWN));
		assertTrue(CatSwapScript.contains(CatSwapScript.CAT_ITEMS, ItemID.OVERGROWNCATOBJECT));
	}
}
