"""Shared loader for tools/skills.json - the single source of truth for the
monorepo's skills. Imported by build-catalog.py, release_matrix.py, and
check_release_contract.py so per-skill metadata lives in exactly one place.

Per-skill metadata (system, status, vendor, binary names, first_party) is
declared in skills.json. Build-time facts (Go module path, cmd/ directory
names) are introspected from each skill's vendored cli/ tree so they stay
correct whether or not the source has been stripped of the -pp- brand.
"""

from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
SKILLS_DIR = ROOT / "skills"
REGISTRY = ROOT / "tools" / "maintainer" / "skills.json"

# os/arch build targets. Raw binaries are uploaded with these suffixes; the
# install scripts download exactly these names. Windows assets carry `.exe`.
TARGETS = [
    {"goos": "darwin", "goarch": "arm64"},
    {"goos": "darwin", "goarch": "amd64"},
    {"goos": "linux", "goarch": "arm64"},
    {"goos": "linux", "goarch": "amd64"},
    {"goos": "windows", "goarch": "amd64"},
    {"goos": "windows", "goarch": "arm64"},
]


def load() -> dict:
    return json.loads(REGISTRY.read_text())


def owner_repo() -> tuple[str, str]:
    reg = load()
    return reg["owner"], reg["repo"]


def skills() -> dict[str, dict]:
    """Return the skills map, ordered by slug, restricted to slugs that have a
    directory under skills/ (so a registry entry without a dir is reported)."""
    reg = load()
    return {slug: reg["skills"][slug] for slug in sorted(reg["skills"])}


def module_path(slug: str) -> str:
    """Read the Go module path from skills/<slug>/cli/go.mod."""
    gomod = SKILLS_DIR / slug / "cli" / "go.mod"
    for line in gomod.read_text().splitlines():
        if line.startswith("module "):
            return line.split(None, 1)[1].strip()
    raise SystemExit(f"{slug}: no module line in {gomod}")


def cmd_dirs(slug: str) -> tuple[str, str]:
    """Return (cli_cmd, mcp_cmd) relative paths under cli/, classified by the
    -mcp vs -cli suffix. Works for stripped (cmd/<slug>-cli) and unstripped
    (cmd/<slug>-pp-cli) layouts alike."""
    cmd_root = SKILLS_DIR / slug / "cli" / "cmd"
    dirs = sorted(p.name for p in cmd_root.iterdir() if p.is_dir())
    cli = next((d for d in dirs if d.endswith("cli")), None)
    mcp = next((d for d in dirs if d.endswith("mcp")), None)
    if not cli or not mcp:
        raise SystemExit(f"{slug}: expected one *-cli and one *-mcp dir under {cmd_root}, found {dirs}")
    return f"./cmd/{cli}", f"./cmd/{mcp}"
