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

echo "==> 5. No real production-tenant transaction counts in human-authored files"
# A contributor dogfooding against their own QuickBooks/MSP tenant can leak the
# tenant's real record counts (confidential business scale) into a CHANGELOG or
# hand-fix note. Build the banned count literals from digit fragments + a runtime
# comma so this guard never contains the strings it bans (same trick as guards
# 1-2). Use a generic phrasing ("the full book") instead of real totals.
cma=$(printf ',')
# Word-boundary anchored so the counts only match as standalone numbers in
# prose, not as an incidental digit run inside a longer token (e.g. one of
# the banned counts surfacing by chance inside the hex of a sha256 hash in a
# generated data file).
counts="\\b(44${cma}?211|17${cma}?940|16${cma}?863)\\b"
if human_files | xargs grep -nIE "$counts" 2>/dev/null; then
  note "real production-tenant transaction count found; replace with generic phrasing (e.g. 'the full book')"
  fail=1
fi

echo "==> 6. No connector selects a Go toolchain below the fleet floor"
# The floor exists because a stdlib advisory is fixed by the toolchain that
# BUILDS the binary, and CI cannot see a regression here: the workflows request
# `go-version: "1.26"`, which resolves to the latest patched Go, so the security
# gate scans a patched toolchain while `build` honours whatever `go.mod` pins.
# A green gate is not evidence the shipped binary is patched (this is how
# GO-2026-6218 rode the whole fleet; see #210).
#
# A reprint regenerates `go.mod`, so the floor is exactly the kind of edit that
# gets silently reverted. One guard covers all 58 connectors, which is why this
# is not 58 entries in 58 handfixes.json files: the press emits go1.26.6 from
# 4.30.2 onward, so a reprint at the current press satisfies this by
# construction and only a reprint at an OLDER press can trip it.
#
# Raising the floor is a deliberate act: bump GO_FLOOR here in the same PR that
# sweeps go.mod, so the guard and the fleet move together.
GO_FLOOR="1.26.6"
floor_bad=0
while IFS= read -r gomod; do
  # The effective toolchain is the `toolchain` line when present, else the `go`
  # directive. A bare `go 1.26` (no patch) defers to the toolchain line.
  sel=$(awk '/^toolchain go/ {print substr($2,3); found=1} END {if (!found) exit 1}' "$gomod" 2>/dev/null) \
    || sel=$(awk '/^go / {print $2}' "$gomod" 2>/dev/null)
  # Sort -V puts the lower version first; if that is not the floor, sel is below it.
  if [ "$(printf '%s\n%s\n' "$GO_FLOOR" "$sel" | sort -V | head -1)" != "$GO_FLOOR" ]; then
    echo "$gomod selects go$sel, below the go$GO_FLOOR floor"
    floor_bad=1
  fi
done < <(git ls-files 'skills/*/cli/go.mod')
if [ "$floor_bad" -ne 0 ]; then
  note "a connector selects a Go toolchain below go$GO_FLOOR; bump it, or raise GO_FLOOR here if the floor itself is moving"
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "All CI guards passed."
else
  echo "CI guards FAILED. Fix the items above." >&2
fi
exit "$fail"
