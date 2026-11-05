# microbot-plugins

Personal Microbot plugins plus an agent-drivable dev harness. Plugins are sideloaded jars
built against the pinned official client; there is no client fork.

```sh
direnv allow            # or: nix develop
mb-vendor               # upstream sources/docs/CLI at the pinned version
gradle pluginJars       # build/plugins/*.jar
mb-client start         # isolated dev client with the Agent Server on
mb-deploy helloagent    # hot deploy, no restart
mb state                # microbot-cli against the dev client
```

The dev client keeps its own config in `~/.local/share/microbot-dev`. To launch it with a
Jagex Account character (the "Play Now" screen), pick one the Microbot launcher knows:
`mb-account list`, then `mb-account use <name>` and `mb-client restart`. If the session
expires, open the Microbot launcher once and run `mb-account use` again.

To use a plugin in the everyday client: `mb-install --live <plugin>` and restart it. The jar
lands in `~/.runelite/microbot-plugins` and its name goes into `.sideload-keep` there. The
client deletes non-Hub jars older than 3 days, so the launcher wrapper in nix-config
(`modules/features/applications/microbot.nix`) touches every jar listed in `.sideload-keep`
before each launch.

Commands, rules and the verification protocol for agents: [AGENTS.md](AGENTS.md).

## Ironman Planner (`fm/ironmanplanner`, 1.2.0)

Read-only tracker for the self-hosted Ironman Planner: it pushes skills, quest states, the
manifest's varbits/varps and, when switched on, bank/inventory/equipment (ingest contract v1,
`ironman-planner/docs/04-arquitetura.md` 6.4). It never performs a game action.

**Route panel.** A side panel (toolbar icon "Ironman Planner") shows the planner's active
route from the cursor on, in RuneLite's look:

- header: route name, the guide's attribution (OzirisLoL / ironman.guide, or So Iron BRUH and
  ParasailerOSRS), done/total, and the connection line ("Conectado · há 12 s", or
  "Desatualizado" with the cause when the planner cannot be reached);
- the current step (AGORA) with its full text, location, objective chips (✓ met, ✗ unmet,
  ? unknown, dashed when unknown; requirements say "requer"), a padlock and "falta …" when
  blocked, and the buttons **Feito**, **Pular** and **Desfazer** (undoes the last mark);
- the items to carry for the cursor's leg (carried, in the bank, missing, unknown) and the
  guide's bank actions; when the guide lists no setup, a preview from the next steps;
- the next 15 steps as a compact list: click one to see its text, chips and Feito/Pular.

The planner is the source of truth: verdicts, statuses and labels come from it, and the marks
are the web's marks (they show up in the web at once). The panel refetches after each
acknowledged push that moved the cursor or changed steps, after each mark, on login and every
30 s while it is visible. The last window stays on screen when the planner is down.

**Setup in dev ("só a URL").** With the planner running `just dev` on this machine
(`AUTH_MODE=dev`, serving on 127.0.0.1:8080):

1. Server URL: `http://127.0.0.1:8080`.
2. Credential: `None (dev, URL only)` (the default). No token: the planner accepts loopback
   requests and keeps this installation's sequence under the installation id the plugin
   generates once (hidden config `installationId`).
3. Conta: leave empty. The planner picks the account of the logged-in character, or its only
   account; fill in the account id (`acc_…`) only when it has several and the character's name
   matches none.
4. Send bank / inventory / equipment: switch on what the planner may read.

Tokenless requests only work over loopback in dev mode. For the compose stack, another host or
`AUTH_MODE=oidc`, choose Credential `ipl_ token` and paste a token; panel marks need its
`marks:write` scope. The token, the username and the RSN are never logged.
