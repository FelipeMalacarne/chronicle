# Dev shells. `default` (direnv, `nix develop`) has everything: Go, the web, the database clients
# and the Java side of microbot/ with its harness. `microbot` is only the Java side and the harness:
# what CI's plugin build and `chronicle agent` (MICROBOT_HARNESS_DIR) enter.
{
  pkgs,
  go,
  microbot,
}:
let
  inherit (pkgs) lib;

  java = [
    pkgs.jdk21
    pkgs.gradle
    pkgs.jq
    pkgs.curl
    pkgs.git
    # Java LSP for editors; .nvim.lua enables it in Neovim
    pkgs.jdt-language-server
  ]
  ++ lib.optionals pkgs.stdenv.isLinux [
    # mb-client --headless: its own X display + VNC for remote viewing
    pkgs.xvfb
    pkgs.x11vnc
    # fits the game window to the headless display
    pkgs.xdotool
  ];

  # The pinned client, read by microbot/build.gradle and the harness (microbot/bin/lib.sh).
  microbotEnv = {
    JAVA_HOME = "${pkgs.jdk21}";
    # jdtls needs the Lombok agent, or every @Getter/@Slf4j reads as an error.
    JDTLS_JVM_ARGS = "-javaagent:${pkgs.lombok}/share/java/lombok.jar";
    MICROBOT_VERSION = microbot.pin.version;
    MICROBOT_CLIENT_JAR = "${microbot.microbot-client}";
    MICROBOT_HWID_AGENT_JAR = "${microbot.hwid-agent}";
    # GPU / 117 HD load libGL.so.1 (libglvnd), which then picks the system driver from
    # /run/opengl-driver. The launcher's AppImage FHS env provides both; plain java doesn't.
    MICROBOT_LD_LIBRARY_PATH = "${pkgs.libGL}/lib:/run/opengl-driver/lib";
  };

  # The harness (mb-client, mb-deploy, mb, ...) on PATH, from the checkout the shell was entered in.
  harnessHook = ''
    _chronicle_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
    if [ -d "$_chronicle_root/microbot/bin" ]; then
      export PATH="$_chronicle_root/microbot/bin:$PATH"
    fi
    unset _chronicle_root
  '';
in
{
  default = pkgs.mkShell (
    microbotEnv
    // {
      packages = [
        # Go
        go
        pkgs.gopls
        pkgs.gotools # goimports, stringer, ...
        pkgs.golangci-lint # v2 config in .golangci.yml
        pkgs.delve
        pkgs.air # live reload for `chronicle serve` (.air.toml)
        pkgs.sqlc # db/queries -> internal/store/db
        pkgs.goose # db/migrations
        # Web
        pkgs.nodejs_22
        pkgs.pnpm
        # Infra
        pkgs.postgresql_17 # psql / pg_dump clients (the server runs in docker compose)
        pkgs.just
        pkgs.docker-compose # `docker compose` plugin; host docker daemon required
      ]
      ++ java;

      shellHook = ''
        # Use exactly the Go from nixpkgs; never auto-download another toolchain.
        export GOTOOLCHAIN=local
        # Pure-Go project (pgx, no sqlite): static binaries, no cgo.
        export CGO_ENABLED=0
        export GOFLAGS="''${GOFLAGS:--mod=mod}"
        export GOPATH="''${GOPATH:-$HOME/go}"
        export PATH="$GOPATH/bin:$PATH"
        ${harnessHook}
        echo "chronicle dev shell (go $(go version | cut -d' ' -f3), node $(node --version), java ${pkgs.jdk21.version}, Microbot ${microbot.pin.version}); just lists the targets" >&2
      '';
    }
  );

  microbot = pkgs.mkShell (
    microbotEnv
    // {
      packages = java;
      shellHook = harnessHook;
    }
  );
}
