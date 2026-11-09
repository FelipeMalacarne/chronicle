#!/usr/bin/env python3
"""Generate docs/arquitetura/modelo-de-dados.dbml from a migrated PostgreSQL database.

Reads tables, columns, keys, foreign keys, indexes, checks and comments from the database pointed to
by DATABASE_URL (after `goose up`), takes inline `-- comments` of column definitions from
db/migrations/*.sql, and rewrites the "current" part of the DBML file. Everything after the
PLANNED_MARKER line is hand-written (tables planned in docs/09) and is preserved as is.

Usage: DATABASE_URL=postgres://... python3 tools/schema_to_dbml.py [output.dbml]
Needs only the standard library and `psql` on PATH.
"""

import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_OUT = ROOT / "docs" / "arquitetura" / "modelo-de-dados.dbml"
PLANNED_MARKER = "// ===== PLANEJADO (doc 09) ====="

GROUPS = {
    "identidade": ["users", "sessions", "api_tokens", "accounts", "settings"],
    "ingest_e_snapshots": [
        "plugin_ingest_tokens", "sync_runs", "snapshots", "snapshot_facts", "snapshot_item_containers",
    ],
    "estado_da_conta": ["observations", "player_state", "ingest_container_freshness"],
    "conteudo_indexado": ["guides", "content_versions", "content_steps", "step_id_aliases"],
    "progresso": [
        "objective_progress", "objective_progress_events", "mark_commands", "step_marks", "step_status_events",
        "route_cursor",
    ],
    "metas_e_caches": ["goals", "wiki_cache", "graph_cache"],
    "controle_microbot": ["microbot_audit"],
}

QUERY = r"""
WITH t AS (
  SELECT c.oid, c.relname
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'goose_db_version'
)
SELECT json_agg(json_build_object(
  'name', t.relname,
  'comment', obj_description(t.oid, 'pg_class'),
  'columns', (
    SELECT json_agg(json_build_object(
      'name', a.attname,
      'type', format_type(a.atttypid, a.atttypmod),
      'not_null', a.attnotnull,
      'default', pg_get_expr(d.adbin, d.adrelid),
      'comment', col_description(t.oid, a.attnum)
    ) ORDER BY a.attnum)
    FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
    WHERE a.attrelid = t.oid AND a.attnum > 0 AND NOT a.attisdropped
  ),
  'constraints', (
    SELECT json_agg(json_build_object(
      'name', co.conname, 'type', co.contype,
      'cols', (SELECT json_agg(att.attname ORDER BY k.ord)
               FROM unnest(co.conkey) WITH ORDINALITY k(num, ord)
               JOIN pg_attribute att ON att.attrelid = co.conrelid AND att.attnum = k.num),
      'ftable', co.confrelid::regclass::text,
      'fcols', (SELECT json_agg(att.attname ORDER BY k.ord)
                FROM unnest(co.confkey) WITH ORDINALITY k(num, ord)
                JOIN pg_attribute att ON att.attrelid = co.confrelid AND att.attnum = k.num),
      'on_delete', co.confdeltype,
      'def', pg_get_constraintdef(co.oid)
    ) ORDER BY co.conname)
    FROM pg_constraint co WHERE co.conrelid = t.oid
  ),
  'indexes', (
    SELECT json_agg(json_build_object('name', ic.relname, 'def', pg_get_indexdef(i.indexrelid),
                                      'unique', i.indisunique) ORDER BY ic.relname)
    FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
    WHERE i.indrelid = t.oid AND NOT i.indisprimary
      AND NOT EXISTS (SELECT 1 FROM pg_constraint u WHERE u.conindid = i.indexrelid AND u.contype = 'u')
  )
) ORDER BY t.relname)
FROM t;
"""

VIEWS_QUERY = r"""
SELECT json_agg(c.relname ORDER BY c.relname)
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind = 'v';
"""


def psql(url: str, sql: str):
    out = subprocess.run(["psql", url, "-X", "-At", "-c", sql], check=True, capture_output=True, text=True)
    return json.loads(out.stdout.strip() or "null")


def migration_comments() -> dict:
    """Map (table, column) -> inline comment from CREATE TABLE blocks in the migrations."""
    out = {}
    for path in sorted((ROOT / "db" / "migrations").glob("*.sql")):
        up = path.read_text().split("-- +goose Down")[0]
        for m in re.finditer(r"CREATE TABLE (\w+)\s*\((.*?)\n\);", up, re.S):
            table, body = m.group(1), m.group(2)
            for line in body.splitlines():
                cm = re.match(r"\s*(\w+)\s+[A-Za-z].*?--\s*(.+)$", line)
                if cm and cm.group(1).upper() not in ("PRIMARY", "UNIQUE", "CONSTRAINT", "CHECK"):
                    out[(table, cm.group(1))] = cm.group(2).strip()
        for m in re.finditer(r"ALTER TABLE (\w+)\s+ADD COLUMN (\w+)[^;\n]*;?\s*--\s*(.+)", up):
            out[(m.group(1), m.group(2))] = m.group(3).strip()
    return out


def q(text: str) -> str:
    return "'" + text.replace("\\", "\\\\").replace("'", "\\'") + "'"


def ident(name: str) -> str:
    return name if re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", name) else f'"{name}"'


def render(tables, views, comments) -> str:
    lines = [
        "// Modelo de dados do ironman-planner (PostgreSQL 17).",
        "// Parte atual GERADA por tools/schema_to_dbml.py a partir das migrations aplicadas;",
        "// não edite à mão acima do marcador PLANEJADO. Regenere com `just dbml`.",
        "// Abra em https://dbdiagram.io ou com qualquer ferramenta DBML.",
        "",
        "Project ironman_planner {",
        "  database_type: 'PostgreSQL'",
        "  Note: '''",
        "  Conteúdo dos guias vive em content/ (JSON versionado); o banco só o indexa (CLAUDE.md, regra 3).",
        "  Snapshots são append-only; player_state é o estado mesclado que o motor de regras lê.",
        "  Seções e regras de negócio: docs/04-arquitetura.md (seção 4) e docs/09-modelo-de-dominio.md.",
        "  '''",
        "}",
        "",
    ]
    refs = []
    for t in tables:
        cons = t["constraints"] or []
        pk = next((c for c in cons if c["type"] == "p"), None)
        pk_cols = pk["cols"] if pk else []
        uniques = [c for c in cons if c["type"] == "u"]
        single_unique = {c["cols"][0] for c in uniques if len(c["cols"]) == 1}
        checks = [c for c in cons if c["type"] == "c"]
        lines.append(f"Table {ident(t['name'])} {{")
        for col in t["columns"]:
            settings = []
            if len(pk_cols) == 1 and col["name"] == pk_cols[0]:
                settings.append("pk")
            elif col["not_null"]:
                settings.append("not null")
            if col["name"] in single_unique:
                settings.append("unique")
            if col["default"] is not None:
                settings.append(f"default: `{col['default']}`")
            note = col["comment"] or comments.get((t["name"], col["name"]))
            if note:
                settings.append(f"note: {q(note)}")
            tail = f" [{', '.join(settings)}]" if settings else ""
            lines.append(f"  {ident(col['name'])} {_type(col['type'])}{tail}")
        idx = []
        if len(pk_cols) > 1:
            idx.append(f"    ({', '.join(pk_cols)}) [pk]")
        for u in uniques:
            if len(u["cols"]) > 1:
                idx.append(f"    ({', '.join(u['cols'])}) [unique]")
        for i in t["indexes"] or []:
            m = re.search(r"USING \w+ \((.*)\)(?: WHERE (.*))?$", i["def"])
            if not m:
                continue
            expr = m.group(1)
            cols = [c.strip().replace('"', "") for c in expr.split(",")]
            simple = all(re.fullmatch(r"[a-z_]+(?: DESC)?", c) for c in cols)
            parts = [c.replace(" DESC", "") for c in cols]
            target = f"({', '.join(parts)})" if simple else f"`{expr}`"
            opts = [f"name: {q(i['name'])}"]
            if i["unique"]:
                opts.append("unique")
            if m.group(2):
                opts.append(f"note: {q('WHERE ' + m.group(2))}")
            idx.append(f"    {target} [{', '.join(opts)}]")
        if idx:
            lines.append("")
            lines.append("  indexes {")
            lines.extend(idx)
            lines.append("  }")
        notes = []
        if t["comment"]:
            notes.append(t["comment"])
        for c in checks:
            notes.append(f"{c['name']}: {c['def']}")
        if notes:
            lines.append("")
            lines.append("  Note: '''")
            for n in notes:
                lines.append("  " + n.replace("'''", "\\'\\'\\'"))
            lines.append("  '''")
        lines.append("}")
        lines.append("")
        for c in cons:
            if c["type"] != "f":
                continue
            ftable = c["ftable"].split(".")[-1].strip('"')
            left = f"{ident(t['name'])}.({', '.join(c['cols'])})" if len(c["cols"]) > 1 else f"{ident(t['name'])}.{c['cols'][0]}"
            right = f"{ident(ftable)}.({', '.join(c['fcols'])})" if len(c["fcols"]) > 1 else f"{ident(ftable)}.{c['fcols'][0]}"
            action = {"c": "cascade", "n": "set null", "r": "restrict", "d": "set default"}.get(c["on_delete"])
            tail = f" [delete: {action}]" if action else ""
            refs.append(f"Ref {c['name']}: {left} > {right}{tail}")
    lines.extend(refs)
    lines.append("")
    known = {t["name"] for t in tables}
    for group, members in GROUPS.items():
        present = [m for m in members if m in known]
        if present:
            lines.append(f"TableGroup {group} {{")
            lines.extend(f"  {m}" for m in present)
            lines.append("}")
            lines.append("")
    ungrouped = sorted(known - {m for ms in GROUPS.values() for m in ms})
    if ungrouped:
        lines.append(f"// Tabelas sem grupo (adicione em GROUPS no gerador): {', '.join(ungrouped)}")
    if views:
        lines.append(f"// Views (fora do DBML): {', '.join(views)}")
    lines.append("")
    return "\n".join(lines)


ALIASES = {"timestamp with time zone": "timestamptz", "timestamp without time zone": "timestamp"}


def _type(pg_type: str) -> str:
    pg_type = ALIASES.get(pg_type, pg_type)
    # DBML accepts types with spaces/parentheses when quoted.
    return pg_type if re.fullmatch(r"[A-Za-z0-9_\[\]]+", pg_type) else f'"{pg_type}"'


def main():
    url = os.environ.get("DATABASE_URL")
    if not url:
        sys.exit("DATABASE_URL is required (a database migrated with goose up)")
    out = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_OUT
    tables = psql(url, QUERY) or []
    views = psql(url, VIEWS_QUERY) or []
    text = render(tables, views, migration_comments())
    planned = ""
    if out.exists():
        old = out.read_text()
        if PLANNED_MARKER in old:
            planned = old[old.index(PLANNED_MARKER):]
    if not planned:
        planned = PLANNED_MARKER + "\n"
    out.write_text(text + "\n" + planned)
    print(f"wrote {out} ({len(tables)} tables)")


if __name__ == "__main__":
    main()
