# microbot/

Chronicle's Microbot plugins, built as sideloadable jars against the pinned official client
(`microbot.json`), and the dev harness that drives the client. No fork: the client comes from
upstream releases; only plugins live here. It was the separate `microbot-plugins` repository
until 2026-11-12 (history kept); the planner's rules are in the root `CLAUDE.md`, its design docs
in `docs/` (06 the plugin, 08 the control, 11 the workers).

Paths below are relative to `microbot/`. The root dev shell (direnv, `nix develop`) has the JDK,
Gradle and the harness `bin/` on PATH; `nix develop .#microbot` is that part alone. Run Gradle here
or from the root with `gradle -p microbot` (`just plugins`, `just test-microbot`).

## Layout
- `src/main/java/net/runelite/client/plugins/microbot/fm/<pkg>/` — one self-contained package
  per plugin, with exactly one `*Plugin.java`. The package must stay under
  `net.runelite.client.plugins.microbot` (the client filters plugin lists and logs by it).
- `fm/helloagent/` — template plugin. Copy it to start a new one.
- `fm/devbootstrap/` — starts the Agent Server in the dev client. Dev home only, never `--live`.
- `fm/chroniclelink/` — Chronicle Link, Chronicle's plugin (was `fm/ironmanplanner`; config group
  still `fm-ironmanplanner`): read-only tracking everywhere; in the dev harness client
  only, also the control bridge (`.runelite/agent/bridge.sock`), the config-options export and
  (1.15.0) the task executor: a state machine for one typed task (travel, bank, buy) that only
  the planner's agent starts through the bridge (`TaskMachine`, tested over the `TaskGame` fake).
- `fm/catswap/` — Cat Swap: one swap, then it stops itself (outcome in its script results):
  sells the grown cat to a West Ardougne Civilian, buys a kitten from Gertrude, lets it follow.
  Chronicle's routine executor runs it between the main activity and the return (docs/08).
- `bin/` — harness (on PATH inside the dev shell).
- The worker image (Chronicle's managed workers, docs/11 section 13.4) is built outside this
  directory: `nix/worker-runtime.nix`, `deploy/worker/` (see "Worker image").
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

Prefer `mb-client start --headless` for anything that runs unattended: the game gets its own
Xvfb display (`:99`) shared over VNC on `127.0.0.1:5999`, GPU and 117 HD are switched off
(no GPU there) and Microbot's onboarding tour is skipped (`-Dmicrobot.tour=false`). A desktop
window that is hidden or minimized throttles the client to about one frame per second, which
makes every action, walk and wait crawl (measured: Agent Server calls ~1000 ms windowed-hidden
vs ~18 ms headless, logged in).

| Command | What it does |
|---|---|
| `mb-client start [--headless]\|stop\|restart\|status\|logs [N]` | Run the dev client; `start` waits for the Agent Server; `--headless` uses Xvfb + VNC |
| `mb-account list\|use <name\|#>\|show\|clear` | Jagex Account character the dev client launches with (from the launcher's accounts) |
| `mb-deploy <plugin>` | Hot compile + load (or reload) a plugin, no restart. `--undeploy`, `--list` |
| `mb <cli args>` | Upstream `microbot-cli` against the dev client (`mb state`, `mb npcs --name X`, `mb screenshot save --dir D`) |
| `mb-install [plugin…]` | Build jars into the dev home (restart to load). `--live` installs into the everyday client |
| `mb-test <plugin>` | Test-mode run: login, start plugin, wait for `result.json`, exit code 0/1/2/3/4 |
| `gradle pluginJars` | Build all jars to `build/plugins/` (`just plugins` from the root) |
| `mb-bump <version>` | Pin a new client release and re-vendor |

`mb-client` reads these, all optional; unset, it behaves as on the workstation:

| Variable | Default | What it does |
|---|---|---|
| `MB_DEV_HOME` | `~/.local/share/microbot-dev` | the client's `user.home` |
| `MB_XMX` | `2g` | java `-Xmx` |
| `MB_HWID_STORE_DIR` | `~/.runelite` | dir of the HWID agent's `spoofed-device-ids.properties` and `spoofed-random-dats.properties` |
| `MB_PREBUILT_PLUGINS` | unset | dir of prebuilt jars: `start` installs all of them (must include `DevBootstrapPlugin.jar` and `ChronicleLinkPlugin.jar`) instead of building DevBootstrap with gradle |
| `MICROBOT_CLI` | `vendor/microbot/microbot-cli` | the upstream CLI `mb` runs |

PipeWire muting and the xdotool window fit are skipped when those tools are missing.

Iterate with `mb-deploy`; confirm with `mb-test` (or `mb-install` + `mb-client restart`).
A plugin cannot be hot-deployed while its jar is sideloaded in the dev home.

## Worker image
`deploy/worker/build-base.sh` (`just worker-base`) builds
`ghcr.io/felipemalacarne/chronicle-worker-base:<Microbot version>-<sha7>` on a machine with Nix
and Docker: it loads the Nix runtime (`nix/worker-runtime.nix`: JDK 21 of the dev shell's nixpkgs,
Xvfb, x11vnc, xdotool, the pinned jars and microbot-cli, this `bin/` in `/opt/microbot/bin`, user
65532, `HOME=/home/worker`), runs `gradle pluginJars` and adds `build/plugins/*.jar` in
`/opt/microbot/plugins` (`MB_PREBUILT_PLUGINS`). The sha7 is the last commit that changed an input
of the base (this directory except `*.md`, the Nix files, the base's Dockerfile and script), so a
change here is a new base. No entrypoint: `chronicle-worker` (`deploy/worker/Dockerfile`, the base
plus the chronicle binary) sets its own. Nix sees only files git tracks, so `git add` new files
before building. CI (root `.github/workflows/ci.yml`, job `nix`) runs `gradle test pluginJars` on
every push and PR; on `main` the `docker` job builds the base when GHCR lacks its tag, then
`chronicle-worker`, and the deploy commit points the saradomin overlay at it. Running workers
switch only on the owner's restart.

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
