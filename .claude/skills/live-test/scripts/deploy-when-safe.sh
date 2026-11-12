#!/usr/bin/env bash
# Redeploys a plugin only at a safe moment: logged in, and either on plane SAFE_PLANE (e.g. 1
# for upstairs) or HP above MIN_HP with no nearby NPC named NPC playing ATTACK_ANIM. A reload
# drops the plugin's state, so one mid-fight leaves the character standing in the fight.
# Usage: deploy-when-safe.sh <pkg> [min hp] ; env NPC, ATTACK_ANIM, SAFE_PLANE, TRIES
PKG=${1:?package}; MIN_HP=${2:-25}; NPC=${NPC:-}; ATTACK_ANIM=${ATTACK_ANIM:--2}
SAFE_PLANE=${SAFE_PLANE:--1}; TRIES=${TRIES:-120}
for i in $(seq 1 "$TRIES"); do
  ok=$({ mb skills --name Hitpoints; if [[ -n "$NPC" ]]; then mb npcs --name "$NPC" --distance 2; else echo '{"npcs":[]}'; fi; mb state; } 2>/dev/null \
    | python3 -I -c "
import sys,json
try:
    txt=sys.stdin.read(); dec=json.JSONDecoder(); i=0; o=[]
    while True:
        rest=txt[i:].lstrip()
        if not rest: break
        v,n=dec.raw_decode(rest); o.append(v); i=len(txt)-len(rest)+n
    hp=o[0]['skills'][0]['boostedLevel']; plane=o[2]['player']['position']['plane']
    hitting=any(n['animation']==$ATTACK_ANIM for n in o[1]['npcs'])
    print(1 if plane==$SAFE_PLANE or (hp>$MIN_HP and not hitting) else 0)
except Exception:
    print(0)")
  if [[ "$ok" == 1 ]]; then echo "safe after $i checks"; mb-deploy "$PKG" 2>&1 | grep -E '"success"|message|rror'; exit 0; fi
  mb observe --wait-ticks 1 --limit 1 >/dev/null 2>&1
done
echo "no safe moment, not deployed"; exit 1
