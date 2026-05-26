#!/usr/bin/env bash
# install.sh - install halopsa-cli and halopsa-mcp on macOS / Linux.
#
# Pulls prebuilt binaries from the latest GitHub Release of msp-skills.
# Both the CLI and the MCP server are installed in one shot.
#
# Env vars:
#   MSP_SKILLS_RELEASE_BASE  Override release base URL for testing.
#   DRY_RUN=1                Print the resolved URLs and exit without downloading.
#   INSTALL_DIR              Destination dir (default: ~/.local/bin).

set -euo pipefail

SKILL="halopsa"
CLI_BIN="halopsa-cli"
MCP_BIN="halopsa-mcp"

OWNER="${MSP_SKILLS_OWNER:-servosity}"
REPO="${MSP_SKILLS_REPO:-msp-skills}"
RELEASE_BASE="${MSP_SKILLS_RELEASE_BASE:-https://github.com/${OWNER}/${REPO}/releases/latest/download}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"

uname_s="$(uname -s)"
uname_m="$(uname -m)"

case "${uname_s}" in
  Darwin) os="darwin" ;;
  Linux)  os="linux" ;;
  *) echo "Unsupported OS: ${uname_s}. This installer covers macOS and Linux." >&2; exit 1 ;;
esac

case "${uname_m}" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64)  arch="amd64" ;;
  *) echo "Unsupported architecture: ${uname_m}. This installer covers arm64 and amd64." >&2; exit 1 ;;
esac

cli_url="${RELEASE_BASE}/${CLI_BIN}-${os}-${arch}"
mcp_url="${RELEASE_BASE}/${MCP_BIN}-${os}-${arch}"

echo "Skill:        ${SKILL}"
echo "Detected:     ${os}/${arch}"
echo "CLI URL:      ${cli_url}"
echo "MCP URL:      ${mcp_url}"
echo "Install dir:  ${INSTALL_DIR}"

if [ "${DRY_RUN:-0}" = "1" ]; then
  echo "DRY_RUN=1 set; not downloading."
  exit 0
fi

mkdir -p "${INSTALL_DIR}"

download() {
  local url="$1" dest="$2"
  echo "  fetching ${url}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${url}" -o "${dest}"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "${url}" -O "${dest}"
  else
    echo "Neither curl nor wget available; install one and retry." >&2
    exit 1
  fi
  chmod +x "${dest}"
}

download "${cli_url}" "${INSTALL_DIR}/${CLI_BIN}"
download "${mcp_url}" "${INSTALL_DIR}/${MCP_BIN}"

# Clear macOS Gatekeeper quarantine attribute (no-op on Linux).
if [ "${os}" = "darwin" ]; then
  xattr -d com.apple.quarantine "${INSTALL_DIR}/${CLI_BIN}" 2>/dev/null || true
  xattr -d com.apple.quarantine "${INSTALL_DIR}/${MCP_BIN}" 2>/dev/null || true
fi

case ":${PATH}:" in
  *:"${INSTALL_DIR}":*) ;;
  *)
    echo ""
    echo "NOTE: ${INSTALL_DIR} is not on your \$PATH."
    echo "  Add this line to your shell rc file (.zshrc, .bashrc, etc.):"
    echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac

echo ""
echo "Installed:"
echo "  ${INSTALL_DIR}/${CLI_BIN}"
echo "  ${INSTALL_DIR}/${MCP_BIN}"
echo ""
echo "Verify:"
echo "  ${CLI_BIN} --version"
echo ""
echo "Next:"
echo "  Read skills/halopsa/README.md for first command + auth."
echo "  For Claude Desktop or ChatGPT Desktop, read skills/halopsa/mcp-install.md."
