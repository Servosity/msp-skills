#!/usr/bin/env bash
# check_dco.sh - verify every commit in a range carries a DCO Signed-off-by line.
#
# In CI (pull_request), pass the range via env:
#   BASE=<base sha> HEAD=<head sha> bash tools/check_dco.sh
# Locally with no env, it checks commits on this branch not on origin/main.
#
# DCO: https://developercertificate.org - `git commit -s` appends the trailer.

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1

# Check only the commits THIS branch introduces - never one already on main.
# A contributor who rebases onto a newer main pulls in our own bot commits
# ("chore: regenerate derived files [bot]"), which carry no sign-off. Failing
# someone for commits they did not write is a wall, not a gate: excluding
# origin/main as well as the base sha is what keeps the check about their work.
excludes=()
if git rev-parse --verify -q origin/main >/dev/null; then
  excludes+=(origin/main)
fi

if [ -n "${BASE:-}" ] && [ -n "${HEAD:-}" ]; then
  tip="${HEAD}"
  excludes+=("${BASE}")
else
  tip="HEAD"
  if [ ${#excludes[@]} -eq 0 ]; then
    echo "No range given and no origin/main; nothing to check."
    exit 0
  fi
fi

fail=0
# Commit hashes reachable from the tip but not from main or the base (newest first).
commits="$(git rev-list "${tip}" --not "${excludes[@]}" 2>/dev/null)"
if [ -z "${commits}" ]; then
  echo "No new commits to check (nothing here that is not already on main)."
  exit 0
fi

# Our own CI bot. catalog.yml / live-verified.yml auto-commit regenerated derived
# files, and those workflows also run on a CONTRIBUTOR'S FORK - so their branch can
# carry bot commits they never wrote. Both workflows now sign off (`git commit -s`),
# but branches opened before that fix still carry unsigned ones. DCO is a licensing
# assertion about human contributions, not a security control, so exempting the bot
# costs nothing and stops us failing people for our own commits.
BOT_EMAIL="41898282+github-actions[bot]@users.noreply.github.com"

for c in ${commits}; do
  # Skip merge commits (two-or-more parents).
  if [ "$(git rev-list --parents -n 1 "$c" | wc -w)" -gt 2 ]; then
    continue
  fi
  if [ "$(git log -1 --format='%ae' "$c")" = "${BOT_EMAIL}" ]; then
    continue
  fi
  if ! git log -1 --format='%B' "$c" | grep -qiE '^Signed-off-by: .+ <.+@.+>'; then
    subject="$(git log -1 --format='%h %s' "$c")"
    echo "  - missing DCO sign-off: ${subject}"
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "DCO check passed."
else
  {
    echo ""
    echo "Sign-off missing on the commits listed above."
    echo ""
    echo "A sign-off is one line git adds for you. It says the code is yours to give."
    echo "For your next commit:   git commit -s -m \"your message\""
    echo "To fix the ones above:  git rebase --signoff origin/main   (then git push --force-with-lease)"
    echo ""
    echo "That is all this check wants. Nothing else about your contribution is in question."
  } >&2
fi
exit "$fail"
