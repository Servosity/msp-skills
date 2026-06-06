# cipp skill - governance and safety model

> Unofficial. Community-built skill for the CIPP API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the cipp skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cipp-cli` binary (and `cipp-mcp`),
authenticating with `CIPP_API_KEY`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the CIPP API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Most mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **`bulk` is the exception - it plans by default.** `cipp-cli bulk --from changes.csv` prints the plan and does **not** execute; it only writes when you add `--execute`. Pair it with `--resume` so a throttled batch checkpoints instead of restarting.
- **Read commands are always safe to run** (reports and cross-tenant rollups); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports and cross-tenant rollups. No change. | `posture --dimension mfa`, `licenses waste`, `users stale --days 90`, `standards drift`, `fanout --endpoint /ListUsers --all-tenants`, `list-tenants`, `doctor`, and any other non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | `exec-csplicense create`, `exec-list-app-id create`, `add-intune-reusable-setting`, `add-intune-reusable-setting-template`, `create-safe-links-policy-template`, `exec-branding-settings`, `exec-combined-setup`, `exec-create-app-template`, ... (51 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | `exec-get-local-admin-password`, `exec-password-config`, `exec-password-never-expires`, `exec-per-user-mfa`, `exec-reset-mfa`, `exec-token-exchange`, `exec-update-refresh-token`, `list-mfausers`, ... (9 total) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `delete-sharepoint-site`, `delete-test-report`, `exec-delete-gdaprelationship`, `exec-delete-gdaprole-mapping`, `exec-delete-safe-links-policy`, `exec-device-delete`, `exec-exchange-role-repair`, `exec-groups-delete`, ... (12 total) | Human-in-the-loop only, explicit confirmation |
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
the CIPP API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
