#!/usr/bin/env bash
# ci_guards.sh - repo hygiene gates that match the promises in CONTRIBUTING.md
# and the PR template. Run locally:  bash tools/ci_guards.sh
#
# Scope note: the human-authored surface is checked for style/personal-path
# rules. The vendored, machine-generated Go source under skills/*/cli/ is
# EXCLUDED from the em-dash and personal-path checks (it is generated and not the
# contribution surface), but secret scanning (gitleaks, separate CI step) still
# covers the whole tree.
#
# A guard fails on its OUTPUT, never on grep's exit status. These used to read
# `if human_files | xargs grep ...; then fail=1`: when any listed file was
# missing from disk (a sparse checkout, or a plain `mv` of a tracked file such
# as the contribution flow's "move README.md to guide.md"), grep exited 2, so
# xargs exited non-zero and the guard printed the violation and then reported
# "All CI guards passed". The file list is now every tracked file that EXISTS
# plus every untracked, non-ignored file (so the moved file is scanned too),
# and a file grep cannot read fails the guard instead of being skipped.
#
#   bash tools/maintainer/ci_guards.sh             # this checkout
#   bash tools/maintainer/ci_guards.sh --self-test # proves the guards both ways

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
SELF="$(pwd)/tools/maintainer/ci_guards.sh"

if [ "${1:-}" = "--self-test" ]; then
  # Each case runs THIS script against a throwaway repository and checks the
  # exit status and the verdict line together.
  em=$(printf '\342\200\224')
  home="/Use""rs/someone"   # built at runtime: guard 3 must not fire on this file
  bad=0
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/ci_guards.selftest.XXXXXX")" || exit 2
  trap 'rm -rf "$tmp"' EXIT
  st_case() { # <label> <want: pass|fail> <setup command string>
    local label="$1" want="$2" setup="$3" d rc got
    d="$tmp/case$RANDOM$RANDOM"
    mkdir -p "$d/tools/maintainer" "$d/skills/acme/cli"
    cp "$SELF" "$d/tools/maintainer/ci_guards.sh"
    printf '# Acme\n\nPlain text.\n' > "$d/skills/acme/README.md"
    printf '# Other\n' > "$d/skills/acme/guide.md"
    printf 'module acme-pp-cli\n\ngo 1.26.9\n' > "$d/skills/acme/cli/go.mod"
    printf 'package main // %s generated code is exempt\n' "$em" > "$d/skills/acme/cli/main.go"
    ( cd "$d" && git init -q && git add -A \
      && git -c user.name=t -c user.email=t@example.com commit -q -m base ) || return 1
    ( cd "$d" && eval "$setup" )
    ( cd "$d" && bash tools/maintainer/ci_guards.sh ) >"$d.out" 2>&1
    rc=$?
    if [ "$rc" -eq 0 ] && grep -q 'All CI guards passed' "$d.out"; then got=pass
    elif [ "$rc" -ne 0 ] && grep -q 'CI guards FAILED' "$d.out"; then got=fail
    else got="inconsistent(rc=$rc)"; fi
    if [ "$got" = "$want" ]; then echo "  ok   $label -> $want"
    else echo "  FAIL $label -> want $want, got $got"; sed 's/^/       /' "$d.out"; bad=1; fi
  }
  st_case "clean tree" pass ":"
  st_case "em-dash in a tracked file" fail "printf 'a $em b\n' >> skills/acme/README.md"
  st_case "em-dash + another tracked file deleted (the F4 false GREEN)" fail \
    "printf 'a $em b\n' >> skills/acme/README.md; rm skills/acme/guide.md"
  st_case "plain mv of a tracked file, em-dash in the moved file" fail \
    "printf 'a $em b\n' >> skills/acme/README.md; mv skills/acme/README.md skills/acme/guide2.md"
  st_case "plain mv of a tracked file, no violation" pass "mv skills/acme/README.md skills/acme/guide2.md"
  st_case "an ignored scratch file is not scanned" pass \
    "printf 'scratch/\n' > .gitignore; git add .gitignore; mkdir scratch; printf 'a $em b\n' > scratch/x.md"
  st_case "a personal path + a missing file" fail \
    "printf 'see $home/x\n' >> skills/acme/README.md; rm skills/acme/guide.md"
  st_case "an em-dash in generated cli/ Go is exempt" pass ":"
  st_case "a go.mod below the toolchain floor + a missing file" fail \
    "printf 'module acme-pp-cli\n\ngo 1.26.1\n' > skills/acme/cli/go.mod; rm skills/acme/guide.md"
  st_case "an unreadable file fails closed" fail \
    "printf 'x\n' > skills/acme/locked.md; chmod 000 skills/acme/locked.md; [ -r skills/acme/locked.md ] && printf 'a $em b\n' > skills/acme/locked.md"
  if [ "$bad" -eq 0 ]; then echo "PASS: ci_guards self-test - every case discriminates"; exit 0; fi
  echo "FAIL: ci_guards self-test" >&2
  exit 1
fi

fail=0
# Explanations print as "hint:" so verify_all.sh can tell them from findings.
note() { echo "  hint: $1"; }

# Every file a commit could carry: tracked files that exist on disk, plus
# untracked files that are not ignored (a plainly moved file lands here).
# NUL-separated. A tracked file missing from disk is skipped, never fatal.
all_files() {
  { git ls-files -z --cached -- "$@"; git ls-files -z --others --exclude-standard -- "$@"; } \
    | python3 -c 'import os, sys
names = sys.stdin.buffer.read().split(b"\0")
sys.stdout.buffer.write(b"".join(n + b"\0" for n in names if n and os.path.isfile(n)))'
}

# The two lists, built once: ALL (every file) and HUMAN (minus the vendored Go
# module trees and binaries).
lists="$(mktemp -d "${TMPDIR:-/tmp}/ci_guards.XXXXXX")" || exit 2
trap 'rm -rf "$lists"' EXIT
all_files . > "$lists/all"
all_files . ':(exclude)skills/*/cli/**' \
  ':(exclude)*.png' ':(exclude)*.jpg' ':(exclude)*.gif' ':(exclude)*.pdf' > "$lists/human"

# scan <all|human> <grep args...>: print every match (file:line:text) and
# succeed iff there is at least one. A file grep cannot read is reported and
# counts as a match, so an I/O error can never read as "clean".
scan() {
  local list="$lists/$1" out err="$lists/grep.err"
  shift
  out="$(xargs -0 grep -nIH "$@" -- < "$list" 2>"$err")"
  if [ -s "$err" ]; then
    echo "  could not scan every file (treated as a failure):"
    sed 's/^/    /' "$err"
    [ -n "$out" ] && printf '%s\n' "$out"
    return 0
  fi
  [ -n "$out" ] || return 1
  printf '%s\n' "$out"
}

echo "==> 1. No unresolved owner placeholder token anywhere"
# Build the token at runtime so this script never contains the literal itself.
token="OWNER_$(printf 'PLACEHOLDER')"
if scan all -e "$token"; then
  note "unresolved owner placeholder token found (replace with the real GitHub owner)"
  fail=1
fi

echo "==> 2. No em-dash in human-authored files"
# Build U+2014 at runtime so this guard never contains the literal it bans.
emdash=$(printf '\342\200\224')
if scan human -e "$emdash"; then
  note "em-dash found; use ' - ', a colon, or parentheses (CONTRIBUTING style rule)"
  fail=1
fi

echo "==> 3. No personal filesystem paths in human-authored files"
# Match a real user dir (/Users/<letter> or /home/<letter>), not the documented
# placeholders /Users/<you> or the rule being referenced in docs.
personal="$(scan human -E -e '/Users/[A-Za-z]|/home/[a-z]+/' | grep -vE '/Users/<|/home/<')"
if [ -n "$personal" ]; then
  printf '%s\n' "$personal"
  note "personal filesystem path found in a human-authored file"
  fail=1
fi

echo "==> 4. No obvious hardcoded secrets in human-authored files"
# Coarse pre-check; gitleaks is the authoritative secret scan in CI.
if scan human -E -e '(ghp_[A-Za-z0-9]{20,}|xox[bap]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----)'; then
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
if scan human -E -e "$counts"; then
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
# is not 58 entries in 58 handfixes.json files. The press emits go1.26.6 (4.30.2
# onward), BELOW this floor, so a raw reprint trips it; msp-skills-publish's
# onboard.py raises go.mod to GO_TOOLCHAIN (1.26.9, #358) and the fleet moved in
# #370.
#
# Raising the floor is a deliberate act: bump GO_FLOOR here in the same PR that
# sweeps go.mod, so the guard and the fleet move together.
GO_FLOOR="1.26.10"
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
done < <(all_files 'skills/*/cli/go.mod' | tr '\0' '\n')
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
