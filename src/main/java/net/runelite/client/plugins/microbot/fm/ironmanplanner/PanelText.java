package net.runelite.client.plugins.microbot.fm.ironmanplanner;

import java.time.Duration;
import java.time.Instant;
import java.util.List;

/**
 * The panel's words (PT-BR, game terms in English) and glyphs. Verdicts and statuses get a
 * distinct glyph each, never only a colour. Pure: no Swing, testable.
 */
final class PanelText
{
	static final String GLYPH_MET = "\u2713";
	static final String GLYPH_UNMET = "\u2717";
	static final String GLYPH_UNKNOWN = "?";

	private PanelText()
	{
	}

	/** Glyph of an objective or item verdict: met, unmet, anything else is unknown. */
	static String verdictGlyph(String verdict)
	{
		if ("met".equals(verdict))
		{
			return GLYPH_MET;
		}
		if ("unmet".equals(verdict))
		{
			return GLYPH_UNMET;
		}
		return GLYPH_UNKNOWN;
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
			return "n\u00e3o atende";
		}
		return "n\u00e3o sabemos";
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

	/** One objective chip: glyph, label and, for requirements, the role. */
	static String chip(RouteWindow.Objective o)
	{
		String label = o.label == null || o.label.isEmpty() ? o.key : o.label;
		String text = verdictGlyph(o.verdict) + " " + label;
		if ("requires".equals(o.role) || "uses".equals(o.role))
		{
			text += " (" + role(o.role) + ")";
		}
		if (o.regressed && "met".equals(o.verdict))
		{
			text += " \u00b7 n\u00e3o vale mais";
		}
		return text;
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

	/** Glyph in front of a step of the list: done, skipped, blocked or open. */
	static String statusGlyph(RouteWindow.Step s)
	{
		if (s.status != null && (s.status.equals("auto_done") || s.status.equals("manual_done") || s.status.equals("implied")))
		{
			return GLYPH_MET;
		}
		if ("skipped".equals(s.status))
		{
			return "\u00bb";
		}
		return s.current ? "\u25b6" : "\u25cb";
	}

	/** "falta 48 Slayer", "falta Twilight's Promise +1"; empty when not blocked. */
	static String blocked(List<RouteWindow.Requirement> requirements)
	{
		if (requirements == null || requirements.isEmpty())
		{
			return "";
		}
		RouteWindow.Requirement first = requirements.get(0);
		String label = first.label == null || first.label.isEmpty() ? first.key : first.label;
		String text = "falta " + label;
		return requirements.size() > 1 ? text + " +" + (requirements.size() - 1) : text;
	}

	/** "2 Hammer", or the name alone when one is enough or the guide gives no quantity. */
	static String carryName(RouteWindow.CarryItem it)
	{
		String name = it.name == null ? "" : it.name;
		return it.qty > 1 ? it.qty + " " + name : name;
	}

	/** Where an item to carry stands: carried, in the bank, missing or unknown. */
	static String carryWhere(RouteWindow.CarryItem it)
	{
		if ("met".equals(it.verdict))
		{
			return "equipment".equals(it.slot) ? "equipado" : "na mochila";
		}
		if ("met".equals(it.owned))
		{
			return "no banco";
		}
		if ("unmet".equals(it.verdict) && "unmet".equals(it.owned))
		{
			return "falta";
		}
		return "n\u00e3o sabemos";
	}

	/** Glyph of an item to carry: carried, owned elsewhere, missing, unknown. */
	static String carryGlyph(RouteWindow.CarryItem it)
	{
		if ("met".equals(it.verdict))
		{
			return GLYPH_MET;
		}
		if ("met".equals(it.owned))
		{
			return "\u25cb";
		}
		if ("unmet".equals(it.verdict) && "unmet".equals(it.owned))
		{
			return GLYPH_UNMET;
		}
		return GLYPH_UNKNOWN;
	}

	/** "34 de 576 feitos \u00b7 2 bloqueados". */
	static String counts(RouteWindow.Counts c)
	{
		if (c == null || c.total == 0)
		{
			return "";
		}
		String text = c.done + " de " + c.total + " feitos";
		return c.blocked > 0 ? text + " \u00b7 " + c.blocked + (c.blocked == 1 ? " bloqueado" : " bloqueados") : text;
	}

	/** The connection line under the header. */
	static String connection(PanelState s, Instant now)
	{
		switch (s.connection)
		{
			case NOT_CONFIGURED:
				return "Configure a URL do planner";
			case CONNECTING:
				return "Conectando\u2026";
			case ONLINE:
				return "Conectado" + since(s.fetchedAt, now);
			case STALE:
				return "Desatualizado" + since(s.fetchedAt, now) + (s.message == null ? "" : " \u00b7 " + s.message);
			case OFFLINE:
			default:
				return "Sem conex\u00e3o" + (s.message == null ? "" : " \u00b7 " + s.message);
		}
	}

	/** The cause of a failed call, in the owner's words. */
	static String error(int status, boolean write)
	{
		switch (status)
		{
			case 0:
				return "servidor fora do ar";
			case 400:
				return write ? "pedido recusado" : "pedido inv\u00e1lido";
			case 401:
				return "credencial recusada";
			case 403:
				return "o token n\u00e3o tem marks:write";
			case 404:
				return "conta ou passo n\u00e3o encontrado";
			case 409:
				return "o passo j\u00e1 est\u00e1 feito";
			case 422:
				return "v\u00e1rias contas: preencha Conta";
			default:
				return "erro HTTP " + status;
		}
	}

	private static String since(Instant at, Instant now)
	{
		if (at == null)
		{
			return "";
		}
		long s = Math.max(0, Duration.between(at, now).getSeconds());
		if (s < 60)
		{
			return " \u00b7 h\u00e1 " + s + " s";
		}
		if (s < 3600)
		{
			return " \u00b7 h\u00e1 " + (s / 60) + " min";
		}
		return " \u00b7 h\u00e1 " + (s / 3600) + " h";
	}
}
