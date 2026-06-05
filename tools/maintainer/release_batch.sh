#!/usr/bin/env bash
# release_batch.sh - the release choreography, isolated from the shared checkout.
#
# Releases used to run release.py directly in the main checkout. That left the
# version/stamp edits sitting dirty in a working tree other agent sessions also
# use - and a broad `git add` from any of them could sweep the release stamp
# into an unrelated commit (it happened: de9b358). This script runs the whole
# batch in a throwaway worktree, stages ONLY the version-bearing files, pushes
# the release commit to main with a rebase-retry loop, and then PRINTS the tag
# commands pinned to the pushed SHA. It never tags and never pushes tags -
# cutting a release stays a deliberate, separate act.
#
# Usage:
#   tools/maintainer/release_batch.sh --all-pending
#   tools/maintainer/release_batch.sh --slug halopsa
#   tools/maintainer/release_batch.sh --slugs halopsa,servosity --bump minor
#   tools/maintainer/release_batch.sh --slug halopsa --dry-run   # rehearsal:
#       worktree + release.py --dry-run + staged-set preview, then teardown.
#
# Concurrency: release.py itself holds a machine-wide lock (repo_lock.py,
# anchored in the git common dir so it is shared across worktrees). This
# script adds the isolation layer; the lock adds the serialization layer.
#
# Derived files (catalog.json, README catalog table, docs/llms*.txt,
# docs/_data/pending.json) are deliberately NOT committed here - the catalog
# CI job regenerates and auto-commits them on both the main push and the tag
# push. Committing them locally is redundant at best and stale at worst.
#
# bash 3.2 compatible (macOS default shell) - no mapfile, no brace ranges.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$SCRIPT_DIR/../.." && pwd)"
WT="$(dirname "$REPO")/msp-skills-wt-release"
BRANCH="release/batch-$(date +%Y%m%d-%H%M%S)"

# The ONLY paths a release commit may contain (the de9b358 fix: narrow add,
# never `git add -A`).
RELEASE_PATHSPECS="skills/*/manifest.json skills/*/.claude-plugin/plugin.json skills/*/server.json skills/*/CHANGELOG.md .claude-plugin/marketplace.json tools/maintainer/skills.json"

DRY_RUN=0
for a in "$@"; do
  [ "$a" = "--dry-run" ] && DRY_RUN=1
done

if [ -e "$WT" ]; then
  echo "release_batch: worktree already exists: $WT" >&2
  echo "release_batch: one release batch at a time. If a prior batch crashed:" >&2
  echo "  git -C $REPO worktree remove --force $WT" >&2
  exit 1
fi

echo "== release_batch: fresh worktree off origin/main =="
git -C "$REPO" fetch origin main
git -C "$REPO" worktree add -b "$BRANCH" "$WT" origin/main

cleanup_worktree() {
  git -C "$REPO" worktree remove --force "$WT" 2>/dev/null || true
  git -C "$REPO" branch -D "$BRANCH" 2>/dev/null || true
}

echo "== release_batch: running release.py in the worktree =="
RELOUT="$(mktemp)"
if ! python3 "$WT/tools/maintainer/release.py" "$@" | tee "$RELOUT"; then
  echo "release_batch: release.py failed - tearing down" >&2
  rm -f "$RELOUT"
  cleanup_worktree
  exit 1
fi

# Tags release.py planned, e.g. "  TAG: git tag halopsa-v0.1.2 && git push ..."
TAGS=""
while IFS= read -r line; do
  tag="$(printf '%s\n' "$line" | sed -n 's/^  TAG: git tag \([^ ]*\) .*/\1/p')"
  [ -n "$tag" ] && TAGS="$TAGS $tag"
done < "$RELOUT"
rm -f "$RELOUT"
TAGS="${TAGS# }"

if [ -z "$TAGS" ]; then
  echo "release_batch: release.py planned no tags (nothing pending?) - tearing down"
  cleanup_worktree
  exit 0
fi

if [ "$DRY_RUN" = "1" ]; then
  echo "== release_batch: DRY-RUN rehearsal - what a real run would stage =="
  # shellcheck disable=SC2086
  git -C "$WT" add --dry-run $RELEASE_PATHSPECS || true
  echo "== release_batch: derived files (must stay UNSTAGED in a real run) =="
  git -C "$WT" status --short -- catalog.json README.md docs/llms.txt docs/llms-full.txt docs/_data/pending.json || true
  echo "== release_batch: dry-run complete - tearing down =="
  cleanup_worktree
  echo "DRY-RUN tags that would be cut:"
  for t in $TAGS; do echo "  $t"; done
  exit 0
fi

echo "== release_batch: committing version files (narrow add) =="
# shellcheck disable=SC2086
git -C "$WT" add $RELEASE_PATHSPECS
if git -C "$WT" diff --cached --quiet; then
  echo "release_batch: nothing to commit (already stamped?) - tearing down"
  cleanup_worktree
  exit 1
fi
git -C "$WT" commit -s -m "chore(release): $TAGS"

echo "== release_batch: pushing to main (rebase-retry) =="
PUSHED=0
for attempt in 1 2 3 4 5; do
  if git -C "$WT" push origin HEAD:main; then
    PUSHED=1
    break
  fi
  echo "release_batch: push rejected (attempt $attempt) - fetch + rebase + retry"
  git -C "$WT" fetch origin main
  if ! git -C "$WT" rebase origin/main; then
    git -C "$WT" rebase --abort || true
    echo "release_batch: rebase conflict - resolve manually in $WT, then:" >&2
    echo "  git -C $WT push origin HEAD:main" >&2
    echo "  (then run the tag commands release.py printed, pinned to the pushed SHA)" >&2
    exit 1
  fi
done
if [ "$PUSHED" != "1" ]; then
  echo "release_batch: push failed after 5 attempts - NOT tagging. Worktree kept at $WT" >&2
  exit 1
fi

# Pin the SHA that actually landed. Tagging 'main' instead would race any
# peer commit (e.g. the catalog bot) landing a second later.
RELSHA="$(git -C "$WT" rev-parse HEAD)"

echo "== release_batch: tearing down worktree =="
cleanup_worktree

echo
echo "============================================================"
echo "Release commit pushed to main: $RELSHA"
echo "Copy-paste to tag + push (this script never runs these):"
for t in $TAGS; do
  echo "  git -C $REPO tag $t $RELSHA && git -C $REPO push origin $t"
done
echo
echo "Each tag push fires release.yml (build) then mcp-publish.yml (registry)."
echo "N tags -> N independent builds; that parallelism is safe."
