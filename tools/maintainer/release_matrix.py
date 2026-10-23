#!/usr/bin/env python3
"""Print the GitHub Actions build matrix for release.yml / ci.yml as JSON.

The matrix is COMPUTED from the repo state - tools/skills.json plus introspection
of each skills/<slug>/cli/ tree - so adding a skill needs zero workflow edits and
the release pipeline can never drift from what is actually vendored.

Usage:
    python3 tools/release_matrix.py              # full {skill, target} matrix (release.yml)
    python3 tools/release_matrix.py --skills-only # {skill} only (ci.yml build+vet)
    python3 tools/release_matrix.py --tag <tag>  # only the skill the tag names
                                                 # (release.yml: one tag = one skill,
                                                 # not N no-op matrix rows)
    python3 tools/release_matrix.py --skills-only --changed-only <base-ref>
                                                 # only skills with changes under
                                                 # skills/<slug>/ since MERGE-BASE
                                                 # with <base-ref> (ci.yml PRs).
    python3 tools/release_matrix.py --skills-only --changed-since <sha>
                                                 # same, but the DIRECT diff
                                                 # <sha>..HEAD, no merge-base
                                                 # (ci.yml pushes to main, where
                                                 # <sha> is github.event.before: a
                                                 # force-push that rewinds main to
                                                 # an ancestor has merge-base ==
                                                 # HEAD and would diff nothing).
                                                 # Both fall back to the FULL
                                                 # matrix if build machinery
                                                 # changed or git fails (fail
                                                 # open, never silently skip a
                                                 # needed build).
    python3 tools/release_matrix.py --self-test  # prove the change-set classifier
                                                 # BOTH directions, offline

Each skill entry carries everything the build step needs:
    name, dir, module, cli_cmd, mcp_cmd, cli_bin, mcp_bin
cmd paths are introspected, so they are correct whether the source is stripped
(cmd/<slug>-cli) or not (cmd/<slug>-pp-cli).

The FULL matrix (the release.yml shape, i.e. not --skills-only) additionally
carries `assets`: the FINAL literal asset filenames per target, keyed
"<goos>-<goarch>", straight from registry.asset_map(). release.yml reads those
names instead of re-deriving the "-<goos>-<goarch>" suffix and the windows
".exe" rule in shell, so the workflow and tools/maintainer/check_release_contract.py
can no longer disagree about what a release asset is called. --skills-only omits
`assets` - that shape (ci.yml build+vet) has no target axis to key them by.

The FULL matrix also carries `mcpb_asset`: the one literal filename of the
cross-platform MCPB bundle release.yml attaches and mcp-publish.yml hands to the
MCP Registry. It lives here for the same reason `assets` does - the bundle name
was previously rebuilt as "<slug>-mcp.mcpb" in two workflows and a gate, three
copies that agree only because every skill happens to name its MCP binary
"<slug>-mcp" today. One definition, read by everyone.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)
import windows_known_failures  # noqa: E402  (local tools/ module)


def skill_entries(with_assets: bool = True) -> list[dict]:
    entries = []
    for slug, meta in registry.skills().items():
        # markdown-only skills have no vendored cli/ - nothing to build or release.
        if registry.is_markdown_only(slug):
            continue
        cli_cmd, mcp_cmd = registry.cmd_dirs(slug)
        entry = {
            "name": slug,
            "dir": f"skills/{slug}/cli",
            "module": registry.module_path(slug),
            "cli_cmd": cli_cmd,
            "mcp_cmd": mcp_cmd,
            "cli_bin": meta["cli_binary"],
            "mcp_bin": meta["mcp_binary"],
        }
        if with_assets:
            # Final literal filenames, one entry per registry.TARGETS row. The
            # release workflow uploads exactly these; nothing downstream rebuilds
            # the suffix or the .exe rule by hand.
            entry["assets"] = registry.asset_map(meta["cli_binary"], meta["mcp_binary"])
            # The single cross-platform MCPB bundle. Not per-target: one bundle
            # carries every platform. Derived from the registry's mcp_binary so
            # a workflow can never disagree with the gate about its name.
            entry["mcpb_asset"] = f"{meta['mcp_binary']}.mcpb"
        entries.append(entry)
    return entries


# --------------------------------------------------------------------------- #
# Change-set classification for --changed-only.
#
# Three buckets, checked in this order:
#
#   MACHINERY  a file that can alter WHICH skills need building or HOW a matrix
#              row builds/verifies them. Scoping is unsafe -> FULL matrix.
#   DERIVED    a file build-catalog.py / build-llms.py / release_state.py
#              REGENERATE (catalog.yml auto-commits them to main as
#              "chore: regenerate derived files ... [bot]"). No build reads
#              any of them and no matrix-row check does -> contributes no row.
#   skills/<slug>/**   everything else under a skill -> that skill's row.
#
# Why DERIVED is an explicit list and not "the commit author is the bot":
#   * the PR path diffs a MERGE-BASE, where a commit author means nothing;
#   * a push range can carry a bot commit AND a human commit, and the human
#     one must still build;
#   * an author string is a convention anyone can set, while this list is the
#     generator's own output list, and --self-test proves it both ways.
# The one derived file that lives INSIDE a skill directory, and so used to
# cost a matrix row on every bot commit, is skills/<slug>/README.md: the
# generator re-mints the version pin in its `.mcpb` download URL after every
# release (build-catalog.py sync_skill_readme_mcpb). That file is ALSO
# hand-edited, and check_cli_claims validates its prose against the built
# binary, so it cannot be classified by path. It is classified by CONTENT
# instead: a README change whose only difference is the tag segment of that
# URL is derived; any other difference is a human edit and builds the row.
# --------------------------------------------------------------------------- #

MACHINERY = (
    # Workflows that FEED builds. Deliberately NOT the whole .github/workflows/
    # prefix: catalog.yml and live-verified.yml are docs auto-committers that
    # never run a matrix row, and a comment edit to either used to cost the
    # full fleet matrix.
    ".github/workflows/ci.yml",
    ".github/workflows/release.yml",
    ".github/workflows/security-gate.yml",
    ".github/workflows/mcp-publish.yml",
    # Called by release.yml before every seal, so it feeds release builds.
    ".github/workflows/windows-smoke.yml",
    # The matrix computation itself.
    "tools/maintainer/release_matrix.py",
    "tools/maintainer/registry.py",
    # Every script a ci.yml matrix row executes (enumerated from its build
    # job), plus the hash that release_state.py compares binaries by. A change
    # to any of these changes what "verified" means for every row.
    "tools/maintainer/check_cli_claims.py",
    "tools/maintainer/check_handfixes.py",
    "tools/maintainer/check_mcp_gate.py",
    "tools/maintainer/check_mcp_file_flags.py",
    "tools/maintainer/mcp_file_flags/",
    "tools/maintainer/check_doctor_truth.py",
    "tools/maintainer/windows_smoke.ps1",
    "tools/maintainer/windows_known_failures.py",
    "tools/maintainer/cli_hash.py",
    # A Go workspace in an ANCESTOR of skills/<slug>/cli. None exists today,
    # but `go build` walks up looking for one (GOWORK=auto), so introducing or
    # editing one at the repo root or at skills/ changes every connector's
    # dependency resolution without touching a single file under a skill.
    # (One inside skills/<slug>/ is that skill's own row.)
    "go.work",
    "go.work.sum",
    "skills/go.work",
    "skills/go.work.sum",
)

# Path-classified derived files (all OUTSIDE skills/). Kept explicit so the
# classification is readable and provable, even though a path outside skills/
# would never have produced a row: what matters is that none of these is
# MACHINERY, and that the list matches what catalog.yml auto-commits.
DERIVED_EXACT = (
    "catalog.json",
    "README.md",
    "docs/llms.txt",
    "docs/llms-full.txt",
    "docs/_data/catalog.json",
    "docs/_data/pending.json",
    ".github/ISSUE_TEMPLATE/it-works.yml",
    ".github/ISSUE_TEMPLATE/bug-report.yml",
)
DERIVED_PREFIX = ("docs/skills/",)

# The registry is an INPUT to the matrix, not machinery: a change to one
# slug's entry (binary names, source_dir, markdown_only) re-builds THAT slug,
# not the fleet. release stamps (version, cli_hash_at_release) land here too
# and cost that one row, which is cheap and correct.
REGISTRY_PATH = "tools/maintainer/skills.json"

# The Windows known-failures ledger is an INPUT too, classified by CONTENT. A
# windows row reads only its own slug's effective exclusions from it
# (windows_known_failures.py --slug), so an edit that changes which tests one
# skill skips re-tests that skill and no other. Before 2026-10-21 it was
# MACHINERY: #395 deleted a few lines and every push cost a 65 x 2 matrix. The
# whole fleet still runs when the change cannot be scoped: any edit to a
# `slugs: ["*"]` (every-connector) entry, or a revision that is missing,
# unreadable or fails the ledger's own validation. The weekly full sweep still
# bounds what any scoping could miss.
WINDOWS_LEDGER_PATH = "tools/maintainer/windows_known_test_failures.json"

# Mirrors build-catalog.py sync_skill_readme_mcpb: only the tag segment of a
# `.mcpb` download URL under this repo's releases is generated. Anything else
# on the line - or any other line - is authored prose.
_OWNER, _REPO = registry.owner_repo()
_MCPB_PIN_RE = re.compile(
    rf"(https://github\.com/{re.escape(_OWNER)}/{re.escape(_REPO)}"
    rf"/releases/download/)[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+\.mcpb)"
)


def is_machinery(path: str) -> bool:
    return any(path == m or path.startswith(m) for m in MACHINERY)


def is_derived_path(path: str) -> bool:
    return path in DERIVED_EXACT or any(path.startswith(p) for p in DERIVED_PREFIX)


def readme_pin_only(old: str | None, new: str | None) -> bool:
    """True when two revisions of a skill README differ ONLY in the version tag
    of their `.mcpb` download URL(s) - the rewrite the catalog generator
    performs after a release. A file that is new, deleted, or different
    anywhere else is a real change."""
    if old is None or new is None:
        return False
    norm = lambda s: _MCPB_PIN_RE.sub(r"\g<1><tag>\g<2>", s)  # noqa: E731
    return norm(old) == norm(new)


def _skill_readme_slug(path: str) -> str | None:
    parts = path.split("/")
    if len(parts) == 3 and parts[0] == "skills" and parts[2] == "README.md":
        return parts[1]
    return None


def _no_ledger_change() -> set[str] | None:
    return set()


def classify(
    files: list[str],
    pin_only: Callable[[str], bool],
    registry_delta: Callable[[], set[str] | None],
    ledger_delta: Callable[[], set[str] | None] = _no_ledger_change,
    reasons: list[str] | None = None,
) -> set[str] | None:
    """Pure core of --changed-only. Returns the set of slugs to build, or None
    for "the FULL matrix" (machinery changed, or a registry or ledger diff
    could not be scoped - fail open).

    `pin_only(path)` answers whether a changed skills/<slug>/README.md is a
    generated pin re-mint; `registry_delta()` answers which slugs' skills.json
    entries changed; `ledger_delta()` answers which slugs' effective Windows
    exclusions changed (None = the whole fleet). All are injected so
    --self-test can drive this without a git checkout, and so the git-backed
    versions below stay thin. `reasons`, when given, collects why a FULL
    matrix was chosen (verify_all.sh records it in its verdict)."""
    def full(why: str) -> None:
        print(f"release_matrix: {why}; FULL matrix", file=sys.stderr)
        if reasons is not None:
            reasons.append(why)
        return None

    slugs: set[str] = set()
    for f in files:
        if is_machinery(f):
            return full(f"machinery changed ({f})")
    for f in files:
        if is_derived_path(f):
            print(f"release_matrix: derived file ({f}); no row", file=sys.stderr)
            continue
        if f == REGISTRY_PATH:
            delta = registry_delta()
            if delta is None:
                return full("registry diff unreadable")
            print(f"release_matrix: registry entries changed for {sorted(delta)}",
                  file=sys.stderr)
            slugs |= delta
            continue
        if f == WINDOWS_LEDGER_PATH:
            delta = ledger_delta()
            if delta is None:
                return full("Windows known-failures change cannot be scoped "
                            "(an every-connector entry changed, or a revision is unreadable or invalid)")
            print(f"release_matrix: Windows known-failure exclusions changed for {sorted(delta)}",
                  file=sys.stderr)
            slugs |= delta
            continue
        slug = _skill_readme_slug(f)
        if slug is not None and pin_only(f):
            print(f"release_matrix: derived .mcpb pin re-mint ({f}); no row",
                  file=sys.stderr)
            continue
        if f.startswith("skills/") and f.count("/") >= 2:
            slugs.add(f.split("/", 2)[1])
    return slugs


def _git_show(rev: str | None, path: str) -> str | None:
    """File content at a revision, or None if it does not exist there.
    rev None means the WORKING TREE (verify_all.sh --changed reads
    uncommitted edits too; CI never passes None)."""
    if rev is None:
        try:
            return (registry.ROOT / path).read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            return None
    p = subprocess.run(
        ["git", "show", f"{rev}:{path}"],
        capture_output=True, text=True, timeout=30,
    )
    return p.stdout if p.returncode == 0 else None


def ledger_delta(old_txt: str | None, new_txt: str | None) -> set[str] | None:
    """Slugs whose EFFECTIVE Windows exclusions differ between two ledger
    revisions, or None for "the whole fleet". Effective = what
    windows_known_failures.py --slug would hand that slug's windows row, so a
    prose-only edit (why/fix/kind/_doc) or a test moved between two entries of
    the same skill changes nothing and builds nothing. None when either
    revision is missing or fails the ledger's own validation (the windows job
    would refuse it for every row), or when any every-connector (`slugs:
    ["*"]`) entry was added, removed or edited at all."""
    def parse(text: str | None) -> list[dict] | None:
        if text is None:
            return None
        fd, tmp = tempfile.mkstemp(suffix=".json")
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                fh.write(text)
            return windows_known_failures.load(Path(tmp))
        except Exception:  # noqa: BLE001 - LedgerError, or a wrong-typed field
            # load() can also raise TypeError/AttributeError on a shape it
            # does not anticipate (`"kind": []`). Any refusal or crash means
            # the revision cannot be scoped: the whole fleet, never a crash.
            return None
        finally:
            os.unlink(tmp)

    old, new = parse(old_txt), parse(new_txt)
    if old is None or new is None:
        return None

    def wildcard(entries: list[dict]) -> list[str]:
        # Whole entries, not just their tests: any edit to an every-connector
        # entry re-tests the fleet, which is the conservative reading.
        return sorted(json.dumps(e, sort_keys=True) for e in entries if "failures" not in e)

    if wildcard(old) != wildcard(new):
        return None
    named = {s for es in (old, new) for e in es if "failures" in e for s in e["failures"]}
    return {s for s in named
            if windows_known_failures.tests_for(s, old) != windows_known_failures.tests_for(s, new)}


def _registry_delta(mb: str, new_rev: str | None = "HEAD") -> set[str] | None:
    """Slugs whose skills.json entry differs between merge-base and HEAD (or
    the working tree when new_rev is None). None (fail open) if either
    revision cannot be read or parsed."""
    try:
        old_txt = _git_show(mb, REGISTRY_PATH)
        new_txt = _git_show(new_rev, REGISTRY_PATH)
        if old_txt is None or new_txt is None:
            return None
        old = json.loads(old_txt).get("skills", {})
        new = json.loads(new_txt).get("skills", {})
    except (subprocess.SubprocessError, OSError, ValueError, AttributeError):
        return None
    if not isinstance(old, dict) or not isinstance(new, dict):
        return None
    return {s for s in set(old) | set(new) if old.get(s) != new.get(s)}


def change_set(base_ref: str, direct: bool = False, worktree: bool = False) -> tuple[str, list[str]] | None:
    """(diff base SHA, changed paths), or None when git cannot compute it.

    `direct=False` diffs from the MERGE-BASE of base_ref and HEAD; `direct=True`
    from base_ref itself (see changed_slugs). `worktree=True` (verify_all.sh
    only, never CI) compares that base with the working tree instead of HEAD
    and adds untracked files, so an uncommitted edit is scoped too."""
    try:
        if direct:
            mb = subprocess.run(
                ["git", "rev-parse", "--verify", f"{base_ref}^{{commit}}"],
                capture_output=True, text=True, check=True, timeout=30,
            ).stdout.strip()
        else:
            mb = subprocess.run(
                ["git", "merge-base", base_ref, "HEAD"],
                capture_output=True, text=True, check=True, timeout=30,
            ).stdout.strip()
        # --no-renames: with rename detection a move out of skills/<slug>/
        # lists only the DESTINATION, and the skill that lost a file is never
        # built. -z: paths arrive NUL-separated and unquoted, so a non-ASCII
        # filename is not wrapped in "..." (core.quotePath) where it would
        # fail every startswith() below and silently skip its skill.
        diff = subprocess.run(
            ["git", "diff", "--no-renames", "--name-only", "-z", mb] + ([] if worktree else ["HEAD"]),
            capture_output=True, text=True, check=True, timeout=60,
        ).stdout
        if worktree:
            diff += "\0" + subprocess.run(
                ["git", "ls-files", "--others", "--exclude-standard", "-z"],
                capture_output=True, text=True, check=True, timeout=60,
            ).stdout
    except (subprocess.SubprocessError, OSError) as e:
        print(f"release_matrix: --changed-only diff failed ({e}); "
              "falling back to FULL matrix", file=sys.stderr)
        return None
    return mb, sorted({f for f in diff.split("\0") if f.strip()})


def classify_change(mb: str, files: list[str], new_rev: str | None = "HEAD",
                    reasons: list[str] | None = None) -> set[str] | None:
    """classify() wired to git: the slugs `files` (changed between mb and
    new_rev; None = the working tree) need built, or None for the FULL matrix."""
    def pin_only(path: str) -> bool:
        try:
            return readme_pin_only(_git_show(mb, path), _git_show(new_rev, path))
        except (subprocess.SubprocessError, OSError, ValueError):  # ValueError: invalid UTF-8
            return False  # unreadable -> treat as a real change (build it)

    def win_delta() -> set[str] | None:
        try:
            return ledger_delta(_git_show(mb, WINDOWS_LEDGER_PATH), _git_show(new_rev, WINDOWS_LEDGER_PATH))
        except (subprocess.SubprocessError, OSError, ValueError):  # ValueError: invalid UTF-8
            return None

    return classify(files, pin_only, lambda: _registry_delta(mb, new_rev), win_delta, reasons)


def changed_slugs(base_ref: str, entries: list[dict], direct: bool = False) -> list[dict] | None:
    """Filter entries to skills changed since base_ref.

    `direct=False` (PRs) diffs from the MERGE-BASE of base_ref and HEAD, so a
    base branch that advanced does not count as this change-set. `direct=True`
    (pushes) diffs base_ref..HEAD as two points: base_ref is the tip main had
    before the push, and the question is what the tree at HEAD changed
    relative to it. A merge-base would be wrong there - a force-push that
    rewinds main to an ancestor has merge-base == HEAD and diffs NOTHING while
    the tree lost commits; the two-point diff sees those files and rebuilds
    them. For a normal fast-forward push the two agree exactly.

    Returns None to mean "use the full matrix" (machinery changed, or git
    failed - fail open). The filter is skills/<slug>/** (not just cli/**):
    the ci.yml matrix job also runs check_cli_claims, which validates the
    skill's DOCS against the built binary, so a docs-only edit to a skill
    still needs that skill's row. Derived files (see classify) and non-skill
    changes produce no row.
    """
    cs = change_set(base_ref, direct=direct)
    if cs is None:
        return None
    mb, files = cs
    touched = classify_change(mb, files)
    if touched is None:
        return None
    return [e for e in entries if e["name"] in touched]


# --------------------------------------------------------------------------- #
# --self-test: the classifier proved BOTH directions, with no git involved.
# --------------------------------------------------------------------------- #

_PIN_LINE = (
    "[**Download Zammad MCP (.mcpb)**](https://github.com/{o}/{r}/releases/"
    "download/{tag}/zammad-mcp.mcpb) - then open **Claude Desktop > Settings > "
    "Extensions** and select the file.\n"
)


def _self_test() -> int:
    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    o, r = _OWNER, _REPO
    pre = "# Zammad\n\nSome prose.\n\n"
    post = "\nPrefer the Claude Code plugin?\n"
    old = pre + _PIN_LINE.format(o=o, r=r, tag="zammad-v0.1.3") + post
    new = pre + _PIN_LINE.format(o=o, r=r, tag="zammad-v0.1.4") + post

    print("readme_pin_only:")
    expect(readme_pin_only(old, new), "tag re-mint only -> derived")
    expect(readme_pin_only(old, old), "identical -> derived (no change)")
    expect(not readme_pin_only(old, new.replace("Some prose.", "Other prose.")),
           "tag re-mint PLUS a prose edit -> real change")
    expect(not readme_pin_only(old, pre + _PIN_LINE.format(o=o, r=r, tag="zammad-v0.1.3")
                               .replace("Extensions", "Plugins") + post),
           "prose edit on the pin line itself -> real change")
    expect(not readme_pin_only(old, new.replace("zammad-mcp.mcpb", "zammad-cli.mcpb")),
           "asset name changed -> real change")
    expect(not readme_pin_only(old, new.replace(f"github.com/{o}/{r}", "github.com/other/repo")),
           "a pin under another repo -> real change")
    expect(not readme_pin_only(None, new), "new file -> real change")
    expect(not readme_pin_only(old, None), "deleted file -> real change")

    print("is_machinery:")
    for path, want in (
        (".github/workflows/ci.yml", True),
        (".github/workflows/release.yml", True),
        (".github/workflows/security-gate.yml", True),
        (".github/workflows/mcp-publish.yml", True),
        (".github/workflows/windows-smoke.yml", True),
        (".github/workflows/installer-fixtures.yml", False),
        (".github/workflows/catalog.yml", False),
        (".github/workflows/live-verified.yml", False),
        ("tools/maintainer/check_doctor_truth.py", True),
        ("tools/maintainer/check_handfixes.py", True),
        ("tools/maintainer/check_mcp_gate.py", True),
        ("tools/maintainer/check_mcp_file_flags.py", True),
        ("tools/maintainer/mcp_file_flags/engine.go", True),
        ("tools/maintainer/mcp_file_flags/testdata/fixture/cli/cli.go", True),
        ("tools/maintainer/check_cli_claims.py", True),
        ("tools/maintainer/windows_smoke.ps1", True),
        ("tools/maintainer/windows_known_failures.py", True),
        # content-classified per slug now (ledger_delta), like skills.json
        ("tools/maintainer/windows_known_test_failures.json", False),
        ("tools/maintainer/release_matrix.py", True),
        ("tools/maintainer/build-catalog.py", False),
        ("tools/maintainer/env_schema_internal.json", False),
        ("docs/_data/pending.json", False),
        ("go.work", True),
        ("go.work.sum", True),
        ("skills/go.work", True),
        ("skills/go.work.sum", True),
        ("skills/zammad/cli/go.work", False),   # inside a skill: that skill's row
    ):
        expect(is_machinery(path) is want, f"{path} -> {'machinery' if want else 'not machinery'}")

    print("is_derived_path:")
    for path, want in (
        ("docs/_data/pending.json", True),
        ("docs/skills/zammad.md", True),
        ("README.md", True),
        (".github/ISSUE_TEMPLATE/it-works.yml", True),
        (".github/ISSUE_TEMPLATE/skill-request.yml", False),
        ("skills/zammad/README.md", False),   # content-classified, not path
        ("skills/zammad/cli/main.go", False),
        ("docs/reprint-survival.md", False),
    ):
        expect(is_derived_path(path) is want, f"{path} -> {'derived' if want else 'not derived'}")

    print("classify:")
    pins = {"skills/zammad/README.md": True, "skills/hudu/README.md": True}
    no_pins = {"skills/zammad/README.md": False}
    empty_reg: Callable[[], set[str] | None] = lambda: set()  # noqa: E731
    bot = ["docs/_data/pending.json", "skills/zammad/README.md", "skills/hudu/README.md"]
    expect(classify(bot, pins.get, empty_reg) == set(),
           "bot regen commit (pending.json + 2 pin re-mints) -> EMPTY matrix")
    expect(classify(["skills/zammad/README.md"], no_pins.get, empty_reg) == {"zammad"},
           "human README edit -> that skill's row")
    expect(classify(["skills/zammad/cli/internal/x.go"], pins.get, empty_reg) == {"zammad"},
           "Go change -> that skill's row")
    expect(classify(bot + ["skills/cove/cli/go.mod"], pins.get, empty_reg) == {"cove"},
           "bot regen + one real change in the same range -> only the real one")
    expect(classify(["tools/maintainer/check_doctor_truth.py"], pins.get, empty_reg) is None,
           "matrix-row checker changed -> FULL matrix")
    expect(classify([".github/workflows/ci.yml"], pins.get, empty_reg) is None,
           "ci.yml changed -> FULL matrix")
    expect(classify([".github/workflows/catalog.yml", "docs/skills/hudu.md"], pins.get, empty_reg) == set(),
           "catalog.yml + a docs page -> EMPTY matrix")
    expect(classify(["docs/reprint-survival.md", "CONTRIBUTING.md"], pins.get, empty_reg) == set(),
           "repo docs -> EMPTY matrix")
    expect(classify([REGISTRY_PATH], pins.get, lambda: {"hudu"}) == {"hudu"},
           "skills.json entry for one slug changed -> that slug's row")
    expect(classify([REGISTRY_PATH], pins.get, lambda: None) is None,
           "skills.json unreadable at a revision -> FULL matrix (fail open)")
    expect(classify(["skills/_meta/SKILL.md"], pins.get, empty_reg) == {"_meta"},
           "a non-connector skill dir still names its slug (filtered by the registry later)")
    expect(classify(["skills/zammad/cli/internal/\u00e9.go"], pins.get, empty_reg) == {"zammad"},
           "a non-ASCII filename (unquoted, as -z delivers it) -> that skill's row")
    expect(classify(["docs/moved.go", "skills/zammad/cli/moved.go"], pins.get, empty_reg) == {"zammad"},
           "a move out of a skill (both sides listed, as --no-renames delivers it) -> that skill's row")
    expect(classify(["skills/go.work"], pins.get, empty_reg) is None,
           "a Go workspace at skills/ (ancestor of every cli/) -> FULL matrix")
    expect(classify([WINDOWS_LEDGER_PATH], pins.get, empty_reg, lambda: {"alpha"}) == {"alpha"},
           "Windows ledger change scoped to one slug -> that slug's row only")
    expect(classify([WINDOWS_LEDGER_PATH], pins.get, empty_reg, lambda: set()) == set(),
           "Windows ledger prose-only change -> EMPTY matrix")
    expect(classify([WINDOWS_LEDGER_PATH], pins.get, empty_reg, lambda: None) is None,
           "Windows ledger change that cannot be scoped -> FULL matrix (fail open)")
    expect(classify([WINDOWS_LEDGER_PATH, "skills/cove/cli/go.mod"], pins.get, empty_reg,
                    lambda: {"alpha"}) == {"alpha", "cove"},
           "Windows ledger change + a skill change -> both rows")
    why: list[str] = []
    classify(["tools/maintainer/check_mcp_gate.py"], pins.get, empty_reg, reasons=why)
    expect(why == ["machinery changed (tools/maintainer/check_mcp_gate.py)"],
           "a FULL-matrix decision records its reason")

    print("ledger_delta (effective Windows exclusions, old vs new):")

    def led(*entries: dict) -> str:
        return json.dumps({"entries": list(entries)})

    star = {"id": "W", "kind": "real-bug", "slugs": ["*"], "tests": ["TestEverywhere"], "why": "w", "fix": "f"}
    per = {"id": "B", "kind": "unix-only-test", "why": "w", "fix": "f",
           "failures": {"alpha": ["TestA1"], "beta": ["TestB1"]}}
    base = led(star, per)

    def with_failures(fmap: dict) -> dict:
        return dict(per, failures=fmap)

    for label, new, want in (
        ("identical", base, set()),
        ("a test ADDED for one skill", led(star, with_failures({"alpha": ["TestA1", "TestA2"], "beta": ["TestB1"]})),
         {"alpha"}),
        ("a test REMOVED for one skill (the #395 shape)", led(star, with_failures({"alpha": ["TestA1"]})), {"beta"}),
        ("a skill ADDED to the ledger", led(star, with_failures({"alpha": ["TestA1"], "beta": ["TestB1"],
                                                                "gamma": ["TestG1"]})), {"gamma"}),
        ("a whole entry REMOVED", led(star), {"alpha", "beta"}),
        ("prose only (why / fix / kind)", led(star, dict(per, why="reworded", fix="later", kind="flaky")), set()),
        ("a test moved between two entries of the same skill",
         led(star, with_failures({"alpha": ["TestA1"]}),
             {"id": "C", "kind": "flaky", "why": "w", "fix": "f", "failures": {"beta": ["TestB1"]}}), set()),
        ("an every-connector ('*') test ADDED", led(dict(star, tests=["TestEverywhere", "TestNew"]), per), None),
        ("an every-connector ('*') entry REMOVED", led(per), None),
        ("an every-connector ('*') entry ADDED", led(star, per, dict(star, id="W2", tests=["TestOther"])), None),
        ("an every-connector ('*') prose-only edit (any wildcard edit is fleet-wide)",
         led(dict(star, why="reworded"), per), None),
        ("new revision is malformed JSON", "{", None),
        ("new revision fails validation (unknown kind)", led(star, dict(per, kind="whatever")), None),
        ("new revision fails validation (a regex for a test name)", led(star, with_failures({"alpha": ["Test.*"]})),
         None),
        ("ledger deleted (no new revision)", None, None),
        ("new revision has a wrong-typed field (kind is a list)", led(star, dict(per, kind=[])), None),
        ("new revision has a wrong-typed field (failures is a list)", led(star, dict(per, failures=["alpha"])), None),
        ("new revision's entries is not a list", json.dumps({"entries": {"a": 1}}), None),
    ):
        got = ledger_delta(base, new)
        expect(got == want, f"{label} -> {'FULL fleet' if want is None else sorted(want) or 'no rows'}"
                            f" (got {'FULL fleet' if got is None else sorted(got) or 'no rows'})")
    expect(ledger_delta(None, base) is None, "ledger added (no old revision) -> FULL fleet")
    expect(ledger_delta("{", base) is None, "old revision unreadable -> FULL fleet")
    real = WINDOWS_LEDGER_PATH
    try:
        real_txt = (registry.ROOT / real).read_text(encoding="utf-8")
    except OSError:
        real_txt = None
    expect(real_txt is not None and ledger_delta(real_txt, real_txt) == set(),
           "the shipped ledger against itself -> no rows")
    if real_txt is not None:
        # Order-independent: drop the first PER-SKILL entry (a '*' entry is
        # the fleet-wide case, asserted above) and compare against what the
        # windows rows would actually see. Overlapping entries can make the
        # expected set empty, which is still a correct answer.
        data = json.loads(real_txt)
        idx = next((i for i, e in enumerate(data["entries"]) if "failures" in e), None)
        if idx is None:
            print("  skip the shipped ledger has no per-skill entry to drop")
        else:
            kept = data["entries"][:idx] + data["entries"][idx + 1:]
            dropped = json.dumps(dict(data, entries=kept))
            want_slugs = {s for s in data["entries"][idx]["failures"]
                          if windows_known_failures.tests_for(s, data["entries"])
                          != windows_known_failures.tests_for(s, kept)}
            expect(ledger_delta(real_txt, dropped) == want_slugs,
                   f"dropping one per-skill entry of the shipped ledger -> exactly {sorted(want_slugs)}")

    if failures:
        print(f"\nFAIL: release_matrix self-test - {len(failures)} case(s):")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("\nPASS: release_matrix change-set classifier - every case discriminates")
    return 0


def main(argv: list[str]) -> int:
    if "--self-test" in argv:
        return _self_test()
    skills_only = "--skills-only" in argv
    tag = ""
    base_ref = ""
    direct = False
    if "--tag" in argv:
        tag = argv[argv.index("--tag") + 1]
    if "--changed-only" in argv:
        base_ref = argv[argv.index("--changed-only") + 1]
    if "--changed-since" in argv:
        base_ref = argv[argv.index("--changed-since") + 1]
        direct = True

    entries = skill_entries(with_assets=not skills_only)

    if tag:
        # <slug>-v<semver> -> <slug>; unknown slug -> empty matrix (the
        # workflow's `any` guard turns that into "no build jobs at all").
        slug = tag.rsplit("-v", 1)[0]
        entries = [e for e in entries if e["name"] == slug]

    if base_ref:
        filtered = changed_slugs(base_ref, entries, direct=direct)
        if filtered is not None:
            entries = filtered

    matrix: dict = {"skill": entries}
    if not skills_only:
        matrix["target"] = registry.TARGETS
    print(json.dumps(matrix))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
