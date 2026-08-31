#!/usr/bin/env python3
"""Gate: the install and remote-MCP paths the docs describe must be ones that exist.

Why this exists
---------------
Four install surfaces per connector - README.md, SKILL.md, guide.md and
mcp-install.md - plus the docs site all described paths the shipped artifacts do
not provide. None of it could fail: every existing gate reads the CLI's *command*
surface (check_cli_claims), the manifest's *credentials* (check_env_schema) or the
*links* (check_md_links). Nothing read a doc's instruction against the binary or
the installer that has to satisfy it, so the fleet drifted together and stayed
wrong for as long as it took a human to try one.

Measured on the tree before this gate landed:

  R1  63 READMEs and 1 page.json told ChatGPT users to publish a local stdio MCP
      server with `mcp-remote`. mcp-remote bridges the OTHER direction - a remote
      HTTPS server down to a local stdio client - and dies immediately:
        $ npx -y mcp-remote --stdio "blumira-mcp" --port 7803
        Fatal error: TypeError: Invalid URL ... code: 'ERR_INVALID_URL',
        input: '--stdio'
      The package that publishes stdio over HTTP/SSE is `supergateway`.

  R2  5 connectors documented `<slug>-mcp --transport http`, a flag their binary
      never parses: cmd/<slug>-mcp/main.go calls server.ServeStdio directly, so
      the flag is inert and NO listener opens. Whether the flag exists is
      spec-driven, not press-version driven (auvik and avanan are both press
      4.30.2; one has it, one does not), so it must be read from main.go.

  R3  64 mcp-install.md pages told operators to expose bare
      `http://localhost:7777`. mcp-go serves Streamable HTTP at `/mcp`
      (server/streamable_http.go: endpointPath: "/mcp"), and no connector passes
      WithEndpointPath. Probed against a real betterstack-mcp: POST / -> 404,
      POST /mcp -> 200.

  R4  57 SKILL.md and 57 guide.md files routed the operator through
      `npx -y @mvanhorn/printing-press-library install <slug>`, and 56 of each
      claimed Windows binaries land in `%LOCALAPPDATA%\\Programs\\PrintingPress\\bin`.
      install.ps1 writes `$env:LOCALAPPDATA\\Programs\\msp-skills` and never
      creates a `bin` child, and install.sh writes `~/.local/bin`, not
      `$GOPATH/bin`. Those are the paths the shipped installers actually use, so
      they are what this gate reads.

What it checks
--------------
Every rule compares doc prose against a SHIPPED artifact - main.go, install.ps1,
install.sh - never against another doc. A line carrying `install-docs:ignore` is
skipped, so a deliberate counter-example (this file's own prose, the docs-site
warning that names mcp-remote in order to tell you not to use it) can say the
banned string.

Usage:
    python3 tools/maintainer/check_install_docs.py
    python3 tools/maintainer/check_install_docs.py --selftest
"""

from __future__ import annotations

import argparse
import re
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
IGNORE = "install-docs:ignore"
FLAG_MARKER = 'flag.String("transport"'

SKILL_DOCS = ("README.md", "SKILL.md", "guide.md", "mcp-install.md", "page.json")

# The stale Windows destination, built from fragments so this file can be
# grepped for the literal without matching itself.
BAD_WIN_DIR = "PrintingPress" + "\\bin"
BAD_INSTALLER = "@mvanhorn/printing-press-library"


def parses_transport(slug: str) -> bool | None:
    """Does this connector's MCP binary parse --transport? None = it has no MCP main."""
    mains = sorted((registry.skill_path(slug) / "cli" / "cmd").glob("*-mcp/main.go"))
    if not mains:
        return None
    return FLAG_MARKER in mains[0].read_text(encoding="utf-8")


def docs_files() -> list[Path]:
    """Every human-authored doc this gate reads: skill docs + the docs site."""
    out: list[Path] = []
    for slug in sorted(registry.skills()):
        for name in SKILL_DOCS:
            p = registry.skill_path(slug) / name
            if p.exists():
                out.append(p)
    out += sorted((ROOT / "docs").rglob("*.md"))
    out.append(ROOT / "README.md")
    return [p for p in out if p.exists()]


def scan(files: list[Path], transports: dict[str, bool | None]) -> list[str]:
    findings: list[str] = []
    bins = {slug: (entry.get("mcp_binary") or f"{slug}-mcp")
            for slug, entry in registry.skills().items()}
    # `<bin> ... --transport` on one line, with the binary name standing alone so
    # `claude mcp add --transport http <name> <url>` (a DIFFERENT tool's flag)
    # never matches.
    transport_pats = {
        slug: re.compile(rf"(?<![\w./-]){re.escape(b)}(?![\w.-])[^\n]*--transport")
        for slug, b in bins.items()
    }
    for fp in files:
        rel = fp.relative_to(ROOT).as_posix()
        slug = fp.parent.name if fp.parent.parent.name == "skills" else None
        for n, line in enumerate(fp.read_text(encoding="utf-8").splitlines(), 1):
            if IGNORE in line:
                continue
            if "mcp-remote" in line:
                findings.append(
                    f"R1 {rel}:{n}: names `mcp-remote` as the way to publish a local "
                    f"stdio MCP server over HTTP. It bridges the other direction and "
                    f"exits ERR_INVALID_URL on --stdio; use `supergateway`, or the "
                    f"binary's own --transport http where it has one."
                )
            for s, pat in transport_pats.items():
                if transports.get(s) is False and pat.search(line):
                    findings.append(
                        f"R2 {rel}:{n}: documents `{bins[s]} --transport ...`, but "
                        f"cmd/{bins[s]}/main.go calls server.ServeStdio with no flag "
                        f"parsing - the flag is inert and no listener opens. Bridge "
                        f"with `supergateway`, or give the spec an http transport and "
                        f"reprint."
                    )
            if re.search(r"localhost:7777(?![\w/])", line) and "/mcp" not in line \
                    and "/sse" not in line and "cloudflared" not in line:
                findings.append(
                    f"R3 {rel}:{n}: points an operator at bare "
                    f"`http://localhost:7777`. mcp-go serves Streamable HTTP at "
                    f"`/mcp` and 404s at the root, so that URL never handshakes."
                )
            if BAD_WIN_DIR in line:
                findings.append(
                    f"R4 {rel}:{n}: claims Windows binaries land in "
                    f"{BAD_WIN_DIR}. install.ps1 writes "
                    f"%LOCALAPPDATA%\\Programs\\msp-skills and creates no bin child."
                )
            if BAD_INSTALLER in line:
                findings.append(
                    f"R4 {rel}:{n}: routes the install through {BAD_INSTALLER}, "
                    f"which does not ship this repo's binaries. Use "
                    f"skills/{slug or '<slug>'}/install.sh / install.ps1."
                )
    return findings


def installer_destinations() -> list[str]:
    """Assert the destinations R4 asserts against are the ones the installers use."""
    findings: list[str] = []
    for slug in sorted(registry.skills()):
        ps1 = registry.skill_path(slug) / "install.ps1"
        sh = registry.skill_path(slug) / "install.sh"
        if ps1.exists() and "Programs\\msp-skills" not in ps1.read_text(encoding="utf-8"):
            findings.append(
                f"R4 skills/{slug}/install.ps1 no longer writes to "
                f"Programs\\msp-skills; this gate's Windows rule is now describing "
                f"a destination the installer does not use. Update both together."
            )
        if sh.exists() and ".local/bin" not in sh.read_text(encoding="utf-8"):
            findings.append(
                f"R4 skills/{slug}/install.sh no longer writes to ~/.local/bin; "
                f"this gate's POSIX rule is now describing a destination the "
                f"installer does not use. Update both together."
            )
    return findings


def selftest() -> int:
    """Prove each rule fires on a broken doc AND stays silent on the fixed one."""
    transports = {"blumira": False, "auvik": True}
    cases = [
        ("R1", "For ChatGPT, expose it via the `mcp-remote` bridge.",
         "For ChatGPT, expose it via the `supergateway` bridge."),
        ("R2", "BLUMIRA_API_TOKEN=x blumira-mcp --transport http --addr :7777",
         'BLUMIRA_API_TOKEN=x npx -y supergateway --stdio "blumira-mcp" --port 7777'),
        ("R3", "Then expose `http://localhost:7777` as a public HTTPS URL.",
         "Then expose `http://localhost:7777/mcp` as a public HTTPS URL."),
        ("R4", "and `%LOCALAPPDATA%\\" + BAD_WIN_DIR + "` on Windows:",
         "and `%LOCALAPPDATA%\\Programs\\msp-skills` on Windows:"),
        ("R4", "   " + "npx -y " + BAD_INSTALLER + " install auvik --cli-only",
         "   bash <(curl -fsSL https://example.invalid/skills/auvik/install.sh)"),
    ]
    ok = True
    with tempfile.TemporaryDirectory() as tmp:
        for i, (rule, broken, fixed) in enumerate(cases):
            for label, text, want in (("broken", broken, True), ("fixed", fixed, False)):
                p = Path(tmp) / f"case{i}-{label}.md"
                p.write_text(text + "\n", encoding="utf-8")
                # scan() reports paths relative to ROOT; feed it an absolute path
                # under ROOT so the relative_to() call holds.
                dst = ROOT / "docs" / f".selftest-{i}-{label}.md"
                dst.write_text(text + "\n", encoding="utf-8")
                try:
                    got = [f for f in scan([dst], transports) if f.startswith(rule)]
                finally:
                    dst.unlink()
                fired = bool(got)
                mark = "PASS" if fired == want else "FAIL"
                if fired != want:
                    ok = False
                print(f"  {mark} {rule} on {label:6s}: fired={fired} expected={want}"
                      f"  <- {text[:72]}")
    # A rule that never fires is worthless; a rule that always fires is worse.
    # Both halves above are asserted, so this prints one line per direction.
    print("selftest:", "PASS" if ok else "FAIL")
    return 0 if ok else 1


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--selftest", action="store_true",
                    help="prove every rule fires on broken input and is silent on good")
    args = ap.parse_args()
    if args.selftest:
        return selftest()

    transports = {slug: parses_transport(slug) for slug in registry.skills()}
    findings = scan(docs_files(), transports) + installer_destinations()
    if findings:
        print("check_install_docs FAILED:")
        for f in sorted(set(findings)):
            print(f"  - {f}")
        print(f"\n{len(set(findings))} finding(s). Every one is a doc describing an "
              f"install or transport path the shipped artifact does not provide.")
        return 1
    print(f"PASS: install and remote-MCP docs match the shipped binaries and "
          f"installers across {len(docs_files())} file(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
