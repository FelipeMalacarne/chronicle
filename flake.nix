{
  description = "Chronicle (Ironman Planner): Go server/CLI/MCP + Svelte SPA for an OSRS ironman account";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system (import nixpkgs { inherit system; }));

      # Version baked into the binary (-X ...internal/buildinfo.Version). `git describe` is not
      # available inside the sandbox, so use the short commit when the tree is
      # clean and "dev" otherwise.
      version = if self ? shortRev then self.shortRev else "dev";

      # Go toolchain: the project targets Go 1.27 (go.mod says `go 1.27`).
      # `pkgs.go` on nixos-unstable is the latest stable, which must be >= 1.27;
      # fall back to a pinned attribute when nixpkgs still ships it.
      goFor = pkgs: pkgs.go_1_27 or pkgs.go;
      buildGoModuleFor = pkgs: pkgs.buildGoModule.override { go = goFor pkgs; };
    in
    {
      devShells = forAllSystems (system: pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            # Go
            (goFor pkgs)
            gopls
            gotools          # goimports, stringer, ...
            golangci-lint    # v2 config in .golangci.yml
            delve
            air              # live reload for `ironman serve` (.air.toml)
            sqlc             # db/queries -> internal/store/db
            goose            # db/migrations
            # Web
            nodejs_22
            pnpm
            # Infra
            postgresql_17    # psql / pg_dump clients (server runs in docker compose)
            just
            docker-compose   # `docker compose` plugin; host docker daemon required
            git
          ];

          shellHook = ''
            # Use exactly the Go from nixpkgs; never auto-download another toolchain.
            export GOTOOLCHAIN=local
            # Pure-Go project (pgx, no sqlite): static binaries, no cgo.
            export CGO_ENABLED=0
            # Keep caches outside the Nix store and out of $HOME clutter.
            export GOFLAGS="''${GOFLAGS:--mod=mod}"
            export GOPATH="''${GOPATH:-$HOME/go}"
            export PATH="$GOPATH/bin:$PATH"

            echo "ironman-planner dev shell (go $(go version | cut -d' ' -f3), node $(node --version), pnpm $(pnpm --version))"
            echo "Targets:"
            just --list --unsorted 2>/dev/null | tail -n +2 || echo "  (justfile not found)"
          '';
        };
      });

      packages = forAllSystems (system: pkgs:
        let
          chronicle = (buildGoModuleFor pkgs) {
            pname = "chronicle";
            inherit version;

            # The binary embeds the SPA from web/dist (web/embed.go). The Nix build
            # does not run pnpm and a flake only sees tracked files, so it embeds
            # the tracked placeholder (web/dist/.gitkeep): this package is the CLI,
            # `chronicle mcp` and `chronicle agent` (what a workstation installs);
            # the server with its web is the Docker image.
            src = ./.;

            subPackages = [ "cmd/chronicle" ];

            # How to fill vendorHash:
            #   1. keep lib.fakeHash and run `nix build`;
            #   2. the build fails with "hash mismatch ... got: sha256-....";
            #   3. paste that `got:` value here. Repeat whenever go.sum changes.
            vendorHash = "sha256-LQaRdDNVDwZEqNTcqN4TB/3gJqcgIrC3hPxPO+zd86Y=";

            env.CGO_ENABLED = 0;
            # Version symbol lives in internal/buildinfo (see its package doc).
            ldflags = [
              "-s"
              "-w"
              "-X github.com/FelipeMalacarne/ironman-planner/internal/buildinfo.Version=${version}"
            ];

            # The binary needs a Postgres to run tests that touch the store;
            # unit tests are run by `just test` / CI instead.
            doCheck = false;

            meta = with pkgs.lib; {
              description = "Chronicle: server, CLI, MCP and Microbot agent in one binary";
              homepage = "https://github.com/FelipeMalacarne/chronicle";
              mainProgram = "chronicle";
              platforms = platforms.unix;
            };
          };
        in
        {
          inherit chronicle;
          default = chronicle;
        });

      apps = forAllSystems (system: pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/chronicle";
        };
      });

      # For another flake (nix-config): `inputs.chronicle.overlays.default` gives
      # `pkgs.chronicle`, built with that flake's nixpkgs.
      overlays.default = final: _prev: {
        chronicle = self.packages.${final.stdenv.hostPlatform.system}.chronicle;
      };
    };
}
