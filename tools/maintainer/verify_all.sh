#!/usr/bin/env bash
# The check functions run indirectly (through gate/check), which ShellCheck
# cannot follow, so its never-invoked notice is off for this file.
# shellcheck disable=SC2329
# verify_all.sh - one command, one verdict: would CI pass, and is it ready for
# a pull request? Runs every gate that protects the monorepo.
#
#   bash tools/maintainer/verify_all.sh                 # the FULL fleet (default)
#   bash tools/maintainer/verify_all.sh --slug axcient  # one connector (repeatable;
#                                                       # a bare slug works too)
#   bash tools/maintainer/verify_all.sh --changed [--base origin/main]
#                                                       # what this branch changed
#
# Scope. --changed asks CI's own change-set classifier (release_matrix.py) what
# the change touches, so a change to shared machinery (a workflow, a matrix-row
# checker, registry.py) checks the WHOLE fleet exactly as CI does, and a
# derived-file-only change checks no connector. The heavy per-connector checks
# (verify_connector.sh: CI's whole Linux build row - build, vet, cross-compile,
# CLI claims, hand-fix ledger, MCP gate, doctor and go test, on linux/amd64
# with cgo on, natively or in a container, failing closed when neither exists -
# plus the Windows predictors: vet, test compile, cgo-only files, Unix-only
# test lint) run for the connectors in scope; the cheap fleet-wide checks run
# for everything. A fleet-wide check that fails ONLY for connectors outside the
# scope does not block: it is listed under out_of_scope_failures in the verdict
# and printed as OUT OF SCOPE (verify_scope.py attribute; a failure it cannot
# attribute blocks). With no arguments every failure blocks.
#
# Parity with CI. The security gate runs with security-gate.yml's exact flags
# (--slug --base --policy-base --require-scanners) for every slug CI's gate
# would scan, and the full run keeps today's --all audit as well. The hand-fix
# ledger diff, install docs, DCO, pinned-artifact and env-schema checks use the
# same flags as their CI steps. On top of CI, before the PR exists: the -pp-
# leak check and a Windows compile of every test plus a lint for Unix-only test
# assertions, the classes that only the windows CI job used to find.
#
# Output. A machine-readable verdict (checks, verdict, seconds, head and base
# SHAs, scope, out_of_scope_failures) is written to .verify/verdict.json (or
# --verdict FILE). All scratch files are private mktemp paths, so parallel
# sessions never share one. Exit code is non-zero if any blocking gate fails.
#
# Other flags: --jobs N (parallel connectors, default 3, or VERIFY_ALL_JOBS),
# --keep-logs (keep the per-check logs and print where they are).

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1

# CI's runners start from a clean Go environment, so every check here does
# too: every GO* and CGO_* variable (GOFLAGS=-tags=..., GOAMD64, GO111MODULE,
# GOROOT, a cross-compile target...), the C toolchain and the go env file go
# away, except those that only say where modules, caches and downloads live,
# which never change what is built.
for v in $(compgen -e); do
  case "$v" in
    GOPATH|GOMODCACHE|GOCACHE|GOTMPDIR|GOBIN|GOPROXY|GONOPROXY|GOPRIVATE|GONOSUMDB|GOSUMDB|GOINSECURE|GOAUTH|GOVCS|GOTELEMETRY*|GOOGLE_*) ;;
    GO*|CGO_*|CC|CXX|AR|PKG_CONFIG|GCCGO) unset "$v" ;;
  esac
done
export GOENV=off

usage() {
  cat <<'USAGE'
usage: bash tools/maintainer/verify_all.sh [--slug S]... [--changed] [--base REF]
                                           [--verdict FILE] [--jobs N] [--keep-logs] [SLUG...]
  no arguments   the full fleet (every connector, every gate)
  --slug S       these skills only (repeatable; a bare SLUG works too)
  --changed      what this branch changed against --base (default origin/main),
                 scoped by CI's own classifier (release_matrix.py)
  --verdict F    where the machine-readable verdict goes (default .verify/verdict.json)
  --jobs N       connectors checked in parallel (default 3, or VERIFY_ALL_JOBS)
  --keep-logs    keep every check's log and print the folder
USAGE
}

slugs=()
changed=0
base="origin/main"
verdict_path=".verify/verdict.json"
jobs="${VERIFY_ALL_JOBS:-3}"
keep=0
while [ $# -gt 0 ]; do
  case "$1" in
    --slug)      [ $# -ge 2 ] || { usage >&2; exit 2; }; slugs+=("$2"); shift 2 ;;
    --slug=*)    slugs+=("${1#--slug=}"); shift ;;
    --changed)   changed=1; shift ;;
    --base)      [ $# -ge 2 ] || { usage >&2; exit 2; }; base="$2"; shift 2 ;;
    --base=*)    base="${1#--base=}"; shift ;;
    --verdict)   [ $# -ge 2 ] || { usage >&2; exit 2; }; verdict_path="$2"; shift 2 ;;
    --verdict=*) verdict_path="${1#--verdict=}"; shift ;;
    --jobs)      [ $# -ge 2 ] || { usage >&2; exit 2; }; jobs="$2"; shift 2 ;;
    --jobs=*)    jobs="${1#--jobs=}"; shift ;;
    --keep-logs) keep=1; shift ;;
    -h|--help)   usage; exit 0 ;;
    -*)          echo "verify_all: unknown option $1" >&2; usage >&2; exit 2 ;;
    *)           slugs+=("$1"); shift ;;   # AGENTS.md documents `verify_all.sh <slug>`
  esac
done
case "$jobs" in ''|*[!0-9]*) echo "verify_all: --jobs needs a number" >&2; exit 2 ;; esac
[ "$jobs" -ge 1 ] || jobs=1

# Authenticate the GitHub API calls the install.sh dry-runs make below
# (unauthenticated = 60 requests an hour, which the fleet exceeds). Keep a
# caller-provided token, else fall back to the gh CLI's if available.
GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token 2>/dev/null)}"
export GITHUB_TOKEN

WORK="$(mktemp -d "${TMPDIR:-/tmp}/verify_all.XXXXXX")" || { echo "verify_all: mktemp failed" >&2; exit 2; }
cleanup() {
  [ -n "${VERIFY_LINUX_VOLUME:-}" ] && docker volume rm -f "$VERIFY_LINUX_VOLUME" >/dev/null 2>&1
  if [ "$keep" -eq 1 ]; then echo "logs kept in $WORK"; else rm -rf "$WORK"; fi
}
trap cleanup EXIT
mkdir -p "$WORK/logs" "$WORK/records"
REC="$WORK/records.tsv"
: > "$REC"
NREC=0     # check results written; the verdict fails if the record file holds any other count
REC_ERR=0  # a check result could not be written; the verdict fails
# Which Go CI installs is setup-go's reading of a version spec against go.dev's
# release index (a bare "1.26" is its newest patch). Read once per run; without
# it, a spec with no patch cannot be resolved and those checks fail closed.
if python3 tools/maintainer/verify_scope.py go-releases --out "$WORK/go-releases.json" 2>/dev/null; then
  export VERIFY_GO_RELEASES="$WORK/go-releases.json"
fi
T0=$(date +%s)
STARTED="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# ---------------------------------------------------------------- scope ----
scope_args=(--base "$base")
[ "$changed" -eq 1 ] && scope_args+=(--changed)
for s in ${slugs[@]+"${slugs[@]}"}; do scope_args+=(--slug "$s"); done
if ! python3 tools/maintainer/verify_scope.py scope "${scope_args[@]}" > "$WORK/scope.json" 2> "$WORK/scope.err"; then
  cat "$WORK/scope.err" >&2
  echo "VERIFY_ALL: FAIL - could not resolve the scope." >&2
  exit 2
fi
jget() { # jget <key>: a scope field, lists one item per line
  python3 -c 'import json,sys
v = json.load(open(sys.argv[1]))[sys.argv[2]]
if isinstance(v, dict):
    if v: print("\n".join(f"{k} {x}" for k, x in v.items()))
elif isinstance(v, list):
    if v: print("\n".join(v))  # an empty list writes NOTHING, so [ -s ] means non-empty
elif isinstance(v, bool): print(str(v).lower())
elif v is not None: print(v)' "$WORK/scope.json" "$1"
}
FULL="$(jget full_fleet)"
BASE_SHA="$(jget base_sha)"
MB="$(jget merge_base)"
jget connector_dirs > "$WORK/connectors.txt"
jget security_gate_slugs > "$WORK/security.txt"
jget touched > "$WORK/touched.txt"
jget affected > "$WORK/affected.txt"
n_conn=$(grep -c . "$WORK/connectors.txt" || true)

echo "== Scope =="
echo "  mode:       $(jget mode)$([ "$FULL" = true ] && echo ' (full fleet)')"
echo "  why:        $(jget reason)"
echo "  head:       $(jget head_sha)$([ "$(jget dirty)" = true ] && echo ' (+ uncommitted changes)')"
echo "  base:       $base = ${BASE_SHA:-unresolved}; merge base ${MB:-none}"
echo "  connectors: $n_conn$([ "$n_conn" -gt 0 ] && [ "$n_conn" -le 12 ] && printf ' (%s)' "$(cut -d' ' -f1 "$WORK/connectors.txt" | tr '\n' ' ' | sed 's/ $//')")"
note="$(jget notes)"; [ -n "$note" ] && echo "  note:       $note"
if [ "$(jget dirty)" = true ]; then
  echo "  note:       the security gate and the hand-fix diff read COMMITTED history, as CI does;"
  echo "              commit before trusting this verdict for a PR."
fi
# A base the scoped checks diff against. The merge base matches CI's
# BASE...HEAD; an unresolvable base is passed through so each check reports it.
DIFF_BASE="${MB:-$base}"

# ------------------------------------------------------------- helpers ----
in_scope() { grep -qxF "$1" "$WORK/affected.txt"; }
logfile() { printf '%s/logs/%s.log' "$WORK" "$(printf '%s__%s' "$1" "${2:-}" | tr -c 'A-Za-z0-9._-' '_')"; }
record() {
  if printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$5" "$6" >> "$REC"; then NREC=$((NREC + 1)); else REC_ERR=1; fi
}
merge_records() { # a worker's record file, appended and counted
  local n
  n=$(awk 'END {print NR}' "$1") && cat "$1" >> "$REC" && NREC=$((NREC + n)) || REC_ERR=1
}

# gate <name> <kind> <slug> <show> <command...>
#   kind: repo | scoped (blocking), fleet (attributed when the scope is not the
#   whole fleet). show=1 prints the output even on success (gates that report
#   a SKIP or a pre-existing finding while passing).
gate() {
  local name="$1" kind="$2" slug="$3" show="$4"; shift 4
  local log t0 st label
  log="$(logfile "$name" "$slug")"
  label="$name${slug:+ ($slug)}"
  t0=$(date +%s)
  if "$@" >"$log" 2>&1; then
    st=pass
    [ "$show" -eq 1 ] && sed 's/^/    /' "$log"
    printf '  PASS  %s\n' "$label"
  else
    st=fail
    if [ "$kind" = fleet ] && [ "$FULL" != true ] && fleet_out_of_scope "$log" "$slug"; then
      if baseline_confirms "$log" "$slug" "$@"; then
        st=out_of_scope
        printf '  OUT OF SCOPE  %s - fails only for: %s, exactly as on the merge base (listed in the verdict, not blocking)\n' "$label" "$OOS"
      else
        sed 's/^/    /' "$log"
        printf '    (it names only %s, but the merge base does not fail this way, so this change caused it)\n' "$OOS"
        printf '  FAIL  %s\n' "$label"
      fi
    else
      sed 's/^/    /' "$log"
      printf '  FAIL  %s\n' "$label"
    fi
  fi
  record "$name" "$kind" "$slug" "$st" "$(( $(date +%s) - t0 ))" "$log"
}

OOS=""
fleet_out_of_scope() { # <log> [slug]: true when the failure is only outside the scope
  if [ -n "${2:-}" ]; then
    OOS="$2"
    ! in_scope "$2"
    return
  fi
  local res
  res="$(python3 tools/maintainer/verify_scope.py attribute --scope "$WORK/scope.json" --log "$1" 2>/dev/null)" || return 1
  OOS="$(printf '%s' "$res" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(", ".join(d["slugs"]) if d["classification"]=="out_of_scope" else "")')"
  [ -n "$OOS" ]
}

# The merge base, checked out once (only when a failure needs it), with what
# CI's checkout calls origin/main pointing at the base, so the base runs its
# OWN scripts on its OWN tree.
BASE_TREE=""
make_base() {
  [ -n "$BASE_TREE" ] && return 0
  [ -n "$MB" ] || return 1
  local d="$WORK/base"
  git clone -q --shared --no-checkout . "$d" >/dev/null 2>&1 || return 1
  git -C "$d" checkout -q --detach "$MB" >/dev/null 2>&1 || return 1
  git -C "$d" update-ref refs/remotes/origin/main "${BASE_SHA:-$MB}" >/dev/null 2>&1 || return 1
  BASE_TREE="$d"
}

# baseline_confirms <log> <slug> <command...>: true only when the merge base
# already fails the same way. Attribution alone cannot prove a failure is
# someone else's: deleting a connector's image under docs/assets/, tightening
# vocab.json or a checker, or removing a file another skill links to all make
# OTHER connectors fail. Anything the base does not already print blocks.
baseline_confirms() {
  local log="$1" slug="$2" blog brc
  shift 2
  make_base || return 1
  blog="$log.base"
  ( cd "$BASE_TREE" && "$@" ) >"$blog" 2>&1
  brc=$?
  [ "$brc" -ne 0 ] || return 1
  if [ -n "$slug" ]; then
    # A per-skill check: every line it printed must already be printed at the base.
    python3 tools/maintainer/verify_scope.py baseline --all-lines --scope "$WORK/scope.json" \
      --head-log "$log" --base-log "$blog" >/dev/null 2>&1
    return
  fi
  python3 tools/maintainer/verify_scope.py baseline --scope "$WORK/scope.json" --head-log "$log" --base-log "$blog" >/dev/null 2>&1
}

warn_rec() { printf '  WARN  %s\n' "$1"; record "$1" repo "" warn 0 /dev/null; }

# ------------------------------------------------ maintainer self-tests ----
echo "== Maintainer gate self-tests (each proved both directions) =="
for t in registry.py check_handfixes.py mcpb_bundle.py check_release_assets.py check_release_pipeline.py \
         release_matrix.py windows_known_failures.py verify_scope.py check_no_pp_leak.py \
         check_unix_test_asserts.py; do
  gate "self-test $t" repo "" 0 python3 "tools/maintainer/$t" --self-test
done
gate "self-test check_install_docs.py" repo "" 0 python3 tools/maintainer/check_install_docs.py --selftest
gate "self-test check_command_tree.py" repo "" 0 python3 tools/maintainer/check_command_tree.py --self-test
gate "self-test ci_guards.sh" repo "" 0 bash tools/maintainer/ci_guards.sh --self-test
matrix_sane() {
  python3 tools/maintainer/release_matrix.py \
    | python3 -c 'import json,sys; m=json.load(sys.stdin); assert m["skill"] and m["target"]'
}
gate "release matrix is valid JSON with skills + targets" repo "" 0 matrix_sane

# ------------------------------------------------- fleet-wide (cheap) ----
echo "== Fleet-wide gates (every skill) =="
gate "registry state"       fleet "" 0 python3 tools/maintainer/check_registry_state.py --base "${BASE_SHA:-$base}"
gate "MCP registry schema"  fleet "" 0 python3 tools/maintainer/check_mcp_registry.py
gate "skill contract"       fleet "" 0 python3 tools/maintainer/check_skill_contract.py
gate "markdown links"       fleet "" 0 python3 tools/maintainer/check_md_links.py
gate "release contract"     fleet "" 0 python3 tools/maintainer/check_release_contract.py
gate "social assets"        fleet "" 0 python3 tools/maintainer/check_social_assets.py
gate "no TODO markers"      fleet "" 0 python3 tools/maintainer/check_no_todos.py
gate "vocabulary contract"  fleet "" 0 python3 tools/maintainer/check_vocabulary.py
gate "video assets"         fleet "" 0 python3 tools/maintainer/check_video_assets.py
gate "AEO answer-first"     fleet "" 0 python3 tools/maintainer/check_aeo.py
gate "surface coverage"     fleet "" 0 python3 tools/maintainer/check_surface_coverage.py
gate "README media block"   fleet "" 0 python3 tools/maintainer/check_media_block.py
gate "hand-fix ledgers"     fleet "" 0 python3 tools/maintainer/check_handfixes.py
gate "copyright uniform"    fleet "" 0 python3 tools/maintainer/check_copyright.py
gate "install docs"         fleet "" 0 python3 tools/maintainer/check_install_docs.py
gate "repo hygiene guards"  fleet "" 0 bash tools/maintainer/ci_guards.sh
# Not in CI: two paths that differ only by case (or Unicode normalization) are
# one file on a macOS or Windows disk, so the checks that read this checkout
# would see one of them while CI's Linux checkout holds both.
gate "paths distinct on case-insensitive disks" repo "" 0 python3 tools/maintainer/verify_scope.py path-collisions
# ci.yml guards: every connector, rules read from the base (findings carry [slug]).
gate "manifest env schema" fleet "" 1 python3 tools/maintainer/check_env_schema.py --base "${BASE_SHA:-$base}"
if [ "$FULL" = true ]; then
  # Today's full-fleet flags: --strict promotes a stale pin to a failure.
  gate "pinned release artifacts" fleet "" 1 python3 tools/maintainer/check_pinned_artifacts.py --strict
else
  # ci.yml pins job flags.
  gate "pinned release artifacts" fleet "" 1 python3 tools/maintainer/check_pinned_artifacts.py --base "${BASE_SHA:-$base}" --no-network
fi

echo "== Change-set gates (the PR diff, CI's flags) =="
jget gate_logic_changed > "$WORK/gate_logic.txt"
gate_logic_scope() { # security-gate.yml's Scope step: gate-logic edits fail by design
  if [ -s "$WORK/gate_logic.txt" ]; then
    echo "This change edits the security gate's own logic:"
    sed 's/^/  /' "$WORK/gate_logic.txt"
    echo "security-gate.yml fails such a PR before scanning anything (it cannot approve itself)."
    echo "Keep these edits in a PR of their own; it needs a security maintainer's admin merge."
    return 1
  fi
  echo "no security-gate logic file changed"
}
gate "security gate scope (gate-logic edits)" repo "" 0 gate_logic_scope
# ci.yml guards: the new-hand-fix ratchet on the PR diff.
gate "hand-fix ledger diff" repo "" 0 python3 tools/maintainer/check_handfixes.py --changed --base "$DIFF_BASE"
# ci.yml guards: DCO sign-off on the commits this branch adds.
if [ -n "$MB" ]; then
  gate "DCO sign-off" repo "" 0 env BASE="$MB" HEAD="$(jget head_sha)" bash tools/maintainer/check_dco.sh
fi
# Not in CI: the -pp- brand must not reach a user-facing surface. Every
# connector, every time (the shared catalogs name all of them, and a branch can
# touch any of them); --base makes it a ratchet, so the fleet's pre-existing
# leaks are listed, never blocking, and only a new or changed line fails.
gate "no -pp- leak" scoped "" 1 python3 tools/maintainer/check_no_pp_leak.py --all --base "$DIFF_BASE"

# ------------------------------------------------ per-connector (heavy) ----
echo "== Per-connector gates ($n_conn connector(s), $jobs at a time) =="
# CI's Linux build row (build, vet, go test on linux/amd64 with cgo on) runs in
# that context: natively on such a host, else in a linux/amd64 container,
# else every connector's Linux checks fail closed. Decided once, here.
overlay_lists() { # the working tree's changes against HEAD: $WORK/overlay.lst (NUL list) and $WORK/deleted.lst
  { git diff -z --name-only --no-renames --diff-filter=d HEAD; git ls-files -z -o --exclude-standard; } > "$WORK/overlay.lst"
  git diff -z --name-only --no-renames --diff-filter=D HEAD > "$WORK/deleted.lst"
}
linux_prepare() { # -> VERIFY_LINUX_MODE native | docker | none, VERIFY_LINUX_SRC (the snapshot)
  # The snapshot is what CI's build job checks out (actions/checkout: depth 1,
  # git metadata included) for this commit, with the working tree's staged,
  # unstaged and untracked changes and deletions laid on top, on a LINUX disk
  # (case-sensitive, Linux permissions and locking), never written back.
  local head v img="" vol
  head="$(git rev-parse HEAD)"
  overlay_lists
  # CI's runner is an unprivileged user, and some tests skip as root: a root
  # caller never runs the Linux checks natively, and containers always run
  # as an unprivileged user.
  VERIFY_LINUX_USER="$(id -u):$(id -g)"
  [ "$(id -u)" = 0 ] && VERIFY_LINUX_USER=1001:1001
  export VERIFY_LINUX_USER
  if [ "$(uname -s)" = Linux ] && [ "$(id -u)" != 0 ] && [ "$(go env GOARCH 2>/dev/null)" = amd64 ] \
     && [ "$(CGO_ENABLED=1 go env CGO_ENABLED 2>/dev/null)" = 1 ] && command -v gcc >/dev/null 2>&1; then
    VERIFY_LINUX_MODE=native
    VERIFY_LINUX_SRC="$WORK/repo"
    if ! { git init -q "$VERIFY_LINUX_SRC" \
           && git -C "$VERIFY_LINUX_SRC" fetch -q --depth 1 "file://$(pwd)" "$head" \
           && git -C "$VERIFY_LINUX_SRC" checkout -q --detach FETCH_HEAD \
           && { [ ! -s "$WORK/overlay.lst" ] || tar --null -T "$WORK/overlay.lst" -cf - | tar -xf - -C "$VERIFY_LINUX_SRC"; } \
           && ( cd "$VERIFY_LINUX_SRC" && xargs -0 rm -f < "$WORK/deleted.lst" ); }; then
      echo "  could not snapshot the repository for the Linux checks: they will fail"
      VERIFY_LINUX_SRC=""
    fi
  elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    VERIFY_LINUX_MODE=docker
    for v in $(cat "$WORK/connectors.txt" "$WORK/modules.txt" 2>/dev/null | cut -d' ' -f2 | while read -r d; do
                 python3 tools/maintainer/verify_scope.py ci-go --gomod "skills/$d/cli/go.mod" \
                   ${VERIFY_GO_RELEASES:+--releases "$VERIFY_GO_RELEASES"} | cut -f1 | sed 's/^go//'; done \
               | grep -v '^-$' | sort -u); do
      # The same tag can be cached for another platform (an arm64 pull):
      # only an amd64 image is CI's context.
      if [ "$(docker image inspect -f '{{.Architecture}}' "golang:${v}-bookworm" 2>/dev/null)" != amd64 ]; then
        docker pull -q --platform linux/amd64 "golang:${v}-bookworm" >/dev/null 2>&1 \
          || echo "  could not pull golang:${v}-bookworm for linux/amd64 (offline?): those connectors' Linux checks will fail"
      fi
      [ -z "$img" ] && img="golang:${v}-bookworm"
    done
    for vol in msp-skills-verify-gomod msp-skills-verify-gocache-amd64; do
      docker volume create "$vol" >/dev/null 2>&1
      [ -n "$img" ] && docker run --rm --platform linux/amd64 -v "$vol:/v" "$img" \
        sh -c '[ "$(stat -c %u:%g /v)" = "$1" ] || chown -R "$1" /v' sh "$VERIFY_LINUX_USER" >/dev/null 2>&1
    done
    # A private volume for this run, removed when it ends. It is filled from a
    # BARE depth-1 clone (git objects only, so this host's case-insensitive
    # disk never holds the checkout) and the overlay, streamed in without
    # macOS metadata.
    VERIFY_LINUX_VOLUME="msp-skills-verify-src-$(basename "$WORK" | tr -c 'A-Za-z0-9' '-')$$"
    VERIFY_LINUX_SRC="$VERIFY_LINUX_VOLUME"
    local tarx="" seed="$WORK/seed.git"
    tar --version 2>/dev/null | grep -q bsdtar && tarx="--no-xattrs"
    # (safe.directory: the seed belongs to this host's user, which the
    # container's user is not when the caller is root)
    in_volume() { docker run --rm -i --platform linux/amd64 --user "$VERIFY_LINUX_USER" -e HOME=/tmp \
                    -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' \
                    -v "$seed:/seed:ro" -v "$VERIFY_LINUX_VOLUME:/src" "$img" "$@"; }   # no -w: it resets /src to root
    if ! { [ -n "$img" ] \
           && git init -q --bare "$seed" \
           && git -C "$seed" fetch -q --depth 1 "file://$(pwd)" "$head" \
           && git -C "$seed" update-ref refs/heads/verify "$head" \
           && docker volume create "$VERIFY_LINUX_VOLUME" >/dev/null \
           && docker run --rm --platform linux/amd64 -v "$VERIFY_LINUX_VOLUME:/src" "$img" chown "$VERIFY_LINUX_USER" /src \
           && in_volume sh -c "git init -q /src && git -C /src fetch -q --depth 1 file:///seed refs/heads/verify \
                               && git -C /src checkout -q --detach FETCH_HEAD" \
           && { [ ! -s "$WORK/overlay.lst" ] \
                || COPYFILE_DISABLE=1 tar $tarx --null -T "$WORK/overlay.lst" -cf - | in_volume tar -xf - -C /src; } \
           && in_volume sh -c 'cd /src && xargs -0 rm -f' < "$WORK/deleted.lst"; }; then
      echo "  could not snapshot the repository into a Linux volume: the Linux checks will fail"
      VERIFY_LINUX_SRC=""
    fi
    rm -rf "$seed"
  else
    VERIFY_LINUX_MODE=none
    VERIFY_LINUX_SRC=""
  fi
  export VERIFY_LINUX_MODE VERIFY_LINUX_SRC
  echo "  linux build row: $VERIFY_LINUX_MODE$([ "$VERIFY_LINUX_MODE" = docker ] && echo ' (linux/amd64 container on a Linux volume, golang:<go.mod toolchain>, cgo on)')"
}
if [ "$n_conn" -gt 0 ]; then
  linux_prepare
  # Each worker writes only its own records and logs, so parallel runs never
  # share a file. xargs keeps the pool full; a worker always exits 0.
  xargs -P "$jobs" -n 2 bash tools/maintainer/verify_connector.sh "$WORK" < "$WORK/connectors.txt"
  xrc=$?
  if [ "$xrc" -ne 0 ]; then
    printf '  FAIL  per-connector workers: xargs exited %s (a worker was killed or could not start)\n' "$xrc"
    record "connector workers" repo "" fail 0 /dev/null
  fi
  while IFS=' ' read -r s _d; do
    [ -n "$s" ] || continue
    r="$WORK/records/$s.tsv"
    if [ ! -s "$r" ] || ! grep -q "^connector complete$(printf '\t')" "$r"; then
      # A worker that never finished: whatever it recorded is not a verdict.
      [ -s "$r" ] && merge_records "$r"
      printf '  FAIL  %s: the per-connector worker did not finish (no completion record)\n' "$s"
      record "connector harness" connector "$s" fail 0 /dev/null
      continue
    fi
    while IFS="$(printf '\t')" read -r name _kind _slug st _secs log; do
      if [ "$st" = pass ]; then
        # Known, non-blocking findings stay visible (and land in the verdict).
        grep -E '\(known Windows failure, ledgered\)' "$log" 2>/dev/null | sed 's/^/    /'
        printf '  PASS  %s (%s)\n' "$name" "$s"
      else
        sed 's/^/    /' "$log" | tail -60
        printf '  FAIL  %s (%s)\n' "$name" "$s"
      fi
    done < "$r"
    merge_records "$r"
  done < "$WORK/connectors.txt"
fi
if [ "$FULL" = true ]; then
  # Every other skills/*/cli module gets CI's Linux build and vet too (the
  # full run always built and vetted every module on disk).
  : > "$WORK/modules.txt"
  for g in skills/*/cli/go.mod; do
    d="$(basename "$(dirname "$(dirname "$g")")")"
    cut -d' ' -f2 "$WORK/connectors.txt" | grep -qxF "$d" || printf '%s %s\n' "$d" "$d" >> "$WORK/modules.txt"
  done
  if [ -s "$WORK/modules.txt" ]; then
    [ -n "${VERIFY_LINUX_MODE:-}" ] || linux_prepare
    VERIFY_MODULE_ONLY=1 xargs -P "$jobs" -n 2 bash tools/maintainer/verify_connector.sh "$WORK" < "$WORK/modules.txt"
    while IFS=' ' read -r s _d; do
      r="$WORK/records/$s.tsv"
      if [ ! -s "$r" ] || ! grep -q "^connector complete$(printf '\t')" "$r"; then
        [ -s "$r" ] && merge_records "$r"
        printf '  FAIL  %s: the module worker did not finish\n' "$s"
        record "connector harness" connector "$s" fail 0 /dev/null
        continue
      fi
      while IFS="$(printf '\t')" read -r name _kind _slug st _secs log; do
        if [ "$st" = pass ]; then printf '  PASS  %s (%s)\n' "$name" "$s"; else sed 's/^/    /' "$log" | tail -60; printf '  FAIL  %s (%s)\n' "$name" "$s"; fi
      done < "$r"
      merge_records "$r"
    done < "$WORK/modules.txt"
  fi
fi

# -------------------------------------------------------- security gate ----
echo "== Security gate (security-gate.yml flags) =="
if [ -s "$WORK/security.txt" ]; then
  # CI's toolchain too: setup-go installs the newest patch of the workflow's
  # go-version spec and the go command then selects as usual (a newer
  # `toolchain` line in a go.mod or go.work upgrades it). GOTOOLCHAIN=<that>+auto
  # is exactly that selection. Unresolvable (no release index): fail closed.
  gate_go="$(python3 tools/maintainer/verify_scope.py gate-go ${VERIFY_GO_RELEASES:+--releases "$VERIFY_GO_RELEASES"} 2>/dev/null)"
  if [ -n "$gate_go" ]; then
    echo "  toolchain: $gate_go+auto (what security-gate.yml's setup-go installs today, then Go's own selection), linux/amd64"
  else
    gate "security gate toolchain" repo "" 0 sh -c 'echo "could not read go.dev'"'"'s release index, so which Go security-gate.yml scans with is unknown"; exit 1'
  fi
  # CI's gate runs on linux/amd64 with cgo ON. The analyzers load packages
  # for the context they are given: GOOS/GOARCH always, and cgo on only where
  # this host can build cgo for linux/amd64 (a native linux/amd64 host with a
  # C compiler). Elsewhere cgo is off, and that speaks for CI's gate only when
  # turning cgo on changes nothing Go compiles (any file of any kind, in the
  # module or a dependency, either way): the context check fails closed
  # otherwise, for EVERY slug the gate scans. The probe and the scanners run on
  # the same toolchain selection as CI's gate.
  if [ "$(uname -s)" = Linux ] && [ "$(go env GOARCH 2>/dev/null)" = amd64 ] && command -v gcc >/dev/null 2>&1; then
    gate_cgo=1
  else
    gate_cgo=0
  fi
  security_context() { # <module dir> <toolchain>
    [ "$gate_cgo" = 1 ] && { echo "linux/amd64 with cgo: the gate reads what CI's gate reads"; return 0; }
    env GOOS=linux GOARCH=amd64 GOFLAGS= GOTOOLCHAIN="$2" python3 tools/maintainer/verify_scope.py cgo-context --dir "$1" \
      && return 0
    echo "CI's security gate scans with cgo on and this host's gate with cgo off, so it does not"
    echo "speak for CI here: run verify_all.sh on a linux/amd64 host with a C compiler, or rely on CI."
    return 1
  }
  while IFS= read -r s; do
    [ -n "$s" ] || continue
    [ -n "$gate_go" ] || continue
    if [ -f "skills/$s/cli/go.mod" ]; then
      gate "security gate context" scoped "$s" 0 security_context "skills/$s/cli" "$gate_go+auto"
    fi
    gate "security gate" scoped "$s" 0 env GOOS=linux GOARCH=amd64 CGO_ENABLED="$gate_cgo" GOFLAGS= \
      GOTOOLCHAIN="$gate_go+auto" python3 tools/maintainer/check_security_gate.py \
      --slug "$s" --base "${BASE_SHA:-$base}" --policy-base "${BASE_SHA:-$base}" --require-scanners
  done < "$WORK/security.txt"
else
  echo "  (no skill path changed against $base: security-gate.yml would pass trivially)"
fi
if [ "$FULL" = true ]; then
  # Today's full audit of every skill, kept alongside the CI-parity runs.
  gate "security gate full audit (--all)" fleet "" 0 python3 tools/maintainer/check_security_gate.py --all
fi

echo "== No destructive create alias (check_command_tree.py) =="
# Every on-disk module the scope covers, registered connector or not (#416's
# fleet-wide run discovers every skills/<dir>/cli/go.mod; so does this).
if [ "$FULL" = true ]; then
  gate "no destructive create alias" fleet "" 1 python3 tools/maintainer/check_command_tree.py
else
  ct_args=()
  while IFS= read -r d; do
    [ -n "$d" ] && [ -f "skills/$d/cli/go.mod" ] && ct_args+=(--slug "$d")
  done < <({ cut -d' ' -f2 "$WORK/connectors.txt"; cat "$WORK/touched.txt"; } | sort -u)
  if [ ${#ct_args[@]} -gt 0 ]; then
    gate "no destructive create alias" scoped "" 1 python3 tools/maintainer/check_command_tree.py "${ct_args[@]}"
  else
    echo "  (no Go module in scope)"
  fi
fi

# --------------------------------------------------------- plugins ----
echo "== Plugin manifest validation (claude plugin validate --strict) =="
if command -v claude >/dev/null 2>&1; then
  gate "marketplace manifest" repo "" 0 claude plugin validate . --strict
  for d in skills/*/; do
    s="$(basename "$d")"
    [ -f "$d/.claude-plugin/plugin.json" ] || continue
    gate "plugin validate" fleet "$s" 0 claude plugin validate "skills/$s" --strict
  done
else
  warn_rec "claude CLI not installed - skipped plugin validate (CI runs it)"
fi

# ------------------------------------------------ install.sh dry runs ----
echo "== Install scripts resolve cleanly (dry run) =="
install_dry_run() { # <slug>
  local slug="$1" sh="skills/$1/install.sh" pin local_tag out
  # First-release chicken-and-egg: a never-released skill has no <slug>-v* tag
  # yet, so pin the release base to the version it WILL ship as.
  pin="$(SLUG="$slug" python3 -c "
import json, os
slug = os.environ['SLUG']
reg = json.load(open('tools/maintainer/skills.json'))['skills'].get(slug, {})
if reg and not reg.get('markdown_only') and reg.get('cli_hash_at_release') in (None, ''):
    print(json.load(open(f'skills/{slug}/manifest.json'))['version'])
")"
  if [ -n "$pin" ]; then
    out="$(DRY_RUN=1 MSP_SKILLS_RELEASE_BASE="https://github.com/servosity/msp-skills/releases/download/${slug}-v${pin}" bash "$sh" 2>&1 || true)"
  else
    # Released: pin to the newest LOCAL tag (deterministic, no API calls);
    # a clone with no tags falls back to the live API path.
    local_tag="$(git tag -l "${slug}-v*" --sort=-version:refname 2>/dev/null | head -1)"
    if [ -n "$local_tag" ]; then
      out="$(DRY_RUN=1 MSP_SKILLS_RELEASE_BASE="https://github.com/servosity/msp-skills/releases/download/${local_tag}" bash "$sh" 2>&1 || true)"
    else
      out="$(DRY_RUN=1 bash "$sh" 2>&1 || true)"
    fi
  fi
  echo "$out"
  if echo "$out" | grep -q 'github.com/servosity/msp-skills' && ! echo "$out" | grep -qi 'PLACEHOLDER'; then
    return 0
  fi
  if echo "$out" | grep -qiE '403|rate limit'; then
    echo "hint: GitHub API rate limit - run 'gh auth login' or export GITHUB_TOKEN, then re-run"
  fi
  return 1
}
for sh in skills/*/install.sh; do
  s="$(basename "$(dirname "$sh")")"
  if [ "$FULL" = true ] || in_scope "$s"; then
    gate "install.sh dry run" scoped "$s" 0 install_dry_run "$s"
  fi
done

# -------------------------------------------- generated-file idempotency ----
echo "== Catalog + llms.txt idempotency =="
# build-catalog.py regenerates the machine catalog, the README marker blocks,
# the Jekyll data file and every per-skill docs page from skills.json; the gate
# proves regeneration is a no-op for every one of them. A generator failure
# fails outright (unchanged files after a crash would otherwise read as PASS).
# Drift is left regenerated in the tree, ready to commit.
catalog_idempotent() {
  local bak="$WORK/catalog.bak"
  mkdir -p "$bak"
  cp catalog.json "$bak/catalog.json"; cp README.md "$bak/README.md"
  cp docs/_data/catalog.json "$bak/docs-catalog.json"; cp -R docs/skills "$bak/docs-skills"
  python3 tools/maintainer/build-catalog.py || { echo "build-catalog.py failed during the idempotency check"; return 1; }
  if diff -q catalog.json "$bak/catalog.json" && diff -q README.md "$bak/README.md" \
     && diff -q docs/_data/catalog.json "$bak/docs-catalog.json" && diff -r docs/skills "$bak/docs-skills"; then
    return 0
  fi
  echo "catalog/README/docs drift - regenerated; re-run was not a no-op (commit the regenerated files)"
  return 1
}
llms_idempotent() {
  local bak="$WORK/llms.bak"
  mkdir -p "$bak"
  cp docs/llms.txt "$bak/llms.txt"; cp docs/llms-full.txt "$bak/llms-full.txt"
  python3 tools/maintainer/build-llms.py || { echo "build-llms.py failed during the idempotency check"; return 1; }
  if diff -q docs/llms.txt "$bak/llms.txt" && diff -q docs/llms-full.txt "$bak/llms-full.txt"; then
    return 0
  fi
  echo "llms drift - regenerated; re-run was not a no-op (run build-llms.py and commit)"
  return 1
}
gate "catalog + README + docs/_data/catalog.json + docs/skills in sync" repo "" 0 catalog_idempotent
gate "llms.txt + llms-full.txt in sync" repo "" 0 llms_idempotent

# --------------------------------------------- best effort (local tools) ----
echo "== Best-effort (warn if a tool is missing locally; CI installs them) =="
if command -v shellcheck >/dev/null 2>&1; then
  # NOT mapfile: macOS ships bash 3.2. tools/maintainer/hooks/* has no .sh
  # extension (git runs a hook by name), so it is listed as ci.yml does.
  shs=()
  while IFS= read -r f; do shs+=("$f"); done < <(git ls-files '*.sh' 'tools/maintainer/hooks/*' 2>/dev/null)
  # gcc format: one `path:line:col: level: message [SCnnnn]` line per finding,
  # which verify_scope.py can attribute (the default format interleaves code).
  if [ "${#shs[@]}" -gt 0 ]; then gate "shellcheck" fleet "" 0 shellcheck -f gcc "${shs[@]}"; fi
else
  warn_rec "shellcheck not installed - skipped (CI runs it)"
fi
if command -v gitleaks >/dev/null 2>&1; then
  gate "gitleaks" fleet "" 0 gitleaks detect --no-git --source . --config .gitleaks.toml --redact --no-banner
else
  warn_rec "gitleaks not installed - skipped (CI runs it)"
fi
if command -v actionlint >/dev/null 2>&1; then
  gate "actionlint" repo "" 0 actionlint .github/workflows/*.yml
else
  warn_rec "actionlint not installed - skipped (try: go run github.com/rhysd/actionlint/cmd/actionlint@latest)"
fi

# --------------------------------------------------------------- verdict ----
echo ""
echo "== Verdict =="
rec_flags=(--expect "$NREC")
[ "$REC_ERR" -eq 0 ] || rec_flags+=(--record-error)
python3 tools/maintainer/verify_scope.py verdict --scope "$WORK/scope.json" --records "$REC" \
  --out "$verdict_path" --seconds "$(( $(date +%s) - T0 ))" --started "$STARTED" "${rec_flags[@]}"
rc=$?
if [ "$rc" -eq 0 ]; then
  echo "VERIFY_ALL: PASS - every blocking gate is green."
else
  echo "VERIFY_ALL: FAIL - fix the items marked FAIL above (the verdict lists them)." >&2
  rc=1
fi
exit "$rc"
