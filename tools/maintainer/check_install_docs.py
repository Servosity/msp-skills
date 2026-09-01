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

  R5  58 guide.md files said the installer "registers the skill with your agent",
      while the same 58 SKILL.md files said it does not. Neither installer writes
      any agent or MCP-client configuration: install.sh downloads two files into
      ~/.local/bin and chmod +x's them; install.ps1 downloads two files into
      %LOCALAPPDATA%\\Programs\\msp-skills and appends that directory to the user
      PATH. Nothing in either touches a `.claude` / `.codex` / `.cursor`
      directory, `claude_desktop_config.json`, an `mcp.json`, or an
      `mcpServers` block.

What it checks
--------------
Every rule compares doc prose against a SHIPPED artifact - main.go, install.ps1,
install.sh - never against another doc. A line carrying `install-docs:ignore` is
skipped, so a deliberate counter-example (this file's own prose, the docs-site
warning that names mcp-remote in order to tell you not to use it) can say the
banned string.

Scope notes, so the docstring and the code agree:

* R3 fires on a bare listener origin on ANY port and any loopback spelling
  (`localhost`, `127.0.0.1`, `0.0.0.0`), not only `localhost:7777` - the defect
  is "no path", not "one particular port". A URL that carries a real path
  (`/mcp`, `/sse`, ...) and a `cloudflared` origin argument are exempt.
* R2 reads logical lines: a shell command split across a trailing-backslash
  continuation is joined first, because multi-line launch commands are the
  established style in these files and a per-physical-line rule cannot see them.
* R4 fires on the npm-scoped spelling `@mvanhorn/printing-press-library`
  anywhere, and on the bare repo path `mvanhorn/printing-press-library` when the
  line also carries an install verb (`npx`, `go install`, `curl ... | sh`, ...).
  The bare path on its own is legitimate release-ledger prose - 63 of the 65
  AGENTS.md files carry it - so it is not banned outright.
* AGENTS.md is scanned too, for that reason: it is the surface where the bare
  path lives, and the gate has to be able to see it to tell the two apart.

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

DESCRIPTION = (__doc__ or "check install docs against the shipped artifacts").splitlines()[0]

SKILL_DOCS = ("README.md", "SKILL.md", "guide.md", "mcp-install.md", "AGENTS.md",
              "page.json")

# The stale Windows destination, built from fragments so this file can be
# grepped for the literal without matching itself.
BAD_WIN_DIR = "PrintingPress" + "\\bin"
# Both the npm-scoped spelling and the bare repo path, with the "-library"
# suffix OPTIONAL. Older prints emit "@mvanhorn/printing-press" without it -
# riverside-fm shipped exactly that shape and slipped past a literal
# "-library" match, which is the whole class this rule exists to catch.
#
# The negative lookbehind for "cli-" is load-bearing: "mvanhorn/cli-printing-press"
# is the GENERATOR, referenced legitimately in provenance text across the fleet,
# and must never be flagged as an install directive.
BAD_INSTALLER_RE = re.compile(r"@mvanhorn/printing-press(?:-library)?\b")
BAD_LIB_PATH_RE = re.compile(r"(?<!cli-)\bmvanhorn/printing-press(?:-library)?\b")

# Kept for the message text and the self-test fixtures.
BAD_INSTALLER = "@mvanhorn/printing-press-library"
BAD_LIB_PATH = "mvanhorn/printing-press-library"

# An install verb. Only with one of these does the bare library path become an
# instruction rather than a reference to where the release ledger lives.
INSTALL_VERB = re.compile(
    r"\b(?:npx|npm\s+(?:i|install)|pnpm\s+(?:dlx|add)|yarn\s+add|go\s+(?:install|get)|"
    r"pip\s+install|brew\s+install)\b|curl\b[^\n]*\|\s*(?:ba)?sh"
)

# A loopback origin with a port and no path. mcp-go serves Streamable HTTP at
# `/mcp` and 404s at the root, so an origin with nothing after it never
# handshakes - on 7777 or on any other port the operator picked.
BARE_LISTENER = re.compile(
    r"(?:https?://)?(?:localhost|127\.0\.0\.1|0\.0\.0\.0):\d{2,5}(?![\w.:-])(?!/\w)"
)

# The claim R5 polices. Only the affirmative "the installer does it" shapes: the
# corrected wording ("It does not register the skill with your agent") and the
# separate-steps wording ("Registering the skill with your agent ... is a
# separate step") are both true and must stay silent.
REGISTERS_CLAIM = re.compile(
    r"\bregisters\s+(?:the\s+skill|it|anything|the\s+MCP\s+server)\b[^\n]*?\bwith\s+your\s+agent\b"
    r"|\b(?:will|also|then)\s+registers?\s+(?:the\s+skill|it)\b[^\n]*?\bwith\s+your\s+agent\b",
    re.IGNORECASE,
)

# What an installer would have to touch for the R5 claim to be true.
AGENT_WIRING = (
    ".claude", ".codex", ".cursor", "claude_desktop_config", "mcp.json",
    "mcpServers", "claude mcp add", "skills install", "npx ",
)


def parses_transport(slug: str) -> bool | None:
    """Does this connector's MCP binary parse --transport? None = it has no MCP main."""
    mains = sorted((registry.skill_path(slug) / "cli" / "cmd").glob("*-mcp/main.go"))
    if not mains:
        return None
    return FLAG_MARKER in mains[0].read_text(encoding="utf-8")


def installer_registers(slug: str) -> bool:
    """Do this skill's shipped installers wire the skill into any agent?

    Read from install.sh / install.ps1, never from a doc. Today the answer is
    False for all 65: both scripts only download binaries and (on Windows)
    extend the user PATH. If an installer ever grows real agent wiring, this
    flips and R5 stops firing for that connector on its own.
    """
    for name in ("install.sh", "install.ps1"):
        p = registry.skill_path(slug) / name
        if not p.exists():
            continue
        text = p.read_text(encoding="utf-8")
        if any(marker in text for marker in AGENT_WIRING):
            return True
    return False


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


def logical_lines(text: str) -> list[tuple[int, str]]:
    """(1-indexed start line, text) with backslash-continued shell commands joined.

    Multi-line launch commands are the established style in mcp-install.md, so a
    rule that only ever sees one physical line at a time cannot see them.
    """
    out: list[tuple[int, str]] = []
    buf: list[str] = []
    start = 1
    for n, line in enumerate(text.splitlines(), 1):
        if not buf:
            start = n
        stripped = line.rstrip()
        if stripped.endswith("\\"):
            buf.append(stripped[:-1])
            continue
        buf.append(line)
        out.append((start, " ".join(s.strip() for s in buf) if len(buf) > 1 else buf[0]))
        buf = []
    if buf:
        out.append((start, " ".join(s.strip() for s in buf) if len(buf) > 1 else buf[0]))
    return out


def scan(files: list[Path], transports: dict[str, bool | None],
         registers: dict[str, bool] | None = None) -> list[str]:
    findings: list[str] = []
    registers = registers or {}
    bins = {slug: (entry.get("mcp_binary") or f"{slug}-mcp")
            for slug, entry in registry.skills().items()}
    # `<bin> ... --transport` on one logical line, with the binary name standing
    # alone so `claude mcp add --transport http <name> <url>` (a DIFFERENT tool's
    # flag) never matches.
    transport_pats = {
        slug: re.compile(rf"(?<![\w./-]){re.escape(b)}(?![\w.-])[^\n]*--transport")
        for slug, b in bins.items()
    }
    for fp in files:
        try:
            rel = fp.relative_to(ROOT).as_posix()
        except ValueError:
            # A file outside the repo: the selftest's hermetic temp dir.
            rel = fp.name
        slug = fp.parent.name if fp.parent.parent.name == "skills" else None
        # An installer that really did register would make the R5 claim true.
        # Off-skill files (the docs site, the root README) are judged against
        # the fleet: no installer anywhere registers.
        this_registers = registers.get(slug, False) if slug else any(registers.values())
        for n, line in logical_lines(fp.read_text(encoding="utf-8")):
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
            if BARE_LISTENER.search(line) and "cloudflared" not in line:
                findings.append(
                    f"R3 {rel}:{n}: points an operator at a loopback origin with no "
                    f"path. mcp-go serves Streamable HTTP at `/mcp` and 404s at the "
                    f"root, so that URL never handshakes."
                )
            if BAD_WIN_DIR in line:
                findings.append(
                    f"R4 {rel}:{n}: claims Windows binaries land in "
                    f"{BAD_WIN_DIR}. install.ps1 writes "
                    f"%LOCALAPPDATA%\\Programs\\msp-skills and creates no bin child."
                )
            if BAD_INSTALLER_RE.search(line) or (BAD_LIB_PATH_RE.search(line) and INSTALL_VERB.search(line)):
                findings.append(
                    f"R4 {rel}:{n}: routes the install through the printing-press "
                    f"library, "
                    f"which does not ship this repo's binaries. Use "
                    f"skills/{slug or '<slug>'}/install.sh / install.ps1."
                )
            if REGISTERS_CLAIM.search(line) and not this_registers:
                findings.append(
                    f"R5 {rel}:{n}: says the installer registers the skill with your "
                    f"agent. install.sh downloads two binaries into ~/.local/bin and "
                    f"install.ps1 downloads them into "
                    f"%LOCALAPPDATA%\\Programs\\msp-skills and extends the user PATH; "
                    f"neither writes any agent or MCP-client configuration. Say so, "
                    f"and point at mcp-install.md for the wire-up."
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
    """Prove each rule fires on a broken doc AND stays silent on the fixed one.

    Hermetic: every fixture is written inside a TemporaryDirectory and scan()
    reports it by basename, so a kill -9 mid-run cannot leave a stray .md in
    the repo for check_md_links / check_no_todos to trip over on the next run.
    """
    transports: dict[str, bool | None] = {"blumira": False, "auvik": True}
    registers: dict[str, bool] = {"auvik": False}
    cases = [
        ("R1", "For ChatGPT, expose it via the `mcp-remote` bridge.",
         "For ChatGPT, expose it via the `supergateway` bridge."),
        ("R2", "BLUMIRA_API_TOKEN=x blumira-mcp --transport http --addr :7777",
         'BLUMIRA_API_TOKEN=x npx -y supergateway --stdio "blumira-mcp" --port 7777'),
        # The same defect written across a shell continuation, which a
        # per-physical-line rule cannot see.
        ("R2", "BLUMIRA_API_TOKEN=x blumira-mcp \\\n  --transport http --addr :7777",
         'BLUMIRA_API_TOKEN=x npx -y supergateway \\\n  --stdio "blumira-mcp" --port 7777'),
        ("R3", "Then expose `http://localhost:7777` as a public HTTPS URL.",
         "Then expose `http://localhost:7777/mcp` as a public HTTPS URL."),
        # The same defect on another port, and spelled 127.0.0.1.
        ("R3", "Then expose `http://localhost:8080` as a public HTTPS URL.",
         "Then expose `http://localhost:8080/mcp` as a public HTTPS URL."),
        ("R3", "Then expose `http://127.0.0.1:7777` as a public HTTPS URL.",
         "Then expose `http://127.0.0.1:7777/mcp` as a public HTTPS URL."),
        ("R4", "and `%LOCALAPPDATA%\\" + BAD_WIN_DIR + "` on Windows:",
         "and `%LOCALAPPDATA%\\Programs\\msp-skills` on Windows:"),
        ("R4", "   " + "npx -y " + BAD_INSTALLER + " install auvik --cli-only",
         "   bash <(curl -fsSL https://example.invalid/skills/auvik/install.sh)"),
        # The go-install spelling of the same instruction, which names the bare
        # repo path rather than the npm scope.
        ("R4", "   go install github.com/" + BAD_LIB_PATH + "/auvik/cmd/auvik-cli@latest",
         "   bash <(curl -fsSL https://example.invalid/skills/auvik/install.sh)"),
        # The bare path with no install verb is release-ledger prose, not an
        # instruction: R4 must stay silent on it. Both halves are "fixed" here,
        # so the pair asserts the exemption rather than a fix.
        ("R4", "the release version is assigned after a publish PR merges in "
                "`" + BAD_LIB_PATH + "`, so do not hand-bump it. "
                + "npx -y " + BAD_INSTALLER,
         "the release version is assigned after a publish PR merges in "
         "`" + BAD_LIB_PATH + "`, so do not hand-bump it."),
        ("R5", "The installer places the binaries and registers the skill with your agent:",
         "The installer downloads the binaries. It does not register the skill with "
         "your agent and writes no MCP client config - see mcp-install.md."),
        ("R5", "The installer will register the skill with your agent for you.",
         "Registering the skill with your agent is a separate step - see "
         "mcp-install.md."),
    ]
    ok = True
    with tempfile.TemporaryDirectory() as tmp:
        for i, (rule, broken, fixed) in enumerate(cases):
            for label, text, want in (("broken", broken, True), ("fixed", fixed, False)):
                p = Path(tmp) / f"case{i}-{label}.md"
                p.write_text(text + "\n", encoding="utf-8")
                got = [f for f in scan([p], transports, registers) if f.startswith(rule)]
                fired = bool(got)
                mark = "PASS" if fired == want else "FAIL"
                if fired != want:
                    ok = False
                shown = text.replace("\n", " / ")[:72]
                print(f"  {mark} {rule} on {label:6s}: fired={fired} expected={want}"
                      f"  <- {shown}")
    # A rule that never fires is worthless; a rule that always fires is worse.
    # Both halves above are asserted, so this prints one line per direction.
    print("selftest:", "PASS" if ok else "FAIL")
    return 0 if ok else 1


def main() -> int:
    ap = argparse.ArgumentParser(description=DESCRIPTION)
    ap.add_argument("--selftest", action="store_true",
                    help="prove every rule fires on broken input and is silent on good")
    args = ap.parse_args()
    if args.selftest:
        return selftest()

    transports = {slug: parses_transport(slug) for slug in registry.skills()}
    registers = {slug: installer_registers(slug) for slug in registry.skills()}
    files = docs_files()
    findings = scan(files, transports, registers) + installer_destinations()
    if findings:
        print("check_install_docs FAILED:")
        for f in sorted(set(findings)):
            print(f"  - {f}")
        print(f"\n{len(set(findings))} finding(s). Every one is a doc describing an "
              f"install or transport path the shipped artifact does not provide.")
        return 1
    print(f"PASS: install and remote-MCP docs match the shipped binaries and "
          f"installers across {len(files)} file(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
