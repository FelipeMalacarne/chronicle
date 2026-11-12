{
  description = "Chronicle: OSRS ironman planner (Go server, CLI, MCP and Microbot agent), its Microbot plugins and the worker image";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  # Outputs (the code of each is in nix/):
  #   packages.<system>.chronicle (default)   the binary: CLI, MCP, agent (nix/chronicle.nix)
  #   packages.<system>.microbot-client, hwid-agent, microbot-cli
  #                                           the pinned Microbot release (nix/microbot.nix)
  #   packages.x86_64-linux.worker-runtime-image
  #                                           the runtime layers of chronicle-worker-base
  #                                           (nix/worker-runtime.nix; deploy/worker/build-base.sh)
  #   devShells.<system>.default, microbot    nix/shells.nix
  #   overlays.default                        pkgs.chronicle for another flake (nix-config)
  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: lib.genAttrs systems (system: f system nixpkgs.legacyPackages.${system});

      # Version baked into the binary (-X .../internal/buildinfo.Version). `git describe` is not
      # available inside the sandbox: the short commit when the tree is clean, "dev" otherwise.
      version = self.shortRev or "dev";

      # The project targets Go 1.27 (go.mod says `go 1.27`).
      goFor = pkgs: pkgs.go_1_27 or pkgs.go;

      microbotFor = pkgs: import ./nix/microbot.nix { inherit (pkgs) fetchurl; };
    in
    {
      packages = forAllSystems (
        system: pkgs:
        let
          microbot = microbotFor pkgs;
          chronicle = pkgs.callPackage ./nix/chronicle.nix {
            inherit version;
            buildGoModule = pkgs.buildGoModule.override { go = goFor pkgs; };
          };
        in
        {
          inherit chronicle;
          inherit (microbot) microbot-client hwid-agent microbot-cli;
          default = chronicle;
        }
        // lib.optionalAttrs (system == "x86_64-linux") {
          # Streams the runtime layers to stdout (`$(nix build --print-out-paths ...) | docker load`);
          # deploy/worker/build-base.sh adds the plugin jars and tags chronicle-worker-base.
          worker-runtime-image = import ./nix/worker-runtime.nix { inherit pkgs microbot; };
        }
      );

      devShells = forAllSystems (
        _system: pkgs:
        import ./nix/shells.nix {
          inherit pkgs;
          go = goFor pkgs;
          microbot = microbotFor pkgs;
        }
      );

      apps = forAllSystems (
        system: _pkgs: {
          default = {
            type = "app";
            program = "${self.packages.${system}.chronicle}/bin/chronicle";
          };
        }
      );

      # For another flake (nix-config): `inputs.chronicle.overlays.default` gives `pkgs.chronicle`.
      overlays.default = final: _prev: {
        chronicle = self.packages.${final.stdenv.hostPlatform.system}.chronicle;
      };
    };
}
