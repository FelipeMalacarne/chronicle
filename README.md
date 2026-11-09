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

## Chronicle Link (`fm/chroniclelink`, 1.4.0)

Chronicle's plugin (the Ironman Planner webapp; called "Ironman Planner", `fm/ironmanplanner`, until
1.3). Its config group is still `fm-ironmanplanner`, so the URL, token and account survive the
rename, and `mb-install` removes the old `IronmanPlannerPlugin.jar` when it installs this one. Its
tracking is read-only: it pushes skills, quest states, the manifest's
varbits/varps and, when switched on, bank/inventory/equipment (ingest contract v1,
`ironman-planner/docs/04-arquitetura.md` 6.4), and never performs a game action.

**Control bridge (dev harness client only).** In the client `bin/mb-client` runs
(`-Dmicrobot.dev.harness=true`) the plugin also serves Chronicle's control bridge on
`.runelite/agent/bridge.sock` (`ironman-planner/docs/08-controle-microbot.md`): the real menu options
of nearby NPCs, objects and worn items, the prayer book and the spellbook, and typed actions for
them (toggle a prayer, cast a spell, Remove/Rub a worn item, Deposit-X). Every request needs the
Agent Server's token; without it nothing answers. It also keeps `.runelite/agent/config-schema.json`
with every loaded Microbot plugin's config options, for Chronicle's config form. In the everyday
client none of this starts.

**Route panel.** A side panel (toolbar icon "Chronicle") shows the planner's active
route from the cursor on, in RuneLite's look:

- header: the route's short name (Oziris, BRUHsailer; full title in the tooltip), a thin
  progress bar with done/total and blocked, a status dot with "há 12 s" (or "desatualizado";
  the full line in the tooltip), the cursor's section with its description (first line, the
  rest on click) and the guide's attribution (OzirisLoL / ironman.guide, or So Iron BRUH and
  ParasailerOSRS);
- the current step (AGORA): its full text, location, the guide's notes ("Atualização:" in
  orange, "Dica:"; hcim notes are hidden, the window does not say the account's mode),
  objectives as rows (icon, label, "feito" / "falta" / "sem dado"), a padlock row "Falta: …"
  when blocked, and the buttons **Feito**, **Pular** and, after a mark, **Desfazer**;
- the items to carry, collapsed into one summary ("5 faltam · 2 com você · 19 no banco") that
  still lists the missing ones; open it for all (a derived preview shows six more and "+N");
  the choice is remembered (hidden config `itemsExpanded`);
- the next 8 steps, one line each with a drawn icon (pending, done, presumed, skipped,
  blocked) and a marker when the step has a note (orange for an update): click one for its
  text, notes, objectives and Feito/Pular.

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
