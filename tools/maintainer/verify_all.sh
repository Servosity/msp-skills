#!/usr/bin/env bash
# verify_all.sh - one command, one verdict. Runs every gate that protects the
# monorepo so an onboarding (or any change) cannot slip a regression past.
#
# Hard gates (must pass): go build+vet per module, skill contract, repo guards,
# markdown links, release contract, catalog idempotency, release matrix sanity,
# install dry-run resolves cleanly.
# Best-effort gates (warn if the tool is absent locally; CI installs them):
# shell linting, secret scanning, workflow linting, and plugin manifest
# validation (`claude plugin validate --strict` for the marketplace + each skill;
# hard-fails when the claude CLI is present).
#
# Run locally:  bash tools/maintainer/verify_all.sh
# Exit code is non-zero if any hard gate fails.

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1

hard_fail=0
pass() { printf '  PASS  %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; hard_fail=1; }
warn() { printf '  WARN  %s\n' "$1"; }
run()  { if "$@" >/tmp/va.out 2>&1; then return 0; else cat /tmp/va.out; return 1; fi; }

echo "== Go build + vet (per vendored module) =="
for mod in skills/*/cli; do
  [ -f "$mod/go.mod" ] || continue
  if ( cd "$mod" && go build ./... && go vet ./... ) >/tmp/va.out 2>&1; then
    pass "$mod build+vet"
  else
    cat /tmp/va.out; fail "$mod build+vet"
  fi
done

echo "== CLI claims vs built binary =="
# Builds each CLI and checks that every command/flag the docs claim exists in the
# real binary surface. WARN mode for now: it prints findings but does not gate,
# while the fleet calibrates. NOTE(calibration): drop --warn after fleet calibration.
if run python3 tools/maintainer/check_cli_claims.py --warn; then pass "CLI claims (warn)"; else warn "CLI claims reported findings"; fi

echo "== Repo gates =="
if run python3 tools/maintainer/check_skill_contract.py;  then pass "skill contract";      else fail "skill contract";      fi
if run python3 tools/maintainer/check_md_links.py;        then pass "markdown links";      else fail "markdown links";      fi
if run python3 tools/maintainer/check_release_contract.py; then pass "release contract";   else fail "release contract";    fi
if run python3 tools/maintainer/check_social_assets.py;   then pass "social assets";       else fail "social assets";       fi
if run python3 tools/maintainer/check_no_todos.py;        then pass "no TODO markers";     else fail "no TODO markers";     fi
if run python3 tools/maintainer/check_vocabulary.py;      then pass "vocabulary contract"; else fail "vocabulary contract"; fi
if run python3 tools/maintainer/check_video_assets.py;    then pass "video assets";        else fail "video assets";        fi
if run python3 tools/maintainer/check_aeo.py;             then pass "AEO answer-first";    else fail "AEO answer-first";    fi
if run python3 tools/maintainer/check_surface_coverage.py; then pass "surface coverage";   else fail "surface coverage";    fi
if run python3 tools/maintainer/check_media_block.py;     then pass "README media block";  else fail "README media block";  fi
if run bash    tools/maintainer/ci_guards.sh;             then pass "repo hygiene guards"; else fail "repo hygiene guards"; fi

echo "== Plugin manifest validation (claude plugin validate --strict) =="
if command -v claude >/dev/null 2>&1; then
  if run claude plugin validate . --strict; then pass "marketplace manifest"; else fail "marketplace manifest"; fi
  for d in skills/*/; do
    slug="$(basename "$d")"
    [ -f "$d/.claude-plugin/plugin.json" ] || continue
    if run claude plugin validate "skills/$slug" --strict; then pass "plugin $slug"; else fail "plugin $slug"; fi
  done
else
  warn "claude CLI not installed - skipped plugin validate (install Claude Code to run it)"
fi

echo "== Release matrix sanity =="
if python3 tools/maintainer/release_matrix.py | python3 -c 'import json,sys; m=json.load(sys.stdin); assert m["skill"] and m["target"]' 2>/tmp/va.out; then
  pass "release matrix is valid JSON with skills + targets"
else
  cat /tmp/va.out; fail "release matrix"
fi

echo "== Catalog idempotency =="
# build-catalog.py regenerates ALL of these from skills.json: the machine
# catalog, the README marker blocks, the Jekyll data file, and every per-skill
# docs page. The gate must prove regeneration is a no-op for every one of them,
# else a stale committed surface ships (the 2026-06-05 staleness class of bug).
cp catalog.json /tmp/cat.bak; cp README.md /tmp/readme.bak
cp docs/_data/catalog.json /tmp/docscat.bak
rm -rf /tmp/docsskills.bak; cp -R docs/skills /tmp/docsskills.bak
# Generator failure must fail the gate outright - unchanged files after a
# crashed regeneration would otherwise read as a PASS.
if ! python3 tools/maintainer/build-catalog.py >/tmp/buildcat.out 2>&1; then
  cat /tmp/buildcat.out
  fail "build-catalog.py failed during the idempotency check"
elif diff -q catalog.json /tmp/cat.bak >/dev/null \
   && diff -q README.md /tmp/readme.bak >/dev/null \
   && diff -q docs/_data/catalog.json /tmp/docscat.bak >/dev/null \
   && diff -r docs/skills /tmp/docsskills.bak >/dev/null; then
  pass "catalog + README + docs/_data/catalog.json + docs/skills already in sync (no drift)"
else
  fail "catalog/README/docs drift - regenerated; re-run was not a no-op"
fi

echo "== llms.txt idempotency =="
cp docs/llms.txt /tmp/llms.bak; cp docs/llms-full.txt /tmp/llmsfull.bak
if ! python3 tools/maintainer/build-llms.py >/tmp/buildllms.out 2>&1; then
  cat /tmp/buildllms.out
  fail "build-llms.py failed during the idempotency check"
elif diff -q docs/llms.txt /tmp/llms.bak >/dev/null && diff -q docs/llms-full.txt /tmp/llmsfull.bak >/dev/null; then
  pass "llms.txt + llms-full.txt already in sync (no drift)"
else
  fail "llms drift - regenerated; re-run was not a no-op (run build-llms.py and commit)"
fi

echo "== Install scripts resolve cleanly (dry run) =="
for sh in skills/*/install.sh; do
  slug="$(basename "$(dirname "$sh")")"
  # First-release chicken-and-egg: a never-released skill (cli_hash_at_release
  # null in skills.json) has no <slug>-v* tag yet, so the live tag lookup can
  # never succeed. Pin the release base to the version it WILL ship as - the
  # script's full logic still runs; only the tag lookup is bypassed.
  pin="$(SLUG="$slug" python3 -c "
import json, os
slug = os.environ['SLUG']
reg = json.load(open('tools/maintainer/skills.json'))['skills'].get(slug, {})
if reg and not reg.get('markdown_only') and reg.get('cli_hash_at_release') in (None, ''):
    print(json.load(open(f'skills/{slug}/manifest.json'))['version'])
")"
  if [ -n "$pin" ]; then
    out="$(DRY_RUN=1 MSP_SKILLS_RELEASE_BASE="https://github.com/servosity/msp-skills/releases/download/${slug}-v${pin}" bash "$sh" 2>&1 || true)"
    if echo "$out" | grep -q 'github.com/servosity/msp-skills' && ! echo "$out" | grep -qi 'PLACEHOLDER'; then
      pass "$slug install.sh URLs resolve (first release pending - pinned ${slug}-v${pin})"
    else
      echo "$out"; fail "$(dirname "$sh") install.sh URL"
    fi
  else
    # Released skill: prefer pinning to the newest LOCAL <slug>-v* tag. The
    # unpinned path makes ~5 paginated releases-API calls per skill per run;
    # at fleet scale that trips GitHub secondary rate limits intermittently
    # (403 -> "No release found" -> false FAIL). Local tags are deterministic.
    # Fallback (shallow clones with no tags, e.g. CI): the live API path.
    local_tag="$(git tag -l "${slug}-v*" --sort=-version:refname 2>/dev/null | head -1)"
    if [ -n "$local_tag" ]; then
      out="$(DRY_RUN=1 MSP_SKILLS_RELEASE_BASE="https://github.com/servosity/msp-skills/releases/download/${local_tag}" bash "$sh" 2>&1 || true)"
    else
      out="$(DRY_RUN=1 bash "$sh" 2>&1 || true)"
    fi
    if echo "$out" | grep -q 'github.com/servosity/msp-skills' && ! echo "$out" | grep -qi 'PLACEHOLDER'; then
      pass "$slug install.sh URLs resolve${local_tag:+ (pinned from local tag ${local_tag})}"
    else
      echo "$out"; fail "$(dirname "$sh") install.sh URL"
    fi
  fi
done

echo "== Best-effort (warn if tool missing locally) =="
if command -v shellcheck >/dev/null 2>&1; then
  # NOT mapfile: macOS ships bash 3.2, where mapfile doesn't exist and the
  # check silently no-ops. while-read is portable to every bash CI or a Mac runs.
  shs=()
  while IFS= read -r f; do shs+=("$f"); done < <(git ls-files '*.sh' 2>/dev/null || find . -name '*.sh' -not -path './.git/*')
  if [ "${#shs[@]}" -gt 0 ] && shellcheck "${shs[@]}" >/tmp/va.out 2>&1; then pass "shellcheck"; else cat /tmp/va.out; fail "shellcheck"; fi
else
  warn "shellcheck not installed - skipped (CI runs it)"
fi

if command -v gitleaks >/dev/null 2>&1; then
  if gitleaks detect --no-git --source . --config .gitleaks.toml --no-banner >/tmp/va.out 2>&1; then pass "gitleaks"; else cat /tmp/va.out; fail "gitleaks"; fi
else
  warn "gitleaks not installed - skipped (CI runs it)"
fi

if command -v actionlint >/dev/null 2>&1; then
  if actionlint .github/workflows/*.yml >/tmp/va.out 2>&1; then pass "actionlint"; else cat /tmp/va.out; fail "actionlint"; fi
else
  warn "actionlint not installed - skipped (CI runs it; try: go run github.com/rhysd/actionlint/cmd/actionlint@latest)"
fi

echo ""
if [ "$hard_fail" -eq 0 ]; then
  echo "VERIFY_ALL: PASS - every hard gate is green."
else
  echo "VERIFY_ALL: FAIL - fix the items marked FAIL above." >&2
fi
exit "$hard_fail"
