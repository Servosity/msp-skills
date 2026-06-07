# kaseya-bms skill - governance and safety model

> Unofficial. Community-built skill for the Kaseya BMS API. Not affiliated with,
> endorsed by, or sponsored by Kaseya US LLC.
> This page tells an MSP owner exactly what the kaseya-bms skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `kaseya-bms-cli` binary (and `kaseya-bms-mcp`), authenticating
as a **Kaseya BMS API user** against your tenant. There are two ways in:

- **Token-mint (recommended):** set `KASEYA_BMS_USERNAME`, `KASEYA_BMS_PASSWORD`, and
  `KASEYA_BMS_TENANT`, then run `kaseya-bms-cli auth login`. The CLI exchanges them for
  a short-lived JWT via `POST /v2/security/authenticate` and stores that token in a
  `0600` config file under your home directory. The password is read from the
  environment only - never passed on the command line, never written to disk.
- **Pre-minted token:** set `KASEYA_BMS_TOKEN` (or `KASEYA_BMS_BEARER_AUTH`) to a JWT
  you already hold; it is sent as a Bearer header and nothing is persisted.

Regional tenants set `KASEYA_BMS_BASE_URL` (default `https://api.bms.kaseya.com`;
EMEA `https://api.bmsemea.kaseya.com`, APAC `https://api.bmsapac.kaseya.com`). The
token is sent only to the Kaseya BMS API, never logged, and never sent anywhere else.
The skill can only ever do what that BMS API user is permitted to do.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything, and every GET carries the `mcp:read-only` annotation so MCP clients
  can gate writes automatically.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line. Tiers are assigned by HTTP method (every
`GET` is a read; `POST`/`PUT`/`PATCH` are writes; `DELETE` is destructive), not by
command name - so a create named `post-ticket` is correctly a write, not a read.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** (243) | Reports, rollups, search, and every GET. No change. | `queue-health`, `stale-tickets`, `workload`, `unbilled`, `pipeline`, `contract-burn`, `servicedesk get-ticket`, `crm get-accounts`, `search` | Allow |
| **Write - routine** (124) | Day-to-day create/update across the service desk, CRM, finance, projects, inventory, and integrations (every `POST`/`PUT`/`PATCH`), plus the verb-less `import`. | `servicedesk post-ticket`, `servicedesk put-ticket`, `servicedesk assign-ticket`, `servicedesk resolve-ticket`, `crm post-account`, `crm patch-contact`, `finance activate-contract`, `finance mark-invoices-as-sent`, `project post-status`, `import` | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Returns or mints stored secrets. | `integrations get-itgpassword-value`, `integrations get-itgaccess-info`, `integrations get-vsa-access-info`, `security authenticate`, `security refresh-token`, `auth login`, `auth set-token` | Human-in-the-loop only |
| **Destructive** (21) | Irreversible data loss. | `servicedesk delete-ticket`, `servicedesk delete-tickets`, `servicedesk delete-service-call`, `crm delete-contact`, `project delete-status`, `rmm mappings delete`, `system delete-attachment`, `system delete-view` | Human-in-the-loop only, explicit confirmation |
| **Admin** (75) | Back-office administration: the entire `admin` group. | `admin post-workflow`, `admin delete-service`, `admin put-webhook-configuration`, `admin post-k1-access-control-mapping`, `admin delete-teams-channel`, `admin clone-workflow` | Operator-only, not for agents |

> `jobs prune` and `auth logout` affect only local state (the job-tracking cache and
> the stored token); they do not touch vendor data.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a BMS API user that can run the Destructive, Credential, or Admin tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
  The Credential tier matters specifically because the ITGlue/VSA integration reads
  can return stored passwords and access info.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md). Because BMS JWTs
  are short-lived, `kaseya-bms-cli auth login` re-mints a fresh one.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Kaseya BMS API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own tenant.
