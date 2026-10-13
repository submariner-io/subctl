#!/usr/bin/env bash

# Verify the real Makefile's deployment prerequisites without building or deploying.
set -e -o pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
shipyard=${1:-"$repo/shipyard"}
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

for released in true false; do
    make --directory "$repo" --no-print-directory --dry-run --always-make \
        LOCAL_BUILD=1 SHIPYARD_DIR="$shipyard" RELEASED_SUBCTL="$released" SUBCTL_VERSION=v0.22.2 \
        -o images -o preload-images -o clusters -o cloud-prepare deploy > "$fixture/$released.log"
done

if grep -q 'compile.sh' "$fixture/true.log"; then
    cat "$fixture/true.log" >&2
    echo 'Published baseline deployment must not build subctl' >&2
    exit 1
fi
grep -q 'compile.sh' "$fixture/false.log"
grep -q 'deploy.sh' "$fixture/true.log"
grep -q 'deploy.sh' "$fixture/false.log"
echo 'Upgrade deployment build rules passed'
