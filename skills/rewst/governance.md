# rewst skill - governance and safety model

> Unofficial. Community-built skill for the Rewst GraphQL API. Not affiliated with,
> endorsed by, or sponsored by Rewst, Inc.
> This page tells an MSP owner exactly what the rewst skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `rewst-cli` binary (and `rewst-mcp`), authenticating with
`REWST_API_TOKEN` (a bearer token from an API client you create in the Rewst console)
and, for non-US regions, `REWST_BASE_URL` (defaults to the US `https://api.rewst.io`).
Credentials are read from the environment only - never written to disk by the skill,
never logged, never sent anywhere except the Rewst GraphQL gateway.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (rollups, search, inventory); they cannot
  change anything. The local `sync` and `workflow archive` utilities only write to the
  local SQLite store, not to Rewst.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

Rewst's API is GraphQL: reads are queries, writes are mutations. The safe default for
an autonomous agent is **read only**. The important nuance for an automation platform:
some writes do not just change a record - they **arm or alter automation that runs
against live customer tenants**. Those get their own tier.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Rollups, search, inventory. No change to Rewst. | `health`, `failures`, `dormant`, `roi`, `drift`, `coverage`, every `get` / `list` / `search`, plus local `sync`, `analytics`, `workflow archive` (local store only) | Allow |
| **Write (config)** | Creates or updates non-automation entities. | `crates create` / `update`, `component-instances create` / `update`, `actions create`, `action-options create`, `conversations create` | Preview with `--dry-run`, then an approved write |
| **Automation and trigger changes** | Arms or alters automation that runs on live customer tenants. | `workflows create` / `update`, `triggers create` / `update`, `org-trigger-instances update`, `packs create` / `update`, `pack-configs create` / `update`, `org-variables create` / `update` | Human-in-the-loop, explicit confirmation |
| **Destructive** | Irreversible deletion of a Rewst record. | `delete-org-interpreter-responses`, `pack-delete-responses` | Human-in-the-loop only, explicit confirmation |
| **Admin and identity** | Tenant, user, and access administration. | `organizations create` / `update`, `users create` / `update`, `user-invites create`, `org-support-accesses` | Operator-only, not for agents |
| **Credential / security** | Stores an API token. | `auth set-token` (saves the token to the local config file) | Human-in-the-loop only |

> Note: there is no single "run this workflow now" command - Rewst fires workflows from
> triggers. The risk in the Automation tier is indirect but real: creating or updating a
> trigger or workflow can cause automation to execute against a real client tenant.

## How to lock it down

- **Scope the API client** to only what your workflow needs. A monitoring/report
  workflow does not need a Rewst token that can create triggers, edit workflows, or
  manage organizations.
- **Keep autonomous agents to the Read tier.** Have a human approve any Write,
  Automation, Destructive, Admin, or Credential command - the gate lives in your agent's
  policy, not in the binary's defaults.
- **Treat the Automation and Admin tiers like a production change window.** A new trigger
  or an edited workflow runs against a live customer's environment: human, reviewed, logged.
- **Rotate the token if it is ever exposed** (for example after bridging the MCP server
  to a public endpoint for ChatGPT - see mcp-install.md). Revoke the API client in the
  Rewst console.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the token, the binary uses it against the
Rewst GraphQL gateway, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
