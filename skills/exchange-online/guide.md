# Exchange Online Admin CLI

**Exchange Online admin for MSPs over REST: forwarding audits, EWS posture, offboarding and quarantine across client tenants, no PowerShell session.**

Runs the Exchange cmdlets MSPs use every week through Microsoft's REST admin endpoints, one customer tenant per call, with JSON output and an MCP server. On top of the raw cmdlets it adds forwarding-audit, inbox-rules sweep and bec triage for compromise work, ews-posture for the EWS shutdown, offboard with a per-step receipt, and a cross-tenant quarantine queue.

Learn more at [Exchange Online Admin](https://learn.microsoft.com/en-us/exchange/reference/admin-api-overview).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `exchange-online-cli` and `exchange-online-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=exchange-online). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/exchange-online --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/exchange-online --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `exchange-online-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the exchange-online skill from https://github.com/Servosity/msp-skills/tree/main/skills/exchange-online. The skill defines how its required CLI (`exchange-online-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=exchange-online).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `EXCHANGE_ONLINE_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "exchange-online": {
      "command": "exchange-online-mcp",
      "env": {
        "EXCHANGE_ONLINE_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`exchange-online-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `EXCHANGE_ONLINE_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `EXCHANGE_ONLINE_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export EXCHANGE_ONLINE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
exchange-online-mcp --transport http
```

Set `EXCHANGE_ONLINE_MCP_READ_ONLY=1` (exactly `1`) to remove `exchange-online_execute` and every tool that changes Exchange or writes the local store: `offboard`, `teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `learnings_confirm`, `learnings_forget`, `learnings_reject`, `playbook_amend`, `inbox_rules_sweep` and `quarantine_snapshot`. `tools/list` then gives 18 tools. The local learn journal is still written in read-only mode.

## Authentication

Calls need an OAuth access token for the resource https://outlook.office365.com, minted for the customer tenant you are working in (GDAP partners mint one per tenant; tokens last about an hour). Export it as EXCHANGE_ONLINE_TOKEN, for example export EXCHANGE_ONLINE_TOKEN=$(az account get-access-token --resource https://outlook.office365.com --tenant <tenant-id> --query accessToken -o tsv). App-only tokens need Exchange.ManageAsApp (cmdlet commands) or Exchange.ManageAsAppV2 (the Admin API v2.0 commands) plus an Exchange role such as Exchange Administrator. Tenant-scoped commands take the tenant ID as their first argument; quarantine queue reads local snapshots and takes --tenant instead.

## Quick Start

```bash
# Check the install without touching a tenant
exchange-online-cli doctor --dry-run

# Prove the token reaches the tenant and see the org EWS switch
exchange-online-cli org-settings 0550b473-9fd2-4dbb-a058-a1640b4bf458 --properties EwsEnabled,Name

# Inventory mailboxes with only the columns you need
exchange-online-cli mailboxes list 0550b473-9fd2-4dbb-a058-a1640b4bf458 --properties UserPrincipalName,RecipientTypeDetails,ForwardingSmtpAddress --agent --select UserPrincipalName,RecipientTypeDetails,ForwardingSmtpAddress

# Find every path mail can leave the tenant
exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent

```

## Unique Features

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

## Usage

Run `exchange-online-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `EXCHANGE_ONLINE_CONFIG_DIR`, `EXCHANGE_ONLINE_DATA_DIR`, `EXCHANGE_ONLINE_STATE_DIR`, or `EXCHANGE_ONLINE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `EXCHANGE_ONLINE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export EXCHANGE_ONLINE_HOME=/srv/exchange-online
exchange-online-cli doctor
```

On Windows (PowerShell):

```powershell
$env:EXCHANGE_ONLINE_HOME = "C:\ProgramData\exchange-online"
exchange-online-cli doctor
```

Under `EXCHANGE_ONLINE_HOME=/srv/exchange-online`, the four dirs resolve to `/srv/exchange-online/config`, `/srv/exchange-online/data`, `/srv/exchange-online/state`, and `/srv/exchange-online/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:EXCHANGE_ONLINE_HOME = "C:\srv\exchange-online"
exchange-online-cli doctor
```

Under `$env:EXCHANGE_ONLINE_HOME = "C:\srv\exchange-online"`, the four dirs resolve to `C:\srv\exchange-online\config`, `C:\srv\exchange-online\data`, `C:\srv\exchange-online\state`, and `C:\srv\exchange-online\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `EXCHANGE_ONLINE_DATA_DIR` overrides an explicit `--home` for that kind. Use `EXCHANGE_ONLINE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `EXCHANGE_ONLINE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `exchange-online-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Cmdlet reads are not saved: every read is a cmdlet POST to the API, whatever `--data-source` says, so `--data-source local` still calls the API. `inbox-rules sweep`, `forwarding-audit --sweep` and `quarantine snapshot` save their results to `exchange-snapshots.db` in the data directory. The on-disk response cache is off (it is off on every client). Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `exchange-online-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `exchange-online-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept. Inbox-rule sweeps and quarantine snapshots live in `exchange-snapshots.db` in the data directory: purge leaves that file, so delete it by hand.
- **Run without a local copy:** set `EXCHANGE_ONLINE_NO_LOCAL_DATA=1`. Reads go to the API (they always do) and the response cache is off (`--no-cache`). `inbox-rules sweep`, `forwarding-audit --sweep` and `quarantine snapshot` still save to `exchange-snapshots.db`. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `EXCHANGE_ONLINE_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### accepted_domains

List accepted domains and their routing type (Admin API v2.0).

- **`exchange-online-cli accepted-domains <tenantId>`** - List accepted domains and their routing type (Admin API v2.0). Runs the Get-AcceptedDomain cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/AcceptedDomain with body {"CmdletInput":{"CmdletName":"Get-AcceptedDomain","Parameters":<flags>}}. Returns DomainName, DomainType (Authoritative, InternalRelay, ExternalRelay), Name, Id, Identity, Guid. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-accepted-domain

### anti_phish_policies

List anti-phishing policies.

- **`exchange-online-cli anti-phish-policies <tenantId>`** - List anti-phishing policies. Runs the Get-AntiPhishPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-AntiPhishPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoAntiPhishPolicies.ps1#L23 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-antiphishpolicy

### audit_config

Audit configuration, including UnifiedAuditLogIngestionEnabled.

- **`exchange-online-cli audit-config <tenantId>`** - Audit configuration, including UnifiedAuditLogIngestionEnabled. Runs the Get-AdminAuditLogConfig cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-AdminAuditLogConfig","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoAdminAuditLogConfig.ps1#L22 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPStandards/Public/Standards/Invoke-CIPPStandardAuditLog.ps1#L53 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-adminauditlogconfig

### auto_reply

A mailbox's automatic reply (out of office) state, audience, messages and window.

- **`exchange-online-cli auto-reply get`** - A mailbox's automatic reply (out of office) state, audience, messages and window. Runs the Get-MailboxAutoReplyConfiguration cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MailboxAutoReplyConfiguration","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Get-CIPPOutOfOffice.ps1#L11 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mailboxautoreplyconfiguration
- **`exchange-online-cli auto-reply set`** - Turn automatic replies on, off or scheduled, and set the messages. Runs the Set-MailboxAutoReplyConfiguration cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Set-MailboxAutoReplyConfiguration","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Set-CIPPOutOfoffice.ps1#L70 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/set-mailboxautoreplyconfiguration

### blocked_senders

List users blocked from sending mail (restricted after outbound spam).

- **`exchange-online-cli blocked-senders list`** - List users blocked from sending mail (restricted after outbound spam). Runs the Get-BlockedSenderAddress cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-BlockedSenderAddress","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Administration/Invoke-ListRestrictedUsers.ps1#L16 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-blockedsenderaddress
- **`exchange-online-cli blocked-senders remove`** - Unblock a restricted sender. Runs the Remove-BlockedSenderAddress cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Remove-BlockedSenderAddress","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Administration/Invoke-ExecRemoveRestrictedUser.ps1#L26 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/remove-blockedsenderaddress

### client_access

Per-mailbox client protocols: EWS, OWA, MAPI, IMAP, POP, ActiveSync, SMTP AUTH.

- **`exchange-online-cli client-access list`** - Per-mailbox client protocols: EWS, OWA, MAPI, IMAP, POP, ActiveSync, SMTP AUTH. Runs the Get-CASMailbox cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-CASMailbox","Parameters":<flags>}}. SmtpClientAuthenticationDisabled null means the mailbox inherits the org default (see transport-config). Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheCASMailboxes.ps1#L24 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-casmailbox
- **`exchange-online-cli client-access set`** - Turn client protocols on or off for one mailbox. Runs the Set-CASMailbox cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Set-CASMailbox","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/BEC/Invoke-CIPPBecContainment.ps1#L248 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPBaselines/Public/Executors/Invoke-CIPPBaselineDisableBasicAuthSMTP.ps1#L32 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/set-casmailbox

### distribution_groups

List distribution and mail-enabled security groups.

- **`exchange-online-cli distribution-groups add-member`** - Add a member to a distribution or mail-enabled security group. Runs the Add-DistributionGroupMember cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Add-DistributionGroupMember","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Add-CIPPGroupMember.ps1#L64 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/add-distributiongroupmember
- **`exchange-online-cli distribution-groups list`** - List distribution and mail-enabled security groups. Runs the Get-DistributionGroup cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-DistributionGroup","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Get-CIPPGroupType.ps1#L71 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-distributiongroup
- **`exchange-online-cli distribution-groups remove-member`** - Remove a member from a distribution or mail-enabled security group. Runs the Remove-DistributionGroupMember cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Remove-DistributionGroupMember","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Remove-CIPPGroupMember.ps1#L72 ; https://github.com/MSPbotsAI/exchange_online_mcp/blob/ee8a6d911665c2161a4d0c4292abc324a636eaae/README.md ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/remove-distributiongroupmember

### dkim

DKIM signing configuration per domain (enabled state, selector CNAMEs).

- **`exchange-online-cli dkim <tenantId>`** - DKIM signing configuration per domain (enabled state, selector CNAMEs). Runs the Get-DkimSigningConfig cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-DkimSigningConfig","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoDkimSigningConfig.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-dkimsigningconfig

### domains

List accepted domains through the cmdlet route (works where the v2.0 preview is not enabled).

- **`exchange-online-cli domains <tenantId>`** - List accepted domains through the cmdlet route (works where the v2.0 preview is not enabled). Runs the Get-AcceptedDomain cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-AcceptedDomain","Parameters":<flags>}}. Same rows as accepted-domains list (DomainName, DomainType, Default, ...). Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoAcceptedDomains.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-accepteddomain

### dynamic_group_members

List the computed members of a dynamic distribution group (Admin API v2.0).

- **`exchange-online-cli dynamic-group-members <tenantId>`** - List the computed members of a dynamic distribution group (Admin API v2.0). Runs the Get-DynamicDistributionGroupMember cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/DynamicDistributionGroupMember with body {"CmdletInput":{"CmdletName":"Get-DynamicDistributionGroupMember","Parameters":<flags>}}. Same member shape as group-members list. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-dynamic-distribution-group-member

### folder_permissions

List permissions on a mailbox folder (Admin API v2.0).

- **`exchange-online-cli folder-permissions add`** - Grant a user permissions on a mailbox folder (Admin API v2.0). Runs the Add-MailboxFolderPermission cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/MailboxFolderPermission with body {"CmdletInput":{"CmdletName":"Add-MailboxFolderPermission","Parameters":<flags>}}. Returns 200 on success, usually with no body. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-mailbox-folder-permission
- **`exchange-online-cli folder-permissions list`** - List permissions on a mailbox folder (Admin API v2.0). Runs the Get-MailboxFolderPermission cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/MailboxFolderPermission with body {"CmdletInput":{"CmdletName":"Get-MailboxFolderPermission","Parameters":<flags>}}. Returns Identity, FolderName, User, AccessRights, SharingPermissionFlags, IsValid, ObjectState. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-mailbox-folder-permission
- **`exchange-online-cli folder-permissions remove`** - Remove a user's permissions from a mailbox folder (Admin API v2.0). Runs the Remove-MailboxFolderPermission cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/MailboxFolderPermission with body {"CmdletInput":{"CmdletName":"Remove-MailboxFolderPermission","Parameters":<flags>}}. Returns 200 on success, usually with no body. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-mailbox-folder-permission
- **`exchange-online-cli folder-permissions set`** - Change a user's permissions on a mailbox folder (Admin API v2.0). Runs the Set-MailboxFolderPermission cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/MailboxFolderPermission with body {"CmdletInput":{"CmdletName":"Set-MailboxFolderPermission","Parameters":<flags>}}. Returns 200 on success, usually with no body. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-mailbox-folder-permission

### group_members

List the members of a distribution group (Admin API v2.0).

- **`exchange-online-cli group-members <tenantId>`** - List the members of a distribution group (Admin API v2.0). Runs the Get-DistributionGroupMember cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/DistributionGroupMember with body {"CmdletInput":{"CmdletName":"Get-DistributionGroupMember","Parameters":<flags>}}. Includes hidden recipients. Returns Identity, Alias, EmailAddresses, PrimarySmtpAddress, DisplayName, FirstName, LastName, Name, RecipientType, RecipientTypeDetails, HiddenFromAddressListsEnabled. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-distribution-group-member

### inbound_connectors

List inbound mail connectors.

- **`exchange-online-cli inbound-connectors <tenantId>`** - List inbound mail connectors. Runs the Get-InboundConnector cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-InboundConnector","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Transport/Invoke-ListExchangeConnectors.ps1#L16 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-inboundconnector

### inbox_rules

List a mailbox's inbox rules (forward, redirect, delete and move actions included).

- **`exchange-online-cli inbox-rules list`** - List a mailbox's inbox rules (forward, redirect, delete and move actions included). Runs the Get-InboxRule cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-InboxRule","Parameters":<flags>}}. The request is routed (X-AnchorMailbox) to the mailbox named by Mailbox. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPActivityTriggers/Public/Entrypoints/Activity Triggers/Push-GetMailboxRulesBatch.ps1#L26 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-inboxrule
- **`exchange-online-cli inbox-rules remove`** - Delete one inbox rule. Runs the Remove-InboxRule cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Remove-InboxRule","Parameters":<flags>}}. CIPP routes this call to the rule owner's mailbox; pass --mailbox so the CLI does the same. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Remove-CIPPMailboxRule.ps1#L27 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/remove-inboxrule

### mail_contacts

List mail contacts.

- **`exchange-online-cli mail-contacts <tenantId>`** - List mail contacts. Runs the Get-MailContact cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MailContact","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoMailContacts.ps1#L34 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mailcontact

### mailbox_folders

List a mailbox's folders with item counts, sizes and folder types (find a localized Calendar folder).

- **`exchange-online-cli mailbox-folders <tenantId>`** - List a mailbox's folders with item counts, sizes and folder types (find a localized Calendar folder). Runs the Get-MailboxFolderStatistics cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MailboxFolderStatistics","Parameters":<flags>}}. Each row carries Name, FolderPath and FolderType; the Calendar folder's Name is localized. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPActivityTriggers/Public/Entrypoints/Activity Triggers/Mailbox Permissions/Push-GetCalendarPermissionsBatch.ps1#L66 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Resources/Invoke-EditRoomMailbox.ps1#L104 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mailboxfolderstatistics

### mailbox_permissions

List mailbox-level permissions (FullAccess and others) on a mailbox.

- **`exchange-online-cli mailbox-permissions add`** - Grant a user FullAccess (or other rights) to a mailbox. Runs the Add-MailboxPermission cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Add-MailboxPermission","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Set-CIPPMailboxPermission.ps1#L38 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/add-mailboxpermission
- **`exchange-online-cli mailbox-permissions list`** - List mailbox-level permissions (FullAccess and others) on a mailbox. Runs the Get-MailboxPermission cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MailboxPermission","Parameters":<flags>}}. Includes inherited and self (NT AUTHORITY\SELF) entries; filter on IsInherited and User. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CippExtensions/Public/Extension Functions/Sync-CippExtensionData.ps1#L202 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mailboxpermission
- **`exchange-online-cli mailbox-permissions remove`** - Remove a user's FullAccess (or other rights) from a mailbox. Runs the Remove-MailboxPermission cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Remove-MailboxPermission","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Set-CIPPMailboxPermission.ps1#L49 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/remove-mailboxpermission

### mailbox_stats

Mailbox size and item counts (primary or archive).

- **`exchange-online-cli mailbox-stats <tenantId>`** - Mailbox size and item counts (primary or archive). Runs the Get-MailboxStatistics cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MailboxStatistics","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPAlerts/Public/Alerts/Get-CIPPAlertArchiveQuota.ps1#L65 ; https://github.com/MSPbotsAI/exchange_online_mcp/blob/ee8a6d911665c2161a4d0c4292abc324a636eaae/README.md ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mailboxstatistics

### mailboxes

List mailboxes with type, forwarding, hold and archive state.

- **`exchange-online-cli mailboxes list`** - List mailboxes with type, forwarding, hold and archive state. Runs the Get-Mailbox cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-Mailbox","Parameters":<flags>}}. Use --properties to cut the ~40 KB per-mailbox payload, e.g. --properties UserPrincipalName,RecipientTypeDetails,ForwardingSmtpAddress,ForwardingAddress,DeliverToMailboxAndForward. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/New-CIPPUserTask.ps1#L126 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPStandards/Public/Standards/Invoke-CIPPStandardDisableSharedMailbox.ps1#L60 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPAlerts/Public/Alerts/Get-CIPPAlertArchiveQuota.ps1#L46 ; https://github.com/MSPbotsAI/exchange_online_mcp/blob/ee8a6d911665c2161a4d0c4292abc324a636eaae/README.md ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mailbox
- **`exchange-online-cli mailboxes set`** - Change a mailbox: convert type (Shared, Regular, Room, Equipment), set or clear forwarding, hide from the address list. Runs the Set-Mailbox cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Set-Mailbox","Parameters":<flags>}}. To clear forwarding, send nulls through --stdin as CIPP does: {"Identity":"alex@contoso.com","ForwardingSmtpAddress":null,"ForwardingAddress":null,"DeliverToMailboxAndForward":false}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Set-CIPPMailboxType.ps1#L15 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Set-CIPPForwarding.ps1#L62 ; https://github.com/MSPbotsAI/exchange_online_mcp/blob/ee8a6d911665c2161a4d0c4292abc324a636eaae/README.md ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/set-mailbox

### malware_policies

List anti-malware policies.

- **`exchange-online-cli malware-policies <tenantId>`** - List anti-malware policies. Runs the Get-MalwareFilterPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MalwareFilterPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoMalwareFilterPolicies.ps1#L23 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-malwarefilterpolicy

### message_trace

Trace messages by sender, recipient, message ID and date window.

- **`exchange-online-cli message-trace detail`** - Delivery events for one traced message and recipient. Runs the Get-MessageTraceDetailV2 cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MessageTraceDetailV2","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Tools/Invoke-ListMessageTrace.ps1#L81 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-messagetracedetailv2
- **`exchange-online-cli message-trace list`** - Trace messages by sender, recipient, message ID and date window. Runs the Get-MessageTraceV2 cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MessageTraceV2","Parameters":<flags>}}. No OData paging: rows come newest first, at most ResultSize per query and 10 days per query; for the next rows pass the last row's Received time as EndDate and its RecipientAddress as StartingRecipientAddress. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/BEC/Get-CIPPBecMessageTrace.ps1#L44 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/BEC/Resolve-CIPPBecMessageSubjects.ps1#L55 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-messagetracev2

### mobile_devices

List a mailbox's ActiveSync / mobile device partnerships.

- **`exchange-online-cli mobile-devices list`** - List a mailbox's ActiveSync / mobile device partnerships. Runs the Get-MobileDevice cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-MobileDevice","Parameters":<flags>}}. The request is routed (X-AnchorMailbox) to the mailbox named by Mailbox. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Remove-CIPPMobileDevice.ps1#L14 ; https://github.com/MSPbotsAI/exchange_online_mcp/blob/ee8a6d911665c2161a4d0c4292abc324a636eaae/README.md ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-mobiledevice
- **`exchange-online-cli mobile-devices remove`** - Remove a mobile device partnership (does not wipe the handset). Runs the Remove-MobileDevice cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Remove-MobileDevice","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Remove-CIPPMobileDevice.ps1#L21 ; https://github.com/MSPbotsAI/exchange_online_mcp/blob/ee8a6d911665c2161a4d0c4292abc324a636eaae/README.md ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/remove-mobiledevice
- **`exchange-online-cli mobile-devices wipe`** - Wipe a mobile device (or only the mailbox account on it) the next time it connects. Runs the Clear-MobileDevice cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Clear-MobileDevice","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Clear-CIPPMobileDevice.ps1#L23 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/clear-mobiledevice

### org_config

Organization MailTips settings and limits (Admin API v2.0).

- **`exchange-online-cli org-config <tenantId>`** - Organization MailTips settings and limits (Admin API v2.0). Runs the Get-OrganizationConfig cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/OrganizationConfig with body {"CmdletInput":{"CmdletName":"Get-OrganizationConfig","Parameters":<flags>}}. Takes no parameters. Returns one organization object (MailTipsAllTipsEnabled, MailTipsExternalRecipientsTipsEnabled, MailTipsLargeAudienceThreshold; the full Get-OrganizationConfig output during Preview). Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-organization-config

### org_settings

Full organization configuration, including EwsEnabled, EwsAllowList and EwsApplicationAccessPolicy.

- **`exchange-online-cli org-settings <tenantId>`** - Full organization configuration, including EwsEnabled, EwsAllowList and EwsApplicationAccessPolicy. Runs the Get-OrganizationConfig cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-OrganizationConfig","Parameters":<flags>}}. The whole Get-OrganizationConfig object; org-config is the documented v2.0 subset. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPStandards/Public/Standards/Invoke-CIPPStandardDisableEWS.ps1#L47 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-organizationconfig

### outbound_connectors

List outbound mail connectors.

- **`exchange-online-cli outbound-connectors <tenantId>`** - List outbound mail connectors. Runs the Get-OutboundConnector cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-OutboundConnector","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Transport/Invoke-ListExchangeConnectors.ps1#L15 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-outboundconnector

### outbound_spam_policies

List outbound spam policies, including AutoForwardingMode and sending limits.

- **`exchange-online-cli outbound-spam-policies <tenantId>`** - List outbound spam policies, including AutoForwardingMode and sending limits. Runs the Get-HostedOutboundSpamFilterPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-HostedOutboundSpamFilterPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoHostedOutboundSpamFilterPolicy.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-hostedoutboundspamfilterpolicy

### quarantine

List quarantined messages.

- **`exchange-online-cli quarantine delete`** - Delete quarantined messages. Runs the Delete-QuarantineMessage cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Delete-QuarantineMessage","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Spamfilter/Invoke-ExecQuarantineManagement.ps1#L38 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/delete-quarantinemessage
- **`exchange-online-cli quarantine list`** - List quarantined messages. Runs the Get-QuarantineMessage cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-QuarantineMessage","Parameters":<flags>}}. Pages with Page/PageSize, not ResultSize. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPAlerts/Public/Alerts/Get-CIPPAlertQuarantineReleaseRequests.ps1#L30 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPActivityTriggers/Public/Entrypoints/Activity Triggers/Push-ListMailQuarantineAllTenants.ps1#L25 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-quarantinemessage
- **`exchange-online-cli quarantine release`** - Release, approve or deny quarantined messages. Runs the Release-QuarantineMessage cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Release-QuarantineMessage","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPHTTP/Public/Entrypoints/HTTP Functions/Email-Exchange/Spamfilter/Invoke-ExecQuarantineManagement.ps1#L40 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/release-quarantinemessage

### quarantine_policies

List quarantine policies (what users may do with quarantined mail).

- **`exchange-online-cli quarantine-policies <tenantId>`** - List quarantine policies (what users may do with quarantined mail). Runs the Get-QuarantinePolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-QuarantinePolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoQuarantinePolicy.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-quarantinepolicy

### recipients

Look up any mail recipient (mailbox, group, contact, mail user).

- **`exchange-online-cli recipients <tenantId>`** - Look up any mail recipient (mailbox, group, contact, mail user). Runs the Get-Recipient cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-Recipient","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Remove-CIPPFolderPermission.ps1#L50 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-recipient

### remote_domains

List remote domains, including AutoForwardEnabled and TNEF settings.

- **`exchange-online-cli remote-domains <tenantId>`** - List remote domains, including AutoForwardEnabled and TNEF settings. Runs the Get-RemoteDomain cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-RemoteDomain","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoRemoteDomain.ps1#L22 ; https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPStandards/Public/Standards/Invoke-CIPPStandardDisableTNEF.ps1#L47 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-remotedomain

### safe_attachment_policies

List Defender for Office 365 Safe Attachments policies.

- **`exchange-online-cli safe-attachment-policies <tenantId>`** - List Defender for Office 365 Safe Attachments policies. Runs the Get-SafeAttachmentPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-SafeAttachmentPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoSafeAttachmentPolicies.ps1#L26 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-safeattachmentpolicy

### safe_links_policies

List Defender for Office 365 Safe Links policies.

- **`exchange-online-cli safe-links-policies <tenantId>`** - List Defender for Office 365 Safe Links policies. Runs the Get-SafeLinksPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-SafeLinksPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoSafeLinksPolicies.ps1#L42 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-safelinkspolicy

### send_as

List Send As permissions on a recipient, or every recipient a trustee can send as.

- **`exchange-online-cli send-as add`** - Grant a trustee Send As on a recipient. Runs the Add-RecipientPermission cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Add-RecipientPermission","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Set-CIPPMailboxPermission.ps1#L61 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/add-recipientpermission
- **`exchange-online-cli send-as list`** - List Send As permissions on a recipient, or every recipient a trustee can send as. Runs the Get-RecipientPermission cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-RecipientPermission","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPActivityTriggers/Public/Entrypoints/Activity Triggers/Mailbox Permissions/Push-GetMailboxPermissionsBatch.ps1#L35 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-recipientpermission
- **`exchange-online-cli send-as remove`** - Remove a trustee's Send As on a recipient. Runs the Remove-RecipientPermission cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Remove-RecipientPermission","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPCore/Public/Remove-CIPPMailboxPermissions.ps1#L76 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/remove-recipientpermission

### send_on_behalf

List mailboxes with their Send on behalf delegates (Admin API v2.0).

- **`exchange-online-cli send-on-behalf list`** - List mailboxes with their Send on behalf delegates (Admin API v2.0). Runs the Get-Mailbox cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/Mailbox with body {"CmdletInput":{"CmdletName":"Get-Mailbox","Parameters":<flags>}}. Core properties: Identity, Id, Name, DisplayName, UserPrincipalName, Alias, ExternalDirectoryObjectId, RecipientType, RecipientTypeDetails, EmailAddresses, PrimarySmtpAddress, MaxSendSize, GrantSendOnBehalfTo. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-mailbox
- **`exchange-online-cli send-on-behalf set`** - Replace a mailbox's Send on behalf delegate list (Admin API v2.0). Runs the Set-Mailbox cmdlet through the Exchange Online Admin API v2.0 (Preview): POST /adminapi/v2.0/{tenantId}/Mailbox with body {"CmdletInput":{"CmdletName":"Set-Mailbox","Parameters":<flags>}}. Replaces the delegate list, or changes it incrementally with the add/remove object form (Learn Examples 3 and 4). Returns 200 with no body. Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-endpoints-mailbox

### sharing_policies

List calendar and contact sharing policies.

- **`exchange-online-cli sharing-policies <tenantId>`** - List calendar and contact sharing policies. Runs the Get-SharingPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-SharingPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoSharingPolicy.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-sharingpolicy

### spam_policies

List inbound anti-spam (hosted content filter) policies.

- **`exchange-online-cli spam-policies <tenantId>`** - List inbound anti-spam (hosted content filter) policies. Runs the Get-HostedContentFilterPolicy cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-HostedContentFilterPolicy","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoHostedContentFilterPolicy.ps1#L21 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-hostedcontentfilterpolicy

### transport_config

Organization transport settings, including the SMTP AUTH default (SmtpClientAuthenticationDisabled).

- **`exchange-online-cli transport-config <tenantId>`** - Organization transport settings, including the SMTP AUTH default (SmtpClientAuthenticationDisabled). Runs the Get-TransportConfig cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-TransportConfig","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoTransportConfig.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-transportconfig

### transport_rules

List mail flow (transport) rules.

- **`exchange-online-cli transport-rules disable`** - Disable a mail flow rule. Runs the Disable-TransportRule cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Disable-TransportRule","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPBaselines/Public/Executors/Invoke-CIPPBaselineTransportRuleTemplate.ps1#L62 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/disable-transportrule
- **`exchange-online-cli transport-rules enable`** - Enable a mail flow rule. Runs the Enable-TransportRule cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Enable-TransportRule","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPBaselines/Public/Executors/Invoke-CIPPBaselineTransportRuleTemplate.ps1#L62 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/enable-transportrule
- **`exchange-online-cli transport-rules list`** - List mail flow (transport) rules. Runs the Get-TransportRule cmdlet through the InvokeCommand route of the ExchangeOnlineManagement module (undocumented by Microsoft): POST /adminapi/beta/{tenantId}/InvokeCommand with body {"CmdletInput":{"CmdletName":"Get-TransportRule","Parameters":<flags>}}. Sources: https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Modules/CIPPDB/Public/DBCache/Set-CIPPDBCacheExoTransportRules.ps1#L22 ; https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/get-transportrule


### access-map


- **`exchange-online-cli access-map <tenantId> <user>`** - List every mailbox one user can open, send as, or send on behalf of.


### bec


- **`exchange-online-cli bec triage <tenantId> <mailbox>`** - Collect compromise evidence for one mailbox: inbox rules, forwarding, restricted-sender status, new mobile devices, outbound mail volume and legacy protocols.


### ews-posture


- **`exchange-online-cli ews-posture <tenantId>`** - See which mailboxes still have EWS, IMAP, POP, ActiveSync or SMTP AUTH effectively on once org defaults are applied.


### forwarding-audit


- **`exchange-online-cli forwarding-audit <tenantId>`** - List every way mail can auto-forward out of a tenant and whether the outbound spam policy or remote-domain settings block it; inbox rules come from the last 'inbox-rules sweep' unless you pass --sweep.


### inbox-rules


- **`exchange-online-cli inbox-rules sweep <tenantId>`** - Pull every mailbox's inbox rules in one tenant and flag the classic attacker rules.


### offboard


- **`exchange-online-cli offboard <tenantId> <mailbox>`** - Run the Exchange side of a leaver in one pass: shared mailbox, GAL hide, mobile device removal, plus auto-reply (--auto-reply), manager FullAccess and Send As (--manager) and forwarding (--forward-to), with a per-step receipt; --plan only reports.


### quarantine


- **`exchange-online-cli quarantine snapshot <tenantId>`** - Save one tenant's pending quarantine release requests locally so the cross-tenant queue can show them.

- **`exchange-online-cli quarantine queue`** - One oldest-first queue of quarantine release requests across every tenant saved with 'quarantine snapshot' (run it per tenant first).


### where-is


- **`exchange-online-cli where-is <tenantId> --recipient <address>`** - Explain why a message did or did not reach a recipient, with the next action to take.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`exchange-online-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`exchange-online-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`exchange-online-cli learnings list`** - Inspect taught rows
- **`exchange-online-cli learnings forget <query>`** - Undo a teach
- **`exchange-online-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`exchange-online-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`exchange-online-cli teach-pattern`** - Install a query/resource template up front
- **`exchange-online-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `EXCHANGE_ONLINE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `exchange-online-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
exchange-online-cli accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458

# JSON for scripting and agents
exchange-online-cli accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458 --json
# Filter to specific fields
exchange-online-cli accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458 --json --select DomainName,DomainType,Identity

# Dry run  -  show the request without sending
exchange-online-cli accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
exchange-online-cli accepted-domains 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Writes** - CLI writes run immediately: no Exchange command asks for confirmation, so preview a write with `--dry-run` (or `offboard --plan`) before running it. Over MCP, `exchange-online_execute` and `offboard` return a preview and send nothing until `confirm` is boolean true.
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Local store** - `quarantine queue` reads snapshots saved by `quarantine snapshot`, and `forwarding-audit` reuses the last `inbox-rules sweep`
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
exchange-online-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `exchange-online-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/exchange-online-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\exchange-online-cli\config.toml` on Windows; `--home`, `EXCHANGE_ONLINE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `EXCHANGE_ONLINE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `EXCHANGE_ONLINE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `EXCHANGE_ONLINE_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `exchange-online-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Known Limitations

- Not yet exercised against a live tenant: commands were built from Microsoft Learn, CIPP-API and MSPbots sources and tested against a local fake of the API.
- The `org-config`, `accepted-domains`, `send-on-behalf`, `folder-permissions`, `group-members` and `dynamic-group-members` commands use Microsoft's Exchange Online Admin API v2.0, a preview that is not enabled in every tenant. The other commands use the InvokeCommand route of the ExchangeOnlineManagement module, which Microsoft does not document.
- A token covers one tenant and lasts about an hour; mint a new one per tenant (including for the MCP server).
- A very large JSON listing can come back as a withheld notice with exit 0, or cut short with exit 74 (seen around 500 rows of 1.6 KB each). The credential check ran out of budget on that much text and withholds what it cannot finish checking, so the notice does not mean a credential was found. Narrow the listing with `--agent` or `--select`, or ask for fewer rows with `--result-size`.
- Message trace reaches back 90 days; `inbox-rules sweep` and `access-map` stop at 2000 mailboxes unless `--max-mailboxes` is raised.
- `quarantine queue` reads only snapshots saved with `quarantine snapshot`; `forwarding-audit` uses the last stored `inbox-rules sweep` unless you pass `--sweep`; over MCP, `forwarding_audit` takes no sweep argument, so run `inbox_rules_sweep` first.
- `quarantine snapshot` keys each tenant by the tenant ID or domain you pass (case-insensitive), so use the same form for a tenant every time; a tenant saved under both shows its requests twice in `quarantine queue`.
- Requests go only to the configured API host (`EXCHANGE_ONLINE_BASE_URL`, default `https://outlook.office365.com`): a redirect or `@odata.nextLink` to another host is refused. Any output, file or MCP result that would repeat the token is replaced by a notice ("[credkit: withheld from <sink>: it carried a credential this program holds]"), and the on-disk response cache is off; reads, sweeps and snapshots go to the local store with secret fields stripped.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `exchange-online-cli doctor` to check credentials
- Verify the token is set without printing it: `test -n "$EXCHANGE_ONLINE_TOKEN" && echo set`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `exchange-online-cli mailboxes list <tenantId>` to see the tenant's mailboxes

### API-specific
- **401 Unauthorized**  -  Mint a fresh token for this tenant with resource https://outlook.office365.com; tokens last about an hour
- **403 Forbidden**  -  Give the app or user an Exchange role (Exchange Administrator) in that tenant
- **400 Bad Request on a v2.0 command**  -  The Admin API v2.0 is in preview and not enabled in every tenant; use the matching cmdlet command (mailboxes list instead of send-on-behalf list)
- **Responses are huge**  -  Pass --properties with the columns you need; Exchange returns every property by default

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

TLS certificates are verified by default. For a trusted development or self-signed endpoint only, pass `--insecure` for one invocation or set `skip_tls_verify = true` in the config file for a persistent override. `--insecure` overrides `--ca-cert` and `--tls-server-name`.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**CIPP-API**](https://github.com/KelvinTegelaar/CIPP-API)  -  PowerShell
- [**exchange_online_mcp**](https://github.com/MSPbotsAI/exchange_online_mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
