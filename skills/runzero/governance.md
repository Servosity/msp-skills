# runzero skill - governance and safety model

> Unofficial. Community-built skill for the runZero API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the runzero skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `runzero-cli` binary (and `runzero-mcp`),
authenticating with `RUNZERO_API_KEY`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the runZero API.

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
| **Read** | Reports, rollups, search. No change. | the cross-entity views and the non-secret reports (assets, sites, services, vulnerabilities, tasks) - NOT the credential/token/key `get-*` commands, which are in the Credential tier below | Allow |
| **Write (routine)** | Day-to-day mutations, including launching scans and importing data. | `scan-watch` and `org create-scan` (launch a real network scan - see the callout below), `import` and `runzero-import` (write to runZero via create/upsert), `org create-site`, `account create-group`, `account create-key`, `account create-organization`, `account create-scan-template`, asset tag/owner updates, ... (60+ total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | `account create-credential`, `account create-organization-export-token`, `account get-apitoken`, `account get-credential`, `account get-credentials`, `account get-organization-export-token`, `account get-organization-export-tokens`, `account remove-credential`, ... (15 total) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `account delete-asset-ownership-type`, `account delete-asset-ownership-types`, `account delete-custom-integration`, `account delete-organization-export-token`, `account delete-organization-export-token-deprecated` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

The `org` namespace carries its own secret/key commands that belong in this
tier even though the auto-generated examples above are account-scoped: `org
get-key` (returns API key details), `org rotate-key` (returns the updated key
secret), and `org remove-key`. Treat every credential/token/key `get-*`,
`rotate-*`, and `create-*` as human-in-the-loop regardless of namespace.

## Active scanning is not a read

`scan-watch` and `org create-scan` do not just query stored data - they tell
runZero to send packets to the targets you name. Treat them as writes, not reads:
an agent should never launch a scan unattended. Confirm the site, the targets, and
the rate with a human first, and make sure the scan is in scope for the network
you are pointing it at. `import` and `runzero-import` likewise change runZero's
own data via create/upsert calls, so they belong in the Write tier even though
their names carry no "create"/"update" verb.

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
the runZero API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
