#!/usr/bin/env bash
# Provision a fresh git worktree with the gitignored, machine-local toolchain files
# it can't carry: web/node_modules (symlinked to the main checkout) and
# android/local.properties (sdk.dir). Idempotent; no-op in the main repo and when
# already provisioned. SessionStart hook — NEVER fails the session.
set +e

common="$(git rev-parse --git-common-dir 2>/dev/null)" || exit 0
main="$(cd "$(dirname "$common")" 2>/dev/null && pwd)" || exit 0
here="$(git rev-parse --show-toplevel 2>/dev/null)" || exit 0
[ -z "$main" ] && exit 0
[ "$here" = "$main" ] && exit 0   # main repo, not a linked worktree

# web/node_modules → symlink to the main checkout (same commit base, so the dependencies match).
if [ -d "$main/web/node_modules" ] && [ ! -e "$here/web/node_modules" ]; then
  ln -s "$main/web/node_modules" "$here/web/node_modules" && echo "[setup-worktree] linked web/node_modules"
fi

# android/local.properties → sdk.dir (copy main's, else $ANDROID_HOME, else macOS default).
if [ ! -f "$here/android/local.properties" ] && [ -d "$here/android" ]; then
  if [ -f "$main/android/local.properties" ]; then
    cp "$main/android/local.properties" "$here/android/local.properties" && echo "[setup-worktree] copied android/local.properties"
  elif [ -n "${ANDROID_HOME:-}" ]; then
    printf 'sdk.dir=%s\n' "$ANDROID_HOME" > "$here/android/local.properties" && echo "[setup-worktree] wrote android/local.properties (ANDROID_HOME)"
  elif [ -d "$HOME/Library/Android/sdk" ]; then
    printf 'sdk.dir=%s\n' "$HOME/Library/Android/sdk" > "$here/android/local.properties" && echo "[setup-worktree] wrote android/local.properties (default SDK)"
  fi
fi

exit 0
