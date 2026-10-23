#!/usr/bin/env bash
# ci_guards.sh - repo hygiene gates that match the promises in CONTRIBUTING.md
# and the PR template. Run locally:  bash tools/ci_guards.sh
#
# Scope note: the human-authored surface is checked for style/personal-path
# rules. The vendored, machine-generated Go source under skills/*/cli/ and the
# press's own metadata under skills/*/press/ (vendor specs: byte-exact, bound by
# a checksum, so never re-styled) are EXCLUDED from the human-authored checks
# (generated, not the contribution surface). check_press_metadata.py enforces
# the personal-path and secret rules on press/ instead, and secret scanning
# (gitleaks, separate CI step) still covers the whole tree.
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
  # The sample connector selects exactly the floor guard 6 enforces, read from
  # this script's own GO_FLOOR line, so raising the floor never breaks the
  # clean case (and the below-floor case stays below it).
  floor="$(sed -n 's/^GO_FLOOR="\([0-9.]*\)"$/\1/p' "$SELF")"
  [ -n "$floor" ] || { echo "FAIL: ci_guards self-test cannot read GO_FLOOR from $SELF" >&2; exit 1; }
  bad=0
  # Guard 7 checks .coderabbit.yaml against its generator, so every case
  # repository carries both (generated before its first commit).
  CR_SRC="$(dirname "$SELF")/gen_coderabbit_config.py"
  [ -f "$CR_SRC" ] || { echo "FAIL: ci_guards self-test needs $CR_SRC" >&2; exit 1; }
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/ci_guards.selftest.XXXXXX")" || exit 2
  trap 'chmod -R u+rwx "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
  st_case() { # <label> <want: pass|fail> <setup command string>
    local label="$1" want="$2" setup="$3" d rc got
    d="$tmp/case$RANDOM$RANDOM"
    mkdir -p "$d/tools/maintainer" "$d/skills/acme/cli"
    cp "$SELF" "$d/tools/maintainer/ci_guards.sh"
    printf '# Acme\n\nPlain text.\n' > "$d/skills/acme/README.md"
    printf '# Other\n' > "$d/skills/acme/guide.md"
    printf 'module acme-pp-cli\n\ngo %s\n' "$floor" > "$d/skills/acme/cli/go.mod"
    printf 'package main // %s generated code is exempt\n' "$em" > "$d/skills/acme/cli/main.go"
    cp "$CR_SRC" "$d/tools/maintainer/gen_coderabbit_config.py"
    ( cd "$d" && git init -q && python3 -I tools/maintainer/gen_coderabbit_config.py >/dev/null && git add -A \
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
  st_case "a tracked file moved to a file named '-', em-dash in it" fail \
    "printf 'a $em b\n' >> skills/acme/README.md; mv skills/acme/README.md ./-"
  st_case "a file named '-' with no violation" pass "printf 'plain\n' > ./-"
  st_case "an ignored scratch file is not scanned" pass \
    "printf 'scratch/\n' > .gitignore; git add .gitignore; mkdir scratch; printf 'a $em b\n' > scratch/x.md"
  st_case "a personal path + a missing file" fail \
    "printf 'see $home/x\n' >> skills/acme/README.md; rm skills/acme/guide.md"
  st_case "an em-dash in generated cli/ Go is exempt" pass ":"
  st_case "an em-dash in press/ vendor metadata is exempt" pass \
    "mkdir -p skills/acme/press; printf '{\"d\": \"a $em b\"}\n' > skills/acme/press/spec.json"
  st_case "an em-dash in a press-named file outside press/ is not" fail \
    "printf 'a $em b\n' > skills/acme/press-notes.md"
  st_case "a go.mod below the toolchain floor + a missing file" fail \
    "printf 'module acme-pp-cli\n\ngo 1.1.0\n' > skills/acme/cli/go.mod; rm skills/acme/guide.md"
  st_case "a go.mod at the floor, selected by a toolchain line" pass \
    "printf 'module acme-pp-cli\n\ngo 1.1\n\ntoolchain go%s\n' \"\$floor\" > skills/acme/cli/go.mod"
  # Guard 7: the CodeRabbit scope matches its generator, both ways. The
  # cases copy the real generator next to this script.
  # The generator's own rules (what it keeps, what it refuses), proved on
  # its throwaway repo; CI runs this self-test, so it runs there too.
  if python3 -I "$CR_SRC" --self-test >"$tmp/cr-selftest.out" 2>&1; then
    echo "  ok   gen_coderabbit_config.py --self-test"
  else
    echo "  FAIL gen_coderabbit_config.py --self-test"; sed 's/^/       /' "$tmp/cr-selftest.out"; bad=1
  fi
  gen_go="printf '// Generated by CLI Printing Press (https://github.com/mvanhorn/cli-printing-press). DO NOT EDIT.\n\npackage cli\n' > skills/acme/cli/gen.go"
  regen="python3 -I tools/maintainer/gen_coderabbit_config.py >/dev/null"
  st_case "a fresh .coderabbit.yaml with a generated file excluded" pass "$gen_go && $regen"
  st_case "a generated file added without regenerating" fail "$gen_go"
  st_case "a hand-edited .coderabbit.yaml" fail \
    "$gen_go && $regen && printf '    - \"!skills/acme/**\"\n' >> .coderabbit.yaml"
  st_case "a stale .coderabbit.yaml (the excluded file gained a hand-fix)" fail \
    "$gen_go && $regen && printf '{\"handfixes\": [{\"asserts\": [{\"file\": \"cli/gen.go\", \"contains\": \"x\"}]}]}\n' > skills/acme/handfixes.json"
  st_case "a .coderabbit.yaml without its generator" fail "rm tools/maintainer/gen_coderabbit_config.py"
  st_case "the generator and .coderabbit.yaml both removed" fail \
    "rm tools/maintainer/gen_coderabbit_config.py .coderabbit.yaml"
  # Guard 10: every connector's funnel copy matches the canonical source.
  SF_SRC="$(dirname "$SELF")/apply_secret_funnel.py"
  SF_CANON="$(dirname "$SELF")/secret_funnel"
  [ -f "$SF_SRC" ] && [ -d "$SF_CANON" ] || { echo "FAIL: ci_guards self-test needs $SF_SRC and $SF_CANON" >&2; exit 1; }
  if python3 -I "$SF_SRC" --self-test >"$tmp/sf-selftest.out" 2>&1; then
    echo "  ok   apply_secret_funnel.py --self-test"
  else
    echo "  FAIL apply_secret_funnel.py --self-test"; sed 's/^/       /' "$tmp/sf-selftest.out"; bad=1
  fi
  sf_install="cp \"$SF_SRC\" \"$(dirname "$SELF")/check_handfixes.py\" tools/maintainer/ && cp -R \"$SF_CANON\" tools/maintainer/"
  sf_carrier="mkdir -p skills/acme/cli/internal/cliutil skills/acme/cli/internal/cli skills/acme/cli/cmd/acme-mcp \
    && printf 'package main\n\nfunc main() { _ = server.NewMCPServer(\n\t\"acme\",\n\tversion,\n) }\n' > skills/acme/cli/cmd/acme-mcp/main.go \
    && for pair in cliutil_secret_funnel.go.tmpl:internal/cliutil/secret_funnel.go cliutil_secret_funnel_test.go.tmpl:internal/cliutil/secret_funnel_test.go \
         cli_secret_funnel.go.tmpl:internal/cli/secret_funnel_cli.go cli_secret_funnel_test.go.tmpl:internal/cli/secret_funnel_cli_test.go \
         mcp_secret_funnel.go.tmpl:cmd/acme-mcp/secret_funnel_mcp.go mcp_secret_funnel_test.go.tmpl:cmd/acme-mcp/secret_funnel_mcp_test.go; do \
         sed 's#MODULE#acme-pp-cli#' \"tools/maintainer/secret_funnel/\${pair%%:*}\" > \"skills/acme/cli/\${pair#*:}\"; done"
  st_case "a connector carrying the canonical funnel copies" pass "$sf_install && $sf_carrier"
  st_case "a connector whose funnel copy was edited by hand" fail \
    "$sf_install && $sf_carrier && printf '// local edit\n' >> skills/acme/cli/internal/cliutil/secret_funnel.go"
  st_case "a carrier missing one backstop file" fail \
    "$sf_install && $sf_carrier && rm skills/acme/cli/cmd/acme-mcp/secret_funnel_mcp.go"
  st_case "a carrier with the funnel tool removed" fail "$sf_install && $sf_carrier && rm tools/maintainer/apply_secret_funnel.py"
  # Guards 8 and 9 read pages and manifests the way a reader and a JSON
  # parser do (Codex review, round 1). The phrase is assembled from two halves
  # so this script never carries it.
  ppc="Printing Press"" CLI"
  ppl="printing press"" cli"
  st_case "the press title on a skill page" fail \
    "printf -- '---\nname: acme\n---\n\n# Acme  -  $ppc\n' > skills/acme/SKILL.md"
  st_case "the press title as a lower-case H2 in a guide" fail \
    "printf '## acme - $ppl\n' >> skills/acme/guide.md"
  st_case "the press title as an indented heading" fail \
    "printf '\n # Acme - $ppc\n' >> skills/acme/guide.md"
  st_case "the press title as a Setext heading" fail \
    "printf '\nAcme - $ppc\n=====\n' >> skills/acme/guide.md"
  st_case "the press title in bold, wrapped over two lines" fail \
    "printf 'Acme - **Printing** Press\nCLI\n' > skills/acme/SKILL.md"
  st_case "the press title in HTML, split by a tag and an entity" fail \
    "printf '<h1>Acme - Printing&#32;<b>Press</b> CLI</h1>\n' > skills/acme/page.md"
  st_case "the press title + a missing file" fail \
    "printf '# Acme - $ppc\n' > skills/acme/SKILL.md; rm skills/acme/guide.md"
  st_case "the press credited as CLI Printing Press, a client profile, a shell comment, cli/ and press/" pass \
    "printf 'Built with CLI Printing Press. Uses a Printing Press client profile.\n' >> skills/acme/README.md; printf '#!/bin/sh\n# $ppc\n' > skills/acme/x.sh; printf '# Acme - $ppc\n' > skills/acme/cli/README.md; mkdir -p skills/acme/press; printf '# $ppc\n' > skills/acme/press/notes.md"
  st_case "a manifest with a null press version (immybot's old shape)" fail \
    "printf '{\n  \"name\": \"acme-mcp\",\n  \"printing_press_version\": null\n}\n' > skills/acme/manifest.json"
  st_case "a null press version nested in press_provenance + a missing file" fail \
    "printf '{\"press_provenance\": {\"printing_press_version\":null}}\n' > skills/acme/manifest.json; rm skills/acme/guide.md"
  st_case "a null press version split across lines" fail \
    "printf '{\n  \"printing_press_version\":\n    null\n}\n' > skills/acme/manifest.json"
  st_case "a press version that is an escaped tab" fail \
    "printf '{\"printing_press_version\": \"\\\\t\"}\n' > skills/acme/manifest.json"
  st_case "a null press version hidden behind a duplicate key" fail \
    "printf '{\"printing_press_version\": null, \"printing_press_version\": \"4.24.0\"}\n' > skills/acme/manifest.json"
  st_case "a manifest that is not JSON" fail \
    "printf '{\"printing_press_version\": \n' > skills/acme/manifest.json"
  st_case "a manifest with a recorded press version, or none at all" pass \
    "printf '{\"printing_press_version\": \"4.24.0\"}\n' > skills/acme/manifest.json; mkdir -p skills/other; printf '{\"name\": \"other-mcp\"}\n' > skills/other/manifest.json"
  # Root (or a disk without permissions) reads a chmod 000 path anyway, so
  # these two cases cannot be set up there: skipped, and said so.
  probe="$(mktemp "${TMPDIR:-/tmp}/ci_guards_probe.XXXXXX")"; chmod 000 "$probe"
  if [ -r "$probe" ]; then
    echo "  skip  the unreadable-path cases (this user reads a chmod 000 file: root, or no permissions here)"
  else
    st_case "a tracked file in an unreadable directory fails closed" fail \
      "mkdir -p docs/locked; printf 'a $em b\n' > docs/locked/README.md; git add docs/locked/README.md; git -c user.name=t -c user.email=t@example.com commit -qm l; chmod 000 docs/locked"
    st_case "an unreadable file fails closed" fail \
      "printf 'x\n' > skills/acme/locked.md; chmod 000 skills/acme/locked.md"
  fi
  chmod 600 "$probe"; rm -f "$probe"
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
    2>>"${GIT_LIST_ERR:-/dev/null}" | python3 -c 'import os, stat, sys
out = []
for n in sys.stdin.buffer.read().split(b"\0"):
    if not n:
        continue
    try:
        st = os.stat(n)
    except (FileNotFoundError, NotADirectoryError):
        continue          # missing from disk (moved or deleted): nothing to scan
    except OSError:
        out.append(n)     # present but unreadable: keep it, grep reports it, the guard fails
        continue
    if stat.S_ISREG(st.st_mode):
        out.append(n)
sys.stdout.buffer.write(b"".join(b"./" + n + b"\0" for n in out))'
}
# Names go out as ./path: a file literally named "-" would otherwise reach grep
# as "read stdin" (even after --) and never be scanned. scan() strips the ./
# again so findings read as repo paths.

# The two lists, built once: ALL (every file) and HUMAN (minus the vendored Go
# module trees and binaries).
lists="$(mktemp -d "${TMPDIR:-/tmp}/ci_guards.XXXXXX")" || exit 2
trap 'rm -rf "$lists"' EXIT
GIT_LIST_ERR="$lists/git.err"
all_files . > "$lists/all"
all_files . ':(exclude)skills/*/cli/**' ':(exclude)skills/*/press/**' \
  ':(exclude)*.png' ':(exclude)*.jpg' ':(exclude)*.gif' ':(exclude)*.pdf' > "$lists/human"
if [ -s "$GIT_LIST_ERR" ]; then
  # git could not list part of the tree (an unreadable directory): what it
  # skipped was never scanned, so that is a failure, not a pass.
  echo "==> 0. Every file could be listed"
  sed 's/^/  /' "$GIT_LIST_ERR"
  note "git could not list part of the tree; fix the permissions so every file can be scanned"
  fail=1
fi

# scan <all|human> <grep args...>: print every match (file:line:text) and
# succeed iff there is at least one. A file grep cannot read is reported and
# counts as a match, so an I/O error can never read as "clean".
scan() {
  local list="$lists/$1" out err="$lists/grep.err"
  shift
  out="$(xargs -0 grep -nIH "$@" -- < "$list" 2>"$err" | sed 's#^\./##')"
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
    echo "${gomod#./} selects go$sel, below the go$GO_FLOOR floor"
    floor_bad=1
  fi
done < <(all_files 'skills/*/cli/go.mod' | tr '\0' '\n')
if [ "$floor_bad" -ne 0 ]; then
  note "a connector selects a Go toolchain below go$GO_FLOOR; bump it, or raise GO_FLOOR here if the floor itself is moving"
  fail=1
fi

echo "==> 7. .coderabbit.yaml is what its generator writes for this tree"
# CodeRabbit's review scope is derived from the tree (DO-NOT-EDIT headers and
# each connector's handfixes.json), so a hand edit, or a new hand-fix or
# hand-written file under an existing pattern, must not leave a stale scope
# on main that hides that file from review. The generator refuses to write a
# pattern that could match a file that must stay in review.
cr_gen="tools/maintainer/gen_coderabbit_config.py"
if [ -f "$cr_gen" ]; then
  if ! cr_out="$(python3 -I "$cr_gen" --check 2>&1)"; then
    printf '%s\n' "$cr_out"
    note "CodeRabbit's review scope is derived from the tree: regenerate it with the fix line above and commit it"
    fail=1
  fi
else
  # Removing the generator (with or without the file) must not turn this
  # guard into a silent pass.
  echo "$cr_gen is missing, so .coderabbit.yaml cannot be checked against the tree"
  note "restore the generator (and regenerate .coderabbit.yaml)"
  fail=1
fi

# pyscan <list> <program>: run a Python program over every file in a list.
# Any line it prints is a finding, and so is a failure of the scan itself:
# what could not be read was not checked. Guards 8 and 9 read pages and
# manifests the way a reader and a JSON parser do, which grep cannot.
pyscan() {
  local list="$lists/$1" out rc
  out="$(xargs -0 python3 -c "$2" < "$list" 2>&1)"
  rc=$?
  if [ "$rc" -ne 0 ]; then
    out="${out:+$out
}  the scan itself failed (exit $rc), which is not a pass"
  fi
  [ -n "$out" ] || return 1
  printf '%s\n' "$out" | sed 's#^\./##'
}

echo "==> 8. No press title on a skill page"
# cli-printing-press titles every SKILL.md "# <Vendor>  -  <press name> CLI".
# This repo ships Claude Code Skills; the press is credited as "CLI Printing
# Press" in each connector's cli/NOTICE and page footer, not as the page's
# title (decision 9 of the 2026-10-21 publish plan). 62 SKILL.md files still
# carried that title in October 2026 because nothing checked it, and
# onboard.py --resume re-vendors SKILL.md from the press source, so a reprint
# brings it straight back. The phrase is banned anywhere in a rendered page
# (Markdown or HTML, outside generated cli/ and press/), read as it renders:
# entities decoded, escapes, tags and emphasis dropped, line breaks folded.
# That covers every heading form (ATX, indented, Setext, HTML) without
# parsing headings; no page uses the phrase in prose.
guard7_py='import html, re, sys
esc = re.compile(r"\\([!-/:-@\[-`{-~])")
rx = re.compile(r"printing\s+press\s+cli(?![a-z0-9])", re.I)
for f in sys.argv[1:]:
    try:
        raw = open(f, encoding="utf-8").read()
    except (OSError, UnicodeDecodeError) as e:
        print(f"{f}: could not be read ({e.__class__.__name__})")
        continue
    t = re.sub(r"[*_`~]", "", re.sub(r"<[^>]*>", " ", esc.sub(r"\1", html.unescape(raw))))
    for m in rx.finditer(t):
        print(f"{f}:{t.count(chr(10), 0, m.start()) + 1}: " + " ".join(m.group(0).split()))'
all_files '*.md' '*.markdown' '*.mdx' '*.html' '*.htm' \
  ':(exclude)skills/*/cli/**' ':(exclude)skills/*/press/**' > "$lists/pages"
if pyscan pages "$guard7_py"; then
  note "a skill page is titled after the press; title it '<Vendor> - Claude Code Skill' (the press stays credited as CLI Printing Press in cli/NOTICE)"
  fail=1
fi

echo "==> 9. No manifest records a null press version"
# An earlier onboard.py wrote "printing_press_version": null (and a
# press_provenance block of nulls) whenever the source had no
# .printing-press.json, and on every --docs-only run; immybot shipped that
# shape until 2026-10-23. A null reads as provenance while recording none.
# Unknown is written by leaving both keys out, which is what the hardened
# onboard.py does; a known version is a string. Never guess one to pass this.
# Parsed as JSON, every key at every depth, duplicates included, so neither
# layout nor an escape can hide a null; a manifest that is not JSON fails.
guard8_py='import json, sys
class Obj(list):
    pass
def walk(o, path):
    if isinstance(o, Obj):
        for k, v in o:
            p = f"{path}.{k}" if path else k
            if k == "printing_press_version" and not (isinstance(v, str) and v.strip()):
                print(f"{f}: {p} is {json.dumps(v)}")
            walk(v, p)
    elif isinstance(o, list):
        for i, x in enumerate(o):
            walk(x, f"{path}[{i}]")
for f in sys.argv[1:]:
    try:
        doc = json.load(open(f, encoding="utf-8"), object_pairs_hook=Obj)
    except (OSError, ValueError) as e:
        print(f"{f}: not readable JSON ({e.__class__.__name__}), so its press version cannot be checked")
        continue
    walk(doc, "")'
all_files 'skills/*/manifest.json' > "$lists/manifests"
if pyscan manifests "$guard8_py"; then
  note "drop a null printing_press_version (and a null-filled press_provenance) instead of guessing; a reprint or onboard records the real one"
  fail=1
fi

echo "==> 10. Every connector's secret-echo funnel matches its canonical source"
# The shared secret-echo funnel (#444) ships byte-identical in every connector
# that carries it: the decoder in internal/cliutil and the CLI and MCP error
# backstops, those two with only the Go module path substituted. A copy edited
# in one connector drifts silently and the class test cannot see it, so every
# carrier is compared with tools/maintainer/secret_funnel/, the one copy of
# record that apply_secret_funnel.py installs from.
sf_tool="tools/maintainer/apply_secret_funnel.py"
if [ -f "$sf_tool" ]; then
  if ! sf_out="$(python3 -I "$sf_tool" --check-drift 2>&1)"; then
    printf '%s\n' "$sf_out"
    note "change the funnel in tools/maintainer/secret_funnel/ only, then re-run apply_secret_funnel.py --slug <slug> for each connector"
    fail=1
  fi
elif compgen -G 'skills/*/cli/internal/cliutil/secret_funnel.go' > /dev/null; then
  # Removing the tool must not turn this guard into a silent pass.
  echo "$sf_tool is missing while connectors carry the funnel, so their copies cannot be checked"
  note "restore apply_secret_funnel.py and tools/maintainer/secret_funnel/"
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "All CI guards passed."
else
  echo "CI guards FAILED. Fix the items above." >&2
fi
exit "$fail"
