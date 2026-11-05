# Mimecast CLI

**Find a held or blocked Mimecast message, explain why in one verdict, and release it behind a safety check, with hold-queue triage, mail-flow health and phish click tracing built in.**

The Mimecast API 2.0 hold-queue, message tracking, TTP and DLP log, audit and Cloud Integrated endpoints, plus workflow commands that join them. 'why' joins tracking, hold queue, release log and rejection log into one answer; 'held triage' groups the queue by reason with a release-risk class; 'held release-safe' refuses risky releases and records the ticket in a local ledger; 'mailflow' gives a single health verdict.

Learn more at [Mimecast](https://developer.services.mimecast.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `mimecast-cli` and `mimecast-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.ps1 | iex
   ```
3. Verify: `mimecast-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=mimecast). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/mimecast --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/mimecast --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `mimecast-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the mimecast skill from https://github.com/Servosity/msp-skills/tree/main/skills/mimecast. The skill defines how its required CLI (`mimecast-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=mimecast).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MIMECAST_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "mimecast": {
      "command": "mimecast-mcp",
      "env": {
        "MIMECAST_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`mimecast-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `MIMECAST_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `MIMECAST_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export MIMECAST_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
mimecast-mcp --transport http
```

On Windows (PowerShell):

```powershell
$b = New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
$env:MIMECAST_MCP_HTTP_TOKEN = -join ($b | ForEach-Object { '{0:x2}' -f $_ })
mimecast-mcp --transport http
```

Set `MIMECAST_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Mimecast API 2.0 uses OAuth2 client credentials. In the Administration Console go to Administration > Services > API and Platform Integrations > Mimecast API 2.0 > Generate Keys, choose the products (Email Security Cloud Gateway, Audit Events, Security Events as needed) and a least-privilege role, then export MIMECAST_CLIENT_ID and MIMECAST_CLIENT_SECRET. The CLI exchanges them at https://api.services.mimecast.com/oauth/token for a 30-minute bearer token and re-mints automatically. API 1.0 keys and hosts (us-api.mimecast.com, eu-api.mimecast.com) do not work with API 2.0. For data residency set both MIMECAST_BASE_URL and MIMECAST_TOKEN_URL to a regional gateway, for example https://uk-api.services.mimecast.com and https://uk-api.services.mimecast.com/oauth/token (us-api and de-api work the same way). MSPs with delegated access can set MIMECAST_ACCOUNT_CODE (or --account-code on the workflow commands): it is sent as meta.accountCode on the classic /api/ envelope endpoints only (hold queue, tracking, logs, managed-sender permit/block); REST endpoints (gateway, journaling, ci-policies, remediation, senders bulk-delete) have no account selector, so while an account code is set the CLI refuses their writes and warns on their reads (they would act on the API application's own account). Delegation is unverified against a live delegate tenant.

## Quick Start

```bash
# Confirm the install (dry run; run plain doctor to check credentials and reachability)
mimecast-cli doctor --dry-run

# See the hold queue grouped by reason with a release-risk class
mimecast-cli held triage --since 24h --agent

# Answer 'why was my email blocked' in one verdict
mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d

# Preview a guarded release: look up and classify the ID without releasing
mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan

# One OK/WARN/FAIL verdict for queues and stuck deliveries
mimecast-cli mailflow

```

## Unique Features

### Explain and investigate
- **`why`**  -  Explain one message in a single verdict: its stage (held, rejected-at-gateway, released-from-hold, rejected-from-hold, the tracking status such as delivery or bounced, or not-found) and why, with policy, reason, spam score, SPF/DKIM/DMARC and who released or rejected it.

  _Reach for this first on any 'my email never arrived' or 'why was this blocked' request instead of chaining five raw endpoint calls._

  ```bash
  mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent
  ```
- **`ttp who-clicked`**  -  Given a raw or Mimecast-rewritten URL, list who clicked it, when, whether it was blocked, and whether they clicked through a warning (delivery-time entry scans are not counted as clicks).

  _Use this on every reported phish to find other users who clicked before deciding on remediation._

  ```bash
  mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent
  ```
- **`threats mailbox`**  -  Show every threat event that touched one mailbox over a window, newest first: URL Protect clicks, attachment sandbox verdicts, impersonation hits, gateway rejections and messages held for it, with a per-source fetch report so a failed source never reads as clean.

  _Use this when a user reports a suspicious email, to see every threat, rejection and hold for them before deciding on remediation._

  ```bash
  mimecast-cli threats mailbox alice@example.com --since 7d --agent
  ```
- **`threats sender`**  -  Given a sender address or @domain, list every recipient it reached or tried to reach across URL clicks, attachment verdicts, impersonation hits, rejections and holds, with per-recipient counts by kind, who clicked (clickers first) and the held message IDs.

  _Use this to scope a phishing campaign: which users received it, which clicked, and which are still waiting in the hold queue._

  ```bash
  mimecast-cli threats sender @phish.example --since 3d --agent
  ```

### Hold queue loop
- **`held triage`**  -  See the whole hold queue grouped by hold reason, policy or sender domain, with a release-risk class per group and ready-to-act ID batches.

  _Use this to work the admin hold queue in grouped decisions rather than reading a flat list item by item._

  ```bash
  mimecast-cli held triage --since 24h --group-by reason --agent
  ```
- **`held release-safe`**  -  Release held messages after a keyword-based hold-reason check (a heuristic, not verified against every Mimecast hold reason) that refuses impersonation, malware, sandbox and policy holds unless you give an override reason, and record the ticket in a local ledger.

  _Use this when acting on an end user's 'release my email' request; it adds a review step before risky releases, and --plan previews the decision without releasing._

  ```bash
  mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan
  ```
- **`held purge-sender`**  -  Plan, then with --yes reject, every held message from one sender address or domain, optionally blocking that sender for each recipient it targeted.

  _Use this when one sender floods the hold queue and burying the real decisions is the problem._

  ```bash
  mimecast-cli held purge-sender spam@bad.example --since 7d --block --plan
  ```
- **`held release-audit`**  -  Review who released or rejected held messages over a period, flagging risky releases and marking those with no ticket in the local release-safe ledger.

  _Use this for the weekly release review or after an incident to see which risky messages were let through and by whom._

  ```bash
  mimecast-cli held release-audit --since 7d --agent
  ```

### Mail-flow and sender hygiene
- **`senders suggest`**  -  Find senders that keep getting held then released (narrow permit candidates, excluding any with impersonation or malware holds or failed SPF/DKIM/DMARC) or keep getting rejected (block candidates), as a ready managed-sender payload.

  _Use this for weekly managed-sender hygiene instead of re-permitting the same vendors by hand._

  ```bash
  mimecast-cli senders suggest --since 30d --min-holds 3 --agent
  ```
- **`mailflow`**  -  One OK/WARN/FAIL verdict for tenant mail flow: queue backlog, no inbound mail within a window, and stuck deliveries grouped by destination, with a monitor-friendly exit code.

  _Use this when someone says 'nobody is getting mail' or as an RMM monitor that fails on a real backlog._

  ```bash
  mimecast-cli mailflow --max-queue 200 --stale-after 30m --agent
  ```

## Recipes

### Triage the hold queue

```bash
mimecast-cli held triage --since 24h --new --agent
```

Groups held messages not shown by a previous --new run (the first run shows the whole queue), with a release-risk class per group.

### Explain a missing email

```bash
mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent
```

Returns the stage (held, rejected, delivered, processing), policy, reason and SPF/DKIM/DMARC checks in one verdict.

### Narrow message detail to the spam verdict

```bash
mimecast-cli messages get --data '[{"id":"eNo1jr0OgjAYRd-lExample"}]' --agent --select data.spamInfo.spamScore,data.spamInfo.spamProcessingDetail.verdict
```

Message detail is deeply nested; --select keeps just the spam score and verdict.

### Find everyone who clicked a phish link

```bash
mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent
```

Decodes rewritten links and matches click logs client-side.

### Monitor mail flow from an RMM

```bash
mimecast-cli mailflow --max-queue 200 --stale-after 30m --agent
```

Exits 1 on FAIL; sections it could not measure appear in section_errors and raise WARN, so use --fail-on-warn to alert on those too.

## Usage

Run `mimecast-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `MIMECAST_CONFIG_DIR`, `MIMECAST_DATA_DIR`, `MIMECAST_STATE_DIR`, or `MIMECAST_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `MIMECAST_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export MIMECAST_HOME=/srv/mimecast
mimecast-cli doctor
```

On Windows (PowerShell):

```powershell
$env:MIMECAST_HOME = "C:\ProgramData\mimecast"
mimecast-cli doctor
```

Under `MIMECAST_HOME=/srv/mimecast`, the four dirs resolve to `/srv/mimecast/config`, `/srv/mimecast/data`, `/srv/mimecast/state`, and `/srv/mimecast/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:MIMECAST_HOME = "C:\srv\mimecast"
mimecast-cli doctor
```

Under `$env:MIMECAST_HOME = "C:\srv\mimecast"`, the four dirs resolve to `C:\srv\mimecast\config`, `C:\srv\mimecast\data`, `C:\srv\mimecast\state`, and `C:\srv\mimecast\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "mimecast": {
      "command": "mimecast-mcp",
      "env": {
        "MIMECAST_HOME": "/srv/mimecast"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `MIMECAST_DATA_DIR` overrides an explicit `--home` for that kind. Use `MIMECAST_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `MIMECAST_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `mimecast-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, `data-<hash>.db` per credential, and `data-<hash>-account-<code>.db` per delegated account code) in the data directory. This connector keeps no HTTP response cache: every read goes to the API or the store, so nothing is written under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `mimecast-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `mimecast-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `MIMECAST_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `MIMECAST_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### archive_logs

Archive search and view logs

- **`mimecast-cli archive-logs search-logs`** - Archive search log entries in a time range. Requires app role permission: Archive | Search Logs | Read.
- **`mimecast-cli archive-logs searches`** - Log of archive searches run by admins and users. Requires app role permission: Archive | Search Logs | Read.
- **`mimecast-cli archive-logs views`** - Log of archived messages viewed by admins. Requires app role permission: Archive | View Logs | Read.

### audit

Administration console audit events and categories

- **`mimecast-cli audit categories`** - List audit event categories (codes for audit events --data categories).
- **`mimecast-cli audit events`** - Admin console audit events by category and time range. Requires app role permission: Account | Logs | Read.

### ci_onboarding

Cloud Integrated customer onboarding

- **`mimecast-cli ci-onboarding`** - Onboard a Cloud Integrated customer account. Requires app role permission: super-admin | full-admin | partner-admin.

### ci_policies

Cloud Integrated security policies, including the default policy

- **`mimecast-cli ci-policies create`** - Create a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli ci-policies delete`** - Delete a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli ci-policies get`** - Get a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli ci-policies get-default`** - Get the Cloud Integrated default policy. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli ci-policies update`** - Update a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli ci-policies update-default`** - Update the Cloud Integrated default policy. Requires app role permission: super-admin | full-admin | partner-admin.

### dlp

Data Leak Prevention policy logs

- **`mimecast-cli dlp`** - Data Leak Prevention policy hit logs. MX-based (Cloud Gateway) deployments only. Requires app role permission: Monitoring | Data Leak Prevention | Read.

### email

Send email through Mimecast

- **`mimecast-cli email`** - Send an email through Mimecast. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Dashboard | Read.

### files

Upload files for email attachments

- **`mimecast-cli files`** - Upload a file for use as an email attachment. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Dashboard | Read.

### gateway

Gateway configuration: details (MX, SPF), protection mode, outbound IPs, statistics

- **`mimecast-cli gateway delete-outbound-ips`** - Delete outbound IP addresses. Requires app role permission: Account | Settings | Edit.
- **`mimecast-cli gateway details`** - Gateway details: MX records, SPF, outbound hostnames, mail platform. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Settings | Read.
- **`mimecast-cli gateway outbound-ips`** - List outbound IP addresses. Requires app role permission: Account | Settings | Read.
- **`mimecast-cli gateway set-outbound-ips`** - Replace outbound IP addresses. Requires app role permission: Account | Settings | Edit.
- **`mimecast-cli gateway set-protection-mode`** - Set protection mode (gateway or gatewayless). Requires app role permission: Account | Settings | Edit.
- **`mimecast-cli gateway statistics`** - Mail-flow statistics: first/last inbound, outbound, internal and queue sizes. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Dashboard | Read.
- **`mimecast-cli gateway update-details`** - Update gateway details. Requires app role permission: Account | Settings | Edit.

### held

Hold queue: find, list, summarize, release and reject held messages, plus the release/reject log

- **`mimecast-cli held find`** - Find held messages with filters (reason, policy, route, held group, spam verdict). Requires app role permission: Account | Monitoring | Held | Read.
- **`mimecast-cli held list`** - List held messages with hold reason, reason code and policy. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Dashboard | Read.
- **`mimecast-cli held reject`** - Reject held messages, optionally notifying the sender. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Monitoring | Held | Edit.
- **`mimecast-cli held release`** - Release held messages to their recipients (up to 25 IDs). MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Monitoring | Held | Edit.
- **`mimecast-cli held release-logs`** - Held-message release and reject log: hold reason, policy, operator, spam and SPF/DKIM/DMARC detail. MX-based (Cloud Gateway) deployments only. Requires app role permission: Monitoring | Held | Read.
- **`mimecast-cli held summary`** - Count held messages per policy or definition. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Monitoring | Held Summary | Read.

### journaling

Journaling connectors and services

- **`mimecast-cli journaling create`** - Create a journaling service. Requires app role permission: Gateway | Policies | Edit.
- **`mimecast-cli journaling delete`** - Delete a journaling service. Requires app role permission: Services | Journaling | Edit.
- **`mimecast-cli journaling get`** - Get a journaling service. Requires app role permission: Services | Journaling | Read.
- **`mimecast-cli journaling get-service`** - Get journaling connector configuration and status. Requires app role permission: Services | Journaling | Read.
- **`mimecast-cli journaling list`** - List journaling services. Requires app role permission: Services | Journaling | Read.
- **`mimecast-cli journaling update`** - Update a journaling service. Requires app role permission: Services | Journaling | Edit.

### messages

Message tracking: search, full detail (spam, SPF/DKIM/DMARC, delivery), file download URL

- **`mimecast-cli messages get`** - Full message detail: delivery, retention, spam verdict, SPF/DKIM/DMARC checks. MX-based (Cloud Gateway) deployments only. Requires app role permission: Gateway | Tracking | Read.
- **`mimecast-cli messages get-file`** - Get a pre-signed download URL for a message file. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Dashboard | Read.
- **`mimecast-cli messages search`** - Track and trace messages by sender, recipient, subject, IP, URL, route and status. MX-based (Cloud Gateway) deployments only. Requires app role permission: Gateway | Tracking | Read.

### processing

Messages in the processing and bulk-processing queues

- **`mimecast-cli processing`** - Find messages stuck in the processing or bulk-processing queue. MX-based (Cloud Gateway) deployments only. Requires app role permission: Monitoring | Processing | Read.

### queues

Inbound and outbound email queue counts

- **`mimecast-cli queues`** - Inbound and outbound email queue counts over a time window. MX-based (Cloud Gateway) deployments only. Requires app role permission: Account | Dashboard | Read.

### rejections

Messages rejected at the gateway (7-day lookback)

- **`mimecast-cli rejections`** - Messages rejected at the gateway (7-day lookback): reason, spam score, sending IP. MX-based (Cloud Gateway) deployments only. Requires app role permission: Monitoring | Rejections | Read.

### remediation

Cloud Integrated threat remediation: delete or release security events, single or bulk

- **`mimecast-cli remediation bulk`** - Remediate many security events in one batch. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli remediation bulk-status`** - Progress of a bulk remediation batch. Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli remediation remediate`** - Delete or release one security event (MANUAL_DELETE or RELEASE). Requires app role permission: super-admin | full-admin | partner-admin.
- **`mimecast-cli remediation status`** - Remediation status of one security event. Requires app role permission: super-admin | full-admin | partner-admin.

### senders

Managed senders: permit or block a sender for a recipient, bulk delete entries

- **`mimecast-cli senders bulk-delete`** - Bulk delete managed sender entries by sender address or domain. MX-based (Cloud Gateway) deployments only. Requires app role permission: Gateway | Managed Senders | Edit.
- **`mimecast-cli senders bulk-delete-for-recipient`** - Bulk delete managed sender entries for one recipient. MX-based (Cloud Gateway) deployments only. Requires app role permission: Gateway | Managed Senders | Edit.
- **`mimecast-cli senders bulk-delete-internal`** - Bulk delete managed sender entries for internal addresses. MX-based (Cloud Gateway) deployments only. Requires app role permission: Gateway | Managed Senders | Edit.
- **`mimecast-cli senders permit-or-block`** - Permit or block a sender for a recipient. MX-based (Cloud Gateway) deployments only. Requires app role permission: Gateway | Managed Senders | Edit.

### ttp

Targeted Threat Protection logs: URL clicks, attachments, impersonation

- **`mimecast-cli ttp attachment-logs`** - Attachment Protect logs: file, hash, sandbox result, action. Requires app role permission: Monitoring | Attachment Protection | Read.
- **`mimecast-cli ttp impersonation-logs`** - Impersonation Protect logs: identifiers hit, action, tagged malicious. Requires app role permission: Monitoring | Impersonation Protection | Read.
- **`mimecast-cli ttp url-logs`** - URL Protect click logs: scan result, category, action. Requires app role permission: Monitoring | URL Protection | Read.

### urls

URL Protect: decode Mimecast-rewritten links

- **`mimecast-cli urls`** - Decode Mimecast-rewritten URL Protect links (up to 25). Requires app role permission: Account | Dashboard | Read.


### held


- **`mimecast-cli held triage`** - See the whole hold queue grouped by hold reason, policy or sender domain, with a release-risk class per group and ready-to-act ID batches.

- **`mimecast-cli held release-safe`** - Release held messages after a keyword-based hold-reason check (a heuristic, not verified against every Mimecast hold reason) that refuses impersonation, malware, sandbox and policy holds unless you give an override reason, and record the ticket in a local ledger.

- **`mimecast-cli held purge-sender`** - Plan, then with --yes reject, every held message from one sender address or domain, optionally blocking that sender for each recipient it targeted.

- **`mimecast-cli held release-audit`** - Review who released or rejected held messages over a period, flagging risky releases and marking those with no ticket in the local release-safe ledger.


### mailflow


- **`mimecast-cli mailflow`** - One OK/WARN/FAIL verdict for tenant mail flow: queue backlog, no inbound mail within a window, and stuck deliveries grouped by destination, with a monitor-friendly exit code.


### senders


- **`mimecast-cli senders suggest`** - Find senders that keep getting held then released (narrow permit candidates, excluding any with impersonation or malware holds or failed SPF/DKIM/DMARC) or keep getting rejected (block candidates), as a ready managed-sender payload.


### threats


- **`mimecast-cli threats mailbox`** - Show every threat event that touched one mailbox over a window, newest first: URL Protect clicks, attachment sandbox verdicts, impersonation hits, gateway rejections and messages held for it, with a per-source fetch report so a failed source never reads as clean.

- **`mimecast-cli threats sender`** - Given a sender address or @domain, list every recipient it reached or tried to reach across URL clicks, attachment verdicts, impersonation hits, rejections and holds, with per-recipient counts by kind, who clicked (clickers first) and the held message IDs.


### ttp


- **`mimecast-cli ttp who-clicked`** - Given a raw or Mimecast-rewritten URL, list who clicked it, when, whether it was blocked, and whether they clicked through a warning (delivery-time entry scans are not counted as clicks).


### why


- **`mimecast-cli why`** - Explain one message in a single verdict: its stage (held, rejected-at-gateway, released-from-hold, rejected-from-hold, the tracking status such as delivery or bounced, or not-found) and why, with policy, reason, spam score, SPF/DKIM/DMARC and who released or rejected it.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`mimecast-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`mimecast-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`mimecast-cli learnings list`** - Inspect taught rows
- **`mimecast-cli learnings forget <query>`** - Undo a teach
- **`mimecast-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`mimecast-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`mimecast-cli teach-pattern`** - Install a query/resource template up front
- **`mimecast-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `MIMECAST_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `mimecast-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
mimecast-cli ci-policies get mock-value

# JSON for scripting and agents
mimecast-cli ci-policies get mock-value --json
# Filter to specific fields
mimecast-cli ci-policies get mock-value --json --select accountId,actions,alerts

# Dry run  -  show the request without sending
mimecast-cli ci-policies get mock-value --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
mimecast-cli ci-policies get mock-value --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Health Check

```bash
mimecast-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `mimecast-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/mimecast-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\mimecast-cli\config.toml` on Windows; `--home`, `MIMECAST_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `MIMECAST_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `MIMECAST_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MIMECAST_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `MIMECAST_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `mimecast-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `mimecast-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MIMECAST_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 invalid_client_identifier or 'Client credentials are invalid'**  -  Use API 2.0 keys (Client ID/Secret from the Mimecast API 2.0 tile), not API 1.0 access/secret keys
- **404 from the token endpoint**  -  Unset MIMECAST_TOKEN_URL (or config token_url) or point it at https://<region>-api.services.mimecast.com/oauth/token; API 1.0 hosts such as us-api.mimecast.com have no /oauth/token. An API 1.0 host in MIMECAST_BASE_URL makes the API calls fail instead
- **app_forbidden: application scope does not extend to this product**  -  Edit the API 2.0 application in the Administration Console and tick the product (for example Audit Events or Security Events), then wait a few minutes
- **403 on held release or reject**  -  Give the application role Account | Monitoring | Held | Edit
- **429 rate_limit 'temporarily busy'**  -  Read commands back off and retry automatically; writes (release, reject, permit/block) exit 7 instead of replaying, so rerun them after the wait
- **Exit code 5 with 'mimecast function failure'**  -  Mimecast returned HTTP 200 with a fail[] entry; the printed code and message name the bad field or ID

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PSMimecast**](https://github.com/MrPig91/PSMimecast)  -  PowerShell (3 stars)
- [**WYRE-AI/mimecast-mcp**](https://github.com/WYRE-AI/mimecast-mcp)  -  TypeScript (1 stars)
- [**splunk-soar-connectors/mimecast**](https://github.com/splunk-soar-connectors/mimecast)  -  Python (1 stars)
- [**Cortex XSOAR MimecastV2**](https://github.com/demisto/content/tree/master/Packs/Mimecast)  -  Python
- [**Rapid7 InsightConnect mimecast**](https://github.com/rapid7/insightconnect-plugins/tree/master/plugins/mimecast)  -  Python
- [**Elastic mimecast integration**](https://github.com/elastic/integrations/tree/main/packages/mimecast)  -  YAML
- [**pwshMimecast**](https://github.com/billkindle/pwshMimecast)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
