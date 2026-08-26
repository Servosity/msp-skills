#!/usr/bin/env python3
"""Assert the install scripts and the release workflow agree on asset names.

This is the gate that guarantees a user's `curl | bash` one-liner resolves to a
real GitHub Release asset. For every skill it cross-checks:

  1. install.sh CLI_BIN/MCP_BIN  ==  install.ps1 CliBin/McpBin (minus .exe)
     ==  tools/maintainer/skills.json cli_binary/mcp_binary.
  2. The asset names the install scripts will DOWNLOAD, RENDERED FROM THE
     SCRIPTS' OWN URL TEMPLATES (install.sh `cli_url=`/`mcp_url=`, install.ps1
     `$cliUrl`/`$mcpUrl`) with the os/arch each script covers substituted in.
  3. The asset names the release workflow will PRODUCE - read from the SAME
     shared source the workflow itself consumes: registry.asset_map(), which
     release_matrix.py embeds into the build matrix as literal filenames.
  ... and asserts set (2) == set (3) exactly, per skill.

Because (3) now comes from the one shared function rather than a second hand-
written copy of the "-<goos>-<goarch>" + windows ".exe" rule, a workflow that
disagrees with this gate is no longer possible: both read the same names. What
the gate still independently proves is that the shipped install scripts - real
text, parsed here, not a mirror of it - resolve to those exact names.

Run locally:  python3 tools/maintainer/check_release_contract.py
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


# The asset-name templates the shipped installers actually use. We parse these
# out of the real script text rather than re-declaring the naming rule, so an
# edit to an installer's URL line is caught instead of silently agreeing with a
# copy of itself.
SH_URL_RE = {
    "cli": re.compile(r'^cli_url="\$\{RELEASE_BASE\}/(?P<t>[^"]+)"', re.MULTILINE),
    "mcp": re.compile(r'^mcp_url="\$\{RELEASE_BASE\}/(?P<t>[^"]+)"', re.MULTILINE),
}
PS_URL_RE = {
    "cli": re.compile(r'^\$cliUrl\s*=\s*"\$ReleaseBase/(?P<t>[^"]+)"', re.MULTILINE),
    "mcp": re.compile(r'^\$mcpUrl\s*=\s*"\$ReleaseBase/(?P<t>[^"]+)"', re.MULTILINE),
}


# Only the variable belonging to THIS kind is substituted, so a cli_url line
# that mistakenly interpolates ${MCP_BIN} leaves an unresolved "$" and is
# reported instead of quietly rendering the right-looking name.
SH_BIN_VAR = {"cli": "${CLI_BIN}", "mcp": "${MCP_BIN}"}
PS_BIN_VAR = {"cli": "$($CliBin.Replace('.exe',''))",
              "mcp": "$($McpBin.Replace('.exe',''))"}


def _render_sh(template: str, kind: str, binary: str, os_: str, arch: str) -> str:
    """Render an install.sh URL-tail template for one target."""
    return (template
            .replace(SH_BIN_VAR[kind], binary)
            .replace("${os}", os_)
            .replace("${arch}", arch))


def _render_ps(template: str, kind: str, binary: str, arch: str) -> str:
    """Render an install.ps1 URL-tail template for one target.

    `$CliBin`/`$McpBin` carry the .exe suffix in PowerShell, and the script
    strips it with .Replace('.exe','') before appending the target suffix; the
    `binary` passed here is already the stripped registry name.
    """
    return (template
            .replace(PS_BIN_VAR[kind], binary)
            .replace("$arch", arch))


def install_expected_assets(sh: str, ps: str, cli_bin: str, mcp_bin: str) -> tuple[set[str], list[str]]:
    """Asset names the installers will fetch, rendered from their own templates.

    Returns (names, problems); a problem is a missing template or one that still
    has an unresolved variable after substitution.
    """
    assets: set[str] = set()
    problems: list[str] = []

    for kind, binary in (("cli", cli_bin), ("mcp", mcp_bin)):
        m = SH_URL_RE[kind].search(sh)
        if not m:
            problems.append(f"install.sh: no {kind}_url=\"${{RELEASE_BASE}}/...\" line found")
        else:
            for os_, arch in SH_TARGETS:
                name = _render_sh(m.group("t"), kind, binary, os_, arch)
                if "$" in name:
                    problems.append(f"install.sh {kind}_url template leaves an unresolved variable: {name!r}")
                assets.add(name)

        m = PS_URL_RE[kind].search(ps)
        if not m:
            problems.append(f"install.ps1: no ${kind}Url = \"$ReleaseBase/...\" line found")
        else:
            for _os, arch in PS_TARGETS:
                name = _render_ps(m.group("t"), kind, binary, arch)
                if "$" in name:
                    problems.append(f"install.ps1 ${kind}Url template leaves an unresolved variable: {name!r}")
                assets.add(name)

    return assets, problems


def release_produced_assets(cli_bin: str, mcp_bin: str) -> set[str]:
    """Asset names the release will upload - the SAME shared function the build
    matrix (and therefore release.yml) resolves its upload filenames through."""
    return {name
            for per_target in registry.asset_map(cli_bin, mcp_bin).values()
            for name in per_target.values()}


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
        sh = (d / "install.sh").read_text(encoding="utf-8")
        ps = (d / "install.ps1").read_text(encoding="utf-8")

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
        expected, problems = install_expected_assets(sh, ps, reg_cli, reg_mcp)
        for p in problems:
            errors.append(f"{slug}: {p}")
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
