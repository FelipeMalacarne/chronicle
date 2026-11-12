#!/usr/bin/env bash
# Builds microbot-worker-base end to end on a machine with Nix and Docker:
#
#   worker/build.sh [image]     image defaults to ghcr.io/felipemalacarne/microbot-worker-base
#
# 1. the runtime layers of worker/image.nix (flake output worker-runtime-image) into docker;
# 2. `gradle test pluginJars` in the dev shell (the same pinned client jar);
# 3. worker/Dockerfile: the runtime plus build/plugins/*.jar, tagged <Microbot version>-<sha7>
#    (-dirty when the tree has uncommitted changes; Nix sees only files git tracks).
# Prints the full tag on the last line. Pushing is up to the caller (CI pushes on main).
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo"
image="${1:-ghcr.io/felipemalacarne/microbot-worker-base}"
nix=(nix --extra-experimental-features 'nix-command flakes')

version="$(jq -r .version microbot.json)"
sha="$(git rev-parse --short=7 HEAD)"
[[ -z "$(git status --porcelain --untracked-files=no)" ]] || sha="$sha-dirty"
tag="$version-$sha"

stream="$("${nix[@]}" build --no-link --print-out-paths .#worker-runtime-image)"
runtime="$("$stream" | docker load | sed -n 's/^Loaded image: //p' | tail -n1)"
[[ -n "$runtime" ]] || { echo "error: docker load printed no image name" >&2; exit 1; }
echo "runtime: $runtime" >&2

"${nix[@]}" develop -c gradle --console=plain -q test pluginJars >&2

docker build -f worker/Dockerfile \
	--build-arg "RUNTIME_IMAGE=$runtime" \
	--build-arg "MICROBOT_VERSION=$version" \
	--build-arg "REVISION=$(git rev-parse HEAD)" \
	-t "$image:$tag" . >&2
echo "$image:$tag"
