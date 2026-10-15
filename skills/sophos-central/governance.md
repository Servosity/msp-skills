# sophos-central skill - governance and safety model

> Unofficial. Community-built skill for the Sophos Central API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the sophos-central skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `sophos-central-cli` binary (and `sophos-central-mcp`) with a
Sophos Central API credential (a service principal) you create: in the Partner
Dashboard under **Settings & Policies > API Credentials** for a partner, or under
**Global Settings > API Credentials Management** for an organization or a single
customer. It needs `SOPHOS_CENTRAL_CLIENT_ID` and `SOPHOS_CENTRAL_CLIENT_SECRET`, and
mints a short-lived bearer token from `https://id.sophos.com/api/v2/oauth2/token`
with the OAuth2 client-credentials grant. `SOPHOS_CENTRAL_TENANT_ID` (or `--tenant`)
is optional and picks one customer for tenant-scoped commands.

**The role you give the credential is the real permission boundary.** Service
Principal Read-Only can run every report in this skill but cannot change anything
and cannot run Live Discover queries. Give a credential write access only when the
workflow needs it.

Where the values live is your choice, and the CLI respects it:

- **Supplied through the environment** (a secret manager, Keychain, or a
  session-scoped variable): the client secret is **never copied to disk**. Minted
  access tokens and the client ID are cached in a permission-restricted (`0600`)
  `credentials.toml` in your user data directory so the next call does not re-mint.
  A client ID in the environment always wins over a different saved one: the token
  saved for the other credential is never sent, and the new token stays in memory,
  so a read-only credential given to an agent cannot pick up an admin credential's
  cached token.
- **Passed explicitly** with `auth login --client-secret`: the secret is written to
  that same `credentials.toml`, because that is what logging in is for. Run
  `sophos-central-cli doctor` to see the resolved paths, and `auth logout` to clear them.

`--dry-run` never mints a token or sends the secret. Credentials are never logged
and nothing is sent anywhere except `id.sophos.com` and the Sophos Central API hosts.
The CLI also caches the caller identity from `whoami` and each tenant's regional API
host, so it can route calls without asking again.

**Secrets Sophos returns are not stored.** Two kinds of response carry a live
credential: an endpoint's tamper protection password (`endpoints
get-tamper-protection`, `endpoints set-tamper-protection`) and a migration job's
token (`migrations get-job`, `get-jobs`, `create-receiving-job`,
`create-sending-job`). A response carrying either is never written to the response
cache or the local store, and those six commands are not exposed through the MCP
server, so an agent cannot read a tamper password on its own.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Generated API commands send immediately unless
  you pass `--dry-run` first to preview the request without sending. Make your
  agent's policy: preview, show the exact command, get approval, then run the write.
- **The two fleet actions preview by default.** `contain` and `fleet tamper` print
  their plan and change nothing unless you add `--yes`. The MCP server never passes
  `--yes`, so over MCP (Claude Desktop, ChatGPT, Copilot) they only preview; the
  change itself is run from the CLI, by you or by an agent you approved in Claude
  Code or Codex.
- **The MCP server can still write.** Its generic `sophos-central_execute` tool can
  call any Sophos Central endpoint except the six credential reads named above,
  including isolation, scans, exclusions and policy changes, gated only by your AI
  app's per-tool approval prompt. For an agent that should only read, set
  `SOPHOS_CENTRAL_MCP_READ_ONLY=1` in the MCP server's environment: it refuses every
  non-GET call and hides the `sync` and `export` tools.
- **Read commands are always safe to run** (the fleet reports, list, get, search);
  they cannot change anything in Sophos Central. `sync` and `fleet refresh` write
  only to your local store.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

Sophos Central is an endpoint security console: some of its commands reach real
customer machines, and some change what Sophos blocks or allows. The safe default
for an autonomous agent is **read only**; require a human for anything below Read.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change in Sophos Central. | `fleet triage`, `fleet health`, `fleet gaps`, `fleet revenue`, `fleet refresh`, `fleet siem-collect`, `whoami`, `tenants list`, `alerts list`, `alerts search`, `endpoints list`, `endpoints get`, `health-check get`, `licenses list`, `billing`, `siem events`, `tamper-protection`, `search`, `sync`, and every other `list` / `get` except the credential reads named below. `contain` and `fleet tamper` without `--yes` only preview. | Allow |
| **Endpoint response** | Acts on real customer machines. | `contain --yes`, `fleet tamper --yes`, `endpoints set-isolation`, `endpoints bulk-isolate`, `endpoints scan`, `endpoints update-check`, `endpoints collect-forensic-log`, `endpoints collect-memory-dump`, `endpoints update-aap`, `live-discover run-query`, `device-software`, `software add-special-static-package` | Human-in-the-loop, explicit confirmation |
| **Protection and policy config** | Changes what Sophos blocks, allows, scans or decrypts, or which policy a machine gets. An exclusion or allowed item can open a hole. | `exclusions add-scanning`, `exclusions update-scanning-by-id`, `exclusions delete-scanning-by-id`, `exclusions add-isolation`, `exclusions update-isolation-by-id`, `exclusions delete-isolation-by-id`, `exclusions add-intrusion-prevention`, `exclusions update-intrusion-prevention-by-id`, `exclusions delete-intrusion-prevention-by-id`, `allowed-items post-by-id`, `allowed-items patch-by-id`, `allowed-items delete-by-id`, `blocked-items post-by-id`, `blocked-items delete-by-id`, `blocked-addresses post-blocked-address-sets`, `blocked-addresses delete-blocked-address-sets`, `policies create-policy`, `policies update-policy`, `policies update-settings-of-policy`, `policies clone-policy`, `policies reset-policy-settings`, `policies reset-setting-in-policy`, `policies update-base-policy`, `policies update-settings-of-base-policy`, `policies update-value-of-setting-item-in-base-policy`, `policies clone-base-policy`, `policies reset-base-policy-settings`, `policies reset-setting-in-base-policy`, `exploit-mitigation add-application`, `exploit-mitigation patch-application-by-id`, `exploit-mitigation delete-application-by-id`, `web-control add-local-site-exclusion`, `web-control update-local-site-setting-by-id`, `web-control delete-local-sites-by-id`, `web-control update-web-decryption-excluded-sites`, `web-control update-web-decryption-settings`, `event-journal update-settings`, `endpoint-groups create-tenant`, `endpoint-groups patch-tenant`, `endpoint-groups add-multiple-endpoints-to-group`, `endpoint-groups remove-multiple-endpoints-from-group`, `endpoint-groups remove-single-endpoint-from-group` | Preview with `--dry-run`, then a human-approved write |
| **Write (routine)** | Day-to-day records with no direct protection effect. | `alerts act-on`, `endpoint-tags`, `users create-tenant`, `users patch-tenant`, `users add-to-multiple-groups`, `users remove-from-multiple-groups`, `users remove-from-single-group`, `user-groups create-tenant`, `user-groups patch-tenant`, `user-groups add-multiple-users-to-group`, `user-groups remove-multiple-users-from-group`, `user-groups remove-single-user-from-group`, `health-check post-snooze`, `xdr start-query-run`, `xdr cancel-query-run`, `enrichment-pivots create`, `enrichment-pivots update`, `software update-comment`, `import` | Preview with `--dry-run`, then a reviewed write |
| **Credential and access** | Returns a live secret, or changes who can do what. `endpoints get-tamper-protection` / `set-tamper-protection` return the tamper protection password; the three `migrations` reads return the migration token. | `endpoints get-tamper-protection`, `endpoints set-tamper-protection`, `migrations get-job`, `migrations get-jobs`, `migrations create-receiving-job`, `auth login`, `auth set-token`, `admins create-tenant`, `admins create-role-assignment`, `admins remove-role-assignment-by-id`, `admins deprovision-by-id`, `roles create-tenant`, `roles patch-tenant`, `roles delete-tenant-by-id`, `partner-admins create`, `partner-admins create-admin-role-assignment`, `partner-admins remove-admin-role-assignment-by-id`, `partner-admins delete-by-id`, `partner-roles create`, `partner-roles patch`, `partner-roles delete-by-id`, `org-admins create-organization-admin`, `org-admins create-admin-role-assignment`, `org-admins remove-admin-role-assignment-by-id`, `org-admins delete-organization-admin-by-id`, `org-roles create-organization-role`, `org-roles patch-organization-role`, `org-roles delete-organization-role-by-id` | Human-in-the-loop only |
| **Destructive and commercial** | Irreversible loss, moves endpoints to another tenant, or changes what a customer is billed. | `endpoints delete`, `endpoints bulk-delete`, `policies delete-policy`, `endpoint-groups delete-tenant-by-id`, `users delete-tenant-by-id`, `user-groups delete-tenant-by-id`, `enrichment-pivots delete`, `software delete-comment`, `migrations create-sending-job`, `tenants create`, `tenants update`, `tenants post-account-amendment`, `tenants post-product-selection`, `tenants cancel-pending-amendment`, `provisioning patch-default-products`, `provisioning reset-default-products`, `web-control clear-web-decryption-excluded-sites` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the credential.** A reporting workflow needs only Service Principal
  Read-Only. Create a separate credential with write access for the rare runs that
  need it.
- **Run the MCP server read-only unless an agent must act.** Set
  `SOPHOS_CENTRAL_MCP_READ_ONLY=1` and keep your AI app's approval prompt on.
- **Keep autonomous agents to Read.** Have a human approve every command below the
  Read tier - the gate lives in your agent's policy and in the credential's role,
  not in the binary's defaults (apart from `contain` and `fleet tamper`, which need
  `--yes`).
- **Never let an agent run Credential, Destructive, or Endpoint response commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md). Sophos shows
  the client secret only once and does not alert you when a credential expires.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Sophos Central API, and you can read every line of how it does so. The skill is
read-first, previews its fleet actions by default, and is scoped to your own account.
