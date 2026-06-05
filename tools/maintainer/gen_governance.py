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

Usage:
    python3 tools/gen_governance.py <slug> > skills/<slug>/governance.md

The output is a DRAFT: review the prose (auth model, why-comfortable) before
shipping. The matrix rows are authoritative.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

SKILLS_DIR = registry.SKILLS_DIR

CREDENTIAL = ("token", "rotate", "mfa", "encryption-key", "password", "reissue", "credential", "secret")
DESTRUCTIVE = ("delete", "prune", "purge", "unlock", "repair", "wipe", "destroy")
WRITE = ("create", "update", "patch", "comment", "ignore", "archive", "reactivate", "triage", "clear", "set")


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
        m = json.loads(mf.read_text())
        env = m.get("server", {}).get("mcp_config", {}).get("env", {})
        return sorted(env.keys())
    except Exception:
        return []


def has_dry_run(slug: str) -> bool:
    root = SKILLS_DIR / slug / "cli" / "internal" / "cli" / "root.go"
    return root.exists() and bool(re.search(r"dry-run", root.read_text(), re.IGNORECASE))


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

The skill drives the `{meta['cli_binary']}` binary (and `{meta['mcp_binary']}`),
authenticating with {env_str}. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the {vendor} API.

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
