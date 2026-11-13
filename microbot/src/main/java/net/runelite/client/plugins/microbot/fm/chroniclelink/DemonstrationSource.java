package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Instant;
import java.util.Map;
import java.util.TreeMap;
import java.util.UUID;
import net.runelite.api.Client;
import net.runelite.api.GameState;
import net.runelite.api.Item;
import net.runelite.api.ItemContainer;
import net.runelite.api.NPC;
import net.runelite.api.Player;
import net.runelite.api.ScriptEvent;
import net.runelite.api.WorldView;
import net.runelite.api.coords.WorldPoint;
import net.runelite.api.events.MenuOptionClicked;
import net.runelite.api.events.ScriptPreFired;
import net.runelite.api.gameval.InterfaceID;
import net.runelite.api.gameval.InventoryID;
import net.runelite.api.widgets.Widget;

/**
 * Reads what the demonstration records from the client and hands it to the
 * {@link DemonstrationRecorder}. Client thread only: every method is called from an event
 * handler. It only reads (ids, names, the player's tile, the inventory and equipment); it never
 * acts, consumes an event or changes a menu.
 */
final class DemonstrationSource
{
	private final Client client;
	private final DemonstrationRecorder recorder;

	DemonstrationSource(Client client, DemonstrationRecorder recorder)
	{
		this.client = client;
		this.recorder = recorder;
	}

	/**
	 * Matches the recorder to whether recording is wanted now (the owner's toggle, the manifest's
	 * flag, a readable character): starts a recording for the logged-in character, or stops it.
	 * True while recording.
	 */
	boolean sync(boolean wanted)
	{
		if (!wanted || client.getGameState() != GameState.LOGGED_IN)
		{
			if (recorder.recording())
			{
				recorder.stop();
			}
			return false;
		}
		Player local = client.getLocalPlayer();
		String me = local == null ? null : local.getName();
		if (me == null || me.trim().isEmpty())
		{
			return false;
		}
		if (!recorder.recordingFor(me))
		{
			recorder.start(me, "rec-" + UUID.randomUUID());
		}
		return true;
	}

	/** A game tick: a position sample when worth one. */
	boolean onTick(Instant now)
	{
		return recorder.onTick(now, here());
	}

	/** A clicked menu entry the demonstration records, if it is one. */
	boolean onMenu(MenuOptionClicked event, Instant now)
	{
		String option = event.getMenuOption();
		switch (DemonstrationMenu.classify(event.getMenuAction(), event.getParam1(), event.isItemOp(), option))
		{
			case NPC:
			{
				NPC npc = event.getMenuEntry().getNpc();
				return npc != null && recorder.onNpc(now, here(), npc.getId(), option, npc.getName());
			}
			case OBJECT:
				return recorder.onObject(now, here(), event.getId(), option, event.getMenuTarget(),
					sceneTile(event.getParam0(), event.getParam1()));
			case ITEM_OP:
				return event.getItemId() >= 0 && recorder.onItemOp(now, here(), event.getItemId(), option, event.getMenuTarget());
			case USE_ON_NPC:
			{
				Integer used = selectedItem();
				NPC npc = event.getMenuEntry().getNpc();
				return used != null && npc != null && recorder.onItemUse(now, here(), used, npc.getId(), null, null, npc.getName());
			}
			case USE_ON_OBJECT:
			{
				Integer used = selectedItem();
				return used != null && recorder.onItemUse(now, here(), used, null, event.getId(), null, objectName(event.getMenuTarget()));
			}
			case USE_ON_ITEM:
			{
				Integer used = selectedItem();
				Widget target = event.getWidget();
				return used != null && target != null && target.getItemId() >= 0
					&& recorder.onItemUse(now, here(), used, null, null, target.getItemId(), null);
			}
			case DIALOGUE:
				return recorder.onDialogue(now, here(), dialogueOption(event.getParam0()));
			default:
				return false;
		}
	}

	/**
	 * A dialogue option chosen with a number key: the dialogue options' key listener runs a script
	 * whose event carries the typed key. Only while the options are on screen; a click on an
	 * option is recorded by {@link #onMenu}, and the recorder keeps one choice per options shown.
	 */
	boolean onScript(ScriptPreFired event, Instant now)
	{
		ScriptEvent se = event.getScriptEvent();
		if (se == null || se.getSource() == null || !DemonstrationMenu.dialogueWidget(se.getSource().getId()))
		{
			return false;
		}
		int index = DemonstrationMenu.keyOption(se.getTypedKeyChar());
		return index > 0 && recorder.onDialogue(now, here(), dialogueOption(index));
	}

	/** An interface loaded: the bank, a shop or a new set of dialogue options. */
	boolean onWidgetLoaded(int groupId, Instant now)
	{
		switch (groupId)
		{
			case InterfaceID.BANKMAIN:
			{
				Map<Integer, Long> carried = items(InventoryID.INV);
				items(InventoryID.WORN).forEach((id, qty) -> carried.merge(id, qty, Long::sum));
				return recorder.onBankOpened(now, here(), carried);
			}
			case InterfaceID.SHOPMAIN:
				recorder.onShopOpened(items(InventoryID.INV));
				return false;
			case InterfaceID.CHATMENU:
				recorder.onDialogueOptions();
				return false;
			default:
				return false;
		}
	}

	void onWidgetClosed(int groupId)
	{
		if (groupId == InterfaceID.BANKMAIN)
		{
			recorder.onClosed(DemonstrationRecorder.Open.BANK);
		}
		else if (groupId == InterfaceID.SHOPMAIN)
		{
			recorder.onClosed(DemonstrationRecorder.Open.SHOP);
		}
	}

	/** The inventory or the equipment changed: withdrawn, deposited or bought, while open. */
	boolean onContainer(int containerId, Instant now)
	{
		if (containerId != InventoryID.INV && containerId != InventoryID.WORN)
		{
			return false;
		}
		return recorder.onCarried(now, here(), items(InventoryID.INV), items(InventoryID.WORN)) > 0;
	}

	/** Where the local player stands (the real tile in an instance's template coordinates). */
	private DemonstrationRecorder.Tile here()
	{
		Player local = client.getLocalPlayer();
		if (local == null)
		{
			return null;
		}
		WorldPoint at = WorldPoint.fromLocalInstance(client, local.getLocalLocation());
		return at == null ? null : new DemonstrationRecorder.Tile(client.getWorld(), at.getX(), at.getY(), at.getPlane());
	}

	/** The world tile of a scene tile of the top-level view (an object's menu entry). */
	private DemonstrationRecorder.Tile sceneTile(int sceneX, int sceneY)
	{
		WorldView wv = client.getTopLevelWorldView();
		if (wv == null)
		{
			return null;
		}
		WorldPoint at = WorldPoint.fromScene(wv, sceneX, sceneY, wv.getPlane());
		return at == null ? null : new DemonstrationRecorder.Tile(client.getWorld(), at.getX(), at.getY(), at.getPlane());
	}

	/** The inventory item selected for "Use", or null (nothing selected, or a spell). */
	private Integer selectedItem()
	{
		if (!client.isWidgetSelected())
		{
			return null;
		}
		Widget selected = client.getSelectedWidget();
		return selected == null || selected.getItemId() < 0 ? null : selected.getItemId();
	}

	/** The text of a dialogue option by its place (1 to n), or null. */
	private String dialogueOption(int index)
	{
		Widget options = client.getWidget(InterfaceID.Chatmenu.OPTIONS);
		Widget[] children = options == null ? null : options.getDynamicChildren();
		if (children == null || index < 1 || index >= children.length || children[index] == null)
		{
			return null;
		}
		return children[index].getText();
	}

	/** "Use Bucket -> Sink" targets end with the object's name after the arrow. */
	static String objectName(String target)
	{
		if (target == null)
		{
			return null;
		}
		int arrow = target.lastIndexOf("->");
		return arrow < 0 ? target : target.substring(arrow + 2);
	}

	private Map<Integer, Long> items(int containerId)
	{
		Map<Integer, Long> out = new TreeMap<>();
		ItemContainer container = client.getItemContainer(containerId);
		if (container == null)
		{
			return out;
		}
		for (Item item : container.getItems())
		{
			if (item != null && item.getId() >= 0 && item.getQuantity() > 0)
			{
				out.merge(item.getId(), (long) item.getQuantity(), Long::sum);
			}
		}
		return out;
	}
}
