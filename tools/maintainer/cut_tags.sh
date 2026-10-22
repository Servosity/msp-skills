#!/usr/bin/env bash
# cut_tags.sh - cut release tags one per push, each behind the release-pipeline
# probe and the CHANGELOG check, then prove each release sealed complete.
#
#   tools/maintainer/cut_tags.sh [--dry-run] [--log FILE] <SHA> <tags-file>
#   tools/maintainer/cut_tags.sh --receipts [--log FILE] <tags-file>
#   tools/maintainer/cut_tags.sh --rerun-failed [--log FILE] <tags-file>
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
# Before every push it waits until fewer than CUT_TAGS_MAX_IN_FLIGHT (default 4)
# Release runs are in flight, repo-wide: one tag per push is not enough on its
# own (52 tags pushed one at a time still put 52 runs in flight together,
# exhausted the Actions-token API limit, and left 9 partial drafts and 9 tags
# with no release). --dry-run runs every check and pushes nothing.
#
# Recovery (--rerun-failed): for each listed tag whose latest Release run
# FAILED while its release is still a draft (or was never created), it re-runs
# the failed jobs (`gh run rerun <id> --failed`), CUT_TAGS_RERUN_BATCH (3) at a
# time under the same in-flight cap, waiting for each batch to finish, then
# prints the receipts. A re-run executes release.yml as it is at the tagged
# commit, so each candidate must pass origin/main's pipeline probe on that
# commit, and the failed run must be for that commit. A release already
# published with assets missing is immutable and is reported, not touched.
# With --dry-run it only lists what it would re-run. It never deletes a
# release and never moves or re-pushes a tag.
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
# CUT_TAGS_SPACING (15s between tags), CUT_TAGS_SEAL_TRIES/_SEAL_SLEEP (90 x 20s),
# CUT_TAGS_MAX_IN_FLIGHT (4), CUT_TAGS_FLIGHT_TRIES/_FLIGHT_SLEEP (120 x 30s),
# CUT_TAGS_RERUN_BATCH (3).
set -euo pipefail

TOOLS=$(cd "$(dirname "$0")" && pwd)
SELF="$TOOLS/$(basename "$0")"
PYTHON=${PYTHON:-python3}
LOG=""
WT_MAP=""
WT_COUNT=0
ROOT_TMP=""
NEW_TMP=""
SELFTEST_TMP=""
NL='
'
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

tmp_root() { # sets ROOT_TMP: this run's ONE private directory (made once)
  # Everything temporary lives under it and cleanup removes exactly it, so no
  # parsing of a path list can ever reach another directory. A TMPDIR holding
  # a newline is refused before anything is created.
  [ -z "$ROOT_TMP" ] || return 0
  case "${TMPDIR:-/tmp}" in *"$NL"*) die "REFUSE TMPDIR contains a newline; set it to a plain directory" ;; esac
  ROOT_TMP=$(mktemp -d "${TMPDIR:-/tmp}/cut_tags.XXXXXX") || die "REFUSE cannot create a temporary directory"
  [ -n "$ROOT_TMP" ] && [ -d "$ROOT_TMP" ] || die "REFUSE mktemp returned no directory"
}

new_tempdir() { # sets NEW_TMP to a fresh directory under ROOT_TMP
  tmp_root
  NEW_TMP=$(mktemp -d "$ROOT_TMP/d.XXXXXX") || die "REFUSE cannot create a temporary directory"
  [ -n "$NEW_TMP" ] && [ -d "$NEW_TMP" ] || die "REFUSE mktemp returned no directory"
}

new_tempfile() { # sets NEW_TMP to a fresh file under ROOT_TMP
  tmp_root
  NEW_TMP=$(mktemp "$ROOT_TMP/f.XXXXXX") || die "REFUSE cannot create a temporary file"
  [ -n "$NEW_TMP" ] && [ -f "$NEW_TMP" ] || die "REFUSE mktemp returned no file"
}

cleanup() {
  local n=0
  tag_unlock
  while [ "$n" -lt "$WT_COUNT" ]; do
    n=$((n + 1))
    git -C "$REPO" worktree remove --force "$ROOT_TMP/wt$n" >/dev/null 2>&1 || true
  done
  if [ -n "$ROOT_TMP" ]; then rm -rf "$ROOT_TMP"; fi
}

worktree_at() { # <commit>: sets NEW_WT to a private worktree at it (one per commit)
  local line
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    if [ "${line%% *}" = "$1" ]; then # "<40-hex commit> <worktree number>"
      NEW_WT="$ROOT_TMP/wt${line#* }"
      return
    fi
  done <<<"$WT_MAP"
  tmp_root
  WT_COUNT=$((WT_COUNT + 1))
  git -C "$REPO" worktree add -q --detach "$ROOT_TMP/wt$WT_COUNT" "$1"
  WT_MAP="$WT_MAP$1 $WT_COUNT
"
  NEW_WT="$ROOT_TMP/wt$WT_COUNT"
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

fetch_retry() { # <fetch args...>: a fetch, retried (another cutter may hold the ref locks)
  local i=0
  while [ "$i" -lt 5 ]; do
    git -C "$REPO" fetch -q "$@" </dev/null && return 0
    i=$((i + 1))
    sleep 1
  done
  return 1
}

# --------------------------------------------------------------------------- #
# The release-tag lock: one cutter (or re-runner) at a time between "is there
# capacity?" and "its Release run is registered", so two runs can never both
# see room under the cap and both push. A flock(2) on a file in the clone's
# shared git dir (the same primitive and place as repo_lock.py), owned by a
# small python HOLDER process: its descriptor is close-on-exec and it starts
# nothing, so no git hook or other child of this script can inherit the lock
# and keep it alive. It releases when told to, and on its own the moment this
# script's pid is gone (even after SIGKILL). It covers every worktree of one
# clone; separate clones do not share it.
# --------------------------------------------------------------------------- #
LOCK_PATH=""
LOCK_HOLDER=""

tag_lock() {
  local common
  if [ -z "$LOCK_PATH" ]; then
    common=$(git -C "$REPO" rev-parse --git-common-dir) || die "REFUSE cannot find the clone's git dir"
    case "$common" in /*) ;; *) common="$REPO/$common" ;; esac
    mkdir -p "$common/msp-skills-locks" || die "REFUSE cannot create the lock directory"
    LOCK_PATH="$common/msp-skills-locks/release-tags.lock"
  fi
  tmp_root
  rm -f "$ROOT_TMP/lock.ready" "$ROOT_TMP/lock.release"
  "$PYTHON" - "$LOCK_PATH" "$$" "$ROOT_TMP/lock.ready" "$ROOT_TMP/lock.release" "${CUT_TAGS_LOCK_WAIT:-7200}" <<'PY' &
import fcntl, json, os, socket, sys, time
path, parent, ready, release, wait = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4], float(sys.argv[5])


def parent_alive() -> bool:
    try:
        os.kill(parent, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_CLOEXEC, 0o644)
deadline = time.time() + wait
while True:
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        break
    except OSError:
        if time.time() > deadline or not parent_alive():
            sys.exit(1)
        time.sleep(0.2)
os.ftruncate(fd, 0)
os.write(fd, (json.dumps({"pid": parent, "host": socket.gethostname(), "tool": "cut_tags"}) + "\n").encode())
open(ready, "w").close()
while not os.path.exists(release) and parent_alive():
    time.sleep(0.1)
os.ftruncate(fd, 0)
fcntl.flock(fd, fcntl.LOCK_UN)
PY
  LOCK_HOLDER=$!
  while [ ! -f "$ROOT_TMP/lock.ready" ]; do
    if ! kill -0 "$LOCK_HOLDER" 2>/dev/null; then
      LOCK_HOLDER=""
      die "LOCK-TIMEOUT another cut_tags run held the release-tag lock too long; stopping"
    fi
    sleep 0.1
  done
}

tag_unlock() {
  [ -n "$LOCK_HOLDER" ] || return 0
  : >"$ROOT_TMP/lock.release"
  wait "$LOCK_HOLDER" 2>/dev/null || true
  LOCK_HOLDER=""
}

fetch_and_check_self() { # fetch tags + main, refuse a stale copy of this script
  fetch_retry "$REMOTE" --tags "+refs/heads/main:refs/remotes/$REMOTE/main" || die "REFUSE cannot fetch $REMOTE"
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

in_flight() { # Release runs not yet completed, repo-wide (every wave counts)
  # GitHub's own total_count per active status: a listing of the newest N runs
  # would miss a long-running release older than N completed ones.
  local total=0 s n
  for s in queued in_progress waiting pending requested; do
    n=$(gh api "repos/$GH_REPO/actions/workflows/release.yml/runs?status=$s&per_page=1" \
      --jq .total_count </dev/null 2>/dev/null) || return 1
    case "$n" in "" | *[!0-9]*) return 1 ;; esac
    total=$((total + n))
  done
  printf '%s\n' "$total"
}

wait_for_capacity() { # block until fewer than the cap of Release runs are in flight
  local i=0 n cap=${CUT_TAGS_MAX_IN_FLIGHT:-4}
  while :; do
    n=$(in_flight) || die "FLIGHT-UNKNOWN cannot count Release runs in flight (gh failed); stopping rather than over-committing"
    [ "$n" -lt "$cap" ] && return 0
    [ "$i" -eq 0 ] && say "WAIT $n Release run(s) in flight (cap $cap); waiting for one to finish"
    i=$((i + 1))
    [ "$i" -lt "${CUT_TAGS_FLIGHT_TRIES:-120}" ] || die "FLIGHT-TIMEOUT still $n Release run(s) in flight; stopping"
    sleep "${CUT_TAGS_FLIGHT_SLEEP:-30}"
  done
}

run_done() { # <run id>: wait until a run completes; prints its conclusion
  local i=0 st
  while :; do
    st=$(gh run view "$1" -R "$GH_REPO" --json status,conclusion -q '"\(.status) \(.conclusion)"' </dev/null 2>/dev/null || true)
    case "$st" in completed*)
      printf '%s\n' "${st#completed }"
      return 0
      ;;
    esac
    i=$((i + 1))
    [ "$i" -lt "${CUT_TAGS_SEAL_TRIES:-90}" ] || {
      printf 'timeout\n'
      return 0
    }
    sleep "${CUT_TAGS_SEAL_SLEEP:-20}"
  done
}

run_attempt() { # <run id>: its current attempt number, or nothing
  gh run view "$1" -R "$GH_REPO" --json attempt -q .attempt </dev/null 2>/dev/null || true
}

still_rerunnable() { # <tag> <run id> <commit> <errfile>: re-read; print why not, or nothing
  local info st concl hsha rel
  info=$(gh run view "$2" -R "$GH_REPO" --json status,conclusion,headSha \
    -q '"\(.status) \(.conclusion) \(.headSha)"' </dev/null 2>/dev/null) || {
    echo "the run could not be read again"
    return 1
  }
  st=$(printf '%s' "$info" | awk '{print $1}')
  concl=$(printf '%s' "$info" | awk '{print $2}')
  hsha=$(printf '%s' "$info" | awk '{print $3}')
  [ "$st" = completed ] || {
    echo "it is running again ($st)"
    return 1
  }
  [ "$concl" != success ] || {
    echo "it already succeeded"
    return 1
  }
  [ "$hsha" = "$3" ] || {
    echo "the run is now for $hsha, not $3"
    return 1
  }
  if ! rel=$(gh release view "$1" -R "$GH_REPO" --json isDraft -q .isDraft </dev/null 2>"$4"); then
    if grep -q "release not found" "$4"; then rel=missing; else rel=unreadable; fi
  fi
  case "$rel" in
    true | missing) ;;
    false)
      echo "the release has been published since; immutable, not re-running"
      return 1
      ;;
    *)
      echo "the release state is now $rel; not re-running"
      return 1
      ;;
  esac
  probe_rerun_from_main "$1" "$3" >"$4" 2>&1 || {
    echo "origin/main's probe now refuses $1 at $3"
    return 1
  }
  return 0
}

rerun_failed() {
  # Recovery after a wave whose runs failed (e.g. the Actions-token API limit:
  # "HTTP 403: API rate limit exceeded for installation" at create-release).
  # A draft is still mutable, so re-running the FAILED jobs of its run finishes
  # it. Never deletes a release, never moves or re-pushes a tag. A re-run
  # executes release.yml AS IT IS AT THE TAGGED COMMIT, so every candidate must
  # pass main's pipeline probe on that commit first, and the run must be for it.
  local dry=$1 fail=0 t info id st concl hsha commit rel queue="" n_q=0 batch="" n_b=0 handled=0 entry bt bid errf
  local why attempt now i requested
  fetch_and_check_self
  new_tempfile
  errf=$NEW_TMP
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    handled=$((handled + 1))
    info=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$t" --limit 1 \
      --json databaseId,status,conclusion,headSha \
      -q '.[0] | "\(.databaseId) \(.status) \(.conclusion) \(.headSha)"' </dev/null 2>/dev/null || true)
    id=${info%% *}
    st=$(printf '%s' "$info" | awk '{print $2}')
    concl=$(printf '%s' "$info" | awk '{print $3}')
    hsha=$(printf '%s' "$info" | awk '{print $4}')
    # Only GitHub's explicit "release not found" means missing; any other
    # failure (a 403, a rate limit, the network) is an unknown state.
    if ! rel=$(gh release view "$t" -R "$GH_REPO" --json isDraft -q .isDraft </dev/null 2>"$errf"); then
      if grep -q "release not found" "$errf"; then rel=missing; else rel="lookup failed: $(head -c 120 "$errf" | tr '\n' ' ')"; fi
    fi
    case "$id" in "" | null)
      say "NO-RUN $t: no Release run exists for this tag, so there is nothing to re-run (needs a human)"
      fail=1
      continue
      ;;
    esac
    if [ "$st" != "completed" ]; then
      say "IN-FLIGHT $t run $id ($st)"
      continue
    fi
    if [ "$concl" = "success" ]; then
      say "OK $t run $id succeeded (release isDraft=$rel)"
      continue
    fi
    case "$rel" in
      true | missing) ;; # still a mutable draft, or never created: a re-run can finish it
      false)
        say "SEALED-PARTIAL $t: published despite run $id ($concl); immutable, a re-run cannot add assets - bump the version"
        fail=1
        continue
        ;;
      *)
        say "UNKNOWN-STATE $t: release state '$rel' unreadable; not re-running run $id"
        fail=1
        continue
        ;;
    esac
    if ! commit=$(git -C "$REPO" rev-parse -q --verify "refs/tags/$t^{commit}"); then
      say "UNKNOWN-STATE $t: the tag is not in this clone, so its release pipeline cannot be checked; not re-running"
      fail=1
      continue
    fi
    if [ "$hsha" != "$commit" ]; then
      say "UNKNOWN-STATE $t: run $id is for $hsha, not the tagged commit $commit; not re-running"
      fail=1
      continue
    fi
    if ! probe_rerun_from_main "$t" "$commit" >"$errf" 2>&1; then
      sed 's/^/    /' "$errf"
      say "PROBE-REFUSED $t: release.yml at $commit must not be re-run (it could seal an incomplete immutable release)"
      fail=1
      continue
    fi
    queue="$queue$t $id $commit
"
    n_q=$((n_q + 1))
  done <<<"$TAG_LIST"
  [ "$handled" -eq "$TAG_COUNT" ] || die "INCOMPLETE checked $handled of $TAG_COUNT listed tags"
  say "rerun-failed: $n_q failed run(s) to re-run, ${CUT_TAGS_RERUN_BATCH:-3} at a time$([ "$dry" = 1 ] && echo ' (dry run: nothing is re-run)')"
  if [ "$dry" = 1 ]; then
    while IFS= read -r entry; do
      [ -n "$entry" ] && say "WOULD-RERUN $(printf '%s' "$entry" | awk '{print $1, $2}')"
    done <<<"$queue"
    rm -f "$errf"
    return "$fail"
  fi
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    tag_lock
    wait_for_capacity
    bt=$(printf '%s' "$entry" | awk '{print $1}')
    bid=$(printf '%s' "$entry" | awk '{print $2}')
    commit=$(printf '%s' "$entry" | awk '{print $3}')
    # Everything that justified this re-run is read again now: another operator
    # may have finished it, sealed the release, or main may have changed.
    if ! why=$(still_rerunnable "$bt" "$bid" "$commit" "$errf"); then
      tag_unlock
      say "SKIP $bt: $why"
      case "$why" in "it already succeeded"* | "it is running"*) ;; *) fail=1 ;; esac
      continue
    fi
    attempt=$(run_attempt "$bid")
    case "$attempt" in "" | *[!0-9]*)
      tag_unlock
      say "SKIP $bt: run $bid's attempt number could not be read; not re-running"
      fail=1
      continue
      ;;
    esac
    requested=ok
    gh run rerun "$bid" -R "$GH_REPO" --failed </dev/null >/dev/null 2>&1 || requested=error
    # Whatever the request returned, hold the lock until the new attempt is
    # visibly registered: an error can still mean GitHub accepted it (a lost
    # response), and until it registers the next capacity check would see
    # room it does not have. No new attempt after an error is a stop, not a
    # guess.
    i=0
    now=$attempt
    while [ "$i" -lt "${CUT_TAGS_RUN_TRIES:-30}" ]; do
      now=$(run_attempt "$bid")
      case "$now" in "" | *[!0-9]*) ;; *) [ "$now" -gt "$attempt" ] && break ;; esac
      i=$((i + 1))
      sleep "${CUT_TAGS_RUN_SLEEP:-5}"
    done
    case "$now" in "" | *[!0-9]*) now=$attempt ;; esac
    if [ "$now" -le "$attempt" ]; then
      [ "$requested" = ok ] && die "RERUN-NOT-SEEN $bt $bid: re-run requested but no new attempt registered; stopping"
      die "RERUN-REFUSED $bt $bid: the request failed and no new attempt registered; stopping (cannot tell what GitHub did)"
    fi
    if [ "$requested" = ok ]; then
      say "RERUN $bt $bid (attempt $now)"
    else
      say "RERUN $bt $bid (attempt $now; the request errored but GitHub registered it)"
    fi
    tag_unlock # the re-run is registered (or refused): the next one may count capacity
    batch="$batch$bt $bid
"
    n_b=$((n_b + 1))
    if [ "$n_b" -ge "${CUT_TAGS_RERUN_BATCH:-3}" ]; then
      while IFS= read -r entry; do
        [ -n "$entry" ] || continue
        say "DONE ${entry%% *} ${entry##* } $(run_done "${entry##* }")"
      done <<<"$batch"
      batch=""
      n_b=0
    fi
  done <<<"$queue"
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    say "DONE ${entry%% *} ${entry##* } $(run_done "${entry##* }")"
  done <<<"$batch"
  rm -f "$errf"
  say "rerun-failed: re-runs finished; receipts follow"
  receipts || fail=1
  return "$fail"
}

probe_from_main() { # <probe args...>: origin/main's probe, freshly fetched, on this clone
  # The probe imports nothing local and reads burned_versions.json beside
  # itself, so the two files from main's tip ARE main's probe; extracting them
  # per check is cheap, so the probe is never older than the moment it runs.
  local d rc=0
  fetch_retry "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main" || return 1
  MAIN_SHA=$(git -C "$REPO" rev-parse "refs/remotes/$REMOTE/main")
  new_tempdir
  d=$NEW_TMP
  if ! git -C "$REPO" show "$MAIN_SHA:tools/maintainer/check_release_pipeline.py" >"$d/check_release_pipeline.py" ||
    ! git -C "$REPO" show "$MAIN_SHA:tools/maintainer/burned_versions.json" >"$d/burned_versions.json"; then
    rm -rf "$d"
    return 1
  fi
  "$PYTHON" "$d/check_release_pipeline.py" --repo "$REPO" "$@" </dev/null || rc=$?
  rm -rf "$d"
  return "$rc"
}

probe_rerun_from_main() { # <tag> <commit>: may this cut tag's release run again?
  # origin/main's probe, freshly fetched, with every requirement a NEW tag must
  # meet except "not cut yet": the commit's pipeline drafts and seals last, the
  # version is not burned, the commit's manifest carries that version.
  local d rc=0
  fetch_retry "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main" || return 1
  MAIN_SHA=$(git -C "$REPO" rev-parse "refs/remotes/$REMOTE/main")
  new_tempdir
  d=$NEW_TMP
  if ! git -C "$REPO" show "$MAIN_SHA:tools/maintainer/check_release_pipeline.py" >"$d/check_release_pipeline.py" ||
    ! git -C "$REPO" show "$MAIN_SHA:tools/maintainer/burned_versions.json" >"$d/burned_versions.json"; then
    rm -rf "$d"
    return 1
  fi
  "$PYTHON" - "$d" "$REPO" "$1" "$2" <<'PY' || rc=$?
import sys
from pathlib import Path
sys.path.insert(0, sys.argv[1])
import check_release_pipeline as p
repo, tag, sha = Path(sys.argv[2]), sys.argv[3], sys.argv[4]
try:
    fails = p.evaluate(p.workflow_at(sha, repo), p.blob_at(sha, p.WINDOWS_SMOKE_WORKFLOW, repo))
    m = p.TAG_RE.match(tag)
    mv = p.manifest_version_at(sha, m.group("slug"), repo) if m else None
    fails += p.evaluate_tag(tag, burned=p.load_burned(), tag_local=False, tag_remote=False, manifest_version=mv)
except Exception as exc:  # an unreadable workflow or ledger proves nothing: refuse
    fails = [("PROBE-ERROR", str(exc))]
for rid, why in fails:
    print(f"  - [{rid}] {why}")
sys.exit(1 if fails else 0)
PY
  rm -rf "$d"
  return "$rc"
}

remote_tag_commit() { # the commit a remote tag points at (peeled), or nothing
  local out
  out=$(git -C "$REPO" ls-remote "$REMOTE" "refs/tags/$1" "refs/tags/$1^{}") || return 1
  printf '%s\n' "$out" | awk -v t="refs/tags/$1" '$2 == t"^{}" {p=$1} $2 == t {d=$1} END {print (p != "" ? p : d)}'
}

cut() {
  local sha=$1 dry=$2 full t slug ver remote_at run i probe_out shawt handled=0
  fetch_and_check_self
  full=$(git -C "$REPO" rev-parse --verify -q "${sha}^{commit}") || die "REFUSE $sha is not a commit in $REPO"
  worktree_at "$full"
  shawt=$NEW_WT
  new_tempfile
  probe_out=$NEW_TMP
  say "cut_tags: $TAG_COUNT tag(s) at $full$([ "$dry" = 1 ] && echo ' (dry run: nothing is pushed)')"
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    slug=${t%-v*}
    ver=${t##*-v}
    # Capacity first: the checks below must be the last thing before the push,
    # not something done before an hour-long wait (main can retire a version
    # or someone can cut the tag meanwhile). The lock is held from here until
    # this tag's Release run is registered.
    if [ "$dry" != 1 ]; then
      tag_lock
      wait_for_capacity
    fi
    remote_at=$(remote_tag_commit "$t") || die "REFUSE $t: cannot ask $REMOTE whether it exists"
    if [ -n "$remote_at" ]; then
      if [ "$remote_at" = "$full" ]; then
        say "SKIP $t (already cut at this SHA)"
        handled=$((handled + 1))
        tag_unlock
        continue
      fi
      die "EXISTS-ELSEWHERE $t points at $remote_at, not $full (stopping; never move a release tag)"
    fi
    if ! probe_from_main --tag "$t" --sha "$full" >"$probe_out" 2>&1; then
      sed 's/^/    /' "$probe_out"
      die "PROBE-REFUSED $t (origin/main ${MAIN_SHA:0:12}'s probe)"
    fi
    if ! (cd "$shawt" && "$PYTHON" tools/maintainer/changelog_section.py --slug "$slug" --version "$ver" --check) >"$probe_out" 2>&1; then
      sed 's/^/    /' "$probe_out"
      die "NOTES-REFUSED $t (write the CHANGELOG section at that commit first)"
    fi
    if [ "$dry" = 1 ]; then
      say "WOULD-PUSH $t at $full (probe and notes OK; Release runs in flight now: $(in_flight || echo unknown), cap ${CUT_TAGS_MAX_IN_FLIGHT:-4})"
      handled=$((handled + 1))
      continue
    fi
    # Exactly this one ref: a user's push.followTags=true would otherwise add
    # every reachable annotated tag to the push, unprobed and uncapped.
    git -C "$REPO" -c push.followTags=false push --no-follow-tags -q "$REMOTE" "${full}:refs/tags/${t}" </dev/null
    say "PUSHED $t"
    run=""
    i=0
    while [ "$i" -lt "${CUT_TAGS_RUN_TRIES:-30}" ]; do
      run=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$t" --json databaseId,headSha \
        -q ".[] | select(.headSha == \"$full\") | .databaseId" </dev/null 2>/dev/null | head -1 || true)
      [ -n "$run" ] && break
      i=$((i + 1))
      sleep "${CUT_TAGS_RUN_SLEEP:-5}"
    done
    [ -n "$run" ] || die "NO-RUN $t: the tag is pushed but no Release run registered (check Actions; stopping)"
    say "RUN $t $run"
    tag_unlock
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
      json=$(gh release view "$t" -R "$GH_REPO" --json isDraft,assets </dev/null 2>/dev/null || true)
      if [ -n "$json" ]; then
        draft=$(printf '%s' "$json" | "$PYTHON" -c 'import json,sys; print(str(json.load(sys.stdin).get("isDraft")).lower())' 2>/dev/null || echo unreadable)
        [ "$draft" = "false" ] && break
      fi
      st=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$t" --json status,conclusion \
        -q '.[0] | "\(.status) \(.conclusion)"' </dev/null 2>/dev/null || true)
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
  drafts=$(gh api "repos/$GH_REPO/releases" --paginate -q '.[] | select(.draft) | .tag_name' </dev/null 2>/dev/null) \
    || { say "DRAFT-SWEEP-FAILED (could not list releases)"; return 1; }
  # A draft whose Release run is still going is a release in flight (another
  # wave, or this one's slowest target), not one that never finished.
  for d in $drafts; do
    st=$(gh run list -R "$GH_REPO" --workflow release.yml --branch "$d" --json status -q '.[0].status' </dev/null 2>/dev/null || true)
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
  # Each case is `<condition list>; expect "label" $?`. Under set -e a list
  # whose LAST clause fails would end the whole self-test silently instead of
  # reporting that case, so the harness runs without it (the tool runs it calls
  # are separate processes with their own set -e).
  set +e
  local tmp failures=0 out rc rrc real_repo sha sha2 sha3 rs pushes_before before_wait before_push good bad at v pa pb ra rb
  local hook_child rc_free
  real_repo=$(git -C "$TOOLS" rev-parse --show-toplevel)
  # Without set -e, an unchecked failure here would point every fixture write
  # at the filesystem root: stop instead.
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/cut_tags_selftest.XXXXXX") || { echo "FAIL: cannot create a temporary directory"; return 1; }
  [ -n "$tmp" ] && [ -d "$tmp" ] || { echo "FAIL: mktemp returned no directory"; return 1; }
  SELFTEST_TMP=$tmp
  trap 'rm -rf "$SELFTEST_TMP"' EXIT
  export CT_STATE="$tmp"  # read by the generated fixture scripts at run time
  export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$tmp/gitconfig"
  printf '[user]\n\tname = Self Test\n\temail = selftest@example.invalid\n[init]\n\tdefaultBranch = main\n' >"$GIT_CONFIG_GLOBAL"

  has() { # <text>: is it in $out? (no pipe: `printf | grep -q` under pipefail
    # fails when grep exits early and printf takes SIGPIPE)
    case "$out" in *"$1"*) return 0 ;; esac
    return 1
  }
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

  # origin: a bare repo whose pre-receive hook records how many refs each push
  # carried and, for a tag, how many Release runs were in flight when it was
  # pushed (its own run included), starting that tag's (fake) Release run.
  git init -q --bare "$tmp/origin.git"
  mkdir -p "$tmp/runs"
  cat >"$tmp/origin.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
state=$(cd "$(dirname "$0")/../.." && pwd)
n=0
while read -r _old _new ref; do
  n=$((n + 1))
  case "$ref" in refs/tags/*)
    # Start this tag's run FIRST, then count: runs in flight including it, so
    # two pushes landing at once are both counted by whichever hook is later.
    echo in_progress >"$state/runs/${ref#refs/tags/}"
    grep -l in_progress "$state"/runs/* 2>/dev/null | wc -l | tr -d ' ' >>"$state/flight.txt" ;;
  esac
done
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
  # (unless the tag is listed in no-run.txt); the in-flight count is the runs
  # the hook started, one of which finishes on every third count (time
  # passing); releases, run states and re-runs come from files, and any delete
  # is recorded as FORBIDDEN.
  mkdir -p "$tmp/bin" "$tmp/releases" "$tmp/elsewhere" "$tmp/rerun"
  # Quoted heredoc: the fixture's own directory is read from CT_STATE when the
  # fake runs, never pasted into its source (a path with "$" or a backtick
  # would otherwise become code).
  cat >"$tmp/bin/gh" <<'FAKE'
#!/usr/bin/env bash
set -u
state=${CT_STATE:?}
case "$*" in *delete*|*DELETE*) echo "FORBIDDEN $*" >>"$state/calls.txt"; exit 3 ;; esac
case "$1 $2" in
  "run list")
    tag=""; prev=""; want=""
    for a in "$@"; do [ "$prev" = "--branch" ] && tag=$a; [ "$prev" = "-q" ] && want=$a; prev=$a; done
    if [ -z "$tag" ]; then
      n=$(grep -l in_progress "$state"/runs/* 2>/dev/null | wc -l | tr -d ' ')
      echo x >>"$state/counts.txt"
      if [ $(($(wc -l <"$state/counts.txt") % 3)) -eq 0 ]; then
        oldest=$(grep -l in_progress "$state"/runs/* 2>/dev/null | head -1)
        [ -n "$oldest" ] && echo completed >"$oldest"
      fi
      echo "$n"; exit 0
    fi
    if grep -qx "$tag" "$state/no-run.txt" 2>/dev/null; then exit 0; fi
    case "$*" in
      *databaseId,headSha*)
        c=$(git --git-dir "$state/origin.git" rev-parse -q --verify "refs/tags/$tag^{commit}" 2>/dev/null) || exit 0
        case "$want" in *"$c"*) echo 4242 ;; esac ;;
      *databaseId,status,conclusion*) cat "$state/releases/$tag.runinfo" 2>/dev/null || echo "null null null" ;;
      *conclusion*) cat "$state/releases/$tag.run" 2>/dev/null || true ;;
      *status*) cat "$state/releases/$tag.status" 2>/dev/null || true ;;
    esac ;;
  "run rerun")
    if [ -f "$state/rerun-refused" ]; then echo "HTTP 403" >&2; exit 1; fi
    echo "rerun $3" >>"$state/calls.txt"; echo 0 >"$state/rerun/$3"
    if [ -f "$state/rerun-lost-response" ]; then echo "connection reset" >&2; exit 1; fi ;;
  "run view")
    case "$*" in *"--json attempt"*)
      # A re-run's new attempt registers only on the second look (Actions
      # lags); with never-register it never does.
      if [ ! -f "$state/rerun/$3" ]; then echo 1; exit 0; fi
      r=$(cat "$state/reg-$3" 2>/dev/null || echo 0)
      r=$((r + 1)); echo "$r" >"$state/reg-$3"
      if [ -f "$state/never-register" ] || [ "$r" -lt 2 ]; then echo 1; exit 0; fi
      if [ ! -f "$state/runs/rr-$3" ]; then
        echo in_progress >"$state/runs/rr-$3"
        echo "registered $3" >>"$state/calls.txt"
      fi
      echo 2; exit 0 ;;
    esac
    if [ ! -f "$state/rerun/$3" ]; then  # not re-run by us: its recorded state
      line=$(grep -h "^$3 " "$state"/releases/*.runinfo 2>/dev/null | head -1)
      [ -n "$line" ] || exit 1
      echo "${line#* }"; exit 0
    fi
    v=$(cat "$state/rerun/$3")
    if [ "$v" -lt 1 ]; then echo 1 >"$state/rerun/$3"; echo "in_progress null"
    else
      echo "done $3" >>"$state/calls.txt"; echo "completed success"
      [ -f "$state/runs/rr-$3" ] && echo completed >"$state/runs/rr-$3"
      [ -f "$state/on-done-$3.sh" ] && sh "$state/on-done-$3.sh"
    fi ;;
  "release view")
    if [ -f "$state/releases/$3.error" ]; then cat "$state/releases/$3.error" >&2; exit 1; fi
    [ -f "$state/releases/$3.json" ] || { echo "release not found" >&2; exit 1; }
    case "$*" in
      *"-q .isDraft"*) sed -nE 's/.*"isDraft": *(true|false).*/\1/p' "$state/releases/$3.json" ;;
      *) cat "$state/releases/$3.json" ;;
    esac ;;
  "api repos/fake/repo/releases") cat "$state/drafts.txt" 2>/dev/null || true ;;
  "api repos/fake/repo/actions/workflows/release.yml/runs?status=in_progress&per_page=1")
    if [ -f "$state/on-count.sh" ]; then mv "$state/on-count.sh" "$state/on-count.ran"; sh "$state/on-count.ran" >/dev/null 2>&1; fi
    n=$(grep -l in_progress "$state"/runs/* 2>/dev/null | wc -l | tr -d ' ')
    [ -f "$state/slow-count" ] && sleep 0.3  # widen the check-then-push window for the concurrency test
    echo x >>"$state/counts.txt"
    if [ -f "$state/drain-at" ]; then
      # Deterministic: a run finishes only while someone is held at the cap.
      if [ "$n" -ge "$(cat "$state/drain-at")" ]; then
        oldest=$(grep -l in_progress "$state"/runs/* 2>/dev/null | head -1)
        [ -n "$oldest" ] && echo completed >"$oldest"
      fi
    elif [ $(($(wc -l <"$state/counts.txt") % 3)) -eq 0 ]; then
      oldest=$(grep -l in_progress "$state"/runs/* 2>/dev/null | head -1)
      [ -n "$oldest" ] && echo completed >"$oldest"
    fi
    echo "$n" ;;
  "api repos/fake/repo/actions/workflows/release.yml/runs?status="*) echo 0 ;;
  *) echo "fake gh: unhandled $*" >&2; exit 2 ;;
esac
FAKE
  chmod +x "$tmp/bin/gh"
  export PATH="$tmp/bin:$PATH" CUT_TAGS_REPO="$tmp/repo" CUT_TAGS_GH_REPO=fake/repo
  export CUT_TAGS_RUN_TRIES=2 CUT_TAGS_RUN_SLEEP=0 CUT_TAGS_SPACING=0 CUT_TAGS_SEAL_TRIES=2 CUT_TAGS_SEAL_SLEEP=0

  echo "tag list handling (the false ALL-PUSHED):"
  printf 'zz-one-v0.1.0\n' >"$tmp/elsewhere/tags-ok.txt"
  out=$(cd "$tmp/elsewhere" && bash "$SELF" --dry-run "$sha" tags-ok.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && has "WOULD-PUSH zz-one-v0.1.0" \
    && has "DRY-RUN-COMPLETE 1 of 1" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.1.0 >/dev/null
  expect "a dry run with a RELATIVE tags path checks the tag, counts 1 of 1, and pushes nothing" $?
  out=$(cd "$tmp/elsewhere" && bash "$SELF" "$sha" missing.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && ! has "ALL-PUSHED"
  expect "a missing tags file fails, and never prints ALL-PUSHED" $?
  printf '# nothing here\n\n' >"$tmp/empty.txt"
  out=$(bash "$SELF" "$sha" "$tmp/empty.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "lists no tags"
  expect "a tags file with no tags fails" $?
  printf 'zz-one-v0.1.0\nzz-one-v0.1.0\n' >"$tmp/dup.txt"
  out=$(bash "$SELF" "$sha" "$tmp/dup.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "listed twice"
  expect "a tag listed twice fails before anything is pushed" $?

  echo "temporary paths with spaces:"
  mkdir -p "$tmp/Release Work" "$tmp/Release"
  echo "someone's work" >"$tmp/Release/keep.txt"
  out=$(cd "$tmp/elsewhere" && TMPDIR="$tmp/Release Work" bash "$SELF" --dry-run "$sha" tags-ok.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && has "DRY-RUN-COMPLETE 1 of 1" && [ -f "$tmp/Release/keep.txt" ] \
    && [ -z "$(ls -A "$tmp/Release Work")" ] && [ "$(git -C "$tmp/repo" worktree list | grep -c .)" -eq 1 ]
  expect "under a TMPDIR containing a space it runs, cleans only its own directories, and spares the sibling" $?
  out=$(cd "$tmp/elsewhere" && TMPDIR="$tmp/Release${NL}Work" bash "$SELF" --dry-run "$sha" tags-ok.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "TMPDIR contains a newline" && [ -f "$tmp/Release/keep.txt" ]
  expect "a TMPDIR containing a newline is refused before anything is created; the sibling survives" $?

  echo "push mode:"
  out=$(cd "$tmp/elsewhere" && bash "$SELF" "$sha" tags-ok.txt 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && has "RUN zz-one-v0.1.0 4242" \
    && has "ALL-PUSHED 1 of 1" \
    && [ "$(git --git-dir "$tmp/origin.git" rev-parse refs/tags/zz-one-v0.1.0)" = "$sha" ]
  expect "an endorsed tag is pushed at the SHA, its Release run is seen, 1 of 1" $?
  out=$(bash "$SELF" "$sha" "$tmp/elsewhere/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && has "SKIP zz-one-v0.1.0" && has "ALL-PUSHED 1 of 1"
  expect "re-running skips a tag already cut at this SHA and still counts it" $?
  printf 'zz-one-v0.2.0\n' >"$tmp/tags-badver.txt"
  out=$(bash "$SELF" "$sha" "$tmp/tags-badver.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "PROBE-REFUSED zz-one-v0.2.0" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.2.0 >/dev/null
  expect "a tag the probe refuses (manifest says 0.1.0) is not pushed" $?
  printf 'zz-two-v0.1.0\n' >"$tmp/tags-notes.txt"
  out=$(bash "$SELF" "$sha" "$tmp/tags-notes.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "NOTES-REFUSED zz-two-v0.1.0"
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
  [ "$rc" -ne 0 ] && has "PROBE-REFUSED zz-one-v0.1.9" && has "TAG-BURNED"
  expect "cut_tags runs origin/main's probe, so the version main retired is refused" $?
  out=$(bash "$SELF" "$sha2" "$tmp/elsewhere/tags-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "EXISTS-ELSEWHERE zz-one-v0.1.0" \
    && ! has "PROBE-REFUSED"
  expect "a tag already on origin at another commit stops the run before anything else" $?
  printf '# wave\n\nzz-one-v0.1.8\n' >"$tmp/tags-norun.txt"
  connector zz-one 0.1.8
  printf '# Changelog\n\n## [0.1.8] - 2026-10-22\n\n### Fixed\n- A fix.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  git -C "$tmp/repo" commit -q -am "zz-one 0.1.8" -s
  git -C "$tmp/repo" push -q origin main
  echo "zz-one-v0.1.8" >"$tmp/no-run.txt"
  out=$(bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-norun.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "NO-RUN zz-one-v0.1.8" && ! has "ALL-PUSHED"
  expect "a pushed tag whose Release run never registers stops the run" $?

  echo "releases in flight are capped (the 52-run API-limit wave):"
  for s in zz-c1 zz-c2 zz-c3 zz-c4 zz-c5 zz-c6; do
    connector "$s" 0.1.0
    printf '# Changelog\n\n## [0.1.0] - 2026-10-22\n\n### Added\n- First release.\n' >"$tmp/repo/skills/$s/CHANGELOG.md"
    [ "$s" = zz-c6 ] || echo "$s-v0.1.0"
  done >"$tmp/tags-wave.txt"
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "five connectors" -s
  git -C "$tmp/repo" push -q origin main
  rm -f "$tmp"/runs/* "$tmp/flight.txt"
  # Unrelated annotated tags reachable from the SHA, and push.followTags=true
  # as a user might have it: every push must still carry exactly its one tag.
  git -C "$tmp/repo" tag -a decoy-annotated-1 -m decoy HEAD
  git -C "$tmp/repo" tag -a decoy-annotated-2 -m decoy HEAD~1
  export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=push.followTags GIT_CONFIG_VALUE_0=true
  echo in_progress >"$tmp/runs/aa-older-wave-1"  # long-running releases from an earlier wave
  echo in_progress >"$tmp/runs/aa-older-wave-2"
  out=$(CUT_TAGS_MAX_IN_FLIGHT=2 CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-wave.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && has "ALL-PUSHED 5 of 5" && has "WAIT 2 Release run(s) in flight (cap 2)" \
    && before_wait=${out%%WAIT *} && before_push=${out%%PUSHED *} && [ "${#before_wait}" -lt "${#before_push}" ] \
    && [ "$(sort -n "$tmp/flight.txt" | tail -1)" -le 2 ] && [ "$(grep -c . "$tmp/flight.txt")" -eq 5 ]
  expect "with a cap of 2 and 2 older runs still going, it waits first, then never has more than 2 in flight" $?
  unset GIT_CONFIG_COUNT GIT_CONFIG_KEY_0 GIT_CONFIG_VALUE_0
  rc=1
  if [ -z "$(git --git-dir "$tmp/origin.git" tag -l 'decoy-*')" ]; then rc=0; fi
  expect "with push.followTags=true and annotated tags reachable, no tag but the requested one is pushed" "$rc"
  # While the run waits for capacity, main retires zz-c6-v0.1.0: the push must
  # be refused by the probe run AFTER the wait, against main as it is then.
  echo in_progress >"$tmp/runs/aa-older-wave-3"
  cat >"$tmp/on-count.sh" <<'RETIRE'
cd "${CT_STATE:?}/repo" || exit 1
printf '{\n  "burned": [\n    {"tag": "zz-c6-v0.1.0", "date": "2026-10-22", "why": "fixture: retired mid-wait"}\n  ]\n}\n' >tools/maintainer/burned_versions.json
git commit -q -am "retire zz-c6-v0.1.0" -s && git push -q origin main
RETIRE
  printf 'zz-c6-v0.1.0\n' >"$tmp/tags-c6.txt"
  out=$(CUT_TAGS_MAX_IN_FLIGHT=1 CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-c6.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && [ -f "$tmp/on-count.ran" ] && has "WAIT" && has "PROBE-REFUSED zz-c6-v0.1.0" && has "TAG-BURNED" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-c6-v0.1.0 >/dev/null
  expect "a version main retires while the push waits for capacity is refused (checks run after the wait)" $?

  echo "two cutters at once share the cap (the release-tag lock):"
  for s in zz-e1 zz-e2 zz-e3 zz-e4 zz-e5 zz-e6; do
    connector "$s" 0.1.0
    printf '# Changelog\n\n## [0.1.0] - 2026-10-22\n\n### Added\n- First release.\n' >"$tmp/repo/skills/$s/CHANGELOG.md"
  done
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "six more connectors" -s
  git -C "$tmp/repo" push -q origin main
  printf 'zz-e1-v0.1.0\nzz-e2-v0.1.0\nzz-e3-v0.1.0\n' >"$tmp/tags-a.txt"
  printf 'zz-e4-v0.1.0\nzz-e5-v0.1.0\nzz-e6-v0.1.0\n' >"$tmp/tags-b.txt"
  rm -f "$tmp"/runs/* "$tmp/flight.txt"
  echo in_progress >"$tmp/runs/aa-older-wave-4"
  : >"$tmp/slow-count"
  echo 2 >"$tmp/drain-at"
  sha3=$(git -C "$tmp/repo" rev-parse HEAD)
  CUT_TAGS_MAX_IN_FLIGHT=2 CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" "$sha3" "$tmp/tags-a.txt" >"$tmp/out-a.txt" 2>&1 &
  pa=$!
  CUT_TAGS_MAX_IN_FLIGHT=2 CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" "$sha3" "$tmp/tags-b.txt" >"$tmp/out-b.txt" 2>&1 &
  pb=$!
  wait "$pa"
  ra=$?
  wait "$pb"
  rb=$?
  rm -f "$tmp/slow-count" "$tmp/drain-at"
  rc=1
  if [ "$ra" -eq 0 ] && [ "$rb" -eq 0 ] && grep -q "ALL-PUSHED 3 of 3" "$tmp/out-a.txt" \
    && grep -q "ALL-PUSHED 3 of 3" "$tmp/out-b.txt" && [ "$(grep -c . "$tmp/flight.txt")" -eq 6 ] \
    && [ "$(sort -n "$tmp/flight.txt" | tail -1)" -le 2 ]; then rc=0; fi
  expect "two concurrent cutters with a cap of 2 push all 6 tags and never have more than 2 in flight" "$rc"
  printf 'zz-one-v0.1.7\n' >"$tmp/tags-unknown.txt"
  connector zz-one 0.1.7
  printf '# Changelog\n\n## [0.1.7] - 2026-10-22\n\n### Fixed\n- A fix.\n' >"$tmp/repo/skills/zz-one/CHANGELOG.md"
  git -C "$tmp/repo" commit -q -am "zz-one 0.1.7" -s
  git -C "$tmp/repo" push -q origin main
  out=$(PATH="$tmp/nogh:$PATH" bash -c 'mkdir -p "$1"; printf "#!/bin/sh\nexit 1\n" >"$1/gh"; chmod +x "$1/gh"; shift; bash "$@"' _ "$tmp/nogh" "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-unknown.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "FLIGHT-UNKNOWN" \
    && ! git --git-dir "$tmp/origin.git" rev-parse -q --verify refs/tags/zz-one-v0.1.7 >/dev/null
  expect "when the in-flight count cannot be read, nothing is pushed" $?
  rc=1
  if [ "$(grep -vcx 1 "$tmp/origin.git/pushes.txt")" -eq 0 ] \
    && [ "$(grep -c . "$tmp/origin.git/pushes.txt")" -ge 8 ]; then rc=0; fi
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
  [ "$rc" -eq 0 ] && has "SEALED zz-new-v0.1.0 assets=$rs" \
    && has "drafts sweep: none" && has "checked 1 of 1"
  expect "receipts check against the tagged tree: a complete release of a new connector is SEALED" $?
  printf 'zz-new-v9.9.8\n' >"$tmp/r-missing.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-missing.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "NOT-COMPLETE zz-new-v9.9.8"
  expect "a published release missing one asset is not" $?
  printf 'zz-new-v9.9.7\n' >"$tmp/r-draft.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-draft.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "RUN-FAILED zz-new-v9.9.7"
  expect "a draft whose Release run failed is reported as stranded" $?
  echo "zz-new-v9.9.6" >"$tmp/drafts.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && has "IN-FLIGHT drafts (their Release run is still running): zz-new-v9.9.6"
  expect "a draft whose Release run is still running is reported, not failed" $?
  printf 'zz-new-v9.9.7\nzz-new-v9.9.6\n' >"$tmp/drafts.txt"
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "DRAFTS (releases that never finished): zz-new-v9.9.7"
  expect "a draft whose Release run ended fails the sweep" $?
  out=$(bash "$SELF" --receipts "$tmp/nope.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && ! has "drafts sweep"
  expect "receipts for a missing tags file fail instead of reporting an empty success" $?

  echo "recovery (--rerun-failed):"
  # r1-r3 and r11 failed with their release still a draft or never created; r4
  # sealed fine; r5 published with assets missing (immutable); r6 has no run;
  # r7's release state is unreadable; r8's lookup gets a 403; r9 is tagged on a
  # commit whose release.yml publishes first; r10's run is for another commit;
  # r12's version is burned on main. Each rN is a real connector at 0.1.0.
  for v in 1 2 3 4 5 6 7 8 9 10 11 12; do connector "r$v" 0.1.0; done
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "recovery connectors" -s
  git -C "$tmp/repo" push -q origin main
  good=$(git -C "$tmp/repo" rev-parse HEAD)
  git -C "$tmp/repo" checkout -q --detach "$good"
  sed 's/ --draft//g' "$tmp/repo/.github/workflows/release.yml" >"$tmp/release.yml.unsafe"
  cp "$tmp/release.yml.unsafe" "$tmp/repo/.github/workflows/release.yml"
  git -C "$tmp/repo" commit -q -am "publish-first pipeline" -s
  bad=$(git -C "$tmp/repo" rev-parse HEAD)
  git -C "$tmp/repo" checkout -q main
  printf '{\n  "burned": [\n    {"tag": "zz-c6-v0.1.0", "date": "2026-10-22", "why": "fixture"},\n    {"tag": "r12-v0.1.0", "date": "2026-10-22", "why": "fixture: retired after its failed release was deleted"}\n  ]\n}\n' \
    >"$tmp/repo/tools/maintainer/burned_versions.json"
  git -C "$tmp/repo" commit -q -am "retire r12-v0.1.0" -s
  git -C "$tmp/repo" push -q origin main
  for v in 1 2 3 4 5 6 7 8 9 10 11 12; do
    at=$good
    [ "$v" = 9 ] && at=$bad
    git -C "$tmp/repo" push -q origin "$at:refs/tags/r$v-v0.1.0"
  done
  rm -f "$tmp"/runs/*
  for v in 1 3 11 12; do printf '{"isDraft": true, "assets": []}' >"$tmp/releases/r$v-v0.1.0.json"; done
  printf '{"isDraft": false, "assets": []}' >"$tmp/releases/r4-v0.1.0.json"
  printf '{"isDraft": false, "assets": []}' >"$tmp/releases/r5-v0.1.0.json"
  printf '{"assets": []}' >"$tmp/releases/r7-v0.1.0.json"  # draft state unreadable
  echo "HTTP 403: API rate limit exceeded for installation" >"$tmp/releases/r8-v0.1.0.error"
  for v in 1 2 3 4 5 7 8 9 10 11 12; do
    at=$good
    [ "$v" = 9 ] && at=$bad
    [ "$v" = 10 ] && at=$bad
    case $v in 4) echo "70$v completed success $at" ;; *) echo "70$v completed failure $at" ;; esac \
      >"$tmp/releases/r$v-v0.1.0.runinfo"
  done
  printf 'r%s-v0.1.0\n' 1 2 3 4 5 6 7 8 9 10 11 12 >"$tmp/tags-recover.txt"
  : >"$tmp/calls.txt"
  : >"$tmp/drafts.txt"
  out=$(CUT_TAGS_RERUN_BATCH=2 CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" --dry-run --rerun-failed "$tmp/tags-recover.txt" 2>&1) && rrc=0 || rrc=$?
  ! grep -q "^rerun" "$tmp/calls.txt" && has "WOULD-RERUN r1-v0.1.0 701" && has "WOULD-RERUN r3-v0.1.0 703"
  expect "--dry-run --rerun-failed lists what it would re-run and re-runs nothing" $?
  pushes_before=$(grep -c . "$tmp/origin.git/pushes.txt")
  # While the first batch runs, another operator seals r3.
  cat >"$tmp/on-done-702.sh" <<'SEAL'
printf '{"isDraft": false, "assets": []}' >"${CT_STATE:?}/releases/r3-v0.1.0.json"
SEAL
  out=$(CUT_TAGS_RERUN_BATCH=2 CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" --rerun-failed "$tmp/tags-recover.txt" 2>&1) && rrc=0 || rrc=$?
  rc=1
  if [ "$(grep '^rerun' "$tmp/calls.txt" | awk '{print $2}' | sort -n | tr '\n' ' ')" = "701 702 7011 " ]; then rc=0; fi
  expect "exactly the failed runs whose release is not published, on a safe pipeline and live version, are re-run" "$rc"
  rc=1
  if [ "$(tr '\n' ' ' <"$tmp/calls.txt")" = "rerun 701 registered 701 rerun 702 registered 702 done 701 done 702 rerun 7011 registered 7011 done 7011 " ]; then rc=0; fi
  expect "re-runs go out in batches of 2, each registered before the next, each batch finished first" "$rc"
  has "SKIP r3-v0.1.0: the release has been published since"
  expect "a release sealed by someone else between batches is re-checked and not re-run" $?
  has "SEALED-PARTIAL r5-v0.1.0" && has "NO-RUN r6-v0.1.0" \
    && has "OK r4-v0.1.0" && has "UNKNOWN-STATE r7-v0.1.0" && has "UNKNOWN-STATE r8-v0.1.0" \
    && [ "$rrc" -ne 0 ]
  expect "sealed-but-partial, no run, unreadable state and a 403 on the lookup are all reported, never re-run" $?
  has "PROBE-REFUSED r9-v0.1.0" && has "CREATE-IS-DRAFT"
  expect "a tag whose commit carries a publish-first release.yml is refused by main's probe, never re-run" $?
  has "UNKNOWN-STATE r10-v0.1.0: run 7010 is for $bad, not the tagged commit $good"
  expect "a run that is not for the tagged commit is never re-run" $?
  has "PROBE-REFUSED r12-v0.1.0" && has "TAG-BURNED"
  expect "a version main has retired is never re-released through recovery" $?
  ! grep -q FORBIDDEN "$tmp/calls.txt" && [ "$(grep -c . "$tmp/origin.git/pushes.txt")" -eq "$pushes_before" ]
  expect "recovery never deletes a release and never pushes a tag" $?

  echo "the release-tag lock never outlives its holder:"
  lock_free() { # can a newcomer take the release-tag lock right now?
    "$PYTHON" - "$tmp/repo/.git/msp-skills-locks/release-tags.lock" <<'PY'
import fcntl, os, sys
fd = os.open(sys.argv[1], os.O_RDWR | os.O_CREAT)
try:
    fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
except OSError:
    sys.exit(1)
PY
  }
  connector r15 0.1.0
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "r15" -s
  git -C "$tmp/repo" push -q origin main
  git -C "$tmp/repo" push -q origin "HEAD:refs/tags/r15-v0.1.0"
  printf '{"isDraft": true, "assets": []}' >"$tmp/releases/r15-v0.1.0.json"
  echo "7015 completed failure $(git -C "$tmp/repo" rev-parse HEAD)" >"$tmp/releases/r15-v0.1.0.runinfo"
  printf 'r15-v0.1.0\n' >"$tmp/tags-r15.txt"
  : >"$tmp/never-register"
  out=$(CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" --rerun-failed "$tmp/tags-r15.txt" 2>&1) && rc=0 || rc=$?
  rm -f "$tmp/never-register"
  [ "$rc" -ne 0 ] && has "RERUN-NOT-SEEN r15-v0.1.0 7015" && lock_free
  expect "a re-run whose new attempt never registers stops the run, and the lock is free afterwards" $?
  rm -f "$tmp/rerun/7015" "$tmp/reg-7015" "$tmp/runs/rr-7015"
  : >"$tmp/rerun-lost-response"
  out=$(CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" --rerun-failed "$tmp/tags-r15.txt" 2>&1) && rc=0 || rc=$?
  rm -f "$tmp/rerun-lost-response"
  has "RERUN r15-v0.1.0 7015 (attempt 2; the request errored but GitHub registered it)" && has "DONE r15-v0.1.0 7015"
  expect "a re-run whose response is lost but which GitHub accepted waits for its registration, then goes on" $?
  rm -f "$tmp/rerun/7015" "$tmp/reg-7015" "$tmp/runs/rr-7015"
  : >"$tmp/rerun-refused"
  out=$(CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" --rerun-failed "$tmp/tags-r15.txt" 2>&1) && rc=0 || rc=$?
  rm -f "$tmp/rerun-refused"
  [ "$rc" -ne 0 ] && has "RERUN-REFUSED r15-v0.1.0 7015" && ! has "DONE r15-v0.1.0" && lock_free
  expect "a refused re-run with no new attempt stops the run (no guess), and the lock is free" $?
  # A failing pre-push hook that leaves a child running (holding whatever
  # descriptors it inherited): the lock must still be free once cut_tags exits.
  connector zz-f1 0.1.0
  printf '# Changelog\n\n## [0.1.0] - 2026-10-22\n\n### Added\n- First release.\n' >"$tmp/repo/skills/zz-f1/CHANGELOG.md"
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "zz-f1" -s
  git -C "$tmp/repo" push -q origin main
  cat >"$tmp/repo/.git/hooks/pre-push" <<'HOOK'
#!/bin/sh
sleep 30 </dev/null >/dev/null 2>&1 &
echo $! >"${CT_STATE:?}/hook-child.pid"
exit 1
HOOK
  chmod +x "$tmp/repo/.git/hooks/pre-push"
  rm -f "$tmp"/runs/*
  printf 'zz-f1-v0.1.0\n' >"$tmp/tags-f1.txt"
  out=$(CUT_TAGS_FLIGHT_SLEEP=0 bash "$SELF" "$(git -C "$tmp/repo" rev-parse HEAD)" "$tmp/tags-f1.txt" 2>&1) && rc=0 || rc=$?
  hook_child=$(cat "$tmp/hook-child.pid" 2>/dev/null)
  rc_free=1
  if [ "$rc" -ne 0 ] && [ -n "$hook_child" ] && kill -0 "$hook_child" 2>/dev/null && lock_free; then rc_free=0; fi
  [ -n "$hook_child" ] && kill "$hook_child" 2>/dev/null
  rm -f "$tmp/repo/.git/hooks/pre-push"
  expect "a hook's leftover child does not keep the lock after cut_tags exits (it is held by a separate holder)" "$rc_free"

  echo "stale copies of this script:"
  : >"$tmp/drafts.txt"
  mkdir -p "$tmp/repo/tools/maintainer"
  printf '#!/usr/bin/env bash\necho a newer cut_tags\n' >"$tmp/repo/tools/maintainer/cut_tags.sh"
  git -C "$tmp/repo" add -A
  git -C "$tmp/repo" commit -q -m "main carries a newer cut_tags.sh" -s
  git -C "$tmp/repo" push -q origin main
  out=$(bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  [ "$rc" -ne 0 ] && has "differs from origin/main"
  expect "a copy that differs from origin/main's refuses to run" $?
  out=$(CUT_TAGS_ALLOW_STALE=1 bash "$SELF" --receipts "$tmp/r-ok.txt" 2>&1) && rc=0 || rc=$?
  has "WARN this cut_tags.sh differs"
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
      --rerun-failed) mode=rerun ;;
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
  if [ "$mode" = receipts ] || [ "$mode" = rerun ]; then
    [ $# -eq 1 ] || die "usage: cut_tags.sh --receipts|--rerun-failed [--log FILE] <tags-file>"
    tags_file=$(abspath "$1")
  else
    [ $# -eq 2 ] || die "usage: cut_tags.sh [--dry-run] [--log FILE] <SHA> <tags-file>"
    tags_file=$(abspath "$2")
  fi
  load_tags "$tags_file"
  setup_repo
  if [ "$mode" = receipts ]; then
    receipts
  elif [ "$mode" = rerun ]; then
    rerun_failed "$dry"
  else
    cut "$1" "$dry"
  fi
}

main "$@"
