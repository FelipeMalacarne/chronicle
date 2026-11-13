#!/usr/bin/env python3
"""Offline extract of the OSRS Wiki's NPCs, shops (with stock) and scenery objects (stdlib only).

The execution model (docs/10 sections 7 and 12) needs three entity kinds the
item catalog does not cover: `npc` (talk, train), `shop` (buy, with its stock)
and `object` (interact). They come from the wiki's Bucket tables, the same
structured data the infoboxes write:

  infobox_npc      one row per NPC page version: name, NPC ids, location text
  infobox_monster  one row per monster page version: name, NPC ids
  infobox_shop     one row per shop page: name, owner and location links
  storeline        one row per shop stock line: item, base stock, price, currency
  infobox_scenery  one row per scenery page version: object ids

Every row is fetched (`limit(500)` with `offset`, 1 s between requests) and
cached per query under --cache-dir, so a rerun is offline; --refresh bypasses
the cache. Nothing is resolved here: the rows are reduced to the fields the
catalog needs, sorted, and written under data/wiki. The Go catalog generator
(internal/core/entities/catalogbuild) derives content/reference/npcs.json,
shops.json and objects.json from them deterministically (item names of the
stock resolve against the item extract there, never here).

Outputs (sorted, 1-space indent, trailing newline):
  data/wiki/npcs.json     {"source", "npcs": [...], "monsters": [...]}
  data/wiki/shops.json    {"source", "shops": [...], "stock": [...]}
  data/wiki/objects.json  {"source", "objects": [...]}

Usage, from the repository root:
  python3 tools/extract_wiki_world.py [--refresh] [--cache-dir DIR]
"""

import argparse
import datetime
import hashlib
import json
import os
import sys
import time
import urllib.parse
import urllib.request

API = "https://oldschool.runescape.wiki/api.php"
USER_AGENT = "ironman-planner world extract (personal)"
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PAGE = 500
SLEEP = 1.0

# bucket -> selected fields (page_name and page_name_sub are always added).
BUCKETS = {
    "infobox_npc": ["npc_name", "npc_id", "location", "is_members_only", "default_version"],
    "infobox_monster": ["name", "id", "is_members_only", "default_version"],
    "infobox_shop": ["shop_name", "owner", "location", "is_members_only"],
    "storeline": ["sold_by", "sold_item", "store_sell_price", "store_stock", "store_currency"],
    "infobox_scenery": ["object_id", "is_members_only", "default_version"],
}


def query(bucket, offset, cache_dir, refresh):
    fields = ["page_name", "page_name_sub"] + BUCKETS[bucket]
    q = "bucket('%s').select(%s).limit(%d).offset(%d).run()" % (
        bucket, ",".join("'%s'" % f for f in fields), PAGE, offset)
    key = hashlib.sha256(q.encode()).hexdigest()[:16]
    path = os.path.join(cache_dir, f"{bucket}-{offset:06d}-{key}.json")
    if not refresh and os.path.exists(path):
        with open(path) as f:
            return json.load(f)
    url = API + "?" + urllib.parse.urlencode({"action": "bucket", "format": "json", "query": q})
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=120) as r:
        data = json.load(r)
    if "error" in data:
        raise SystemExit(f"{bucket} offset {offset}: {data['error']}")
    rows = data.get("bucket", [])
    os.makedirs(cache_dir, exist_ok=True)
    with open(path, "w") as f:
        json.dump(rows, f)
    time.sleep(SLEEP)
    return rows


def fetch_all(bucket, cache_dir, refresh):
    rows, offset = [], 0
    while True:
        page = query(bucket, offset, cache_dir, refresh)
        rows.extend(page)
        if len(page) < PAGE:
            return rows
        offset += PAGE


def text(v):
    """A bucket TEXT value; repeated fields come as lists."""
    if v is None:
        return ""
    if isinstance(v, list):
        return ",".join(str(x) for x in v)
    return str(v).strip()


def ints(v):
    """Positive integer ids of a repeated field (TEXT or INTEGER)."""
    out = set()
    for x in (v if isinstance(v, list) else [v] if v not in (None, "") else []):
        for part in str(x).split(","):
            part = part.strip()
            if part.isdigit() and int(part) > 0:
                out.add(int(part))
    return sorted(out)


def flag(v):
    return v is True or str(v).lower() in ("true", "1", "yes")


def version(row):
    sub = text(row.get("page_name_sub"))
    page = text(row.get("page_name"))
    return sub[len(page) + 1:] if sub.startswith(page + "#") else ""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--refresh", action="store_true")
    ap.add_argument("--cache-dir", default=os.path.expanduser("~/.cache/ironman-planner/wiki-buckets"))
    args = ap.parse_args()

    raw = {b: fetch_all(b, args.cache_dir, args.refresh) for b in BUCKETS}
    digest = hashlib.sha256(json.dumps(raw, sort_keys=True).encode()).hexdigest()
    stamp = datetime.date.today().isoformat()
    if not args.refresh:
        # Keep the date of the cached fetch stable across offline reruns.
        prev = os.path.join(ROOT, "data", "wiki", "npcs.json")
        if os.path.exists(prev):
            with open(prev) as f:
                old = json.load(f).get("source", {})
            if old.get("rows_sha256") == digest:
                stamp = old.get("fetched_at", stamp)
    source = {"wiki": API, "buckets": sorted(BUCKETS), "fetched_at": stamp, "rows_sha256": digest}

    npcs = sorted(({
        "page": text(r.get("page_name")),
        "version": version(r),
        "name": text(r.get("npc_name")),
        "ids": ints(r.get("npc_id")),
        "location": text(r.get("location")),
        "members": flag(r.get("is_members_only")),
    } for r in raw["infobox_npc"]), key=lambda x: (x["page"], x["version"], x["ids"]))
    monsters = sorted(({
        "page": text(r.get("page_name")),
        "version": version(r),
        "name": text(r.get("name")),
        "ids": ints(r.get("id")),
        "members": flag(r.get("is_members_only")),
    } for r in raw["infobox_monster"]), key=lambda x: (x["page"], x["version"], x["ids"]))
    shops = sorted(({
        "page": text(r.get("page_name")),
        "version": version(r),
        "name": text(r.get("shop_name")),
        "owner": text(r.get("owner")),
        "location": text(r.get("location")),
        "members": flag(r.get("is_members_only")),
    } for r in raw["infobox_shop"]), key=lambda x: (x["page"], x["version"]))
    stock = sorted(({
        "page": text(r.get("page_name")),
        "sold_by": text(r.get("sold_by")),
        "item": text(r.get("sold_item")),
        "price": text(r.get("store_sell_price")),
        "stock": text(r.get("store_stock")),
        "currency": text(r.get("store_currency")),
    } for r in raw["storeline"]), key=lambda x: (x["sold_by"], x["item"], x["page"], x["price"], x["stock"], x["currency"]))
    objects = sorted(({
        "page": text(r.get("page_name")),
        "version": version(r),
        "ids": ints(r.get("object_id")),
        "members": flag(r.get("is_members_only")),
    } for r in raw["infobox_scenery"]), key=lambda x: (x["page"], x["version"], x["ids"]))

    outputs = {
        "npcs.json": {"source": source, "npcs": npcs, "monsters": monsters},
        "shops.json": {"source": source, "shops": shops, "stock": stock},
        "objects.json": {"source": source, "objects": objects},
    }
    for name, doc in outputs.items():
        path = os.path.join(ROOT, "data", "wiki", name)
        with open(path, "w") as f:
            json.dump(doc, f, indent=1, ensure_ascii=False, sort_keys=True)
            f.write("\n")
        print(f"{os.path.relpath(path, ROOT)}: {os.path.getsize(path)} bytes", file=sys.stderr)
    print(f"{len(npcs)} npc rows, {len(monsters)} monster rows, {len(shops)} shops, "
          f"{len(stock)} stock lines, {len(objects)} scenery rows", file=sys.stderr)


if __name__ == "__main__":
    main()
