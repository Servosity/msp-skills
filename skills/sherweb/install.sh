#!/usr/bin/env bash
# install.sh - install sherweb-cli and sherweb-mcp on macOS / Linux.
#
# Pulls prebuilt binaries from this skill's latest GitHub Release (sherweb-v*).
# Both the CLI and the MCP server are installed in one shot.
#
# What this script checks before it touches anything in INSTALL_DIR:
#   1. The release it resolved is SEALED: the GitHub API reports
#      "immutable": true for the tag (a sealed release can never have an asset
#      swapped after publication). Anything else - false, missing, malformed -
#      is refused as ambiguous. MSP_SKILLS_ALLOW_MUTABLE=1 waives only this.
#      Boundary: the object is read by a small awk validator (top-level field
#      of one well-formed object from api.github.com over TLS); any document
#      it cannot validate makes the installer refuse, never accept, and an
#      escape-encoded top-level key is rejected by design.
#   2. Every binary matches its published .sha256 sidecar (one record, the
#      asset's own name, verified with shasum/sha256sum). Never waived.
#   3. No other installer is working in INSTALL_DIR: an exclusive lock
#      directory, INSTALL_DIR/.msp-install.lock, is held for the whole run. A
#      lock older than one hour is reported as stale, never stolen.
#
# Replacement is one tracked transaction: both binaries are downloaded and
# verified into a private staging directory under INSTALL_DIR first; then, for
# the CLI and then the MCP server, the old file is renamed aside to
# <dest>.prev.<txid> and the new file is renamed into place. On ANY failure
# every completed rename is undone in reverse, so both destinations end up
# exactly as they were; if an undo itself fails the backups are kept, named,
# and the exit status is non-zero. Backups are deleted only after both
# replacements succeed.
#
# Crash window: a kill signal that cannot be trapped (SIGKILL, power loss)
# between the first and the last rename can leave one binary replaced and the
# other not, plus a <dest>.prev.<txid> backup. Re-running the installer
# repairs it: it re-verifies and replaces both binaries again.
#
# Env vars:
#   MSP_SKILLS_ALLOW_MUTABLE=1  Install even when the release is not sealed
#                               (or when MSP_SKILLS_RELEASE_BASE is set).
#   MSP_SKILLS_RELEASE_BASE     Override release base URL for testing. An
#                               arbitrary base cannot be proven sealed, so it
#                               also requires MSP_SKILLS_ALLOW_MUTABLE=1.
#   MSP_SKILLS_API_BASE         Test hook: replaces BOTH the api.github.com and
#                               the github.com origins with one fixture server.
#                               GITHUB_TOKEN is never sent to an override.
#   GITHUB_TOKEN / GH_TOKEN     Optional; sent to https://api.github.com only.
#   DRY_RUN=1                   Print the resolved URLs and exit without
#                               locking, fetching or writing anything.
#   INSTALL_DIR                 Destination dir (default: ~/.local/bin).

set -euo pipefail

SKILL="sherweb"
CLI_BIN="sherweb-cli"
MCP_BIN="sherweb-mcp"

OWNER="${MSP_SKILLS_OWNER:-servosity}"
REPO="${MSP_SKILLS_REPO:-msp-skills}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"

if [ -n "${MSP_SKILLS_API_BASE:-}" ]; then
  API_BASE="${MSP_SKILLS_API_BASE%/}"
  DOWNLOAD_ORIGIN="${API_BASE}"
else
  API_BASE="https://api.github.com"
  DOWNLOAD_ORIGIN="https://github.com"
fi

fetch_stdout() {
  # GITHUB_TOKEN/GH_TOKEN (optional) authenticates GitHub API calls - lifts the
  # 60/hr unauthenticated rate limit that bites shared/corporate IPs and CI.
  # The token is attached ONLY to the real API origin, never to an override.
  _tok=""
  case "$1" in
    https://api.github.com/*) _tok="${GITHUB_TOKEN:-${GH_TOKEN:-}}" ;;
  esac
  # A request that carries the token follows NO redirects, so the header can
  # never be forwarded to another origin (older wget forwards custom headers
  # across hosts); a redirect then fails loudly as an API error instead of
  # being read as an empty listing. The API endpoints used here answer directly.
  if command -v curl >/dev/null 2>&1; then
    if [ -n "${_tok}" ]; then
      curl -fsSL --max-redirs 0 -H "Authorization: Bearer ${_tok}" "$1"
    else
      curl -fsSL "$1"
    fi
  elif command -v wget >/dev/null 2>&1; then
    if [ -n "${_tok}" ]; then
      wget -qO- --max-redirect=0 --header="Authorization: Bearer ${_tok}" "$1"
    else
      wget -qO- "$1"
    fi
  else
    echo "Neither curl nor wget available; install one and retry." >&2
    exit 1
  fi
}

# top_level_immutable: validate the JSON document on stdin (a full
# recursive-descent parse in POSIX awk: object/array/string/number/literal
# grammar, escape validation incl. 4-hex \u, no trailing data) and print the
# value of its top-level "immutable" key: true, false, string, null, number,
# object, array, none (absent), multiple (repeated), or malformed:<why> when
# the document is not one well-formed JSON object. A top-level key that uses
# any escape is refused (it cannot be decoded here, so "immutable" can
# neither match nor hide a duplicate). Nested objects and string contents can
# never match. Bytes outside JSON's alphabet (NUL and control bytes other
# than tab/LF/CR) are refused before awk reads the document, since awk could
# not report one that happens to be its record separator. Nesting deeper than
# 64 levels is refused (a release object is 3 deep) so no awk implementation
# can be driven into a recursion crash. The caller treats anything but
# true/false as ambiguous.
top_level_immutable() {
  _doc="$(mktemp)" || { echo "malformed:no-tempfile"; return; }
  cat > "${_doc}"
  if [ "$(LC_ALL=C tr -d '\011\012\015\040-\377' < "${_doc}" | wc -c | tr -d ' ')" -gt 0 ]; then
    rm -f -- "${_doc}"; echo "malformed:control-byte"; return
  fi
  awk '
  function ws() { while (P <= N && substr(S, P, 1) ~ /[ \t\r\n]/) P++ }
  function fail(w) { if (BAD == "") BAD = w }
  function pstr(  c, h) {
    P++
    while (P <= N) {
      c = substr(S, P, 1)
      if (c == "\"") { P++; return }
      if (c == "\\") {
        STRESC = 1; P++; c = substr(S, P, 1)
        if (c == "u") {
          h = substr(S, P + 1, 4)
          if (h !~ /^[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]$/) { fail("bad-escape"); return }
          P += 4
        } else if (c !~ /["\\\/bfnrt]/) { fail("bad-escape"); return }
        P++; continue
      }
      if (c < " ") { fail("control-char"); return }
      P++
    }
    fail("unterminated-string")
  }
  function pval(depth,  c, tok) {
    ws(); if (BAD != "") return ""
    if (depth > 64) { fail("too-deep"); return "" }
    c = substr(S, P, 1)
    if (c == "\"") { pstr(); return "string" }
    if (c == "{") { pobj(depth + 1); return "object" }
    if (c == "[") { parr(depth + 1); return "array" }
    tok = ""
    while (P <= N && substr(S, P, 1) ~ /[-+.A-Za-z0-9]/) { tok = tok substr(S, P, 1); P++ }
    if (tok == "true" || tok == "false" || tok == "null") return tok
    if (tok ~ /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?$/) return "number"
    fail("bad-value"); return ""
  }
  function pobj(depth,  c, key, v) {
    P++; ws()
    if (substr(S, P, 1) == "}") { P++; return }
    while (BAD == "") {
      ws()
      if (substr(S, P, 1) != "\"") { fail("expected-key"); return }
      STRESC = 0; KP = P + 1; pstr(); if (BAD != "") return
      key = substr(S, KP, P - KP - 1)
      if (depth == 1 && STRESC) { fail("escaped-key"); return }
      ws(); if (substr(S, P, 1) != ":") { fail("expected-colon"); return }
      P++
      v = pval(depth); if (BAD != "") return
      if (depth == 1 && key == "immutable") { FOUND++; VAL = v }
      ws(); c = substr(S, P, 1)
      if (c == ",") { P++; continue }
      if (c == "}") { P++; return }
      fail("expected-comma-or-brace"); return
    }
  }
  function parr(depth,  c) {
    P++; ws()
    if (substr(S, P, 1) == "]") { P++; return }
    while (BAD == "") {
      pval(depth); if (BAD != "") return
      ws(); c = substr(S, P, 1)
      if (c == ",") { P++; continue }
      if (c == "]") { P++; return }
      fail("expected-comma-or-bracket"); return
    }
  }
  BEGIN { RS = "\001" }
  {
    S = $0; N = length(S); P = 1; BAD = ""; FOUND = 0; VAL = "none"
    ws()
    if (substr(S, P, 1) != "{") fail("not-an-object"); else pobj(1)
    ws(); if (BAD == "" && P <= N) fail("trailing-data")
    if (BAD != "") print "malformed:" BAD
    else if (FOUND > 1) print "multiple"
    else if (FOUND == 0) print "none"
    else print VAL
  }' < "${_doc}"
  rm -f -- "${_doc}"
}

api_failed() {
  echo "GitHub API request failed (HTTP error, rate limit, or empty reply): $1" >&2
  echo "Set GITHUB_TOKEN to lift the unauthenticated rate limit, then retry." >&2
  exit 1
}

# Each skill is versioned and tagged independently (sherweb-vX.Y.Z), so we
# resolve THIS skill's latest release rather than the
# repo-wide /releases/latest/ (GitHub allows only one "latest" per repo). Query
# the releases API, keep tags matching this skill's prefix, take the newest (the
# API returns releases newest-first). MSP_SKILLS_RELEASE_BASE overrides this.
tag=""
if [ -n "${MSP_SKILLS_RELEASE_BASE:-}" ]; then
  RELEASE_BASE="${MSP_SKILLS_RELEASE_BASE%/}"
else
  # Paginate: with many skills releasing independently, this skill's newest
  # tag can sit beyond the first 100 repo releases. Bound: 5 pages x 100 = the
  # newest 500 releases. Every skill releases in fleet waves, so its newest tag
  # is always near the top; a skill absent from the newest 500 is reported
  # below with that bound named. An API failure is reported as such and stops
  # the install - only a successful listing with no match means "no release".
  page=1
  while [ -z "${tag}" ] && [ "${page}" -le 5 ]; do
    list_url="${API_BASE}/repos/${OWNER}/${REPO}/releases?per_page=100&page=${page}"
    releases="$(fetch_stdout "${list_url}")" || api_failed "${list_url}"
    [ -n "${releases}" ] || api_failed "${list_url}"
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
    echo "No ${SKILL}-v* release found among the newest 500 releases (5 pages x 100) of ${OWNER}/${REPO}. Has the first release been published?" >&2
    exit 1
  fi
  RELEASE_BASE="${DOWNLOAD_ORIGIN}/${OWNER}/${REPO}/releases/download/${tag}"
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
cli_asset="${cli_url##*/}"
mcp_asset="${mcp_url##*/}"

echo "Skill:        ${SKILL}"
echo "Detected:     ${os}/${arch}"
echo "CLI URL:      ${cli_url}"
echo "MCP URL:      ${mcp_url}"
echo "Install dir:  ${INSTALL_DIR}"

if [ "${DRY_RUN:-0}" = "1" ]; then
  echo "DRY_RUN=1 set; not downloading."
  exit 0
fi

# --- 1. Sealed-release check -------------------------------------------------
if [ "${MSP_SKILLS_ALLOW_MUTABLE:-0}" = "1" ]; then
  echo "WARNING: MSP_SKILLS_ALLOW_MUTABLE=1 set; skipping the sealed-release check."
elif [ -n "${MSP_SKILLS_RELEASE_BASE:-}" ]; then
  echo "MSP_SKILLS_RELEASE_BASE is set: an arbitrary release base cannot be proven to be a sealed (immutable) GitHub release." >&2
  echo "Set MSP_SKILLS_ALLOW_MUTABLE=1 to install from it anyway." >&2
  exit 1
else
  tag_url="${API_BASE}/repos/${OWNER}/${REPO}/releases/tags/${tag}"
  release_json="$(fetch_stdout "${tag_url}")" || api_failed "${tag_url}"
  [ -n "${release_json}" ] || api_failed "${tag_url}"
  # The response is ONE release object; only its TOP-LEVEL "immutable" field
  # counts. top_level_immutable (a full JSON validator in awk, stock on macOS
  # and Linux - no python3 or jq dependency) rejects any document that is not
  # one well-formed object and reads the field at depth 1 only, so neither
  # text inside a string value nor an "immutable" key inside a nested object
  # (assets, author, ...) can be mistaken for it. Anything but the bare token
  # true / false - missing, quoted, null, malformed, repeated - is ambiguous
  # and refused.
  sealed="$(printf '%s' "${release_json}" | top_level_immutable)"
  case "${sealed}" in
    true) echo "Release:      ${tag} (sealed)" ;;
    false)
      echo "Release ${tag} is NOT sealed (\"immutable\": false): its assets could be replaced after publication." >&2
      echo "Refusing to install. Set MSP_SKILLS_ALLOW_MUTABLE=1 to install anyway." >&2
      exit 1 ;;
    *)
      echo "Release ${tag}: the GitHub API did not report a usable top-level \"immutable\" field (got: ${sealed}); refusing as ambiguous." >&2
      echo "Set MSP_SKILLS_ALLOW_MUTABLE=1 to install anyway." >&2
      exit 1 ;;
  esac
fi

# --- 2. Lock, staging, download, verify --------------------------------------
mkdir -p "${INSTALL_DIR}"

LOCK_DIR="${INSTALL_DIR}/.msp-install.lock"
STAGE=""
COMMITTED=0
UNDO_FROM=()
UNDO_TO=()
HAVE_LOCK=0

cleanup() {
  rc=$?
  set +e
  if [ "${COMMITTED}" -ne 1 ] && [ "${#UNDO_TO[@]}" -gt 0 ]; then
    echo "Install failed; restoring the previous binaries..." >&2
    undo_failed=0
    i=$(( ${#UNDO_TO[@]} - 1 ))
    while [ "${i}" -ge 0 ]; do
      # An entry is journaled BEFORE its rename runs, so only reverse the ones
      # whose target actually came into existence.
      if [ -e "${UNDO_TO[$i]}" ] || [ -L "${UNDO_TO[$i]}" ]; then
        if ! mv -- "${UNDO_TO[$i]}" "${UNDO_FROM[$i]}"; then
          undo_failed=1
          echo "  could not restore ${UNDO_FROM[$i]} from ${UNDO_TO[$i]}" >&2
        fi
      fi
      i=$(( i - 1 ))
    done
    if [ "${undo_failed}" -ne 0 ]; then
      echo "Restore incomplete. Backups kept (transaction ${TXID:-?}):" >&2
      for b in "${INSTALL_DIR}/${CLI_BIN}.prev.${TXID:-?}" "${INSTALL_DIR}/${MCP_BIN}.prev.${TXID:-?}"; do
        [ -e "${b}" ] && echo "  ${b}" >&2
      done
      rc=1
    else
      echo "Previous binaries restored; nothing changed." >&2
    fi
    [ "${rc}" -ne 0 ] || rc=1
  fi
  if [ -n "${STAGE}" ]; then
    rm -rf -- "${STAGE}" 2>/dev/null || echo "NOTE: could not remove staging dir ${STAGE}; remove it by hand." >&2
  fi
  if [ "${HAVE_LOCK}" -eq 1 ]; then
    rmdir -- "${LOCK_DIR}" 2>/dev/null || true
  fi
  exit "${rc}"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

if ! mkdir -- "${LOCK_DIR}" 2>/dev/null; then
  if [ -n "$(find "${LOCK_DIR}" -maxdepth 0 -mmin +60 2>/dev/null)" ]; then
    echo "Stale lock: ${LOCK_DIR} is older than one hour. If no installer is running, remove it with: rmdir \"${LOCK_DIR}\"" >&2
  else
    echo "Another installer is running in ${INSTALL_DIR} (lock: ${LOCK_DIR}). Retry when it finishes." >&2
  fi
  exit 1
fi
HAVE_LOCK=1

# Staging lives INSIDE INSTALL_DIR so every rename below stays on one
# filesystem and is therefore atomic.
STAGE="$(mktemp -d "${INSTALL_DIR}/.msp-install.XXXXXX")"
TXID="${STAGE##*.msp-install.}"

download() {
  url="$1"; dest="$2"
  echo "  fetching ${url}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${url}" -o "${dest}" || { echo "Download failed: ${url}" >&2; exit 1; }
  elif command -v wget >/dev/null 2>&1; then
    wget -q "${url}" -O "${dest}" || { echo "Download failed: ${url}" >&2; exit 1; }
  else
    echo "Neither curl nor wget available; install one and retry." >&2
    exit 1
  fi
}

# verify_sha256 <asset-name>: the sidecar <asset-name>.sha256 must hold exactly
# one record, "<64 hex>  <asset-name>" (shasum/sha256sum format, optional "*"
# binary marker). Nothing else in the file is interpreted: the parsed hash and
# the expected name are written to a fresh one-line check file, and that is
# what shasum -a 256 -c / sha256sum -c verifies.
verify_sha256() {
  name="$1"
  sidecar="${STAGE}/${name}.sha256"
  n=0; rec=""
  while IFS= read -r line || [ -n "${line}" ]; do
    n=$(( n + 1 )); rec="${line}"
  done < "${sidecar}"
  if [ "${n}" -ne 1 ]; then
    echo "Checksum sidecar ${name}.sha256 has ${n} lines; expected exactly one." >&2
    exit 1
  fi
  hash="${rec%%[[:space:]]*}"
  rest="${rec#"${hash}"}"
  rest="${rest#"${rest%%[![:space:]]*}"}"
  rest="${rest#\*}"
  case "${hash}" in
    *[!0-9a-fA-F]*|"") echo "Checksum sidecar ${name}.sha256 is not a SHA-256 record." >&2; exit 1 ;;
  esac
  if [ "${#hash}" -ne 64 ]; then
    echo "Checksum sidecar ${name}.sha256 is not a SHA-256 record." >&2; exit 1
  fi
  if [ "${rest}" != "${name}" ]; then
    echo "Checksum sidecar ${name}.sha256 names '${rest}', not '${name}'; refusing." >&2
    exit 1
  fi
  printf '%s  %s\n' "${hash}" "${name}" > "${STAGE}/.check.${name}"
  if command -v sha256sum >/dev/null 2>&1; then
    ( cd "${STAGE}" && sha256sum -c "./.check.${name}" >/dev/null 2>&1 ) || { echo "SHA-256 MISMATCH for ${name}; refusing to install." >&2; exit 1; }
  elif command -v shasum >/dev/null 2>&1; then
    ( cd "${STAGE}" && shasum -a 256 -c "./.check.${name}" >/dev/null 2>&1 ) || { echo "SHA-256 MISMATCH for ${name}; refusing to install." >&2; exit 1; }
  else
    echo "Neither sha256sum nor shasum available; cannot verify downloads." >&2
    exit 1
  fi
  echo "  verified ${name} (sha256 ${hash})"
}

download "${cli_url}"        "${STAGE}/${cli_asset}"
download "${cli_url}.sha256" "${STAGE}/${cli_asset}.sha256"
download "${mcp_url}"        "${STAGE}/${mcp_asset}"
download "${mcp_url}.sha256" "${STAGE}/${mcp_asset}.sha256"

verify_sha256 "${cli_asset}"
verify_sha256 "${mcp_asset}"
chmod +x "${STAGE}/${cli_asset}" "${STAGE}/${mcp_asset}"

# --- 3. Tracked rename transaction -------------------------------------------
# tx_mv <from> <to>: journal the rename, THEN run it, so a signal landing
# between the two can never leave a completed rename unrecorded. <to> never
# exists beforehand (backups carry the transaction id; a destination has just
# been moved aside), so cleanup reverses exactly the entries whose <to> exists.
tx_mv() {
  UNDO_FROM[${#UNDO_FROM[@]}]="$1"
  UNDO_TO[${#UNDO_TO[@]}]="$2"
  mv -- "$1" "$2"
}

for pair in "${cli_asset}:${CLI_BIN}" "${mcp_asset}:${MCP_BIN}"; do
  asset="${pair%%:*}"
  bin="${pair#*:}"
  dest="${INSTALL_DIR}/${bin}"
  prev="${dest}.prev.${TXID}"
  if [ -e "${dest}" ] || [ -L "${dest}" ]; then
    tx_mv "${dest}" "${prev}"
  fi
  tx_mv "${STAGE}/${asset}" "${dest}"
done
COMMITTED=1

for bin in "${CLI_BIN}" "${MCP_BIN}"; do
  prev="${INSTALL_DIR}/${bin}.prev.${TXID}"
  if [ -e "${prev}" ] || [ -L "${prev}" ]; then
    rm -f -- "${prev}" || echo "NOTE: could not delete backup ${prev}; remove it by hand." >&2
  fi
done

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
