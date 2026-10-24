# acronis skill - governance and safety model

> Unofficial. Community-built skill for the Acronis API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the acronis skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `acronis-cli` binary (and `acronis-mcp`),
authenticating with `ACRONIS_BEARER_AUTH`, or with `ACRONIS_CLIENT_ID` and
`ACRONIS_CLIENT_SECRET` exchanged for a token, against the `ACRONIS_DATACENTER` host.
Credentials come from the environment, or from `auth login` / `auth set-token`, which
save them to a credentials file in an owner-only directory under your user profile.
They are never logged and never sent anywhere except the Acronis API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (rollups, search, list and get); they
  cannot change anything. `reports` is not one: it creates a report configuration.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Rollups, search, list and get. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | `agent-manager force-agent-update`, `agent-manager update-agent-update-settings`, `reports`, `tenants create`, `tenants offering-items update`, `tenants update` | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | `clients create` (returns the new client's secret), `idp request-token`, `idp revoke-token` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `agent-manager delete-agent`, `agent-manager delete-agents`, `clients delete`, `tenants delete` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **For an MCP host, set `ACRONIS_MCP_READ_ONLY=1` unless it must write.** The
  endpoint tools that delete or replace (DELETE, PUT) carry the MCP
  `destructiveHint` annotation, and those that create or trigger (POST, such as
  `agent-manager_force-agent-update` and `idp_revoke-token`) are marked not
  read-only but not destructive. The server asks for no confirmation on either:
  that step is your MCP host's. With the switch set, the server lists only tools
  marked read-only.
- **Rotate the credential if it is ever exposed** (for example if the MCP HTTP
  bearer token or the host serving it leaks - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Acronis API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
