# google-workspace skill - governance and safety model

> Unofficial. Community-built skill for the Google Workspace Admin APIs. Not affiliated
> with, endorsed by, or sponsored by Google LLC.
> This page tells an MSP owner exactly what the google-workspace skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `google-workspace-cli` binary (and `google-workspace-mcp`). There are
two ways to authenticate:

- **Client tenants (the MSP path).** Each client is registered with `tenants add`: a
  Google Cloud service account with domain-wide delegation that impersonates that
  client's super admin. `tenants add` copies the service-account JSON key into the
  CLI's config directory (`keys/<tenant>.json`, file mode 0600 on macOS and Linux; on Windows the file inherits the
  config folder's ACL, which under your user profile means your account, SYSTEM and Administrators) and records the tenant
  in `tenants.toml`. For every command the CLI mints a short-lived access token for the
  selected tenant, requesting exactly that tenant's configured scopes, and sends it only
  to Google. `tenants remove <name> --yes` deletes the stored key copy.
- **A single organization.** `GOOGLE_WORKSPACE_TOKEN` (or `ADMIN_ACCESS_TOKEN`, or a token
  saved with `auth set-token`) is used as a bearer token when no client tenant is
  registered; the cross-client reports then run against that one organization as the
  tenant `default`. Its power is whatever scopes the token carries. Unset it once you
  register tenants: with two or more registered and none selected, any generated API
  command (including `users delete` and the MCP raw-execute tool) falls back to this
  token and acts on its organization, not a client's. The cross-client reports never
  fall back.

The service-account key is the most sensitive thing this skill holds. Anyone with the key
file and a client's delegation can act as that client's super admin within the authorized
scopes, so treat the CLI's config directory like a password vault.

## Scope profiles: the real boundary

Each tenant mints only the scopes you chose with `tenants add --scopes`, and the client's
Admin console must authorize exactly those scopes for the service account's Client ID.
That authorization, not the CLI, is the hard limit. Profiles combine with commas and only
`full` includes `read`, so register the policy columns as `--scopes read,policy` (or
`full,policy`); `--scopes policy` alone leaves the reports nothing to read.

| Profile | Scopes | What it unlocks |
| --- | --- | --- |
| `read` (default) | Read-only Directory, Reports, Data Transfer; plus `admin.directory.user.security` and `apps.licensing` | Every report. **Not purely read-only** - see below. |
| `full` | `read` plus `admin.directory.user`, `admin.directory.group.member`, `admin.datatransfer` | `offboard --yes`: OU move, password scramble, suspend, group removal, Drive and Calendar transfer. **Also every user write**: `admin.directory.user` authorizes `users delete`, `users insert`, password changes and `users make-admin` (super admin), including through the MCP execute tool |
| `policy` | `cloud-identity.policies.readonly` | `posture` columns for current 2SV enforcement, Drive external sharing, Gmail auto-forwarding |
| `reseller` | `apps.order.readonly` | Reseller reads for your own reseller domain (register it with `--reseller`; it is excluded from `--all-tenants`) |

**Why `read` is not purely read-only.** Google has no read-only scope for OAuth tokens and
app passwords (`admin.directory.user.security`) or for license assignments
(`apps.licensing`). The reports need both, so the default profile also permits signing a
user out, revoking their OAuth tokens and app passwords, reading and regenerating their
backup verification codes, turning off their 2-Step Verification, and assigning or
removing licenses. The CLI never does any of that on its own, but an agent could call
those commands. Keep them in the human-in-the-loop tier below.

## Default-safe behavior

- **The cross-client reports only read.** `mfa-gaps`, `stale-licenses`, `leavers`,
  `posture`, `sharing-drift`, `oauth-apps`, and `login-alerts` issue GET requests and
  change nothing.
- **`offboard` previews by default.** Without `--yes` it reads the account and prints
  every step it would take; `--dry-run` prints the plan with no network calls at all.
  It acts only when `--yes` is typed on the command line: a saved run profile cannot
  supply it, and an MCP client cannot pass it, so over MCP `offboard` is preview-only.
- **`tenants remove` asks for `--yes`** before deleting a stored key.
- **`--dry-run` is opt-in for every other write.** The generated API commands (users,
  groups, licenses, devices, subscriptions) send immediately unless you pass `--dry-run`
  first. Make your agent's policy: preview, show the exact command, get approval, then
  run the write.
- **The MCP server has a read-only switch.** Set `GOOGLE_WORKSPACE_MCP_READ_ONLY` to
  exactly `1` (the "Read-only mode" field in Claude Desktop; `true` or `yes` are ignored and
  leave every tool enabled) and the server hides `offboard`, `sync`,
  `export`, and `tenants remove`, and refuses every non-GET API call (`import` is
  never exposed over MCP, because it reads a local file). Backup
  codes are still readable in that mode (`verification-codes list` is a GET), so the
  Credential / security tier below still applies.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, lists, and the local mirror. No change in Google. | `mfa-gaps`, `stale-licenses`, `leavers`, `posture`, `sharing-drift`, `oauth-apps`, `login-alerts`, `tenants list`, `tenants scopes`, `tenants test`, `users list`, `users get`, `groups list`, `activities list`, `sync`, `search`, `analytics`, `offboard "<email>"` without `--yes` | Allow |
| **Write (routine)** | Day-to-day directory changes. | `users insert`, `users patch`, `users aliases insert`, `groups insert`, `groups patch`, `members insert`, `orgunits insert`, `orgunits patch`, `resources calendars insert`, `tenants add` (local config only) | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Reads or changes sign-in secrets, sessions, and admin rights. | `verification-codes list` (returns a user's live backup codes), `verification-codes generate`, `verification-codes invalidate`, `two-step-verification` (turns 2SV off), `tokens delete`, `asps delete`, `users sign-out`, `users make-admin`, `role-assignments insert`, `role-assignments delete`, `roles insert`, `roles patch`, `roles update`, `tenants remove` | Human-in-the-loop only |
| **Data egress** | Tells Google to push a client's data to a URL you name. | `activities watch`, `users watch`, `users aliases watch` (`--address <url>`; they work under the default `read` profile) | Human-in-the-loop only |
| **Destructive / billing** | Irreversible data, device, or billing changes. | `offboard --yes`, `users delete`, `groups delete`, `members delete`, `domains delete`, `transfers insert` (moves Drive and Calendar ownership), `license-assignments insert`, `license-assignments update`, `license-assignments delete`, `subscriptions insert`, `subscriptions change-plan`, `subscriptions change-seats`, `subscriptions change-renewal-settings`, `subscriptions start-paid-service`, `subscriptions activate`, `subscriptions suspend`, `subscriptions delete`, `reseller-customers insert`, `mobiledevices action` (can wipe a device), `mobiledevices delete`, `chromeosdevices action`, `customer devices-chromeos issue-command`, `customer devices-chromeos batch-change-status`, `import` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Register reporting-only clients with the default `read` profile**, and add `full`
  only for clients where you offboard through the CLI: it authorizes every user write,
  including deleting users and granting super admin. A profile you never authorize in
  the client's Admin console cannot be used, whatever the agent asks for.
- **Keep autonomous agents to Read + previewed writes.** The gate lives in your agent's
  policy for every generated write command, not in the binary's defaults.
- **Never let an agent run Credential / security or Destructive / billing commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Protect the config directory.** It holds every client's service-account key. Keep
  it on an encrypted disk under your own account, and remove a client with
  `tenants remove <name> --yes` when you stop managing it.
- **Rotate a key if it is ever exposed** (for example after bridging the MCP server to
  a public endpoint for ChatGPT - see mcp-install.md): delete the key in Google Cloud,
  create a new one, and re-run `tenants add`.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply each client's service account, the client's own
super admin decides which scopes it gets, and you can read every line of how the binary
uses them. The cross-client reports are read-only and offboarding is preview-first.
