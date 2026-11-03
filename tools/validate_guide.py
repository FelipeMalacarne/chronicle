#!/usr/bin/env python3
"""
validate_guide.py - Sanity checks for data/guide/normalized/guide.json.

    python3 -I tools/validate_guide.py [path/to/guide.json]

Checks
  * step ids are unique across the whole guide, and match the section prefix
  * every skill_level rule / prerequisite uses a canonical skill name and level 1..99
  * every quest "start" step has a matching "complete" step later (and is linked via completes_in/requires)
  * every quest_state rule names a quest that exists in the catalog (/quests page + referenced list)
  * requires[] / completes_in point at existing step ids
  * diary_tier rules use known areas/tiers, boss_kc rules use known hiscores boss names
  * complete_when is never empty; manual steps have exactly [{"type":"manual"}]
Exit code 1 on errors (warnings do not fail).
"""
from __future__ import annotations

import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from normalize_guide import SKILLS, DIARY_AREAS, BOSSES, load_quest_catalog, norm_key  # noqa: E402

TIERS = {"easy", "medium", "hard", "elite"}
RULE_TYPES = {"skill_level", "quest_state", "diary_tier", "boss_kc", "combat_achievement", "manual"}


def main(argv=None):
    path = (argv or sys.argv[1:] or [os.path.join(ROOT, "data", "guide", "normalized", "guide.json")])[0]
    g = json.load(open(path, encoding="utf-8"))
    errors, warnings = [], []
    quest_names, quest_lookup = load_quest_catalog()
    all_steps = [(sec, st) for sec in g["sections"] for st in sec["steps"]]
    ids = [st["id"] for _, st in all_steps]
    id_set = set(ids)

    # unique ids + prefix
    seen = set()
    for sec, st in all_steps:
        if st["id"] in seen:
            errors.append(f"duplicate step id {st['id']}")
        seen.add(st["id"])
        if not st["id"].startswith(sec["id"] + "."):
            errors.append(f"step {st['id']} does not carry section prefix {sec['id']}.")

    # rules
    for sec, st in all_steps:
        rules = st.get("complete_when") or []
        if not rules:
            errors.append(f"{st['id']}: empty complete_when")
        if any(r["type"] == "manual" for r in rules) and len(rules) != 1:
            errors.append(f"{st['id']}: manual mixed with other rules")
        for r in rules + st.get("prerequisites", []):
            t = r.get("type")
            if t not in RULE_TYPES:
                errors.append(f"{st['id']}: unknown rule type {t}")
            if t == "skill_level":
                if r.get("skill") not in SKILLS:
                    errors.append(f"{st['id']}: unknown skill {r.get('skill')!r}")
                if not isinstance(r.get("level"), int) or not 1 <= r["level"] <= 99:
                    errors.append(f"{st['id']}: bad level {r.get('level')!r}")
            elif t == "quest_state":
                if norm_key(r.get("quest", "")) not in quest_lookup:
                    warnings.append(f"{st['id']}: quest not in catalog: {r.get('quest')!r}")
                if r.get("state") not in ("FINISHED", "IN_PROGRESS"):
                    errors.append(f"{st['id']}: bad quest state {r.get('state')!r}")
            elif t == "diary_tier":
                if r.get("area") not in DIARY_AREAS and r.get("area") != "all":
                    errors.append(f"{st['id']}: unknown diary area {r.get('area')!r}")
                if r.get("tier") not in TIERS:
                    errors.append(f"{st['id']}: unknown diary tier {r.get('tier')!r}")
            elif t == "boss_kc":
                if r.get("boss") not in BOSSES:
                    errors.append(f"{st['id']}: unknown boss {r.get('boss')!r}")
                if not isinstance(r.get("kc"), int) or r["kc"] < 1:
                    errors.append(f"{st['id']}: bad kc {r.get('kc')!r}")
        for dep in st.get("requires", []):
            if dep not in id_set:
                errors.append(f"{st['id']}: requires unknown step {dep}")
        if st.get("completes_in") and st["completes_in"] not in id_set:
            errors.append(f"{st['id']}: completes_in unknown step {st['completes_in']}")
        if st.get("layer") not in ("classic", "enhanced"):
            errors.append(f"{st['id']}: bad layer {st.get('layer')!r}")
        if st.get("track_via") not in ("hiscores", "wikisync", "manual"):
            errors.append(f"{st['id']}: bad track_via")

    # quest start -> complete pairing
    starts = [(i, st) for i, (_, st) in enumerate(all_steps) if st.get("quest") and st["quest"].get("status") == "start"]
    completes = {}
    for i, (_, st) in enumerate(all_steps):
        for r in st.get("complete_when", []):
            if r["type"] == "quest_state" and r["state"] == "FINISHED":
                completes.setdefault(r["quest"], []).append((i, st["id"]))
    unpaired = []
    for i, st in starts:
        later = [cid for (j, cid) in completes.get(st["quest"]["name"], []) if j > i]
        if not later:
            unpaired.append(f"{st['id']} ({st['quest']['name']})")
        elif not st.get("completes_in"):
            errors.append(f"{st['id']}: start step not linked to its complete step")
    for u in unpaired:
        warnings.append(f"quest started but never completed in the guide: {u}")
    quests_in_guide = {st["quest"]["name"] for _, st in all_steps if st.get("quest")}
    unknown = sorted(q for q in quests_in_guide if norm_key(q) not in quest_lookup)
    for q in unknown:
        warnings.append(f"quest field not in catalog: {q!r}")

    # summary
    print(f"guide: {g['guide']['id']}  sections={len(g['sections'])}  steps={len(all_steps)}")
    print(f"quest start steps: {len(starts)}  (unpaired: {len(unpaired)})   quests referenced: {len(quests_in_guide)}  catalog size: {len(quest_names)}")
    print(f"errors: {len(errors)}  warnings: {len(warnings)}")
    for e in errors:
        print("ERROR  ", e)
    for w in warnings:
        print("WARN   ", w)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
