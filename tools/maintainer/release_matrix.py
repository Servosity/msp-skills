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
import re
import subprocess
import sys
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)


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
    # The matrix computation itself.
    "tools/maintainer/release_matrix.py",
    "tools/maintainer/registry.py",
    # Every script a ci.yml matrix row executes (enumerated from its build
    # job), plus the hash that release_state.py compares binaries by. A change
    # to any of these changes what "verified" means for every row.
    "tools/maintainer/check_cli_claims.py",
    "tools/maintainer/check_handfixes.py",
    "tools/maintainer/check_mcp_gate.py",
    "tools/maintainer/check_doctor_truth.py",
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


def classify(
    files: list[str],
    pin_only: Callable[[str], bool],
    registry_delta: Callable[[], set[str] | None],
) -> set[str] | None:
    """Pure core of --changed-only. Returns the set of slugs to build, or None
    for "the FULL matrix" (machinery changed, or a registry diff could not be
    read - fail open).

    `pin_only(path)` answers whether a changed skills/<slug>/README.md is a
    generated pin re-mint; `registry_delta()` answers which slugs' skills.json
    entries changed. Both are injected so --self-test can drive this without
    a git checkout, and so the git-backed versions below stay thin."""
    slugs: set[str] = set()
    for f in files:
        if is_machinery(f):
            print(f"release_matrix: machinery changed ({f}); FULL matrix",
                  file=sys.stderr)
            return None
    for f in files:
        if is_derived_path(f):
            print(f"release_matrix: derived file ({f}); no row", file=sys.stderr)
            continue
        if f == REGISTRY_PATH:
            delta = registry_delta()
            if delta is None:
                print("release_matrix: registry diff unreadable; FULL matrix",
                      file=sys.stderr)
                return None
            print(f"release_matrix: registry entries changed for {sorted(delta)}",
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


def _git_show(rev: str, path: str) -> str | None:
    """File content at a revision, or None if it does not exist there."""
    p = subprocess.run(
        ["git", "show", f"{rev}:{path}"],
        capture_output=True, text=True, timeout=30,
    )
    return p.stdout if p.returncode == 0 else None


def _registry_delta(mb: str) -> set[str] | None:
    """Slugs whose skills.json entry differs between merge-base and HEAD.
    None (fail open) if either revision cannot be read or parsed."""
    try:
        old_txt = _git_show(mb, REGISTRY_PATH)
        new_txt = _git_show("HEAD", REGISTRY_PATH)
        if old_txt is None or new_txt is None:
            return None
        old = json.loads(old_txt).get("skills", {})
        new = json.loads(new_txt).get("skills", {})
    except (subprocess.SubprocessError, OSError, ValueError, AttributeError):
        return None
    if not isinstance(old, dict) or not isinstance(new, dict):
        return None
    return {s for s in set(old) | set(new) if old.get(s) != new.get(s)}


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
            ["git", "diff", "--no-renames", "--name-only", "-z", mb, "HEAD"],
            capture_output=True, text=True, check=True, timeout=60,
        ).stdout
    except (subprocess.SubprocessError, OSError) as e:
        print(f"release_matrix: --changed-only diff failed ({e}); "
              "falling back to FULL matrix", file=sys.stderr)
        return None
    files = [f for f in diff.split("\0") if f.strip()]

    def pin_only(path: str) -> bool:
        try:
            return readme_pin_only(_git_show(mb, path), _git_show("HEAD", path))
        except (subprocess.SubprocessError, OSError):
            return False  # unreadable -> treat as a real change (build it)

    touched = classify(files, pin_only, lambda: _registry_delta(mb))
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
        (".github/workflows/catalog.yml", False),
        (".github/workflows/live-verified.yml", False),
        ("tools/maintainer/check_doctor_truth.py", True),
        ("tools/maintainer/check_handfixes.py", True),
        ("tools/maintainer/check_mcp_gate.py", True),
        ("tools/maintainer/check_cli_claims.py", True),
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
