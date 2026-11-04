# blackpoint skill - governance and safety model

> Unofficial. Community-built skill for the Blackpoint Cyber API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the blackpoint skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `blackpoint-cli` binary (and `blackpoint-mcp`),
authenticating with `BLACKPOINT_API_TOKEN` (a CompassOne API token; a partner token reaches every
customer tenant). The token is read from the environment or saved with `auth set-token` to a
credentials file only your user can read; it is never logged and never sent anywhere except the
Blackpoint Cyber API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** From the CLI, mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **MCP writes preview first.** Over MCP, `blackpoint_execute` returns a preview of any write until the call passes `confirm: true`, and `BLACKPOINT_MCP_READ_ONLY=1` (preset on in the Claude Desktop bundle) hides every write tool while live reads stay available.
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
| **Read** | Reports, rollups, search, local sync. No change in CompassOne. | `triage`, `overview`, `client-report`, `gaps`, `stale-agents`, `cve-spread`, `leaderboard`, `geo-audit`, `fleet-sync`, `sync`, `search`, every `list` and `get` | Allow |
| **Credential** | Returns or takes a secret. | `darkweb-exposures` (lists breached accounts; each password is replaced by `[withheld]`), `cisco-onboardings create` (takes a Duo secret key), `auth set-token`, `auth logout`; any command that can emit a stored secret field | Human-in-the-loop only |
| **Write (routine)** | Escalation contacts and onboarding steps. | `contact-groups create`, `contact-groups update`, `contact-groups add-member`, `contact-groups update-member`, `contact-groups assign-tenants`, `cisco-onboardings sync-users`, `cisco-onboardings complete`, `import` | Preview with `--dry-run`, then an approved write |
| **Access policy (Cloud Response)** | Changes which countries a customer's users may sign in from. | `ms365 approve-country`, `ms365 approve-user-country`, `ms365 remove-country`, `ms365 remove-user-country`, `cloud-connections approve-country`, `cloud-connections approve-user-country`, `cloud-connections remove-approved-country`, `cloud-connections remove-user-country` | Human-in-the-loop only |
| **Destructive** | Irreversible removal. | `contact-groups delete`, `contact-groups delete-many`, `contact-groups delete-member`, `cisco-onboardings delete` (also removes the Duo onboarding app) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none) | Operator-only, not for agents |

These policies are yours to apply in your agent setup; the binary does not enforce the Credential tier. The MCP server answers `darkweb-exposures` reads even with `BLACKPOINT_MCP_READ_ONLY=1` (read-only mode drops writes, not reads). Each breached password and each Duo secret key is replaced by `[withheld]` before any output, in every mode, and is never written to the response cache or the local store: read a breached password in the CompassOne portal.

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
the Blackpoint Cyber API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
