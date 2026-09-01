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
# TAG-TIME SAFETY. A tag push runs .github/workflows/release.yml AS IT EXISTS AT
# THE TAGGED COMMIT, not as it exists on main. Under the repository's immutable
# releases, tagging a commit whose workflow still publishes before it uploads
# seals an EMPTY release and spends that version number permanently - twenty
# staged tags at such a commit is twenty burned versions. Prose in this epilogue
# cannot stop a paste, so the endorsement is mechanical:
# tools/maintainer/check_release_pipeline.py reads release.yml at the SHA in
# question and refuses it unless that commit assembles into a draft, gates the
# asset set, and seals last. This script runs it twice - once against
# origin/main before it stamps anything, and once against the pushed SHA before
# it prints a single tag line - and prints NO tag commands for a SHA it refuses.
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

PIPELINE_CHECK="$SCRIPT_DIR/check_release_pipeline.py"

# Refuse a SHA whose release.yml would run the old publish-first pipeline.
# $1 = commit-ish, $2 = what it is, for the message.
assert_taggable() {
  if [ ! -f "$PIPELINE_CHECK" ]; then
    echo "release_batch: $PIPELINE_CHECK is missing - cannot prove that tagging" >&2
    echo "  this commit runs the draft-then-seal release pipeline. Refusing." >&2
    exit 1
  fi
  if python3 "$PIPELINE_CHECK" --sha "$1" --repo "$REPO"; then
    return 0
  fi
  echo >&2
  echo "release_batch: REFUSING to release from $2 ($1)." >&2
  echo "  A tag push runs release.yml FROM THE TAGGED COMMIT. This one still" >&2
  echo "  publishes the release before its assets are uploaded, and the repository" >&2
  echo "  has immutable releases enabled: every tag cut here becomes a permanently" >&2
  echo "  sealed, empty release and a spent version number." >&2
  echo "  Land the draft-then-seal release pipeline on main first, then re-run this" >&2
  echo "  script so the release commit sits on top of it." >&2
  exit 1
}

if [ -e "$WT" ]; then
  echo "release_batch: worktree already exists: $WT" >&2
  echo "release_batch: one release batch at a time. If a prior batch crashed:" >&2
  echo "  git -C $REPO worktree remove --force $WT" >&2
  exit 1
fi

echo "== release_batch: fresh worktree off origin/main =="
git -C "$REPO" fetch origin main

# BEFORE anything is stamped. The release commit will land on top of
# origin/main, so if origin/main cannot be tagged safely, neither can the commit
# this script is about to build. Failing here costs nothing; failing after the
# stamp lands means a version bump on main with no releasable tag.
echo "== release_batch: origin/main must carry the draft-then-seal pipeline =="
assert_taggable "origin/main" "origin/main"

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

# The SHA the printed commands will name is the one that must be endorsed.
# origin/main passed above, but this commit is what a tag will actually run:
# a rebase in the retry loop could have landed it on a different main, and it
# is the SHA an operator will paste - possibly days later, out of a scrollback.
# Endorse THAT, or print no tag commands at all.
echo "== release_batch: the pushed SHA must carry the draft-then-seal pipeline =="
if ! python3 "$PIPELINE_CHECK" --sha "$RELSHA" --repo "$WT"; then
  echo >&2
  echo "release_batch: the release commit $RELSHA is PUSHED, but its release.yml" >&2
  echo "  does not assemble into a draft and seal last (see above). Tagging it would" >&2
  echo "  seal $(echo "$TAGS" | wc -w | tr -d ' ') empty, immutable releases and spend those version numbers" >&2
  echo "  permanently, so NO tag commands are printed." >&2
  echo >&2
  echo "  The tags that are staged but NOT endorsed:" >&2
  for t in $TAGS; do echo "    $t" >&2; done
  echo >&2
  echo "  Land the draft-then-seal release pipeline on main, then tag a commit" >&2
  echo "  that contains it. Verify any SHA before tagging it:" >&2
  echo "    python3 $PIPELINE_CHECK --sha <sha> --repo $REPO" >&2
  cleanup_worktree
  exit 1
fi

echo "== release_batch: tearing down worktree =="
cleanup_worktree

echo
echo "============================================================"
echo "Release commit pushed to main: $RELSHA"
echo "This SHA was checked: its release.yml assembles into a draft, gates the"
echo "asset set, and seals last, so these tags are safe to cut."
echo "Copy-paste to tag + push (this script never runs these):"
for t in $TAGS; do
  echo "  git -C $REPO tag $t $RELSHA && git -C $REPO push origin $t"
done
echo
echo "Each tag push fires release.yml AS IT EXISTS AT THE TAGGED COMMIT, which"
echo "assembles the release as a DRAFT (6 targets x 4 files, plus the .mcpb"
echo "bundle), asserts the set is complete, and only then publishes it."
echo "Publishing is what makes a release immutable, so it is the last step: a"
echo "failed target leaves a DRAFT you can re-run, never a public release with"
echo "missing assets. mcp-publish.yml then records the published .mcpb in the"
echo "MCP Registry."
echo
echo "STALE PRINTOUT? Tag commands from an EARLIER batch name an EARLIER SHA, and"
echo "a SHA from before the draft-then-seal pipeline landed would run the old"
echo "publish-first workflow and seal one empty, unrepairable release per tag."
echo "Before pasting any tag command you did not just generate, run:"
echo "  python3 $PIPELINE_CHECK --sha <the sha in that command> --repo $REPO"
echo
echo "WATCH FOR: a tag whose release never leaves draft state. That means a build"
echo "target or the bundle failed - re-run the Release workflow for that tag."
echo "N tags -> N independent builds; that parallelism is safe."
