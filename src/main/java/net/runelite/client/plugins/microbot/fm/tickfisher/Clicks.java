package net.runelite.client.plugins.microbot.fm.tickfisher;

import java.awt.Rectangle;
import java.util.HashMap;
import java.util.Map;
import net.runelite.api.MenuAction;
import net.runelite.api.NPCComposition;
import net.runelite.api.Point;
import net.runelite.api.gameval.InterfaceID;
import net.runelite.api.widgets.Widget;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.menu.NewMenuEntry;
import net.runelite.client.plugins.microbot.util.misc.Rs2UiHelper;

/**
 * The clicks of the cycle, built as the menu entries Rs2Inventory.invokeMenu and
 * Rs2NpcModel.click send, but timed by us.
 * <ul>
 * <li>{@link MouseMode#NATURAL}: the cursor travels a natural path to the target (unless it is
 * already on it) and clicks where it stops. {@link #hover} moves it ahead of time, so the click
 * that has to land at the start of a tick does not wait on a movement.</li>
 * <li>{@link MouseMode#QUICK}: the click lands at once and the cursor jumps there (~150 ms per
 * click, against 350-600 ms for a natural path).</li>
 * </ul>
 */
final class Clicks
{
	private static final MenuAction[] NPC_OPTIONS = {
		MenuAction.NPC_FIRST_OPTION, MenuAction.NPC_SECOND_OPTION, MenuAction.NPC_THIRD_OPTION,
		MenuAction.NPC_FOURTH_OPTION, MenuAction.NPC_FIFTH_OPTION
	};

	private final MouseMode mode;
	/** NPC options by NPC id, read once: the click must not wait on the client thread. */
	private final Map<Integer, String[]> optionsById = new HashMap<>();

	Clicks(MouseMode mode)
	{
		this.mode = mode;
	}

	/** Clicks an option of an NPC; false when the NPC has no such option. */
	boolean npc(Rs2NpcModel npc, String action)
	{
		int id = npc.getId();
		String[] options = optionsById.get(id);
		if (options == null)
		{
			options = Microbot.getClientThread().runOnClientThreadOptional(() -> {
				NPCComposition composition = Microbot.getClient().getNpcDefinition(id);
				return composition == null ? null : composition.getActions();
			}).orElse(null);
			if (options == null)
			{
				return false;
			}
			optionsById.put(id, options);
		}
		for (int i = 0; i < Math.min(options.length, NPC_OPTIONS.length); i++)
		{
			if (action.equalsIgnoreCase(options[i]))
			{
				send(new NewMenuEntry()
					.param0(0)
					.param1(0)
					.opcode(NPC_OPTIONS[i].getId())
					.identifier(npc.getIndex())
					.itemId(-1)
					.target(npc.getName())
					.actor(npc.getNpc())
					.option(options[i]), Rs2UiHelper.getActorClickbox(npc.getNpc()));
				return true;
			}
		}
		return false;
	}

	/** Selects an inventory item, the first half of using it on another. */
	boolean select(Rs2ItemModel item)
	{
		return item(item, "Use", MenuAction.WIDGET_TARGET);
	}

	/** Uses the selected item on this one. */
	boolean useOn(Rs2ItemModel item)
	{
		return item(item, "Use", MenuAction.WIDGET_TARGET_ON_WIDGET);
	}

	boolean eat(Rs2ItemModel item)
	{
		return item(item, "Eat", MenuAction.CC_OP);
	}

	boolean drop(Rs2ItemModel item)
	{
		return item(item, "Drop", MenuAction.CC_OP);
	}

	/** Natural mode: brings the cursor onto the item's slot without clicking. */
	void hover(Rs2ItemModel item)
	{
		if (mode != MouseMode.NATURAL)
		{
			return;
		}
		SlotView view = slot(item.getSlot());
		if (view != null)
		{
			aim(view.bounds);
		}
	}

	/** An inventory slot's option. Only with the plain inventory showing (no bank, no shop). */
	private boolean item(Rs2ItemModel item, String option, MenuAction opcode)
	{
		int slot = item.getSlot();
		SlotView view = slot(slot);
		if (view == null)
		{
			return false;
		}
		send(new NewMenuEntry()
			.option(option)
			.param0(slot)
			.param1(InterfaceID.Inventory.ITEMS)
			.opcode(opcode.getId())
			.identifier(view.identifier(option))
			.itemId(item.getId())
			.target(item.getName()), view.bounds);
		return true;
	}

	private static SlotView slot(int slot)
	{
		return Microbot.getClientThread().runOnClientThreadOptional(() -> {
			Widget inventory = Microbot.getClient().getWidget(InterfaceID.Inventory.ITEMS);
			if (inventory == null || inventory.isHidden())
			{
				return null;
			}
			Widget widget = inventory.getChild(slot);
			return widget == null ? null : new SlotView(widget.getActions(), widget.getBounds());
		}).orElse(null);
	}

	private void send(NewMenuEntry entry, Rectangle bounds)
	{
		Rectangle visible = Rs2UiHelper.isFallbackRectangle(bounds) ? null : Rs2UiHelper.clipToCanvas(bounds);
		if (visible == null)
		{
			// Off screen: the click goes through the client's own fallback.
			Microbot.doInvokeQuick(entry, bounds);
			return;
		}
		Microbot.getMouse().quickClick(aim(visible), entry);
	}

	/**
	 * Where to click inside the target. Natural: the cursor itself when it is already inside,
	 * else a point the cursor travels to first. Quick: a point the cursor jumps to.
	 */
	private Point aim(Rectangle target)
	{
		java.awt.Point at = Microbot.getMouse().getMousePosition();
		if (mode == MouseMode.NATURAL && at != null && target.contains(at))
		{
			return new Point(at.x, at.y);
		}
		Point point = Rs2UiHelper.getClickingPoint(target, true, false);
		if (mode == MouseMode.NATURAL)
		{
			Microbot.naturalMouse.moveTo(point.getX(), point.getY());
		}
		return point;
	}

	private static final class SlotView
	{
		final String[] actions;
		final Rectangle bounds;

		SlotView(String[] actions, Rectangle bounds)
		{
			this.actions = actions;
			this.bounds = bounds;
		}

		/** The option's 1-based index among the slot's actions, -1 when absent (as Rs2Inventory does). */
		int identifier(String option)
		{
			if (actions == null)
			{
				return -1;
			}
			for (int i = 0; i < actions.length; i++)
			{
				if (actions[i] != null && option.equalsIgnoreCase(actions[i].replaceAll("<[^>]*>", "")))
				{
					return i + 1;
				}
			}
			return -1;
		}
	}
}
