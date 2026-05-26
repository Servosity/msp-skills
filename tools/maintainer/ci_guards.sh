#!/usr/bin/env bash
# ci_guards.sh - repo hygiene gates that match the promises in CONTRIBUTING.md
# and the PR template. Run locally:  bash tools/ci_guards.sh
#
# Scope note: the human-authored surface is checked for style/personal-path
# rules. The vendored, machine-generated Go source under skills/*/cli/ is
# EXCLUDED from the em-dash and personal-path checks (it is generated and not the
# contribution surface), but secret scanning (gitleaks, separate CI step) still
# covers the whole tree.

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1

fail=0
note() { echo "  - $1"; }

# Files git tracks, minus the vendored Go module trees and binaries.
human_files() {
  git ls-files -- . \
    ':(exclude)skills/*/cli/**' \
    ':(exclude)*.png' ':(exclude)*.jpg' ':(exclude)*.gif' ':(exclude)*.pdf'
}

echo "==> 1. No unresolved owner placeholder token anywhere"
# Build the token at runtime so this script never contains the literal itself.
token="OWNER_$(printf 'PLACEHOLDER')"
if git ls-files | xargs grep -nI "$token" 2>/dev/null; then
  note "unresolved owner placeholder token found (replace with the real GitHub owner)"
  fail=1
fi

echo "==> 2. No em-dash in human-authored files"
# Build U+2014 at runtime so this guard never contains the literal it bans.
emdash=$(printf '\342\200\224')
if human_files | xargs grep -nI "$emdash" 2>/dev/null; then
  note "em-dash found; use ' - ', a colon, or parentheses (CONTRIBUTING style rule)"
  fail=1
fi

echo "==> 3. No personal filesystem paths in human-authored files"
# Match a real user dir (/Users/<letter> or /home/<letter>), not the documented
# placeholders /Users/<you> or the rule being referenced in docs.
if human_files | xargs grep -nIE '/Users/[A-Za-z]|/home/[a-z]+/' 2>/dev/null \
  | grep -vE '/Users/<|/home/<'; then
  note "personal filesystem path found in a human-authored file"
  fail=1
fi

echo "==> 4. No obvious hardcoded secrets in human-authored files"
# Coarse pre-check; gitleaks is the authoritative secret scan in CI.
if human_files | xargs grep -nIE '(ghp_[A-Za-z0-9]{20,}|xox[bap]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----)' 2>/dev/null; then
  note "possible hardcoded secret found"
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "All CI guards passed."
else
  echo "CI guards FAILED. Fix the items above." >&2
fi
exit "$fail"
