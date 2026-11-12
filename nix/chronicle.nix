# The chronicle binary: server, CLI, both MCP servers and the Microbot agent (cmd/chronicle).
#
# The binary embeds the SPA from web/dist (web/embed.go). The Nix build does not run pnpm and a flake
# only sees tracked files, so it embeds the tracked placeholder (web/dist/.gitkeep): this package is
# what a workstation installs (CLI, `chronicle mcp`, `chronicle agent`); the server with its web is
# the Docker image.
{
  lib,
  buildGoModule,
  version,
}:
let
  root = ../.;
in
buildGoModule {
  pname = "chronicle";
  inherit version;

  # Only what the Go build reads: a change in microbot/, docs/, data/ or the web sources does not
  # rebuild the binary.
  src = lib.fileset.toSource {
    inherit root;
    fileset = lib.fileset.unions [
      (root + "/go.mod")
      (root + "/go.sum")
      (root + "/cmd")
      (root + "/internal")
      (root + "/db")
      (root + "/content/schema")
      (root + "/content/reference")
      (root + "/web/embed.go")
      (root + "/web/dist/.gitkeep")
    ];
  };

  subPackages = [ "cmd/chronicle" ];

  # How to fill vendorHash:
  #   1. set lib.fakeHash and run `nix build .#chronicle`;
  #   2. the build fails with "hash mismatch ... got: sha256-....";
  #   3. paste that `got:` value here. Repeat whenever go.sum changes (CI's nix job catches it).
  vendorHash = "sha256-LQaRdDNVDwZEqNTcqN4TB/3gJqcgIrC3hPxPO+zd86Y=";

  env.CGO_ENABLED = 0;
  # Version symbol lives in internal/buildinfo (see its package doc).
  ldflags = [
    "-s"
    "-w"
    "-X github.com/FelipeMalacarne/chronicle/internal/buildinfo.Version=${version}"
  ];

  # Store tests need a Postgres; `just test` and CI run the tests.
  doCheck = false;

  meta = {
    description = "Chronicle: server, CLI, MCP and Microbot agent in one binary";
    homepage = "https://github.com/FelipeMalacarne/chronicle";
    mainProgram = "chronicle";
    platforms = lib.platforms.unix;
  };
}
