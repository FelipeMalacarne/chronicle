#!/usr/bin/env bash
# chronicle-worker-base (docs/11 section 13.4), on a machine with Nix and Docker:
#
#   deploy/worker/build-base.sh [image]   build it; image defaults to ghcr.io/felipemalacarne/chronicle-worker-base
#   deploy/worker/build-base.sh --tag     only print the tag
#
# The tag is <Microbot version>-<sha7 of the last commit that changed an input of the base> (the
# plugins and harness in microbot/, the Nix files, this script and base.Dockerfile), so it changes
# only when the base does: CI builds and pushes it only when GHCR lacks that tag, and every
# chronicle-worker of a commit is that base plus the binary (deploy/worker/Dockerfile). Uncommitted
# changes to an input add -dirty (Nix sees only what git tracks).
#
# Building: 1. the runtime layers of nix/worker-runtime.nix (flake output worker-runtime-image) into
# docker; 2. `gradle pluginJars` in the microbot dev shell (the tests run in `just test-microbot` and
# CI); 3. base.Dockerfile: the runtime plus microbot/build/plugins/*.jar. Prints the full image
# reference on the last line; pushing is up to the caller.
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo"

inputs=(microbot ':(exclude,glob)microbot/**/*.md' nix/microbot.nix nix/worker-runtime.nix
	flake.nix flake.lock deploy/worker/base.Dockerfile deploy/worker/build-base.sh)

if [[ "$(git rev-parse --is-shallow-repository)" == true ]]; then
	echo "error: shallow clone: the tag needs the history of the base's inputs (fetch-depth: 0)" >&2
	exit 1
fi
version="$(jq -r .version microbot/microbot.json)"
sha="$(git log -1 --format=%h --abbrev=7 -- "${inputs[@]}")"
[[ -n "$sha" ]] || { echo "error: no commit touches the base's inputs" >&2; exit 1; }
[[ -z "$(git status --porcelain --untracked-files=no -- "${inputs[@]}")" ]] || sha="$sha-dirty"
tag="$version-$sha"

if [[ "${1:-}" == --tag ]]; then
	echo "$tag"
	exit 0
fi
image="${1:-ghcr.io/felipemalacarne/chronicle-worker-base}"
nix=(nix --extra-experimental-features 'nix-command flakes')

stream="$("${nix[@]}" build --no-link --print-out-paths .#worker-runtime-image)"
runtime="$("$stream" | docker load | sed -n 's/^Loaded image: //p' | tail -n1)"
[[ -n "$runtime" ]] || { echo "error: docker load printed no image name" >&2; exit 1; }
echo "runtime: $runtime" >&2

"${nix[@]}" develop .#microbot -c gradle -p microbot --console=plain -q pluginJars >&2

docker build -f deploy/worker/base.Dockerfile \
	--build-arg "RUNTIME_IMAGE=$runtime" \
	--build-arg "MICROBOT_VERSION=$version" \
	--build-arg "REVISION=$(git rev-parse HEAD)" \
	-t "$image:$tag" microbot/build/plugins >&2
echo "$image:$tag"
