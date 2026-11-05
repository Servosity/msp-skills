# rocketcyber skill - governance and safety model

> Unofficial. Community-built skill for the RocketCyber API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the rocketcyber skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `rocketcyber-cli` binary (and `rocketcyber-mcp`),
authenticating with `ROCKETCYBER_API_TOKEN`, read from the environment or from the credentials
file `auth set-token` writes (readable only by your account). The token is never logged, never
printed, and sent only to the configured RocketCyber API host over HTTPS.

## Default-safe behavior

- **Nothing writes to RocketCyber.** The Customer API is read-only, so every API call this skill
  makes is a read. `--dry-run` previews any request without sending it.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read and local-only commands**; require a
human for erasing local data and for anything that touches the credential.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, analytics, search. No change. | `triage`, `incidents` (+ `mttr`), `agents` (+ `stale`), `defender` (+ `riskiest`), `events list`/`summary`, `office` (+ `trend`), `apps`, `firewalls`, `account`, `suppression rules`/`rule`/`audit`, `reports`, `search`, `sync`, `export` | Allow |
| **Write (API)** | Changes data in RocketCyber. | (none - the RocketCyber Customer API is read-only) | n/a |
| **Local only** | Writes only to this machine: the learning store, saved profiles, local feedback, erasing local data. | `teach`, `profile save`/`delete`, `feedback` (POSTed only with a configured endpoint and `--send`), `local-data purge` (dry run unless `--yes`) | Allow; a human approves `local-data purge --yes` |
| **Credential / security** | Touches the stored API token. | `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | (none in RocketCyber; `local-data purge --yes` erases only this machine's local copy) | n/a |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read and Local only.** Have a human approve
  `local-data purge --yes` and every Credential tier command - the gate lives in your
  agent's policy, not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed.** Keep the MCP server on your own
  machine: its HTTP mode listens on loopback and needs `ROCKETCYBER_MCP_HTTP_TOKEN`.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the RocketCyber API, and you can read every line of how it does so. The skill is
read-only against RocketCyber and scoped to your own account.
