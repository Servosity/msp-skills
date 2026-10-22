#!/usr/bin/env bash
# cut_tags.sh - cut release tags one per push, each behind the release-pipeline
# probe and the CHANGELOG check, then prove each release sealed complete.
#
#   tools/maintainer/cut_tags.sh [--dry-run] [--log FILE] <SHA> <tags-file>
#   tools/maintainer/cut_tags.sh --receipts [--log FILE] <tags-file>
#   tools/maintainer/cut_tags.sh --self-test
#
# <tags-file> holds one tag per line (<slug>-v<x.y.z>); blank lines and lines
# starting with # are ignored.
#
# Push mode, per tag, in order, stopping at the first refusal:
#   1. a tag already on the remote AT THIS SHA is skipped (safe to re-run); a
#      tag on the remote at any other commit stops the run;
#   2. check_release_pipeline.py --tag T --sha S, from THIS checkout (the
#      newest probe), must endorse it: that commit's release.yml drafts, gates
#      and seals last, the version is not burned, the manifest at S says x.y.z;
#   3. changelog_section.py --check, run from a worktree AT S, must find real
#      release notes (release.yml builds the notes from that same file and
#      that same script at the tagged commit);
#   4. the tag is pushed on its own (`git push <remote> S:refs/tags/T`; never
#      --tags, and never more than one tag per push: GitHub fires no workflow
#      at all for a push of more than three tags);
#   5. it waits until a Release run for that tag at that SHA has registered,
#      and stops if none does.
# --dry-run runs every check and pushes nothing.
#
# Receipts mode: a pushed tag only STARTS a release. For each tag it waits for
# the release to be published (not a draft) and runs check_release_assets.py
# --with-mcpb on its asset list; a Release run that ended without success is
# reported at once (that is a stranded draft: re-run the workflow while the
# draft is still mutable). Then it sweeps for any draft release in the repo.
# Exit 0 only when every release sealed complete and no draft exists.
#
# Environment (for tests and unusual setups): CUT_TAGS_REPO (default: this
# checkout), CUT_TAGS_REMOTE (origin), CUT_TAGS_GH_REPO (owner/repo; default:
# read from the remote URL), CUT_TAGS_RUN_TRIES/_RUN_SLEEP (30 x 5s),
# CUT_TAGS_SPACING (15s between tags), CUT_TAGS_SEAL_TRIES/_SEAL_SLEEP (90 x 20s).
set -euo pipefail

TOOLS=$(cd "$(dirname "$0")" && pwd)
SELF="$TOOLS/$(basename "$0")"
PYTHON=${PYTHON:-python3}
LOG=""
WT_PARENT=""

say() {
  echo "$*"
  if [ -n "$LOG" ]; then echo "$(date -u +%FT%TZ) $*" >>"$LOG"; fi
}

die() {
  say "$*"
  exit 1
}

cleanup() {
  if [ -n "$WT_PARENT" ] && [ -d "$WT_PARENT/wt" ]; then
    git -C "$REPO" worktree remove --force "$WT_PARENT/wt" >/dev/null 2>&1 || true
  fi
  if [ -n "$WT_PARENT" ]; then rm -rf "$WT_PARENT"; fi
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
}

read_tags() { # prints the tags in a file, trimmed, skipping blanks and comments
  local line
  while IFS= read -r line || [ -n "$line" ]; do
    line=$(printf '%s' "$line" | tr -d '[:space:]')
    case "$line" in "" | \#*) continue ;; esac
    printf '%s\n' "$line"
  done <"$1"
}

remote_tag_commit() { # the commit a remote tag points at (peeled), or nothing
  local out
  out=$(git -C "$REPO" ls-remote "$REMOTE" "refs/tags/$1" "refs/tags/$1^{}") || return 1
  printf '%s\n' "$out" | awk -v t="refs/tags/$1" '$2 == t"^{}" {p=$1} $2 == t {d=$1} END {print (p != "" ? p : d)}'
}

cut() {
  local sha=$1 tags=$2 dry=$3 full t slug ver remote_at run i probe_out
  git -C "$REPO" fetch -q "$REMOTE" --tags
  full=$(git -C "$REPO" rev-parse --verify -q "${sha}^{commit}") || die "REFUSE $sha is not a commit in $REPO"
  WT_PARENT=$(mktemp -d)
  trap cleanup EXIT
  git -C "$REPO" worktree add -q --detach "$WT_PARENT/wt" "$full"
  probe_out="$WT_PARENT/probe.txt"
  say "cut_tags: $(read_tags "$tags" | wc -l | tr -d ' ') tag(s) at $full$([ "$dry" = 1 ] && echo ' (dry run: nothing is pushed)')"
  for t in $(read_tags "$tags"); do
    case "$t" in
      *-v[0-9]*.[0-9]*.[0-9]*) ;;
      *) die "REFUSE $t: not a <slug>-v<x.y.z> release tag" ;;
    esac
    slug=${t%-v*}
    ver=${t##*-v}
    remote_at=$(remote_tag_commit "$t") || die "REFUSE $t: cannot ask $REMOTE whether it exists"
    if [ -n "$remote_at" ]; then
      if [ "$remote_at" = "$full" ]; then
        say "SKIP $t (already cut at this SHA)"
        continue
      fi
      die "EXISTS-ELSEWHERE $t points at $remote_at, not $full (stopping; never move a release tag)"
    fi
    if ! "$PYTHON" "$TOOLS/check_release_pipeline.py" --repo "$REPO" --tag "$t" --sha "$full" >"$probe_out" 2>&1; then
      sed 's/^/    /' "$probe_out"
      die "PROBE-REFUSED $t"
    fi
    if ! (cd "$WT_PARENT/wt" && "$PYTHON" tools/maintainer/changelog_section.py --slug "$slug" --version "$ver" --check) >"$probe_out" 2>&1; then
      sed 's/^/    /' "$probe_out"
      die "NOTES-REFUSED $t (write the CHANGELOG section at that commit first)"
    fi
    if [ "$dry" = 1 ]; then
      say "WOULD-PUSH $t at $full (probe and notes OK)"
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
    sleep "${CUT_TAGS_SPACING:-15}"
  done
  say "$([ "$dry" = 1 ] && echo DRY-RUN-COMPLETE || echo ALL-PUSHED)"
}

receipts() {
  local tags=$1 fail=0 t json draft n i st drafts d stranded="" inflight=""
  for t in $(read_tags "$tags"); do
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
    n=$(printf '%s' "$json" | "$PYTHON" -c 'import json,sys; print(len(json.load(sys.stdin).get("assets") or []))')
    if printf '%s' "$json" | "$PYTHON" "$TOOLS/check_release_assets.py" --tag "$t" --with-mcpb >/dev/null 2>&1; then
      say "SEALED $t assets=$n"
    else
      say "NOT-COMPLETE $t assets=$n (published with assets missing; see check_release_assets.py)"
      fail=1
    fi
  done
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
  return "$fail"
}

# --------------------------------------------------------------------------- #
# --self-test: a throwaway origin, a fake `gh`, every path both directions
# --------------------------------------------------------------------------- #
self_test() {
  local tmp failures=0 out rc real_repo
  real_repo=$(git -C "$TOOLS" rev-parse --show-toplevel)
  tmp=$(mktemp -d)
  # shellcheck disable=SC2064  # expand now: tmp is local
  trap "rm -rf '$tmp'" EXIT
  export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$tmp/gitconfig"
  printf '[user]\n\tname = Self Test\n\temail = selftest@example.invalid\n[init]\n\tdefaultBranch = main\n' >"$GIT_CONFIG_GLOBAL"

  expect() { # <label> <condition-exit-code>
    if [ "$2" -eq 0 ]; then echo "  ok   $1"; else
      echo "  FAIL $1"
      failures=$((failures + 1))
    fi
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
  cp "$TOOLS/changelog_section.py" "$tmp/repo/tools/maintainer/"
  for s in zz-one zz-two; do
    mkdir -p "$tmp/repo/skills/$s"
    printf '{\n  "version": "0.1.0"\n}\n' >"$tmp/repo/skills/$s/manifest.json"
  done
  printf '# Changelog\n\n## [0.1.0] - 2026-10-21\n\n### Added\n- First release.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  printf '# Changelog\n\n## [0.1.0]\n\n### Changed\n- Describe the changes in this release.\n' >"$tmp/repo/skills/zz-two/CHANGELOG.md"
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "fixture" -s
  git -C "$tmp/repo" remote add origin "$tmp/origin.git"
  git -C "$tmp/repo" push -q origin main
  local sha
  sha=$(git -C "$tmp/repo" rev-parse HEAD)

  # fake gh: a Release run "registers" once its tag is on origin (unless the
  # tag is listed in no-run.txt); releases and run conclusions come from files.
  mkdir -p "$tmp/bin" "$tmp/releases"
  cat >"$tmp/bin/gh" <<FAKE
#!/usr/bin/env bash
set -u
state="$tmp"
case "\$1 \$2" in
  "run list")
    tag=""; prev=""
    for a in "\$@"; do [ "\$prev" = "--branch" ] && tag=\$a; prev=\$a; done
    if grep -qx "\$tag" "\$state/no-run.txt" 2>/dev/null; then exit 0; fi
    case "\$*" in
      *databaseId*)
        c=\$(git --git-dir "\$state/origin.git" rev-parse -q --verify "refs/tags/\$tag^{commit}" 2>/dev/null) || exit 0
        [ "\$c" = "$sha" ] && echo 4242 ;;
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

  echo "push mode:"
  printf 'zz-one-v0.1.0\n' >"$tmp/tags-ok.txt"
  out=$(bash "$SELF" --dry-run "$sha" "$tmp/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "WOULD-PUSH zz-one-v0.1.0" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.1.0 >/dev/null
  expect "a dry run checks the tag and pushes nothing" $?
  out=$(bash "$SELF" "$sha" "$tmp/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "RUN zz-one-v0.1.0 4242" \
    && [ "$(git --git-dir "$tmp/origin.git" rev-parse refs/tags/zz-one-v0.1.0)" = "$sha" ]
  expect "an endorsed tag is pushed at the SHA and its Release run is seen" $?
  out=$(bash "$SELF" "$sha" "$tmp/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "SKIP zz-one-v0.1.0"
  expect "re-running skips a tag already cut at this SHA" $?
  printf 'zz-one-v0.2.0\n' >"$tmp/tags-badver.txt"
  out=$(bash "$SELF" "$sha" "$tmp/tags-badver.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "PROBE-REFUSED zz-one-v0.2.0" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.2.0 >/dev/null
  expect "a tag the probe refuses (manifest says 0.1.0) is not pushed" $?
  printf 'zz-two-v0.1.0\n' >"$tmp/tags-notes.txt"
  out=$(bash "$SELF" "$sha" "$tmp/tags-notes.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "NOTES-REFUSED zz-two-v0.1.0"
  expect "a placeholder CHANGELOG section stops the tag before it is pushed" $?
  git -C "$tmp/repo" commit -q --allow-empty -m "later" -s
  git -C "$tmp/repo" push -q origin main
  out=$(bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "EXISTS-ELSEWHERE zz-one-v0.1.0" \
    && ! printf '%s' "$out" | grep -q "PROBE-REFUSED"
  expect "a tag already on origin at another commit stops the run before anything else" $?
  printf '# wave\n\nzz-one-v0.1.1\n' >"$tmp/tags-norun.txt"
  printf '{\n  "version": "0.1.1"\n}\n' >"$tmp/repo/skills/zz-one/manifest.json"
  printf '# Changelog\n\n## [0.1.1] - 2026-10-22\n\n### Fixed\n- A fix.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  git -C "$tmp/repo" commit -q -am "zz-one 0.1.1" -s
  git -C "$tmp/repo" push -q origin main
  echo "zz-one-v0.1.1" >"$tmp/no-run.txt"
  out=$(bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-norun.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "NO-RUN zz-one-v0.1.1"
  expect "a pushed tag whose Release run never registers stops the run" $?
  rc=1
  if [ "$(grep -vcx 1 "$tmp/origin.git/pushes.txt")" -eq 0 ] \
    && [ "$(grep -c . "$tmp/origin.git/pushes.txt")" -ge 3 ]; then rc=0; fi
  expect "every push carried exactly one ref (one tag per push)" "$rc"

  echo "receipts mode:"
  # Expected asset names come from this checkout's own registry (the first
  # releasable connector), so the proof tracks the real naming rule.
  local rs
  rs=$("$PYTHON" - "$TOOLS" "$tmp/releases" <<'PY'
import json, subprocess, sys
sys.path.insert(0, sys.argv[1])
import release_matrix
tools, out = sys.argv[1], sys.argv[2]
slug = release_matrix.skill_entries()[0]["name"]
names = subprocess.run([sys.executable, f"{tools}/check_release_assets.py", "--tag", f"{slug}-v9.9.9", "--with-mcpb",
                        "--print-expected"], capture_output=True, text=True, check=True).stdout.split()
full = [{"name": n, "size": 1024, "state": "uploaded"} for n in names]
for ver, draft, assets in (("9.9.9", False, full), ("9.9.8", False, full[1:]), ("9.9.7", True, full)):
    open(f"{out}/{slug}-v{ver}.json", "w").write(json.dumps({"isDraft": draft, "assets": assets}))
open(f"{out}/{slug}-v9.9.7.run", "w").write("completed failure\n")
open(f"{out}/{slug}-v9.9.7.status", "w").write("completed\n")
open(f"{out}/{slug}-v9.9.6.status", "w").write("in_progress\n")
print(slug)
PY
  )
  printf '%s-v9.9.9\n' "$rs" >"$tmp/r-ok.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "SEALED $rs-v9.9.9" && printf '%s' "$out" | grep -q "drafts sweep: none"
  expect "a published release with its full asset set is a receipt" $?
  printf '%s-v9.9.8\n' "$rs" >"$tmp/r-missing.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-missing.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "NOT-COMPLETE $rs-v9.9.8"
  expect "a published release missing one asset is not" $?
  printf '%s-v9.9.7\n' "$rs" >"$tmp/r-draft.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-draft.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "RUN-FAILED $rs-v9.9.7"
  expect "a draft whose Release run failed is reported as stranded" $?
  echo "$rs-v9.9.6" >"$tmp/drafts.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "IN-FLIGHT drafts (their Release run is still running): $rs-v9.9.6"
  expect "a draft whose Release run is still running is reported, not failed" $?
  printf '%s-v9.9.7\n%s-v9.9.6\n' "$rs" "$rs" >"$tmp/drafts.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "DRAFTS (releases that never finished): $rs-v9.9.7"
  expect "a draft whose Release run ended fails the sweep" $?

  if [ "$failures" -ne 0 ]; then
    echo ""
    echo "FAIL: cut_tags self-test - $failures case(s)"
    return 1
  fi
  echo ""
  echo "PASS: cut_tags - every case discriminates"
}

main() {
  local mode=push dry=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --self-test)
        self_test
        return
        ;;
      --receipts) mode=receipts ;;
      --dry-run) dry=1 ;;
      --log)
        LOG=$2
        shift
        ;;
      -h | --help)
        sed -n '2,40p' "$SELF"
        return 0
        ;;
      -*) die "unknown option $1" ;;
      *) break ;;
    esac
    shift
  done
  setup_repo
  if [ "$mode" = receipts ]; then
    [ $# -eq 1 ] || die "usage: cut_tags.sh --receipts [--log FILE] <tags-file>"
    receipts "$1"
  else
    [ $# -eq 2 ] || die "usage: cut_tags.sh [--dry-run] [--log FILE] <SHA> <tags-file>"
    cut "$1" "$2" "$dry"
  fi
}

main "$@"
