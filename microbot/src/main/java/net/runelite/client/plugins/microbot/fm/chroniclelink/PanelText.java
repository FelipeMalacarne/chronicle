package net.runelite.client.plugins.microbot.fm.chroniclelink;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.function.ToIntFunction;

/**
 * The panel's words (PT-BR, game terms in English), the kind of each verdict, item and step
 * (each kind gets its own drawn icon, never only a colour) and the text fitting (wrap and
 * ellipsis against a width function). Pure: no Swing, testable.
 */
final class PanelText
{
	static final String ELLIPSIS = "…";
	static final String DOT = " · ";

	/** How a step of the list stands, each one with its own icon. */
	enum StepKind
	{
		PENDING,
		/** auto_done or manual_done. */
		DONE,
		/** Presumed done by a later step; not proven, so it never looks like DONE. */
		IMPLIED,
		SKIPPED,
		/** Pending with unmet requirements: the cursor passes it. */
		BLOCKED
	}

	/** Where an item to carry stands, in the order the panel lists them. */
	enum ItemKind
	{
		/** Not carried and not owned anywhere. */
		MISSING,
		/** Not enough data to tell. */
		UNKNOWN,
		/** In the inventory or worn. */
		CARRIED,
		/** Owned, not carried: in the bank (or elsewhere). */
		BANK
	}

	private PanelText()
	{
	}

	/**
	 * The route's short name for the header: the guide id's first word, spelled as the title
	 * spells it when the title has that word ("bruhsailer-main" and "BRUHsailer Ironman Guide"
	 * give "BRUHsailer"), else capitalised ("oziris-main" gives "Oziris"). The full title goes
	 * in the tooltip.
	 */
	static String routeName(String guide, String title)
	{
		String stem = guide == null ? "" : guide.split("[-_.]", 2)[0];
		if (stem.isEmpty())
		{
			return title == null || title.isEmpty() ? "Chronicle" : title;
		}
		if (title != null)
		{
			for (String word : title.split("[^\\p{L}\\p{N}]+"))
			{
				if (word.equalsIgnoreCase(stem))
				{
					return word;
				}
			}
		}
		return stem.substring(0, 1).toUpperCase(Locale.ROOT) + stem.substring(1);
	}

	/** "249/564", empty without counts. */
	static String progress(RouteWindow.Counts c)
	{
		return c == null || c.total == 0 ? "" : c.done + "/" + c.total;
	}

	/** "62 bloqueados", empty when none. */
	static String blockedCount(RouteWindow.Counts c)
	{
		if (c == null || c.blocked <= 0)
		{
			return "";
		}
		return c.blocked + (c.blocked == 1 ? " bloqueado" : " bloqueados");
	}

	/** Footer line when URL and token come from the environment (managed worker). */
	static final String ENVIRONMENT_SOURCE = "credencial do ambiente";
	static final String ENVIRONMENT_SOURCE_TOOLTIP = "URL e token vêm de " + LinkEnvironment.SERVER_URL + " e "
		+ LinkEnvironment.TOKEN + "; a URL e a credencial das opções do plugin não valem aqui";
	static final String ENVIRONMENT_URL_INVALID = "A URL de " + LinkEnvironment.SERVER_URL + " não é válida.";

	/** The short status next to the header dot: "há 10 s", "desatualizado", ... */
	static String connectionShort(PanelState s, Instant now)
	{
		switch (s.connection)
		{
			case NOT_CONFIGURED:
				return "sem URL";
			case CONNECTING:
				return "conectando" + ELLIPSIS;
			case ONLINE:
				return s.fetchedAt == null ? "conectado" : ago(s.fetchedAt, now);
			case STALE:
				return "desatualizado";
			case OFFLINE:
			default:
				return "sem conexão";
		}
	}

	/** The full connection line, for the tooltip of the header status. */
	static String connection(PanelState s, Instant now)
	{
		switch (s.connection)
		{
			case NOT_CONFIGURED:
				return "Configure a URL do planner";
			case CONNECTING:
				return "Conectando" + ELLIPSIS;
			case ONLINE:
				return "Conectado" + since(s.fetchedAt, now);
			case STALE:
				return "Desatualizado" + since(s.fetchedAt, now) + (s.message == null ? "" : DOT + s.message);
			case OFFLINE:
			default:
				return "Sem conexão" + (s.message == null ? "" : DOT + s.message);
		}
	}

	/** The verdict as one short word next to an objective: feito, falta, sem dado. */
	static String verdictShort(RouteWindow.Objective o)
	{
		if ("met".equals(o.verdict))
		{
			return o.regressed ? "não vale mais" : "feito";
		}
		if ("unmet".equals(o.verdict))
		{
			return "falta";
		}
		return "sem dado";
	}

	/** A task's state in a word: done, still to do, or not known (never "to do" by default). */
	static String taskState(RouteWindow.Task t)
	{
		if ("met".equals(t.state))
		{
			return "feito";
		}
		if ("unmet".equals(t.state))
		{
			return "a fazer";
		}
		return "sem dado";
	}

	/** What ends a task and why its state is unknown, for its tooltip. */
	static String taskTooltip(RouteWindow.Task t)
	{
		StringBuilder b = new StringBuilder();
		if (t.until != null && !t.until.isEmpty())
		{
			b.append("Termina: ").append(t.until);
		}
		if (t.reason != null && !t.reason.isEmpty())
		{
			b.append(b.length() > 0 ? " · " : "").append(t.reason);
		}
		if (t.recipe)
		{
			b.append(b.length() > 0 ? " · " : "").append("da receita do objetivo");
		}
		return b.length() == 0 ? null : b.toString();
	}

	/** What the server's verdict means, for a tooltip. */
	static String verdictWord(String verdict)
	{
		if ("met".equals(verdict))
		{
			return "feito";
		}
		if ("unmet".equals(verdict))
		{
			return "não atende";
		}
		return "não sabemos";
	}

	/** The objective's label, with the role for requirements: "12 quest points (requer)". */
	static String objectiveLabel(RouteWindow.Objective o)
	{
		String label = o.label == null || o.label.isEmpty() ? o.key : o.label;
		if ("requires".equals(o.role) || "uses".equals(o.role))
		{
			return label + " (" + role(o.role) + ")";
		}
		return label;
	}

	/** The objective role as the web writes it. */
	static String role(String role)
	{
		if (role == null)
		{
			return "";
		}
		switch (role)
		{
			case "completes":
				return "conclui";
			case "starts":
				return "inicia";
			case "requires":
				return "requer";
			case "uses":
				return "usa";
			default:
				return role;
		}
	}

	/** The status word of a step; empty for a plain pending step. */
	static String status(String status)
	{
		if (status == null)
		{
			return "";
		}
		switch (status)
		{
			case "auto_done":
				return "detectado";
			case "manual_done":
				return "marcado";
			case "implied":
				return "presumido";
			case "skipped":
				return "pulado";
			case "pinned_open":
				return "reaberto";
			default:
				return "";
		}
	}

	static StepKind stepKind(RouteWindow.Step s)
	{
		if ("auto_done".equals(s.status) || "manual_done".equals(s.status))
		{
			return StepKind.DONE;
		}
		if ("implied".equals(s.status))
		{
			return StepKind.IMPLIED;
		}
		if ("skipped".equals(s.status))
		{
			return StepKind.SKIPPED;
		}
		return s.blocked() ? StepKind.BLOCKED : StepKind.PENDING;
	}

	/** The word of a step kind, for the row's tooltip. */
	static String stepKindWord(StepKind k)
	{
		switch (k)
		{
			case DONE:
				return "feito";
			case IMPLIED:
				return "presumido (feito por um passo seguinte)";
			case SKIPPED:
				return "pulado";
			case BLOCKED:
				return "bloqueado";
			case PENDING:
			default:
				return "pendente";
		}
	}

	/** "Falta: Black Knights' Fortress", "Falta: 32 quest points +1"; empty when not blocked. */
	static String blocked(List<RouteWindow.Requirement> requirements)
	{
		if (requirements == null || requirements.isEmpty())
		{
			return "";
		}
		String text = "Falta: " + requirementLabel(requirements.get(0));
		return requirements.size() > 1 ? text + " +" + (requirements.size() - 1) : text;
	}

	/** Every requirement, for the blocked row's tooltip. */
	static String blockedAll(List<RouteWindow.Requirement> requirements)
	{
		List<String> labels = new ArrayList<>();
		for (RouteWindow.Requirement r : requirements)
		{
			labels.add(requirementLabel(r));
		}
		return "Falta: " + String.join(", ", labels);
	}

	private static String requirementLabel(RouteWindow.Requirement r)
	{
		return r.label == null || r.label.isEmpty() ? r.key : r.label;
	}

	/** The label in front of a note: "Atualiza\u00e7\u00e3o:" or "Dica:". */
	static String noteLabel(RouteWindow.Note n)
	{
		if (RouteWindow.Note.UPDATE.equals(n.kind))
		{
			return "Atualiza\u00e7\u00e3o:";
		}
		return "Dica:";
	}

	/** The marker of a collapsed row: an update note outranks a tip; null without notes. */
	static String noteMarker(List<RouteWindow.Note> notes)
	{
		String marker = null;
		for (RouteWindow.Note n : notes)
		{
			if (RouteWindow.Note.UPDATE.equals(n.kind))
			{
				return RouteWindow.Note.UPDATE;
			}
			marker = RouteWindow.Note.TIP;
		}
		return marker;
	}

	/** "2 Hammer", or the name alone when one is enough or the guide gives no quantity. */
	static String carryName(RouteWindow.CarryItem it)
	{
		String name = it.name == null ? "" : it.name;
		return it.qty > 1 ? it.qty + " " + name : name;
	}

	static ItemKind itemKind(RouteWindow.CarryItem it)
	{
		if ("met".equals(it.verdict))
		{
			return ItemKind.CARRIED;
		}
		if ("met".equals(it.owned))
		{
			return ItemKind.BANK;
		}
		if ("unmet".equals(it.verdict) && "unmet".equals(it.owned))
		{
			return ItemKind.MISSING;
		}
		return ItemKind.UNKNOWN;
	}

	/** Where an item to carry stands: carried, in the bank, missing or unknown. */
	static String carryWhere(RouteWindow.CarryItem it)
	{
		switch (itemKind(it))
		{
			case CARRIED:
				return "equipment".equals(it.slot) ? "equipado" : "com você";
			case BANK:
				return "no banco";
			case MISSING:
				return "falta";
			case UNKNOWN:
			default:
				return "sem dado";
		}
	}

	/** The items in the order that matters: missing, unknown, carried, in the bank. */
	static List<RouteWindow.CarryItem> carryOrdered(List<RouteWindow.CarryItem> items)
	{
		List<RouteWindow.CarryItem> out = new ArrayList<>(items.size());
		for (ItemKind k : ItemKind.values())
		{
			for (RouteWindow.CarryItem it : items)
			{
				if (itemKind(it) == k)
				{
					out.add(it);
				}
			}
		}
		return out;
	}

	/** "5 faltam · 2 com você · 19 no banco · 1 sem dado"; zero counts are left out. */
	static String carrySummary(List<RouteWindow.CarryItem> items)
	{
		int[] n = new int[ItemKind.values().length];
		for (RouteWindow.CarryItem it : items)
		{
			n[itemKind(it).ordinal()]++;
		}
		List<String> parts = new ArrayList<>();
		int missing = n[ItemKind.MISSING.ordinal()];
		if (missing > 0)
		{
			parts.add(missing + (missing == 1 ? " falta" : " faltam"));
		}
		if (n[ItemKind.CARRIED.ordinal()] > 0)
		{
			parts.add(n[ItemKind.CARRIED.ordinal()] + " com você");
		}
		if (n[ItemKind.BANK.ordinal()] > 0)
		{
			parts.add(n[ItemKind.BANK.ordinal()] + " no banco");
		}
		if (n[ItemKind.UNKNOWN.ordinal()] > 0)
		{
			parts.add(n[ItemKind.UNKNOWN.ordinal()] + " sem dado");
		}
		return parts.isEmpty() ? "nenhum item" : String.join(DOT, parts);
	}

	/** The cause of a failed call, in the owner's words. */
	static String error(int status, boolean write)
	{
		switch (status)
		{
			case 0:
				return "servidor fora do ar";
			case 400:
				return write ? "pedido recusado" : "pedido inválido";
			case 401:
				return "credencial recusada";
			case 403:
				return "o token não tem marks:write";
			case 404:
				return "conta ou passo não encontrado";
			case 409:
				return "o passo já está feito";
			case 422:
				return "várias contas: preencha Conta";
			default:
				return "erro HTTP " + status;
		}
	}

	/**
	 * Word-wraps {@code text} into lines no wider than {@code width} as measured by
	 * {@code measure}; explicit newlines are kept and a word longer than a line is cut.
	 */
	static List<String> wrap(String text, int width, ToIntFunction<String> measure)
	{
		List<String> lines = new ArrayList<>();
		if (text == null || text.isEmpty())
		{
			return lines;
		}
		for (String paragraph : text.split("\n", -1))
		{
			StringBuilder line = new StringBuilder();
			for (String word : paragraph.trim().split("\\s+"))
			{
				if (word.isEmpty())
				{
					continue;
				}
				String candidate = line.length() == 0 ? word : line + " " + word;
				if (measure.applyAsInt(candidate) <= width)
				{
					line.setLength(0);
					line.append(candidate);
					continue;
				}
				if (line.length() > 0)
				{
					lines.add(line.toString());
					line.setLength(0);
				}
				// a word wider than the line: cut it where it no longer fits
				while (word.length() > 1 && measure.applyAsInt(word) > width)
				{
					int cut = word.length() - 1;
					while (cut > 1 && measure.applyAsInt(word.substring(0, cut)) > width)
					{
						cut--;
					}
					lines.add(word.substring(0, cut));
					word = word.substring(cut);
				}
				line.append(word);
			}
			lines.add(line.toString());
		}
		return lines;
	}

	/** {@code text} as is when it fits, else cut with an ellipsis to fit {@code width}. */
	static String ellipsize(String text, int width, ToIntFunction<String> measure)
	{
		if (text == null)
		{
			return "";
		}
		if (measure.applyAsInt(text) <= width)
		{
			return text;
		}
		int end = text.length();
		while (end > 0 && measure.applyAsInt(text.substring(0, end).trim() + ELLIPSIS) > width)
		{
			end--;
		}
		return end == 0 ? ELLIPSIS : text.substring(0, end).trim() + ELLIPSIS;
	}

	/** "há 10 s", "há 3 min", "há 2 h". */
	static String ago(Instant at, Instant now)
	{
		long s = Math.max(0, Duration.between(at, now).getSeconds());
		if (s < 60)
		{
			return "há " + s + " s";
		}
		if (s < 3600)
		{
			return "há " + (s / 60) + " min";
		}
		return "há " + (s / 3600) + " h";
	}

	private static String since(Instant at, Instant now)
	{
		return at == null ? "" : DOT + ago(at, now);
	}
}
