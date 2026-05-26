#!/usr/bin/env python3
"""Print the GitHub Actions build matrix for release.yml / ci.yml as JSON.

The matrix is COMPUTED from the repo state - tools/skills.json plus introspection
of each skills/<slug>/cli/ tree - so adding a skill needs zero workflow edits and
the release pipeline can never drift from what is actually vendored.

Usage:
    python3 tools/release_matrix.py              # full {skill, target} matrix (release.yml)
    python3 tools/release_matrix.py --skills-only # {skill} only (ci.yml build+vet)

Each skill entry carries everything the build step needs:
    name, dir, module, cli_cmd, mcp_cmd, cli_bin, mcp_bin
cmd paths are introspected, so they are correct whether the source is stripped
(cmd/<slug>-cli) or not (cmd/<slug>-pp-cli).
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)


def skill_entries() -> list[dict]:
    entries = []
    for slug, meta in registry.skills().items():
        cli_cmd, mcp_cmd = registry.cmd_dirs(slug)
        entries.append(
            {
                "name": slug,
                "dir": f"skills/{slug}/cli",
                "module": registry.module_path(slug),
                "cli_cmd": cli_cmd,
                "mcp_cmd": mcp_cmd,
                "cli_bin": meta["cli_binary"],
                "mcp_bin": meta["mcp_binary"],
            }
        )
    return entries


def main(argv: list[str]) -> int:
    skills_only = "--skills-only" in argv
    matrix: dict = {"skill": skill_entries()}
    if not skills_only:
        matrix["target"] = registry.TARGETS
    print(json.dumps(matrix))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
