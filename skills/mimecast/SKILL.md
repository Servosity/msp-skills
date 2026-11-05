---
name: mimecast
description: "Find a held or blocked Mimecast message, explain why in one verdict, and release it behind a safety check, with hold-queue triage, mail-flow health and phish click tracing built in. Trigger phrases: `why was this email blocked`, `release my held email`, `check the mimecast hold queue`, `who clicked this link`, `is mail flowing`, `use mimecast`, `run mimecast`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Mimecast"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - mimecast-cli
    install:
      - kind: script
        bins: [mimecast-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.ps1
---

# Mimecast Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `mimecast-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `mimecast-cli` and `mimecast-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The Mimecast API 2.0 hold-queue, message tracking, TTP and DLP log, audit and Cloud Integrated endpoints, plus workflow commands that join them. 'why' joins tracking, hold queue, release log and rejection log into one answer; 'held triage' groups the queue by reason with a release-risk class; 'held release-safe' refuses risky releases and records the ticket in a local ledger; 'mailflow' gives a single health verdict.

## When to Use This CLI

Use this CLI for Mimecast email-security operations: tracing a missing or blocked message, explaining why it was held or rejected, working the admin hold queue, releasing or rejecting held mail, managed-sender permit/block hygiene, mail-flow health checks, phish click tracing, release audits, and Cloud Integrated policy and threat remediation calls. Most workflows use endpoints Mimecast marks MX-based (Cloud Gateway) deployments only; remediation and ci-* commands are for Cloud Integrated tenants.

## Anti-triggers

Do not use this CLI for:
- Do not use it for Mimecast archive search or message retrieval (the archive product API is not included)
- Do not use it to manage directory users and groups, managed URLs, or blocked-sender, anti-spoofing or address-alteration policies (separate Mimecast products not included); per-recipient permit/block is supported through senders permit-or-block and held purge-sender --block
- Do not use it for Threat Management remediation incidents or file-hash search; only Cloud Integrated security-event remediation is included
- Do not use it with API 1.0 access keys or HMAC auth
- Do not use it to pull SIEM batch or stream event feeds; use a SIEM connector

## Unique Capabilities

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

## Command Reference

**archive_logs**  -  Archive search and view logs

- `mimecast-cli archive-logs search-logs`  -  Archive search log entries in a time range. Requires app role permission: Archive | Search Logs | Read.
- `mimecast-cli archive-logs searches`  -  Log of archive searches run by admins and users. Requires app role permission: Archive | Search Logs | Read.
- `mimecast-cli archive-logs views`  -  Log of archived messages viewed by admins. Requires app role permission: Archive | View Logs | Read.

**audit**  -  Administration console audit events and categories

- `mimecast-cli audit categories`  -  List audit event categories (codes for audit events --data categories).
- `mimecast-cli audit events`  -  Admin console audit events by category and time range. Requires app role permission: Account | Logs | Read.

**ci_onboarding**  -  Cloud Integrated customer onboarding

- `mimecast-cli ci-onboarding`  -  Onboard a Cloud Integrated customer account. Requires app role permission: super-admin | full-admin | partner-admin.

**ci_policies**  -  Cloud Integrated security policies, including the default policy

- `mimecast-cli ci-policies create`  -  Create a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli ci-policies delete`  -  Delete a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli ci-policies get`  -  Get a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli ci-policies get-default`  -  Get the Cloud Integrated default policy. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli ci-policies update`  -  Update a Cloud Integrated policy. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli ci-policies update-default`  -  Update the Cloud Integrated default policy. Requires app role permission: super-admin | full-admin | partner-admin.

**dlp**  -  Data Leak Prevention policy logs

- `mimecast-cli dlp`  -  Data Leak Prevention policy hit logs. MX-based (Cloud Gateway) deployments only.

**email**  -  Send email through Mimecast

- `mimecast-cli email`  -  Send an email through Mimecast. MX-based (Cloud Gateway) deployments only.

**files**  -  Upload files for email attachments

- `mimecast-cli files`  -  Upload a file for use as an email attachment. MX-based (Cloud Gateway) deployments only.

**gateway**  -  Gateway configuration: details (MX, SPF), protection mode, outbound IPs, statistics

- `mimecast-cli gateway delete-outbound-ips`  -  Delete outbound IP addresses. Requires app role permission: Account | Settings | Edit.
- `mimecast-cli gateway details`  -  Gateway details: MX records, SPF, outbound hostnames, mail platform. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli gateway outbound-ips`  -  List outbound IP addresses. Requires app role permission: Account | Settings | Read.
- `mimecast-cli gateway set-outbound-ips`  -  Replace outbound IP addresses. Requires app role permission: Account | Settings | Edit.
- `mimecast-cli gateway set-protection-mode`  -  Set protection mode (gateway or gatewayless). Requires app role permission: Account | Settings | Edit.
- `mimecast-cli gateway statistics`  -  Mail-flow statistics: first/last inbound, outbound, internal and queue sizes. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli gateway update-details`  -  Update gateway details. Requires app role permission: Account | Settings | Edit.

**held**  -  Hold queue: find, list, summarize, release and reject held messages, plus the release/reject log

- `mimecast-cli held find`  -  Find held messages with filters (reason, policy, route, held group, spam verdict).
- `mimecast-cli held list`  -  List held messages with hold reason, reason code and policy. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli held reject`  -  Reject held messages, optionally notifying the sender. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli held release`  -  Release held messages to their recipients (up to 25 IDs). MX-based (Cloud Gateway) deployments only.
- `mimecast-cli held release-logs`  -  Held-message release and reject log: hold reason, policy, operator, spam and SPF/DKIM/DMARC detail.
- `mimecast-cli held summary`  -  Count held messages per policy or definition. MX-based (Cloud Gateway) deployments only.

**journaling**  -  Journaling connectors and services

- `mimecast-cli journaling create`  -  Create a journaling service. Requires app role permission: Gateway | Policies | Edit.
- `mimecast-cli journaling delete`  -  Delete a journaling service. Requires app role permission: Services | Journaling | Edit.
- `mimecast-cli journaling get`  -  Get a journaling service. Requires app role permission: Services | Journaling | Read.
- `mimecast-cli journaling get-service`  -  Get journaling connector configuration and status. Requires app role permission: Services | Journaling | Read.
- `mimecast-cli journaling list`  -  List journaling services. Requires app role permission: Services | Journaling | Read.
- `mimecast-cli journaling update`  -  Update a journaling service. Requires app role permission: Services | Journaling | Edit.

**messages**  -  Message tracking: search, full detail (spam, SPF/DKIM/DMARC, delivery), file download URL

- `mimecast-cli messages get`  -  Full message detail: delivery, retention, spam verdict, SPF/DKIM/DMARC checks.
- `mimecast-cli messages get-file`  -  Get a pre-signed download URL for a message file. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli messages search`  -  Track and trace messages by sender, recipient, subject, IP, URL, route and status.

**processing**  -  Messages in the processing and bulk-processing queues

- `mimecast-cli processing`  -  Find messages stuck in the processing or bulk-processing queue. MX-based (Cloud Gateway) deployments only.

**queues**  -  Inbound and outbound email queue counts

- `mimecast-cli queues`  -  Inbound and outbound email queue counts over a time window. MX-based (Cloud Gateway) deployments only.

**rejections**  -  Messages rejected at the gateway (7-day lookback)

- `mimecast-cli rejections`  -  Messages rejected at the gateway (7-day lookback): reason, spam score, sending IP.

**remediation**  -  Cloud Integrated threat remediation: delete or release security events, single or bulk

- `mimecast-cli remediation bulk`  -  Remediate many security events in one batch. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli remediation bulk-status`  -  Progress of a bulk remediation batch. Requires app role permission: super-admin | full-admin | partner-admin.
- `mimecast-cli remediation remediate`  -  Delete or release one security event (MANUAL_DELETE or RELEASE).
- `mimecast-cli remediation status`  -  Remediation status of one security event. Requires app role permission: super-admin | full-admin | partner-admin.

**senders**  -  Managed senders: permit or block a sender for a recipient, bulk delete entries

- `mimecast-cli senders bulk-delete`  -  Bulk delete managed sender entries by sender address or domain. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli senders bulk-delete-for-recipient`  -  Bulk delete managed sender entries for one recipient. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli senders bulk-delete-internal`  -  Bulk delete managed sender entries for internal addresses. MX-based (Cloud Gateway) deployments only.
- `mimecast-cli senders permit-or-block`  -  Permit or block a sender for a recipient. MX-based (Cloud Gateway) deployments only.

**ttp**  -  Targeted Threat Protection logs: URL clicks, attachments, impersonation

- `mimecast-cli ttp attachment-logs`  -  Attachment Protect logs: file, hash, sandbox result, action.
- `mimecast-cli ttp impersonation-logs`  -  Impersonation Protect logs: identifiers hit, action, tagged malicious.
- `mimecast-cli ttp url-logs`  -  URL Protect click logs: scan result, category, action. Requires app role permission: Monitoring | URL Protection | Read.

**urls**  -  URL Protect: decode Mimecast-rewritten links

- `mimecast-cli urls`  -  Decode Mimecast-rewritten URL Protect links (up to 25). Requires app role permission: Account | Dashboard | Read.


**held**


- `mimecast-cli held triage`  -  See the whole hold queue grouped by hold reason, policy or sender domain, with a release-risk class per group and ready-to-act ID batches.

- `mimecast-cli held release-safe`  -  Release held messages after a keyword-based hold-reason check (a heuristic, not verified against every Mimecast hold reason) that refuses impersonation, malware, sandbox and policy holds unless you give an override reason, and record the ticket in a local ledger.

- `mimecast-cli held purge-sender`  -  Plan, then with --yes reject, every held message from one sender address or domain, optionally blocking that sender for each recipient it targeted.

- `mimecast-cli held release-audit`  -  Review who released or rejected held messages over a period, flagging risky releases and marking those with no ticket in the local release-safe ledger.


**mailflow**


- `mimecast-cli mailflow`  -  One OK/WARN/FAIL verdict for tenant mail flow: queue backlog, no inbound mail within a window, and stuck deliveries grouped by destination, with a monitor-friendly exit code.


**senders**


- `mimecast-cli senders suggest`  -  Find senders that keep getting held then released (narrow permit candidates, excluding any with impersonation or malware holds or failed SPF/DKIM/DMARC) or keep getting rejected (block candidates), as a ready managed-sender payload.


**threats**


- `mimecast-cli threats mailbox`  -  Show every threat event that touched one mailbox over a window, newest first: URL Protect clicks, attachment sandbox verdicts, impersonation hits, gateway rejections and messages held for it, with a per-source fetch report so a failed source never reads as clean.

- `mimecast-cli threats sender`  -  Given a sender address or @domain, list every recipient it reached or tried to reach across URL clicks, attachment verdicts, impersonation hits, rejections and holds, with per-recipient counts by kind, who clicked (clickers first) and the held message IDs.


**ttp**


- `mimecast-cli ttp who-clicked`  -  Given a raw or Mimecast-rewritten URL, list who clicked it, when, whether it was blocked, and whether they clicked through a warning (delivery-time entry scans are not counted as clicks).


**why**


- `mimecast-cli why`  -  Explain one message in a single verdict: its stage (held, rejected-at-gateway, released-from-hold, rejected-from-hold, the tracking status such as delivery or bounced, or not-found) and why, with policy, reason, spam score, SPF/DKIM/DMARC and who released or rejected it.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
mimecast-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

Mimecast API 2.0 uses OAuth2 client credentials. In the Administration Console go to Administration > Services > API and Platform Integrations > Mimecast API 2.0 > Generate Keys, choose the products (Email Security Cloud Gateway, Audit Events, Security Events as needed) and a least-privilege role, then export MIMECAST_CLIENT_ID and MIMECAST_CLIENT_SECRET. The CLI exchanges them at https://api.services.mimecast.com/oauth/token for a 30-minute bearer token and re-mints automatically. API 1.0 keys and hosts (us-api.mimecast.com, eu-api.mimecast.com) do not work with API 2.0. For data residency set both MIMECAST_BASE_URL and MIMECAST_TOKEN_URL to a regional gateway, for example https://uk-api.services.mimecast.com and https://uk-api.services.mimecast.com/oauth/token (us-api and de-api work the same way). MSPs with delegated access can set MIMECAST_ACCOUNT_CODE (or --account-code on the workflow commands): it is sent as meta.accountCode on the classic /api/ envelope endpoints only (hold queue, tracking, logs, managed-sender permit/block); REST endpoints (gateway, journaling, ci-policies, remediation, senders bulk-delete) have no account selector, so while an account code is set the CLI refuses their writes and warns on their reads (they would act on the API application's own account). Delegation is unverified against a live delegate tenant.

Run `mimecast-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  mimecast-cli ci-policies get mock-value --agent --select accountId,actions,alerts
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

- Use `--home <dir>` for one invocation, or set `MIMECAST_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `MIMECAST_CONFIG_DIR`, `MIMECAST_DATA_DIR`, `MIMECAST_STATE_DIR`, `MIMECAST_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `MIMECAST_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `mimecast-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `MIMECAST_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `MIMECAST_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; The connector keeps no HTTP response cache. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `MIMECAST_NO_LOCAL_DATA=1` disables store writes (the read-only workflow commands included), turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `MIMECAST_DATA_SOURCE` sets the `--data-source` default.

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
mimecast-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "mimecast-cli learnings confirm 12"] }
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
       materially more, record the divergence via `mimecast-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `mimecast-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `mimecast-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
mimecast-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
mimecast-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
mimecast-cli teach-playbook \
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
mimecast-cli playbook amend \
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

`mimecast-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `MIMECAST_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
mimecast-cli feedback "the --since flag is inclusive but docs say exclusive"
mimecast-cli feedback --stdin < notes.txt
mimecast-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `MIMECAST_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MIMECAST_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
mimecast-cli profile save briefing --json
mimecast-cli --profile briefing ci-policies get mock-value
mimecast-cli profile list --json
mimecast-cli profile show briefing
mimecast-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

If `--wait` times out or polling fails after the job was accepted, the command exits `8` and prints the job ID plus a recovery command that fetches the result. The job may still be running and may already be billed, so run the recovery command instead of resubmitting.

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
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `mimecast-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add mimecast-mcp -- mimecast-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `mimecast-mcp --transport http` requires `MIMECAST_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `MIMECAST_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `MIMECAST_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which mimecast-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   mimecast-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `mimecast-cli <command> --help`.
