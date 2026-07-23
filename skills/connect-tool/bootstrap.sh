#!/usr/bin/env bash
# bootstrap.sh - install the connect-tool Skill on macOS, no Git required.
#
# Downloads the repo tarball over HTTPS, extracts ONLY skills/connect-tool, and
# installs it to ~/.claude/skills/connect-tool. Then reports every dependency and
# stops at the one step a script cannot do for you: approving the Chrome extension.
#
#   curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connect-tool/bootstrap.sh | bash
#
# Env vars:
#   MSP_SKILLS_OWNER / MSP_SKILLS_REPO / MSP_SKILLS_REF   override the source
#   INSTALL_DIR                                           override the destination
#   DRY_RUN=1                                             print the plan and exit
set -euo pipefail

OWNER=${MSP_SKILLS_OWNER:-servosity}
REPO=${MSP_SKILLS_REPO:-msp-skills}
REF=${MSP_SKILLS_REF:-main}
INSTALL_DIR=${INSTALL_DIR:-$HOME/.claude/skills/connect-tool}
EXTENSION_URL="https://chromewebstore.google.com/detail/opencli/ildkmabpimmkaediidaifkhjpohdnifk"
TARBALL="https://codeload.github.com/$OWNER/$REPO/tar.gz/refs/heads/$REF"

echo "connect-tool bootstrap"
echo "  source:      $OWNER/$REPO@$REF"
echo "  destination: $INSTALL_DIR"

[ "${DRY_RUN:-}" = "1" ] && { echo "DRY_RUN=1 set; not downloading."; exit 0; }

# --- 1. Fetch the Skill ------------------------------------------------------
# A tarball, not a clone, so this works with no Git installed.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "  fetching $TARBALL"
curl -fsSL "$TARBALL" -o "$tmp/repo.tar.gz"
mkdir -p "$tmp/x"
tar -xzf "$tmp/repo.tar.gz" -C "$tmp/x"
src=$(find "$tmp/x" -maxdepth 3 -type d -path '*/skills/connect-tool' | head -1)
[ -n "$src" ] || { echo "FAIL: skills/connect-tool not in the archive" >&2; exit 1; }
[ -f "$src/SKILL.md" ] || { echo "FAIL: downloaded copy has no SKILL.md" >&2; exit 1; }

mkdir -p "$(dirname "$INSTALL_DIR")"
rm -rf "$INSTALL_DIR.new"
cp -R "$src" "$INSTALL_DIR.new"
if [ -e "$INSTALL_DIR" ] || [ -L "$INSTALL_DIR" ]; then
  rm -rf "$INSTALL_DIR.old"
  mv "$INSTALL_DIR" "$INSTALL_DIR.old"
fi
mv "$INSTALL_DIR.new" "$INSTALL_DIR"
rm -rf "$INSTALL_DIR.old"
echo "  installed the Skill to $INSTALL_DIR"

# --- 2. Dependencies ---------------------------------------------------------
missing=()
if ! command -v node >/dev/null 2>&1; then
  missing+=("Node 20+       brew install node")
else
  major=$(node --version | sed 's/^v//' | cut -d. -f1)
  [ "$major" -ge 20 ] 2>/dev/null || missing+=("Node 20+       you have v$major; brew install node")
fi
command -v uv >/dev/null 2>&1      || missing+=("uv             brew install uv")
command -v opencli >/dev/null 2>&1 || missing+=("opencli        npm install -g @jackwener/opencli")

if [ ${#missing[@]} -gt 0 ]; then
  echo
  echo "Install these, then re-run this script:"
  printf '  - %s\n' "${missing[@]}"
fi

# --- 3. The step no script can do for you ------------------------------------
cat <<EOF

NEXT, in this order:
  1. Add the OpenCLI Chrome extension (one click, opening now):
     $EXTENSION_URL
  2. Restart Claude Code so it picks up the new Skill.
  3. Ask it: run the connect-tool dependency check

EOF
open "$EXTENSION_URL" 2>/dev/null || true

[ ${#missing[@]} -eq 0 ]
