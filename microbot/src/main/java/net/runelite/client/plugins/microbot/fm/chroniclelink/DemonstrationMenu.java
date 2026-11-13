package net.runelite.client.plugins.microbot.fm.chroniclelink;

import net.runelite.api.MenuAction;
import net.runelite.api.gameval.InterfaceID;

/**
 * Which clicked menu entries the demonstration records (planner docs/10 section 11), from the
 * entry's action and widget only. Pure: the plugin reads the ids and names after this said what
 * the click was. Examine, Walk here, spells, other players, ground items and every interface
 * other than the inventory and the dialogue options are never recorded.
 */
final class DemonstrationMenu
{
	enum Click
	{
		/** Not recorded. */
		NONE,
		/** A menu option on an NPC. */
		NPC,
		/** A menu option on a game object. */
		OBJECT,
		/** An inventory item's own option. */
		ITEM_OP,
		/** The selected inventory item used on an NPC. */
		USE_ON_NPC,
		/** The selected inventory item used on a game object. */
		USE_ON_OBJECT,
		/** The selected inventory item used on another inventory item. */
		USE_ON_ITEM,
		/** A dialogue option clicked. */
		DIALOGUE
	}

	private DemonstrationMenu()
	{
	}

	/**
	 * @param param1 the entry's widget id (CC_OP, WIDGET_CONTINUE, WIDGET_TARGET_ON_WIDGET)
	 * @param itemOp whether the entry is an item op (MenuEntry.isItemOp: never Use or Examine)
	 * @param option the option text, for the item ops the demonstration skips
	 */
	static Click classify(MenuAction action, int param1, boolean itemOp, String option)
	{
		if (action == null)
		{
			return Click.NONE;
		}
		switch (action)
		{
			case NPC_FIRST_OPTION:
			case NPC_SECOND_OPTION:
			case NPC_THIRD_OPTION:
			case NPC_FOURTH_OPTION:
			case NPC_FIFTH_OPTION:
				return Click.NPC;
			case GAME_OBJECT_FIRST_OPTION:
			case GAME_OBJECT_SECOND_OPTION:
			case GAME_OBJECT_THIRD_OPTION:
			case GAME_OBJECT_FOURTH_OPTION:
			case GAME_OBJECT_FIFTH_OPTION:
				return Click.OBJECT;
			case WIDGET_TARGET_ON_NPC:
				return Click.USE_ON_NPC;
			case WIDGET_TARGET_ON_GAME_OBJECT:
				return Click.USE_ON_OBJECT;
			case WIDGET_TARGET_ON_WIDGET:
				return param1 == InterfaceID.Inventory.ITEMS ? Click.USE_ON_ITEM : Click.NONE;
			case WIDGET_CONTINUE:
				return param1 == InterfaceID.Chatmenu.OPTIONS ? Click.DIALOGUE : Click.NONE;
			case CC_OP:
			case CC_OP_LOW_PRIORITY:
				return itemOp && param1 == InterfaceID.Inventory.ITEMS && !skippedItemOp(option) ? Click.ITEM_OP : Click.NONE;
			default:
				return Click.NONE;
		}
	}

	/** Dropping is not a step of a route, and a power-dropping stretch would flood the recording. */
	static boolean skippedItemOp(String option)
	{
		String o = DemonstrationRecorder.text(option);
		return o == null || o.equalsIgnoreCase("Drop") || o.equalsIgnoreCase("Examine") || o.equalsIgnoreCase("Cancel");
	}

	/**
	 * The dialogue option a typed key chooses: '1' to '9' are the options' places, which are the
	 * indexes of the options' children in {@code Chatmenu.OPTIONS} (child 0 is the title); -1 for
	 * every other key.
	 */
	static int keyOption(int typedKeyChar)
	{
		return typedKeyChar >= '1' && typedKeyChar <= '9' ? typedKeyChar - '0' : -1;
	}

	/** Whether a widget id belongs to the dialogue options interface. */
	static boolean dialogueWidget(int widgetId)
	{
		return widgetId >>> 16 == InterfaceID.CHATMENU;
	}
}
