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

if [ -n "${BASE:-}" ] && [ -n "${HEAD:-}" ]; then
  range="${BASE}..${HEAD}"
else
  if git rev-parse --verify -q origin/main >/dev/null; then
    range="origin/main..HEAD"
  else
    echo "No range given and no origin/main; checking HEAD only."
    range="HEAD~1..HEAD 2>/dev/null || HEAD"
  fi
fi

fail=0
# List commit hashes in the range (newest first).
commits="$(git rev-list "${range}" 2>/dev/null || git rev-list HEAD)"
if [ -z "${commits}" ]; then
  echo "No commits in range ${range}; nothing to check."
  exit 0
fi

for c in ${commits}; do
  # Skip merge commits (two-or-more parents).
  if [ "$(git rev-list --parents -n 1 "$c" | wc -w)" -gt 2 ]; then
    continue
  fi
  if ! git log -1 --format='%B' "$c" | grep -qiE '^Signed-off-by: .+ <.+@.+>'; then
    subject="$(git log -1 --format='%h %s' "$c")"
    echo "  - missing DCO sign-off: ${subject}"
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "DCO check passed for range ${range}."
else
  echo "DCO check FAILED. Sign commits with 'git commit -s'." >&2
fi
exit "$fail"
