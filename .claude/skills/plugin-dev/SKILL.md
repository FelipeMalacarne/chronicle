---
name: plugin-dev
description: Build or change a Microbot plugin in this repo and verify it in the running dev client. Use when asked to create a plugin, change plugin behavior, or test/confirm a plugin works in game.
---

# Plugin dev loop

Follow `AGENTS.md`. The goal is behavior confirmed in the dev client, not code that compiles.

1. **Orient.** `mb-vendor` (no-op if current). Read the upstream AGENTS.md files and the
   util/API classes you will call under `vendor/microbot/`. Check entity guides for the
   entities involved.
2. **Scaffold.** New plugin: copy `fm/helloagent/` to `fm/<pkg>/`, rename the classes,
   config group and descriptor name. Keep the package self-contained.
3. **Compile.** `gradle pluginJars`. Fix errors before touching the client.
4. **Client.** `mb-client status`; if down, `mb-client start`. Check login with `mb state`.
   If logged out, ask the user before logging in. `mb-account show` names the character;
   if it says none, ask the user which character to use (`mb-account list`).
5. **Deploy.** `mb-deploy <pkg>`. On compile errors, fix and redeploy. Start/stop with
   `mb scripts start --name "<name>"` / `mb scripts stop --name "<name>"` when the plugin
   is not enabled on deploy.
6. **Observe.** Drive and read the game with `mb` (state, inventory, npcs, objects,
   widgets, dialogue), `mb scripts results --class <fqcn>`, `mb scripts health`,
   `mb-client logs 200`, and `mb screenshot save --dir <scratch dir>` then read the PNG.
   Judge the in-game effect, e.g. items actually moved, player actually arrived.
7. **Iterate.** Edit, `mb-deploy <pkg>` (reloads), observe again. If the same approach
   fails three times, stop and report what you saw.
8. **Confirm.** `mb-client stop`, then `mb-test <pkg>` for a clean end-to-end run. Report
   the `result.json` checks and the evidence (state output, screenshot paths).

If an `mb` command itself misbehaves, say so with the raw output instead of working around
it silently: it may be an upstream bug in the pinned version.
