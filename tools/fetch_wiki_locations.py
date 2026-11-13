#!/usr/bin/env python3
"""Fetch the infobox map of every catalog location from the OSRS Wiki.

Reads content/reference/locations.json (owner-confirmed locations, each with a
wiki_key), fetches each page's wikitext through the MediaWiki API (50 titles per
request, redirects followed) and keeps the infobox's "|map = {{Map|...}}"
template (or the page's first {{Map}}) as it is. Writes data/wiki/location_maps.json;
a page without a map, or a location without a page, is kept with the reason.
Nothing is interpreted here: internal/core/entities/catalogbuild reads the
templates and derives content/reference/location_maps.json (the map point of
each location, which the Agora's "aqui perto" uses, and its area in tiles,
where a travel task ends; docs/10 section 7), with
`go test ./internal/core/entities/catalogbuild -update`.

The wikitext is cached under --cache-dir, so reruns do not hit the network;
--refresh bypasses the cache.

  python3 tools/fetch_wiki_locations.py [--refresh] [--cache-dir DIR]
"""

import argparse
import hashlib
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request

API = "https://oldschool.runescape.wiki/api.php"
USER_AGENT = "ironman-planner location extract (personal, github FelipeMalacarne)"
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CATALOG = os.path.join(ROOT, "content", "reference", "locations.json")
OUT = os.path.join(ROOT, "data", "wiki", "location_maps.json")

MAP_RE = re.compile(r"\|\s*map\s*=\s*(\{\{\s*Map\s*\|.*?\}\})", re.IGNORECASE | re.DOTALL)
ANY_MAP_RE = re.compile(r"\{\{\s*Map\s*\|.*?\}\}", re.IGNORECASE | re.DOTALL)


def fetch(titles, cache_dir, refresh):
    """wikitext by requested title, following redirects; cached per batch."""
    key = hashlib.sha256("\n".join(titles).encode()).hexdigest()[:16]
    path = os.path.join(cache_dir, f"locations-{key}.json")
    if not refresh and os.path.exists(path):
        with open(path) as f:
            return json.load(f)
    params = {
        "action": "query", "format": "json", "prop": "revisions", "rvprop": "content|ids",
        "rvslots": "main", "redirects": "1", "titles": "|".join(titles),
    }
    req = urllib.request.Request(API + "?" + urllib.parse.urlencode(params), headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=60) as r:
        data = json.load(r)
    q = data.get("query", {})
    alias = {}
    for n in q.get("normalized", []):
        alias[n["to"]] = n["from"]
    for rd in q.get("redirects", []):
        alias[rd["to"]] = alias.get(rd["from"], rd["from"])
    out = {}
    for p in q.get("pages", {}).values():
        if "revisions" not in p:
            continue
        rev = p["revisions"][0]
        asked = alias.get(p["title"], p["title"])
        out[asked] = {"title": p["title"], "revid": rev["revid"], "text": rev["slots"]["main"]["*"]}
    os.makedirs(cache_dir, exist_ok=True)
    with open(path, "w") as f:
        json.dump(out, f)
    time.sleep(1)
    return out


def infobox_map(text):
    """The infobox map template, else the page's first {{Map}}, else None."""
    m = MAP_RE.search(text)
    if m:
        return m.group(1)
    m = ANY_MAP_RE.search(text)
    return m.group(0) if m else None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--refresh", action="store_true")
    ap.add_argument("--cache-dir", default=os.path.expanduser("~/.cache/ironman-planner/wiki-locations"))
    args = ap.parse_args()
    with open(CATALOG) as f:
        locations = json.load(f)["locations"]
    keyed = [loc for loc in locations if loc.get("wiki_key")]
    pages = {}
    for i in range(0, len(keyed), 50):
        titles = [loc["wiki_key"] for loc in keyed[i:i + 50]]
        pages.update(fetch(titles, args.cache_dir, args.refresh))
    out = []
    for loc in sorted(locations, key=lambda l: l["id"]):
        if not loc.get("wiki_key"):
            out.append({"id": loc["id"], "reason": "no wiki_key"})
            continue
        page = pages.get(loc["wiki_key"])
        if not page:
            out.append({"id": loc["id"], "reason": "no wiki page"})
            continue
        entry = {"id": loc["id"], "page": page["title"], "revid": page["revid"]}
        tpl = infobox_map(page["text"])
        if tpl is None:
            entry["reason"] = "no map"
        else:
            entry["map"] = tpl
        out.append(entry)
    doc = {
        "source": {"wiki": API, "catalog": "content/reference/locations.json"},
        "locations": out,
    }
    with open(OUT, "w") as f:
        json.dump(doc, f, indent=1, ensure_ascii=False)
        f.write("\n")
    n = sum(1 for e in out if "map" in e)
    print(f"{n} maps, {len(out) - n} without -> {os.path.relpath(OUT, ROOT)}", file=sys.stderr)


if __name__ == "__main__":
    main()
