package net.runelite.client.plugins.microbot.fm.blackjacker;

import java.util.HashMap;
import java.util.Locale;
import java.util.Map;
import lombok.extern.slf4j.Slf4j;
import net.runelite.api.EquipmentInventorySlot;
import net.runelite.api.MenuAction;
import net.runelite.api.NPCComposition;
import net.runelite.api.gameval.AnimationID;
import net.runelite.client.plugins.microbot.Microbot;
import net.runelite.client.plugins.microbot.api.npc.models.Rs2NpcModel;
import net.runelite.client.plugins.microbot.util.Global;
import net.runelite.client.plugins.microbot.util.menu.NewMenuEntry;
import net.runelite.client.plugins.microbot.util.misc.Rs2UiHelper;
import net.runelite.client.plugins.microbot.util.equipment.Rs2Equipment;
import net.runelite.client.plugins.microbot.util.inventory.Rs2Inventory;
import net.runelite.client.plugins.microbot.util.inventory.Rs2ItemModel;
import net.runelite.client.plugins.microbot.util.player.Rs2Player;

/**
 * The thieving itself, as steps: keep a blackjack wielded, open coin pouches, knock out and
 * pickpocket twice. The target's fall (animation 838) shows a tick before the "You smack..."
 * message, but a pick clicked then is dropped: we are still finishing the knock-out. So the
 * first pick goes out on the message and the second two ticks later, both inside the few ticks
 * it stays down. When HP is low a drink takes the second pick's slot: the target cannot hit
 * back while it is down. A glance is knocked out again.
 */
@Slf4j
final class Blackjacking
{
	private static final int PICKS_PER_KNOCKOUT = 2;
	// A knock-out shows its result (fall, or a glance message) within two ticks; none by then
	// means the click did not take: send it again.
	private static final long KNOCK_OUT_RESULT_MS = 1800;
	// After a glance the knock-out has to land before the target's first hit; after that the
	// game refuses it "during combat". A single retry is sometimes dropped (measured: no response
	// at all), so knock out again every tick until the game answers, as the Hub's HSBlackJack does.
	private static final long RETRY_EVERY_MS = 700;
	// How long after a glance knock-outs are resent that fast.
	private static final long RETRY_WINDOW_MS = 3000;
	// Wiki: after a glance, a pickpocket on the target with a full inventory (it cannot land)
	// cancels the aggression the glance caused, so its first hit never comes. Clicked a tick after
	// the glance message: earlier clicks are dropped. The knock-out follows a tick later.
	private static final long CALM_AFTER_GLANCE_MS = 600;
	private static final long STUN_WINDOW_MS = 8000;
	// Measured: with the first pick clicked within a tick of the knock-out message, the picks
	// start at +600 and +1800 and both land; the target is up by +2400, when a pick fails (stun,
	// fight) or finds no room. So the first only within a tick of the message, the second two
	// ticks after the first and not later.
	private static final long FIRST_PICK_MS = 600;
	private static final long PICK_SPACING_MS = 1200;
	private static final long SECOND_PICK_LATE_MS = 1700;
	// No Knock-Out clicks before then ("They're unconscious").
	private static final long DOWN_MS = 3000;
	// The chat message trails the fall; one this soon after a fall seen is the same knock-out.
	private static final long SAME_KNOCKOUT_MS = 2000;
	// "I can't do that. They're unconscious.": give it a tick before trying again.
	private static final long STILL_DOWN_WAIT_MS = 600;

	private static final MenuAction[] NPC_OPTIONS = {
		MenuAction.NPC_FIRST_OPTION, MenuAction.NPC_SECOND_OPTION, MenuAction.NPC_THIRD_OPTION,
		MenuAction.NPC_FOURTH_OPTION, MenuAction.NPC_FIFTH_OPTION
	};

	/** NPC options by NPC id, read once: the click must not wait on the client thread. */
	private final Map<Integer, String[]> optionsById = new HashMap<>();
	private final Healing healing;
	private boolean knockOutPending;
	private long knockOutClickedAt;
	private long downAt;
	private long lastGlanceAt;
	private boolean calmPending;
	/** The knock-out message of the current knock-out: the clock for the picks. */
	private long knockOutMessageAt;
	private long firstPickAt;
	private int picksSent;
	private boolean drankWhileDown;
	private long holdUntil;
	private long stunnedSince;

	Blackjacking(Healing healing)
	{
		this.healing = healing;
	}

	/** Step: wield a blackjack from the inventory when none is wielded. */
	boolean wield(Session s)
	{
		Rs2ItemModel weapon = Rs2Equipment.get(EquipmentInventorySlot.WEAPON);
		if (weapon != null && isBlackjack(weapon))
		{
			return false;
		}
		Rs2ItemModel inInventory = Rs2Inventory.get(Blackjacking::isBlackjack);
		if (inInventory == null)
		{
			s.stop("No blackjack equipped or in the inventory");
			return true;
		}
		Microbot.status = "Blackjacker: wielding " + inInventory.getName();
		Rs2Inventory.interact(inInventory.getId(), "Wield");
		Global.sleepUntil(() -> Rs2Equipment.isWearing(inInventory.getId()), 1200);
		return true;
	}

	private static boolean isBlackjack(Rs2ItemModel item)
	{
		return item.getName() != null && item.getName().toLowerCase(Locale.ROOT).contains("blackjack");
	}

	/**
	 * Step: open coin pouches as soon as there is one, outside the picks after a knock-out. Pouches
	 * stack, so with one in a full inventory a pickpocket on a target that is already up still
	 * lands (stun, fight) instead of failing on "inventory full".
	 */
	boolean openPouches(Session s)
	{
		return s.now - downAt >= DOWN_MS && openAllPouches();
	}

	/** Opens every coin pouch in the inventory; false when there were none. */
	static boolean openAllPouches()
	{
		if (pouchCount() == 0)
		{
			return false;
		}
		Microbot.status = "Blackjacker: opening coin pouches";
		if (Rs2Inventory.interact("Coin pouch", "Open-all", true))
		{
			Global.sleepUntil(() -> pouchCount() == 0, 2400);
		}
		return true;
	}

	private static int pouchCount()
	{
		Rs2ItemModel pouch = Rs2Inventory.get("Coin pouch", true);
		return pouch == null ? 0 : pouch.getQuantity();
	}

	/**
	 * Step, ahead of healing and upkeep: while a knock-out is in flight, the target is down, or a
	 * glance has to be answered, only the timing matters. It reads results and picks, and after a
	 * glance it knocks out again ahead of a drink while HP is above {@link Session#retryFloor()}:
	 * a drink there costs the tick the next knock-out needs before the target's first hit, which
	 * starts a fight (wine goes in the knock-out window instead). Other knock-outs are left to the
	 * last step, after healing.
	 */
	boolean burst(Session s)
	{
		boolean retry = s.now - lastGlanceAt < RETRY_WINDOW_MS && s.hp() >= s.retryFloor();
		return s.victim != null && (knockOutPending || calmPending || retry || s.now - downAt < DOWN_MS) && act(s, retry);
	}

	/** Step: one knock-out or pickpocket on the target in the room. */
	boolean strike(Session s)
	{
		return act(s, true);
	}

	private boolean act(Session s, boolean mayKnockOut)
	{
		Rs2NpcModel victim = s.victim;
		if (victim == null)
		{
			return false;
		}
		if (!readOutcomes(s))
		{
			return true;
		}

		if (knockOutPending)
		{
			int animation = victim.getAnimation();
			// A glance is read from chat ("blow only glances"): the target can still be finishing
			// a swing from an earlier fight as it falls, so its animation cannot tell.
			if (animation == AnimationID.HUMAN_UNCONSCIOUS)
			{
				fell(s.now);
			}
			else if (s.now - knockOutClickedAt < (s.now - lastGlanceAt < RETRY_WINDOW_MS ? RETRY_EVERY_MS : KNOCK_OUT_RESULT_MS))
			{
				return true;
			}
			else
			{
				knockOutPending = false;
			}
		}
		if (s.now < holdUntil)
		{
			return true;
		}
		if (s.now - stunnedSince < STUN_WINDOW_MS && Rs2Player.isStunned())
		{
			Microbot.status = "Blackjacker: stunned";
			return true;
		}

		if (s.now - downAt < DOWN_MS)
		{
			down(s, victim);
			return true;
		}
		if (calmPending)
		{
			if (s.now - lastGlanceAt < CALM_AFTER_GLANCE_MS)
			{
				return true;
			}
			calmPending = false;
			if (Rs2Inventory.isFull() && !Rs2Inventory.hasItem("Coin pouch"))
			{
				Microbot.status = "Blackjacker: glanced, pickpocketing to calm it down";
				log.info("Blackjacker timing: calming pickpocket clicked {} ms after the glance", s.now - lastGlanceAt);
				quickClick(s, victim, "Pickpocket");
				holdUntil = s.now + 600;
				return true;
			}
		}
		if (Session.unconscious(victim))
		{
			// Still lying there: Knock-Out now would only get "They're unconscious".
			return true;
		}
		if (!mayKnockOut)
		{
			return false;
		}
		Microbot.status = "Blackjacker: knocking out " + victim.getName();
		Global.sleep(40, 140);
		if (s.now - lastGlanceAt < 5000)
		{
			log.info("Blackjacker timing: knock-out retry clicked {} ms after the glance", System.currentTimeMillis() - lastGlanceAt);
		}
		if (quickClick(s, victim, "Knock-Out"))
		{
			knockOutPending = true;
			knockOutClickedAt = System.currentTimeMillis();
		}
		return true;
	}

	/**
	 * The menu entry Rs2NpcModel.click sends, through the quick click. The normal click first
	 * moves the mouse along a natural path: 350-600 ms measured here, the difference between the
	 * first pick landing the tick after the fall or a tick later, when the second no longer fits.
	 */
	private boolean quickClick(Session s, Rs2NpcModel npc, String action)
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
				return npc.click(action);
			}
			optionsById.put(id, options);
		}
		for (int i = 0; i < Math.min(options.length, NPC_OPTIONS.length); i++)
		{
			if (action.equalsIgnoreCase(options[i]))
			{
				Microbot.doInvokeQuick(new NewMenuEntry()
					.param0(0)
					.param1(0)
					.opcode(NPC_OPTIONS[i].getId())
					.identifier(npc.getIndex())
					.itemId(-1)
					.target(s.target.npcName)
					.actor(npc.getNpc())
					.option(options[i]), Rs2UiHelper.getActorClickbox(npc.getNpc()));
				return true;
			}
		}
		return npc.click(action);
	}

	/**
	 * While the target is down: the first pick once the knock-out message is in (within a tick
	 * of it), then two ticks later the second, or a drink in its place when HP is low.
	 */
	private void down(Session s, Rs2NpcModel victim)
	{
		if (picksSent == 0)
		{
			if (knockOutMessageAt < downAt)
			{
				// Fallen, message not in yet: still finishing the knock-out.
				return;
			}
			if (s.now - knockOutMessageAt < FIRST_PICK_MS)
			{
				firstPickAt = s.now;
				pick(s, victim);
			}
			else
			{
				// Too late for the first: the second slot still counts from where it would have been.
				firstPickAt = knockOutMessageAt;
				picksSent = 1;
			}
			return;
		}
		if (picksSent > 1 || s.now - firstPickAt < PICK_SPACING_MS)
		{
			return;
		}
		picksSent = PICKS_PER_KNOCKOUT;
		if (s.now - firstPickAt >= SECOND_PICK_LATE_MS)
		{
			return;
		}
		if (!drankWhileDown && s.lowHp() && healing.drinkIfReady(s))
		{
			// Low: the drink takes the second pick's slot, while it is down and cannot hit back.
			drankWhileDown = true;
			return;
		}
		pick(s, victim);
	}

	private void pick(Session s, Rs2NpcModel victim)
	{
		picksSent++;
		Microbot.status = "Blackjacker: pickpocketing (" + picksSent + "/" + PICKS_PER_KNOCKOUT + ")";
		long before = System.currentTimeMillis();
		quickClick(s, victim, "Pickpocket");
		log.info("Blackjacker timing: pick {} clicked {} ms after the knock-out message (click took {} ms)",
			picksSent, before - knockOutMessageAt, System.currentTimeMillis() - before);
	}

	/** The target just fell: no knock-outs until it is up, picks once the message is in. */
	private void fell(long now)
	{
		knockOutPending = false;
		downAt = now;
		picksSent = 0;
		drankWhileDown = false;
	}

	/** Counts what chat reports; false when the run has to stop or wait on it. */
	private boolean readOutcomes(Session s)
	{
		GameEvents.Outcome outcome;
		while ((outcome = s.events.poll()) != null)
		{
			switch (outcome)
			{
				case KNOCKED_OUT:
					s.knockouts++;
					if (s.now - downAt > SAME_KNOCKOUT_MS)
					{
						// The fall was missed: it went down about a tick before the message.
						fell(s.events.knockedOutAt() - 600);
					}
					knockOutMessageAt = s.events.knockedOutAt();
					break;
				case KNOCKOUT_FAILED:
					// Knock out again right away (wiki), after a drink if low.
					s.knockoutFails++;
					knockOutPending = false;
					lastGlanceAt = s.now;
					calmPending = true;
					break;
				case ALREADY_UNCONSCIOUS:
					knockOutPending = false;
					holdUntil = s.now + STILL_DOWN_WAIT_MS;
					break;
				case PICKED:
					s.picks++;
					break;
				case PICK_FAILED:
					s.pickFails++;
					stunnedSince = s.now;
					break;
				case POUCHES_FULL:
					openAllPouches();
					return false;
				case SEEN:
					s.stop("\"Perhaps I shouldn't do this here\": another " + s.target.npcName + " can see the room");
					return false;
				default:
					break;
			}
		}
		return true;
	}
}
