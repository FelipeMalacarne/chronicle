#!/usr/bin/env python3
"""Extract the Mage Training Arena reward table from the OSRS Wiki.

Reads the wikitext of "Template:Mage Training Arena Shop stock" (the Rewards Guardian's
stock: what each item costs in each room's pizazz points) and writes
internal/core/mta/mta.json, the table both calculators read: the Go core
(internal/core/mta) embeds it, and `tools/gen_mta_java.py` turns it into the constants of the
Microbot plugin fm/mtaarena (which must compile on its own, so it cannot read the JSON).

What the wiki states in prose, not in a table, is kept here in ROOMS and ENCHANT_LEVELS with
its source; the extractor refuses to write when the shop table does not add up to the totals
the template itself prints (2825 / 2825 / 29000 / 3275).

The wikitext is cached under --cache-dir, so reruns do not hit the network; --refresh bypasses
the cache.

  python3 tools/extract_mta.py [--refresh] [--cache-dir DIR]
"""

import argparse
import json
import os
import re
import sys
import urllib.parse
import urllib.request

API = "https://oldschool.runescape.wiki/api.php"
USER_AGENT = "ironman-planner mta extract (personal, github FelipeMalacarne)"
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "internal", "core", "mta", "mta.json")
PAGES = ["Template:Mage Training Arena Shop stock"]

# Room facts from the wiki's "Pizazz points" page, section "Cost per point", and the "Mage
# Training Arena" page. Runes per point are what a room spends on average with staves covering
# the elemental runes. Caps are the most points a player can hold per room.
ROOMS = [
    {
        "id": "telekinetic", "name": "Telekinetic Theatre", "rune": "law",
        # 31.5 law runes buy 18 points: 8.3 casts per maze, five mazes in a row give 8 bonus
        # points and 10 law runes back.
        "runes_per_point": {"num": 63, "den": 36}, "cap": 8000,
    },
    {
        "id": "graveyard", "name": "Creature Graveyard", "rune": "nature",
        # Every nature rune (Bones to Bananas) deposits 20 bananas, 1.25 points.
        "runes_per_point": {"num": 4, "den": 5}, "cap": 8000,
    },
    {
        "id": "enchantment", "name": "Enchanting Chamber", "rune": "cosmic",
        # One cosmic rune is a cast: 1 point per spell level, plus a bonus every tenth shape
        # (1.1 points per level on average). Divided by the level of the Enchant used.
        "runes_per_point": {"num": 10, "den": 11}, "cap": 32000,
    },
    {
        "id": "alchemist", "name": "Alchemists' Playground", "rune": "nature",
        # Alchemising only the 30 coin items: three nature runes are worth one point.
        "runes_per_point": {"num": 3, "den": 1}, "cap": 16000,
    },
]

# Lvl-N Enchant: the Magic level that casts it (OSRS Wiki "Enchant crossbow bolt"/"Enchantment
# spells"). The Enchanting Chamber gives N points per cast.
ENCHANT_LEVELS = [
    {"level": 1, "magic": 7}, {"level": 2, "magic": 27}, {"level": 3, "magic": 49},
    {"level": 4, "magic": 57}, {"level": 5, "magic": 68}, {"level": 6, "magic": 87},
]

# Wiki name -> id, wand tier required and the client's item id (ItemID.MAGICTRAINING_*).
REWARDS = {
    "Infinity hat": ("infinity_hat", None, 6918),
    "Infinity top": ("infinity_top", None, 6916),
    "Infinity bottoms": ("infinity_bottoms", None, 6924),
    "Infinity gloves": ("infinity_gloves", None, 6922),
    "Infinity boots": ("infinity_boots", None, 6920),
    "Beginner wand": ("beginner_wand", None, 6908),
    "Apprentice wand": ("apprentice_wand", "beginner_wand", 6910),
    "Teacher wand": ("teacher_wand", "apprentice_wand", 6912),
    "Master wand": ("master_wand", "teacher_wand", 6914),
    "Mage's book": ("mages_book", None, 6889),
    "Rune pouch": ("rune_pouch", None, 12791),
    "Bones to Peaches": ("bones_to_peaches", None, 6926),
}
TOTALS = [2825, 2825, 29000, 3275]
ROOM_ORDER = ["telekinetic", "graveyard", "enchantment", "alchemist"]


def fetch(title, cache_dir, refresh):
    path = os.path.join(cache_dir, re.sub(r"\W+", "_", title) + ".wt")
    if not refresh and os.path.exists(path):
        with open(path, encoding="utf-8") as f:
            return f.read()
    params = {
        "action": "query", "format": "json", "formatversion": "2", "prop": "revisions",
        "rvprop": "content", "rvslots": "main", "titles": title,
    }
    req = urllib.request.Request(API + "?" + urllib.parse.urlencode(params), headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=60) as r:
        data = json.load(r)
    text = data["query"]["pages"][0]["revisions"][0]["slots"]["main"]["content"]
    os.makedirs(cache_dir, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    return text


ROW_RE = re.compile(
    r"\{\{plinkt\|([^}|]+)\}\}[^\n|]*(?:\|\||\n\|)\s*(\d+)\s*\|\|\s*(\d+)\s*\|\|\s*(\d+)\s*\|\|\s*(\d+)"
)


def parse(text):
    """The shop rows of the first table (items), before the Runes table."""
    items_part = text.split("===Runes===")[0]
    rows = {}
    for m in ROW_RE.finditer(items_part):
        name = m.group(1).strip()
        rows[name] = [int(m.group(i)) for i in range(2, 6)]
    return rows


def build(rows):
    missing = [n for n in REWARDS if n not in rows]
    extra = [n for n in rows if n not in REWARDS]
    if missing or extra:
        sys.exit(f"shop rows differ from REWARDS: missing {missing}, unexpected {extra}")
    totals = [sum(rows[n][i] for n in REWARDS) for i in range(4)]
    if totals != TOTALS:
        sys.exit(f"shop totals {totals} differ from the template's {TOTALS}")
    rewards = []
    for name, (rid, requires, item_id) in REWARDS.items():
        cost = dict(zip(ROOM_ORDER, rows[name]))
        r = {"id": rid, "name": name, "item_id": item_id, "cost": cost}
        if requires:
            r["requires"] = requires
        rewards.append(r)
    return {
        "source": "OSRS Wiki: Template:Mage Training Arena Shop stock; Pizazz points (CC BY-NC-SA 3.0)",
        "rooms": ROOMS,
        "enchant_levels": ENCHANT_LEVELS,
        "rewards": rewards,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--refresh", action="store_true")
    ap.add_argument("--cache-dir", default=os.path.expanduser("~/.cache/ironman-planner/wiki-mta"))
    args = ap.parse_args()
    text = "\n".join(fetch(t, args.cache_dir, args.refresh) for t in PAGES)
    out = build(parse(text))
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(out, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(f"wrote {OUT}: {len(out['rewards'])} rewards")


if __name__ == "__main__":
    main()
