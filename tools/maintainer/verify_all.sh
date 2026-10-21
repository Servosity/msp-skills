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
# (verify_connector.sh: build, vet, cross-compile, go test, Windows vet and
# test compile, the Unix-only test lint, CLI claims, doctor, MCP gate, hand-fix
# ledger) run for the connectors in scope; the cheap fleet-wide checks run for
# everything. A fleet-wide check that fails ONLY for connectors outside the
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
  if [ "$keep" -eq 1 ]; then echo "logs kept in $WORK"; else rm -rf "$WORK"; fi
}
trap cleanup EXIT
mkdir -p "$WORK/logs" "$WORK/records"
REC="$WORK/records.tsv"
: > "$REC"
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
record() { printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$5" "$6" >> "$REC"; }

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
    python3 - "$log" "$blog" <<'PY'
import re, sys
norm = lambda l: " ".join(re.sub(r":\d+", ":N", l).split())
head, base = (open(p, encoding="utf-8", errors="replace").read().splitlines() for p in sys.argv[1:3])
sys.exit(0 if {norm(l) for l in head if l.strip()} <= {norm(l) for l in base} else 1)
PY
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
gate "self-test ci_guards.sh" repo "" 0 bash tools/maintainer/ci_guards.sh --self-test
matrix_sane() {
  python3 tools/maintainer/release_matrix.py \
    | python3 -c 'import json,sys; m=json.load(sys.stdin); assert m["skill"] and m["target"]'
}
gate "release matrix is valid JSON with skills + targets" repo "" 0 matrix_sane

# ------------------------------------------------- fleet-wide (cheap) ----
echo "== Fleet-wide gates (every skill) =="
gate "registry state"       fleet "" 0 python3 tools/maintainer/check_registry_state.py
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
if [ "$n_conn" -gt 0 ]; then
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
      [ -s "$r" ] && cat "$r" >> "$REC"
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
    cat "$r" >> "$REC"
  done < "$WORK/connectors.txt"
fi

# -------------------------------------------------------- security gate ----
echo "== Security gate (security-gate.yml flags) =="
if [ -s "$WORK/security.txt" ]; then
  # CI's toolchain too: setup-go installs the newest patch of the workflow's
  # go-version spec, while a local `go` follows go.mod's (older) toolchain
  # line, so a stdlib advisory fixed in the newer patch would block here and
  # pass in CI. Offline, go.mod's toolchain is used (stricter) and said so.
  gate_go="$(python3 tools/maintainer/verify_scope.py gate-go 2>/dev/null)"
  if [ -n "$gate_go" ]; then
    echo "  toolchain: $gate_go (what security-gate.yml's setup-go resolves today)"
  else
    echo "  toolchain: go.mod's (could not reach go.dev to resolve CI's; this is stricter than CI)"
  fi
  while IFS= read -r s; do
    [ -n "$s" ] || continue
    gate "security gate" scoped "$s" 0 env ${gate_go:+GOTOOLCHAIN="$gate_go"} \
      python3 tools/maintainer/check_security_gate.py \
      --slug "$s" --base "${BASE_SHA:-$base}" --policy-base "${BASE_SHA:-$base}" --require-scanners
  done < "$WORK/security.txt"
else
  echo "  (no skill path changed against $base: security-gate.yml would pass trivially)"
fi
if [ "$FULL" = true ]; then
  # Today's full audit of every skill, kept alongside the CI-parity runs.
  gate "security gate full audit (--all)" fleet "" 0 python3 tools/maintainer/check_security_gate.py --all
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
python3 tools/maintainer/verify_scope.py verdict --scope "$WORK/scope.json" --records "$REC" \
  --out "$verdict_path" --seconds "$(( $(date +%s) - T0 ))" --started "$STARTED"
rc=$?
if [ "$rc" -eq 0 ]; then
  echo "VERIFY_ALL: PASS - every blocking gate is green."
else
  echo "VERIFY_ALL: FAIL - fix the items marked FAIL above (the verdict lists them)." >&2
  rc=1
fi
exit "$rc"
