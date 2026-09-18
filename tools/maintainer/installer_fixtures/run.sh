#!/usr/bin/env bash
# run.sh - behavioural fixture suite for skills/<slug>/install.sh.
#
# Runs the shipped installer (default: skills/hudu/install.sh; every other
# slug is the same script modulo slug, proven by render_fleet.py --check)
# against fake_github.py, a local stand-in for api.github.com + github.com.
# Each case asserts the exit status AND, after every rejection, that the
# pre-existing binaries in INSTALL_DIR are byte-identical and no lock,
# staging directory or backup is left behind.
#
# Rename-boundary failures are injected by putting a counting `mv` wrapper
# first on PATH (FIXTURE_FAIL_MV_AT=<n>[,<n>...] makes the n-th mv call fail),
# so the installer's transaction is exercised exactly as shipped.
#
# Usage: bash tools/maintainer/installer_fixtures/run.sh [slug]

set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "${HERE}/../../.." && pwd)"
SLUG="${1:-hudu}"
INSTALLER="${ROOT}/skills/${SLUG}/install.sh"
[ -f "${INSTALLER}" ] || { echo "no installer at ${INSTALLER}" >&2; exit 2; }

case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo "unsupported OS" >&2; exit 2 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo "unsupported arch" >&2; exit 2 ;; esac
CLI_ASSET="${SLUG}-cli-${os}-${arch}"
MCP_ASSET="${SLUG}-mcp-${os}-${arch}"
TAG="${SLUG}-v9.9.9"

WORK="$(mktemp -d)"
SERVER_PID=""
stop_server() { if [ -n "${SERVER_PID}" ]; then kill "${SERVER_PID}" 2>/dev/null || true; wait "${SERVER_PID}" 2>/dev/null || true; SERVER_PID=""; fi; }
trap 'stop_server; rm -rf "${WORK}"' EXIT

# A counting mv wrapper, first on PATH for every installer run.
SHIM_DIR="${WORK}/shim"
mkdir -p "${SHIM_DIR}"
REAL_MV="$(command -v mv)"
cat > "${SHIM_DIR}/mv" <<EOF
#!/bin/sh
c="\${FIXTURE_MV_COUNTER:-}"
if [ -n "\$c" ]; then
  n=0; [ -f "\$c" ] && n=\$(cat "\$c"); n=\$((n + 1)); echo "\$n" > "\$c"
  case ",\${FIXTURE_FAIL_MV_AT:-}," in
    *",\$n,"*) echo "mv: injected failure at call \$n" >&2; exit 1 ;;
  esac
fi
exec "${REAL_MV}" "\$@"
EOF
chmod +x "${SHIM_DIR}/mv"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# make_assets DIR: fresh "new" binaries plus correct sidecars.
make_assets() {
  mkdir -p "$1"
  printf '#!/bin/sh\necho new-cli %s\n' "${RANDOM}${RANDOM}" > "$1/${CLI_ASSET}"
  printf '#!/bin/sh\necho new-mcp %s\n' "${RANDOM}${RANDOM}" > "$1/${MCP_ASSET}"
  for a in "${CLI_ASSET}" "${MCP_ASSET}"; do
    printf '%s  %s\n' "$(sha256_of "$1/${a}")" "${a}" > "$1/${a}.sha256"
  done
}

# start_server ASSETS_DIR [fake_github.py flags...]; exports API_BASE and LOG.
start_server() {
  stop_server
  assets="$1"; shift
  PORT_FILE="${WORK}/port.$$.${RANDOM}"
  LOG="${WORK}/requests.$$.${RANDOM}.log"
  : > "${LOG}"
  python3 "${HERE}/fake_github.py" --port-file "${PORT_FILE}" --assets "${assets}" \
    --slug "${SLUG}" --tag "${TAG}" --log "${LOG}" "$@" &
  SERVER_PID=$!
  for _ in $(seq 1 100); do [ -f "${PORT_FILE}" ] && break; sleep 0.05; done
  [ -f "${PORT_FILE}" ] || { echo "fake_github.py did not start" >&2; exit 2; }
  API_BASE="http://127.0.0.1:$(cat "${PORT_FILE}")"
}

PASS=0; FAIL=0; CUR=""; CUR_OK=1
begin() { CUR="$1"; CUR_OK=1; }
check() { # check DESCRIPTION CONDITION...
  d="$1"; shift
  if "$@"; then :; else CUR_OK=0; echo "    FAIL: ${CUR}: ${d}"; fi
}
end() {
  if [ "${CUR_OK}" -eq 1 ]; then PASS=$((PASS + 1)); echo "  PASS  ${CUR}"
  else FAIL=$((FAIL + 1)); echo "  FAIL  ${CUR}"; sed 's/^/        | /' "${OUT}" | head -40; fi
}

# fresh_dir NAME: an INSTALL_DIR with pre-existing "old" binaries; snapshots kept.
fresh_dir() {
  DIR="${WORK}/cases/$1"
  mkdir -p "${DIR}"
  printf 'old-cli %s\n' "$1" > "${DIR}/${SLUG}-cli"; chmod +x "${DIR}/${SLUG}-cli"
  printf 'old-mcp %s\n' "$1" > "${DIR}/${SLUG}-mcp"; chmod +x "${DIR}/${SLUG}-mcp"
  cp "${DIR}/${SLUG}-cli" "${WORK}/cases/$1.old-cli"
  cp "${DIR}/${SLUG}-mcp" "${WORK}/cases/$1.old-mcp"
}

# run_installer [VAR=value ...]: runs with the shim on PATH; RC and OUT set.
run_installer() {
  OUT="${WORK}/out.$$.${RANDOM}"
  env -i PATH="${SHIM_DIR}:${PATH}" HOME="${HOME}" INSTALL_DIR="${DIR}" \
    FIXTURE_MV_COUNTER="${WORK}/mvcount.${RANDOM}" "$@" bash "${INSTALLER}" > "${OUT}" 2>&1
  RC=$?
}

unchanged() { # pre-existing binaries byte-identical
  cmp -s "${DIR}/${SLUG}-cli" "${WORK}/cases/${NAME}.old-cli" && cmp -s "${DIR}/${SLUG}-mcp" "${WORK}/cases/${NAME}.old-mcp"
}
installed() { # both destinations equal the served assets
  cmp -s "${DIR}/${SLUG}-cli" "${ASSETS}/${CLI_ASSET}" && cmp -s "${DIR}/${SLUG}-mcp" "${ASSETS}/${MCP_ASSET}"
}
clean() { # no lock, no staging dir, no backups
  [ ! -e "${DIR}/.msp-install.lock" ] && [ -z "$(ls -d "${DIR}"/.msp-install.* 2>/dev/null)" ] && [ -z "$(ls "${DIR}"/*.prev.* 2>/dev/null)" ]
}
out_has() { grep -q -- "$1" "${OUT}"; }
out_lacks() { ! grep -q -- "$1" "${OUT}"; }
out_has_any() { grep -qE -- "$1" "${OUT}"; }
log_has() { grep -q -- "$1" "${LOG}"; }
log_lacks() { ! grep -q -- "$1" "${LOG}"; }
rc_is() { [ "${RC}" -eq "$1" ]; }
rc_nonzero() { [ "${RC}" -ne 0 ]; }

echo "installer fixtures: ${INSTALLER} (${os}/${arch})"

# ---------------------------------------------------------------- happy path
NAME=happy; begin "happy path installs both binaries, hashes verified, backups gone"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "exit 0" rc_is 0
check "both binaries replaced" installed
check "executable" test -x "${DIR}/${SLUG}-cli"
check "sealed reported" out_has "(sealed)"
check "both verified" out_has "verified ${MCP_ASSET}"
check "clean" clean
check "tag endpoint consulted" log_has "/releases/tags/${TAG}"
check "tag found on page 2 (pagination exercised)" log_has "page=2"
end

# ---------------------------------------------------------------- sidecar cases
NAME=tampered; begin "tampered sidecar (hash altered) is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"
h="$(cut -d' ' -f1 "${ASSETS}/${MCP_ASSET}.sha256")"; case "${h}" in 0*) x=1 ;; *) x=0 ;; esac
printf '%s%s  %s\n' "${x}" "${h#?}" "${MCP_ASSET}" > "${ASSETS}/${MCP_ASSET}.sha256"
start_server "${ASSETS}"; fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "mismatch named" out_has "SHA-256 MISMATCH for ${MCP_ASSET}"
check "unchanged" unchanged; check "clean" clean
end

NAME=tampered_binary; begin "tampered binary (sidecar honest) is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; echo "evil" >> "${ASSETS}/${CLI_ASSET}"
start_server "${ASSETS}"; fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "mismatch named" out_has "SHA-256 MISMATCH for ${CLI_ASSET}"
check "unchanged" unchanged; check "clean" clean
end

NAME=missing_sidecar; begin "missing sidecar (404) is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; rm "${ASSETS}/${MCP_ASSET}.sha256"
start_server "${ASSETS}"; fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "download failure named" out_has "Download failed: .*${MCP_ASSET}.sha256"
check "unchanged" unchanged; check "clean" clean
end

NAME=wrong_name; begin "sidecar naming another asset is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"
printf '%s  %s\n' "$(sha256_of "${ASSETS}/${CLI_ASSET}")" "${MCP_ASSET}" > "${ASSETS}/${CLI_ASSET}.sha256"
start_server "${ASSETS}"; fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "name mismatch named" out_has "names '${MCP_ASSET}', not '${CLI_ASSET}'"
check "unchanged" unchanged; check "clean" clean
end

NAME=two_records; begin "sidecar with two records is refused (exactly one interpreted)"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"
printf '%s  %s\n' "$(sha256_of "${ASSETS}/${MCP_ASSET}")" "${MCP_ASSET}" >> "${ASSETS}/${CLI_ASSET}.sha256"
start_server "${ASSETS}"; fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "line count named" out_has "has 2 lines; expected exactly one"
check "unchanged" unchanged; check "clean" clean
end

NAME=not_hex; begin "sidecar whose record is not a SHA-256 is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"
printf 'deadbeef  %s\n' "${CLI_ASSET}" > "${ASSETS}/${CLI_ASSET}.sha256"
start_server "${ASSETS}"; fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "not a record named" out_has "is not a SHA-256 record"
check "unchanged" unchanged; check "clean" clean
end

# ---------------------------------------------------------------- immutability
NAME=immutable_nested; begin "top-level immutable:true with a nested immutable:false is sealed (field, not substring)"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --immutable nested
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "exit 0" rc_is 0; check "sealed reported" out_has "(sealed)"; check "installed" installed; check "clean" clean
end

for mode in missing string null nested-only broken; do
  NAME="immutable_${mode}"; begin "immutable field ${mode} is refused as ambiguous"
  ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --immutable "${mode}"
  fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
  check "non-zero" rc_nonzero; check "ambiguous named" out_has "refusing as ambiguous"
  check "unchanged" unchanged; check "clean" clean; check "nothing downloaded" log_lacks "/releases/download/"
  end
done

NAME=mutable_refused; begin "immutable:false without the hatch is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --immutable false
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "not sealed named" out_has "is NOT sealed"
check "unchanged" unchanged; check "clean" clean; check "nothing downloaded" log_lacks "/releases/download/"
end

NAME=mutable_hatch; begin "immutable:false with MSP_SKILLS_ALLOW_MUTABLE=1 installs"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --immutable false
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}" MSP_SKILLS_ALLOW_MUTABLE=1
check "exit 0" rc_is 0; check "warning printed" out_has "skipping the sealed-release check"
check "installed" installed; check "clean" clean
end

NAME=hatch_does_not_waive_hash; begin "the hatch does not waive hash verification"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; echo "evil" >> "${ASSETS}/${MCP_ASSET}"
start_server "${ASSETS}" --immutable false
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}" MSP_SKILLS_ALLOW_MUTABLE=1
check "non-zero" rc_nonzero; check "mismatch named" out_has "SHA-256 MISMATCH"
check "unchanged" unchanged; check "clean" clean
end

NAME=release_base_refused; begin "MSP_SKILLS_RELEASE_BASE override cannot claim immutability"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"; run_installer MSP_SKILLS_RELEASE_BASE="${API_BASE}/servosity/msp-skills/releases/download/${TAG}"
check "non-zero" rc_nonzero; check "override named" out_has "MSP_SKILLS_RELEASE_BASE is set"
check "unchanged" unchanged; check "clean" clean; check "no API call" log_lacks "/repos/"
end

NAME=release_base_hatch; begin "MSP_SKILLS_RELEASE_BASE override with the hatch installs (hashes still verified)"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"; run_installer MSP_SKILLS_RELEASE_BASE="${API_BASE}/servosity/msp-skills/releases/download/${TAG}" MSP_SKILLS_ALLOW_MUTABLE=1
check "exit 0" rc_is 0; check "installed" installed; check "clean" clean; check "no API call" log_lacks "/repos/"
end

# ---------------------------------------------------------------- API failures
NAME=api_500_list; begin "API 500 on the releases list is an API error, not 'no release'"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --list-status 500
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "API failure named" out_has "GitHub API request failed"
check "not misreported" out_lacks "release found"
check "unchanged" unchanged; check "clean" clean
end

NAME=api_500_tag; begin "API 500 on /releases/tags/<tag> stops the install"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --tag-status 500
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "API failure named" out_has "GitHub API request failed"
check "unchanged" unchanged; check "clean" clean; check "nothing downloaded" log_lacks "/releases/download/"
end

NAME=no_release; begin "a listing with no matching tag names the 500-release bound"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --slug "nosuch" --tag "nosuch-v1.0.0"
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "bound named" out_has "No ${SLUG}-v\* release found among the newest 500 releases"
check "unchanged" unchanged; check "clean" clean
end

NAME=api_down; begin "unreachable API is an API error"
fresh_dir "${NAME}"; stop_server; run_installer MSP_SKILLS_API_BASE="http://127.0.0.1:9"
check "non-zero" rc_nonzero; check "API failure named" out_has "GitHub API request failed"
check "unchanged" unchanged; check "clean" clean
end

# ---------------------------------------------------------------- download
NAME=truncated; begin "truncated download is refused"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}" --truncate "${CLI_ASSET}"
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero
check "refused as download or hash failure" out_has_any "Download failed|SHA-256 MISMATCH"
check "unchanged" unchanged; check "clean" clean
end

# ---------------------------------------------------------------- rename boundaries
# mv calls, in order: 1 old CLI -> prev, 2 new CLI -> dest, 3 old MCP -> prev, 4 new MCP -> dest.
for n in 1 2 3 4; do
  NAME="rename_fail_${n}"; begin "failure at rename boundary ${n} restores both destinations"
  ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
  fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}" FIXTURE_FAIL_MV_AT="${n}"
  check "non-zero" rc_nonzero; check "injected failure surfaced" out_has "injected failure at call ${n}"
  if [ "${n}" -gt 1 ]; then check "restore reported" out_has "Previous binaries restored; nothing changed"; fi
  check "unchanged" unchanged; check "clean" clean
  end
done

NAME=undo_fails; begin "failed undo keeps and names the backup, exits non-zero"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"; run_installer MSP_SKILLS_API_BASE="${API_BASE}" FIXTURE_FAIL_MV_AT="4,5"
check "non-zero" rc_nonzero; check "restore incomplete reported" out_has "Restore incomplete. Backups kept"
backup_named() { b="$(ls "${DIR}"/*.prev.* 2>/dev/null | head -1)"; [ -n "${b}" ] && grep -qF -- "${b}" "${OUT}"; }
check "backup named" backup_named
check "lock released" test ! -e "${DIR}/.msp-install.lock"
end

# ---------------------------------------------------------------- locking
NAME=lock_held; begin "a second installer is refused while the lock is held"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"; mkdir "${DIR}/.msp-install.lock"; run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "lock named" out_has "Another installer is running"
check "unchanged" unchanged; check "lock not stolen" test -d "${DIR}/.msp-install.lock"
check "nothing downloaded" log_lacks "/releases/download/"
end

NAME=lock_stale; begin "a stale lock (older than 1h) is reported, never stolen"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"; mkdir "${DIR}/.msp-install.lock"; touch -t 202001010000 "${DIR}/.msp-install.lock"
run_installer MSP_SKILLS_API_BASE="${API_BASE}"
check "non-zero" rc_nonzero; check "stale named" out_has "Stale lock"
check "unchanged" unchanged; check "lock not stolen" test -d "${DIR}/.msp-install.lock"
end

# ---------------------------------------------------------------- DRY_RUN
NAME=dry_run_api; begin "DRY_RUN=1 exits 0, prints URLs, creates nothing, consults no tag/download"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
DIR="${WORK}/cases/${NAME}/never-created"; run_installer MSP_SKILLS_API_BASE="${API_BASE}" DRY_RUN=1
check "exit 0" rc_is 0; check "CLI URL printed" out_has "CLI URL:      ${API_BASE}/servosity/msp-skills/releases/download/${TAG}/${CLI_ASSET}"
check "MCP URL printed" out_has "MCP URL:      ${API_BASE}/servosity/msp-skills/releases/download/${TAG}/${MCP_ASSET}"
check "install dir not created" test ! -e "${DIR}"
check "no tag fetch" log_lacks "/releases/tags/"; check "no download" log_lacks "/releases/download/"
end

NAME=dry_run_pinned; begin "DRY_RUN=1 with MSP_SKILLS_RELEASE_BASE makes no network call at all (verify_all.sh path)"
stop_server; DIR="${WORK}/cases/${NAME}/never-created"
run_installer MSP_SKILLS_API_BASE="http://127.0.0.1:9" MSP_SKILLS_RELEASE_BASE="https://github.com/servosity/msp-skills/releases/download/${TAG}" DRY_RUN=1
check "exit 0" rc_is 0; check "URL printed" out_has "github.com/servosity/msp-skills/releases/download/${TAG}/${CLI_ASSET}"
check "install dir not created" test ! -e "${DIR}"
end

# ---------------------------------------------------------------- token scoping
NAME=token_scoped; begin "GITHUB_TOKEN is not sent to an override API base"
ASSETS="${WORK}/assets.${NAME}"; make_assets "${ASSETS}"; start_server "${ASSETS}"
fresh_dir "${NAME}"
# Point the log at headers by using a request-capturing proxy? Simpler: run
# with a token and assert the installer still succeeds AND that curl's verbose
# trace (via a wrapper) shows no Authorization header.
CURL_SHIM="${WORK}/curlshim"; mkdir -p "${CURL_SHIM}"
cat > "${CURL_SHIM}/curl" <<EOF
#!/bin/sh
for a in "\$@"; do case "\$a" in Authorization:*) echo "\$a" >> "${WORK}/auth.seen" ;; esac; done
exec "$(command -v curl)" "\$@"
EOF
chmod +x "${CURL_SHIM}/curl"
OUT="${WORK}/out.token"; rm -f "${WORK}/auth.seen"
env -i PATH="${CURL_SHIM}:${SHIM_DIR}:${PATH}" HOME="${HOME}" INSTALL_DIR="${DIR}" GITHUB_TOKEN="ghp_test_should_not_leak" MSP_SKILLS_API_BASE="${API_BASE}" bash "${INSTALLER}" > "${OUT}" 2>&1; RC=$?
check "exit 0" rc_is 0; check "no Authorization header sent (curl argv)" test ! -e "${WORK}/auth.seen"
check "no Authorization header received (server)" log_lacks "auth=1"
end

# ---------------------------------------------------------------- sealed-field parser
# Unit probes of top_level_immutable, extracted from the shipped installer.
begin "top_level_immutable reads only a well-formed top-level boolean"
sed -n '/^top_level_immutable() {/,/^}/p' "${INSTALLER}" > "${WORK}/tli.sh"
. "${WORK}/tli.sh"
tli() { printf '%s' "$1" | top_level_immutable; }
tli_is() { [ "$(tli "$1")" = "$2" ]; }
tli_not_true() { [ "$(tli "$1")" != "true" ]; }
tli_raw_not_true() { [ "$(printf "$1" | top_level_immutable)" != "true" ]; }  # $1 is a printf FORMAT (raw bytes)
check "plain true" tli_is '{"immutable": true}' true
check "plain false" tli_is '{"immutable":false}' false
check "true with escaped opposite in a string and nested false" tli_is '{"immutable": true, "body": "see \"immutable\": false", "assets":[{"immutable":false}]}' true
check "key text as a value, nested true, then top-level true" tli_is '{"a":"immutable","b":{"immutable":true},"immutable":true}' true
check "escaped quotes inside a value" tli_is '{"a":"see \"immutable\": true here","immutable":false}' false
check "nested only" tli_not_true '{"source":{"immutable":true}}'
check "null with nested true" tli_not_true '{"immutable":null,"source":{"immutable":true}}'
check "malformed token" tli_not_true '{"immutable":trueBROKEN}'
check "quoted" tli_not_true '{"immutable":"true"}'
check "repeated key" tli_not_true '{"immutable":true,"x":1,"immutable":true}'
check "top-level array" tli_not_true '[{"immutable":true}]'
check "array with colon" tli_not_true '["immutable":true]'
check "unterminated object" tli_not_true '{"immutable":true'
check "two tokens" tli_not_true '{"immutable":true false}'
check "invalid escape elsewhere" tli_not_true '{"immutable":true,"x":"\q"}'
check "escape-encoded duplicate key" tli_not_true '{"immutable":true,"\u0069mmutable":false}'
check "escape-encoded key alone" tli_not_true '{"\u0069mmutable":true}'
check "empty value" tli_not_true '{"immutable":}'
check "trailing comma" tli_not_true '{"immutable":true,}'
check "second document" tli_not_true '{"immutable":true}{}'
check "trailing garbage" tli_not_true '{"immutable":true}garbage'
check "mismatched containers" tli_not_true '{"immutable":true,"x":[}}'
check "bad unicode escape" tli_not_true '{"immutable":true,"x":"\uZZZZ"}'
check "missing value" tli_not_true '{"immutable":true,"x":}'
check "raw control char in a string" tli_not_true "$(printf '{"immutable":true,"x":"a\tb"}')"
check "all valid escapes in values" tli_is '{"immutable":true,"x":"\" \\ \/ \b \f \n \r \t \u00e9 \uD83D\uDE00","n":[-1.5e+3,0,12],"o":{"immutable":false,"k":null}}' true
check "empty input" tli_not_true ''
check "trailing 0x01 byte" tli_raw_not_true '{"immutable":true}\001'
check "trailing NUL byte" tli_raw_not_true '{"immutable":true}\000'
check "embedded 0x01 byte" tli_raw_not_true '{"immutable":true,\001"x":1}'
check "nesting deeper than 64" tli_not_true "{\"immutable\":true,\"x\":$(printf '[%.0s' $(seq 1 70))1$(printf ']%.0s' $(seq 1 70))}"
check "nesting of 60 is fine" tli_is "{\"immutable\":true,\"x\":$(printf '[%.0s' $(seq 1 60))1$(printf ']%.0s' $(seq 1 60))}" true
end

echo
echo "installer fixtures: ${PASS} passed, ${FAIL} failed"
[ "${FAIL}" -eq 0 ]
