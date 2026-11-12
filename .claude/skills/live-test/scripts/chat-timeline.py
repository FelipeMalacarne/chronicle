#!/usr/bin/env python3
"""Reads `mb observe --chat-since N --limit M` JSON on stdin and prints game chat with each
line's time relative to the last line containing ANCHOR (e.g. "render them unconscious").
Usage: mb observe --chat-since 0 --limit 120 | python3 -I chat-timeline.py "<anchor>" [skip...]"""
import json, sys
anchor = sys.argv[1] if len(sys.argv) > 1 else ""
skip = sys.argv[2:]
o = json.load(sys.stdin)
t0 = None
for c in (o.get("chat") or {}).get("messages", []):
    m = c["message"]
    if any(s in m for s in skip):
        continue
    if anchor and anchor in m:
        t0 = c["at"]
    rel = "" if t0 is None else c["at"] - t0
    print(f"{rel:>6} {c['type'][:5]} {m[:90]}")
