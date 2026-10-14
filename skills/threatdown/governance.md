# threatdown skill - governance and safety model

> Unofficial. Community-built skill for the ThreatDown OneView API. Not affiliated
> with, endorsed by, or sponsored by Malwarebytes Inc. (ThreatDown is a Malwarebytes
> brand).
> This page tells an MSP owner exactly what the threatdown skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `threatdown-cli` binary (and `threatdown-mcp`) with an OAuth client
you create in OneView: open **Integrate** in the left menu, click **Add**, name the
application and tick the scopes it may use (`read`, `write`, `execute`). ThreatDown
requires a Global or Site Administrator to create it, and shows the secret only once.
Set `THREATDOWN_CLIENT_ID` and `THREATDOWN_CLIENT_SECRET`; the CLI exchanges them for a
short-lived bearer token (OAuth2 client credentials) at
`https://api.threatdown.com/oneview/oauth2/token`.

Optional settings:

- `THREATDOWN_OAUTH_SCOPE` - the scopes to request, space-separated (default
  `execute read write`). Set it to `read` for a reporting-only setup. Run
  `threatdown-cli auth login` after changing it so the next token carries the new scopes.
- `THREATDOWN_BASE_URL` / `THREATDOWN_TOKEN_URL` - override the API and token hosts.
- `THREATDOWN_USER_AGENT` - override the User-Agent header.

**What is stored on disk.** After a successful token request the CLI caches the bearer
token and its expiry in a private `credentials.toml` (file mode 0600) under the CLI's
data directory. The client ID and secret are not written there; they are read from the
environment each time a new token is needed. A cached token keeps working until it
expires even after you remove the environment variables, so run
`threatdown-cli auth logout` to clear it, or delete the OAuth client in OneView to
revoke access outright. Credentials are never sent anywhere except the ThreatDown token
and API endpoints.

## Default-safe behavior

- **The `respond` commands preview by default.** `respond isolate`, `respond restore`,
  `respond scan`, `respond reboot` and `respond update` resolve host names to endpoints
  across every site, print which machines would get the job and which are skipped, and
  send nothing until you add `--confirm` (or the global `--yes`). They refuse more than
  `--max-targets` endpoints (default 25), and refuse a host name that matches machines
  in more than one site unless `--site` narrows it or `--all-matches` is given. The
  global `--dry-run` on these commands only prints a placeholder; the real preview is
  the command without `--confirm`.
- **`--dry-run` is opt-in for every other write - use it.** Generated commands such as
  `endpoint-jobs issue`, `exclusions create`, `policies update` or `endpoints delete`
  send immediately unless you pass `--dry-run` to preview the request first. Make your
  agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (the fleet reports, triage views, sync,
  search, and every list, get, search and export command); they cannot change anything
  in OneView. Many OneView reads are POST searches, which is why the tiers below are
  based on what a command does, not its HTTP method.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, triage, search, lookups. No change in OneView. | `fleet stale`, `fleet health`, `seats reconcile`, `respond pending`, `threats spread`, `threats queue`, `customer report`, `fleet sync`, `sync`, `search`, `analytics`, `export`, the `respond` commands without `--confirm`, every `list` / `get` / `search` / `export` command | Allow |
| **Endpoint actions** | Sends a job that runs on real client machines: isolate, release, scan and quarantine, reboot, update, remediate. | `respond isolate` / `restore` / `scan` / `reboot` / `update` with `--confirm`, `endpoint-jobs issue`, `endpoint-jobs bulk-issue`, `endpoint-jobs quarantine-bulk`, `endpoint-jobs parent-reissue`, `endpoint-jobs parent-children-reissue`, `grid jobs-bulk`, `endpoints sa-remediate`, `suspicious-activity remediate`, `import endpoint_jobs` (each line issues a job, with no preview) | Human-in-the-loop, explicit confirmation |
| **Security config** | Changes what the agent protects against or how. An exclusion lets software run that would otherwise be blocked. | `exclusions create` / `update`, `policies create` / `update` / `promote`, `policies auto-isolation-set`, `firewall rules-create` / `rules-update`, `content-filtering create` / `update` / `enable`, `device-control exclusions-create`, `app-block rules-create`, `schedules create` / `update`, `webhooks create`, `notifications create`, `syslog create` | Preview with `--dry-run`, then a human-approved write |
| **Destructive** | Removes endpoints, sites, policies or users; not undoable from the CLI. | `endpoints delete`, `endpoints delete-bulk`, `grid endpoints-delete`, `sites delete`, `policies delete`, `exclusions delete`, `groups delete`, `users delete`, `subscriptions delete`, `reports delete-bulk`, `notifications delete-by-owner` | Human-in-the-loop only, explicit confirmation |
| **Credential and admin** | Issues enrollment tokens, sets the tamper-protection password, manages users, sites and billing. | `installation-tokens create` / `send` / `revoke`, `policies secret-set`, `users create` / `update` / `resend-invite`, `sites create` / `update`, `subscriptions create` / `update` / `extend`, `email-protection migrate`, `auth login`, `auth set-token` | Human-in-the-loop only |

**The MCP server.** Besides one tool per fleet, triage and `respond` command, the MCP
server exposes a generic `threatdown_execute` tool that can call any OneView API
operation, including `jobs/bulk` (isolate, scan, reboot) and endpoint deletes, with no
preview step. An allowlist keyed on CLI command names does not cover it. Set
`THREATDOWN_MCP_READ_ONLY=1` in the MCP server's environment and it drops its write tools
(among them the five `respond` job verbs, `sync`, `import` and `export`) and refuses
every non-GET call through `threatdown_execute`. Because OneView searches are POSTs,
read-only mode also blocks raw searches through `threatdown_execute`; the fleet and
triage tools keep working.

## How to lock it down

- **Scope the OAuth client** to only what your workflow needs. A reporting workflow
  needs `read` only. Tick `execute` only if a human-approved workflow sends response
  jobs, and `write` only if it changes policies, exclusions, sites or users.
- **Request fewer scopes from the CLI** with `THREATDOWN_OAUTH_SCOPE=read`, then run
  `threatdown-cli auth login`. A token already cached keeps its old scopes until you do.
- **Run the MCP server read-only for reporting agents:** `THREATDOWN_MCP_READ_ONLY=1`.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the actual
  write for the Endpoint actions tier and above - the gate lives in your agent's policy,
  not in the binary's defaults (except the `respond` commands, which preview by default).
- **Never let an agent run Endpoint actions, Destructive or Credential tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the client secret if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md): generate a new
  secret in OneView, update `THREATDOWN_CLIENT_SECRET`, then run
  `threatdown-cli auth logout` to clear the cached token.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the OAuth client, the binary uses it against
the ThreatDown OneView API, and you can read every line of how it does so. The skill is
read-first, previews response jobs by default, and is limited to the scopes your OAuth
client was granted.
