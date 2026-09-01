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
                                                 # skills/<slug>/ since merge-base
                                                 # with <base-ref> (ci.yml PRs).
                                                 # Falls back to the FULL matrix if
                                                 # build machinery changed or git
                                                 # diff fails (fail open, never
                                                 # silently skip a needed build).

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
import subprocess
import sys
from pathlib import Path

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


# Changes to these paths can alter WHICH skills need building or HOW they are
# built/verified - when one of them changes, scoping is unsafe and we fall back
# to the full matrix.
MACHINERY = (
    ".github/workflows/",
    "tools/maintainer/release_matrix.py",
    "tools/maintainer/registry.py",
    "tools/maintainer/check_cli_claims.py",
    "tools/maintainer/cli_hash.py",
    "tools/maintainer/check_mcp_gate.py",
)


def changed_slugs(base_ref: str, entries: list[dict]) -> list[dict] | None:
    """Filter entries to skills changed since merge-base with base_ref.

    Returns None to mean "use the full matrix" (machinery changed, or git
    failed - fail open). The filter is skills/<slug>/** (not just cli/**):
    the ci.yml matrix job also runs check_cli_claims, which validates the
    skill's DOCS against the built binary, so a docs-only edit to a skill
    still needs that skill's row. Non-skill changes produce an empty matrix.
    """
    try:
        mb = subprocess.run(
            ["git", "merge-base", base_ref, "HEAD"],
            capture_output=True, text=True, check=True, timeout=30,
        ).stdout.strip()
        diff = subprocess.run(
            ["git", "diff", "--name-only", mb, "HEAD"],
            capture_output=True, text=True, check=True, timeout=60,
        ).stdout
    except (subprocess.SubprocessError, OSError) as e:
        print(f"release_matrix: --changed-only diff failed ({e}); "
              "falling back to FULL matrix", file=sys.stderr)
        return None
    files = [f for f in diff.splitlines() if f.strip()]
    for f in files:
        if any(f == m or f.startswith(m) for m in MACHINERY):
            print(f"release_matrix: machinery changed ({f}); FULL matrix",
                  file=sys.stderr)
            return None
    touched = {f.split("/", 2)[1] for f in files
               if f.startswith("skills/") and f.count("/") >= 2}
    return [e for e in entries if e["name"] in touched]


def main(argv: list[str]) -> int:
    skills_only = "--skills-only" in argv
    tag = ""
    base_ref = ""
    if "--tag" in argv:
        tag = argv[argv.index("--tag") + 1]
    if "--changed-only" in argv:
        base_ref = argv[argv.index("--changed-only") + 1]

    entries = skill_entries(with_assets=not skills_only)

    if tag:
        # <slug>-v<semver> -> <slug>; unknown slug -> empty matrix (the
        # workflow's `any` guard turns that into "no build jobs at all").
        slug = tag.rsplit("-v", 1)[0]
        entries = [e for e in entries if e["name"] == slug]

    if base_ref:
        filtered = changed_slugs(base_ref, entries)
        if filtered is not None:
            entries = filtered

    matrix: dict = {"skill": entries}
    if not skills_only:
        matrix["target"] = registry.TARGETS
    print(json.dumps(matrix))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
