---
name: cisco-duo
description: "Use when the user asks to fix a Cisco Duo MFA lockout, issue a Duo bypass code, move a user to a new phone, find which customer a Duo user belongs to, or see Duo risk and capacity across every MSP customer subaccount. Signed Duo Admin API calls from a CLI and MCP server. Trigger phrases: `user locked out of duo`, `issue a duo bypass code`, `reactivate duo mobile`, `duo mfa lockout`, `which customers are near their duo user limit`, `use cisco duo`, `run cisco-duo`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Cisco Duo"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cisco-duo-cli
    install:
      - kind: script
        bins: [cisco-duo-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.ps1
---

# Cisco Duo - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `cisco-duo-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.ps1 | iex
   ```
3. Verify: `cisco-duo-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `cisco-duo-cli` and `cisco-duo-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The Duo Admin API calls an MSP help desk uses (users, phones, bypass codes, groups, hardware tokens, admins, authentication logs and MSP subaccounts) signed with Duo v5 HMAC, plus read access to integrations, policies and settings. --account-id routes any command to one customer subaccount. On top: lockout triage and lockout fix for the 'locked out of MFA' ticket, phone-swap for lost phones, fleet summary and fleet risk across all customers, and local audits of who can skip MFA.

## When to Use This CLI

Use this CLI for Duo administration from a terminal or agent: unlocking users, issuing bypass codes, reactivating or replacing phones, managing users, groups, tokens and admins, reading authentication logs, and MSP work across many customer subaccounts from one parent Admin API application.

## Anti-triggers

Do not use this CLI for:
- Do not use it to perform end-user Duo authentication (that is the Duo Auth API or Web SDK).
- Do not use it to read or rotate integration secret keys or OAuth client secrets; those endpoints are intentionally not included.
- Do not use it for Duo policy authoring; policies are read-only here, use the Admin Panel to edit them.

## Unique Capabilities

### Lockout desk
- **`lockout triage`**  -  See why one user cannot complete Duo MFA: status, lockout reason, the group status that overrides it, phones, outstanding bypass codes, last 24h failures, and the exact next command to run.

  _Reach for this first on any 'locked out of Duo' ticket; it is read-only and tells you which fix applies._

  ```bash
  cisco-duo-cli lockout triage jdoe --account-id DA9VZOC5X63I2W72NRP9 --agent
  ```
- **`lockout fix`**  -  Unlock a locked-out user, and with --bypass issue a single-use one-hour bypass code or with --reactivate-phone re-send Duo Mobile activation; a preview by default, --apply makes the changes and prints a before/after receipt.

  _Use after lockout triage when the user is locked out but still has their phone or needs a temporary bypass code._

  ```bash
  cisco-duo-cli lockout fix jdoe --account-id DA9VZOC5X63I2W72NRP9 --bypass --reactivate-phone
  ```
- **`phone-swap`**  -  Move a user to a new phone number: reuse or create the phone, attach it and text the activation link; a preview by default, --apply makes the changes. Old phones are retired on a second run once the new phone is activated, and a phone shared with another user is never deleted.

  _Use for lost, replaced or re-numbered phones instead of hand-sequencing phone create, associate and activation calls._

  ```bash
  cisco-duo-cli phone-swap jdoe --number +15551234567 --remove-old
  ```
- **`failures`**  -  Roll up failed and fraud authentications per user, reason, factor and application over a window, with first and last seen and source IPs.

  _Use for weekly failure reviews and 'I am getting pushes I did not send' investigations._

  ```bash
  cisco-duo-cli failures --since 7d --top 20 --agent
  ```

### MSP fleet
- **`fleet summary`**  -  One row per customer subaccount: edition, users against user limit, telephony credits, admin and integration counts. Needs an Admin API app in the MSP parent account; covers every subaccount and ignores --account-id.

  _Use for the weekly MSP capacity and credit sweep or before a customer review._

  ```bash
  cisco-duo-cli fleet summary --agent
  ```
- **`fleet risk`**  -  Per-customer counts of users in bypass, locked out, disabled, pending deletion or unenrolled, members of Bypass or Disabled groups, and standing bypass codes. Needs an MSP parent Admin API app; ignores --account-id.

  _Use to spot which customers have MFA gaps before drilling into one tenant._

  ```bash
  cisco-duo-cli fleet risk --agent
  ```
- **`fleet find-user`**  -  Find which customer tenants (and the MSP parent) contain a username, alias or email, with status and last login for each hit. Needs an MSP parent Admin API app; ignores --account-id.

  _Use when a ticket arrives without the customer name, before lockout triage._

  ```bash
  cisco-duo-cli fleet find-user jdoe@example.com --agent
  ```

### Tenant hygiene
- **`audit bypass`**  -  List every synced user who can skip MFA: bypass status, a Bypass group, or a bypass code that never expires or has unlimited reuse, with the admin who issued it. Needs 'sync --resources users,bypass_codes' first; JSON reports complete=false when data is missing.

  _Use for security reviews and weekly hygiene in one tenant._

  ```bash
  cisco-duo-cli audit bypass --older-than 30d --agent
  ```
- **`audit stale`**  -  Find unenrolled users heading for a lockout, inactive and pending-deletion users, expired, inactive or never-logged-in admins, and phones not seen recently. Needs 'sync --resources users,admins,phones' first; JSON reports complete=false when data is missing.

  _Use for offboarding and onboarding follow-up passes._

  ```bash
  cisco-duo-cli audit stale --days 90 --agent
  ```
- **`enrollment chase`**  -  List users still unenrolled after a given age (by default) and, with --apply, re-send their Duo enrollment emails, oldest first, up to --limit.

  _Use to head off 'Not enrolled' lockouts for new hires._

  ```bash
  cisco-duo-cli enrollment chase --older-than 72h --limit 25
  ```

## Command Reference

**account_info**  -  Manage account info

- `cisco-duo-cli account-info get-auth-attempts`  -  Authentication attempt counts by result (default last 30 days).
- `cisco-duo-cli account-info get-summary`  -  Account summary: users, admins, integrations, telephony credits, edition.
- `cisco-duo-cli account-info get-telephony-credits-used`  -  Telephony credits used in a window (default last 30 days).
- `cisco-duo-cli account-info get-user-auth-attempts`  -  Number of users with authentication attempts (default last 30 days).

**accounts**  -  Manage accounts

- `cisco-duo-cli accounts create`  -  Create a subaccount.
- `cisco-duo-cli accounts delete`  -  Delete a subaccount.
- `cisco-duo-cli accounts list`  -  List subaccounts of the MSP parent account.

**admin_roles**  -  Manage admin roles

- `cisco-duo-cli admin-roles get`  -  Get an administrator role.
- `cisco-duo-cli admin-roles list`  -  List administrator roles.

**admin_units**  -  Manage admin units

- `cisco-duo-cli admin-units get`  -  Get an administrative unit.
- `cisco-duo-cli admin-units list`  -  List administrative units.

**admins**  -  Manage admins

- `cisco-duo-cli admins clear-inactivity`  -  Clear an administrator's inactivity expiration.
- `cisco-duo-cli admins create`  -  Create an administrator (activation link flow).
- `cisco-duo-cli admins delete`  -  Delete an administrator.
- `cisco-duo-cli admins get`  -  Get an administrator.
- `cisco-duo-cli admins list`  -  List administrators.
- `cisco-duo-cli admins list-activations`  -  List pending administrator activations.
- `cisco-duo-cli admins reset`  -  Reset an administrator's failed authentication attempt count (admin lockout).
- `cisco-duo-cli admins update`  -  Modify an administrator.

**billing**  -  Manage billing

- `cisco-duo-cli billing get-edition`  -  Get a subaccount's Duo edition.
- `cisco-duo-cli billing get-telephony-credits`  -  Get a subaccount's telephony credits.
- `cisco-duo-cli billing get-user-limit`  -  Get a subaccount's user limit and user count.
- `cisco-duo-cli billing set-edition`  -  Set a subaccount's Duo edition.
- `cisco-duo-cli billing set-telephony-credits`  -  Set a subaccount's telephony credit total (moved from the parent).
- `cisco-duo-cli billing set-user-limit`  -  Set or remove a subaccount's user limit.

**bypass_codes**  -  Manage bypass codes

- `cisco-duo-cli bypass-codes delete`  -  Delete (revoke) a bypass code.
- `cisco-duo-cli bypass-codes get`  -  Get bypass code metadata.
- `cisco-duo-cli bypass-codes list`  -  List bypass code metadata across all users.

**desktop_authenticators**  -  Manage desktop authenticators

- `cisco-duo-cli desktop-authenticators delete`  -  Delete a Duo Desktop authenticator.
- `cisco-duo-cli desktop-authenticators get`  -  Get a Duo Desktop authenticator.
- `cisco-duo-cli desktop-authenticators list`  -  List Duo Desktop authenticators.

**directory_sync**  -  Manage directory sync

- `cisco-duo-cli directory-sync list`  -  List external directory syncs.
- `cisco-duo-cli directory-sync sync-user`  -  Create or update one user from an external directory.

**endpoints**  -  Manage endpoints

- `cisco-duo-cli endpoints get`  -  Get an endpoint.
- `cisco-duo-cli endpoints list`  -  List endpoints (devices that accessed Duo apps).

**groups**  -  Manage groups

- `cisco-duo-cli groups create`  -  Create a group.
- `cisco-duo-cli groups delete`  -  Delete a group.
- `cisco-duo-cli groups get`  -  Get a group.
- `cisco-duo-cli groups list`  -  List groups.
- `cisco-duo-cli groups list-members`  -  List a group's members.
- `cisco-duo-cli groups update`  -  Update a group.

**identity_verification**  -  Manage identity verification

- `cisco-duo-cli identity-verification cancel`  -  Cancel an identity verification.
- `cisco-duo-cli identity-verification get`  -  Get identity verification status for a user.
- `cisco-duo-cli identity-verification start`  -  Start a Duo identity verification for a user.

**integrations**  -  Manage integrations

- `cisco-duo-cli integrations get`  -  Get an integration.
- `cisco-duo-cli integrations list`  -  List integrations (applications); secret keys are not requested.

**logs**  -  Manage logs

- `cisco-duo-cli logs get-activity`  -  Activity log events (v2).
- `cisco-duo-cli logs get-admin`  -  Administrator action log (v1, 1000 per call).
- `cisco-duo-cli logs get-auth`  -  Authentication log events (v2, filterable).
- `cisco-duo-cli logs get-offline-enrollment`  -  Offline enrollment log (v1).
- `cisco-duo-cli logs get-telephony`  -  Telephony log events (v2).

**passport**  -  Manage passport

- `cisco-duo-cli passport`  -  Get Duo Passport configuration.

**phones**  -  Manage phones

- `cisco-duo-cli phones create`  -  Create a phone.
- `cisco-duo-cli phones create-activation-url`  -  Create a Duo Mobile activation code/URL for a phone (reactivation).
- `cisco-duo-cli phones delete`  -  Delete a phone.
- `cisco-duo-cli phones get`  -  Get a phone.
- `cisco-duo-cli phones list`  -  List phones, or look one up by number.
- `cisco-duo-cli phones send-sms-activation`  -  Text a Duo Mobile activation link to a phone.
- `cisco-duo-cli phones send-sms-installation`  -  Text Duo Mobile install instructions to a phone.
- `cisco-duo-cli phones send-sms-passcodes`  -  Text a new batch of SMS passcodes to a phone.
- `cisco-duo-cli phones set-quarantine-status`  -  Quarantine or release a device.
- `cisco-duo-cli phones update`  -  Modify a phone.

**policies**  -  Manage policies

- `cisco-duo-cli policies calculate-policy`  -  Show the effective policy for a user in an application.
- `cisco-duo-cli policies get-global-policy`  -  Get the global policy.
- `cisco-duo-cli policies get-policy`  -  Get a policy.
- `cisco-duo-cli policies list`  -  List policies with all sections.
- `cisco-duo-cli policies summarize`  -  Summarize policies and where they apply.

**registered_devices**  -  Manage registered devices

- `cisco-duo-cli registered-devices block-device`  -  Block a registered device.
- `cisco-duo-cli registered-devices list`  -  List Duo Passport / registered devices.
- `cisco-duo-cli registered-devices list-blocked-devices`  -  List blocked registered devices.
- `cisco-duo-cli registered-devices unblock-device`  -  Unblock a registered device.

**settings**  -  Manage settings

- `cisco-duo-cli settings`  -  Get account settings.

**tokens**  -  Manage tokens

- `cisco-duo-cli tokens delete`  -  Delete a hardware token.
- `cisco-duo-cli tokens get`  -  Get a hardware token.
- `cisco-duo-cli tokens list`  -  List hardware tokens.
- `cisco-duo-cli tokens resync`  -  Resynchronize an HOTP or Duo-D100 token with three successive codes.

**trust_monitor**  -  Manage trust monitor

- `cisco-duo-cli trust-monitor`  -  Trust Monitor events.

**users**  -  Manage users

- `cisco-duo-cli users add-group`  -  Add a user to a group.
- `cisco-duo-cli users add-phone`  -  Associate a phone with a user.
- `cisco-duo-cli users add-token`  -  Associate a hardware token with a user.
- `cisco-duo-cli users bulk-create`  -  Create up to 100 users in one call.
- `cisco-duo-cli users bulk-restore`  -  Restore up to 100 users from the Trash.
- `cisco-duo-cli users bulk-trash`  -  Send up to 100 users to the Trash.
- `cisco-duo-cli users create`  -  Create a user.
- `cisco-duo-cli users create-bypass-codes`  -  Generate bypass codes for a user (replaces existing codes unless --preserve-existing).
- `cisco-duo-cli users delete`  -  Permanently delete a user (API deletes skip the Trash; use users bulk-trash for a 7-day restore window).
- `cisco-duo-cli users enroll`  -  Email an enrollment link to a new user.
- `cisco-duo-cli users get`  -  Get a user by ID (with phones, tokens, groups).
- `cisco-duo-cli users get-verification-push-response`  -  Check the result of a verification push.
- `cisco-duo-cli users list`  -  List users, or look one up by username or email.
- `cisco-duo-cli users list-bypass-codes`  -  List bypass code metadata for a user (codes themselves are never returned).
- `cisco-duo-cli users list-desktop-authenticators`  -  List a user's Duo Desktop authenticators.
- `cisco-duo-cli users list-groups`  -  List a user's groups.
- `cisco-duo-cli users list-phones`  -  List a user's phones.
- `cisco-duo-cli users list-tokens`  -  List a user's hardware tokens.
- `cisco-duo-cli users list-webauthn`  -  List a user's WebAuthn credentials.
- `cisco-duo-cli users remove-group`  -  Remove a user from a group.
- `cisco-duo-cli users remove-phone`  -  Disassociate a phone from a user (the phone itself is kept).
- `cisco-duo-cli users remove-token`  -  Disassociate a hardware token from a user.
- `cisco-duo-cli users send-verification-push`  -  Send a Duo Push to confirm a user's identity (returns a confirmation code).
- `cisco-duo-cli users update`  -  Modify a user; --status active unlocks a locked-out user.

**webauthn**  -  Manage webauthn

- `cisco-duo-cli webauthn delete`  -  Delete a WebAuthn credential.
- `cisco-duo-cli webauthn get`  -  Get a WebAuthn credential.
- `cisco-duo-cli webauthn list`  -  List WebAuthn credentials.


**audit**


- `cisco-duo-cli audit bypass`  -  List every synced user who can skip MFA: bypass status, a Bypass group, or a bypass code that never expires or has unlimited reuse, with the admin who issued it. Needs 'sync --resources users,bypass_codes' first; JSON reports complete=false when data is missing.

- `cisco-duo-cli audit stale`  -  Find unenrolled users heading for a lockout, inactive and pending-deletion users, expired, inactive or never-logged-in admins, and phones not seen recently. Needs 'sync --resources users,admins,phones' first; JSON reports complete=false when data is missing.


**enrollment**


- `cisco-duo-cli enrollment chase`  -  List users still unenrolled after a given age (by default) and, with --apply, re-send their Duo enrollment emails, oldest first, up to --limit.


**failures**


- `cisco-duo-cli failures`  -  Roll up failed and fraud authentications per user, reason, factor and application over a window, with first and last seen and source IPs.


**fleet**


- `cisco-duo-cli fleet summary`  -  One row per customer subaccount: edition, users against user limit, telephony credits, admin and integration counts. Needs an Admin API app in the MSP parent account; covers every subaccount and ignores --account-id.

- `cisco-duo-cli fleet risk`  -  Per-customer counts of users in bypass, locked out, disabled, pending deletion or unenrolled, members of Bypass or Disabled groups, and standing bypass codes. Needs an MSP parent Admin API app; ignores --account-id.

- `cisco-duo-cli fleet find-user`  -  Find which customer tenants (and the MSP parent) contain a username, alias or email, with status and last login for each hit. Needs an MSP parent Admin API app; ignores --account-id.


**lockout**


- `cisco-duo-cli lockout triage`  -  See why one user cannot complete Duo MFA: status, lockout reason, the group status that overrides it, phones, outstanding bypass codes, last 24h failures, and the exact next command to run.

- `cisco-duo-cli lockout fix`  -  Unlock a locked-out user, and with --bypass issue a single-use one-hour bypass code or with --reactivate-phone re-send Duo Mobile activation; a preview by default, --apply makes the changes and prints a before/after receipt.


**phone-swap**


- `cisco-duo-cli phone-swap`  -  Move a user to a new phone number: reuse or create the phone, attach it and text the activation link; a preview by default, --apply makes the changes. Old phones are retired on a second run once the new phone is activated, and a phone shared with another user is never deleted.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cisco-duo-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Locked-out caller, end to end

```bash
cisco-duo-cli lockout fix jdoe --account-id DA9VZOC5X63I2W72NRP9 --bypass --reactivate-phone
```

Prints the unlock, bypass code and SMS reactivation plan; add --apply to make the changes and get a before/after receipt.

### Narrow a user lookup for an agent

```bash
cisco-duo-cli lockout triage jdoe --agent --select effective_status,lockout_reason,next_actions
```

Returns only the diagnosis fields an agent needs to choose the fix.

### Monday MSP sweep

```bash
cisco-duo-cli fleet summary --agent
```

Edition, user-limit pressure and telephony credits for every customer subaccount.

### Who can skip MFA

```bash
cisco-duo-cli audit bypass --older-than 30d --agent
```

Run 'cisco-duo-cli sync --resources users,bypass_codes' first; lists bypass-status users, Bypass-group members and standing bypass codes, and reports complete=false if data is missing.

### Last week's failures

```bash
cisco-duo-cli failures --since 7d --top 20 --agent
```

Per-user failure counts by reason, factor and application.

## Auth Setup

Duo signs every Admin API request with HMAC-SHA512. In the Duo Admin Panel create an Admin API application (in the MSP parent account, with subaccount permissions, to manage customers) and set three variables: CISCO_DUO_IKEY (integration key), CISCO_DUO_SKEY (secret key) and CISCO_DUO_API_HOST (the API hostname shown there, e.g. api-xxxxxxxx.duosecurity.com). The CLI computes a fresh signature for every request and never sends the secret key. Pass --account-id (an account ID or exact name from 'accounts list'), or set CISCO_DUO_ACCOUNT_ID, to run any command against one customer subaccount; the CLI looks up that subaccount's API host and keeps a separate local store for it.

Run `cisco-duo-cli doctor` to verify setup: it sends one signed read and reports the credential as valid, rejected (401) or scope-limited (403); add `--fail-on warn` to make a scope-limited key exit non-zero. When pinning a subaccount, pass the account ID rather than the name every time: the local store is keyed by exactly what you pass.

## Safety before any fix

A caller who says "I'm locked out" may be the attacker MFA was there to stop. Before running `lockout fix --apply`, `phone-swap --apply`, `users create-bypass-codes`, `phones create-activation-url`, `users enroll` (it returns the enrollment code to the caller), `enrollment chase --apply` or `phones update --number`:

1. Run the command WITHOUT `--apply` (or with `--dry-run` for generated commands) and show the user the plan.
2. Confirm the caller's identity through a channel other than the ticket, for example `cisco-duo-cli users send-verification-push <user_id> --phone-id <phone_id>` (both IDs are in the `lockout triage` output) sends a Duo Push to a device they already have.
3. Get explicit human approval, then run with `--apply`.

Never issue a bypass code, activation link or new phone on the strength of a ticket alone. A user an administrator deliberately held (disabled, in the Trash, locked from Trust Monitor or through the Admin API) or in a Disabled group gets no bypass code, activation or new phone from `lockout fix` or `phone-swap` unless you pass `--force`. A phone that other users share is never reactivated or reused without `--force` either. Never add `--force` without that human approval. Generated write commands (`users update`, `phones delete`, `users create-bypass-codes` and the rest) send immediately in `--agent` mode and are not gated by `--yes`; preview them with `--dry-run`. Deleting users or factors, administrator changes, billing changes and `accounts create` / `accounts delete` are operator-only. See [governance.md](./governance.md).

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  cisco-duo-cli accounts list --agent --select account_id,api_hostname,name
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `CISCO_DUO_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CISCO_DUO_CONFIG_DIR`, `CISCO_DUO_DATA_DIR`, `CISCO_DUO_STATE_DIR`, `CISCO_DUO_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CISCO_DUO_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `cisco-duo-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "cisco-duo": {
        "command": "cisco-duo-mcp",
        "env": {
          "CISCO_DUO_HOME": "/srv/cisco-duo"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CISCO_DUO_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CISCO_DUO_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
cisco-duo-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "cisco-duo-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `cisco-duo-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `cisco-duo-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `cisco-duo-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
cisco-duo-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
cisco-duo-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
cisco-duo-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
cisco-duo-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`cisco-duo-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CISCO_DUO_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cisco-duo-cli feedback "the --since flag is inclusive but docs say exclusive"
cisco-duo-cli feedback --stdin < notes.txt
cisco-duo-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CISCO_DUO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CISCO_DUO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
cisco-duo-cli profile save briefing --json
cisco-duo-cli --profile briefing accounts list
cisco-duo-cli profile list --json
cisco-duo-cli profile show briefing
cisco-duo-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `cisco-duo-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add cisco-duo-mcp -- cisco-duo-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which cisco-duo-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   cisco-duo-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `cisco-duo-cli <command> --help`.
