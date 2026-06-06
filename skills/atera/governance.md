# atera skill - governance and safety model

> Unofficial. Community-built skill for the Atera API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the atera skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `atera-cli` binary (and `atera-mcp`),
authenticating with an Atera API key created under **Admin → API**, supplied via
`ATERA_API_KEY` (or its alias `ATERA_ACCOUNT_API`). The key is sent as the
`X-API-KEY` header. Credentials are read from the environment or the local config
the CLI writes via `auth set-token` - never logged, never bundled into this repo,
and never sent anywhere except the Atera API (`https://app.atera.com/api/v3`).

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
| **Read** | Reports, rollups, search. No change. | the cross-client analytics views (`agents stale`, `agents inventory`, `agents noisy`, `agents patch-status`, `alerts triage`, `tickets sla`, `tickets workload`, `customers book`, `customers coverage`, `contracts expiring`, `since`), every `get-`/query command, `search`, and `sync` (≈81 total) | Allow |
| **Write (routine)** | Day-to-day mutations: create and update. | `tickets post`, `tickets put`, `contacts post`, `contacts put`, `customers post`, `customers put`, `contracts post`, `contracts update`, `alerts post`, `alerts resolve`, `devices create-*`, `customvalues custom-values-set-*`, `rates post-*`/`put-*`, `import` (61 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches the stored API token. | `auth set-token`, `auth setup`, `auth logout` (3 total) | Human-in-the-loop only |
| **Destructive** | Irreversible data loss. | `agents delete`, `alerts delete`, `contacts delete`, `customers delete`, `departments delete`, `devices delete`, `devices delete-http`, `devices delete-snmp`, ... (13 total) | Human-in-the-loop only, explicit confirmation |

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
the Atera API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
