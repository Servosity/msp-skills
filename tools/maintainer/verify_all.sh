#!/usr/bin/env bash
# verify_all.sh - one command, one verdict. Runs every gate that protects the
# monorepo so an onboarding (or any change) cannot slip a regression past.
#
# Hard gates (must pass): go build+vet per module, skill contract, repo guards,
# markdown links, release contract, catalog idempotency, release matrix sanity,
# install dry-run resolves cleanly.
# Best-effort gates (warn if the tool is absent locally; CI installs them):
# shell linting, secret scanning, and workflow linting.
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

echo "== Repo gates =="
if run python3 tools/maintainer/check_skill_contract.py;  then pass "skill contract";      else fail "skill contract";      fi
if run python3 tools/maintainer/check_md_links.py;        then pass "markdown links";      else fail "markdown links";      fi
if run python3 tools/maintainer/check_release_contract.py; then pass "release contract";   else fail "release contract";    fi
if run python3 tools/maintainer/check_social_assets.py;   then pass "social assets";       else fail "social assets";       fi
if run bash    tools/maintainer/ci_guards.sh;             then pass "repo hygiene guards"; else fail "repo hygiene guards"; fi

echo "== Release matrix sanity =="
if python3 tools/maintainer/release_matrix.py | python3 -c 'import json,sys; m=json.load(sys.stdin); assert m["skill"] and m["target"]' 2>/tmp/va.out; then
  pass "release matrix is valid JSON with skills + targets"
else
  cat /tmp/va.out; fail "release matrix"
fi

echo "== Catalog idempotency =="
cp catalog.json /tmp/cat.bak; cp README.md /tmp/readme.bak
python3 tools/maintainer/build-catalog.py >/dev/null 2>&1
if diff -q catalog.json /tmp/cat.bak >/dev/null && diff -q README.md /tmp/readme.bak >/dev/null; then
  pass "catalog + README already in sync (no drift)"
else
  fail "catalog/README drift - regenerated; re-run was not a no-op"
fi

echo "== Install scripts resolve cleanly (dry run) =="
for sh in skills/*/install.sh; do
  out="$(DRY_RUN=1 bash "$sh" 2>&1 || true)"
  if echo "$out" | grep -q 'github.com/servosity/msp-skills' && ! echo "$out" | grep -qi 'PLACEHOLDER'; then
    pass "$(dirname "$sh" | xargs basename) install.sh URLs resolve"
  else
    echo "$out"; fail "$(dirname "$sh") install.sh URL"
  fi
done

echo "== Best-effort (warn if tool missing locally) =="
if command -v shellcheck >/dev/null 2>&1; then
  mapfile -t shs < <(git ls-files '*.sh' 2>/dev/null || find . -name '*.sh' -not -path './.git/*')
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
