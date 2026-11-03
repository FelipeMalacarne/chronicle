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
