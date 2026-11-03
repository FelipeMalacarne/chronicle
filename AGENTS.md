# microbot-plugins

Personal Microbot plugins, built as sideloadable jars against the pinned official client
(`microbot.json`). No fork: the client comes from upstream releases; only plugins live here.

## Layout
- `src/main/java/net/runelite/client/plugins/microbot/fm/<pkg>/` — one self-contained package
  per plugin, with exactly one `*Plugin.java`. The package must stay under
  `net.runelite.client.plugins.microbot` (the client filters plugin lists and logs by it).
- `fm/helloagent/` — template plugin. Copy it to start a new one.
- `fm/devbootstrap/` — starts the Agent Server in the dev client. Dev home only, never `--live`.
- `bin/` — harness (on PATH inside `nix develop`/direnv).
- `vendor/microbot/` — sparse upstream checkout at the pinned tag (`mb-vendor`), gitignored.
  This is the API you compile against: read it instead of guessing.

## Rules
- Read `vendor/microbot/AGENTS.md` and
  `vendor/microbot/runelite-client/src/main/java/net/runelite/client/plugins/microbot/AGENTS.md`
  before writing plugin logic. Its non-negotiables apply here: caches via
  `Microbot.getRs2XxxCache().query()`, never block the client thread, `sleepUntil` not static sleeps.
- Before relying on an entity/util assumption, check `vendor/microbot/docs/entity-guides/`.
- Java 11 source level. A plugin package must compile on its own (no cross-package imports):
  hot deploys compile one package against the client classpath only.
- Lombok works (sources are delomboked for hot deploys), but nothing beyond the client jar
  and Lombok is available at runtime.
- Jar name == plugin class simple name. Never reuse a Plugin Hub plugin's class name: the
  client would overwrite the jar with the Hub's.
- Bump `version` in the `@PluginDescriptor` when behavior changes.

## Harness (dev client)
The dev client is the pinned jar running with `user.home=~/.local/share/microbot-dev`, so it
has its own profiles/config and never touches the everyday client in `~/.runelite`. The Agent
Server listens on a Unix socket inside that home. Like the launcher, it attaches the release's
HWID agent; its per-account stores are shared with the everyday client in `~/.runelite`.
`mb-client start` comes up logged out (AutoLogin is switched off); log in with
`mb login now --world <w>` only after the user agrees. `mb-test` logs in by itself.

| Command | What it does |
|---|---|
| `mb-client start\|stop\|restart\|status\|logs [N]` | Run the dev client; `start` waits for the Agent Server |
| `mb-account list\|use <name\|#>\|show\|clear` | Jagex Account character the dev client launches with (from the launcher's accounts) |
| `mb-deploy <plugin>` | Hot compile + load (or reload) a plugin, no restart. `--undeploy`, `--list` |
| `mb <cli args>` | Upstream `microbot-cli` against the dev client (`mb state`, `mb npcs --name X`, `mb screenshot save --dir D`) |
| `mb-install [plugin…]` | Build jars into the dev home (restart to load). `--live` installs into the everyday client |
| `mb-test <plugin>` | Test-mode run: login, start plugin, wait for `result.json`, exit code 0/1/2/3/4 |
| `gradle pluginJars` | Build all jars to `build/plugins/` |
| `mb-bump <version>` | Pin a new client release and re-vendor |

Iterate with `mb-deploy`; confirm with `mb-test` (or `mb-install` + `mb-client restart`).
A plugin cannot be hot-deployed while its jar is sideloaded in the dev home.

## Verifying behavior
- Verify game effects, not config values: query state with `mb` (inventory, npcs, widgets,
  position) and take screenshots (`mb screenshot save --dir <dir>`), then read the PNG.
- Plugins report through `Microbot.status`, `ScriptResultStore.submit(...)` (read with
  `mb scripts results --class <fqcn>`) and `/scripts/health` heartbeats (automatic in `Script`).
- In test mode the plugin writes its own `TestResult` and exits (see `HelloAgentScript`).
- Logs: `mb-client logs 200` (client.log of the dev home).

## Safety
- Being logged in means acting on a real account. Use only the character set with
  `mb-account` (check `mb-account show`); never switch it yourself. Ask before logging in,
  world hopping, trading, or anything irreversible in game.
- One character cannot be logged in from two clients: logging in the dev client kicks the
  same character out of the everyday client.
- Never `mb-install --live` without the user asking.
- Never print or commit `.agent-token`, profiles or credentials from the dev home.
