#!/usr/bin/env bash
set -euo pipefail

# Fixed repo as requested
OWNER_REPO="ly-schneider/comanda"

# Defaults
DEST_DIR="./dist"
GH_HOST="${GH_HOST:-github.com}"

# Interactive prompts
echo "=== Comanda POS APK Installer ==="
echo ""

# Prompt for environment
while true; do
  read -rp "Select environment (production/staging): " ENV_INPUT
  ENV_INPUT=$(echo "$ENV_INPUT" | tr '[:upper:]' '[:lower:]')
  if [[ "$ENV_INPUT" == "production" ]] || [[ "$ENV_INPUT" == "staging" ]]; then
    ENVIRONMENT="$ENV_INPUT"
    break
  else
    echo "Invalid environment. Please enter 'production' or 'staging'." >&2
  fi
done

# Prompt for version
while true; do
  read -rp "Enter version (e.g., 1.0.9): " VERSION_INPUT
  # Strip leading 'v' if present
  VERSION_INPUT="${VERSION_INPUT#v}"
  if [[ -n "$VERSION_INPUT" ]] && [[ "$VERSION_INPUT" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    VERSION="$VERSION_INPUT"
    break
  else
    echo "Invalid version format. Please use semantic versioning (e.g., 1.0.9)." >&2
  fi
done

# Construct tag and APK name
TAG="android-${ENVIRONMENT}-v${VERSION}"
APK_NAME="comanda-pos.apk"

echo ""
echo "Configuration:"
echo "  Environment: $ENVIRONMENT"
echo "  Version: $VERSION"
echo "  Tag: $TAG"
echo "  APK: $APK_NAME"
echo ""

mkdir -p "$DEST_DIR"

ensure_gh_auth() {
  if gh auth status -h "$GH_HOST" >/dev/null 2>&1; then
    echo "gh is already authenticated for $GH_HOST"
    return
  fi

  local TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
  if [ -z "${TOKEN:-}" ]; then
    echo "ERROR: No GH_TOKEN/GITHUB_TOKEN found. Export a PAT with repo read access." >&2
    echo "       Fine-grained PAT: grant Contents: Read for this repo; Classic PAT: 'repo' scope." >&2
    exit 1
  fi

  echo "Logging in gh with token from env for $GH_HOST…"
  printf '%s' "$TOKEN" | gh auth login --hostname "$GH_HOST" --with-token
}

echo "→ Ensuring GitHub auth…"
ensure_gh_auth

echo "→ Downloading APK from ${OWNER_REPO} (tag $TAG)…"
gh release download "$TAG" --repo "$OWNER_REPO" \
  --pattern "$APK_NAME" --pattern "${APK_NAME}.sha256" --clobber --dir "$DEST_DIR"

# Set APK file path
APK_FILE="$DEST_DIR/$APK_NAME"

if [ ! -f "$APK_FILE" ]; then
  echo "ERROR: APK file '$APK_NAME' not found in release '$TAG'" >&2
  echo "       Expected file: $APK_NAME" >&2
  exit 1
fi
echo "✓ APK: $APK_FILE"

# Install via ADB
echo "→ Installing on connected device (adb)…"
adb devices
adb install -r "$APK_FILE"

echo "🎉 Done."
