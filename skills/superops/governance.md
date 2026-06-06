# superops skill - governance and safety model

> Unofficial. Community-built skill for the SuperOps API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the superops skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `superops-cli` binary (and `superops-mcp`), authenticating with
a SuperOps **API token**: `SUPEROPS_API_TOKEN` sent as `Authorization: Bearer <token>`,
plus `SUPEROPS_SUBDOMAIN` (your tenant subdomain, sent as the `CustomerSubDomain`
header) and an optional `SUPEROPS_REGION` (`us` default, or `eu` for euapi.superops.ai).
Credentials are read from the environment only - never written to disk by the skill,
never logged, never sent anywhere except the SuperOps API. The **scope of the token you
mint** in SuperOps is the real permission boundary - the CLI can only do what that
token is permitted to do.

## Default-safe behavior

- **Every typed command is read-only.** Tickets, assets, alerts, clients, contracts,
  invoices, worklogs, sites, technicians, the cross-entity views (`sla-watch`,
  `client-360`, `at-risk-assets`, `alert-coverage`, `unbilled`, `stale-tickets`,
  `context-ticket`), `sync`, `search`, and `analytics` all read; none of them change
  remote state.
- **The single write path is `raw mutation`.** `raw query` reads; `raw mutation` is the
  one supported escape hatch for operations the typed commands don't wrap (for example
  `createTicket`, `updateTicket`, `resolveAlerts`). `--dry-run` prints the exact GraphQL
  request without sending it - make your agent's policy: preview, show the request, get
  approval, then run.
- **`import` is a no-op.** Bulk import is not supported on this GraphQL API; the command
  exists only to say so.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no write
  gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) mutations**;
require a human for any `raw mutation` that actually sends.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search, the cross-entity views. No change. | every typed command + `raw query`; e.g. `sla-watch`, `client-360`, `at-risk-assets`, `unbilled`, `tickets list`, `search`, `sync` | Allow |
| **Write (mutation escape hatch)** | The only path that changes remote state. | `raw mutation` (createTicket, updateTicket, resolveAlerts, and anything the typed commands don't wrap) | Preview with `--dry-run`, then an approved, reviewed send |
| **Destructive** | Irreversible data or config loss. | no typed destructive command exists; a delete would be an explicit destructive GraphQL operation run through `raw mutation` | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Touches tokens, keys, MFA. | none on the remote API; `auth set-token` / `auth logout` only write the local config file | Human-in-the-loop only |

## How to lock it down

- **Scope the token** to only what your workflow needs. A read/report workflow does not
  need a token that can run mutations.
- **Keep autonomous agents to Read + previewed mutations.** Have a human approve the
  actual `raw mutation` send - the gate lives in your agent's policy, not in the binary's
  defaults.
- **Never let an agent run a destructive `raw mutation` unattended.** Treat it like a
  production database change: human, reviewed, logged.
- **Rotate the token if it is ever exposed** (for example after bridging the MCP server
  to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the token, the binary uses it against the
SuperOps API, and you can read every line of how it does so. The skill is read-only by
default, with one clearly marked, preview-able mutation path, scoped to your own tenant.
