#!/usr/bin/env bash
# Independent HP safety net for a live test. Run it as a Monitor (each stdout line is an event).
# Prints "low hp N" at or below REPORT_AT (default 18); at or below LIMIT stops the plugin, then
# runs ESCAPE (a shell command, e.g. 'mb objects interact Ladder Climb-up') or, without one,
# drinks/eats FOOD (default "Jug of wine"), and exits 1.
# Usage: hp-watchdog.sh <plugin fqcn> <limit> [escape command]
CLS=${1:?plugin class}; LIMIT=${2:?hp limit}; ESCAPE=${3:-}
FOOD=${FOOD:-Jug of wine}; ACTION=${ACTION:-Drink}; REPORT_AT=${REPORT_AT:-18}
last=""
while true; do
  hp=$(mb skills --name Hitpoints 2>/dev/null | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["skills"][0]["boostedLevel"])' 2>/dev/null)
  # 0 is what the client reports while logged out (a restart, a hop), not a hit.
  if [[ -z "$hp" || "$hp" == 0 ]]; then sleep 0.5; continue; fi
  if [[ "$hp" != "$last" ]] && (( hp <= REPORT_AT )); then echo "low hp $hp"; fi; last=$hp
  if (( hp <= LIMIT )); then
    mb scripts stop --class "$CLS" >/dev/null 2>&1
    if [[ -n "$ESCAPE" ]]; then eval "$ESCAPE" >/dev/null 2>&1; act="ran: $ESCAPE"
    else mb inventory interact "$FOOD" "$ACTION" >/dev/null 2>&1; act="$ACTION $FOOD"; fi
    echo "WATCHDOG: HP $hp -> stopped plugin, $act"
    exit 1
  fi
  sleep 0.4
done
