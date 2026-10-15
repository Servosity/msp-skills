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
  their plan and change nothing unless you add `--yes`.
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
| **Read** | Reports, rollups, search. No change in Sophos Central. | `fleet triage`, `fleet health`, `fleet gaps`, `fleet revenue`, `fleet refresh`, `fleet siem-collect`, `whoami`, `tenants list`, `alerts list`, `alerts search`, `endpoints list`, `endpoints get`, `health-check get`, `licenses list`, `billing`, `siem events`, `tamper-protection`, `search`, `sync`, and every other `list` / `get` except the credential reads below. `contain` and `fleet tamper` without `--yes` only preview. | Allow |
| **Endpoint response** | Acts on real customer machines. | `contain --yes`, `endpoints set-isolation`, `endpoints bulk-isolate`, `endpoints scan`, `endpoints update-check`, `endpoints collect-forensic-log`, `endpoints collect-memory-dump`, `endpoints update-aap`, `live-discover run-query` (runs a query on endpoints), `fleet tamper --yes`, `device-software`, `software add-special-static-package` | Human-in-the-loop, explicit confirmation |
| **Protection and policy config** | Changes what Sophos blocks, allows, scans or decrypts. An exclusion or allowed item can open a hole. | `exclusions add-scanning`, `exclusions add-isolation`, `exclusions add-intrusion-prevention` and their `update-*` / `delete-*` twins, `allowed-items post-by-id`, `allowed-items patch-by-id`, `allowed-items delete-by-id`, `blocked-items post-by-id`, `blocked-items delete-by-id`, `blocked-addresses post-blocked-address-sets`, `blocked-addresses delete-blocked-address-sets`, `policies create-policy`, `policies update-policy`, `policies update-settings-of-policy`, `policies clone-policy`, `policies reset-policy-settings`, `policies update-base-policy`, `exploit-mitigation add-application`, `web-control update-web-decryption-excluded-sites`, `web-control update-web-decryption-settings`, `event-journal update-settings` | Preview with `--dry-run`, then a human-approved write |
| **Write (routine)** | Day-to-day records with no direct protection effect. | `alerts act-on`, `endpoint-groups create-tenant`, `endpoint-groups add-multiple-endpoints-to-group`, `endpoint-tags`, `users create-tenant`, `user-groups create-tenant`, `health-check post-snooze`, `xdr start-query-run`, `xdr cancel-query-run`, `enrichment-pivots create`, `software update-comment` | Preview with `--dry-run`, then a reviewed write |
| **Credential and access** | Returns a live secret, or changes who can do what. | `endpoints get-tamper-protection` and `endpoints set-tamper-protection` (return the tamper protection password), `migrations get-job`, `migrations get-jobs`, `migrations create-receiving-job` (return the migration token), `auth login`, `auth set-token`, `partner-admins create`, `partner-admins create-admin-role-assignment`, `partner-roles create`, `org-admins create-organization-admin`, `org-roles create-organization-role`, `admins create-tenant`, `admins create-role-assignment`, `roles create-tenant`, `roles patch-tenant` | Human-in-the-loop only |
| **Destructive and commercial** | Irreversible loss, moves endpoints between tenants, or changes what a customer is billed. | `endpoints delete`, `endpoints bulk-delete`, `policies delete-policy`, `endpoint-groups delete-tenant-by-id`, `users delete-tenant-by-id`, `user-groups delete-tenant-by-id`, `admins deprovision-by-id`, `partner-admins delete-by-id`, `partner-roles delete-by-id`, `migrations create-sending-job`, `tenants create`, `tenants update`, `tenants post-account-amendment`, `tenants post-product-selection`, `provisioning patch-default-products`, `provisioning reset-default-products`, `web-control clear-web-decryption-excluded-sites` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the credential.** A reporting workflow needs only Service Principal
  Read-Only. Create a separate credential with write access for the rare runs that
  need it.
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
