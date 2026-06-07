# xero skill - governance and safety model

> Unofficial. Community-built skill for the Xero API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the xero skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `xero-cli` binary (and `xero-mcp`),
authenticating with `XERO_ACCESS_TOKEN`, `XERO_ACCOUNTING_OAUTH2`, `XERO_TENANT_ID`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Xero API.

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
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | `accounts create`, `accounts update`, `bank-transactions create`, `bank-transactions update`, `bank-transactions update-or-create`, `contacts create`, `contacts update`, `contacts update-or-create`, `invoices create`, `invoices update`, `items create`, `items update`, `payments create`, `payments create-endpoint`, the per-object `history create-*` commands, and `import` (bulk POST, one request per JSONL record) - 22 mutating commands in all | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches stored auth tokens. | `auth logout` (removes the stored OAuth2 token); `auth login` / `login` (writes a token after the OAuth2 flow) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `accounts delete`, `items delete`, `payments delete` | Human-in-the-loop only, explicit confirmation |
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
the Xero API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
