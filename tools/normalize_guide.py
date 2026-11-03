#!/usr/bin/env python3
"""
normalize_guide.py - Normalize the extracted ironman.guide "Efficiency guide" (OzirisLoL) into the
planner's step schema with machine-evaluable completion rules.

    python3 -I tools/normalize_guide.py            # writes data/guide/normalized/guide.json + stats.json
    python3 -I tools/normalize_guide.py --print    # also prints the review tables (markdown) to stdout

INPUT  : data/guide/_index.json, data/guide/<section>.json (raw extraction of the RSC props),
         data/guide/_quests-referenced.txt and, if present, data/raw/ironman-guide/pages/quests.md
         (canonical quest names: the 215 "### N. Name" headings of /quests).
OUTPUT : data/guide/normalized/guide.json, data/guide/normalized/stats.json

TARGET SHAPE (draft v0.1)
-------------------------
{
  "schema_version": "0.1",
  "guide": {"id": "oziris-efficiency", "title": ..., "source": {url, author, maintainer, extracted_at, license_note}},
  "sections": [
    {"id": "1.1", "slug": "early-game", "title": ..., "description": ..., "track": false,
     "related": [{"href","label"}],
     "steps": [
       {"id": "1.1.11",
        "text": "Light the 4 logs until 15 Firemaking",
        "location": "Lumbridge" | null,
        "note": str | null,           # OzirisLoL's inline note
        "update_note": str | null,    # ironman.guide maintainer's "this changed" note
        "items": [str],               # items to bring (raw strings from the source)
        "links": [{"type": "image|video|guide", "url", "text"}],
        "layer": "classic" | "enhanced",   # enhanced = step added by ironman.guide, not in the original guide
        "modes": ["ironman","hcim","uim","gim"],   # always all four; HCIM relevance is a *tag*
        "tags": ["hcim", "sailing", "new-quest", "phase:late", "quest", "skill", "diary-task", ...],
        "phase": "mid"|"late"|"endgame"|null,
        "quest": {"name", "status": "start"|"complete"|null, "wiki": url} | null,
        "complete_when": [rule, ...],  # ALL rules must hold (AND). [] never happens: manual gets [{"type":"manual"}]
        "prerequisites": [rule, ...],  # informational: levels/quests the text says you need *before* this step
        "requires": ["1.1.5"],         # explicit dependencies on other step ids (quest start -> complete pair);
                                        # sequential order is implicit and NOT repeated here
        "completes_in": "1.1.9" | null,# on a quest-start step: id of the matching quest-complete step
        "track_via": "hiscores"|"wikisync"|"manual"  # convenience: cheapest data source that can evaluate ALL rules
       }
     ]}
  ]
}

RULE TYPES
----------
  {"type":"skill_level", "skill":"firemaking", "level":15}
  {"type":"quest_state", "quest":"Sea Slug", "state":"FINISHED"}
  {"type":"quest_state", "quest":"Sea Slug", "state":"IN_PROGRESS", "or_finished":true}
  {"type":"diary_tier",  "area":"ardougne", "tier":"easy"}            # area "all" = every area
  {"type":"boss_kc",     "boss":"Wintertodt", "kc":50}                # hiscores boss names
  {"type":"manual"}
Every rule carries "source": "skillGoal"|"quest"|"questStatus"|"text" and, when it came from the free
text, "inferred": true and "confidence": "high"|"medium" plus "match": the phrase that triggered it.

Skill names are the lowercase canonical list below; quest names are canonicalised against the /quests
page when possible (with a short alias table for OzirisLoL's shorthand).
"""
from __future__ import annotations

import argparse
import collections
import datetime as _dt
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GUIDE_DIR = os.path.join(ROOT, "data", "guide")
OUT_DIR = os.path.join(GUIDE_DIR, "normalized")
QUESTS_MD = os.path.join(ROOT, "data", "raw", "ironman-guide", "pages", "quests.md")

SKILLS = [
    "attack", "strength", "defence", "ranged", "prayer", "magic", "runecraft", "construction", "hitpoints",
    "agility", "herblore", "thieving", "crafting", "fletching", "slayer", "hunter", "mining", "smithing",
    "fishing", "cooking", "firemaking", "woodcutting", "farming", "sailing",
]
SKILL_ALIASES = {
    "att": "attack", "atk": "attack", "str": "strength", "def": "defence", "defense": "defence",
    "range": "ranged", "rng": "ranged", "pray": "prayer", "mage": "magic", "rc": "runecraft",
    "runecrafting": "runecraft", "con": "construction", "cons": "construction", "hp": "hitpoints",
    "agil": "agility", "herb": "herblore", "thiev": "thieving", "craft": "crafting", "fletch": "fletching",
    "slay": "slayer", "hunt": "hunter", "smith": "smithing", "fish": "fishing", "cook": "cooking",
    "fm": "firemaking", "wc": "woodcutting", "farm": "farming", "sail": "sailing", "mine": "mining",
}
# abbreviations get "medium" confidence when they come from free text
SHORT_ALIASES = {k for k in SKILL_ALIASES if len(k) <= 5} | {"range", "craft", "fletch", "smith"}
_skill_words = sorted(set(SKILLS) | set(SKILL_ALIASES), key=len, reverse=True)
SKILL_RE = "(?:" + "|".join(re.escape(w) for w in _skill_words) + ")"

DIARY_AREAS = {
    "ardougne": ["ardougne", "ardy"], "desert": ["desert"], "falador": ["falador", "fally"],
    "fremennik": ["fremennik", "fremmy", "frem", "fremenik"], "kandarin": ["kandarin", "kand"],
    "karamja": ["karamja"], "kourend": ["kourend & kebos", "kourend and kebos", "kourend", "kebos"],
    "lumbridge": ["lumbridge & draynor", "lumbridge and draynor", "lumbridge", "lumb"],
    "morytania": ["morytania", "mory"], "varrock": ["varrock"],
    "western": ["western provinces", "western"], "wilderness": ["wilderness", "wildy"],
}
AREA_LOOKUP = {alias: canon for canon, aliases in DIARY_AREAS.items() for alias in aliases}
AREA_RE = "(?:" + "|".join(re.escape(a) for a in sorted(AREA_LOOKUP, key=len, reverse=True)) + ")"
TIER_RE = "(?:easy|medium|med|hard|elite)"
CLOAK_TIER = {"1": "easy", "2": "medium", "3": "hard", "4": "elite"}

# hiscores boss names (canonical) -> aliases used in the guide text
BOSSES = {
    "Abyssal Sire": ["abyssal sire", "sire"], "Alchemical Hydra": ["alchemical hydra", "hydra"],
    "Amoxliatl": ["amoxliatl"], "Araxxor": ["araxxor"], "Artio": ["artio"], "Barrows Chests": ["barrows"],
    "Bryophyta": ["bryophyta"], "Callisto": ["callisto"], "Calvar'ion": ["calvarion", "calvar'ion"],
    "Cerberus": ["cerberus", "cerb"], "Chambers of Xeric": ["chambers of xeric", "cox"],
    "Chaos Elemental": ["chaos elemental"], "Chaos Fanatic": ["chaos fanatic"],
    "Commander Zilyana": ["commander zilyana", "zilyana", "saradomin gwd"], "Corporeal Beast": ["corporeal beast", "corp"],
    "Crazy Archaeologist": ["crazy archaeologist"], "Dagannoth Prime": ["dagannoth prime", "prime"],
    "Dagannoth Rex": ["dagannoth rex", "rex"], "Dagannoth Supreme": ["dagannoth supreme", "supreme"],
    "Deranged Archaeologist": ["deranged archaeologist"], "Doom of Mokhaiotl": ["doom of mokhaiotl", "doom"],
    "Duke Sucellus": ["duke sucellus", "duke"], "General Graardor": ["general graardor", "graardor", "bandos gwd"],
    "Giant Mole": ["giant mole", "mole"], "Grotesque Guardians": ["grotesque guardians", "ggs"],
    "Hespori": ["hespori"], "Kalphite Queen": ["kalphite queen", "kq"], "King Black Dragon": ["king black dragon", "kbd"],
    "Kraken": ["kraken"], "Kree'Arra": ["kree'arra", "kreearra", "kree", "armadyl gwd"],
    "K'ril Tsutsaroth": ["k'ril tsutsaroth", "kril", "zamorak gwd"], "Lunar Chests": ["lunar chests", "moons of peril", "perilous moons boss"],
    "Mimic": ["mimic"], "Nex": ["nex"], "Nightmare": ["nightmare"], "Phosani's Nightmare": ["phosani's nightmare", "phosani"],
    "Obor": ["obor"], "Phantom Muspah": ["phantom muspah", "muspah"], "Sarachnis": ["sarachnis"], "Scorpia": ["scorpia"],
    "Scurrius": ["scurrius"], "Skotizo": ["skotizo"], "Sol Heredit": ["sol heredit", "colosseum"], "Spindel": ["spindel"],
    "Tempoross": ["tempoross"], "The Gauntlet": ["the gauntlet", "gauntlet"], "The Corrupted Gauntlet": ["corrupted gauntlet", "cg"],
    "The Hueycoatl": ["hueycoatl"], "The Leviathan": ["leviathan"], "The Royal Titans": ["royal titans"],
    "The Whisperer": ["whisperer"], "Theatre of Blood": ["theatre of blood", "tob"],
    "Thermonuclear Smoke Devil": ["thermonuclear smoke devil", "thermy"], "Tombs of Amascut": ["tombs of amascut", "toa"],
    "TzKal-Zuk": ["tzkal-zuk", "zuk", "inferno"], "TzTok-Jad": ["tztok-jad", "jad", "fight caves", "fight cave"],
    "Vardorvis": ["vardorvis", "vard"], "Venenatis": ["venenatis"], "Vet'ion": ["vet'ion", "vetion"], "Vorkath": ["vorkath", "vork"],
    "Wintertodt": ["wintertodt", "wt"], "Yama": ["yama"], "Zalcano": ["zalcano"], "Zulrah": ["zulrah"],
}
BOSS_LOOKUP = {a: canon for canon, aliases in BOSSES.items() for a in aliases}
# very short aliases only count when a number is attached (avoid "cg" in random words etc.)
BOSS_RE = "(?:" + "|".join(re.escape(a) for a in sorted(BOSS_LOOKUP, key=len, reverse=True)) + ")"

QUEST_ALIASES = {  # OzirisLoL shorthand -> canonical (normalised keys, see norm_key)
    "lunars diplomacy": "Lunar Diplomacy", "vampire slayer": "Vampyre Slayer", "lost tribe": "The Lost Tribe",
    "grand tree": "The Grand Tree", "x marks the spot": "X Marks the Spot", "sea slug": "Sea Slug",
    "fairytale 1": "Fairytale I - Growing Pains", "fairytale 2": "Fairytale II - Cure a Queen",
    "fairy tale 1": "Fairytale I - Growing Pains", "fairy tale 2": "Fairytale II - Cure a Queen",
    "ds1": "Dragon Slayer I", "ds2": "Dragon Slayer II", "mm1": "Monkey Madness I", "mm2": "Monkey Madness II",
    "rfd": "Recipe for Disaster", "wgs": "While Guthix Sleeps", "dt1": "Desert Treasure I", "dt2": "Desert Treasure II",
    "desert treasure": "Desert Treasure I", "dragon slayer": "Dragon Slayer I", "monkey madness": "Monkey Madness I",
    "sote": "Song of the Elves", "mep1": "Mourning's End Part I", "mep2": "Mourning's End Part II",
    "another slice of ham": "Another Slice of H.A.M.", "icthlarins little helper": "Icthlarin's Little Helper",
    "a souls bane": "A Soul's Bane", "eagles peak": "Eagles' Peak", "contact": "Contact!",
    "abyss miniquest": "Enter the Abyss", "the abyss": "Enter the Abyss", "barcrawl": "Alfred Grimhand's Barcrawl",
    "recipe for disaster evil dave": "Recipe for Disaster/Freeing Evil Dave",
    "vampire slayer": "Vampyre Slayer", "desert treasure ii": "Desert Treasure II - The Fallen Empire",
    "forgettable tale": "Forgettable Tale of a Drunken Dwarf", "garden of tranquility": "Garden of Tranquillity",
    "rag and bone man": "Rag and Bone Man I", "fairytale2": "Fairytale II - Cure a Queen",
    "dragon slayer 1": "Dragon Slayer I", "dragon slayer 2": "Dragon Slayer II", "desert treasure 1": "Desert Treasure I",
    "desert treasure 2": "Desert Treasure II - The Fallen Empire", "monkey madness 1": "Monkey Madness I",
    "monkey madness 2": "Monkey Madness II", "rag and bone man 1": "Rag and Bone Man I", "rag and bone man 2": "Rag and Bone Man II",
    "big chompy bird hunt": "Big Chompy Bird Hunting", "sleeping giants quest": "Sleeping Giants",
    "fairytale 2": "Fairytale II - Cure a Queen", "legends quest": "Legends' Quest", "legend s quest": "Legends' Quest",
}

GOAL_VERB = r"(?:until|till|get|getting|reach|reaching|hit|train|training|finish|finishing|achieve|have|for|to)"
REQ_BEFORE = re.compile(r"(?:req\w*|need\w*|after|when you|once you|if you|when you reach|when you(?:'re| are)|once you(?:'re| have| are)|if you have|if you(?:'re| are)|now that you(?:'re| are)|plus|also|should be(?: at)?|you have|at|with)\s*$", re.I)
APPROX_BEFORE = re.compile(r"(?:around|about|roughly|~|like|close to)\s*$", re.I)


# ----------------------------------------------------------------------------- helpers

def norm_key(s: str) -> str:
    s = s.lower().replace("&", "and").replace("’", "'")
    s = re.sub(r"[^a-z0-9 ]+", " ", s)
    s = re.sub(r"\s+", " ", s).strip()
    s = re.sub(r"^the ", "", s)
    return s


def canon_skill(word: str) -> str:
    w = word.lower()
    return SKILL_ALIASES.get(w, w)


def in_parens(text: str, pos: int) -> bool:
    return text.count("(", 0, pos) > text.count(")", 0, pos)


def wiki_url(name: str) -> str:
    return "https://oldschool.runescape.wiki/w/" + name.replace(" ", "_")


def load_quest_catalog() -> tuple[set[str], dict[str, str]]:
    names: set[str] = set()
    ref = os.path.join(GUIDE_DIR, "_quests-referenced.txt")
    if os.path.exists(ref):
        names |= {ln.strip() for ln in open(ref, encoding="utf-8") if ln.strip()}
    if os.path.exists(QUESTS_MD):
        for ln in open(QUESTS_MD, encoding="utf-8"):
            m = re.match(r"^### \d+\. (.+?)\s*$", ln)
            if m:
                names.add(m.group(1).strip())
    # "(miniquest)" suffix on the /quests page is presentation; wiki page names (and WikiSync) omit it
    names = {re.sub(r"\s*\(miniquest\)$", "", n) for n in names}
    lookup = {norm_key(n): n for n in names}
    for k, v in QUEST_ALIASES.items():  # aliases win over the guide's own spellings
        lookup[norm_key(k)] = v
        names.add(v)
    # drop guide spellings that an alias redirects elsewhere
    for k, v in list(lookup.items()):
        if v in QUEST_ALIASES.values() and v not in names:
            del lookup[k]
    return names, lookup


# ----------------------------------------------------------------------------- text parsers

def parse_skill_levels(text: str):
    """Return (goals, prereqs): lists of (skill, level, confidence, phrase)."""
    goals, prereqs = [], []
    seen = set()

    def add(bucket, skill_word, level, phrase, conf):
        skill = canon_skill(skill_word)
        level = int(level)
        if skill not in SKILLS or not 1 <= level <= 99:
            return
        key = (skill, level, id(bucket))
        if key in seen:
            return
        seen.add(key)
        if skill_word.lower() in SHORT_ALIASES and conf == "high":
            conf = "medium"
        bucket.append((skill, level, conf, phrase))

    # "<verb> [level] N <skill>" (+ "and N <skill>" / "and <skill>" chains)
    pat = re.compile(rf"\b({GOAL_VERB})\s+(?:level\s+|lvl\s+)?(\d{{1,2}})\s*\+?\s+({SKILL_RE})\b", re.I)
    for m in pat.finditer(text):
        verb, level, skill = m.group(1).lower(), m.group(2), m.group(3)
        before = text[: m.start()]
        bucket = goals
        conf = "high" if verb in ("until", "till", "get", "getting", "reach", "train", "training", "finish", "finishing") else "medium"
        if APPROX_BEFORE.search(before) or re.match(r"(?i)\s*(?:xp|exp)\b", text[m.end():]):
            continue
        if in_parens(text, m.start()) or REQ_BEFORE.search(before) or verb in ("have",) and re.search(r"if you", before, re.I):
            bucket, conf = prereqs, "medium"
        add(bucket, skill, level, m.group(0), conf)
        # chains: "and 18 slayer", ", 65 str", "and Slayer" (same level)
        tail = text[m.end():]
        for c in re.finditer(rf"^\s*(?:,|and|&)\s*(?:(\d{{1,2}})\s+)?({SKILL_RE})\b", tail, re.I):
            add(bucket, c.group(2), c.group(1) or level, c.group(0).strip(), conf)
            tail = tail[c.end():]
            break
        while True:
            c = re.match(rf"\s*(?:,|and|&)\s*(?:(\d{{1,2}})\s+)?({SKILL_RE})\b", tail, re.I)
            if not c:
                break
            add(bucket, c.group(2), c.group(1) or level, c.group(0).strip(), conf)
            tail = tail[c.end():]
    # "do/doing <skill> ... until level N"  (skill before, level after)
    for m in re.finditer(rf"\b(?:do|doing|train|training|keep doing)\s+({SKILL_RE})\b[^.;]*?\buntil\s+(?:level|lvl)\s+(\d{{1,2}})\b", text, re.I):
        add(goals, m.group(1), m.group(2), m.group(0), "high")
    # "<skill> until/to [level] N"
    for m in re.finditer(rf"\b({SKILL_RE})\s+(?:until|to)\s+(?:level\s+|lvl\s+)?(\d{{1,2}})\b(?!\s*\+?\s*{SKILL_RE}\b)", text, re.I):
        if not in_parens(text, m.start()):
            add(goals, m.group(1), m.group(2), m.group(0), "medium")
    # requirement phrasing anywhere: "(... 46 Hunter, 30 Herblore ...)", "Req 42 slayer", "at 58 slayer:"
    for m in re.finditer(rf"\b(\d{{1,2}})\s+({SKILL_RE})\b", text, re.I):
        before = text[: m.start()]
        if in_parens(text, m.start()) or REQ_BEFORE.search(before):
            if not any(p[3] == m.group(0) for p in goals):
                add(prereqs, m.group(2), m.group(1), m.group(0), "medium")
    for m in re.finditer(rf"\b({SKILL_RE})\s+(\d{{1,2}})\b", text, re.I):
        before = text[: m.start()]
        if m.group(1).lower() in SHORT_ALIASES:
            continue
        if in_parens(text, m.start()) or re.search(r"\b(?:at|needs?|requires?|req)\s*$", before, re.I):
            add(prereqs, m.group(1), m.group(2), m.group(0), "medium")
    return goals, prereqs


def parse_diaries(text: str):
    out = []
    t = text
    for m in re.finditer(rf"\b({AREA_RE})\s+({TIER_RE})\s+(?:diary|diaries|tasks?)\b", t, re.I):
        out.append((AREA_LOOKUP[m.group(1).lower()], m.group(2).lower().replace("med", "medium") if m.group(2).lower() == "med" else m.group(2).lower(), "high", m.group(0)))
    for m in re.finditer(rf"\b({TIER_RE})\s+({AREA_RE})\s+(?:diary|diaries)\b", t, re.I):
        out.append((AREA_LOOKUP[m.group(2).lower()], m.group(1).lower(), "high", m.group(0)))
    for m in re.finditer(r"\b(?:ardy|ardougne)\s+cloak\s*([1-4])\b", t, re.I):
        out.append(("ardougne", CLOAK_TIER[m.group(1)], "high", m.group(0)))
    m = re.search(rf"\ball\s+({TIER_RE})(?:\s+and\s+({TIER_RE}))?\s+diaries\b", t, re.I)
    if m:
        for tier in (m.group(1), m.group(2)):
            if tier:
                out.append(("all", tier.lower(), "medium", m.group(0)))
    dedup = {}
    for a, tier, conf, phrase in out:
        dedup.setdefault((a, tier), (conf, phrase))
    return [(a, t, c, p) for (a, t), (c, p) in dedup.items()]


def parse_boss_kc(text: str):
    """Return (rules, mentions): rules are (boss, kc, conf, phrase); mentions are boss names without a count."""
    rules, mentions = [], []
    for m in re.finditer(rf"\b(?:kill|do|camp|farm|get|complete)\s+(\d{{1,4}})\s*(?:x\s*)?({BOSS_RE})\b", text, re.I):
        rules.append((BOSS_LOOKUP[m.group(2).lower()], int(m.group(1)), "high", m.group(0)))
    for m in re.finditer(rf"\b(\d{{1,4}})\s*(?:kc|kills?|chests?|runs?|completions?)\s+(?:at|of|on|in)?\s*(?:the\s+)?({BOSS_RE})\b", text, re.I):
        rules.append((BOSS_LOOKUP[m.group(2).lower()], int(m.group(1)), "high", m.group(0)))
    for m in re.finditer(rf"\b({BOSS_RE})\s+(?:until|to)\s+(\d{{1,4}})\s*(?:kc|kills)\b", text, re.I):
        rules.append((BOSS_LOOKUP[m.group(1).lower()], int(m.group(2)), "high", m.group(0)))
    for m in re.finditer(rf"\b({BOSS_RE})\b", text, re.I):
        alias = m.group(1).lower()
        if len(alias) <= 4 and alias not in ("jad", "zuk", "nex", "kq", "kbd", "rex", "corp", "mole"):
            continue  # "cg", "wt", "tob"... only count with a number
        boss = BOSS_LOOKUP[alias]
        if boss not in [r[0] for r in rules] and boss not in mentions:
            mentions.append(boss)
    return rules, mentions


def parse_quests_from_text(text: str, lookup: dict[str, str]):
    """Quest names the text says to complete/start. Returns list of (name, state, conf, phrase)."""
    out = []
    nt = norm_key(text)
    keys = sorted(lookup, key=len, reverse=True)
    # "Do these quests ...: A, B, C"
    m = re.search(r"quests?[^:]*:\s*(.+)$", text, re.I)
    if m:
        parts = []
        tail = re.sub(r"\([^)]*\)", "", m.group(1))  # drop "(first part, ...)" remarks before splitting
        for part in tail.split(","):
            part = part.strip()
            if not part:
                continue
            if norm_key(re.sub(r"^(?i:start|begin)\s+", "", part)) in lookup or " and " not in part:
                parts.append(part)
            else:
                parts.extend(p.strip() for p in part.split(" and "))
        for part in parts:
            state = "FINISHED"
            if re.match(r"(?i)^(start|begin)\s+", part):
                state, part = "IN_PROGRESS", re.sub(r"(?i)^(start|begin)\s+", "", part)
            k = norm_key(part)
            k = re.sub(r"\b(quest|miniquest)$", "", k).strip()
            if k in lookup and not any(o[0] == lookup[k] for o in out):
                out.append((lookup[k], state, "medium", part.strip()))
    for k in keys:
        if len(k) < 6:
            continue
        # not followed by a sequel marker ("dragon slayer 2", "desert treasure ii")
        for mm in re.finditer(rf"\b(complete|finish|do|start|begin|continue)\s+(?:the\s+)?{re.escape(k)}\b(?!\s+(?:\d+|i{{1,3}}|iv|v)\b)", nt):
            name = lookup[k]
            if any(o[0] == name for o in out):
                continue
            verb = mm.group(1)
            state = "IN_PROGRESS" if verb in ("start", "begin", "continue") else "FINISHED"
            if re.search(r"\bsubquests?\b|\buntil (?:the part|you)\b", text, re.I):
                state = "IN_PROGRESS"
            out.append((name, state, "medium", mm.group(0)))
    return out


# ----------------------------------------------------------------------------- normalisation

def rule(**kw):
    return {k: v for k, v in kw.items() if v is not None}


def normalize_step(raw: dict, section_id: str, quest_lookup: dict[str, str], unknown_quests: set) -> dict:
    text = raw["text"]
    tags: list[str] = []
    rules: list[dict] = []
    prereqs: list[dict] = []

    # --- fielded data
    if raw.get("hcim"):
        tags.append("hcim")
    if raw.get("sailingRelated") or section_id == "S":
        tags.append("sailing")
    if raw.get("newQuest"):
        tags.append("new-quest")
    if raw.get("phase"):
        tags.append("phase:" + raw["phase"])
    if re.search(r"\bHCIM\b", text) and "hcim" not in tags:
        tags.append("hcim")
    if re.search(r"\bUIM\b", text):
        tags.append("uim")

    quest = None
    if raw.get("quest"):
        qname = raw["quest"]
        canon = quest_lookup.get(norm_key(qname), qname)
        if norm_key(qname) not in quest_lookup:
            unknown_quests.add(qname)
        status = raw.get("questStatus")
        quest = {"name": canon, "status": status, "wiki": wiki_url(canon)}
        tags.append("quest")
        if status == "complete":
            rules.append(rule(type="quest_state", quest=canon, state="FINISHED", source="questStatus", confidence="high"))
        elif status == "start":
            rules.append(rule(type="quest_state", quest=canon, state="IN_PROGRESS", or_finished=True, source="questStatus", confidence="high"))
        else:
            # quest named but no status: treat as "must be finished" only if the text says so
            if re.search(r"\bsubquests?\b|\buntil (?:the part|you)\b", text, re.I):
                rules.append(rule(type="quest_state", quest=canon, state="IN_PROGRESS", or_finished=True, source="quest", inferred=True, confidence="medium", match="subquests/partial"))
            elif re.search(r"\b(complete|finish|do)\b", text, re.I):
                rules.append(rule(type="quest_state", quest=canon, state="FINISHED", source="quest", inferred=True, confidence="medium"))

    if raw.get("skillGoal"):
        sg = raw["skillGoal"]
        sk = canon_skill(sg["skill"])
        rules.append(rule(type="skill_level", skill=sk, level=int(sg["level"]), source="skillGoal", confidence="high"))
        tags.append("skill")

    # --- text inference
    goals, text_prereqs = parse_skill_levels(text)
    for skill, level, conf, phrase in goals:
        if any(r["type"] == "skill_level" and r["skill"] == skill for r in rules):
            continue  # skillGoal already covers this skill (trust the fielded value)
        rules.append(rule(type="skill_level", skill=skill, level=level, source="text", inferred=True, confidence=conf, match=phrase))
        if "skill" not in tags:
            tags.append("skill")
    for skill, level, conf, phrase in text_prereqs:
        if any(r["type"] == "skill_level" and r["skill"] == skill and r["level"] == level for r in rules):
            continue
        prereqs.append(rule(type="skill_level", skill=skill, level=level, source="text", inferred=True, confidence=conf, match=phrase))

    for area, tier, conf, phrase in parse_diaries(text):
        if not re.search(r"\b(complete|do|finish|get|off)\b", text, re.I):
            conf = "medium"
        if re.search(r"\bfor (?:a |the )?(?:\w+ )?diary\b", text, re.I) and "cloak" not in phrase.lower():
            tags.append("diary-task")  # "mine 2 blurite, 1 is for falador diary": a task, not the tier
            continue
        rules.append(rule(type="diary_tier", area=area, tier=tier, source="text", inferred=True, confidence=conf, match=phrase))
        tags.append("diary")
    if re.search(r"\bdiar(?:y|ies)\b", text, re.I) and "diary" not in tags and "diary-task" not in tags:
        tags.append("diary-task")

    boss_rules, boss_mentions = parse_boss_kc(text)
    for boss, kc, conf, phrase in boss_rules:
        rules.append(rule(type="boss_kc", boss=boss, kc=kc, source="text", inferred=True, confidence=conf, match=phrase))
        tags.append("boss")
    for boss in boss_mentions:
        tags.append("boss:" + boss)

    for name, state, conf, phrase in parse_quests_from_text(text, quest_lookup):
        if any(r["type"] == "quest_state" and r["quest"] == name for r in rules):
            continue
        if quest is not None and name == quest["name"]:
            continue
        if True:
            if state == "FINISHED":
                rules.append(rule(type="quest_state", quest=name, state="FINISHED", source="text", inferred=True, confidence=conf, match=phrase))
            else:
                rules.append(rule(type="quest_state", quest=name, state="IN_PROGRESS", or_finished=True, source="text", inferred=True, confidence=conf, match=phrase))
            if "quest" not in tags:
                tags.append("quest")

    if not rules:
        rules = [{"type": "manual"}]

    links = [{"type": l.get("type", "link"), "url": l.get("url"), "text": l.get("text")} for l in raw.get("links", [])]

    kinds = {r["type"] for r in rules}
    if kinds <= {"skill_level", "boss_kc"}:
        track_via = "hiscores"
    elif kinds <= {"skill_level", "boss_kc", "quest_state", "diary_tier", "combat_achievement"}:
        track_via = "wikisync"
    else:
        track_via = "manual"

    return {
        "id": raw["id"],
        "text": text,
        "location": raw.get("location"),
        "note": raw.get("note"),
        "update_note": raw.get("updateNote"),
        "items": list(raw.get("items", [])),
        "links": links,
        "layer": "enhanced" if raw.get("enhanced") else "classic",
        "modes": ["ironman", "hcim", "uim", "gim"],
        "tags": sorted(set(tags), key=tags.index),
        "phase": raw.get("phase"),
        "quest": quest,
        "complete_when": rules,
        "prerequisites": prereqs,
        "requires": [],
        "completes_in": None,
        "track_via": track_via,
    }


def link_quest_pairs(sections: list[dict]) -> int:
    """quest-start step -> matching quest-complete step (first later step with same quest & status complete)."""
    all_steps = [(s, st) for s in sections for st in s["steps"]]
    linked = 0
    for i, (_, st) in enumerate(all_steps):
        q = st.get("quest")
        if not q or q.get("status") != "start":
            continue
        for _, later in all_steps[i + 1:]:
            finished = any(r["type"] == "quest_state" and r["quest"] == q["name"] and r["state"] == "FINISHED"
                           for r in later["complete_when"])
            if finished:
                st["completes_in"] = later["id"]
                if st["id"] not in later["requires"]:
                    later["requires"].append(st["id"])
                linked += 1
                break
    return linked


# ----------------------------------------------------------------------------- stats / review

HISCORE_TYPES = {"skill_level", "boss_kc"}
WIKISYNC_TYPES = {"quest_state", "diary_tier", "combat_achievement"}


def suggest_for_manual(step: dict):
    """Heuristic: does a manual step *look* trackable? Return (score, suggestion) or None."""
    t = step["text"]
    tl = t.lower()
    sugg, score = [], 0
    for tag in step["tags"]:
        if tag.startswith("boss:"):
            if re.search(r"\b(farm|camp|kill|do|until|masses|for)\b", tl) and not re.search(r"\b(make your way|from wintertodt|for wintertodt)\b", tl):
                sugg.append(f"boss_kc {tag[5:]} (sem contagem no texto; definir kc alvo)")
                score += 3
    if "diary-task" in step["tags"]:
        sugg.append("diary_task (tarefa individual de diário; WikiSync expõe tarefas por área)")
        score += 2
    m = re.search(r"\bkill\s+(\d+|a|an|the)?\s*([a-z' -]+?)(?:\s+for|\s+with|,|$)", tl)
    if m and "boss" not in " ".join(step["tags"]):
        sugg.append(f"kill_count '{m.group(2).strip()}' (não existe no hiscores; só com plugin próprio)")
        score += 1
    if re.search(r"\buntil you (?:have|get)\b|\bbanked\b|\bget (?:a|an|the|\d+)\s+[a-z]", tl):
        sugg.append("item_obtained / item_count (inventário ou banco via plugin; collection log se for drop)")
        score += 2
    if re.search(r"\bgraceful|cape\b|outfit|unlock", tl):
        sugg.append("item/varbit unlock (plugin ou marcação manual)")
        score += 2
    if re.search(r"(?<![+\-])\b\d{1,2}\s+(?:" + "|".join(SKILLS) + r")\b(?!\s+(?:rods?|xp|boost|pots?|potions?))", tl) and not step["prerequisites"]:
        sugg.append("skill_level? (há 'N skill' no texto sem verbo de meta claro)")
        score += 3
    if step["prerequisites"]:
        sugg.append("prerequisites presentes: " + ", ".join(f"{p['skill']} {p['level']}" for p in step["prerequisites"] if p["type"] == "skill_level"))
        score += 1
    if re.search(r"\b(quest|miniquest)\b", tl) and "quest" not in step["tags"]:
        sugg.append("quest_state? (menciona quest sem nome reconhecido)")
        score += 2
    if "barcrawl" in tl and "quest" not in step["tags"]:
        sugg.insert(0, "sub-passo do Alfred Grimhand's Barcrawl (varbit por bar via plugin; ou quest_state IN_PROGRESS)")
        score += 1
    if re.search(r"\bqp\b|quest points?", tl):
        sugg.append("quest_points >= N (hiscores não tem; WikiSync/plugin)")
        score += 2
    if re.search(r"\b(\d+)\s*(?:k|m)\s*(?:gp|cash|coins)\b", tl):
        sugg.append("gp_banked (plugin)")
        score += 1
    if not sugg:
        return None
    return score, "; ".join(dict.fromkeys(sugg))


def build_stats(sections: list[dict], unknown_quests: set, quest_names: set) -> dict:
    rule_counts = collections.Counter()
    track = collections.Counter()
    conf = collections.Counter()
    steps = [st for s in sections for st in s["steps"]]
    for st in steps:
        for r in st["complete_when"]:
            rule_counts[r["type"]] += 1
            conf[(r["type"], r.get("source"), r.get("confidence", "-"))] += 1
        track[st["track_via"]] += 1
    per_section = {s["id"]: collections.Counter(st["track_via"] for st in s["steps"]) for s in sections}
    manual = [st for st in steps if st["track_via"] == "manual"]
    review = []
    for st in manual:
        r = suggest_for_manual(st)
        if r:
            review.append({"id": st["id"], "score": r[0], "text": st["text"], "suggestion": r[1]})
    review.sort(key=lambda x: (-x["score"], x["id"]))
    locations = collections.Counter(st["location"] for st in steps)
    quests_in_guide = sorted({st["quest"]["name"] for st in steps if st["quest"]} | {
        r["quest"] for st in steps for r in st["complete_when"] if r["type"] == "quest_state"})
    inferred = sum(1 for st in steps for r in st["complete_when"] if r.get("inferred"))
    return {
        "steps_total": len(steps),
        "rule_counts": dict(rule_counts),
        "rules_inferred_from_text": inferred,
        "track_via": dict(track),
        "track_via_per_section": {k: dict(v) for k, v in per_section.items()},
        "rule_confidence": [{"type": k[0], "source": k[1], "confidence": k[2], "count": v} for k, v in sorted(conf.items(), key=lambda kv: (kv[0][0], str(kv[0][1]), kv[0][2]))],
        "layers": dict(collections.Counter(st["layer"] for st in steps)),
        "tags": dict(collections.Counter(t for st in steps for t in st["tags"] if not t.startswith("boss:"))),
        "locations": [{"location": k, "count": v} for k, v in locations.most_common()],
        "quests_in_guide": quests_in_guide,
        "quests_not_in_catalog": sorted(unknown_quests),
        "quest_catalog_size": len(quest_names),
        "manual_review_top30": review[:30],
        "manual_review_total_candidates": len(review),
        "steps_with_prerequisites": sum(1 for st in steps if st["prerequisites"]),
        "quest_pairs_linked": sum(1 for st in steps if st["completes_in"]),
    }


def print_report(stats: dict):
    print("## Regras por tipo\n\n| tipo | passos com a regra |\n|---|---|")
    for k, v in sorted(stats["rule_counts"].items(), key=lambda kv: -kv[1]):
        print(f"| {k} | {v} |")
    print("\n## Rastreamento\n\n| via | passos |\n|---|---|")
    for k, v in stats["track_via"].items():
        print(f"| {k} | {v} |")
    print("\n## Top 30 manuais para revisão\n\n| id | texto | sugestão |\n|---|---|---|")
    for r in stats["manual_review_top30"]:
        print(f"| {r['id']} | {r['text'][:110]} | {r['suggestion']} |")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--print", action="store_true", help="print markdown review tables")
    ap.add_argument("--out", default=os.path.join(OUT_DIR, "guide.json"))
    args = ap.parse_args(argv)

    index = json.load(open(os.path.join(GUIDE_DIR, "_index.json"), encoding="utf-8"))
    quest_names, quest_lookup = load_quest_catalog()
    unknown_quests: set = set()
    sections = []
    for sec in index:
        path = os.path.join(GUIDE_DIR, f"{sec['id'].replace('.', '-')}-{sec['slug']}.json")
        raw = json.load(open(path, encoding="utf-8"))
        steps = [normalize_step(s, sec["id"], quest_lookup, unknown_quests) for s in raw["steps"]]
        sections.append({
            "id": sec["id"], "slug": sec["slug"], "title": raw.get("title", sec["title"]),
            "description": raw.get("description"), "seo_title": raw.get("seoTitle"),
            "track": bool(sec.get("track")), "related": raw.get("related", []), "steps": steps,
        })
    link_quest_pairs(sections)

    guide = {
        "schema_version": "0.1",
        "guide": {
            "id": "oziris-efficiency",
            "title": "OSRS Ironman Efficiency Guide (OzirisLoL)",
            "source": {
                "url": "https://ironman.guide/guide",
                "author": "OzirisLoL",
                "maintainer": "ironman.guide",
                "extracted_at": _dt.date.today().isoformat(),
                "license_note": "Republished guide text by OzirisLoL (maintained/extended by ironman.guide). "
                                "No explicit licence: personal, self-hosted use only; keep attribution.",
            },
        },
        "sections": sections,
    }
    os.makedirs(os.path.dirname(args.out), exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as fh:
        json.dump(guide, fh, ensure_ascii=False, indent=1)
    stats = build_stats(sections, unknown_quests, quest_names)
    with open(os.path.join(os.path.dirname(args.out), "stats.json"), "w", encoding="utf-8") as fh:
        json.dump(stats, fh, ensure_ascii=False, indent=1)
    print(f"wrote {args.out}: {stats['steps_total']} steps; track_via={stats['track_via']}; rules={stats['rule_counts']}")
    if args.print:
        print_report(stats)
    return 0


if __name__ == "__main__":
    sys.exit(main())
