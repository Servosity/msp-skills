# avanan skill - governance and safety model

> Unofficial. Community-built skill for the Avanan API. Not affiliated with,
> endorsed by, or sponsored by Check Point.
> This page tells an MSP owner exactly what the avanan skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `avanan-cli` binary (and `avanan-mcp`), authenticating with
`AVANAN_APP_ID` and `AVANAN_CLIENT_SECRET`. Those two values are exchanged for a
one-hour session token, which is replayed on later calls. Credentials are read
from the environment by default and are never logged or sent anywhere except the
Avanan API. They are written to disk only when you ask for it: `auth login --save`
persists the application ID and secret, and `auth set-token` persists a session
token, both to the credentials file under your own user profile.

Three properties of the Avanan API are worth knowing before you mint an
application credential:

- **Credentials are region-scoped and regions are hard-isolated.** A key issued
  for the USA farm cannot read the EU farm; cross-region access is refused by
  design, not by permission. Set the region once with
  `avanan-cli auth login --region <us|eu|ca|ap|uk|uae|in> --save`.
- **One application credential can reach many tenants.** `avanan-cli scopes` lists
  the `{farm}:{tenant}` scopes your credential covers. Every query command
  defaults to all of them, so a single credential is a fleet-wide credential. If
  you want an agent constrained to one customer, mint an application credential
  bound to that tenant rather than relying on a `--scope` flag the agent could
  omit.
- **Action endpoints are single-scope by design.** Quarantine, restore, and task
  lookups accept exactly one scope and return HTTP 400 without one. The
  `remediate` command turns that into an error naming your actual scopes, so a
  mailbox-affecting write cannot land on a tenant you did not name.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you
  pass `--dry-run` first to preview the exact request without sending it. Make your
  agent's policy: preview, show the command, get approval, then run the write.
- **Read commands cannot change anything** (triage, campaign, timeline, exception
  lookup and audit, MSP fleet rollup, search, and every list/get command). Safe to
  run is not the same as safe to sweep: the commands in the Message content egress
  tier below are reads too, and they pull real customer email onto your machine.
- **`sync` and `mirror` are read-only against the API.** They issue read requests
  and write only to the local SQLite mirror on your own machine.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add
  any write gating - the preview-then-approve policy above still applies. See
  [AGENTS.md](./AGENTS.md).

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `triage`, `campaign`, `timeline`, `exceptions find`, `exceptions audit`, `msp fleet`, `event query`, `event get`, `avanan-search query-saas-entity`, `scopes`, `task`, `msp list*`, `msp describe-*`, `sectool-exceptions exceptions list-sectool`, `sectools list-*`, `sync`, `mirror`, `search`, `analytics`, `export` | Allow |
| **Message content egress** | Pulls real customer email out of Avanan and onto your machine (and into the agent's context). | `download`, `download-large-email`, `avanan-search get-saas-entity` (`soar get-entity` currently returns 404 on every tenant tested) | Allow only for a named message under an open investigation; never sweep |
| **Write (routine)** | Policy exceptions and reclassification. Reversible. | `exceptions create`, `exceptions create-whitelist`, `exceptions update`, `exceptions update-whitelist`, `sectool-exceptions create`, `sectool-exceptions update`, `sectool-exceptions exceptions create-sectool-entry`, `sectools create-anomaly-exception`, `sectools create-ctp-item`, `sectools update-ctp-item`, `sectools update-ctp-items`, `report` | Preview with `--dry-run`, then an approved write |
| **Bulk write (`import`)** | Replays a JSONL file as one POST per record against a chosen resource. | `import` (bulk POST from JSONL; `import action` reaches live mail, `import soar` notifies end users, `import msp` mutates tenants) | Human-approved only. It inherits the tier of whatever resource it targets, and the resources it accepts reach the mailbox-affecting and human-in-the-loop endpoints below. Preview with `--dry-run` first. |
| **Mailbox-affecting** | Reaches into a real user's mailbox in Microsoft 365 or Google Workspace. | `remediate quarantine`, `remediate restore`, `action post-entity`, `action post-event` | Preview with `--dry-run`, then a human-approved write, always with an explicit `--scope` |
| **End-user contact** | Sends mail to real people on your behalf. | `soar post-notify` | Human-in-the-loop only |
| **Tenant and billing lifecycle** | Creates tenants, users, and license assignments that show up on an invoice. | `msp create`, `msp create-tenants`, `msp create-users`, `msp update`, `msp update-or-create-tenant-license` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `msp delete`, `msp delete-tenants`, `msp delete-users`, `exceptions delete`, `exceptions delete ap-exception`, `sectool-exceptions delete`, `sectool-exceptions exceptions delete-sectool-entries`, `sectools delete-anomaly-exceptions`, `sectools delete-ctp-item`, `sectools delete-ctp-items`, `sectools delete-ctp-lists` | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Touches tokens and keys. | `auth login`, `auth set-token`, `auth logout` | Operator-only, not for agents |
| **Admin** | Back-office administration. | (none - Avanan exposes no admin surface on this API) | n/a |

Two rows above are corrections to what a purely verb-based reading of the command
set produces, and they matter:

- `remediate`, `action post-entity`, and `action post-event` carry no
  create/update/delete verb, but they quarantine and restore live mail. They are
  the highest-consequence routine operations in this connector.
- `download`, `download-large-email`, and `soar get-entity` are reads by HTTP
  method and by tier, but what they read is the body of somebody's email. Treat
  them as a privacy decision, not a lookup.

`msp delete-tenants` deletes an entire customer tenant. Nothing else here is
close to it in blast radius.

## How to lock it down

- **Scope the credential to a region and, where you can, to a tenant.** A
  fleet-wide application credential is convenient for `msp fleet` and dangerous
  for everything below the line. Run `avanan-cli scopes` to see exactly how far
  the credential you handed the agent actually reaches.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Require an explicit `--scope` on every mailbox-affecting command,** even when
  the credential only covers one tenant today. It will not always only cover one.
- **Never let an agent run Destructive, Tenant/billing, End-user contact, or
  Credential tier commands unattended.** Treat them like a production database
  drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see
  [mcp-install.md](./mcp-install.md)).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Avanan API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account. The local SQLite
mirror stays on your machine, and the MCP server speaks stdio only, so it opens no
network listener.
