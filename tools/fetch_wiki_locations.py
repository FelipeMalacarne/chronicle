#!/usr/bin/env python3
"""Fetch the map point of every catalog location from the OSRS Wiki.

Reads content/reference/locations.json (owner-confirmed locations, each with a
wiki_key), fetches each page's wikitext through the MediaWiki API (50 titles per
request, redirects followed) and takes the point of the infobox's
"|map = {{Map|...}}" (x=, y=, plane= or a positional "x,y" pair; the centre of
several points). Writes content/reference/location_maps.json, which the
registry loads to put the "aqui perto" steps of the Agora on the map. A page
without a map, or a map of another map id (an instance), is listed under
"unmapped" with the reason; nothing is guessed.

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
OUT = os.path.join(ROOT, "content", "reference", "location_maps.json")

MAP_RE = re.compile(r"\|\s*map\s*=\s*(\{\{\s*Map\s*\|.*?\}\})", re.IGNORECASE | re.DOTALL)
ANY_MAP_RE = re.compile(r"\{\{\s*Map\s*\|.*?\}\}", re.IGNORECASE | re.DOTALL)
PAIR_RE = re.compile(r"^\s*(\d{3,5})\s*,\s*(\d{3,5})\s*$")


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


def map_point(text):
    """(x, y, plane) of the infobox map, or (None, reason)."""
    m = MAP_RE.search(text) or ANY_MAP_RE.search(text)
    if not m:
        return None, "no map"
    tpl = m.group(1) if m.lastindex else m.group(0)
    args = [a.strip() for a in tpl.strip("{}")[3:].split("|")]
    named = {}
    points = []
    for a in args:
        if "=" in a:
            k, v = a.split("=", 1)
            named[k.strip().lower()] = v.strip()
        elif PAIR_RE.match(a):
            x, y = PAIR_RE.match(a).groups()
            points.append((int(x), int(y)))
    mapid = named.get("mapid", "")
    if mapid not in ("", "0", "-1"):
        return None, f"map id {mapid}"
    if "x" in named and "y" in named:
        try:
            points.append((int(named["x"]), int(named["y"])))
        except ValueError:
            return None, "map x/y not numbers"
    if not points:
        return None, "map without a point"
    x = round(sum(p[0] for p in points) / len(points))
    y = round(sum(p[1] for p in points) / len(points))
    plane = int(named.get("plane", "0") or 0)
    return (x, y, plane), None


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
    maps, unmapped = [], []
    for loc in sorted(locations, key=lambda l: l["id"]):
        page = pages.get(loc.get("wiki_key", ""))
        if not page:
            unmapped.append({"id": loc["id"], "reason": "no wiki page" if loc.get("wiki_key") else "no wiki_key"})
            continue
        point, why = map_point(page["text"])
        if point is None:
            unmapped.append({"id": loc["id"], "reason": why})
            continue
        x, y, plane = point
        maps.append({"id": loc["id"], "x": x, "y": y, "plane": plane, "page": page["title"], "revid": page["revid"]})
    out = {
        "catalog": "location_maps",
        "version": "2026.11.0",
        "source": "OSRS Wiki infobox maps of the pages in locations.json (tools/fetch_wiki_locations.py)",
        "notes": "Generated. The point of each location's infobox map (the centre of its points), used to list the pending steps near the character. Locations without a map, or whose map is an instance, are listed under unmapped and never placed.",
        "maps": maps,
        "unmapped": unmapped,
    }
    with open(OUT, "w") as f:
        json.dump(out, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(f"{len(maps)} mapped, {len(unmapped)} unmapped -> {os.path.relpath(OUT, ROOT)}", file=sys.stderr)


if __name__ == "__main__":
    main()
