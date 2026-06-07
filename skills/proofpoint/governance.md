# proofpoint skill - governance and safety model

> Unofficial. Community-built skill for the Proofpoint API. Not affiliated with,
> endorsed by, or sponsored by Proofpoint, Inc.
> This page tells an MSP owner exactly what the proofpoint skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `proofpoint-cli` binary (and `proofpoint-mcp`),
authenticating with `PROOFPOINT_SERVICE_PRINCIPAL` and `PROOFPOINT_API_SECRET` (an
HTTP Basic credential pair created under Settings then Connected Applications in the
TAP dashboard). Credentials are read from the environment only - never written to
disk by the skill, never logged, never sent anywhere except the Proofpoint TAP API.
The `url` decode command works without credentials; everything that touches threat
data requires them.

## Default-safe behavior

- **The CLI is read-first.** The Threat Insight API is threat *intelligence* - every
  endpoint this skill calls for threat data is a GET. The bundled MCP server exposes
  only those read tools.
- **`--dry-run` is opt-in for the one write path - use it.** The CLI-only `import`
  command issues API create/upsert calls; pass `--dry-run` first to preview the
  request without sending. Make your agent's policy: preview, show the exact command,
  get approval, then run the write.
- **Read commands are always safe to run** (SIEM feeds, people, campaigns, threats,
  incident briefs, IOCs, search, the local sync/archive); they cannot change anything
  in your TAP tenant.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add
  any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Threat feeds, rollups, briefs, IOC extraction, decode, search, local sync. No tenant change. | `siem list-issues`, `siem list-clicks-permitted`, `people list-vap`, `people list-top-clickers`, `campaign get`, `campaign-threats`, `threat`, `incident`, `iocs`, `forensics`, `risk-overlap`, `user`, `url`, `search`, `export`, `sync`, `workflow status` | Allow |
| **Write (routine)** | Bulk data import via API create/upsert. | `import` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Manages the locally stored TAP credential. | `auth set-token`, `auth logout` | Human-in-the-loop only (operator) |
| **Destructive** | Irreversible data or config loss. | (none - TAP exposes no delete endpoints) | n/a |
| **Admin** | Back-office administration. | (none - Essentials/PPS admin is a different API) | n/a |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the `import` write path.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual `import` write - the gate lives in your agent's policy, not in the binary's
  defaults.
- **Never let an agent run the credential commands unattended.** `auth set-token`
  and `auth logout` change which TAP credential the CLI uses; treat them as
  operator-only.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Proofpoint TAP API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
