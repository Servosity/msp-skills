# sentinelone skill - governance and safety model

> Unofficial. Community-built skill for the SentinelOne API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the sentinelone skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `sentinelone-cli` binary (and `sentinelone-mcp`),
authenticating with `SENTINELONE_API_TOKEN`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the SentinelOne API.

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
| **Write (routine)** | Day-to-day mutations. | `accounts create`, `accounts policy update-account`, `accounts reactivate`, `accounts reactivate account`, `accounts update`, `agents clear-remote-shell`, `agents reset-local-config`, `agents set-external-id`, ... (102 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | `accounts uninstall-password`, `accounts uninstall-password generate-regenerate`, `accounts uninstall-password get`, `accounts uninstall-password get-metadata`, `accounts uninstall-password revoke`, `groups regenerate-key regenerate-group-token`, `users api-token-details`, `users api-token-details token-by-user-id`, ... (16 total) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `cloud-detection delete-rules`, `config-override delete`, `config-override delete-configoverride`, `device-control delete-rules`, `exclusions delete`, `filters delete`, `filters delete-deep-visibility`, `firewall-control delete-rules`, ... (27 total) | Human-in-the-loop only, explicit confirmation |
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
the SentinelOne API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
