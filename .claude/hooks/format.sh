#!/usr/bin/env bash
# Best-effort auto-format of the file Claude just edited. NEVER fails the tool call.
# Receives the PostToolUse JSON payload on stdin.
set +e
payload="$(cat)"
file="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // .tool_input.path // empty' 2>/dev/null)"
[ -z "$file" ] && exit 0
[ -f "$file" ] || exit 0

case "$file" in
  *.go)
    command -v gofmt >/dev/null 2>&1 && gofmt -w "$file" >/dev/null 2>&1
    ;;
  *.ts|*.tsx|*.js|*.jsx|*.mjs|*.cjs|*.json|*.css|*.md)
    # Use a locally-resolvable prettier only; skip silently if absent or config-less.
    root="$(git -C "$(dirname "$file")" rev-parse --show-toplevel 2>/dev/null)"
    if [ -n "$root" ] && [ -x "$root/node_modules/.bin/prettier" ]; then
      "$root/node_modules/.bin/prettier" --write "$file" >/dev/null 2>&1
    fi
    ;;
esac
exit 0
