#!/usr/bin/env python3
"""Fetch the game icons the web shows (decision 8 of docs/ui v4).

Copies into web/static/icons the inventory icon of every item the guides cite
(guide.json and annotation sidecars), the 24 skill icons and the quest point
icon, all from the Old School RuneScape Wiki, and writes the manifest the SPA
imports (web/src/lib/icons/manifest.json). The images are © Jagex, served by
the OSRS Wiki under CC BY-NC-SA 3.0; the manifest carries that attribution and
the web shows it on every screen with icons.

Item icon file names come from the `image`/`imageN` fields of {{Infobox Item}}
in the wikitext cached by tools/extract_wiki_items.py (read only; run that
first). An item without a cached infobox falls back to "<name>.png".

File URLs come from the MediaWiki imageinfo API (50 titles per call) and the
images from the wiki's /images/ path. Both are cached under --cache-dir so
reruns do not hit the network; --refresh bypasses the cache. Every download
must be a PNG under 256 KiB.

Usage:
  python3 tools/fetch_wiki_icons.py [--refresh] [--cache-dir DIR] [--wiki-cache DIR]
"""

import argparse
import hashlib
import json
import os
import re
import shutil
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
API = "https://oldschool.runescape.wiki/api.php"
USER_AGENT = "ironman-planner icon fetch (personal, github FelipeMalacarne)"
PNG = b"\x89PNG\r\n\x1a\n"
MAX_BYTES = 256 * 1024
MAX_SIDE = 64
ATTRIBUTION = "Imagens do jogo © Jagex via Old School RuneScape Wiki, CC BY-NC-SA 3.0"


def load_json(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def cited_items():
    """Every item id cited by a guide or an annotation sidecar."""
    ids = set()

    def walk(o):
        if isinstance(o, dict):
            for k, v in o.items():
                if k == "item_id" and isinstance(v, int):
                    ids.add(v)
                elif k == "any_of" and isinstance(v, list):
                    ids.update(x for x in v if isinstance(x, int))
                elif k in ("ref", "key") and isinstance(v, str):
                    m = re.match(r"item:(\d+)", v)
                    if m:
                        ids.add(int(m.group(1)))
                walk(v)
        elif isinstance(o, list):
            for x in o:
                walk(x)

    guides = os.path.join(ROOT, "content", "guides")
    for gid in sorted(os.listdir(guides)):
        gdir = os.path.join(guides, gid)
        files = [os.path.join(gdir, "guide.json")]
        adir = os.path.join(gdir, "annotations")
        if os.path.isdir(adir):
            files += [os.path.join(adir, n) for n in sorted(os.listdir(adir)) if n.endswith(".json")]
        for f in files:
            if os.path.exists(f):
                walk(load_json(f))
    return ids


def infobox_images(wiki_cache):
    """item id -> icon file name, from the cached {{Infobox Item}} wikitext."""
    out = {}
    if not os.path.isdir(wiki_cache):
        return out
    for name in sorted(os.listdir(wiki_cache)):
        if not name.endswith(".json"):
            continue
        batch = load_json(os.path.join(wiki_cache, name))
        for resp in batch.get("responses", []):
            for page in resp.get("query", {}).get("pages", []) or []:
                for rev in page.get("revisions", [])[:1]:
                    text = rev.get("slots", {}).get("main", {}).get("content") or rev.get("content") or ""
                    absorb_infobox(text, out)
    return out


FIELD = re.compile(r"^\|\s*(id|image)(\d*)\s*=\s*(.*?)\s*$", re.M)
FILE = re.compile(r"\[\[File:([^|\]]+)")


def absorb_infobox(text, out):
    ids, images = {}, {}
    for kind, version, value in FIELD.findall(text):
        if kind == "id":
            ids[version] = [int(x) for x in re.findall(r"\d+", value)]
        else:
            m = FILE.search(value)
            if m:
                images[version] = m.group(1).strip()
    for version, values in ids.items():
        image = images.get(version) or images.get("")
        if image:
            for item_id in values:
                out.setdefault(item_id, image)


def resolve_urls(titles, cache_dir, refresh):
    """File title -> image URL via the imageinfo API (50 titles per call),
    cached in urls.json; missing files map to None."""
    path = os.path.join(cache_dir, "urls.json")
    known = {} if refresh or not os.path.exists(path) else load_json(path)
    todo = sorted(t for t in titles if t not in known)
    for i in range(0, len(todo), 50):
        batch = todo[i:i + 50]
        query = urllib.parse.urlencode({
            "action": "query", "format": "json", "formatversion": "2", "prop": "imageinfo",
            "iiprop": "url|mime|size", "titles": "|".join("File:" + t for t in batch),
        })
        data = json.loads(get(API + "?" + query))
        normalized = {n["to"]: n["from"] for n in data.get("query", {}).get("normalized", [])}
        for page in data.get("query", {}).get("pages", []):
            title = normalized.get(page["title"], page["title"])
            title = title[len("File:"):] if title.startswith("File:") else title
            info = (page.get("imageinfo") or [{}])[0]
            # Inventory icons are at most 36x32; anything larger is a detail
            # or "equipped" image picked up from another infobox field.
            ok = (not page.get("missing") and info.get("mime") == "image/png" and info.get("size", 0) <= MAX_BYTES
                  and info.get("width", 0) <= MAX_SIDE and info.get("height", 0) <= MAX_SIDE)
            known[title] = info.get("url") if ok else None
        for t in batch:
            known.setdefault(t, None)
        time.sleep(0.5)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(known, f, ensure_ascii=False, indent="\t", sort_keys=True)
    return known


def get(url):
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return resp.read(MAX_BYTES + 1)
        except (urllib.error.HTTPError, urllib.error.URLError):
            if attempt == 2:
                raise
            time.sleep(2 * (attempt + 1))
    return b""


def fetch(title, url, cache_dir, refresh):
    """Return the PNG bytes of a wiki file (cached), or None."""
    if url is None:
        return None
    key = hashlib.sha1(title.encode("utf-8")).hexdigest()
    path = os.path.join(cache_dir, key + ".png")
    if not refresh and os.path.exists(path):
        with open(path, "rb") as f:
            return f.read()
    data = get(url)
    time.sleep(0.1)
    if len(data) > MAX_BYTES or not data.startswith(PNG):
        return None
    with open(path, "wb") as f:
        f.write(data)
    return data


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--refresh", action="store_true")
    ap.add_argument("--cache-dir", default=os.path.expanduser("~/.cache/ironman-planner/icons"))
    ap.add_argument("--wiki-cache", default=os.path.expanduser("~/.cache/ironman-planner/wiki-items/wiki"))
    ap.add_argument("--out", default=os.path.join(ROOT, "web", "static", "icons"))
    ap.add_argument("--manifest", default=os.path.join(ROOT, "web", "src", "lib", "icons", "manifest.json"))
    args = ap.parse_args()
    os.makedirs(args.cache_dir, exist_ok=True)

    catalog = {i["id"]: i for i in load_json(os.path.join(ROOT, "content", "reference", "items.json"))["items"]}
    skills = load_json(os.path.join(ROOT, "content", "reference", "skills.json"))["skills"]
    images = infobox_images(args.wiki_cache)
    if not images:
        print(f"warning: no cached infobox wikitext under {args.wiki_cache}; using <name>.png", file=sys.stderr)

    wanted = {}  # (kind, id) -> candidate wiki file titles, best first
    for item_id in sorted(cited_items()):
        entry = catalog.get(item_id)
        if entry is None:
            continue
        candidates = [images[item_id]] if item_id in images else []
        candidates.append(entry["name"] + ".png")
        wanted[("item", str(item_id))] = list(dict.fromkeys(candidates))
    for s in skills:
        wanted[("skill", s["id"])] = [s["name"] + " icon.png"]
    wanted[("misc", "quest_point")] = ["Quest point icon.png"]

    urls = resolve_urls({t for ts in wanted.values() for t in ts}, args.cache_dir, args.refresh)
    staging = args.out + ".tmp"
    shutil.rmtree(staging, ignore_errors=True)
    manifest = {"attribution": ATTRIBUTION, "source": "https://oldschool.runescape.wiki", "item": {}, "skill": {}, "misc": {}, "missing": []}
    for (kind, ident), titles in sorted(wanted.items()):
        title, data = titles[0], None
        for candidate in titles:
            data = fetch(candidate, urls.get(candidate), args.cache_dir, args.refresh)
            if data is not None:
                title = candidate
                break
        if data is None:
            manifest["missing"].append(f"{kind}:{ident} ({' | '.join(titles)})")
            continue
        os.makedirs(os.path.join(staging, kind), exist_ok=True)
        with open(os.path.join(staging, kind, ident + ".png"), "wb") as f:
            f.write(data)
        manifest[kind][ident] = title
    shutil.rmtree(args.out, ignore_errors=True)
    os.replace(staging, args.out)
    os.makedirs(os.path.dirname(args.manifest), exist_ok=True)
    with open(args.manifest, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent="\t", sort_keys=True)
        f.write("\n")
    total = sum(len(manifest[k]) for k in ("item", "skill", "misc"))
    print(f"wrote {total} icons to {args.out} ({len(manifest['missing'])} missing); manifest {args.manifest}")


if __name__ == "__main__":
    main()
