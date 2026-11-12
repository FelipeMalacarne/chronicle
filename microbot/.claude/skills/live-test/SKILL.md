---
name: live-test
description: Test a plugin that acts in game on the real account (fights, thieving, anything that can take damage or is tick-timed) in the dev client, safely, and read what actually happened. Use with plugin-dev whenever a plugin clicks NPCs/objects under time pressure or can get the character hurt.
---

# Live testing a plugin in game

`plugin-dev` is the build/deploy loop; this is the part that touches a real account. The
character is the user's: a death, a used Ring of life or a stuck run is real damage.

## Before the first run

- **Ask.** A run that can take damage is an irreversible in-game action (AGENTS.md safety).
  Prefer the user watching. Say what the plugin will do and what the safety net is.
- **Client speed.** A hidden or minimized desktop window drops the client to ~1 FPS and every
  reaction to seconds (that is how a first run got hit down to the Ring of life). Use
  `mb-client start --headless`, or have the user keep the window visible. A plugin that can
  take damage should pause itself below ~15 FPS (`Microbot.getClient().getFPS()`).
- **Safety net outside the plugin.** Start `scripts/hp-watchdog.sh` as a Monitor before
  deploying:
  `scripts/hp-watchdog.sh <plugin fqcn> <limit> ['<escape command>']`. It stops the plugin
  and escapes (or eats) at the limit. Keep the limit well above 10% of max HP (Ring of life)
  and below the plugin's own emergency line, so the two do not race.
- **Game settings that turn skilling into combat**, e.g. auto retaliate: on, a hit makes the
  player attack back and later actions get refused "during combat". Have the plugin set them.

## Deploying again during a run

`mb-deploy` reloads the plugin and drops its state. A reload mid-fight leaves the character
standing in the fight with a plugin that does not know it is in one. Use
`scripts/deploy-when-safe.sh <pkg> [min hp]` (env `NPC`, `ATTACK_ANIM`, `SAFE_PLANE`): it waits
for a moment with HP above the line and nothing swinging, or a known safe place.

When stopping a plugin by hand, check where the character is right after: it may have just
walked back into danger.

## Reading what happened

- **Chat timeline.** Game messages are the ground truth for action results. Line them up
  against the event that matters:
  `mb observe --chat-since 0 --limit 120 | python3 -I scripts/chat-timeline.py "<anchor text>"`
  prints each message's ms since the anchor (e.g. picks at +600/+1800 after a knock-out).
- **What was clicked.** `mb observe --menu-since 0` lists the menu actions the client sent,
  including ones the plugin did not mean (an "Attack" from auto retaliate, a cancel).
- **Why.** Log the reason with every safety action ("refused during combat" vs "hit at low
  HP"); counts alone do not tell a bug from a correct reaction.
- **Log monitor.** `tail -F client.log | grep --line-buffered ... | awk '{...; fflush()}'`.
  `cut` and `head` buffer: the monitor stays silent. Logback abbreviates logger names
  differently per class (`n.r.c.p.m.f.b.X`, `n.r.c.p.m.f.blackjacker.Y`), so match both.
- **Screenshots** for overlays and positions: `mb screenshot save --dir <scratch>`.

## Tick timing and API gotchas (client 2.6.52)

See `reference.md`: slow natural-mouse clicks vs `doInvokeQuick`, clicks the game drops,
NPC follow-mode pathing and testing it offline, shops vs world hops, overlays and Guice.

## When the user corrects the behavior

They are watching the game; trust what they see over the plugin's counters. Change one thing,
redeploy safely, and measure the same window again before claiming it is fixed.
