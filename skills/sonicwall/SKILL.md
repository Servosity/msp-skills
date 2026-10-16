---
name: sonicwall
description: "Use when the user asks about client SonicWall firewalls through NSM SaaS or MySonicWall: grades every firewall for firmware, end of support, PSIRT advisories, licence expiry, admin TOTP and SSL-VPN exposure in one read-only pass, triages one advisory or CVE across the fleet, lists SSL-VPN users without MFA, builds the licence renewal list and finds MySonicWall firewalls NSM cannot see. Trigger phrases: `sonicwall posture report`, `which sonicwall firewalls are vulnerable`, `which clients are hit by CVE-2024-40766`, `sonicwall mfa audit`, `sonicwall licences expiring`, `sonicwall firmware behind`, `firewalls missing from NSM`, `use sonicwall`, `run sonicwall-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "SonicWall"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - sonicwall-cli
    install:
      - kind: script
        bins: [sonicwall-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1
---

# SonicWall - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `sonicwall-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex
   ```
3. Verify: `sonicwall-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `sonicwall-cli` and `sonicwall-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Reads Network Security Manager (NSM SaaS) and MySonicWall with one MySonicWall API key. 'posture' answers the insurer questionnaire per firewall, 'exposure' triages a SonicWall advisory down to the firewalls with SSL-VPN reachable on WAN, and 'mfa-audit' lists every local account without a one-time password and flags firewalls whose users sign in through SAML or a directory. Read-only by design.

## When to Use This CLI

Use this CLI to answer fleet-level security posture questions about SonicWall firewalls managed in NSM SaaS: patch level, published advisory exposure, admin and SSL-VPN MFA, licence and support expiry, and firewalls missing from NSM. It suits cyber-insurance questionnaires, compliance evidence and weekly patch reviews where an agent needs per-firewall rows with pass, warn, fail or unknown (unknown means the data could not be read; it is never a pass).

## Anti-triggers

Do not use this CLI for:
- Changing firewall configuration, pushing policy, upgrading firmware or committing changes; this CLI is read-only
- Firewalls not onboarded to NSM SaaS, or on-box SonicOS API sessions with local admin credentials
- Real-time log or traffic analysis from syslog streams
- SonicWall Email Security, SMA remote access appliances or Capture Client endpoint management

## Unique Capabilities

### Cyber-insurance evidence
- **`posture`**  -  One row per firewall with firmware currency, end-of-support date, published advisories, soonest licence expiry, admin TOTP, SSL-VPN exposure and SSL-VPN users without one-time passwords; each check is pass, warn, fail or unknown, and the licence check needs SONICWALL_MSW_API_KEY.

  _Reach for this when an insurer or compliance questionnaire asks whether the SonicWall fleet is patched, MFA-protected and licensed; it answers per firewall with pass, warn, fail or unknown._

  ```bash
  sonicwall-cli posture --agent
  ```
- **`licenses renewals`**  -  Every licence and security service expiring in the next N days (expired ones included), per firewall and client, flagged when the hardware is near end of support; needs SONICWALL_MSW_API_KEY.

  _Use it to build the renewal pipeline and spot boxes that should be replaced rather than renewed._

  ```bash
  sonicwall-cli licenses renewals --days 90 --agent
  ```
- **`mfa-audit`**  -  Per-account remediation list: admin TOTP per firewall and every local SSL-VPN user (direct or nested group member) without a one-time password, with SAML, RADIUS, LDAP or TACACS+ sign-in marked as delegated.

  _Use it when the question is which accounts lack MFA, the gap attackers used against SonicWall SSL-VPN in 2024 and 2025._

  ```bash
  sonicwall-cli mfa-audit --agent
  ```
- **`blind-spots`**  -  Find firewalls registered in MySonicWall that NSM cannot see, with their registered firmware's advisories and support expiry; needs SONICWALL_MSW_API_KEY and an NSM token.

  _Run it before trusting any posture report: firewalls outside NSM are invisible to every other command._

  ```bash
  sonicwall-cli blind-spots --agent
  ```

### Patch and exposure
- **`exposure`**  -  Rank the SonicWall PSIRT advisories NSM matched to your fleet, or name one advisory (SNWLID ID) to list every affected firewall and whether SSL-VPN is enabled on a zone whose name contains WAN; a CVE ID only matches when NSM's advisory text names it.

  _Use it the moment a SonicWall advisory drops (for example CVE-2024-40766) to pick the firewalls that need tonight's change window._

  ```bash
  sonicwall-cli exposure SNWLID-2024-0015 --agent
  ```
- **`firmware drift`**  -  See how far each firewall's running SonicOS build is behind the newest available build and which advisories the upgrade clears.

  _Use it for the weekly patch review instead of comparing firmware strings by hand._

  ```bash
  sonicwall-cli firmware drift --agent
  ```

### Local history
- **`posture changes`**  -  What changed in fleet posture between saved posture runs of the same scope (--fleet, --tenant or --serial picks it; the default is the newest run's): newly vulnerable, MFA turned off, SSL-VPN newly on WAN, licence lapsed, firmware fixed. Local only: run posture now and again later, with the same SONICWALL_MSW_API_KEY and NSM tenant.

  _Use it for the Monday question of what got worse since last week._

  ```bash
  sonicwall-cli posture changes --fleet --since 7d --agent
  ```

## Command Reference

**nsm-login / nsm-get**  -  NSM auth and raw reads

- `sonicwall-cli nsm-login`  -  Mint and save the NSM session token from SONICWALL_MSW_API_KEY (`--tenant` when several NSM tenants).
- `sonicwall-cli nsm-get /path --device <serial>`  -  GET a vendor-documented NSM or SonicOS-passthrough read route without a dedicated command (credential and state-changing routes refused; credential fields redacted).

**alerts**  -  Manage alerts

- `sonicwall-cli alerts count`  -  Get alert count.
- `sonicwall-cli alerts list`  -  Get alerts log data.

**config-audit**  -  Manage config audit

- `sonicwall-cli config-audit count`  -  To get config list count of device / devices for tenants.
- `sonicwall-cli config-audit diff`  -  To get config diff.
- `sonicwall-cli config-audit list`  -  To get config list of device / devices for tenants.

**device-groups**  -  Manage device groups

- `sonicwall-cli device-groups get`  -  Gets device group information by group ID.
- `sonicwall-cli device-groups list`  -  Retrieves list of device groups by tenant.

**devices**  -  Manage devices

- `sonicwall-cli devices get`  -  Gets device information by device serial number.
- `sonicwall-cli devices inventory`  -  Retrieves inventory list of device by tenant.
- `sonicwall-cli devices lifecycle`  -  Gets product life cycle information by device serial number.
- `sonicwall-cli devices list`  -  Retrieves list of device by tenant.
- `sonicwall-cli devices locations`  -  Gets Devices info aggregated on location.
- `sonicwall-cli devices online-status`  -  Gets online status information by device serial number.
- `sonicwall-cli devices summary`  -  Retrieves summary of device list by tenant id.
- `sonicwall-cli devices trend`  -  Retrieves trend of device list.

**firewall**  -  Manage firewall

- `sonicwall-cli firewall access-rules`  -  Retrieve an IPv4 access rule policy. IPv4 access rules (from/to zone, action, source/destination, service, enable).
- `sonicwall-cli firewall address-objects`  -  Retrieve IPv4 address object configuration.
- `sonicwall-cli firewall administration`  -  Retrieve administration configuration.
- `sonicwall-cli firewall license-report`  -  Retrieve License status.
- `sonicwall-cli firewall license-security-services`  -  Retrieve License status.
- `sonicwall-cli firewall local-groups`  -  Retrieve user local group configuration.
- `sonicwall-cli firewall local-users`  -  Retrieve local user configuration. Local users with their one-time-password method (none, OTP via e-mail, or TOTP).
- `sonicwall-cli firewall security-services-status`  -  Retrieve security services status.
- `sonicwall-cli firewall sslvpn-access`  -  Retrieve SSL VPN server accesses configuration.
- `sonicwall-cli firewall sslvpn-portal`  -  Retrieve SSL VPN portal configuration.
- `sonicwall-cli firewall sslvpn-server`  -  Retrieve SSL VPN server configuration.
- `sonicwall-cli firewall sslvpn-sessions`  -  Retrieve SSL VPN sessions statistics.
- `sonicwall-cli firewall system-status`  -  Retrieve system status.
- `sonicwall-cli firewall user-auth`  -  Retrieve user authentication base settings.

**firmware**  -  Manage firmware

- `sonicwall-cli firmware available`  -  Gets new firmware information by device serial number.
- `sonicwall-cli firmware available-list`  -  get available firmware list for serialNumber.
- `sonicwall-cli firmware devices-on-version`  -  List the devices running a given firmware version.
- `sonicwall-cli firmware upgrade-history`  -  get firmware upgrade status history.
- `sonicwall-cli firmware upgrade-status`  -  get latest firmware-upgrade status for serialNumber.
- `sonicwall-cli firmware version-list`  -  get firmware version list.

**licenses**  -  Manage licenses

- `sonicwall-cli licenses expired`  -  Retrieves list of device with expired licenses by tenant.
- `sonicwall-cli licenses expiring`  -  Gets License expiration summary.
- `sonicwall-cli licenses get`  -  Gets license information by device serial number.
- `sonicwall-cli licenses service-status`  -  Gets whether license in enabled for this service of this serial number.
- `sonicwall-cli licenses services`  -  Gets status of services for a serial number.

**msw-activity**  -  Manage msw activity

- `sonicwall-cli msw-activity detail`  -  View user activity log details.
- `sonicwall-cli msw-activity export`  -  View user activity logs for all the users from organization.

**msw-licenses**  -  Manage msw licenses

- `sonicwall-cli msw-licenses`  -  License report.

**msw-products**  -  Manage msw products

- `sonicwall-cli msw-products applicable-services`  -  Gets List of Applicable Products, Services and DataCenter Details for Provisioning a Prodduct or Editing a Provision.
- `sonicwall-cli msw-products firmware`  -  Get a product firmware versions, including latest available firmware.
- `sonicwall-cli msw-products list`  -  Get a list of Products.
- `sonicwall-cli msw-products services`  -  Get product details like license type, node counts and expiry date.

**msw-system**  -  Manage msw system

- `sonicwall-cli msw-system`  -  GET /api/version.

**msw-tenants**  -  Manage msw tenants

- `sonicwall-cli msw-tenants access-code`  -  Get the access code to generate API keys from your cloud subscriptions. Step 2 of the NSM token exchange.
- `sonicwall-cli msw-tenants address`  -  Get tenant address information.
- `sonicwall-cli msw-tenants cloud`  -  Get the list of tenantIds for all active cloud subscriptions. Step 1 of the NSM token exchange.
- `sonicwall-cli msw-tenants firewalls`  -  Returns a list of eligible firewalls for a given tenantId (productgroupId).
- `sonicwall-cli msw-tenants list`  -  Get a list of tenants.

**msw-users**  -  Manage msw users

- `sonicwall-cli msw-users groups`  -  Get a list of user groups.
- `sonicwall-cli msw-users list`  -  Get a list of users and associated user groups.

**nsm-session**  -  Manage nsm session

- `sonicwall-cli nsm-session refresh`  -  Provides a new session token. Invalidates the old token.
- `sonicwall-cli nsm-session sso`  -  GET SSO token from URL for user authentication. NSM will return Bearer Token after successful login.

**nsm-system**  -  Manage nsm system

- `sonicwall-cli nsm-system health`  -  GET /healthcheck. Unauthenticated; returns status.info[0].message HEALTH OK when NSM is up.
- `sonicwall-cli nsm-system status`  -  Show the system status.
- `sonicwall-cli nsm-system version`  -  Get version. Unauthenticated; returns productName, version and buildVersion of the NSM SaaS instance.

**tenants**  -  Manage tenants

- `sonicwall-cli tenants by-serial`  -  To get tenant by serial.
- `sonicwall-cli tenants count`  -  To get tenants count.
- `sonicwall-cli tenants default`  -  To get defaut tenant.
- `sonicwall-cli tenants get`  -  To get tenant by id.
- `sonicwall-cli tenants list`  -  To get all tenants.

**vulnerabilities**  -  Manage vulnerabilities

- `sonicwall-cli vulnerabilities by-firmware`  -  Get vulnerable devices by firmware version. Advisories that apply to one SonicOS firmware version string.
- `sonicwall-cli vulnerabilities devices`  -  Get vulnerable devices by tenant IDs.
- `sonicwall-cli vulnerabilities serials`  -  Get device serialNumbers with vulnerable exp by tenant IDs.


**blind-spots**


- `sonicwall-cli blind-spots`  -  Find firewalls registered in MySonicWall that NSM cannot see, with their registered firmware's advisories and support expiry; needs SONICWALL_MSW_API_KEY and an NSM token.


**exposure**


- `sonicwall-cli exposure`  -  Rank the SonicWall PSIRT advisories NSM matched to your fleet, or name one advisory (SNWLID ID) to list every affected firewall and whether SSL-VPN is enabled on a zone whose name contains WAN; a CVE ID only matches when NSM's advisory text names it.


**firmware**


- `sonicwall-cli firmware drift`  -  See how far each firewall's running SonicOS build is behind the newest available build and which advisories the upgrade clears.


**licenses**


- `sonicwall-cli licenses renewals`  -  Every licence and security service expiring in the next N days (expired ones included), per firewall and client, flagged when the hardware is near end of support; needs SONICWALL_MSW_API_KEY.


**mfa-audit**


- `sonicwall-cli mfa-audit`  -  Per-account remediation list: admin TOTP per firewall and every local SSL-VPN user (direct or nested group member) without a one-time password, with SAML, RADIUS, LDAP or TACACS+ sign-in marked as delegated.


**posture**


- `sonicwall-cli posture`  -  One row per firewall with firmware currency, end-of-support date, published advisories, soonest licence expiry, admin TOTP, SSL-VPN exposure and SSL-VPN users without one-time passwords; each check is pass, warn, fail or unknown, and the licence check needs SONICWALL_MSW_API_KEY.

- `sonicwall-cli posture changes`  -  What changed in fleet posture between saved posture runs of the same scope (--fleet, --tenant or --serial picks it; the default is the newest run's): newly vulnerable, MFA turned off, SSL-VPN newly on WAN, licence lapsed, firmware fixed. Local only: run posture now and again later, with the same SONICWALL_MSW_API_KEY and NSM tenant.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
sonicwall-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Insurance evidence for one firewall

```bash
sonicwall-cli posture --serial 2CB8ED000001 --agent
```

Pass, warn, fail or unknown for firmware, advisories, end of support, licences, admin TOTP and SSL-VPN exposure on one firewall; warn marks firmware behind, SSL-VPN on WAN, support ending within a year, a licence within 30 days and delegated MFA. Drop --serial for the whole fleet.

### Advisory triage

```bash
sonicwall-cli exposure SNWLID-2024-0015 --agent
```

Lists firewalls matched to SonicWall advisory SNWLID-2024-0015 (CVE-2024-40766) and marks which have SSL-VPN enabled on a zone whose name contains WAN (custom-named internet zones are missed); an empty list for a CVE ID means no advisory text matched, not zero exposure.

### Lean inventory for an agent

```bash
sonicwall-cli devices list --agent --select serialNumber,friendlyName,firmwareVersion,liveStatus
```

Narrows the large device payload to the four fields most posture questions need.

### Renewal pipeline

```bash
sonicwall-cli licenses renewals --days 90 --agent
```

Returns {days, renewals[], fetch_failures[]} (inside .results under --agent; fetch_failures only when a lookup failed): every licence expiring within 90 days, already expired, or with no readable expiry (expiry_unknown), with replace_instead yes/no/unknown. A firewall in fetch_failures was not checked.

### What got worse this week

```bash
sonicwall-cli posture changes --fleet --since 7d --agent
```

Compares the newest saved posture run with the latest same-scope run at least 7 days old (or the oldest one). Reads local history only, so run posture first and again later.

## Auth Setup

You need one credential: a MySonicWall API key (MySonicWall > My Workspace > User Groups > User list > Generate My API Key; valid up to a year, and MSW API permission must be enabled for your user). Export it as SONICWALL_MSW_API_KEY; MySonicWall commands, licence checks and blind-spots read it from the environment on every run. NSM calls need a separate NSM session token minted from that key: run 'sonicwall-cli nsm-login' (add --tenant "<name or product group ID>" when the key sees more than one NSM tenant, the usual MSP case) and it performs SonicWall's three-step exchange and saves the token. The token lasts 8 hours with a 15-minute idle timeout; when NSM calls exit 4, rerun nsm-login. Exit 4 from MySonicWall commands means the key is missing or not permitted. If SONICWALL_NSM_TOKEN is set it overrides the saved token. EU portals: export SONICWALL_REGION=eucentral (default uswest). For the MCP server, pass SONICWALL_MSW_API_KEY (and SONICWALL_REGION when not uswest) in the server's env block.

Run `sonicwall-cli doctor` to verify setup.

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
  sonicwall-cli alerts list --agent --select hits,max_score,timestamp
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Read-only**  -  no command changes firewall, NSM or MySonicWall configuration; the only POSTs are read-style queries and the NSM token exchange (`nsm-login`, `nsm-session`, `msw-tenants access-code`)
- **Unknown is never pass**  -  posture, mfa-audit, firmware drift and licenses renewals mark anything they could not read as `unknown` (or list it in `fetch_failures`). A fan-out that could not check a firewall exits 6; `licenses renewals` exits with the underlying error (3, 4 or 5) when every lookup failed, so always read `fetch_failures`. When no NSM firewall matches the token and filter, posture, mfa-audit, firmware drift, licenses renewals and exposure exit 3 with a JSON error instead of an empty list. exposure exits 6 when a firewall has no reported SonicOS version (NSM cannot match advisories to it) or the device inventory is unreadable, and blind-spots exits 6 when NSM firewalls are missing from the MySonicWall registry the key reads; blind-spots drops a product by default only when it is a known non-firewall family and lists anything unrecognised with `product_class: unknown`

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

- Use `--home <dir>` for one invocation, or set `SONICWALL_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `SONICWALL_CONFIG_DIR`, `SONICWALL_DATA_DIR`, `SONICWALL_STATE_DIR`, `SONICWALL_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `SONICWALL_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `sonicwall-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "sonicwall": {
        "command": "sonicwall-mcp",
        "env": {
          "SONICWALL_HOME": "/srv/sonicwall"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `SONICWALL_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `SONICWALL_HOME`, or `doctor` will not find credentials left under the former root.

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
sonicwall-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "sonicwall-cli learnings confirm 12"] }
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
       materially more, record the divergence via `sonicwall-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `sonicwall-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `sonicwall-cli sync` to refresh entity lookups, after the user confirms: sync is a Sensitive read (it copies config-audit configuration text into the local store).
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
sonicwall-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias satisfies future queries under another automatically. This CLI ships no alias seeds; serials, tenant IDs and SNWLID/CVE IDs are matched as identifiers.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
sonicwall-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
sonicwall-cli teach-playbook \
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
sonicwall-cli playbook amend \
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

`sonicwall-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `SONICWALL_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
sonicwall-cli feedback "the --since flag is inclusive but docs say exclusive"
sonicwall-cli feedback --stdin < notes.txt
sonicwall-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `SONICWALL_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SONICWALL_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
sonicwall-cli profile save briefing --json
sonicwall-cli --profile briefing alerts list
sonicwall-cli profile list --json
sonicwall-cli profile show briefing
sonicwall-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Other error |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `sonicwall-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add sonicwall-mcp --env SONICWALL_MSW_API_KEY="<your MySonicWall API key>" -- sonicwall-mcp
   ```
   Add `--env SONICWALL_REGION=eucentral` for EU NSM portals.
3. Run `sonicwall-cli nsm-login` before using NSM tools, and again whenever they exit 4 (8-hour token, 15-minute idle timeout); the MCP server reads the saved token.
4. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which sonicwall-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   sonicwall-cli posture --agent
   ```
4. If ambiguous, drill into subcommand help, for example `sonicwall-cli firmware --help`.
