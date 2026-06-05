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


def _plugin_version(skill_dir: Path) -> str:
    """Read the version from .claude-plugin/plugin.json (markdown-only fallback)."""
    pj = skill_dir / ".claude-plugin" / "plugin.json"
    if pj.exists():
        try:
            return json.loads(pj.read_text()).get("version", "0.0.0")
        except (OSError, ValueError):
            return "0.0.0"
    return "0.0.0"


def build_entry(skill_dir: Path) -> dict:
    dir_name = skill_dir.name
    slug = registry.slug_for_dir(dir_name)

    meta = SKILL_META.get(slug)
    if meta is None:
        raise SystemExit(
            f"new skill '{slug}' (skills/{dir_name}) has no entry in "
            "tools/skills.json. Add one before the catalog can be regenerated."
        )

    markdown_only = bool(meta.get("markdown_only"))

    # markdown-only skills have no manifest.json or binaries; version comes from
    # .claude-plugin/plugin.json and the binary fields are null. Binary skills
    # keep their manifest.json as the version + description source of truth.
    if markdown_only:
        manifest = {}
        version = _plugin_version(skill_dir)
    else:
        manifest_path = skill_dir / "manifest.json"
        if not manifest_path.exists():
            raise SystemExit(f"missing manifest.json for skill {slug}")
        manifest = json.loads(manifest_path.read_text())
        version = manifest.get("version", "0.0.0")

    entry = {
        "name": slug,
        "system": meta["system"],
        "status": meta["status"],
        "skill_path": f"skills/{dir_name}",
        "cli_binary": meta.get("cli_binary"),
        "mcp_binary": meta.get("mcp_binary"),
        "version": version,
        "license": manifest.get("license", "Apache-2.0"),
        "vendor": meta["vendor"],
        "vendor_trademark_owner": meta["vendor_trademark_owner"],
        "first_party": meta["first_party"],
        "install_skill_one_liner": (
            "" if markdown_only else install_url(dir_name, "install.sh")
        ),
        "install_mcp_doc": (
            "" if markdown_only else f"skills/{dir_name}/mcp-install.md"
        ),
        "description": manifest.get("description", meta.get("tagline", "")),
        "verification": _verification_state(meta, markdown_only),
    }
    if meta.get("category"):
        entry["category"] = meta["category"]
    if meta.get("tagline"):
        entry["tagline"] = meta["tagline"]
    # Only stamp the flag when true so binary skills' catalog entries stay
    # byte-identical (the CI drift gate compares the catalog exactly).
    if markdown_only:
        entry["markdown_only"] = True
    return entry


def _verification_state(meta: dict, markdown_only: bool) -> str:
    """The honest badge state: 'live-verified' only when a real MSP confirmed
    the skill against a live tenant (verify_live.py flipped it); 'awaiting'
    otherwise; 'n/a' for markdown-only skills (no tenant to verify)."""
    if markdown_only:
        return "n/a"
    lv = meta.get("live_verified") or {}
    return "live-verified" if lv.get("status") == "live-verified" else "awaiting"


def status_badge(verification: str) -> str:
    """Map a skill's VERIFICATION state to a shields.io badge URL.

    'live-verified' (a real MSP confirmed it against a live tenant; flipped
    only by verify_live.py with a date + source + evidence): green badge.
    'awaiting' (passes every mechanical gate; not yet confirmed live): amber
    badge framed as an invitation - the report is the high-leverage signal.
    'n/a' (markdown-only, no tenant to verify): neutral badge.
    """
    if verification == "live-verified":
        return "![Live-verified](https://img.shields.io/badge/Live--verified-by_a_real_MSP-2E7D32)"
    if verification == "n/a":
        return "![Meta](https://img.shields.io/badge/Meta-skill-6B7280)"
    return "![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308)"


def render_catalog_table(skills: list[dict]) -> str:
    rows = [
        "| Skill | System | Status | Install |",
        "| --- | --- | --- | --- |",
    ]
    for s in skills:
        # Links resolve to the on-disk directory (skill_path), which differs from
        # the slug for a markdown-only skill (msp-skills-concierge -> skills/_meta).
        path = s["skill_path"]
        install = (
            "Marketplace"
            if s.get("markdown_only")
            else "Install"
        )
        rows.append(
            f"| [{s['name']}](./{path}) | {s['system']} | "
            f"{status_badge(s.get('verification', 'awaiting'))} | "
            f"[{install}](./{path}/README.md) |"
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

    # Preserve the existing generated_at when the substantive content is
    # unchanged, so re-running build-catalog.py is a TRUE no-op. The CI drift
    # gate (catalog.yml) compares the regenerated file byte-for-byte; a live
    # `dt.date.today()` stamp made every PR fail on any day other than the one
    # the catalog was last committed. Bump the date only when skills change.
    generated_at = dt.date.today().isoformat()
    if CATALOG.exists():
        try:
            prev = json.loads(CATALOG.read_text())
            if prev.get("schema_version") == 1 and prev.get("skills") == skills:
                generated_at = prev.get("generated_at", generated_at)
        except Exception:
            pass

    catalog = {
        "schema_version": 1,
        "generated_at": generated_at,
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
