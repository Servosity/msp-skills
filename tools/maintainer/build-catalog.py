#!/usr/bin/env python3
"""Regenerate catalog.json and the README catalog table from each skill's manifest.json.

This script is the single source of truth for the catalog. CI runs it on every PR
that touches `skills/` and fails if the committed catalog drifts from what this
script would produce.

Run locally:
    python3 tools/build-catalog.py
"""

from __future__ import annotations

import datetime as dt
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = Path(__file__).resolve().parent.parent.parent
SKILLS_DIR = ROOT / "skills"
README = ROOT / "README.md"
CATALOG = ROOT / "catalog.json"

# Owner/repo and per-skill metadata are the single source of truth in
# tools/skills.json (loaded via registry). Every install URL is built from
# these, so the catalog can never drift back to an unresolved owner token; CI
# fails the build if one is reintroduced anywhere in the tree.
OWNER, REPO = registry.owner_repo()
SKILL_META = registry.load()["skills"]


def install_url(skill: str, script: str) -> str:
    return (
        f"bash <(curl -fsSL https://raw.githubusercontent.com/{OWNER}/"
        f"{REPO}/main/skills/{skill}/{script})"
    )


def build_entry(skill_dir: Path) -> dict:
    name = skill_dir.name
    manifest_path = skill_dir / "manifest.json"
    if not manifest_path.exists():
        raise SystemExit(f"missing manifest.json for skill {name}")
    manifest = json.loads(manifest_path.read_text())

    meta = SKILL_META.get(name)
    if meta is None:
        raise SystemExit(
            f"new skill '{name}' has no entry in tools/skills.json. "
            "Add one before the catalog can be regenerated."
        )

    return {
        "name": name,
        "system": meta["system"],
        "status": meta["status"],
        "skill_path": f"skills/{name}",
        "cli_binary": meta["cli_binary"],
        "mcp_binary": meta["mcp_binary"],
        "version": manifest.get("version", "0.0.0"),
        "license": manifest.get("license", "Apache-2.0"),
        "vendor": meta["vendor"],
        "vendor_trademark_owner": meta["vendor_trademark_owner"],
        "first_party": meta["first_party"],
        "install_skill_one_liner": install_url(name, "install.sh"),
        "install_mcp_doc": f"skills/{name}/mcp-install.md",
        "description": manifest.get("description", ""),
    }


def status_badge(status: str) -> str:
    """Map a skill's status to a shields.io badge URL.

    'tested' / 'beta' (the launch state for halopsa + servosity, where MSPs
    have run the skill against a real production tenant): green Tested badge.
    Anything else ('untested', etc.) for a skill that shipped but has not
    been driven against live data yet: yellow Untested badge. Feedback on
    untested skills is the high-leverage signal we want from MSPs.
    """
    if status in ("tested", "beta"):
        return "![Tested](https://img.shields.io/badge/Tested-by_MSPs-2E7D32)"
    return "![Untested](https://img.shields.io/badge/Untested-feedback_welcome-EAB308)"


def render_catalog_table(skills: list[dict]) -> str:
    rows = [
        "| Skill | System | Status | Install |",
        "| --- | --- | --- | --- |",
    ]
    for s in skills:
        rows.append(
            f"| [{s['name']}](./skills/{s['name']}) | {s['system']} | "
            f"{status_badge(s['status'])} | "
            f"[Install](./skills/{s['name']}/README.md) |"
        )
    return "\n".join(rows)


def replace_block(content: str, marker_start: str, marker_end: str, block: str) -> str:
    pattern = re.compile(
        rf"({re.escape(marker_start)})(.*?)({re.escape(marker_end)})",
        re.DOTALL,
    )
    if not pattern.search(content):
        raise SystemExit(
            f"README is missing the {marker_start} / {marker_end} markers; "
            "cannot regenerate catalog table."
        )
    return pattern.sub(rf"\1\n{block}\n\3", content)


def main() -> int:
    skill_dirs = sorted(p for p in SKILLS_DIR.iterdir() if p.is_dir())
    skills = [build_entry(d) for d in skill_dirs]

    catalog = {
        "schema_version": 1,
        "generated_at": dt.date.today().isoformat(),
        "skills": skills,
    }
    CATALOG.write_text(json.dumps(catalog, indent=2) + "\n")

    readme = README.read_text()
    new_readme = replace_block(
        readme,
        "<!-- catalog:start -->",
        "<!-- catalog:end -->",
        render_catalog_table(skills),
    )
    README.write_text(new_readme)

    print(f"Regenerated catalog.json ({len(skills)} skills) and README catalog table.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
