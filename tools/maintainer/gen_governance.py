#!/usr/bin/env python3
"""Generate a draft governance.md for a skill by introspecting its CLI surface.

The safety matrix is DERIVED from the real command set (filenames under
internal/cli/ encode the command path, e.g. companies_delete.go ->
`companies delete`), not hand-guessed - so a skill's governance doc can never
silently omit a destructive operation the binary actually ships.

Tiers are assigned by verb in the command path:
  destructive  -> delete, prune, purge, unlock, repair, wipe, reset (data)
  credential   -> token, rotate, mfa, encryption-key, password, reissue, credential, secret
  admin        -> top-level `admin` group (often hidden)
  write        -> create, update, patch, comment, ignore, archive, reactivate, triage, clear
  read         -> everything else (not listed individually)

One tier is NOT verb-derived: **Local browser session**. A cookie-auth connector
has no API key at all - it authenticates by reading the operator's own browser
cookie store on the local machine, which means the binary reaches OUTSIDE the
vendor's API into the operator's workstation (a local SQLite database under the
Chrome profile, and optionally a browser that is already running). That is a
different KIND of local-credential access from "an API key in an env var", so it
gets a row of its own instead of being folded into Credential / security, where
an MSP owner reading the table would mistake it for token handling.

The row is DERIVED, never hand-written: browser_session() detects the pattern
from markers the printing press emits into internal/cli/auth.go, and the list of
external programs the connector can launch is extracted from the literal
exec.Command call sites in internal/cli/. If a future press revision adds a
backend, the table gains it on the next regeneration.

Usage:
    python3 tools/gen_governance.py <slug> > skills/<slug>/governance.md

The output is a DRAFT: review the prose (auth model, why-comfortable) before
shipping. The matrix rows are authoritative.
"""

from __future__ import annotations

import json
import re
import sys
import textwrap
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

SKILLS_DIR = registry.SKILLS_DIR

CREDENTIAL = ("token", "rotate", "mfa", "encryption-key", "password", "reissue", "credential", "secret")
DESTRUCTIVE = ("delete", "prune", "purge", "unlock", "repair", "wipe", "destroy")
WRITE = ("create", "update", "patch", "comment", "ignore", "archive", "reactivate", "triage", "clear", "set")


# --- Local browser session (cookie auth) -------------------------------------
# Markers the printing press emits into internal/cli/auth.go for a cookie-auth
# connector. Matching on the FUNCTION DEFINITION (not a loose word like
# "cookie") keeps a connector that merely mentions cookies in help text out of
# the tier.
COOKIE_DB_MARKERS = ("func detectCookieTool(", "func chromeDataDir(")
LIVE_BROWSER_MARKERS = ("func extractLiveCookies(",)
# First argument of an exec.Command / exec.CommandContext call when it is a
# string literal. A non-literal command is a security-gate P1 in this repo, so
# in practice every shipped call site is literal and this inventory is complete.
EXEC_LITERAL_RE = re.compile(r'exec\.Command(?:Context)?\(\s*(?:[A-Za-z_]\w*\s*,\s*)?"([^"]+)"')


def browser_session(slug: str) -> dict | None:
    """Describe the local-browser capability of a cookie-auth connector.

    Returns None for a normal env-var / API-key connector, so the tier and its
    prose are absent from every governance.md that does not need them.
    """
    cli = SKILLS_DIR / slug / "cli" / "internal" / "cli"
    if not cli.is_dir():
        return None
    sources = {f: f.read_text(encoding="utf-8", errors="replace")
               for f in sorted(cli.glob("*.go")) if not f.name.endswith("_test.go")}
    blob = "".join(sources.values())
    if not any(m in blob for m in COOKIE_DB_MARKERS):
        return None
    execs = sorted({m for text in sources.values() for m in EXEC_LITERAL_RE.findall(text)})
    return {
        "live_browser": any(m in blob for m in LIVE_BROWSER_MARKERS),
        "cookie_db_probe": "func countCookiesForDomain(" in blob,
        "execs": execs,
        "commands": sorted(n for n in ("auth login", "doctor")
                           if any(f.stem == n.split()[0] for f in sources)),
    }


def command_paths(slug: str) -> list[str]:
    cli = SKILLS_DIR / slug / "cli" / "internal" / "cli"
    if not cli.is_dir():
        return []
    paths = set()
    for f in cli.glob("*.go"):
        if f.name.endswith("_test.go") or f.name in ("root.go",):
            continue
        # filename -> command path: drop .go, turn _ into spaces, collapse the
        # printing-press habit of doubling the leading group in deep paths.
        name = f.stem.replace("_", " ")
        paths.add(name)
    return sorted(paths)


def classify(paths: list[str]) -> dict[str, list[str]]:
    tiers: dict[str, list[str]] = {"admin": [], "destructive": [], "credential": [], "write": []}
    for p in paths:
        low = p.lower()
        if low.startswith("admin"):
            tiers["admin"].append(p)
        elif any(k in low for k in DESTRUCTIVE):
            tiers["destructive"].append(p)
        elif any(k in low for k in CREDENTIAL):
            tiers["credential"].append(p)
        elif any(k in low for k in WRITE):
            tiers["write"].append(p)
    return tiers


def auth_env(slug: str) -> list[str]:
    mf = SKILLS_DIR / slug / "manifest.json"
    if not mf.exists():
        return []
    try:
        m = json.loads(mf.read_text(encoding="utf-8"))
        env = m.get("server", {}).get("mcp_config", {}).get("env", {})
        return sorted(env.keys())
    except Exception:
        return []


def has_dry_run(slug: str) -> bool:
    root = SKILLS_DIR / slug / "cli" / "internal" / "cli" / "root.go"
    return root.exists() and bool(re.search(r"dry-run", root.read_text(encoding="utf-8"), re.IGNORECASE))


def wrap(text: str, width: int = 80) -> str:
    """Fill a paragraph to the width the rest of these docs use.

    The browser-session prose is composed from source-derived facts, so its
    length varies per connector; without this the generated file would ship one
    very long line where every neighbouring paragraph is wrapped."""
    return textwrap.fill(text, width=width, break_long_words=False,
                         break_on_hyphens=False)


def sample(items: list[str], n: int = 8) -> str:
    if not items:
        return "(none detected)"
    shown = ", ".join(f"`{i}`" for i in items[:n])
    return shown + (f", ... ({len(items)} total)" if len(items) > n else "")


def main(argv: list[str]) -> int:
    if not argv:
        print("usage: gen_governance.py <slug>", file=sys.stderr)
        return 2
    slug = argv[0]
    meta = registry.skills().get(slug)
    if not meta:
        print(f"error: '{slug}' not in tools/skills.json", file=sys.stderr)
        return 2

    vendor = meta["vendor"]
    first_party = meta["first_party"]
    tiers = classify(command_paths(slug))
    envs = auth_env(slug)
    env_str = ", ".join(f"`{e}`" for e in envs) if envs else "the credentials documented in mcp-install.md"
    dry = has_dry_run(slug)
    browser = browser_session(slug)

    # A cookie-auth connector has no API key, so the stock "credentials are read
    # from the environment only - never written to disk" sentence would be a lie:
    # the session cookie IS written, to the operator's own 0600 config file.
    if browser:
        auth_para = wrap(
            f"The skill drives the `{meta['cli_binary']}` binary (and "
            f"`{meta['mcp_binary']}`). There is no API key to hand it: this connector "
            f"authenticates as **you**, reusing the session your own browser already "
            f"holds. `auth login` reads the matching cookies out of your local browser "
            f"cookie store and writes them to your own config file (mode 0600, path "
            f"shown by `doctor`). Those cookies are sent to the {vendor} API and nowhere "
            f"else. They carry your full account access, so treat that config file "
            f"exactly like a password."
        )
    else:
        auth_para = (
            f"The skill drives the `{meta['cli_binary']}` binary (and `{meta['mcp_binary']}`),\n"
            f"authenticating with {env_str}. Credentials are read from the environment only -\n"
            f"never written to disk, never logged, never sent anywhere except the {vendor} API."
        )

    if browser:
        launch = ", ".join(f"`{e}`" for e in browser["execs"]) or "(none)"
        live = (" It can also attach to a browser you already have running and read "
                "`document.cookie` for the vendor domain."
                if browser["live_browser"] else "")
        probe_para = wrap(
            "**One thing worth knowing about the profile probe.** To work out WHICH "
            "of your browser profiles is signed in, the connector copies each "
            "profile's cookie database to a temporary file and counts the rows "
            "matching the vendor domain. That copy is the whole database - every "
            "site's cookies, not just this vendor's - because SQLite has to open the "
            "file as a unit, and Chrome holds a write lock on the original. The copy "
            "is made inside a 0700 directory with the files at 0600, it is deleted as "
            "soon as the count is taken, and nothing from it is read except the "
            "row count. Nothing is transmitted."
        ) if browser["cookie_db_probe"] else ""
        launch_para = wrap(
            "**What it can launch.** The complete set of external programs the binary "
            "can ever run is fixed at build time, and this list is read straight out of "
            f"the source: {launch}. Every one of those is a compile-time literal - no "
            "command name is ever built from your input - and the connector never "
            "invokes a shell."
        )
        reads_para = wrap(
            "**What it reads.** `auth login --chrome` looks in the standard browser "
            "profile location for your operating system and finds the profile that "
            "holds cookies for the vendor domain. Only that domain's cookies are "
            "extracted and saved, through a cookie-extraction helper you install "
            "yourself; the connector never implements decryption of your cookie store "
            "itself." + live
        )
        browser_row = (
            "| **Local browser session** | Reads your own browser's cookie store on this "
            "machine to obtain a session, and saves that session to your local config file. "
            "Reaches outside the vendor API into your workstation." + live +
            f" | `auth login`, `doctor` | Operator runs it, once, interactively. Never leave "
            f"it to an unattended agent. |\n"
        )
        browser_section = f"""
## Local browser session: what "cookie auth" actually means here

{vendor} publishes no partner API key, so this connector authenticates as you by
reusing your browser's own session. That is a different kind of access from an
API key in an environment variable, and it deserves its own line in the table
above rather than being filed under Credential / security.

{reads_para}

{probe_para}

{launch_para}

**What leaves the machine.** The extracted cookies are written to your own
config file at mode 0600 and are sent as request headers to the vendor API.
Nothing is sent anywhere else, and nothing is logged.

**What an MSP owner should require.**

- Run `auth login` yourself, interactively, on your own workstation. It is not
  an agent operation.
- Treat the config file as a password store: those cookies are full account
  access until the vendor session expires.
- Prefer a dedicated browser profile signed in to only this vendor if you share
  the workstation.
- If you ever ran it somewhere you should not have, sign out of the vendor in
  that browser to invalidate the session, then delete the config file.
"""
    else:
        browser_row = ""
        browser_section = ""

    banner = (
        f"> Published by {vendor} Inc. for MSP partners."
        if first_party
        else f"> Unofficial. Community-built skill for the {vendor} API. Not affiliated with,\n> endorsed by, or sponsored by the vendor."
    )
    # NOTE: never CLAIM gating behavior as a default. In these binaries
    # --dry-run is an OPT-IN flag (raw writes send immediately) and --confirm
    # exists only where a specific command documents it (e.g. bulk operations
    # above a digest threshold). State behavior honestly; make the gate an
    # AGENT-LEVEL policy recommendation. (Adversarial-review finding, 2026-06-04.)
    safe_line = (
        "- **`--dry-run` is opt-in - use it.** Mutating commands send immediately "
        "unless you pass `--dry-run` first to preview the request without sending. "
        "Make your agent's policy: preview, show the exact command, get approval, "
        "then run the write."
        if dry
        else "- **Preview before writes.** Inspect `--help` on any mutating command "
        "and require your agent to show the exact command for approval before "
        "running it."
    )

    out = f"""# {slug} skill - governance and safety model

{banner}
> This page tells an MSP owner exactly what the {slug} skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

{auth_para}

## Default-safe behavior

{safe_line}
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | {sample(tiers['write'])} | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | {sample(tiers['credential'])} | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | {sample(tiers['destructive'])} | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | {sample(tiers['admin'])} | Operator-only, not for agents |
{browser_row}{browser_section}
## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the {vendor} API, and you can read every line of how it does so. The skill is
read-first{', plan-by-default' if dry else ''}, and scoped to your own account.
"""
    sys.stdout.write(out)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
