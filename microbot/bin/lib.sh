# Shared environment for the harness scripts. Sourced, not executed.
# shellcheck shell=bash

MB_REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MB_VERSION="${MICROBOT_VERSION:-$(jq -r .version "$MB_REPO/microbot.json")}"
MB_JAR="${MICROBOT_CLIENT_JAR:-$HOME/.microbot/microbot-${MB_VERSION}.jar}"
MB_HWID_AGENT="${MICROBOT_HWID_AGENT_JAR:-}"

# Isolated user.home for the dev client: its own .runelite (profiles, config, plugins,
# agent socket and token), so the everyday client in ~/.runelite is never touched.
MB_DEV_HOME="${MB_DEV_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/microbot-dev}"
MB_RL="$MB_DEV_HOME/.runelite"
MB_RUN="$MB_DEV_HOME/run"
MB_PIDFILE="$MB_RUN/client.pid"
MB_STDOUT="$MB_RUN/client.out"
MB_CLIENT_LOG="$MB_RL/logs/client.log"
MB_SOCKET="$MB_RL/agent/agent.sock"
MB_TOKEN_FILE="$MB_RL/.agent-token"
MB_PLUGIN_DIR="$MB_RL/microbot-plugins"
MB_BUILD_PLUGINS="$MB_REPO/build/plugins"
MB_PLUGIN_SRC="$MB_REPO/src/main/java/net/runelite/client/plugins/microbot/fm"
MB_DELOMBOK_SRC="$MB_REPO/build/delombok/net/runelite/client/plugins/microbot/fm"

die() { echo "error: $*" >&2; exit 1; }

mb_java() {
	if [[ -n "${JAVA_HOME:-}" ]]; then echo "$JAVA_HOME/bin/java"; else command -v java; fi
}

mb_gradle() {
	(cd "$MB_REPO" && gradle --console=plain -q "$@")
}

mb_client_pid() {
	[[ -f "$MB_PIDFILE" ]] || return 1
	local pid
	pid="$(cat "$MB_PIDFILE")"
	kill -0 "$pid" 2>/dev/null || return 1
	echo "$pid"
}

# curl against the dev client's Agent Server (UDS + token).
mb_api() {
	local method="$1" path="$2" body="${3:-}"
	[[ -S "$MB_SOCKET" ]] || die "agent socket missing ($MB_SOCKET); is the dev client up? try: mb-client start"
	local args=(-sS --max-time "${MB_API_TIMEOUT:-120}" --unix-socket "$MB_SOCKET"
		-H "X-Agent-Token: $(tr -d '\r\n' < "$MB_TOKEN_FILE")" -X "$method")
	if [[ -n "$body" ]]; then
		args+=(-H 'Content-Type: application/json' --data "$body")
	fi
	curl "${args[@]}" "http://127.0.0.1$path"
}

# Resolve a plugin by package dir name or class name (helloagent, HelloAgentPlugin, HelloAgent).
mb_plugin_dir() {
	local want="${1,,}" dir cls
	want="${want%plugin}"
	for dir in "$MB_PLUGIN_SRC"/*/; do
		dir="${dir%/}"
		cls="$(basename "$(ls "$dir"/*Plugin.java 2>/dev/null | head -n1)" .java)"
		[[ -n "$cls" ]] || continue
		if [[ "$(basename "$dir")" == "$want" || "${cls,,}" == "${want}plugin" ]]; then
			echo "$(basename "$dir") $cls"
			return 0
		fi
	done
	return 1
}
