---
name: eset-protect
description: "Use when the user asks about ESET PROTECT across every customer: which customers have devices that need attention, are muted or stopped checking in, the morning detection and incident queue, which customers and devices have a CVE, missing security patches, how far a file hash spread, one customer's QBR summary, or scanning, isolating or releasing a device by name. Covers every ESET Connect endpoint (101 operations) as commands, with a local SQLite mirror for per-customer rollups. Trigger phrases: `which eset customers need attention`, `eset protect stale devices`, `are we affected by this CVE in eset`, `eset detections this morning`, `eset qbr report for a customer`, `isolate this machine in eset`, `use eset-protect`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ESET"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - eset-protect-cli
    install:
      - kind: script
        bins: [eset-protect-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1
---

# ESET PROTECT CLI

## Prerequisites: Install the CLI

This skill drives the `eset-protect-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex
   ```
3. Verify: `eset-protect-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `eset-protect-cli` and `eset-protect-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Covers devices, detections, incidents, EDR rules, automation, vulnerabilities, patches, policies, quarantine, installers and IAM, with the password-grant token minted and refreshed for you (the jpn region has no quarantine, user-management or v2 detection list). Sync before you ask: 'fleet health', 'fleet gaps' and 'triage' answer from the local mirror (re-sync detections and incidents before each triage run, and run 'sync --full' now and then so devices and groups deleted in ESET leave the mirror), while 'exposure', 'patches backlog' and 'report customer' scan ESET live and use the mirror to name each customer.

## When to Use This CLI

Use this CLI for ESET PROTECT cloud (ESET Connect) work: listing and changing devices, detections, incidents, EDR rules, automation tasks, policies, quarantine and installers, and for per-customer MSP questions such as which customer has stale or unprotected devices, who is exposed to a CVE, what patches are pending, and a customer's QBR numbers. Every command speaks JSON. 'respond' and 'policies clone' preview unless --yes; every other write executes immediately, so run it with --dry-run first.

## Anti-triggers

Do not use this CLI for:
- ESET PROTECT On-Prem consoles (POST /GetTokens on port 9443): this CLI targets the ESET Connect cloud API only
- ESET licensing, seat counts or usage billing: those live in the ESET MSP Administrator 2 / PROTECT Hub API, not ESET Connect
- Editing the settings inside an existing policy: ESET Connect has no policy update call; decode it with 'policies decode' and create a new policy instead
- Real-time detection streaming into a SIEM: use ESET's Elastic, Sentinel or Splunk integrations
- Quarantine, Cloud Office users and the v2 detection list in the jpn region: ESET Connect does not offer them there

## Unique Capabilities

### Per-customer rollups
- **`fleet health`**  -  One row per customer with device count, attention-required and attention-recommended devices, problem count, muted, stale and unenrolled devices, worst customer first.

  _Reach for this to answer 'which customer needs attention today' across the whole MSP tree in one call._

  ```bash
  eset-protect-cli fleet health --stale-days 7 --agent
  ```
- **`report customer`**  -  One customer's QBR one-pager: current device health, detections by severity and response, incidents with triage and response times, top CVEs, pending and failed security patches, and vulnerability-scan coverage (ESET keeps only a few days of scan and patching history).

  _Use it to build a customer QBR or answer 'how is this customer doing' without five separate queries._

  ```bash
  eset-protect-cli report customer "Acme Dental" --since 90d --agent
  ```
- **`fleet gaps`**  -  Devices missing a component or product their customer's other devices have, running an older component version, or not finished enrolling.

  _Use it in the weekly agent-health sweep to find unprotected or out-of-date machines per customer._

  ```bash
  eset-protect-cli fleet gaps --only missing,outdated --agent
  ```

### Vulnerability and patch intelligence
- **`exposure`**  -  Which customers and devices have a given CVE or vulnerable application, with risk score, first detection and whether a patch is available.

  _Use it the moment a CVE makes the news and a customer asks 'are we affected?'._

  ```bash
  eset-protect-cli exposure --cve CVE-2024-3400 --agent
  ```
- **`patches backlog`**  -  Available-but-not-installed patches ranked by the devices and CVE risk they fix per customer, plus recent failed patch attempts by reason.

  _Use it after Patch Tuesday to decide what to push first and to see what failed._

  ```bash
  eset-protect-cli patches backlog --security-only --since 30d --agent
  ```

### Triage and response
- **`detections spread`**  -  How far each detected file hash has spread across devices and customers in a window, joined to whether that executable is blocked, its signer and its LiveGrid reputation when executables are synced.

  _Use it to decide false positive versus outbreak before resolving or writing an exclusion._

  ```bash
  eset-protect-cli detections spread --since 7d --min-customers 2
  ```
- **`respond`**  -  Scan, isolate or release a device by name (or scan a whole customer): prints the exact ESET task first and creates it only with --yes; with --wait it polls the task runs until they finish. Over MCP the root --yes is never forwarded, so the tool only previews.

  _Use it to act on a finding by device name without hand-building an automation task payload._

  ```bash
  eset-protect-cli respond scan FRONTDESK-01 --yes --wait 10m
  ```

### Everyday ESET workflows
- **`triage`**  -  Endpoint detections in a window (v1 has no resolved state) and open incidents in one ranked queue, worst first, with device, customer and a suggested next command.

  _Use it for the morning queue instead of paging detections and incidents separately._

  ```bash
  eset-protect-cli triage --since 24h --agent
  ```
- **`fleet stale`**  -  Devices that stopped checking in for N days, most stale first, with customer and group path.

  _Use it to find offboarding candidates or broken agents per customer._

  ```bash
  eset-protect-cli fleet stale --days 14 --agent
  ```
- **`detections latest`**  -  The newest endpoint detections straight from ESET for a time window and minimum severity, with device and customer names when the mirror is synced.

  _Use it when you need what fired in the last hours without a sync._

  ```bash
  eset-protect-cli detections latest --since 6h --min-severity high --agent
  ```
- **`devices profile`**  -  Everything about one device in one call: health, recent detections, CVEs, pending patches and recent scans, with partial-failure reporting.

  _Use it before touching a device or answering a ticket about it._

  ```bash
  eset-protect-cli devices profile FRONTDESK-01 --agent
  ```
- **`incidents context`**  -  One incident with its comments and every linked detection and device, falling back from v1 to v2 detection lookups.

  _Use it to work one incident end to end._

  ```bash
  eset-protect-cli incidents context 8d4f2a17-3b6c-4e1d-9a2f-5c7e1b0d4f6a --agent
  ```
- **`report vulnerabilities`**  -  Vulnerable devices, CVE counts by severity, top CVEs and patchable share per customer from a live vulnerability scan.

  _Use it for a monthly vulnerability review across customers._

  ```bash
  eset-protect-cli report vulnerabilities --top 10 --agent
  ```
- **`executables find`**  -  Turn an MD5, SHA-1 or SHA-256 hash or a name into the executable UUID that 'executables block' needs, with blocked state and signer.

  _Use it before blocking or excluding a file._

  ```bash
  eset-protect-cli executables find --hash 3395856ce81f2b7382dee72602f798b642f14140 --agent
  ```
- **`policies decode`**  -  Decode a policy's base64 and LZMA settings blobs into readable JSON, list settings that mention a word, or pull one value by path.

  _Use it to see what a policy actually sets before cloning or assigning it._

  ```bash
  eset-protect-cli policies decode 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --find firewall
  ```
- **`policies clone`**  -  Copy an existing policy's feature settings byte for byte into a new policy under a new name; preview unless --yes.

  _Use it to start a customer-specific policy from a known-good one._

  ```bash
  eset-protect-cli policies clone 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --name "Acme Dental - Workstations"
  ```

## Command Reference

**application-patching-processes**  -  Manage application patching processes

- `eset-protect-cli application-patching-processes`  -  List the patching details of all application patching attempts performed recently (within the last few days).

**detection-groups**  -  Manage detection groups

- `eset-protect-cli detection-groups get`  -  Get details about a detection group.
- `eset-protect-cli detection-groups list`  -  Return a list of all the [detection group]s matching criteria.
- `eset-protect-cli detection-groups resolve`  -  Mark all the [detections] in a group as resolved.

**detection-groups-search**  -  Manage detection groups search

- `eset-protect-cli detection-groups-search`  -  Search a collection for [detection groups]s matching criteria defined by the filter (if given).

**detections**  -  Manage detections

- `eset-protect-cli detections get`  -  Endpoint detection from ESET PROTECT and ESET Inspect (v1).
- `eset-protect-cli detections list`  -  Endpoint detections from ESET PROTECT and ESET Inspect (v1). Return a list of all the [detection]s matching criteria.

**detections-v2**  -  Manage detections v2

- `eset-protect-cli detections-v2 batch-get`  -  Batch-get detections from the v2 API (ESET Inspect and ESET Cloud Office Security). Batch-get detections.
- `eset-protect-cli detections-v2 get`  -  Detection by UUID from the v2 API (ESET Inspect and ESET Cloud Office Security). Get detection.
- `eset-protect-cli detections-v2 list`  -  ESET Cloud Office Security detections only (v2).
- `eset-protect-cli detections-v2 resolve`  -  Resolve an ESET Inspect detection (v2). Mark a [detection] as resolved.

**device-groups**  -  Manage device groups

- `eset-protect-cli device-groups`  -  Hierarchy can be reconstructed from the returned `device_groups`.

**device-os-vulnerabilities**  -  Manage device os vulnerabilities

- `eset-protect-cli device-os-vulnerabilities`  -  List vulnerabilities of the operating system of the given device.

**device-patches**  -  Manage device patches

- `eset-protect-cli device-patches`  -  If the same application, package or operating system is unpatched on multiple devices

**device-tasks**  -  Manage device tasks

- `eset-protect-cli device-tasks create`  -  Create a [device task].
- `eset-protect-cli device-tasks delete`  -  Delete the specified task. Deletion of the template leads to deletion of all templated tasks.
- `eset-protect-cli device-tasks get`  -  Get the task entity. For a complete history, call `ListTaskRuns()`.
- `eset-protect-cli device-tasks list`  -  List all device tasks matching criteria.
- `eset-protect-cli device-tasks update-targets`  -  Update task targets.
- `eset-protect-cli device-tasks update-triggers`  -  Update the list of task triggers.

**device-vulnerabilities**  -  Manage device vulnerabilities

- `eset-protect-cli device-vulnerabilities`  -  List device vulnerabilities where the vulnerabilities fit the criteria from the request.

**devices**  -  Manage devices

- `eset-protect-cli devices get`  -  Get details of a [device].
- `eset-protect-cli devices list`  -  List all the managed [device]s.
- `eset-protect-cli devices move`  -  Move [Device] under new [Group]. It is possible to move Groups only in the scope of the same tenant.
- `eset-protect-cli devices rename`  -  Update the device's display name.

**devices-batch-get**  -  Manage devices batch get

- `eset-protect-cli devices-batch-get`  -  Retrieves a specific set of devices at a consistent time point.

**devices-batch-import**  -  Manage devices batch import

- `eset-protect-cli devices-batch-import`  -  Import a batch of devices into device management. Imported devices cannot be managed until properly enrolled.

**edr-rule-exclusions**  -  Manage edr rule exclusions

- `eset-protect-cli edr-rule-exclusions create`  -  Create an [EDR rule exclusion].
- `eset-protect-cli edr-rule-exclusions delete`  -  Delete an [EDR rule exclusion].
- `eset-protect-cli edr-rule-exclusions get`  -  Get details of an [EDR rule exclusion].
- `eset-protect-cli edr-rule-exclusions list`  -  List [EDR rule exclusion]s matching criteria.
- `eset-protect-cli edr-rule-exclusions update-definition`  -  Update the definition of an [EDR rule exclusion].

**edr-rules**  -  Manage edr rules

- `eset-protect-cli edr-rules create`  -  Create an [EDR rule]. Only the rule with valid `xml_definition` can be accepted.
- `eset-protect-cli edr-rules delete`  -  Delete the given [EDR rule].
- `eset-protect-cli edr-rules disable`  -  Disable the given [EDR rule].
- `eset-protect-cli edr-rules enable`  -  Enable the given [EDR rule].
- `eset-protect-cli edr-rules get`  -  Get [EDR rule] details.
- `eset-protect-cli edr-rules list`  -  List [EDR rule]s matching the criteria.
- `eset-protect-cli edr-rules update-definition`  -  Update the [EDR rule] definition. If the rule_definition is invalid, an error 400 BAD REQUEST is returned.

**executables**  -  Manage executables

- `eset-protect-cli executables block`  -  Blocked executables will not be executed. Idempotent
- `eset-protect-cli executables get`  -  Get details of a specific executable.
- `eset-protect-cli executables list`  -  List all the executables.
- `eset-protect-cli executables unblock`  -  Unblock a file to be executable again. Idempotent

**gpo-sccm-files-generate**  -  Manage gpo sccm files generate

- `eset-protect-cli gpo-sccm-files-generate`  -  Generates a GPO/SCCM-compatible configuration file for deploying the security product via centralized IT management

**groups**  -  Manage groups

- `eset-protect-cli groups create`  -  Create a [Group] in the hierarchy and assign a UUID to it.
- `eset-protect-cli groups delete`  -  Delete the group referenced by the uuid and any groups under it.
- `eset-protect-cli groups move`  -  Move a [Group] under another [Group]. It is possible to move groups only in the scope of the same tenant.
- `eset-protect-cli groups rename`  -  Update the group's display name.

**incidents**  -  Manage incidents

- `eset-protect-cli incidents close`  -  Close an incident.
- `eset-protect-cli incidents get`  -  Get details of a specific incident.
- `eset-protect-cli incidents list`  -  Returns a list of incidents matching the `filter`. The list is ordered according to `order_by`.
- `eset-protect-cli incidents reopen`  -  Reopening an incident causes the change of the status to 'Open'.

**installers**  -  Manage installers

- `eset-protect-cli installers create`  -  Create [installer].
- `eset-protect-cli installers delete`  -  Delete [installer].
- `eset-protect-cli installers get`  -  Get details of the requested [installer].
- `eset-protect-cli installers list`  -  List all installers available to the caller.

**ip-sets**  -  Manage ip sets

- `eset-protect-cli ip-sets get`  -  Get details about an IP set.
- `eset-protect-cli ip-sets list`  -  List all IP sets.
- `eset-protect-cli ip-sets update`  -  Update the selected IP set; attributes that can be updated

**mobile-devices-batch-activate-product**  -  Manage mobile devices batch activate product

- `eset-protect-cli mobile-devices-batch-activate-product`  -  Create a product activation task with mobile devices as its targets.

**mobile-devices-batch-get-enrollment-links**  -  Manage mobile devices batch get enrollment links

- `eset-protect-cli mobile-devices-batch-get-enrollment-links`  -  Get enrollment links for a batch of **mobile** devices in device management.

**patching-process-details**  -  Manage patching process details

- `eset-protect-cli patching-process-details`  -  History is pruned repeatedly to prevent database flooding.

**permissions**  -  Manage permissions

- `eset-protect-cli permissions`  -  List all possible permissions.

**policies**  -  Manage policies

- `eset-protect-cli policies create`  -  Create a policy. All the included feature policies must be valid.
- `eset-protect-cli policies delete`  -  Delete the given policy.
- `eset-protect-cli policies get`  -  Get policy details.
- `eset-protect-cli policies list`  -  List all the policies accessible to the caller (based on the token).

**policy-assignments**  -  Manage policy assignments

- `eset-protect-cli policy-assignments assign-policy`  -  Create a policy assignment. Assignments to the same targets are ordered and prioritized.
- `eset-protect-cli policy-assignments get`  -  Get details of the given policy assignment.
- `eset-protect-cli policy-assignments list`  -  List all the policy assignments available to the caller.
- `eset-protect-cli policy-assignments unassign-policy`  -  Delete the policy assignment. The remaining assignments to the target are reordered.
- `eset-protect-cli policy-assignments update-ranking`  -  Move the policy assignment within ranks.

**quarantined-objects**  -  Manage quarantined objects

- `eset-protect-cli quarantined-objects get`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)
- `eset-protect-cli quarantined-objects get-count`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)
- `eset-protect-cli quarantined-objects list`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**quarantined-objects-batch-delete**  -  Manage quarantined objects batch delete

- `eset-protect-cli quarantined-objects-batch-delete`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**quarantined-objects-batch-download**  -  Manage quarantined objects batch download

- `eset-protect-cli quarantined-objects-batch-download`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**quarantined-objects-batch-restore**  -  Manage quarantined objects batch restore

- `eset-protect-cli quarantined-objects-batch-restore`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**quarantined-objects-download**  -  Manage quarantined objects download

- `eset-protect-cli quarantined-objects-download`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**quarantined-objects-purge**  -  Manage quarantined objects purge

- `eset-protect-cli quarantined-objects-purge`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**quarantined-objects-restore**  -  Manage quarantined objects restore

- `eset-protect-cli quarantined-objects-restore`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**role-assignments**  -  Manage role assignments

- `eset-protect-cli role-assignments`  -  List [role]s of [subject]s. > **Info:** > Not only Users are [subject]s, but anything that can have roles attached.

**role-assignments-assign-role**  -  Manage role assignments assign role

- `eset-protect-cli role-assignments-assign-role`  -  Assign a role to a subject. Eventually, the subject ends with its existing roles and those assigned by this operation.

**role-assignments-revoke-role**  -  Manage role assignments revoke role

- `eset-protect-cli role-assignments-revoke-role`  -  Remove the assigned role from the subject.

**roles**  -  Manage roles

- `eset-protect-cli roles create`  -  Creates a new custom [Role]. The role name must be unique within the scope in which the role is created.
- `eset-protect-cli roles delete`  -  Delete specified role.

**scans**  -  Manage scans

- `eset-protect-cli scans`  -  List the scan details of all vulnerability scan attempts performed recently (within the last few days).

**users**  -  Manage users

- `eset-protect-cli users get`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)
- `eset-protect-cli users list`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**users-batch-get-users**  -  Manage users batch get users

- `eset-protect-cli users-batch-get-users`  -  Requires ESET Cloud Office Security access (the API user must sign in to that console once)

**vulnerable-devices**  -  Manage vulnerable devices

- `eset-protect-cli vulnerable-devices`  -  List vulnerable devices matching the criteria from the request.

**web-address-rules**  -  Manage web address rules

- `eset-protect-cli web-address-rules <policyUuid>`  -  List all address rules containing given domain


**detections**


- `eset-protect-cli detections spread`  -  How far each detected file hash has spread across devices and customers in a window, joined to whether that executable is blocked, its signer and its LiveGrid reputation when executables are synced.

- `eset-protect-cli detections latest`  -  The newest endpoint detections straight from ESET for a time window and minimum severity, with device and customer names when the mirror is synced.


**devices**


- `eset-protect-cli devices profile`  -  Everything about one device in one call: health, recent detections, CVEs, pending patches and recent scans, with partial-failure reporting.


**executables**


- `eset-protect-cli executables find`  -  Turn an MD5, SHA-1 or SHA-256 hash or a name into the executable UUID that 'executables block' needs, with blocked state and signer.


**exposure**


- `eset-protect-cli exposure`  -  Which customers and devices have a given CVE or vulnerable application, with risk score, first detection and whether a patch is available.


**fleet**


- `eset-protect-cli fleet health`  -  One row per customer with device count, attention-required and attention-recommended devices, problem count, muted, stale and unenrolled devices, worst customer first.

- `eset-protect-cli fleet gaps`  -  Devices missing a component or product their customer's other devices have, running an older component version, or not finished enrolling.

- `eset-protect-cli fleet stale`  -  Devices that stopped checking in for N days, most stale first, with customer and group path.


**incidents**


- `eset-protect-cli incidents context`  -  One incident with its comments and every linked detection and device, falling back from v1 to v2 detection lookups.


**patches**


- `eset-protect-cli patches backlog`  -  Available-but-not-installed patches ranked by the devices and CVE risk they fix per customer, plus recent failed patch attempts by reason.


**policies**


- `eset-protect-cli policies decode`  -  Decode a policy's base64 and LZMA settings blobs into readable JSON, list settings that mention a word, or pull one value by path.

- `eset-protect-cli policies clone`  -  Copy an existing policy's feature settings byte for byte into a new policy under a new name; preview unless --yes.


**report**


- `eset-protect-cli report customer`  -  One customer's QBR one-pager: current device health, detections by severity and response, incidents with triage and response times, top CVEs, pending and failed security patches, and vulnerability-scan coverage (ESET keeps only a few days of scan and patching history).

- `eset-protect-cli report vulnerabilities`  -  Vulnerable devices, CVE counts by severity, top CVEs and patchable share per customer from a live vulnerability scan.


**respond**


- `eset-protect-cli respond`  -  Scan, isolate or release a device by name (or scan a whole customer): prints the exact ESET task first and creates it only with --yes; with --wait it polls the task runs until they finish. Over MCP the root --yes is never forwarded, so the tool only previews.


**triage**


- `eset-protect-cli triage`  -  Endpoint detections in a window (v1 has no resolved state) and open incidents in one ranked queue, worst first, with device, customer and a suggested next command.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
eset-protect-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning triage

```bash
eset-protect-cli triage --since 24h --agent
```

Endpoint detections from the last day (v1 has no resolved state) and open incidents in one ranked queue with device and customer names.

### Are we affected by a CVE

```bash
eset-protect-cli exposure --cve CVE-2024-3400 --agent --select customers.customer,customers.affected_devices,customers.patch_available,truncated,scanned_vulnerabilities,note
```

Per-customer exposure; truncated, scanned_vulnerabilities and note show whether the live scan was complete before you tell a customer they are clear.

### Outbreak or false positive

```bash
eset-protect-cli detections spread --since 7d --min-customers 2
```

Hashes seen at two or more customers this week (devices the mirror cannot place count as possible customers, and thresholds_proven is false when only they reach the threshold); blocked state and signer appear when executables are synced.

### Customer QBR

```bash
eset-protect-cli report customer "Acme Dental" --since 90d --agent
```

One customer's health, detections, incidents, CVEs and patches; failed live sections are null and listed in fetch_failures.

### Isolate a machine

```bash
eset-protect-cli respond isolate FRONTDESK-01
```

Prints the exact StartNetworkIsolation task for the named device; nothing is sent until you add --yes, and 'respond release' ends the isolation.

## Auth Setup

ESET Connect has no API keys. Create a dedicated API user: a Superuser or Root adds it in ESET PROTECT Hub (Users), ESET Business Account or ESET MSP Administrator 2 (User management, Access Rights) and turns on the Integrations toggle. The new user accepts the invitation email and finishes account setup, and a Hub user must also sign in to ESET PROTECT Hub once. Then set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD (`export VAR=value` on macOS/Linux, `$env:VAR = "value"` in PowerShell) and set ESET_PROTECT_REGION to the tenant's data center (eu, de, us, ca or jpn; default eu). The CLI exchanges them for a one-hour bearer token with the OAuth2 password grant, caches it for that region and user, and refreshes it automatically. ESET_PROTECT_TOKEN overrides with a token you minted yourself.

Run `eset-protect-cli doctor` to verify setup.

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
  eset-protect-cli detection-groups list --agent --select category,circumstances,cloudOfficeTenantUuid
  ```
- **Previewable**  -  `--dry-run` shows the request without sending (the API request only: `--deliver webhook:<url>` still posts the output, `feedback --send` still sends, and `import --dry-run` may still request an access token)
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

- Use `--home <dir>` for one invocation, or set `ESET_PROTECT_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ESET_PROTECT_CONFIG_DIR`, `ESET_PROTECT_DATA_DIR`, `ESET_PROTECT_STATE_DIR`, `ESET_PROTECT_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ESET_PROTECT_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml` (the cached access and refresh tokens), the synced mirror `data-<hash>.db` (the hash is scoped to region and API user; `doctor` prints the exact path), cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `eset-protect-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "eset-protect": {
        "command": "eset-protect-mcp",
        "env": {
          "ESET_PROTECT_HOME": "/srv/eset-protect"
        }
      }
    }
  }
  ```

  On Windows, use a Windows path with escaped backslashes, for example `"ESET_PROTECT_HOME": "C:\\Users\\<you>\\AppData\\Local\\eset-protect"` (a folder under your own profile, so other local users cannot read the mirror or the cached tokens).

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ESET_PROTECT_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ESET_PROTECT_HOME`, or `doctor` will not find credentials left under the former root.

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
eset-protect-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "eset-protect-cli learnings confirm 12"] }
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
       materially more, record the divergence via `eset-protect-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `eset-protect-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `eset-protect-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
eset-protect-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
eset-protect-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
eset-protect-cli teach-playbook \
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
eset-protect-cli playbook amend \
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

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the CLI generator. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`eset-protect-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ESET_PROTECT_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
eset-protect-cli feedback "the --since flag is inclusive but docs say exclusive"
eset-protect-cli feedback --stdin < notes.txt
eset-protect-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ESET_PROTECT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ESET_PROTECT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). This happens even under `--dry-run`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
eset-protect-cli profile save briefing --json
eset-protect-cli --profile briefing detection-groups list
eset-protect-cli profile list --json
eset-protect-cli profile show briefing
eset-protect-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `eset-protect-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add eset-protect-mcp -- eset-protect-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which eset-protect-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   eset-protect-cli fleet health --agent
   ```
4. If ambiguous, drill into subcommand help, e.g. `eset-protect-cli respond isolate --help`.
