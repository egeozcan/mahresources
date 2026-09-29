#!/bin/bash

# Rebuilds the ketchup plugin's editor bundle from a ketchup commit.
#
# plugins/ketchup/public/ketchup.js is ketchup's `npm run build:lib` output,
# committed so the plugin works on networks with no internet access. This
# builds it from a clean clone of the named ref, stamps the resolved commit
# into the file's first line, and writes it to the plugin and to its e2e copy
# (the e2e server loads plugins from e2e/test-plugins, and a Go test fails if
# the two differ). The stamp is what answers "which ketchup is this?".
#
# Usage: ./scripts/update-ketchup.sh <ref> [repository-url]
#   ref             branch, tag or commit of ketchup to build
#   repository-url  defaults to https://github.com/egeozcan/ketchup.git

set -euo pipefail
cd "$(dirname "$0")/.."

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "usage: $0 <ref> [repository-url]" >&2
  exit 2
fi
REF=$1
REPO=${2:-https://github.com/egeozcan/ketchup.git}

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

git -C "$WORK" init -q
git -C "$WORK" fetch -q --depth 1 "$REPO" "$REF"
git -C "$WORK" checkout -q FETCH_HEAD
SHA=$(git -C "$WORK" rev-parse HEAD)

(cd "$WORK" && npm ci --no-audit --no-fund --loglevel=error && npm run -s build:lib)

BUILT="$WORK/dist-lib/ketchup.js"
if [ ! -s "$BUILT" ]; then
  echo "build:lib did not produce dist-lib/ketchup.js" >&2
  exit 1
fi

# Stamp once, then copy, so the two files cannot differ and a failure midway
# leaves the committed ones untouched.
STAMPED="$WORK/ketchup.stamped.js"
# A */ in the ref or URL would end the comment early.
SAFE_REF=${REF//\*\//*_/}
SAFE_REPO=${REPO//\*\//*_/}
{ echo "/*! ketchup $SHA ($SAFE_REF) from $SAFE_REPO, built by scripts/update-ketchup.sh with node $(node --version) */"; cat "$BUILT"; } > "$STAMPED"
for target in plugins/ketchup/public/ketchup.js e2e/test-plugins/ketchup/public/ketchup.js; do
  cp "$STAMPED" "$target"
done

echo "ketchup.js updated to $SHA"
