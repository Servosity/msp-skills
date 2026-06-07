#!/usr/bin/env bash
# install.sh - install sherweb-cli and sherweb-mcp on macOS / Linux.
#
# Pulls prebuilt binaries from this skill's latest GitHub Release (sherweb-v*).
# Both the CLI and the MCP server are installed in one shot.
#
# Env vars:
#   MSP_SKILLS_RELEASE_BASE  Override release base URL for testing.
#   DRY_RUN=1                Print the resolved URLs and exit without downloading.
#   INSTALL_DIR              Destination dir (default: ~/.local/bin).

set -euo pipefail

SKILL="sherweb"
CLI_BIN="sherweb-cli"
MCP_BIN="sherweb-mcp"

OWNER="${MSP_SKILLS_OWNER:-servosity}"
REPO="${MSP_SKILLS_REPO:-msp-skills}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"

fetch_stdout() {
  # GITHUB_TOKEN/GH_TOKEN (optional) authenticates GitHub API calls - lifts the
  # 60/hr unauthenticated rate limit that bites shared/corporate IPs and CI.
  _tok="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
  if command -v curl >/dev/null 2>&1; then
    if [ -n "${_tok}" ]; then
      curl -fsSL -H "Authorization: Bearer ${_tok}" "$1"
    else
      curl -fsSL "$1"
    fi
  elif command -v wget >/dev/null 2>&1; then
    if [ -n "${_tok}" ]; then
      wget -qO- --header="Authorization: Bearer ${_tok}" "$1"
    else
      wget -qO- "$1"
    fi
  else
    echo "Neither curl nor wget available; install one and retry." >&2
    exit 1
  fi
}

# Each skill is versioned and tagged independently (sherweb-vX.Y.Z), so we
# resolve THIS skill's latest release rather than the
# repo-wide /releases/latest/ (GitHub allows only one "latest" per repo). Query
# the releases API, keep tags matching this skill's prefix, take the newest (the
# API returns releases newest-first). MSP_SKILLS_RELEASE_BASE overrides this.
if [ -n "${MSP_SKILLS_RELEASE_BASE:-}" ]; then
  RELEASE_BASE="${MSP_SKILLS_RELEASE_BASE}"
else
  # Paginate: with many skills releasing independently, this skill's newest
  # tag can sit beyond the first 100 repo releases.
  tag=""
  page=1
  while [ -z "${tag}" ] && [ "${page}" -le 5 ]; do
    releases="$(fetch_stdout "https://api.github.com/repos/${OWNER}/${REPO}/releases?per_page=100&page=${page}")" || break
    # pure-shell emptiness check: grep -q on a pipe trips pipefail via SIGPIPE
    case "${releases}" in
      *'"tag_name"'*) ;;
      *) break ;;
    esac
    tag="$(printf '%s' "${releases}" \
      | grep '"tag_name"' \
      | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/' \
      | grep -m1 "^${SKILL}-v")" || true
    page=$((page + 1))
  done
  if [ -z "${tag:-}" ]; then
    echo "No ${SKILL}-v* release found in ${OWNER}/${REPO}. Has the first release been published?" >&2
    exit 1
  fi
  RELEASE_BASE="https://github.com/${OWNER}/${REPO}/releases/download/${tag}"
fi

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
echo "  First command + auth: https://github.com/${OWNER}/${REPO}/tree/main/skills/sherweb#readme"
echo "  Claude Desktop / ChatGPT wire-up: https://github.com/${OWNER}/${REPO}/blob/main/skills/sherweb/mcp-install.md"
