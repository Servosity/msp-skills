---
name: fortinet
description: "Use when the user asks to audit, inventory or back up client FortiGate firewalls, or read FortiManager or FortiGate Cloud: grades every FortiGate in a devices file for firmware, SSL-VPN exposure, admin accounts and license expiry in one read-only pass, plus HA, tunnel and certificate health, policy hygiene and config changes. Trigger phrases: `audit our fortigates`, `which client firewalls are exposing ssl-vpn`, `which fortigates need a firmware update`, `fortiguard license expiry across clients`, `back up every fortigate config`, `what changed on the firewall`, `list fortimanager managed devices`, `use fortinet`, `run fortinet-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Fortinet"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - fortinet-cli
    install:
      - kind: script
        bins: [fortinet-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.ps1
---

# Fortinet - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `fortinet-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.ps1 | iex
   ```
3. Verify: `fortinet-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `fortinet-cli` and `fortinet-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Typed, read-only commands cover the FortiOS REST API (cmdb configuration tables and monitor endpoints) on any FortiGate. A devices file turns them into fleet commands: fleet audit grades every client box, fleet health shows operational state, and fleet backup plus fleet changes track configuration drift. fmg commands read FortiManager ADOMs and managed devices, and cloud commands read the FortiGate Cloud inventory.

## When to Use This CLI

Use this CLI when an agent or MSP technician needs to read FortiGate configuration or live state, or to audit many client FortiGates at once: firmware currency, SSL-VPN exposure, admin account hygiene, licence expiry, HA and tunnel health, policy hygiene and configuration drift. Fleet and audit commands need a devices file for more than one FortiGate (check it with 'fleet devices'); fmg commands need FortiManager credentials (check with 'fmg status') and cloud commands need a FortiCloud IAM API user (check with 'cloud devices'). It never creates, updates or deletes anything on FortiGate, FortiManager or FortiGate Cloud, so it is safe to point at production firewalls.

## Anti-triggers

Do not use this CLI for:
- Do not use it to change configuration, push policies or upgrade firmware; it is read-only. Use the FortiGate GUI, FortiManager or Ansible for writes.
- Do not use it to search historical traffic or event logs; that belongs to FortiAnalyzer or your SIEM.
- Do not use it for FortiClient EMS endpoint management or FortiSwitch/FortiAP configuration beyond the status the FortiGate reports.

## Unique Capabilities

### Fleet security posture
- **`fleet audit`**  -  One PASS/WARN/FAIL/UNKNOWN matrix across every FortiGate in your devices file for firmware, SSL-VPN exposure, admin accounts and FortiGuard licence expiry.

  _Reach for this when asked which client firewalls are at risk; one call replaces dozens of per-device lookups, and --fail-on fail makes it exit 6 on any FAIL or unchecked (UNKNOWN) device for cron or CI._

  ```bash
  fortinet-cli fleet audit --new --agent
  ```
- **`audit firmware`**  -  Flags FortiGates below your per-branch firmware floor and how many patches each is behind the newest image offered in its branch.

  _Use it after a Fortinet PSIRT advisory to list exactly which boxes are below your patched floor; run 'monitor upgrade-paths' on a device for its supported upgrade hops._

  ```bash
  fortinet-cli audit firmware --min-version 7.2.10,7.4.5 --agent
  ```
- **`audit sslvpn`**  -  Grades each FortiGate and VDOM on whether SSL-VPN is enabled, listening on a WAN interface, and open to any source address.

  _SSL-VPN is the most exploited FortiGate surface; this answers 'who is exposed' across clients in one call._

  ```bash
  fortinet-cli audit sslvpn --agent
  ```
- **`audit admins`**  -  Flags admins with no trusted hosts (FAIL when HTTPS/SSH management is open on WAN), super_admin accounts without two-factor, WAN interfaces allowing HTTP or Telnet, and API users trusted from broad networks or using super_admin.

  _Use it for access reviews and after staff changes; it shows which admin accounts are a real exposure, not just present._

  ```bash
  fortinet-cli audit admins --broad-prefix 16 --agent
  ```
- **`audit licenses`**  -  One renewal queue of FortiGuard, FortiCare and service licences across the fleet, sorted by days left.

  _Use it monthly for renewals so no client silently loses FortiGuard protection._

  ```bash
  fortinet-cli audit licenses --within 60d --agent
  ```

### Fleet operations
- **`fleet health`**  -  One PASS/WARN/FAIL/UNKNOWN row per FortiGate: HA config sync, IPsec tunnels down, SD-WAN members down, CPU and memory pressure, expiring certificates; anything it could not read makes the row UNKNOWN.

  _Use it for the weekly NOC check or first thing in an incident to see which boxes are unhealthy._

  ```bash
  fortinet-cli fleet health --agent
  ```
- **`policies hygiene`**  -  Lists unused, any-any accept, logging-disabled and disabled firewall policies per VDOM.

  _Use it before a policy cleanup or audit to find dead and risky rules without exporting anything._

  ```bash
  fortinet-cli policies hygiene --unused-days 90 --agent
  ```
- **`fleet changes`**  -  Shows which FortiOS objects were added, removed or modified between two saved 'fleet backup' files (needs at least two backups per device; secrets are redacted, so a secret-only change is not reported).

  _Use it to answer 'what changed on this client's firewall this week' during change review or after an incident._

  ```bash
  fortinet-cli fleet changes --since 7d --agent
  ```

## Command Reference

**address_groups**  -  Manage address groups

- `fortinet-cli address-groups get`  -  Get one address-groups entry by name.
- `fortinet-cli address-groups list`  -  Firewall address groups (firewall addrgrp).

**addresses**  -  Manage addresses

- `fortinet-cli addresses get`  -  Get one addresses entry by name.
- `fortinet-cli addresses list`  -  Firewall address objects (firewall address).

**admin_profiles**  -  Manage admin profiles

- `fortinet-cli admin-profiles get`  -  Get one admin-profiles entry by name.
- `fortinet-cli admin-profiles list`  -  Administrator access profiles (system accprofile) with read/read-write permission groups.

**administrators**  -  Manage administrators

- `fortinet-cli administrators get`  -  Get one administrators entry by name.
- `fortinet-cli administrators list`  -  Administrator accounts (system admin).

**api_users**  -  Manage api users

- `fortinet-cli api-users get`  -  Get one api-users entry by name.
- `fortinet-cli api-users list`  -  REST API administrators (system api-user) and their trusted hosts.

**dhcp_servers**  -  Manage dhcp servers

- `fortinet-cli dhcp-servers get`  -  Get one dhcp-servers entry by id.
- `fortinet-cli dhcp-servers list`  -  DHCP server scopes (system dhcp server).

**interfaces**  -  Manage interfaces

- `fortinet-cli interfaces get`  -  Get one interfaces entry by name.
- `fortinet-cli interfaces list`  -  Interface configuration (system interface), including allowaccess (which management protocols listen on each interface).

**ipsec_phase1**  -  Manage ipsec phase1

- `fortinet-cli ipsec-phase1 get`  -  Get one ipsec-phase1 entry by name.
- `fortinet-cli ipsec-phase1 list`  -  IPsec phase 1 (route-based) tunnel definitions (vpn ipsec phase1-interface).

**ipsec_phase2**  -  Manage ipsec phase2

- `fortinet-cli ipsec-phase2 get`  -  Get one ipsec-phase2 entry by name.
- `fortinet-cli ipsec-phase2 list`  -  IPsec phase 2 selectors (vpn ipsec phase2-interface).

**ldap_servers**  -  Manage ldap servers

- `fortinet-cli ldap-servers get`  -  Get one ldap-servers entry by name.
- `fortinet-cli ldap-servers list`  -  LDAP authentication servers (user ldap).

**local_in_policies**  -  Manage local in policies

- `fortinet-cli local-in-policies get`  -  Get one local-in-policies entry by policyid.
- `fortinet-cli local-in-policies list`  -  Local-in policies (firewall local-in-policy) that restrict traffic to the FortiGate itself.

**local_users**  -  Manage local users

- `fortinet-cli local-users get`  -  Get one local-users entry by name.
- `fortinet-cli local-users list`  -  Local firewall users (user local), including two-factor state.

**monitor**  -  Manage monitor

- `fortinet-cli monitor admin-sessions`  -  Administrators currently logged in.
- `fortinet-cli monitor aps`  -  FortiAPs managed by this FortiGate.
- `fortinet-cli monitor auth-users`  -  Users currently authenticated through firewall policies.
- `fortinet-cli monitor central-management`  -  Connection status to FortiManager / central management.
- `fortinet-cli monitor certificates`  -  Local, CA and remote certificates with validity window.
- `fortinet-cli monitor config-revisions`  -  Saved configuration revisions.
- `fortinet-cli monitor dhcp-leases`  -  Active DHCP leases.
- `fortinet-cli monitor firmware-images`  -  Running firmware image and the images FortiGuard offers for upgrade.
- `fortinet-cli monitor fortiguard`  -  FortiGuard server list and reachability.
- `fortinet-cli monitor fortitokens`  -  FortiToken inventory and activation state.
- `fortinet-cli monitor ha-checksums`  -  Per-member configuration checksums; mismatches mean HA is out of sync.
- `fortinet-cli monitor ha-peers`  -  HA cluster members and which unit is primary.
- `fortinet-cli monitor interfaces`  -  Live interface link, speed, counters.
- `fortinet-cli monitor ipsec`  -  IPsec tunnels with phase-2 selector status.
- `fortinet-cli monitor license-status`  -  Licence and FortiGuard/FortiCare entitlement status with expiry epochs.
- `fortinet-cli monitor log-disk`  -  Local log disk usage.
- `fortinet-cli monitor ntp`  -  NTP server reachability.
- `fortinet-cli monitor performance`  -  Performance snapshot (CPU, memory, sessions, uptime).
- `fortinet-cli monitor policy-stats`  -  Per-policy hit counters, bytes and last-used timestamps (monitor firewall policy).
- `fortinet-cli monitor resources`  -  CPU, memory, session and disk usage.
- `fortinet-cli monitor routes`  -  IPv4 routing table (all route types).
- `fortinet-cli monitor sdwan-health`  -  SD-WAN performance SLA health-check results per member.
- `fortinet-cli monitor sdwan-members`  -  SD-WAN member interface state.
- `fortinet-cli monitor sslvpn-sessions`  -  Connected SSL-VPN users and their tunnel/web sub-sessions.
- `fortinet-cli monitor sslvpn-stats`  -  SSL-VPN usage statistics.
- `fortinet-cli monitor status`  -  System status: model, hostname; serial, FortiOS version and build are in the envelope.
- `fortinet-cli monitor switches`  -  FortiSwitches managed by this FortiGate.
- `fortinet-cli monitor upgrade-paths`  -  Supported firmware upgrade paths for this model.

**policies**  -  Manage policies

- `fortinet-cli policies get`  -  Get one policies entry by policyid.
- `fortinet-cli policies list`  -  IPv4/IPv6 firewall policies (firewall policy) in sequence order.

**radius_servers**  -  Manage radius servers

- `fortinet-cli radius-servers get`  -  Get one radius-servers entry by name.
- `fortinet-cli radius-servers list`  -  RADIUS authentication servers (user radius).

**service_groups**  -  Manage service groups

- `fortinet-cli service-groups get`  -  Get one service-groups entry by name.
- `fortinet-cli service-groups list`  -  Firewall service groups (firewall service group).

**services**  -  Manage services

- `fortinet-cli services get`  -  Get one services entry by name.
- `fortinet-cli services list`  -  Custom firewall services (firewall service custom).

**settings**  -  Manage settings

- `fortinet-cli settings autoupdate`  -  FortiGuard definitions auto-update schedule (system autoupdate schedule).
- `fortinet-cli settings central-management`  -  Central management settings (system central-management): FortiManager / FortiGate Cloud link.
- `fortinet-cli settings dns`  -  DNS resolver settings (system dns).
- `fortinet-cli settings fortianalyzer`  -  FortiAnalyzer log forwarding (log fortianalyzer setting).
- `fortinet-cli settings fortiguard`  -  FortiGuard update and rating settings (system fortiguard), including automatic firmware upgrade.
- `fortinet-cli settings global`  -  Global system settings (system global): hostname, admin ports, admin lockout and timeout.
- `fortinet-cli settings ha`  -  High-availability configuration (system ha).
- `fortinet-cli settings ntp`  -  NTP settings (system ntp).
- `fortinet-cli settings password-policy`  -  Administrator password policy (system password-policy).
- `fortinet-cli settings syslog`  -  Syslog forwarding (log syslogd setting).
- `fortinet-cli settings vpn-ssl`  -  SSL-VPN settings (vpn ssl settings): status, listening port, source interfaces and source addresses.

**sslvpn_portals**  -  Manage sslvpn portals

- `fortinet-cli sslvpn-portals get`  -  Get one sslvpn-portals entry by name.
- `fortinet-cli sslvpn-portals list`  -  SSL-VPN web portals (vpn ssl web portal).

**static_routes**  -  Manage static routes

- `fortinet-cli static-routes get`  -  Get one static-routes entry by seq-num.
- `fortinet-cli static-routes list`  -  Static routes (router static).

**user_groups**  -  Manage user groups

- `fortinet-cli user-groups get`  -  Get one user-groups entry by name.
- `fortinet-cli user-groups list`  -  User groups (user group) used in policies and SSL-VPN authentication rules.

**vdoms**  -  Manage vdoms

- `fortinet-cli vdoms get`  -  Get one vdoms entry by name.
- `fortinet-cli vdoms list`  -  Virtual domains configured on the FortiGate (system vdom).

**vips**  -  Manage vips

- `fortinet-cli vips get`  -  Get one vips entry by name.
- `fortinet-cli vips list`  -  Virtual IPs / port forwards (firewall vip): which internal hosts are published to the internet.


**audit**


- `fortinet-cli audit firmware`  -  Flags FortiGates below your per-branch firmware floor and how many patches each is behind the newest image offered in its branch.

- `fortinet-cli audit sslvpn`  -  Grades each FortiGate and VDOM on whether SSL-VPN is enabled, listening on a WAN interface, and open to any source address.

- `fortinet-cli audit admins`  -  Flags admins with no trusted hosts (FAIL when HTTPS/SSH management is open on WAN), super_admin accounts without two-factor, WAN interfaces allowing HTTP or Telnet, and API users trusted from broad networks or using super_admin.

- `fortinet-cli audit licenses`  -  One renewal queue of FortiGuard, FortiCare and service licences across the fleet, sorted by days left.


**fleet**


- `fortinet-cli fleet audit`  -  One PASS/WARN/FAIL/UNKNOWN matrix across every FortiGate in your devices file for firmware, SSL-VPN exposure, admin accounts and FortiGuard licence expiry.

- `fortinet-cli fleet health`  -  One PASS/WARN/FAIL/UNKNOWN row per FortiGate: HA config sync, IPsec tunnels down, SD-WAN members down, CPU and memory pressure, expiring certificates; anything it could not read makes the row UNKNOWN.

- `fortinet-cli fleet changes`  -  Shows which FortiOS objects were added, removed or modified between two saved 'fleet backup' files (needs at least two backups per device; secrets are redacted, so a secret-only change is not reported).


**policies**


- `fortinet-cli policies hygiene`  -  Lists unused, any-any accept, logging-disabled and disabled firewall policies per VDOM.


**Inventory, backups and many-client views**

- `fortinet-cli fleet devices`  -  Lists the devices the fleet commands will query (devices file, or the single `FORTINET_BASE_URL` device) and whether each is ready. Run it first when a fleet command reports UNKNOWN.
- `fortinet-cli fleet backup`  -  Downloads each FortiGate's full configuration into dated 0600 files under `<data dir>/backups` (or `--out-dir`; pass the same directory to `fleet changes --dir`). Backups contain encrypted secrets. Exits 5 if any backup fails.
- `fortinet-cli fmg status` / `fmg adoms` / `fmg devices [--adom a,b]`  -  Read-only FortiManager view. Needs `FORTINET_FMG_URL` and `FORTINET_FMG_API_KEY`; check with `fmg status`.
- `fortinet-cli cloud devices` / `cloud device <serial>`  -  Read-only FortiGate Cloud inventory (serial, model, firmware, contract end, management tunnel). Needs `FORTINET_CLOUD_API_USER` and `FORTINET_CLOUD_API_PASSWORD` (+ `FORTINET_CLOUD_REGION`, `FORTINET_CLOUD_ACCOUNT_ID` when applicable).

Prerequisites: without a devices file (`<config dir>/devices.toml` or `$FORTINET_DEVICES`), `fleet *`, `audit *` and `policies hygiene` check only the `FORTINET_BASE_URL` device. `doctor` checks only that device too.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
fortinet-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index (the headline commands above; for `fleet devices`, `fleet backup`, `fmg *` and `cloud *` use the Command Reference). Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Weekly posture diff

```bash
fortinet-cli fleet audit --new --agent
```

Shows only findings that were not present in the previous run; add --fail-on fail to exit 6 on any FAIL or UNKNOWN.

### FortiManager firmware and sync view

```bash
fortinet-cli fmg devices --agent --select devices.adom,devices.name,devices.firmware,devices.conn_status,devices.conf_status
```

Lists every managed FortiGate per client ADOM with only the firmware and sync fields an agent needs.

### SSL-VPN exposure across clients

```bash
fortinet-cli audit sslvpn --agent
```

Grades every device and VDOM on SSL-VPN status, WAN listening and source restriction.

### Dead and risky policies

```bash
fortinet-cli policies hygiene --unused-days 90 --agent
```

Joins policy config with hit counters to list unused, any-any and logging-disabled rules.

### Back up every FortiGate, then see what changed

```bash
fortinet-cli fleet backup
```

Writes dated 0600 config files under <data dir>/backups (they contain encrypted secrets); after the next scheduled backup, fleet changes --since 7d lists added, removed and modified objects.

## Auth Setup

Each FortiGate uses its own REST API administrator token (System > Administrators > Create New > REST API Admin, or `execute api-user generate-key <name>`), sent as `Authorization: Bearer`; your source IP must be one of that API user's trusted hosts. Single device: set FORTINET_BASE_URL=https://<fortigate>/api/v2 and FORTINET_API_TOKEN. Fleet: list devices in <config dir>/devices.toml (or the file named by FORTINET_DEVICES or --devices), each with token_file naming a file that holds that device's token (on macOS and Linux the CLI refuses it unless chmod 600; on Windows protect it with NTFS permissions); token values never go in the devices file. Without a devices file, fleet and audit commands check only the single FORTINET_BASE_URL device. Self-signed certificates: install the device CA, or opt in with --insecure, FORTINET_SKIP_TLS_VERIFY=true, or insecure = true for one device in devices.toml. FortiManager: FORTINET_FMG_URL plus an API key in FORTINET_FMG_API_KEY (FortiManager 7.2.2 or later; FORTINET_FMG_INSECURE=true for a self-signed certificate). FortiGate Cloud: a FortiCloud IAM API user in FORTINET_CLOUD_API_USER and FORTINET_CLOUD_API_PASSWORD, FORTINET_CLOUD_REGION (global, us, eu or jp) and, with FortiCloud Organizations, FORTINET_CLOUD_ACCOUNT_ID; repeated failed logins lock the FortiCloud account for 15-30 minutes, so the CLI caches the token and logs in at most once per run.

Run `fortinet-cli doctor` to verify setup.

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
  fortinet-cli address-groups list --agent --select category,color,comment
  ```
- **Previewable**  -  `--dry-run` shows the request without sending (fleet, audit, policies hygiene, fmg and cloud commands report the action they would run)
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  never creates, updates or deletes anything on FortiGate, FortiManager or FortiGate Cloud; do not use it for configuration changes or firmware upgrades

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

- Use `--home <dir>` for one invocation, or set `FORTINET_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `FORTINET_CONFIG_DIR`, `FORTINET_DATA_DIR`, `FORTINET_STATE_DIR`, `FORTINET_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `FORTINET_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `fortinet-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "fortinet": {
        "command": "fortinet-mcp",
        "env": {
          "FORTINET_HOME": "/srv/fortinet"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `FORTINET_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `FORTINET_HOME`, or `doctor` will not find credentials left under the former root.

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
fortinet-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "fortinet-cli learnings confirm 12"] }
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
       materially more, record the divergence via `fortinet-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `fortinet-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `fortinet-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
fortinet-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
fortinet-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
fortinet-cli teach-playbook \
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
fortinet-cli playbook amend \
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

`fortinet-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `FORTINET_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
fortinet-cli feedback "the --since flag is inclusive but docs say exclusive"
fortinet-cli feedback --stdin < notes.txt
fortinet-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `FORTINET_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `FORTINET_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
fortinet-cli profile save briefing --json
fortinet-cli --profile briefing address-groups list
fortinet-cli profile list --json
fortinet-cli profile show briefing
fortinet-cli profile delete briefing --yes
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
| 5 | API error (upstream issue; also a failed `fleet backup`) |
| 6 | Findings at or above `--fail-on` (only `fleet audit` and `audit *`, only when `--fail-on` is passed; UNKNOWN counts) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `fortinet-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add fortinet-mcp --env FORTINET_BASE_URL=https://<fortigate>/api/v2 --env FORTINET_API_TOKEN=<token> -- fortinet-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which fortinet-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   fortinet-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `fortinet-cli <command> --help`.
