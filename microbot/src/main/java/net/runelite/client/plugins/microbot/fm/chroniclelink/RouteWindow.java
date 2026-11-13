package net.runelite.client.plugins.microbot.fm.chroniclelink;

import com.google.gson.annotations.SerializedName;
import java.util.Collections;
import java.util.List;

/**
 * Wire DTOs of {@code GET /api/plugin/route} (docs/04-arquitetura.md 2.5): the
 * active route from the cursor on, as the planner computes it. Every verdict, status and label
 * comes from the server; the plugin only displays them. Unknown fields are ignored (Gson).
 */
final class RouteWindow
{
	String guide;
	String title;
	/** Credit line of the guide's authors; every view of the steps shows it. */
	String attribution;
	Cursor cursor;
	Counts counts;
	List<Step> steps;
	Carry carry;
	/** The cursor's section with its description; null from an older planner. */
	Section section;

	List<Step> stepsOrEmpty()
	{
		return steps == null ? Collections.emptyList() : steps;
	}

	/** The cursor step, or null when the route is finished. */
	Step current()
	{
		for (Step s : stepsOrEmpty())
		{
			if (s.current)
			{
				return s;
			}
		}
		return null;
	}

	Step step(String id)
	{
		for (Step s : stepsOrEmpty())
		{
			if (s.id != null && s.id.equals(id))
			{
				return s;
			}
		}
		return null;
	}

	String cursorStepId()
	{
		return cursor == null ? null : cursor.stepId;
	}

	static final class Cursor
	{
		@SerializedName("step_id")
		String stepId;
		String cause;
	}

	static final class Counts
	{
		int total;
		int done;
		int remaining;
		int blocked;
	}

	static final class Step
	{
		String id;
		String ref;
		@SerializedName("section_id")
		String sectionId;
		@SerializedName("section_title")
		String sectionTitle;
		String title;
		String text;
		String location;
		/** pending, auto_done, manual_done, implied, skipped, pinned_open. */
		String status;
		Mark mark;
		boolean current;
		@SerializedName("blocked_by")
		List<Requirement> blockedBy;
		List<Objective> objectives;
		/** The guide's notes of the step; absent when none (and from an older planner). */
		List<Note> notes;
		/**
		 * The cursor step's typed tasks with whether the planner sees each one done: the checklist of
		 * the assisted mode (planner docs/10 phase 2). Only on the current step; absent when it has
		 * none and from an older planner.
		 */
		List<Task> tasks;

		List<Task> tasksOrEmpty()
		{
			return tasks == null ? Collections.emptyList() : tasks;
		}

		List<Note> notesOrEmpty()
		{
			return notes == null ? Collections.emptyList() : notes;
		}

		/**
		 * The notes the panel shows. hcim notes are left out: the window does not say the
		 * account's mode, and a hardcore-only warning would mislead a plain ironman.
		 */
		List<Note> visibleNotes()
		{
			List<Note> out = new java.util.ArrayList<>();
			for (Note n : notesOrEmpty())
			{
				if (n != null && n.text != null && !n.text.isEmpty() && !Note.HCIM.equals(n.kind))
				{
					out.add(n);
				}
			}
			return out;
		}

		List<Requirement> blockedOrEmpty()
		{
			return blockedBy == null ? Collections.emptyList() : blockedBy;
		}

		List<Objective> objectivesOrEmpty()
		{
			return objectives == null ? Collections.emptyList() : objectives;
		}

		boolean blocked()
		{
			return !blockedOrEmpty().isEmpty();
		}
	}

	/** A step mark, also the body of a restore. */
	static final class Mark
	{
		String kind;
		@SerializedName("implied_by_step_id")
		String impliedBy;
		String note;

		Mark()
		{
		}

		Mark(String kind, String impliedBy)
		{
			this.kind = kind;
			this.impliedBy = impliedBy;
		}
	}

	/** A note of the guide on a step. */
	static final class Note
	{
		static final String TIP = "tip";
		static final String UPDATE = "update";
		static final String HCIM = "hcim";

		/** tip, update or hcim. */
		String kind;
		String text;
	}

	/** The cursor's section. */
	static final class Section
	{
		String id;
		String title;
		String description;
	}

	static final class Requirement
	{
		String key;
		String label;
	}

	/** One line of the assisted mode's checklist: what to do, what ends it, and the planner's view. */
	static final class Task
	{
		/** travel, bank, buy, talk, interact, train or manual. */
		String type;
		String label;
		/** What ends the task ({@code until}), labelled; empty when the task does not say. */
		String until;
		/** met, unmet or unknown: unknown is never shown as not done. */
		String state;
		/** Why the planner cannot tell, when unknown. */
		String reason;
		/** Inherited from the recipe of an objective the step completes. */
		boolean recipe;
	}

	static final class Objective
	{
		String key;
		String role;
		String label;
		/** met, unmet or unknown. */
		String verdict;
		boolean regressed;
	}

	static final class Carry
	{
		@SerializedName("segment_id")
		String segmentId;
		String title;
		String status;
		@SerializedName("loadout_status")
		String loadoutStatus;
		/** The guide lists no setup: the items preview what the next steps use. */
		boolean derived;
		List<CarryItem> items;
		@SerializedName("bank_actions")
		List<String> bankActions;

		List<CarryItem> itemsOrEmpty()
		{
			return items == null ? Collections.emptyList() : items;
		}

		List<String> bankActionsOrEmpty()
		{
			return bankActions == null ? Collections.emptyList() : bankActions;
		}
	}

	static final class CarryItem
	{
		String name;
		long qty;
		@SerializedName("qty_stated")
		boolean qtyStated;
		String slot;
		boolean optional;
		/** About the live inventory/equipment. */
		String verdict;
		long carried;
		/** Ownership anywhere (bank included), or null. */
		String owned;
		long held;
	}
}
