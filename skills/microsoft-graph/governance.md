# microsoft-graph skill - governance and safety model

> Unofficial. Community-built skill for the Microsoft Graph API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the microsoft-graph skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `microsoft-graph-cli` binary (and `microsoft-graph-mcp`),
authenticating with `MICROSOFT_GRAPH_TOKEN`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Microsoft Graph API.

## Default-safe behavior

- **The typed commands are read-only.** Directory, licensing, security, device, and the
  cross-entity analytics (`licenses waste`, `admins audit`, `security triage`,
  `managed-devices drift`, `groups risk`, `tenant snapshot`) only read - from the live
  Graph API or the local SQLite mirror. They cannot change anything in your tenant.
- **There is exactly one write path: `import`.** It issues a POST per JSONL record to
  create objects via Graph. Pass `--dry-run` first to preview every request without
  sending, then make your agent's policy: preview, show the exact command, get approval,
  then run the write.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy for `import` still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, audits, search. No change. | `licenses waste`, `licenses orphans`, `admins audit`, `security triage`, `managed-devices drift`, `groups risk`, `tenant snapshot`, `users list`, `pull`, `search`, `export` | Allow |
| **Write (import only)** | The sole create path: POST per JSONL record. | `import <resource> --input data.jsonl` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Touches tokens, keys, MFA. | (none detected - `auth login` only mints/caches your own token locally) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | (none - the CLI exposes no delete or update path) | Human-in-the-loop only, explicit confirmation |

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
the Microsoft Graph API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
