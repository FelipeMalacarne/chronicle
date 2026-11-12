{
  description = "Ironman Planner: Go server/CLI/MCP + Svelte SPA for an OSRS ironman account";

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
          ironman = (buildGoModuleFor pkgs) {
            pname = "ironman";
            inherit version;

            # IMPORTANT: `cmd/ironman` embeds the SPA from web/dist via web/embed.go.
            # The Nix build does not run pnpm, so web/dist must exist in the source
            # tree before `nix build` (run `just web-build` first). Note that a
            # flake built from a git checkout only sees tracked files, so for a
            # local build use `nix build path:.` or commit the dist in a release
            # branch; the Dockerfile is the fully reproducible path.
            src = ./.;

            subPackages = [ "cmd/ironman" ];

            # How to fill vendorHash:
            #   1. keep lib.fakeHash and run `nix build`;
            #   2. the build fails with "hash mismatch ... got: sha256-....";
            #   3. paste that `got:` value here. Repeat whenever go.sum changes.
            vendorHash = pkgs.lib.fakeHash;

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
              description = "Ironman Planner server, CLI and MCP in one binary";
              homepage = "https://github.com/FelipeMalacarne/chronicle";
              mainProgram = "ironman";
              platforms = platforms.unix;
            };
          };
        in
        {
          inherit ironman;
          default = ironman;
        });

      apps = forAllSystems (system: pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/ironman";
        };
      });
    };
}
