#!/usr/bin/env bash
# cut_tags.sh - cut release tags one per push, each behind the release-pipeline
# probe and the CHANGELOG check, then prove each release sealed complete.
#
#   tools/maintainer/cut_tags.sh [--dry-run] [--log FILE] <SHA> <tags-file>
#   tools/maintainer/cut_tags.sh --receipts [--log FILE] <tags-file>
#   tools/maintainer/cut_tags.sh --self-test
#
# <tags-file> holds one tag per line (<slug>-v<x.y.z>); blank lines and lines
# starting with # are ignored. Its path is resolved before anything else, and a
# missing, unreadable or empty list, a malformed tag or a duplicate is refused.
# A run succeeds only when the number of tags it handled equals the number
# listed (a loop that silently read nothing once printed ALL-PUSHED).
#
# Every checker runs from a fresh tree, never from the checkout this script
# happens to live in (a stale shared checkout lacks new connectors and newer
# burned versions): the pipeline probe from a worktree at origin/main (newest
# probe, newest burned_versions.json), the CHANGELOG check from a worktree at
# the SHA being tagged (what release.yml runs there), and the asset check from
# a worktree at each tag's own commit. The script also refuses to run when its
# own bytes differ from origin/main's copy (CUT_TAGS_ALLOW_STALE=1 overrides,
# loudly).
#
# Push mode, per tag, in order, stopping at the first refusal:
#   1. a tag already on the remote AT THIS SHA is skipped (safe to re-run); a
#      tag on the remote at any other commit stops the run;
#   2. check_release_pipeline.py --tag T --sha S must endorse it;
#   3. changelog_section.py --check must find real release notes at S;
#   4. the tag is pushed on its own (`git push <remote> S:refs/tags/T`; never
#      --tags, never more than one tag per push: GitHub fires no workflow at
#      all for a push of more than three tags);
#   5. it waits until a Release run for that tag at that SHA has registered.
# --dry-run runs every check and pushes nothing.
#
# Receipts mode: a pushed tag only STARTS a release. For each tag it waits for
# the release to be published and runs check_release_assets.py --with-mcpb on
# its asset list; a Release run that ended without success is reported at once
# (a stranded draft: re-run the workflow while the draft is still mutable).
# Then it sweeps the repo for drafts: one whose Release run is still going is
# reported as in flight, any other fails the run. Exit 0 only when every
# release sealed complete and no draft is stranded.
#
# Environment (for tests and unusual setups): CUT_TAGS_REPO (default: this
# checkout's clone), CUT_TAGS_REMOTE (origin), CUT_TAGS_GH_REPO (owner/repo;
# default: read from the remote URL), CUT_TAGS_RUN_TRIES/_RUN_SLEEP (30 x 5s),
# CUT_TAGS_SPACING (15s between tags), CUT_TAGS_SEAL_TRIES/_SEAL_SLEEP (90 x 20s).
set -euo pipefail

TOOLS=$(cd "$(dirname "$0")" && pwd)
SELF="$TOOLS/$(basename "$0")"
PYTHON=${PYTHON:-python3}
LOG=""
WT_PARENTS=""
WT_MAP=""
NEW_WT=""
TAG_LIST=""
TAG_COUNT=0

say() {
  echo "$*"
  if [ -n "$LOG" ]; then echo "$(date -u +%FT%TZ) $*" >>"$LOG"; fi
}

die() {
  say "$*"
  exit 1
}

abspath() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$(pwd)" "$1" ;;
  esac
}

cleanup() {
  local p
  for p in $WT_PARENTS; do
    git -C "$REPO" worktree remove --force "$p/wt" >/dev/null 2>&1 || true
    rm -rf "$p"
  done
}

worktree_at() { # <commit>: sets NEW_WT to a private worktree at it (one per commit)
  local hit parent
  hit=$(printf '%s\n' "$WT_MAP" | awk -v c="$1" '$1 == c {print $2; exit}')
  if [ -n "$hit" ]; then
    NEW_WT=$hit
    return
  fi
  parent=$(mktemp -d)
  WT_PARENTS="$WT_PARENTS $parent"
  git -C "$REPO" worktree add -q --detach "$parent/wt" "$1"
  WT_MAP="$WT_MAP
$1 $parent/wt"
  NEW_WT="$parent/wt"
}

setup_repo() {
  REPO=${CUT_TAGS_REPO:-$(git -C "$TOOLS" rev-parse --show-toplevel)}
  REMOTE=${CUT_TAGS_REMOTE:-origin}
  if [ -n "${CUT_TAGS_GH_REPO:-}" ]; then
    GH_REPO=$CUT_TAGS_GH_REPO
  else
    local url
    url=$(git -C "$REPO" remote get-url "$REMOTE")
    GH_REPO=$(printf '%s\n' "$url" | sed -nE 's#.*github\.com[:/]([^/]+)/([^/]+)$#\1/\2#p' | sed 's/\.git$//')
    [ -n "$GH_REPO" ] || die "cannot read owner/repo from $REMOTE ($url); set CUT_TAGS_GH_REPO"
  fi
  trap cleanup EXIT
}

load_tags() { # <abs path>: sets TAG_LIST and TAG_COUNT, or dies
  local f=$1 line seen=""
  [ -f "$f" ] && [ -r "$f" ] || die "REFUSE tags file is missing or unreadable: $f"
  TAG_LIST=""
  TAG_COUNT=0
  while IFS= read -r line || [ -n "$line" ]; do
    line=$(printf '%s' "$line" | tr -d '[:space:]')
    case "$line" in "" | \#*) continue ;; esac
    case "$line" in
      *-v[0-9]*.[0-9]*.[0-9]*) ;;
      *) die "REFUSE $line: not a <slug>-v<x.y.z> release tag" ;;
    esac
    case " $seen " in *" $line "*) die "REFUSE $line is listed twice" ;; esac
    seen="$seen $line"
    TAG_LIST="$TAG_LIST$line
"
    TAG_COUNT=$((TAG_COUNT + 1))
  done <"$f"
  [ "$TAG_COUNT" -gt 0 ] || die "REFUSE tags file lists no tags: $f"
}

fetch_and_check_self() { # fetch tags + main, refuse a stale copy of this script
  git -C "$REPO" fetch -q "$REMOTE" --tags "+refs/heads/main:refs/remotes/$REMOTE/main"
  MAIN_SHA=$(git -C "$REPO" rev-parse "refs/remotes/$REMOTE/main")
  local theirs
  if theirs=$(git -C "$REPO" show "$MAIN_SHA:tools/maintainer/cut_tags.sh" 2>/dev/null); then
    if [ "$theirs" != "$(cat "$SELF")" ]; then
      if [ "${CUT_TAGS_ALLOW_STALE:-}" = 1 ]; then
        say "WARN this cut_tags.sh differs from origin/main's; running anyway (CUT_TAGS_ALLOW_STALE=1)"
      else
        die "REFUSE this cut_tags.sh differs from origin/main's copy; run it from an up-to-date checkout"
      fi
    fi
  else
    say "NOTE origin/main has no tools/maintainer/cut_tags.sh to compare this copy with"
  fi
}

remote_tag_commit() { # the commit a remote tag points at (peeled), or nothing
  local out
  out=$(git -C "$REPO" ls-remote "$REMOTE" "refs/tags/$1" "refs/tags/$1^{}") || return 1
  printf '%s\n' "$out" | awk -v t="refs/tags/$1" '$2 == t"^{}" {p=$1} $2 == t {d=$1} END {print (p != "" ? p : d)}'
}

cut() {
  local sha=$1 dry=$2 full t slug ver remote_at run i probe_out shawt trustwt handled=0
  fetch_and_check_self
  full=$(git -C "$REPO" rev-parse --verify -q "${sha}^{commit}") || die "REFUSE $sha is not a commit in $REPO"
  worktree_at "$full"
  shawt=$NEW_WT
  worktree_at "$MAIN_SHA"
  trustwt=$NEW_WT
  probe_out=$(mktemp)
  say "cut_tags: $TAG_COUNT tag(s) at $full$([ "$dry" = 1 ] && echo ' (dry run: nothing is pushed)')"
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    slug=${t%-v*}
    ver=${t##*-v}
    remote_at=$(remote_tag_commit "$t") || die "REFUSE $t: cannot ask $REMOTE whether it exists"
    if [ -n "$remote_at" ]; then
      if [ "$remote_at" = "$full" ]; then
        say "SKIP $t (already cut at this SHA)"
        handled=$((handled + 1))
        continue
      fi
      die "EXISTS-ELSEWHERE $t points at $remote_at, not $full (stopping; never move a release tag)"
    fi
    if ! (cd "$trustwt" && "$PYTHON" tools/maintainer/check_release_pipeline.py --repo "$REPO" --tag "$t" --sha "$full") >"$probe_out" 2>&1; then
      sed 's/^/    /' "$probe_out"
      die "PROBE-REFUSED $t"
    fi
    if ! (cd "$shawt" && "$PYTHON" tools/maintainer/changelog_section.py --slug "$slug" --version "$ver" --check) >"$probe_out" 2>&1; then
      sed 's/^/    /' "$probe_out"
      die "NOTES-REFUSED $t (write the CHANGELOG section at that commit first)"
    fi
    if [ "$dry" = 1 ]; then
      say "WOULD-PUSH $t at $full (probe and notes OK)"
      handled=$((handled + 1))
      continue
    fi
    git -C "$REPO" push -q "$REMOTE" "${full}:refs/tags/${t}"
    say "PUSHED $t"
    run=""
    i=0
    while [ "$i" -lt "${CUT_TAGS_RUN_TRIES:-30}" ]; do
      run=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$t" --json databaseId,headSha \
        -q ".[] | select(.headSha == \"$full\") | .databaseId" 2>/dev/null | head -1 || true)
      [ -n "$run" ] && break
      i=$((i + 1))
      sleep "${CUT_TAGS_RUN_SLEEP:-5}"
    done
    [ -n "$run" ] || die "NO-RUN $t: the tag is pushed but no Release run registered (check Actions; stopping)"
    say "RUN $t $run"
    handled=$((handled + 1))
    sleep "${CUT_TAGS_SPACING:-15}"
  done <<<"$TAG_LIST"
  rm -f "$probe_out"
  [ "$handled" -eq "$TAG_COUNT" ] || die "INCOMPLETE handled $handled of $TAG_COUNT listed tags"
  say "$([ "$dry" = 1 ] && echo DRY-RUN-COMPLETE || echo ALL-PUSHED) $handled of $TAG_COUNT"
}

receipts() {
  local fail=0 checked=0 t json draft n i st drafts d stranded="" inflight="" commit wt
  fetch_and_check_self
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    checked=$((checked + 1))
    draft=missing
    json=""
    i=0
    while [ "$i" -lt "${CUT_TAGS_SEAL_TRIES:-90}" ]; do
      json=$(gh release view "$t" -R "$GH_REPO" --json isDraft,assets 2>/dev/null || true)
      if [ -n "$json" ]; then
        draft=$(printf '%s' "$json" | "$PYTHON" -c 'import json,sys; print(str(json.load(sys.stdin).get("isDraft")).lower())' 2>/dev/null || echo unreadable)
        [ "$draft" = "false" ] && break
      fi
      st=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$t" --json status,conclusion \
        -q '.[0] | "\(.status) \(.conclusion)"' 2>/dev/null || true)
      case "$st" in
        "completed success" | "" | "null null") ;;
        completed*)
          say "RUN-FAILED $t ($st): a stranded draft or no release; re-run the Release workflow while the draft is mutable"
          fail=1
          continue 2
          ;;
      esac
      i=$((i + 1))
      sleep "${CUT_TAGS_SEAL_SLEEP:-20}"
    done
    if [ "$draft" != "false" ]; then
      say "NOT-PUBLISHED $t (draft=$draft)"
      fail=1
      continue
    fi
    # The asset contract of the commit the tag names (its registry knows the
    # connector even when this checkout is older), else origin/main's.
    if commit=$(git -C "$REPO" rev-parse -q --verify "refs/tags/$t^{commit}"); then :; else
      say "NOTE $t is not a tag in this clone; checking it against origin/main's asset contract"
      commit=$MAIN_SHA
    fi
    worktree_at "$commit"
    wt=$NEW_WT
    n=$(printf '%s' "$json" | "$PYTHON" -c 'import json,sys; print(len(json.load(sys.stdin).get("assets") or []))')
    if printf '%s' "$json" | (cd "$wt" && "$PYTHON" tools/maintainer/check_release_assets.py --tag "$t" --with-mcpb) >/dev/null 2>&1; then
      say "SEALED $t assets=$n"
    else
      say "NOT-COMPLETE $t assets=$n (published with assets missing; see check_release_assets.py)"
      fail=1
    fi
  done <<<"$TAG_LIST"
  [ "$checked" -eq "$TAG_COUNT" ] || die "INCOMPLETE checked $checked of $TAG_COUNT listed tags"
  drafts=$(gh api "repos/$GH_REPO/releases" --paginate -q '.[] | select(.draft) | .tag_name' 2>/dev/null) \
    || { say "DRAFT-SWEEP-FAILED (could not list releases)"; return 1; }
  # A draft whose Release run is still going is a release in flight (another
  # wave, or this one's slowest target), not one that never finished.
  for d in $drafts; do
    st=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$d" --json status -q '.[0].status' 2>/dev/null || true)
    case "$st" in
      queued | in_progress | waiting | pending | requested) inflight="$inflight $d" ;;
      *) stranded="$stranded $d" ;;
    esac
  done
  if [ -n "$inflight" ]; then say "IN-FLIGHT drafts (their Release run is still running):$inflight"; fi
  if [ -n "$stranded" ]; then
    say "DRAFTS (releases that never finished):$stranded"
    fail=1
  elif [ -z "$inflight" ]; then
    say "drafts sweep: none"
  fi
  say "receipts: checked $checked of $TAG_COUNT"
  return "$fail"
}

# --------------------------------------------------------------------------- #
# --self-test: a throwaway origin, a fake `gh`, every path both directions
# --------------------------------------------------------------------------- #
self_test() {
  local tmp failures=0 out rc real_repo sha sha2 rs
  real_repo=$(git -C "$TOOLS" rev-parse --show-toplevel)
  tmp=$(mktemp -d)
  # shellcheck disable=SC2064  # expand now: tmp is local
  trap "rm -rf '$tmp'" EXIT
  export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$tmp/gitconfig"
  printf '[user]\n\tname = Self Test\n\temail = selftest@example.invalid\n[init]\n\tdefaultBranch = main\n' >"$GIT_CONFIG_GLOBAL"

  expect() { # <label> <exit code of the condition>
    if [ "$2" -eq 0 ]; then echo "  ok   $1"; else
      echo "  FAIL $1"
      failures=$((failures + 1))
    fi
  }
  connector() { # <slug> <version>: a releasable connector in the fixture tree
    mkdir -p "$tmp/repo/skills/$1/cli/cmd/$1-cli" "$tmp/repo/skills/$1/cli/cmd/$1-mcp"
    printf 'module example.invalid/%s\n\ngo 1.26.9\n' "$1" >"$tmp/repo/skills/$1/cli/go.mod"
    printf 'package main\n' >"$tmp/repo/skills/$1/cli/cmd/$1-cli/main.go"
    printf 'package main\n' >"$tmp/repo/skills/$1/cli/cmd/$1-mcp/main.go"
    printf '{\n  "version": "%s"\n}\n' "$2" >"$tmp/repo/skills/$1/manifest.json"
  }
  registry() { # <slug>...: the fixture's skills.json
    "$PYTHON" - "$tmp/repo/tools/maintainer/skills.json" "$@" <<'PY'
import json, sys
path, slugs = sys.argv[1], sys.argv[2:]
reg = {"owner": "servosity", "repo": "msp-skills",
       "skills": {s: {"cli_binary": f"{s}-cli", "mcp_binary": f"{s}-mcp", "version": "0.1.0"} for s in slugs}}
open(path, "w").write(json.dumps(reg, indent=2) + "\n")
PY
  }

  # origin: a bare repo whose pre-receive hook records how many refs each push carried
  git init -q --bare "$tmp/origin.git"
  cat >"$tmp/origin.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
n=0
while read -r _old _new _ref; do n=$((n + 1)); done
echo "$n" >>"$(dirname "$0")/../pushes.txt"
HOOK
  chmod +x "$tmp/origin.git/hooks/pre-receive"
  git init -q "$tmp/repo"
  mkdir -p "$tmp/repo/.github/workflows" "$tmp/repo/tools/maintainer"
  git -C "$real_repo" show HEAD:.github/workflows/release.yml >"$tmp/repo/.github/workflows/release.yml"
  git -C "$real_repo" show HEAD:.github/workflows/windows-smoke.yml >"$tmp/repo/.github/workflows/windows-smoke.yml"
  for f in changelog_section.py check_release_pipeline.py check_release_assets.py registry.py release_matrix.py; do
    cp "$TOOLS/$f" "$tmp/repo/tools/maintainer/"
  done
  # The fixture's own burned ledger retires zz-one-v0.1.9; this checkout's does not.
  printf '{\n  "burned": [\n    {"tag": "zz-one-v0.1.9", "date": "2026-10-21", "why": "fixture: retired on main only"}\n  ]\n}\n' \
    >"$tmp/repo/tools/maintainer/burned_versions.json"
  connector zz-one 0.1.0
  connector zz-two 0.1.0
  registry zz-one zz-two
  printf '# Changelog\n\n## [0.1.0] - 2026-10-21\n\n### Added\n- First release.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  printf '# Changelog\n\n## [0.1.0]\n\n### Changed\n- Describe the changes in this release.\n' >"$tmp/repo/skills/zz-two/CHANGELOG.md"
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "fixture" -s
  git -C "$tmp/repo" remote add origin "$tmp/origin.git"
  git -C "$tmp/repo" push -q origin main
  sha=$(git -C "$tmp/repo" rev-parse HEAD)

  # fake gh: a Release run "registers" once its tag is on origin at that SHA
  # (unless the tag is listed in no-run.txt); releases and runs come from files.
  mkdir -p "$tmp/bin" "$tmp/releases" "$tmp/elsewhere"
  cat >"$tmp/bin/gh" <<FAKE
#!/usr/bin/env bash
set -u
state="$tmp"
case "\$1 \$2" in
  "run list")
    tag=""; prev=""; want=""
    for a in "\$@"; do [ "\$prev" = "--branch" ] && tag=\$a; [ "\$prev" = "-q" ] && want=\$a; prev=\$a; done
    if grep -qx "\$tag" "\$state/no-run.txt" 2>/dev/null; then exit 0; fi
    case "\$*" in
      *databaseId*)
        c=\$(git --git-dir "\$state/origin.git" rev-parse -q --verify "refs/tags/\$tag^{commit}" 2>/dev/null) || exit 0
        case "\$want" in *"\$c"*) echo 4242 ;; esac ;;
      *conclusion*) cat "\$state/releases/\$tag.run" 2>/dev/null || true ;;
      *status*) cat "\$state/releases/\$tag.status" 2>/dev/null || true ;;
    esac ;;
  "release view") cat "\$state/releases/\$3.json" 2>/dev/null || exit 1 ;;
  "api repos/fake/repo/releases") cat "\$state/drafts.txt" 2>/dev/null || true ;;
  *) echo "fake gh: unhandled \$*" >&2; exit 2 ;;
esac
FAKE
  chmod +x "$tmp/bin/gh"
  export PATH="$tmp/bin:$PATH" CUT_TAGS_REPO="$tmp/repo" CUT_TAGS_GH_REPO=fake/repo
  export CUT_TAGS_RUN_TRIES=2 CUT_TAGS_RUN_SLEEP=0 CUT_TAGS_SPACING=0 CUT_TAGS_SEAL_TRIES=2 CUT_TAGS_SEAL_SLEEP=0

  echo "tag list handling (the false ALL-PUSHED):"
  printf 'zz-one-v0.1.0\n' >"$tmp/elsewhere/tags-ok.txt"
  out=$(cd "$tmp/elsewhere" && bash "$SELF" --dry-run "$sha" tags-ok.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "WOULD-PUSH zz-one-v0.1.0" \
    && printf '%s' "$out" | grep -q "DRY-RUN-COMPLETE 1 of 1" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.1.0 >/dev/null
  expect "a dry run with a RELATIVE tags path checks the tag, counts 1 of 1, and pushes nothing" $?
  out=$(cd "$tmp/elsewhere" && bash "$SELF" "$sha" missing.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && ! printf '%s' "$out" | grep -q "ALL-PUSHED"
  expect "a missing tags file fails, and never prints ALL-PUSHED" $?
  printf '# nothing here\n\n' >"$tmp/empty.txt"
  out=$(bash "$SELF" "$sha" "$tmp/empty.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "lists no tags"
  expect "a tags file with no tags fails" $?
  printf 'zz-one-v0.1.0\nzz-one-v0.1.0\n' >"$tmp/dup.txt"
  out=$(bash "$SELF" "$sha" "$tmp/dup.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "listed twice"
  expect "a tag listed twice fails before anything is pushed" $?

  echo "push mode:"
  out=$(cd "$tmp/elsewhere" && bash "$SELF" "$sha" tags-ok.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "RUN zz-one-v0.1.0 4242" \
    && printf '%s' "$out" | grep -q "ALL-PUSHED 1 of 1" \
    && [ "$(git --git-dir "$tmp/origin.git" rev-parse refs/tags/zz-one-v0.1.0)" = "$sha" ]
  expect "an endorsed tag is pushed at the SHA, its Release run is seen, 1 of 1" $?
  out=$(bash "$SELF" "$sha" "$tmp/elsewhere/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "SKIP zz-one-v0.1.0" && printf '%s' "$out" | grep -q "ALL-PUSHED 1 of 1"
  expect "re-running skips a tag already cut at this SHA and still counts it" $?
  printf 'zz-one-v0.2.0\n' >"$tmp/tags-badver.txt"
  out=$(bash "$SELF" "$sha" "$tmp/tags-badver.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "PROBE-REFUSED zz-one-v0.2.0" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.2.0 >/dev/null
  expect "a tag the probe refuses (manifest says 0.1.0) is not pushed" $?
  printf 'zz-two-v0.1.0\n' >"$tmp/tags-notes.txt"
  out=$(bash "$SELF" "$sha" "$tmp/tags-notes.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "NOTES-REFUSED zz-two-v0.1.0"
  expect "a placeholder CHANGELOG section stops the tag before it is pushed" $?

  echo "checkers run from a fresh tree, not this checkout (the wrong-tree bug):"
  connector zz-one 0.1.9
  printf '# Changelog\n\n## [0.1.9] - 2026-10-21\n\n### Fixed\n- A fix.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  git -C "$tmp/repo" commit -q -am "zz-one 0.1.9" -s
  git -C "$tmp/repo" push -q origin main
  sha2=$(git -C "$tmp/repo" rev-parse HEAD)
  rc=1
  if "$PYTHON" "$TOOLS/check_release_pipeline.py" --repo "$tmp/repo" --tag zz-one-v0.1.9 --sha "$sha2" >/dev/null 2>&1; then rc=0; fi
  expect "(this checkout's probe, whose ledger lacks the burn, WOULD endorse zz-one-v0.1.9)" "$rc"
  printf 'zz-one-v0.1.9\n' >"$tmp/tags-burned.txt"
  out=$(bash "$SELF" "$sha2" "$tmp/tags-burned.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "PROBE-REFUSED zz-one-v0.1.9" && printf '%s' "$out" | grep -q "TAG-BURNED"
  expect "cut_tags runs origin/main's probe, so the version main retired is refused" $?
  out=$(bash "$SELF" "$sha2" "$tmp/elsewhere/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "EXISTS-ELSEWHERE zz-one-v0.1.0" \
    && ! printf '%s' "$out" | grep -q "PROBE-REFUSED"
  expect "a tag already on origin at another commit stops the run before anything else" $?
  printf '# wave\n\nzz-one-v0.1.8\n' >"$tmp/tags-norun.txt"
  connector zz-one 0.1.8
  printf '# Changelog\n\n## [0.1.8] - 2026-10-22\n\n### Fixed\n- A fix.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  git -C "$tmp/repo" commit -q -am "zz-one 0.1.8" -s
  git -C "$tmp/repo" push -q origin main
  echo "zz-one-v0.1.8" >"$tmp/no-run.txt"
  out=$(bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-norun.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "NO-RUN zz-one-v0.1.8" && ! printf '%s' "$out" | grep -q "ALL-PUSHED"
  expect "a pushed tag whose Release run never registers stops the run" $?
  rc=1
  if [ "$(grep -vcx 1 "$tmp/origin.git/pushes.txt")" -eq 0 ] \
    && [ "$(grep -c . "$tmp/origin.git/pushes.txt")" -ge 3 ]; then rc=0; fi
  expect "every push carried exactly one ref (one tag per push)" "$rc"

  echo "receipts mode:"
  # A connector newer than this checkout: only the tagged tree knows its assets.
  connector zz-new 0.1.0
  registry zz-one zz-two zz-new
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "zz-new" -s
  git -C "$tmp/repo" push -q origin main
  git -C "$tmp/repo" push -q origin "HEAD:refs/tags/zz-new-v0.1.0"
  rs=$("$PYTHON" - "$tmp/repo" "$tmp/releases" <<'PY'
import json, subprocess, sys
repo, out = sys.argv[1], sys.argv[2]
names = subprocess.run([sys.executable, "tools/maintainer/check_release_assets.py", "--tag", "zz-new-v0.1.0",
                        "--with-mcpb", "--print-expected"], cwd=repo, capture_output=True, text=True,
                       check=True).stdout.split()
full = [{"name": n, "size": 1024, "state": "uploaded"} for n in names]
for ver, draft, assets in (("0.1.0", False, full), ("9.9.8", False, full[1:]), ("9.9.7", True, full)):
    open(f"{out}/zz-new-v{ver}.json", "w").write(json.dumps({"isDraft": draft, "assets": assets}))
open(f"{out}/zz-new-v9.9.7.run", "w").write("completed failure\n")
open(f"{out}/zz-new-v9.9.7.status", "w").write("completed\n")
open(f"{out}/zz-new-v9.9.6.status", "w").write("in_progress\n")
print(len(full))
PY
  )
  rc=1
  if ! gh release view zz-new-v0.1.0 | "$PYTHON" "$TOOLS/check_release_assets.py" --tag zz-new-v0.1.0 --with-mcpb >/dev/null 2>&1; then rc=0; fi
  expect "(this checkout's asset checker does NOT know zz-new, so it would report NOT-COMPLETE)" "$rc"
  printf 'zz-new-v0.1.0\n' >"$tmp/r-ok.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "SEALED zz-new-v0.1.0 assets=$rs" \
    && printf '%s' "$out" | grep -q "drafts sweep: none" && printf '%s' "$out" | grep -q "checked 1 of 1"
  expect "receipts check against the tagged tree: a complete release of a new connector is SEALED" $?
  printf 'zz-new-v9.9.8\n' >"$tmp/r-missing.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-missing.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "NOT-COMPLETE zz-new-v9.9.8"
  expect "a published release missing one asset is not" $?
  printf 'zz-new-v9.9.7\n' >"$tmp/r-draft.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-draft.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "RUN-FAILED zz-new-v9.9.7"
  expect "a draft whose Release run failed is reported as stranded" $?
  echo "zz-new-v9.9.6" >"$tmp/drafts.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "IN-FLIGHT drafts (their Release run is still running): zz-new-v9.9.6"
  expect "a draft whose Release run is still running is reported, not failed" $?
  printf 'zz-new-v9.9.7\nzz-new-v9.9.6\n' >"$tmp/drafts.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "DRAFTS (releases that never finished): zz-new-v9.9.7"
  expect "a draft whose Release run ended fails the sweep" $?
  out=$(bash "$SELF" --receipts "$tmp/nope.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && ! printf '%s' "$out" | grep -q "drafts sweep"
  expect "receipts for a missing tags file fail instead of reporting an empty success" $?

  echo "stale copies of this script:"
  mkdir -p "$tmp/repo/tools/maintainer"
  printf '#!/usr/bin/env bash\necho a newer cut_tags\n' >"$tmp/repo/tools/maintainer/cut_tags.sh"
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "main carries a newer cut_tags.sh" -s
  git -C "$tmp/repo" push -q origin main
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "differs from origin/main"
  expect "a copy that differs from origin/main's refuses to run" $?
  out=$(CUT_TAGS_ALLOW_STALE=1 bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  printf '%s' "$out" | grep -q "WARN this cut_tags.sh differs"
  expect "... unless CUT_TAGS_ALLOW_STALE=1, which says so" $?

  if [ "$failures" -ne 0 ]; then
    echo ""
    echo "FAIL: cut_tags self-test - $failures case(s)"
    return 1
  fi
  echo ""
  echo "PASS: cut_tags - every case discriminates"
}

main() {
  local mode=push dry=0 tags_file
  while [ $# -gt 0 ]; do
    case "$1" in
      --self-test)
        self_test
        return
        ;;
      --receipts) mode=receipts ;;
      --dry-run) dry=1 ;;
      --log)
        LOG=$(abspath "$2")
        shift
        ;;
      -h | --help)
        sed -n '2,50p' "$SELF"
        return 0
        ;;
      -*) die "unknown option $1" ;;
      *) break ;;
    esac
    shift
  done
  if [ "$mode" = receipts ]; then
    [ $# -eq 1 ] || die "usage: cut_tags.sh --receipts [--log FILE] <tags-file>"
    tags_file=$(abspath "$1")
  else
    [ $# -eq 2 ] || die "usage: cut_tags.sh [--dry-run] [--log FILE] <SHA> <tags-file>"
    tags_file=$(abspath "$2")
  fi
  load_tags "$tags_file"
  setup_repo
  if [ "$mode" = receipts ]; then
    receipts
  else
    cut "$1" "$dry"
  fi
}

main "$@"
