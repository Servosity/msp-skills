#!/usr/bin/env python3
"""Assert the install scripts and the release workflow agree on asset names.

This is the gate that guarantees a user's `curl | bash` one-liner resolves to a
real GitHub Release asset. For every skill it cross-checks three things:

  1. install.sh CLI_BIN/MCP_BIN  ==  install.ps1 CliBin/McpBin (minus .exe)
     ==  tools/skills.json cli_binary/mcp_binary.
  2. The set of asset names the install scripts will DOWNLOAD
     (CLI_BIN/MCP_BIN x the os/arch each script covers).
  3. The set of asset names the release workflow will PRODUCE
     (release_matrix.py: cli_bin/mcp_bin x TARGETS, .exe on windows).
  ... and asserts set (2) == set (3) exactly.

Run locally:  python3 tools/check_release_contract.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR

# Coverage each installer is responsible for (mirrors the scripts themselves).
SH_TARGETS = [("darwin", "arm64"), ("darwin", "amd64"), ("linux", "arm64"), ("linux", "amd64")]
PS_TARGETS = [("windows", "amd64"), ("windows", "arm64")]


def _sh_var(text: str, name: str) -> str | None:
    m = re.search(rf'^{name}="([^"]+)"', text, re.MULTILINE)
    return m.group(1) if m else None


def _ps_var(text: str, name: str) -> str | None:
    m = re.search(rf'\${name}\s*=\s*"([^"]+)"', text)
    return m.group(1) if m else None


def install_expected_assets(cli_bin: str, mcp_bin: str) -> set[str]:
    assets = set()
    for bin_ in (cli_bin, mcp_bin):
        for os_, arch in SH_TARGETS:
            assets.add(f"{bin_}-{os_}-{arch}")
        for os_, arch in PS_TARGETS:
            assets.add(f"{bin_}-{os_}-{arch}.exe")
    return assets


def release_produced_assets(cli_bin: str, mcp_bin: str) -> set[str]:
    assets = set()
    for bin_ in (cli_bin, mcp_bin):
        for t in registry.TARGETS:
            ext = ".exe" if t["goos"] == "windows" else ""
            assets.add(f"{bin_}-{t['goos']}-{t['goarch']}{ext}")
    return assets


def main() -> int:
    errors: list[str] = []
    meta = registry.skills()

    checked = 0
    for slug, m in meta.items():
        # markdown-only skills have no install scripts or binaries to cross-check.
        if registry.is_markdown_only(slug):
            continue
        checked += 1
        d = registry.skill_path(slug)
        sh = (d / "install.sh").read_text()
        ps = (d / "install.ps1").read_text()

        sh_cli, sh_mcp = _sh_var(sh, "CLI_BIN"), _sh_var(sh, "MCP_BIN")
        ps_cli = (_ps_var(ps, "CliBin") or "").removesuffix(".exe")
        ps_mcp = (_ps_var(ps, "McpBin") or "").removesuffix(".exe")
        reg_cli, reg_mcp = m["cli_binary"], m["mcp_binary"]

        # (1) names agree across install.sh, install.ps1, and the registry.
        for label, got, want in (
            ("install.sh CLI_BIN", sh_cli, reg_cli),
            ("install.sh MCP_BIN", sh_mcp, reg_mcp),
            ("install.ps1 CliBin", ps_cli, reg_cli),
            ("install.ps1 McpBin", ps_mcp, reg_mcp),
        ):
            if got != want:
                errors.append(f"{slug}: {label} is {got!r}, expected {want!r} (from skills.json)")

        # (2) == (3): the assets installers fetch == the assets release builds.
        expected = install_expected_assets(reg_cli, reg_mcp)
        produced = release_produced_assets(reg_cli, reg_mcp)
        missing = expected - produced
        extra = produced - expected
        if missing:
            errors.append(f"{slug}: installers request assets the release won't produce: {sorted(missing)}")
        if extra:
            errors.append(f"{slug}: release produces assets no installer fetches: {sorted(extra)}")

    if errors:
        print("Release contract check FAILED:")
        for e in errors:
            print(f"  - {e}")
        return 1

    print(f"Release contract check passed for {checked} skill(s): install scripts and release assets agree.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
