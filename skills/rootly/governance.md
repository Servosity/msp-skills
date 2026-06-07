# rootly skill - governance and safety model

> Unofficial. Community-built skill for the Rootly API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the rootly skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `rootly-cli` binary (and `rootly-mcp`),
authenticating with `ROOTLY_API_KEY`, `ROOTLY_API_TOKEN`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Rootly API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
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
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command **except secret-returning reads** (`secrets get`, `secrets list`, `api-keys` reads belong in the Credential tier below) | Allow |
| **Write (routine)** | Day-to-day mutations. | `import` (bulk create/upsert from a JSONL file - treat as a large write), `action-items update-incident`, `alert-events update`, `alert-fields create`, `alert-fields update`, `alert-groups create`, `alert-groups update`, `alert-routes create`, `alert-routes patch`, ... (217 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | `api-keys rotate`, `api-keys rotate api-key`, `secrets`, `secrets create`, `secrets get`, `secrets list`, `secrets update` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `action-items delete-incident`, `alert-events delete`, `alert-fields delete`, `alert-groups delete`, `alert-routes delete`, `alert-routing-rules delete`, `alert-sources delete-alerts-source`, `alert-urgencies delete-alert-urgency`, ... (91 total) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

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
the Rootly API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
