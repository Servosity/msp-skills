#!/usr/bin/env bash
# install.sh - install the canonical claude-code-statusline.
#
# Pulls statusline.py from github.com/Servosity/claude-code-statusline (MIT
# licensed; Windows-fixing PRs already merged) and writes it to ~/.claude/.
#
# Env vars:
#   STATUSLINE_RAW_URL  Override the source URL for testing.
#   DRY_RUN=1           Print the resolved URL and exit without downloading.

set -euo pipefail

DEFAULT_URL="https://raw.githubusercontent.com/Servosity/claude-code-statusline/main/statusline.py"
SRC_URL="${STATUSLINE_RAW_URL:-${DEFAULT_URL}}"
DEST_DIR="${HOME}/.claude"
DEST="${DEST_DIR}/statusline.py"

echo "Statusline source: ${SRC_URL}"
echo "Destination:       ${DEST}"

if [ "${DRY_RUN:-0}" = "1" ]; then
  echo "DRY_RUN=1 set; not downloading."
  exit 0
fi

mkdir -p "${DEST_DIR}"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${SRC_URL}" -o "${DEST}"
elif command -v wget >/dev/null 2>&1; then
  wget -q "${SRC_URL}" -O "${DEST}"
else
  echo "Neither curl nor wget available; install one and retry." >&2
  exit 1
fi

chmod +x "${DEST}"

if ! command -v python3 >/dev/null 2>&1; then
  echo ""
  echo "NOTE: python3 was not found on this machine. The statusline is Python."
  echo "      Install Python 3 (https://www.python.org/downloads/) before"
  echo "      Claude Code will be able to run it."
fi

cat <<'EOF'

Installed.

Wire it into Claude Code: open ~/.claude/settings.json and add (merge with
existing keys if the file exists):

{
  "statusLine": {
    "type": "command",
    "command": "python3 ~/.claude/statusline.py"
  }
}

Then restart Claude Code. For options and full docs, see:
  https://github.com/Servosity/claude-code-statusline
EOF
