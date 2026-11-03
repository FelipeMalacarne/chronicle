#!/usr/bin/env python3
"""
extract_quest_order.py - Parse data/raw/ironman-guide/pages/quests.md (output of extract_page.py for
https://ironman.guide/quests) into data/guide/normalized/quest-order.json:

  {"source": url, "phases": [{"number": 1, "title": "Immediate Priority", "description": str,
     "quests": [{"order": 1, "name": "Cook's Assistant", "qp": 1, "milestone": bool, "why": str,
                 "skill_reqs": str, "quest_prereqs": str, "key_rewards": str, "wiki": url}]}]}

The markdown is regular: "Phase N" / "## Title" / description, then per quest "### <order>. <name>",
"N QP" (optionally preceded by "Milestone"), a one-line reason, and three "Label  \\nvalue" pairs.
Run with `python3 -I tools/extract_quest_order.py`.
"""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = os.path.join(ROOT, "data", "raw", "ironman-guide", "pages", "quests.md")
OUT = os.path.join(ROOT, "data", "guide", "normalized", "quest-order.json")


def main():
    text = open(SRC, encoding="utf-8").read()
    blocks = [b.strip() for b in text.split("\n\n") if b.strip()]
    phases, cur_phase, cur_quest = [], None, None
    i = 0
    while i < len(blocks):
        b = blocks[i]
        m = re.fullmatch(r"Phase (\d+)", b)
        if m and i + 1 < len(blocks) and blocks[i + 1].startswith("## "):
            cur_phase = {"number": int(m.group(1)), "title": blocks[i + 1][3:].strip(), "description": "", "quests": []}
            if i + 2 < len(blocks) and not blocks[i + 2].startswith("#"):
                cur_phase["description"] = blocks[i + 2]
                i += 1
            phases.append(cur_phase)
            cur_quest = None
            i += 2
            continue
        if b.startswith("## ") and cur_phase is not None and not re.fullmatch(r"Phase \d+", blocks[i - 1]):
            cur_phase = None  # left the phase list (Quest Chain Reference, Top XP ...)
        m = re.fullmatch(r"### (\d+)\. (.+)", b)
        if m and cur_phase is not None:
            cur_quest = {"order": int(m.group(1)), "name": m.group(2).strip(), "qp": None, "milestone": False,
                         "why": "", "skill_reqs": "", "quest_prereqs": "", "key_rewards": "",
                         "wiki": "https://oldschool.runescape.wiki/w/" + m.group(2).strip().replace(" ", "_")}
            cur_phase["quests"].append(cur_quest)
            i += 1
            continue
        if cur_quest is not None:
            mq = re.fullmatch(r"(?:(Milestone)\s*)?(\d+) QP", b.replace("  \n", " ").strip())
            if mq:
                cur_quest["milestone"] = bool(mq.group(1))
                cur_quest["qp"] = int(mq.group(2))
            elif b == "Milestone":  # miniquests: no QP line
                cur_quest["milestone"] = True
                cur_quest["qp"] = 0
            elif b.startswith("Skill Reqs"):
                cur_quest["skill_reqs"] = b.split("\n", 1)[1].strip() if "\n" in b else ""
            elif b.startswith("Quest Prereqs"):
                cur_quest["quest_prereqs"] = b.split("\n", 1)[1].strip() if "\n" in b else ""
            elif b.startswith("Key Rewards"):
                cur_quest["key_rewards"] = b.split("\n", 1)[1].strip() if "\n" in b else ""
            elif not cur_quest["why"] and not b.startswith("#"):
                cur_quest["why"] = b
        i += 1
    out = {"source": "https://ironman.guide/quests", "attribution": "ironman.guide (OzirisLoL); data per OSRS Wiki", "phases": phases}
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    json.dump(out, open(OUT, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
    n = sum(len(p["quests"]) for p in phases)
    print(f"wrote {OUT}: {len(phases)} phases, {n} quests, milestones={sum(q['milestone'] for p in phases for q in p['quests'])}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
