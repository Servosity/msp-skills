# knowbe4 skill - governance and safety model

> Unofficial. Community-built skill for the KnowBe4 API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the knowbe4 skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `knowbe4-cli` binary (and `knowbe4-mcp`),
authenticating with `KNOWBE4_API_KEY` (the KMSAT Reporting API key) and
`KNOWBE4_REGION` (your tenant's server: `us`, `eu`, `ca`, `uk`, or `de`; default
`us`). The `events` write commands use a **second, opt-in key**,
`KNOWBE4_USER_EVENT_API_KEY`, for the separate KnowBe4 User Event API - you only
set it if you want to push custom risk events. The bundled MCP server exposes
read-only reporting tools only and never needs the User Event key. Credentials are
read from the environment only - never written to disk, never logged, never sent
anywhere except the KnowBe4 API.

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
| **Read** | Reports, rollups, search, local sync. No change. | `account info`, `users list`, `groups list`, `phishing-tests list`, `training-enrollments list`, `risk-leaderboard`, `repeat-clickers`, `risk-drift`, `untrained-clickers`, `coverage-gaps`, `qbr`, `sync`, `search`, `events list/get/types` | Allow |
| **Write (routine)** | Pushes data to KnowBe4 via the API. | `events create` (push a custom risk event; needs `KNOWBE4_USER_EVENT_API_KEY`), `import` (bulk create/upsert from JSONL) | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Touches tokens, keys, MFA. | (none) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `events delete` (remove a custom risk event from a user's timeline) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none) | Operator-only, not for agents |

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
the KnowBe4 API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
