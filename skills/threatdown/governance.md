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
  `execute read write`). Set it to `read` for a reporting-only setup. Changing it makes
  the CLI request a new token with the new scopes on its next call.
- `THREATDOWN_BASE_URL` / `THREATDOWN_TOKEN_URL` - override the API and token hosts.
- `THREATDOWN_USER_AGENT` - override the User-Agent header.

**Credentials from OneView are never stored.** `sync` and the HTTP response cache drop
the secret fields OneView returns inside ordinary rows (`as_uninstall_password`,
`secret`, `secret_hash`, `secret_token`, `custom_headers`, `password`, `token`) before anything reaches
the local SQLite mirror, and a response that carries one is never cached; `tail` strips
them from what it streams. The `installers` and `endpoints diag-log` responses (signed
download links) are never cached or stored either. Other live command output still shows
the secret fields, which is why those reads sit in the Credential tier. Webhook and
notification delivery URLs are stored; if your receiver URL carries its own token (a
Power Automate or Logic Apps `sig=` link), treat the CLI's data directory as holding it.

**What is stored on disk.** After a successful token request the CLI caches the bearer
token and its expiry in a private `credentials.toml` (file mode 0600) under the CLI's
data directory. A client ID and secret taken from the environment are not written there;
they are read again each time a new token is needed. `auth login` given `--client-id`
or `--client-secret` saves the client ID and secret it used in that file (including a
half taken from the environment), so later runs can request new tokens with no
environment variables until you run `threatdown-cli auth logout`. The cached
token is reused only by a process with the same client ID, scope and token URL; a
process configured with a different OAuth client or `THREATDOWN_OAUTH_SCOPE` requests
its own token, and one that has no client secret to do that stops with an error rather
than borrow the cached token. `auth set-token` clears any saved client ID and secret, and its token is used only
while no client ID or secret is configured. Run `threatdown-cli auth logout` to clear the
cache, or delete the OAuth client in OneView to revoke access outright. The local
SQLite mirror is shared by every process run as the same OS user, whichever OAuth client
synced it, so give an agent that must not see that data its own OS user or its own
`THREATDOWN_HOME` (the CLI's `--home`), which also gives it its own credentials file. Credentials are never sent anywhere except the ThreatDown token
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
  search, and nearly every list, get, search and export command); they cannot change anything
  in OneView. Some reads return credentials, though: `policies secret-get`,
  `policies secret-get-v2` and `sites secret` return tamper-protection passwords,
  `installation-tokens search` returns agent enrollment tokens, the other `policies` reads
  include each policy's uninstall password (`as_uninstall_password`) or its hash
  (`secret_hash`), `notifications` and `webhooks` list / get include the HMAC signing key
  (`secret_token`) and any custom auth headers (`custom_headers`), `export` of policies or
  notifications carries the same fields, and `installers` download links may embed the site's
  enrollment credential. Treat those as the Credential tier below. Many OneView reads are POST searches, which is why the tiers below are
  based on what a command does, not its HTTP method.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, triage, search, lookups. No change in OneView. | `fleet stale`, `fleet health`, `seats reconcile`, `respond pending`, `threats spread`, `threats queue`, `customer report`, `fleet sync`, `sync`, `search`, `analytics`, `tail` (it strips the secret fields), the `respond` commands without `--confirm`, every `list` / `get` / `search` / `search-groupby` / `export` command (many are POST queries, as are the `flight-recorder`, `vulnerability-assessment`, `usage` and `email-protection` stats and usage reads) except the credential reads in the last row and `endpoints diag-log` | Allow |
| **Endpoint actions** | Sends or cancels a job that runs on real client machines: isolate, release, scan and quarantine, reboot, update, remediate, collect logs. | `respond isolate` / `restore` / `scan` / `reboot` / `update` with `--confirm`, `endpoint-jobs issue`, `endpoint-jobs bulk-issue`, `endpoint-jobs quarantine-bulk`, `endpoint-jobs parent-reissue`, `endpoint-jobs parent-children-reissue`, the job cancels (`endpoint-jobs bulk-cancel`, `parent-cancel`, `parent-children-cancel`), `grid jobs-bulk`, `endpoints sa-remediate`, `suspicious-activity remediate`, `endpoints diag-log` (a GET that has the endpoint generate a diagnostic log), `import endpoint_jobs` (each line issues a job; preview it with `--dry-run`) | Human-in-the-loop, explicit confirmation |
| **Security config** | Changes what the agent protects against, which policy an endpoint gets, or where alerts and logs go. An exclusion lets software run that would otherwise be blocked; a changed webhook, notification or syslog target can silence alerting. | `exclusions create` / `update`; `policies create` / `update` / `promote` / `clone` / `set-default` and their `-v2` forms, `policies auto-isolation-set`, `expire-endpoints-set` / `expire-endpoints-disable`, `isolation-image-add` / `isolation-image-delete`; `groups create` / `update` / `assign-endpoints` and `grid groups-assign-endpoints`; every other write under `firewall`, `content-filtering`, `device-control`, `app-block`, `aidr` and `asset-ignore-rules`; `schedules create` / `update` / `groups`; `endpoints set-tags` / `sa-close` / `sa-open`, `suspicious-activity close` / `open`, `detections submit-fp`, `cve ignore` / `bulk`, the `cases` writes; `webhooks create` / `update`, `notifications create` / `update` / `bulk-update` / `change-owner` / `remove-email`, the `preferences` updates, `syslog create` / `update` / `machine-enable` / `machines-assign`; every other `email-protection` write (`disable`, `integrations-disable`, `auto-sync-enable` / `auto-sync-disable`, `incidents-classify`, allow-list, settings and mailbox updates); `reports create` / `update` / `generate` / `change-owner`; `sites mdr-set`; `import` of exclusions, policies, notifications, device_control, content_filtering, cve or cases | Preview with `--dry-run`, then a human-approved write |
| **Destructive** | Removes endpoints, sites, policies, rules, alert routes or users; not undoable from the CLI. | `endpoints delete`, `endpoints delete-bulk`, `grid endpoints-delete`, `sites delete`, `policies delete` / `delete-v2`, `exclusions delete`, `groups delete`, `users delete`, `subscriptions delete`, `webhooks delete`, `notifications delete` / `bulk-delete` / `delete-by-owner`, `schedules delete`, every delete under `firewall`, `content-filtering`, `device-control`, `app-block`, `aidr` and `asset-ignore-rules`, `reports delete` / `delete-bulk`, the `email-protection` allow-list, 911-mailbox and integration-domain deletes | Human-in-the-loop only, explicit confirmation |
| **Credential and admin** | Reads or sets tamper-protection passwords and webhook signing keys, issues and lists enrollment tokens, manages users, sites, billing and email-protection integrations. | every `policies` read (`list`, `get`, `search`, `system-default`, the `-v2` forms, `secret-get` / `secret-get-v2`), `sites secret`, `installation-tokens search`, `installers` (its download links may embed the site's enrollment credential), `webhooks list` / `get`, `notifications list` / `get`, and `export` of policies or notifications (their live output carries those secrets), `installation-tokens create` / `send` / `revoke`, `policies secret-set` / `secret-set-v2`, `users create` / `update` / `resend-invite`, `sites create` / `update`, `import` of users or sites, `subscriptions create` / `create-v2` / `create-v3` / `update` / `update-v2` / `extend` / `addons-update`, `email-protection register` / `register-msp` / `register-msp-site` / `migrate` and the integration authorize commands, `auth login`, `auth set-token` | Human-in-the-loop only |

The tables name every write command in this release. A command missing from them is a write if `threatdown-cli agent-context` gives it a POST, PUT or DELETE `pp:method` and it is not a `search`, `export` or report query: treat it as Security config at minimum.

**The MCP server.** Besides one tool per fleet, triage and `respond` command, the MCP
server exposes a generic `threatdown_execute` tool that can call any OneView API
operation, including `jobs/bulk` (isolate, scan, reboot) and endpoint deletes, with no
preview step. An allowlist keyed on CLI command names does not cover it. Set
`THREATDOWN_MCP_READ_ONLY=1` in the MCP server's environment and it drops its write tools
(among them the five `respond` job verbs, `sync`, `import` and `export`) and refuses
every non-GET call through `threatdown_execute`, plus `endpoints diag-log`, a GET that
makes the endpoint generate a log. Because OneView searches are POSTs,
read-only mode also blocks raw searches through `threatdown_execute`; the fleet and
triage tools keep working. Read-only mode still allows other GET calls, including the
tamper-protection password reads (`GET /v1/policies/{id}/secret`,
`GET /v1/sites/{id}/secret`), so do not hand a read-only MCP server to an agent you
would not trust with those passwords.
MCP tools never take a local file path: `--input`, `--notes-file`, `--playbook-file`
and `--playbook-notes-file` are refused, so the server account cannot be pointed at a
file to read. MCP agents pass playbooks and notes inline; run `import` from the CLI.

## How to lock it down

- **Scope the OAuth client** to only what your workflow needs. A reporting workflow
  needs `read` only. Tick `execute` only if a human-approved workflow sends response
  jobs, and `write` only if it changes policies, exclusions, sites or users.
- **Request fewer scopes from the CLI** with `THREATDOWN_OAUTH_SCOPE=read`. The CLI
  requests a new token whenever the scope differs from the one the cached token has.
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
read-first, its `respond` commands preview response jobs by default, and it is limited to the scopes your OAuth
client was granted.
