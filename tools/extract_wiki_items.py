#!/usr/bin/env python3
"""Offline extract of the OSRS items cited by the two guides (stdlib only).

Inputs (read from the repository root):
  - content/guides/*/entities.report.json: `ambiguous` and `unresolved` entries
    with kind == "item" (their `source_text`).
  - content/guides/*/guide.json: every step `items[]` (string or {"name"}) and
    every segment `loadout.inventory[]` / `loadout.equipment[]` `name`.
  - content/reference/items.json: every catalog entry's `wiki_key` (always
    re-fetched, so a rerun never drops an id the catalog already has; the
    hand-curated subset this replaced lives in
    internal/core/entities/catalogbuild/testdata/items_curated.json).
  - SEED_PAGES below: item families (potions, runes, tools, food, charged
    jewellery, teleport tablets, coins) fetched even when no guide cites them.

Network sources:
  - OSRS Wiki MediaWiki API (api.php, prop=revisions, up to 50 titles per
    request, 1 s between requests). Only pages whose wikitext contains
    {{Infobox Item}} are kept; {{Infobox Bonuses}} gives the equipment slot.
  - RuneLite runelite-api gameval ItemID.java pinned to one commit SHA: the
    top-level constants are items, the nested `Cert` class holds the noted
    form under the same constant name, which yields noted_id.

Raw responses are cached under --cache-dir so reruns do not hit the network;
--refresh bypasses the cache (and re-pins the RuneLite SHA to current master).

Output: data/wiki/items.json (sorted, 2-space indent, trailing newline) with
`source`, `items` (one entry per item id), `not_found` (cited names for which
no candidate title produced an item page) and `conflicts` (ids seen on more
than one page/version). The file is an input of the Go catalog generator
(internal/core/entities/catalogbuild); it is never read at runtime. Guide
names map to pages only to decide what to fetch: the catalog resolves names
by exact name, curated alias or plural, never through this script's
normalization.

Usage, from the repository root:
  python3 tools/extract_wiki_items.py [--refresh] [--cache-dir DIR] [--out FILE]
"""

import argparse
import datetime
import glob
import hashlib
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = "https://oldschool.runescape.wiki/api.php"
USER_AGENT = "ironman-planner item extract (personal, github FelipeMalacarne)"
RUNELITE_COMMITS = "https://api.github.com/repos/runelite/runelite/commits/master"
ITEMID_PATH = "runelite-api/src/main/java/net/runelite/api/gameval/ItemID.java"
BATCH = 50
SLEEP = 1.0

# Item families fetched regardless of citations (sentence-case wiki titles).
SEED_PAGES = [
    # Potions (dose forms).
    "Prayer potion", "Super restore", "Stamina potion", "Energy potion", "Super energy",
    "Antipoison", "Superantipoison", "Antidote+", "Antidote++", "Antifire potion",
    "Extended antifire", "Super antifire potion", "Saradomin brew", "Zamorak brew",
    "Super attack", "Super strength", "Super defence", "Super combat potion",
    "Divine super combat potion", "Ranging potion", "Divine ranging potion",
    "Bastion potion", "Magic potion", "Divine magic potion", "Battlemage potion",
    "Combat potion", "Agility potion", "Fishing potion", "Hunter potion", "Attack potion",
    "Strength potion", "Defence potion", "Restore potion", "Sanfew serum", "Relicym's balm",
    "Guthix rest", "Anti-venom", "Anti-venom+", "Compost potion", "Guthix balance",
    "Prayer enhance", "Menaphite remedy",
    # Runes.
    "Air rune", "Water rune", "Earth rune", "Fire rune", "Mind rune", "Body rune",
    "Cosmic rune", "Chaos rune", "Nature rune", "Law rune", "Death rune", "Astral rune",
    "Blood rune", "Soul rune", "Wrath rune", "Mist rune", "Dust rune", "Mud rune",
    "Smoke rune", "Steam rune", "Lava rune", "Sunfire rune", "Aether rune",
    "Rune essence", "Pure essence", "Daeyalt essence",
    # Axes and pickaxes.
    "Bronze axe", "Iron axe", "Steel axe", "Black axe", "Mithril axe", "Adamant axe",
    "Rune axe", "Dragon axe", "Crystal axe", "3rd age axe", "Infernal axe",
    "Bronze pickaxe", "Iron pickaxe", "Steel pickaxe", "Black pickaxe", "Mithril pickaxe",
    "Adamant pickaxe", "Rune pickaxe", "Dragon pickaxe", "Crystal pickaxe",
    "3rd age pickaxe", "Infernal pickaxe",
    # Food.
    "Shrimps", "Anchovies", "Sardine", "Herring", "Mackerel", "Trout", "Cod", "Pike",
    "Salmon", "Tuna", "Lobster", "Bass", "Swordfish", "Monkfish", "Shark", "Manta ray",
    "Sea turtle", "Anglerfish", "Dark crab", "Cooked karambwan", "Jug of wine", "Cake",
    "Chocolate cake", "Meat pie", "Apple pie", "Redberry pie", "Bread", "Cooked chicken",
    "Cooked meat", "Potato with cheese", "Tuna potato", "Stew", "Pineapple pizza",
    # Charged jewellery and teleport items.
    "Games necklace", "Ring of dueling", "Necklace of passage", "Amulet of glory",
    "Skills necklace", "Combat bracelet", "Digsite pendant", "Ring of wealth",
    "Burning amulet", "Slayer ring", "Ring of the elements", "Amulet of glory (t)",
    "Ectophial", "Royal seed pod", "Xeric's talisman", "Drakan's medallion",
    # Containers with charges.
    "Waterskin", "Watering can", "Bucket", "Bucket of water", "Jug", "Jug of water",
    "Vial", "Vial of water",
    # Teleport tablets.
    "Varrock teleport (tablet)", "Lumbridge teleport (tablet)", "Falador teleport (tablet)",
    "Camelot teleport (tablet)", "Ardougne teleport (tablet)", "Watchtower teleport (tablet)",
    "Teleport to house (tablet)", "Varrock teleport", "Lumbridge teleport",
    "Falador teleport", "Camelot teleport", "Ardougne teleport", "Watchtower teleport",
    "Teleport to house",
    # Currency.
    "Coins",
]

# Version labels that are not current, obtainable items (case-insensitive).
JUNK_VERSION = re.compile(
    r"beta|histor|removed|last man standing|\blms\b|deadman|leagues?\b|trouver|locked|"
    r"broken|placeholder|unobtainable|emir's arena", re.I)

# Cited text that is clearly not a single item name.
NON_ITEM = re.compile(r"\b(gp|gear|food|items?|quests?)\b", re.I)

NUMBER_WORDS = {
    "a", "an", "the", "one", "two", "three", "four", "five", "six", "seven", "eight",
    "nine", "ten", "twelve", "few", "some", "many", "several", "all", "any", "more",
    "multiple", "both", "each", "your", "of", "noted", "x",
}

# Shorthand used by the guides (lowercase phrase, after quantity stripping) ->
# wiki title. Tried before the mechanical candidates.
ALIASES = {
    "addy pickaxe": "Adamant pickaxe",
    "addy nails": "Adamant nails",
    "rune pick": "Rune pickaxe",
    "rune scim": "Rune scimitar",
    "cosmics": "Cosmic rune",
    "d bones": "Dragon bones",
    "games neck": "Games necklace",
    "glory": "Amulet of glory",
    "dueling ring": "Ring of dueling",
    "dueling rings": "Ring of dueling",
    "energy pot": "Energy potion",
    "energy pots": "Energy potion",
    "prayer pot": "Prayer potion",
    "prayer pots": "Prayer potion",
    "ranging pot": "Ranging potion",
    "ranging pots": "Ranging potion",
    "superantipoison potion": "Superantipoison",
    "superantipoison potions": "Superantipoison",
    "ardy cloak 1": "Ardougne cloak 1",
    "pestle/mortar": "Pestle and mortar",
    "pestle+mortar": "Pestle and mortar",
    "house tab": "Teleport to house (tablet)",
    "house teletab": "Teleport to house (tablet)",
    "house teletabs": "Teleport to house (tablet)",
    "poh tab": "Teleport to house (tablet)",
    "falador tab": "Falador teleport (tablet)",
    "fally teletab": "Falador teleport (tablet)",
    "paddewwa tab": "Paddewwa teleport (tablet)",
    "paddewwa tabs": "Paddewwa teleport (tablet)",
    "normal log": "Logs",
    "regular log": "Logs",
    "regular logs": "Logs",
    "any log": "Logs",
    "log": "Logs",
    "guam": "Guam leaf",
    "clean guam": "Guam leaf",
    "clean marrentill": "Marrentill",
    "varrock armor 2": "Varrock armour 2",
    "varrock body 2": "Varrock armour 2",
    "fremennik boots 1": "Fremennik sea boots 1",
    "shayzien boots 5": "Shayzien boots (5)",
    "copper": "Copper ore",
    "shantay pass": "Shantay pass (item)",
    "shantay passes": "Shantay pass (item)",
    "holy grail": "Holy Grail (item)",
    "onions": "Onion",
    "empty bucket": "Bucket",
    "empty buckets": "Bucket",
    "buckets": "Bucket",
    "empty pot": "Pot",
    "empty pots": "Pot",
    "empty vial": "Vial",
    "empty vials": "Vial",
    "gold ores": "Gold ore",
    "charos(a)": "Ring of charos (a)",
    "ring of charos(a)": "Ring of charos (a)",
    "ring of charos": "Ring of charos",
    "wizard's mind bomb": "Wizard's mind bomb",
    "wizard mind bomb": "Wizard's mind bomb",
    "wizard mind bombs": "Wizard's mind bomb",
    "kitten/cat": "Kitten",
    "red spider eggs": "Red spiders' eggs",
    "red spider's eggs": "Red spiders' eggs",
    "facemask": "Facemask",
    "face mask": "Facemask",
    "seed dibbe": "Seed dibber",
    "pickaxe (for wildy med diary": "Pickaxe",
}

IRREGULAR = {
    "leaves": "leaf", "knives": "knife", "potatoes": "potato", "tomatoes": "tomato",
    "axes": "axe", "pickaxes": "pickaxe", "battleaxes": "battleaxe", "teeth": "tooth",
    "feet": "foot", "geese": "goose", "mice": "mouse", "pieces": "piece",
    "dyes": "dye", "staves": "staff", "battlestaves": "battlestaff",
}

INVALID_TITLE = re.compile(r"[#<>\[\]{}|]")


# ---------------------------------------------------------------------------
# Inputs


def load_json(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def cited_names(root):
    """Return the set of item names cited by the guides and their reports."""
    names = set()
    for path in sorted(glob.glob(os.path.join(root, "content/guides/*/entities.report.json"))):
        report = load_json(path)
        for key in ("ambiguous", "unresolved"):
            for entry in report.get(key) or []:
                if entry.get("kind") == "item" and entry.get("source_text"):
                    names.add(entry["source_text"])
    for path in sorted(glob.glob(os.path.join(root, "content/guides/*/guide.json"))):
        guide = load_json(path)
        for variant in guide.get("variants") or []:
            for section in variant.get("sections") or []:
                for segment in section.get("segments") or []:
                    loadout = segment.get("loadout") or {}
                    for key in ("inventory", "equipment"):
                        for entry in loadout.get(key) or []:
                            if entry.get("name"):
                                names.add(entry["name"])
                    for step in segment.get("steps") or []:
                        for item in step.get("items") or []:
                            name = item if isinstance(item, str) else (item or {}).get("name")
                            if name:
                                names.add(name)
    return names


def curated(root):
    return load_json(os.path.join(root, "content/reference/items.json"))


# ---------------------------------------------------------------------------
# Name -> candidate wiki titles


def fold(text):
    text = (text.replace("’", "'").replace("‘", "'").replace("“", '"')
            .replace("”", '"').replace("…", "...").replace(" ", " "))
    return re.sub(r"\s+", " ", text).strip()


def strip_quantity(text):
    """Strip leading quantities and determiners ("5", "1.1k", "(4+)", "a", "two")."""
    while True:
        before = text
        text = re.sub(r"^\(?\d[\d.,]*k?\+?(\(/\d+\))?\)?(?=\s|$)\s*", "", text, flags=re.I)
        text = re.sub(r"^\d+-dose\s+", "", text, flags=re.I)
        words = text.split(" ", 1)
        if len(words) == 2 and words[0].lower() in NUMBER_WORDS:
            text = words[1]
        text = text.strip()
        if text == before:
            return text


def strip_parens(text):
    """Drop trailing parentheticals, including an unclosed trailing one."""
    while True:
        before = text
        text = re.sub(r"\s*\([^()]*\)\s*$", "", text)
        text = re.sub(r"\s*\([^()]*$", "", text)
        text = text.strip(" .,;:!?)")
        if text == before:
            return text


def singular_word(word):
    lw = word.lower()
    if lw in IRREGULAR:
        out = IRREGULAR[lw]
        return [word[0] + out[1:] if word[:1].isupper() else out]
    if len(lw) < 4 or lw.endswith(("ss", "us", "is")) or not lw.endswith("s"):
        return []
    outs = []
    if lw.endswith("ies"):
        outs.append(word[:-3] + "y")
    elif lw.endswith(("ches", "shes", "xes", "sses", "oes")):
        outs.append(word[:-2])
    elif lw.endswith("ves"):
        outs.append(word[:-3] + "f")
    outs.append(word[:-1])
    return outs


def singular_forms(phrase):
    """Singularize the head noun ("buckets of sand" -> "bucket of sand")."""
    words = phrase.split(" ")
    idx = len(words) - 1
    if " of " in phrase:
        idx = words.index("of") - 1
        if idx < 0:
            return []
    return [" ".join(words[:idx] + [w] + words[idx + 1:]) for w in singular_word(words[idx])]


def split_alternatives(text):
    parts = [p.strip() for p in re.split(r"\s*/\s*|\s+or\s+|\s*\+\s*|\s*,\s*", text) if p.strip()]
    if len(parts) < 2:
        return parts
    first, last = parts[0].split(" "), parts[-1].split(" ")
    middle_single = all(len(p.split(" ")) == 1 for p in parts[1:-1])
    if len(first) == 1 and len(last) > 1 and middle_single:
        # "air/water/earth runes": distribute the trailing noun.
        tail = " ".join(last[1:])
        parts = [p + " " + tail for p in parts[:-1]] + [parts[-1]]
    elif len(first) > 1 and len(last) == 1 and middle_single:
        # "desert shirt/robe": distribute the leading words.
        head = " ".join(first[:-1])
        parts = [parts[0]] + [head + " " + p for p in parts[1:]]
    return parts


def sentence_case(text):
    return text[:1].upper() + text[1:] if text else text


def candidates(raw):
    """Candidate titles for a cited name, one ordered group per alternative.

    "trout/salmon" yields two groups ([Trout...], [Salmon...]); each group is
    resolved independently (first candidate with an item page wins). Returns
    None when the text is clearly not an item name.
    """
    text = fold(raw)
    if len(text.split(" ")) > 6 or NON_ITEM.search(text):
        return None

    def add(group, title):
        title = sentence_case(title.strip(" .,;:!?\""))
        if title and not INVALID_TITLE.search(title) and len(title) <= 200 and title not in group:
            group.append(title)

    def expand(group, phrase):
        alias = ALIASES.get(phrase.lower())
        if alias:
            add(group, alias)
        for form in [phrase] + singular_forms(phrase):
            alias = ALIASES.get(form.lower())
            if alias:
                add(group, alias)
            add(group, form)
            add(group, form.lower())

    base = strip_quantity(text)
    stripped = strip_quantity(strip_parens(base))
    whole = []
    if stripped != base:
        expand(whole, base)
    elif ALIASES.get(base.lower()):
        add(whole, ALIASES[base.lower()])
    groups = []
    for part in split_alternatives(stripped):
        group = []
        expand(group, strip_quantity(part))
        if group:
            groups.append(group)
    if whole:
        if len(groups) == 1:
            groups[0] = whole + [t for t in groups[0] if t not in whole]
        else:
            groups.insert(0, whole)
    return groups


# ---------------------------------------------------------------------------
# HTTP with cache


def http_get(url):
    last = None
    for attempt in range(4):
        req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                return resp.read()
        except (urllib.error.URLError, TimeoutError) as err:
            last = err
            time.sleep(2 * (attempt + 1))
    raise RuntimeError(f"GET {url}: {last}")


class Wiki:
    def __init__(self, cache_dir, refresh):
        self.dir = os.path.join(cache_dir, "wiki")
        os.makedirs(self.dir, exist_ok=True)
        self.refresh = refresh
        self.requests = 0
        self.last = 0.0

    def _get(self, params):
        wait = SLEEP - (time.time() - self.last)
        if wait > 0:
            time.sleep(wait)
        url = API + "?" + urllib.parse.urlencode(params)
        body = http_get(url)
        self.last = time.time()
        self.requests += 1
        return json.loads(body)

    def fetch_batch(self, titles):
        """Fetch one batch (following `continue`), cache it and return the raw responses."""
        params = {
            "action": "query", "prop": "revisions", "rvprop": "content|ids",
            "rvslots": "main", "format": "json", "formatversion": "2", "redirects": "1",
            "titles": "|".join(titles),
        }
        responses, cont = [], {}
        while True:
            data = self._get({**params, **cont})
            if "error" in data:
                raise RuntimeError(f"wiki API error: {data['error']}")
            responses.append(data)
            if "continue" not in data:
                break
            cont = data["continue"]
        key = hashlib.sha1("\n".join(titles).encode("utf-8")).hexdigest()
        with open(os.path.join(self.dir, key + ".json"), "w", encoding="utf-8") as f:
            json.dump({"titles": titles, "responses": responses}, f)
        return responses

    @staticmethod
    def absorb(titles, responses, resolve, pages):
        """Merge one batch's raw responses into resolve/pages."""
        hops = {}
        for data in responses:
            query = data.get("query") or {}
            for n in query.get("normalized") or []:
                hops[n["from"]] = n["to"]
            for r in query.get("redirects") or []:
                hops[r["from"]] = r["to"]
            for page in query.get("pages") or []:
                revs = page.get("revisions")
                if revs and "missing" not in page:
                    rev = revs[0]
                    content = rev.get("slots", {}).get("main", {}).get("content")
                    old = pages.get(page["title"])
                    if content is not None and (old is None or rev["revid"] > old[0]):
                        pages[page["title"]] = (rev["revid"], content)
        for title in titles:
            cur, seen = title, set()
            while cur in hops and cur not in seen:
                seen.add(cur)
                cur = hops[cur]
            resolve[title] = cur

    def fetch(self, titles):
        """Return (resolve map requested->final title, pages title->(revid, wikitext)).

        Without --refresh every cached batch is loaded first and only titles
        never requested before hit the network.
        """
        titles = sorted(set(titles))
        resolve, pages = {}, {}
        if not self.refresh:
            for path in sorted(glob.glob(os.path.join(self.dir, "*.json"))):
                cached = load_json(path)
                self.absorb(cached["titles"], cached["responses"], resolve, pages)
        todo = [t for t in titles if t not in resolve]
        for i in range(0, len(todo), BATCH):
            batch = todo[i:i + BATCH]
            self.absorb(batch, self.fetch_batch(batch), resolve, pages)
            print(f"  wiki: {min(i + BATCH, len(todo))}/{len(todo)} new titles", file=sys.stderr)
        return {t: resolve[t] for t in titles}, pages


# ---------------------------------------------------------------------------
# RuneLite noted ids


def runelite_ids(cache_dir, refresh, sha_override):
    rdir = os.path.join(cache_dir, "runelite")
    os.makedirs(rdir, exist_ok=True)
    sha_path = os.path.join(rdir, "sha.txt")
    sha = sha_override
    if not sha and not refresh and os.path.exists(sha_path):
        with open(sha_path, encoding="utf-8") as f:
            sha = f.read().strip()
    if not sha:
        sha = json.loads(http_get(RUNELITE_COMMITS))["sha"]
    with open(sha_path, "w", encoding="utf-8") as f:
        f.write(sha + "\n")
    path = os.path.join(rdir, f"ItemID-{sha}.java")
    if refresh or not os.path.exists(path):
        body = http_get(f"https://raw.githubusercontent.com/runelite/runelite/{sha}/{ITEMID_PATH}")
        with open(path, "wb") as f:
            f.write(body)
    top, cert = {}, {}
    depth, klass = 0, None
    const = re.compile(r"public static final int (\w+)\s*=\s*(-?\d+)\s*;")
    nested = re.compile(r"public static final class (\w+)")
    with open(path, encoding="utf-8") as f:
        for line in f:
            m = nested.search(line)
            if m and depth == 1:
                klass = m.group(1)
            m = const.search(line)
            if m:
                name, value = m.group(1), int(m.group(2))
                if depth == 1:
                    top[name] = value
                elif depth == 2 and klass == "Cert":
                    cert[name] = value
            prev = depth
            depth += line.count("{") - line.count("}")
            if prev > 1 and depth <= 1:
                klass = None
    noted = {}
    for name, value in top.items():
        if name in cert:
            noted.setdefault(value, cert[name])
    return sha, noted


# ---------------------------------------------------------------------------
# Wikitext parsing


def templates(text, name):
    """Yield the top-level params dict of every {{name ...}} template in text."""
    pat = re.compile(r"\{\{\s*" + r"[ _]".join(map(re.escape, name.split(" "))) + r"\s*(?=[|}\n])",
                     re.I)
    for m in pat.finditer(text):
        i, depth = m.end(), 1
        while i < len(text) and depth:
            if text.startswith("{{", i):
                depth += 1
                i += 2
            elif text.startswith("}}", i):
                depth -= 1
                i += 2
            else:
                i += 1
        body = text[m.end():i - 2]
        parts, cur, d1, d2, j = [], [], 0, 0, 0
        while j < len(body):
            two = body[j:j + 2]
            if two in ("{{", "}}", "[[", "]]"):
                if two == "{{":
                    d1 += 1
                elif two == "}}":
                    d1 -= 1
                elif two == "[[":
                    d2 += 1
                else:
                    d2 -= 1
                cur.append(two)
                j += 2
                continue
            if body[j] == "|" and d1 == 0 and d2 == 0:
                parts.append("".join(cur))
                cur = []
            else:
                cur.append(body[j])
            j += 1
        parts.append("".join(cur))
        params = {}
        for p in parts:
            if "=" in p:
                k, v = p.split("=", 1)
                params[k.strip().lower()] = v.strip()
        yield params


def yes_no(value):
    if value is None:
        return None
    v = value.strip().lower()
    if v.startswith("yes"):
        return True
    if v.startswith("no"):
        return False
    return None


def plain(value):
    if value is None:
        return None
    value = re.sub(r"\[\[(?:[^\]|]*\|)?([^\]]*)\]\]", r"\1", value)
    value = re.sub(r"<[^>]+>", "", value)
    return value.strip()


def version_numbers(params):
    nums = set()
    for k in params:
        m = re.fullmatch(r"(?:version|name|id)(\d+)", k)
        if m:
            nums.add(int(m.group(1)))
    return sorted(nums)


def parse_page(text, stats):
    """Return the list of item versions on a page (empty when no Infobox Item)."""
    text = re.sub(r"<!--.*?-->", "", text, flags=re.S)
    boxes = list(templates(text, "Infobox Item"))
    if not boxes:
        return None
    slot_by_label, base_slot = {}, None
    for bonus in templates(text, "Infobox Bonuses"):
        nums = sorted(int(m.group(1)) for k in bonus for m in [re.fullmatch(r"version(\d+)", k)] if m)
        if base_slot is None and bonus.get("slot"):
            base_slot = plain(bonus["slot"]).lower()
        for n in nums:
            slot = bonus.get(f"slot{n}") or bonus.get("slot")
            if slot:
                slot_by_label.setdefault(plain(bonus[f"version{n}"]), plain(slot).lower())
    out = []
    for params in boxes:
        nums = version_numbers(params) or [None]
        for n in nums:
            def get(key):
                if n is not None and f"{key}{n}" in params:
                    return params[f"{key}{n}"]
                return params.get(key)
            label = plain(params.get(f"version{n}")) if n is not None else None
            if label and JUNK_VERSION.search(label):
                stats["skipped_versions"] += 1
                continue
            removal = plain(get("removal"))
            if removal and removal.lower() not in ("no", "n/a", ""):
                stats["skipped_versions"] += 1
                continue
            name = plain(get("name"))
            ids = [int(x.strip()) for x in (get("id") or "").split(",") if x.strip().isdigit()]
            if not name or not ids:
                stats["versions_without_id"] += 1
                continue
            equipable = yes_no(get("equipable"))
            slot = None
            if equipable is not False:
                slot = slot_by_label.get(label) if label else None
                slot = slot or base_slot
                if equipable is None and slot is None:
                    equipable = None
            out.append({
                "label": label or None,
                "name": name,
                "ids": ids,
                "stackable": yes_no(get("stackable")),
                "noteable": yes_no(get("noteable")),
                "equipable": equipable,
                "members": yes_no(get("members")),
                "equipment_slot": slot if equipable is not False else None,
            })
    return out


# ---------------------------------------------------------------------------
# Main


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n", 1)[0])
    ap.add_argument("--refresh", action="store_true", help="bypass the cache and re-pin RuneLite")
    ap.add_argument("--cache-dir", default=os.path.join(
        os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache"),
        "ironman-planner", "wiki-items"))
    ap.add_argument("--out", default="data/wiki/items.json")
    ap.add_argument("--runelite-sha", default=None, help="pin ItemID.java to this commit")
    args = ap.parse_args()
    root = os.getcwd()

    names = cited_names(root)
    ref = curated(root)
    curated_keys = sorted({i["wiki_key"] for i in ref["items"] if i.get("wiki_key")})

    name_candidates, skipped = {}, []
    for raw in sorted(names):
        cands = candidates(raw)
        if cands is None or not cands:
            skipped.append(raw)
        else:
            name_candidates[raw] = cands
    titles = set(SEED_PAGES) | set(curated_keys)
    for groups in name_candidates.values():
        for group in groups:
            titles.update(group)
    print(f"cited names: {len(names)} ({len(skipped)} skipped as non-items), "
          f"titles to fetch: {len(titles)}", file=sys.stderr)

    wiki = Wiki(args.cache_dir, args.refresh)
    resolve, pages = wiki.fetch(sorted(titles))

    stats = {"skipped_versions": 0, "versions_without_id": 0}
    parsed = {}
    for title in sorted(pages):
        versions = parse_page(pages[title][1], stats)
        if versions:
            parsed[title] = versions

    def page_of(title):
        final = resolve.get(title, title)
        return final if final in parsed else None

    cited_by_page, not_found = {}, []
    matched = 0
    for raw, groups in sorted(name_candidates.items()):
        hits = [next((page_of(c) for c in group if page_of(c)), None) for group in groups]
        for hit in hits:
            if hit:
                cited_by_page.setdefault(hit, set()).add(raw)
        if any(hits):
            matched += 1
        else:
            not_found.append({"text": raw, "tried": [c for group in groups for c in group]})
    wanted = set(cited_by_page)
    for title in list(SEED_PAGES) + curated_keys:
        if page_of(title):
            wanted.add(page_of(title))

    sha, noted = runelite_ids(args.cache_dir, args.refresh, args.runelite_sha)

    items, conflicts, owner = {}, [], {}
    noted_warn = {"noteable_without_cert": 0, "cert_without_noteable": 0}
    for title in sorted(wanted):
        versions = parsed[title]
        distinct = {i for v in versions for i in v["ids"]}
        family = title if len([v for v in versions if v["label"]]) >= 2 and len(distinct) >= 2 else None
        revid = pages[title][0]
        for v in versions:
            for item_id in v["ids"]:
                if item_id in items:
                    conflicts.append({"id": item_id, "kept": owner[item_id],
                                      "dropped": {"wiki_key": title, "name": v["name"],
                                                  "variant": v["label"]}})
                    continue
                cert = noted.get(item_id)
                noted_id = None
                if v["noteable"] is True:
                    if cert is None:
                        noted_warn["noteable_without_cert"] += 1
                    else:
                        noted_id = cert
                elif cert is not None:
                    noted_warn["cert_without_noteable"] += 1
                items[item_id] = {
                    "id": item_id,
                    "name": v["name"],
                    "wiki_key": title,
                    "page_revision": revid,
                    "family": family,
                    "variant": v["label"],
                    "noted_id": noted_id,
                    "stackable": v["stackable"],
                    "noteable": v["noteable"],
                    "equipable": v["equipable"],
                    "equipment_slot": v["equipment_slot"],
                    "members": v["members"],
                    "cited_as": sorted(cited_by_page.get(title, ())),
                }
                owner[item_id] = {"wiki_key": title, "name": v["name"], "variant": v["label"]}

    out = {
        "source": {
            "wiki": "https://oldschool.runescape.wiki (Infobox Item / Infobox Bonuses wikitext via api.php)",
            "runelite_item_ids": f"https://github.com/runelite/runelite/blob/{sha}/{ITEMID_PATH}",
            "fetched_at": datetime.date.today().isoformat(),
            "script": "tools/extract_wiki_items.py",
        },
        "items": [items[k] for k in sorted(items)],
        "not_found": sorted(not_found, key=lambda e: e["text"]),
        "conflicts": sorted(conflicts, key=lambda c: (c["id"], c["dropped"]["wiki_key"])),
    }
    os.makedirs(os.path.dirname(os.path.abspath(args.out)), exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(out, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
    with open(os.path.join(args.cache_dir, "skipped_names.json"), "w", encoding="utf-8") as f:
        json.dump(skipped, f, indent=2, ensure_ascii=False)

    # Summary and sanity checks.
    print(f"wiki requests: {wiki.requests} (0 means fully cached); pages fetched: {len(pages)}, "
          f"with Infobox Item: {len(parsed)}, kept: {len(wanted)}")
    print(f"items written: {len(items)}; cited names: {len(names)}, matched: "
          f"{matched}, not found: {len(not_found)}, "
          f"skipped as non-items: {len(skipped)}")
    print(f"versions skipped (historical/removed/etc.): {stats['skipped_versions']}; "
          f"versions without name/id: {stats['versions_without_id']}; conflicts: {len(conflicts)}")
    print(f"noted id warnings: {noted_warn}; RuneLite SHA: {sha}")
    mismatches = []
    for entry in ref["items"]:
        got = items.get(entry["id"])
        if got is None:
            mismatches.append(f"  missing id {entry['id']} ({entry['name']}, wiki_key {entry.get('wiki_key')})")
        elif got["name"] != entry["name"]:
            mismatches.append(f"  id {entry['id']}: curated {entry['name']!r} vs wiki {got['name']!r}")
    print(f"curated ids checked: {len(ref['items'])}, mismatches: {len(mismatches)}")
    for line in mismatches:
        print(line)

    def check(label, ok):
        print(f"check {label}: {'OK' if ok else 'FAIL'}")
    check("Logs 1511 noted_id 1512", items.get(1511, {}).get("noted_id") == 1512)
    check("Coins 995 stackable", items.get(995, {}).get("stackable") is True)
    prayer = [i for i in items.values() if i["family"] == "Prayer potion"]
    check(f"Prayer potion family has 4 ids ({sorted(i['id'] for i in prayer)})", len(prayer) == 4)
    return 0


if __name__ == "__main__":
    sys.exit(main())
