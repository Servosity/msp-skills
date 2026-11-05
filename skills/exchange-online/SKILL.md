---
name: exchange-online
description: "Exchange Online admin for MSPs over REST: forwarding audits, EWS posture, offboarding and quarantine across client tenants, no PowerShell session. Trigger phrases: `audit mailbox forwarding`, `which mailboxes still have EWS on`, `offboard this user's mailbox`, `why didn't this email arrive`, `release quarantined email`, `use exchange-online`, `run exchange-online`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Microsoft Exchange Online"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - exchange-online-cli
    install:
      - kind: script
        bins: [exchange-online-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.ps1
---

# Microsoft Exchange Online - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `exchange-online-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.ps1 | iex
   ```
3. Verify: `exchange-online-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `exchange-online-cli` and `exchange-online-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Runs the Exchange cmdlets MSPs use every week through Microsoft's REST admin endpoints, one customer tenant per call, with JSON output and an MCP server. On top of the raw cmdlets it adds forwarding-audit, inbox-rules sweep and bec triage for compromise work, ews-posture for the EWS shutdown, offboard with a per-step receipt, and a cross-tenant quarantine queue.

## When to Use This CLI

Use this CLI for Exchange Online administration in a customer tenant: mailbox inventory and changes, forwarding and inbox-rule audits, client protocol posture, mailbox and folder permissions, quarantine, message trace, mobile devices and mail-flow policies. Limits: it has not yet been exercised against a live tenant; the Admin API v2.0 commands are a Microsoft preview not enabled in every tenant; the other commands use the InvokeCommand route that Microsoft does not document; message trace covers 90 days; fan-out commands stop at 2000 mailboxes unless --max-mailboxes is raised; writes run immediately, so preview them with --dry-run (or offboard --plan).

## Anti-triggers

Do not use this CLI for:
- Users, licenses, sign-in blocks, session revocation and Microsoft 365 Groups: use a Microsoft Graph tool
- Purview compliance (retention, DLP, eDiscovery): not covered
- Exchange Server on-premises: not supported
- Bulk creation of transport rules or policies: use Exchange Online PowerShell

## Unique Capabilities

### Security posture
- **`ews-posture`**  -  See which mailboxes still have EWS, IMAP, POP, ActiveSync or SMTP AUTH effectively on once org defaults are applied.

  _Reach for it while retiring EWS (Microsoft scheduled its Exchange Online deprecation for October 2026) or any legacy-auth cleanup, instead of exporting Get-CASMailbox to a spreadsheet._

  ```bash
  exchange-online-cli ews-posture 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent
  ```

### Offboarding
- **`offboard`**  -  Run the Exchange side of a leaver in one pass: shared mailbox, GAL hide, mobile device removal, plus auto-reply (--auto-reply), manager FullAccess and Send As (--manager) and forwarding (--forward-to), with a per-step receipt; --plan only reports.

  _Use it for leaver tickets so every Exchange step is proven, not assumed._

  ```bash
  exchange-online-cli offboard 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --manager boss@example.com --plan --agent
  ```
- **`access-map`**  -  List every mailbox one user can open, send as, or send on behalf of.

  _Run it before an offboard or to size a compromised account's blast radius._

  ```bash
  exchange-online-cli access-map 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --agent
  ```

### Business email compromise
- **`forwarding-audit`**  -  List every way mail can auto-forward out of a tenant and whether the outbound spam policy or remote-domain settings block it; inbox rules come from the last 'inbox-rules sweep' unless you pass --sweep.

  _The first question in any business email compromise review: is mail leaving this tenant?_

  ```bash
  exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent
  ```
- **`bec triage`**  -  Collect compromise evidence for one mailbox: inbox rules, forwarding, restricted-sender status, new mobile devices, outbound mail volume and legacy protocols.

  _Use it when a client says a colleague sent something odd._

  ```bash
  exchange-online-cli bec triage 0550b473-9fd2-4dbb-a058-a1640b4bf458 alex@example.com --agent
  ```
- **`inbox-rules sweep`**  -  Pull every mailbox's inbox rules in one tenant and flag the classic attacker rules.

  _Inbox rules exist only per mailbox; this is the tenant-wide view._

  ```bash
  exchange-online-cli inbox-rules sweep 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent
  ```

### Quarantine desk
- **`quarantine snapshot`**  -  Save one tenant's pending quarantine release requests locally so the cross-tenant queue can show them.

  _Run it per tenant (each with its own token) before reading quarantine queue._

  ```bash
  exchange-online-cli quarantine snapshot 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent
  ```
- **`quarantine queue`**  -  One oldest-first queue of quarantine release requests across every tenant saved with 'quarantine snapshot' (run it per tenant first).

  _Use it for the daily quarantine desk pass instead of one admin portal tab per client._

  ```bash
  exchange-online-cli quarantine queue --older-than 24h --agent
  ```
- **`where-is`**  -  Explain why a message did or did not reach a recipient, with the next action to take.

  _Answers the daily missing-email ticket in one call._

  ```bash
  exchange-online-cli where-is 0550b473-9fd2-4dbb-a058-a1640b4bf458 --recipient alex@example.com --since 48h --agent
  ```

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

`--ca-cert`, `EXCHANGE_ONLINE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `EXCHANGE_ONLINE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. `doctor` reports the trust in use and names the setting that fixes a chain or hostname failure.

## Command Reference

**accepted_domains**  -  List accepted domains and their routing type (Admin API v2.0).

- `exchange-online-cli accepted-domains <tenantId>`  -  List accepted domains and their routing type (Admin API v2.0).

**anti_phish_policies**  -  List anti-phishing policies.

- `exchange-online-cli anti-phish-policies <tenantId>`  -  List anti-phishing policies.

**audit_config**  -  Audit configuration, including UnifiedAuditLogIngestionEnabled.

- `exchange-online-cli audit-config <tenantId>`  -  Audit configuration, including UnifiedAuditLogIngestionEnabled.

**auto_reply**  -  A mailbox's automatic reply (out of office) state, audience, messages and window.

- `exchange-online-cli auto-reply get`  -  A mailbox's automatic reply (out of office) state, audience, messages and window.
- `exchange-online-cli auto-reply set`  -  Turn automatic replies on, off or scheduled, and set the messages.

**blocked_senders**  -  List users blocked from sending mail (restricted after outbound spam).

- `exchange-online-cli blocked-senders list`  -  List users blocked from sending mail (restricted after outbound spam).
- `exchange-online-cli blocked-senders remove`  -  Unblock a restricted sender.

**client_access**  -  Per-mailbox client protocols: EWS, OWA, MAPI, IMAP, POP, ActiveSync, SMTP AUTH.

- `exchange-online-cli client-access list`  -  Per-mailbox client protocols: EWS, OWA, MAPI, IMAP, POP, ActiveSync, SMTP AUTH.
- `exchange-online-cli client-access set`  -  Turn client protocols on or off for one mailbox.

**distribution_groups**  -  List distribution and mail-enabled security groups.

- `exchange-online-cli distribution-groups add-member`  -  Add a member to a distribution or mail-enabled security group.
- `exchange-online-cli distribution-groups list`  -  List distribution and mail-enabled security groups.
- `exchange-online-cli distribution-groups remove-member`  -  Remove a member from a distribution or mail-enabled security group.

**dkim**  -  DKIM signing configuration per domain (enabled state, selector CNAMEs).

- `exchange-online-cli dkim <tenantId>`  -  DKIM signing configuration per domain (enabled state, selector CNAMEs).

**domains**  -  List accepted domains through the cmdlet route (works where the v2.0 preview is not enabled).

- `exchange-online-cli domains <tenantId>`  -  List accepted domains through the cmdlet route (works where the v2.0 preview is not enabled).

**dynamic_group_members**  -  List the computed members of a dynamic distribution group (Admin API v2.0).

- `exchange-online-cli dynamic-group-members <tenantId>`  -  List the computed members of a dynamic distribution group (Admin API v2.0).

**folder_permissions**  -  List permissions on a mailbox folder (Admin API v2.0).

- `exchange-online-cli folder-permissions add`  -  Grant a user permissions on a mailbox folder (Admin API v2.0).
- `exchange-online-cli folder-permissions list`  -  List permissions on a mailbox folder (Admin API v2.0).
- `exchange-online-cli folder-permissions remove`  -  Remove a user's permissions from a mailbox folder (Admin API v2.0).
- `exchange-online-cli folder-permissions set`  -  Change a user's permissions on a mailbox folder (Admin API v2.0).

**group_members**  -  List the members of a distribution group (Admin API v2.0).

- `exchange-online-cli group-members <tenantId>`  -  List the members of a distribution group (Admin API v2.0).

**inbound_connectors**  -  List inbound mail connectors.

- `exchange-online-cli inbound-connectors <tenantId>`  -  List inbound mail connectors.

**inbox_rules**  -  List a mailbox's inbox rules (forward, redirect, delete and move actions included).

- `exchange-online-cli inbox-rules list`  -  List a mailbox's inbox rules (forward, redirect, delete and move actions included).
- `exchange-online-cli inbox-rules remove`  -  Delete one inbox rule.

**mail_contacts**  -  List mail contacts.

- `exchange-online-cli mail-contacts <tenantId>`  -  List mail contacts.

**mailbox_folders**  -  List a mailbox's folders with item counts, sizes and folder types (find a localized Calendar folder).

- `exchange-online-cli mailbox-folders <tenantId>`  -  List a mailbox's folders with item counts, sizes and folder types (find a localized Calendar folder).

**mailbox_permissions**  -  List mailbox-level permissions (FullAccess and others) on a mailbox.

- `exchange-online-cli mailbox-permissions add`  -  Grant a user FullAccess (or other rights) to a mailbox.
- `exchange-online-cli mailbox-permissions list`  -  List mailbox-level permissions (FullAccess and others) on a mailbox.
- `exchange-online-cli mailbox-permissions remove`  -  Remove a user's FullAccess (or other rights) from a mailbox.

**mailbox_stats**  -  Mailbox size and item counts (primary or archive).

- `exchange-online-cli mailbox-stats <tenantId>`  -  Mailbox size and item counts (primary or archive).

**mailboxes**  -  List mailboxes with type, forwarding, hold and archive state.

- `exchange-online-cli mailboxes list`  -  List mailboxes with type, forwarding, hold and archive state.
- `exchange-online-cli mailboxes set`  -  Change a mailbox: convert type (Shared, Regular, Room, Equipment), set or clear forwarding, hide from the address list.

**malware_policies**  -  List anti-malware policies.

- `exchange-online-cli malware-policies <tenantId>`  -  List anti-malware policies.

**message_trace**  -  Trace messages by sender, recipient, message ID and date window.

- `exchange-online-cli message-trace detail`  -  Delivery events for one traced message and recipient.
- `exchange-online-cli message-trace list`  -  Trace messages by sender, recipient, message ID and date window.

**mobile_devices**  -  List a mailbox's ActiveSync / mobile device partnerships.

- `exchange-online-cli mobile-devices list`  -  List a mailbox's ActiveSync / mobile device partnerships.
- `exchange-online-cli mobile-devices remove`  -  Remove a mobile device partnership (does not wipe the handset).
- `exchange-online-cli mobile-devices wipe`  -  Wipe a mobile device (or only the mailbox account on it) the next time it connects.

**org_config**  -  Organization MailTips settings and limits (Admin API v2.0).

- `exchange-online-cli org-config <tenantId>`  -  Organization MailTips settings and limits (Admin API v2.0).

**org_settings**  -  Full organization configuration, including EwsEnabled, EwsAllowList and EwsApplicationAccessPolicy.

- `exchange-online-cli org-settings <tenantId>`  -  Full organization configuration, including EwsEnabled, EwsAllowList and EwsApplicationAccessPolicy.

**outbound_connectors**  -  List outbound mail connectors.

- `exchange-online-cli outbound-connectors <tenantId>`  -  List outbound mail connectors.

**outbound_spam_policies**  -  List outbound spam policies, including AutoForwardingMode and sending limits.

- `exchange-online-cli outbound-spam-policies <tenantId>`  -  List outbound spam policies, including AutoForwardingMode and sending limits.

**quarantine**  -  List quarantined messages.

- `exchange-online-cli quarantine delete`  -  Delete quarantined messages.
- `exchange-online-cli quarantine list`  -  List quarantined messages.
- `exchange-online-cli quarantine release`  -  Release, approve or deny quarantined messages.

**quarantine_policies**  -  List quarantine policies (what users may do with quarantined mail).

- `exchange-online-cli quarantine-policies <tenantId>`  -  List quarantine policies (what users may do with quarantined mail).

**recipients**  -  Look up any mail recipient (mailbox, group, contact, mail user).

- `exchange-online-cli recipients <tenantId>`  -  Look up any mail recipient (mailbox, group, contact, mail user).

**remote_domains**  -  List remote domains, including AutoForwardEnabled and TNEF settings.

- `exchange-online-cli remote-domains <tenantId>`  -  List remote domains, including AutoForwardEnabled and TNEF settings.

**safe_attachment_policies**  -  List Defender for Office 365 Safe Attachments policies.

- `exchange-online-cli safe-attachment-policies <tenantId>`  -  List Defender for Office 365 Safe Attachments policies.

**safe_links_policies**  -  List Defender for Office 365 Safe Links policies.

- `exchange-online-cli safe-links-policies <tenantId>`  -  List Defender for Office 365 Safe Links policies.

**send_as**  -  List Send As permissions on a recipient, or every recipient a trustee can send as.

- `exchange-online-cli send-as add`  -  Grant a trustee Send As on a recipient.
- `exchange-online-cli send-as list`  -  List Send As permissions on a recipient, or every recipient a trustee can send as.
- `exchange-online-cli send-as remove`  -  Remove a trustee's Send As on a recipient.

**send_on_behalf**  -  List mailboxes with their Send on behalf delegates (Admin API v2.0).

- `exchange-online-cli send-on-behalf list`  -  List mailboxes with their Send on behalf delegates (Admin API v2.0).
- `exchange-online-cli send-on-behalf set`  -  Replace a mailbox's Send on behalf delegate list (Admin API v2.0).

**sharing_policies**  -  List calendar and contact sharing policies.

- `exchange-online-cli sharing-policies <tenantId>`  -  List calendar and contact sharing policies.

**spam_policies**  -  List inbound anti-spam (hosted content filter) policies.

- `exchange-online-cli spam-policies <tenantId>`  -  List inbound anti-spam (hosted content filter) policies.

**transport_config**  -  Organization transport settings, including the SMTP AUTH default (SmtpClientAuthenticationDisabled).

- `exchange-online-cli transport-config <tenantId>`  -  Organization transport settings, including the SMTP AUTH default (SmtpClientAuthenticationDisabled).

**transport_rules**  -  List mail flow (transport) rules.

- `exchange-online-cli transport-rules disable`  -  Disable a mail flow rule.
- `exchange-online-cli transport-rules enable`  -  Enable a mail flow rule.
- `exchange-online-cli transport-rules list`  -  List mail flow (transport) rules.


**access-map**


- `exchange-online-cli access-map <tenantId> <user>`  -  List every mailbox one user can open, send as, or send on behalf of.


**bec**


- `exchange-online-cli bec triage <tenantId> <mailbox>`  -  Collect compromise evidence for one mailbox: inbox rules, forwarding, restricted-sender status, new mobile devices, outbound mail volume and legacy protocols.


**ews-posture**


- `exchange-online-cli ews-posture <tenantId>`  -  See which mailboxes still have EWS, IMAP, POP, ActiveSync or SMTP AUTH effectively on once org defaults are applied.


**forwarding-audit**


- `exchange-online-cli forwarding-audit <tenantId>`  -  List every way mail can auto-forward out of a tenant and whether the outbound spam policy or remote-domain settings block it; inbox rules come from the last 'inbox-rules sweep' unless you pass --sweep.


**inbox-rules**


- `exchange-online-cli inbox-rules sweep <tenantId>`  -  Pull every mailbox's inbox rules in one tenant and flag the classic attacker rules.


**offboard**


- `exchange-online-cli offboard <tenantId> <mailbox>`  -  Run the Exchange side of a leaver in one pass: shared mailbox, GAL hide, mobile device removal, plus auto-reply (--auto-reply), manager FullAccess and Send As (--manager) and forwarding (--forward-to), with a per-step receipt; --plan only reports.


**quarantine**


- `exchange-online-cli quarantine snapshot <tenantId>`  -  Save one tenant's pending quarantine release requests locally so the cross-tenant queue can show them.

- `exchange-online-cli quarantine queue`  -  One oldest-first queue of quarantine release requests across every tenant saved with 'quarantine snapshot' (run it per tenant first).


**where-is**


- `exchange-online-cli where-is <tenantId> --recipient <address>`  -  Explain why a message did or did not reach a recipient, with the next action to take.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
exchange-online-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Shared mailboxes and their forwarding

```bash
exchange-online-cli mailboxes list 0550b473-9fd2-4dbb-a058-a1640b4bf458 --recipient-type-details SharedMailbox --properties UserPrincipalName,ForwardingSmtpAddress,DeliverToMailboxAndForward --agent --select UserPrincipalName,ForwardingSmtpAddress
```

Server-side type filter and property selection keep the payload small.

### Who can open this mailbox

```bash
exchange-online-cli mailbox-permissions list 0550b473-9fd2-4dbb-a058-a1640b4bf458 --identity shared@example.com --agent
```

FullAccess and other mailbox rights, including inherited entries.

### Convert a leaver to shared

```bash
exchange-online-cli mailboxes set 0550b473-9fd2-4dbb-a058-a1640b4bf458 --identity jane@example.com --type Shared --dry-run
```

Preview the cmdlet call before sending it.

### Release requests waiting longest

```bash
exchange-online-cli quarantine queue --older-than 24h --agent
```

Cross-tenant, oldest first, from snapshots saved per tenant with 'quarantine snapshot <tenant-id>'.

## Auth Setup

Calls need an OAuth access token for the resource https://outlook.office365.com, minted for the customer tenant you are working in (GDAP partners mint one per tenant; tokens last about an hour). Export it as EXCHANGE_ONLINE_TOKEN, for example export EXCHANGE_ONLINE_TOKEN=$(az account get-access-token --resource https://outlook.office365.com --tenant <tenant-id> --query accessToken -o tsv). App-only tokens need Exchange.ManageAsApp (cmdlet commands) or Exchange.ManageAsAppV2 (the Admin API v2.0 commands) plus an Exchange role such as Exchange Administrator. Tenant-scoped commands take the tenant ID as their first argument; quarantine queue reads local snapshots and takes --tenant instead.

Run `exchange-online-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, and `--deliver` paths:

- `--json`  -  one JSON document on stdout
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  exchange-online-cli accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent --select DomainName,DomainType,Identity
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Local store** - `quarantine queue` reads snapshots saved by `quarantine snapshot`, and `forwarding-audit` reuses the last `inbox-rules sweep`
- **Non-interactive**  -  never prompts, every input is a flag
- **Writes** - CLI writes run immediately: no Exchange command asks for confirmation, so preview a write with `--dry-run` (or `offboard --plan`) before running it. Over MCP, `exchange-online_execute` and `offboard` return a preview and send nothing until `confirm` is boolean true.
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

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

- Use `--home <dir>` for one invocation, or set `EXCHANGE_ONLINE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `EXCHANGE_ONLINE_CONFIG_DIR`, `EXCHANGE_ONLINE_DATA_DIR`, `EXCHANGE_ONLINE_STATE_DIR`, `EXCHANGE_ONLINE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `EXCHANGE_ONLINE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `exchange-online-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "exchange-online": {
        "command": "exchange-online-mcp",
        "env": {
          "EXCHANGE_ONLINE_HOME": "/srv/exchange-online"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `EXCHANGE_ONLINE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `EXCHANGE_ONLINE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Cmdlet reads are not saved: every read is a cmdlet POST to the API, whatever `--data-source` says, so `--data-source local` still calls the API. The on-disk response cache is off. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `EXCHANGE_ONLINE_NO_LOCAL_DATA=1` disables the response cache; `inbox-rules sweep`, `forwarding-audit --sweep` and `quarantine snapshot` still save to `exchange-snapshots.db`. `EXCHANGE_ONLINE_DATA_SOURCE` sets the `--data-source` default.

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
exchange-online-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "exchange-online-cli learnings confirm 12"] }
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
       materially more, record the divergence via `exchange-online-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `exchange-online-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "shared" → the SharedMailbox type and a mailbox named Shared). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
exchange-online-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "shared mailbox") satisfies future queries under another alias (e.g., "SharedMailbox") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
exchange-online-cli teach \
  --query "$QUERY" \
  --resource-type <type> \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
exchange-online-cli teach-playbook \
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
exchange-online-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between API versions, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-mailbox / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`exchange-online-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `EXCHANGE_ONLINE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
exchange-online-cli feedback "the --since flag is inclusive but docs say exclusive"
exchange-online-cli feedback --stdin < notes.txt
exchange-online-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `EXCHANGE_ONLINE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `EXCHANGE_ONLINE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
exchange-online-cli profile save briefing --json
exchange-online-cli --profile briefing accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458
exchange-online-cli profile list --json
exchange-online-cli profile show briefing
exchange-online-cli profile delete briefing --yes
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
| 74 | Output could not be delivered: part of stdout did not reach its reader, because a unit was withheld and its notice dropped too, or the reader closed the stream |
| 78 | Refused to start: a value it must hold as a credential occurs in public text, as when a malformed config file cannot be read safely and every word in it is held |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `exchange-online-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add exchange-online-mcp -- exchange-online-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `exchange-online-mcp --transport http` requires `EXCHANGE_ONLINE_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `EXCHANGE_ONLINE_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `EXCHANGE_ONLINE_MCP_READ_ONLY=1` removes `exchange-online_execute` and every tool that changes Exchange or writes the local store, `teach` and `inbox_rules_sweep` among them, leaving 18 tools.

## Direct Use

1. Check if installed: `which exchange-online-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   exchange-online-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `exchange-online-cli <command> --help`.

## Known Limitations

- Not yet exercised against a live tenant: commands were built from Microsoft Learn, CIPP-API and MSPbots sources and tested against a local fake of the API.
- The `org-config`, `accepted-domains`, `send-on-behalf`, `folder-permissions`, `group-members` and `dynamic-group-members` commands use Microsoft's Exchange Online Admin API v2.0, a preview that is not enabled in every tenant. The other commands use the InvokeCommand route of the ExchangeOnlineManagement module, which Microsoft does not document.
- A token covers one tenant and lasts about an hour; mint a new one per tenant (including for the MCP server).
- A very large JSON listing can come back as a withheld notice with exit 0, or cut short with exit 74 (seen around 500 rows of 1.6 KB each). The credential check ran out of budget on that much text and withholds what it cannot finish checking, so the notice does not mean a credential was found. Narrow the listing with `--agent` or `--select`, or ask for fewer rows with `--result-size`.
- Message trace reaches back 90 days; `inbox-rules sweep` and `access-map` stop at 2000 mailboxes unless `--max-mailboxes` is raised.
- `quarantine queue` reads only snapshots saved with `quarantine snapshot`; `forwarding-audit` uses the last stored `inbox-rules sweep` unless you pass `--sweep`; over MCP, `forwarding_audit` takes no sweep argument, so run `inbox_rules_sweep` first.
- `quarantine snapshot` keys each tenant by the tenant ID or domain you pass (case-insensitive), so use the same form for a tenant every time; a tenant saved under both shows its requests twice in `quarantine queue`.
- Requests go only to the configured API host (`EXCHANGE_ONLINE_BASE_URL`, default `https://outlook.office365.com`): a redirect or `@odata.nextLink` to another host is refused. Any output, file or MCP result that would repeat the token is replaced by a notice ("[credkit: withheld from <sink>: it carried a credential this program holds]"), and the on-disk response cache is off; reads, sweeps and snapshots go to the local store with secret fields stripped.
