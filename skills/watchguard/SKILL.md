---
name: watchguard
description: "Use when the user asks which WatchGuard Cloud customer needs attention first, which Fireboxes are offline or behind on firmware, which endpoints are unprotected, isolated or waiting on a reboot across customers, which ThreatSync incidents are untouched, who is exposed to a CVE, whether license allocations match the Fireboxes that report in, or to wait for a Firebox deployment to finish; also to run any WatchGuard Cloud API call (Firebox, FireCloud, Endpoint Security, ThreatSync, NDR, AuthPoint, accounts, operators, allocations). Trigger phrases: `which fireboxes are offline`, `which customers have unprotected endpoints`, `who is exposed to this CVE`, `threatsync incident queue`, `fireboxes behind on firmware`, `wait for the firebox deployment`, `watchguard license allocation`, `use watchguard`, `run watchguard-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "WatchGuard"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - watchguard-cli
    install:
      - kind: script
        bins: [watchguard-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1
---

# WatchGuard Cloud Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `watchguard-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex
   ```
3. Verify: `watchguard-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `watchguard-cli` and `watchguard-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

WatchGuard splits its public API into separate per-product services behind one OAuth client-credentials login and an API key. This CLI covers the Firebox, FireCloud, Endpoint Security, ThreatSync, AuthPoint, NDR and WatchGuard Cloud platform APIs, built from WatchGuard's own published OpenAPI files, and adds Service Provider fan-out across managed accounts, offline sync and search, and agent-ready JSON on every command.

## When to Use This CLI

Use this CLI when an MSP or admin needs to read or act on WatchGuard Cloud: Firebox fleet status and configuration, WatchGuard Endpoint Security (EPDR/EDR/EPP) posture and device actions, ThreatSync incidents, AuthPoint authentications, WatchGuard NDR assets and Smart Alerts, or Service Provider account, operator and license management. It is built for multi-account work where one credential spans many Subscriber accounts.

## Anti-triggers

Do not use this CLI for:
- Do not use it to manage firewall policies on locally-managed Fireboxes; WatchGuard's Firebox Management API manages firewall policies only for cloud-managed Fireboxes (device status, firmware and offline checks cover every Firebox in WatchGuard Cloud that runs Fireware v12.5.3 or higher)
- Do not use it for Panda-branded Aether products (Adaptive Defense 360, Endpoint Protection); those use the separate Aether Endpoint Security API
- Do not use it to place distributor orders or create Tier-1 partner accounts; those Subscription Billing APIs are distributor-only
- Do not use it for on-premises WatchGuard System Manager, Dimension or Fireware Web UI tasks

## Unique Capabilities

### Service Provider fan-out
- **`endpoint-devices posture`**  -  List every endpoint that is unprotected, out of date, isolated, unlicensed or waiting on a reboot, across the accounts a Service Provider manages.

  _Reach for this when asked which customers have unprotected or isolated endpoints; one call replaces a per-subscriber tour._

  ```bash
  watchguard-cli endpoint-devices posture --agent
  ```
- **`threatsync-incidents queue`**  -  See open ThreatSync incidents across your managed accounts, ranked by severity, threat score and age, with untouched incidents and recommended-but-not-taken actions called out.

  _Use this to decide what to triage next instead of paging raw incident lists._

  ```bash
  watchguard-cli threatsync-incidents queue --duration 7D --agent
  ```
- **`accounts health`**  -  One row per managed account: Fireboxes offline, endpoints at risk, open incidents by severity and licenses expiring soon (Service Provider accounts).

  _Use this for a morning check or customer review that spans every WatchGuard product._

  ```bash
  watchguard-cli accounts health --agent
  ```
- **`allocations reconcile`**  -  Compare a Service Provider's Firebox allocations against the Fireboxes that actually report in, and summarize owned, allocated and available software licenses.

  _Use this before a billing run or after onboarding and offboarding to find allocation drift._

  ```bash
  watchguard-cli allocations reconcile --expiring-within 30d --agent
  ```
- **`endpoint-patches exposure`**  -  Group missing patches by CVE and criticality across managed accounts (needs WatchGuard Patch Management), showing which customers and devices are exposed.

  _Use this on Patch Tuesday or when a new CVE lands to see who is exposed._

  ```bash
  watchguard-cli endpoint-patches exposure --agent
  ```

### Change control
- **`firebox-transactions wait`**  -  Block until a Firebox deployment or certificate-install transaction finishes: exit 0 on complete, 8 if it ended without completing (failed, timed out or canceled), 9 if still running at --max-wait.

  _Use this right after a deploy so a script or agent gets a pass or fail instead of polling._

  ```bash
  watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m
  ```

### Local fleet history
- **`firebox-devices changes`**  -  After a sync, see which Fireboxes, FireClusters and Access Points went offline, came back, appeared, disappeared or changed firmware since the previous sync.

  _Use this to answer what changed overnight in the firewall fleet._

  ```bash
  watchguard-cli firebox-devices changes --agent
  ```
- **`firebox-devices firmware-lag`**  -  From synced devices, find Fireboxes and Access Points running older firmware than the newest version seen for the same model in your fleet.

  _Use this after a firmware advisory to list the units that still need an upgrade._

  ```bash
  watchguard-cli firebox-devices firmware-lag --agent
  ```

## Safety for agents

- **Read freely:** `accounts health`, `endpoint-devices posture`, `threatsync-incidents queue`, `endpoint-patches exposure`, `allocations reconcile`, `firebox-devices changes` / `firmware-lag`, `firebox-transactions wait`, `sync`, `search`, and every `list` / `get` except the credential reads below.
- **Ask a human first, every time:** `firebox-bovpn-tunnels list` / `get` (BOVPN pre-shared keys), `firebox-certificates list` / `get` (private keys and their passwords), `firebox-networks list` / `get` (PPPoE, Dynamic DNS and Wi-Fi credentials), `firebox-users list` / `get` and `firebox-content-filtering list` / `get` (passwords), `activations list-recent` / `status` (activation keys), `allocations list-owned` / `list-assigned` / `list-allocated` (serial numbers and software license keys) and `endpoint-installers` (an enrollment download link) print secrets in plaintext; `export` of those resources prints or writes them too; and `--deliver webhook:<url>` on any command posts its output off the machine. The local mirror and the response cache never store those fields.
- **Preview, then ask:** no write asks for confirmation. Pass `--dry-run` first and show the user the request (it sends nothing, requests no token and changes nothing on disk; `jobs prune`, `profile save` and `profile delete` refuse it). `endpoint-devices isolate` / `stop-isolation` / `scan` / `action` / `uninstall` act on customer computers, `threatsync-actions perform` and `threatsync-incident-actions perform` act on incident entities, `authpoint push` sends a push to a user's phone, `firebox-deployments` pushes configuration to Fireboxes, and the `firebox-policies`, exception, BOVPN, certificate, template, allocation, account and operator writes change configuration or licensing. Never run these, or any `delete`, without explicit confirmation. `accounts delete` with no flags targets `--account-id`, which defaults to your own `WATCHGUARD_ACCOUNT_ID`.
- **MCP read-only mode:** with `WATCHGUARD_MCP_READ_ONLY=1` the MCP server drops its write tools and `watchguard_execute` refuses every non-GET call; GET credential reads still pass, so pair it with the read-only Access ID. `tail` and `import` are not MCP tools, and MCP tools never take a local file path for the CLI to read (`teach-playbook` notes and playbook files, `import --input`).
- **Credentials:** a cached token is reused only by a process with the same Access ID, scope and token URL; a process with a different Access ID and no password stops with exit 4 instead of borrowing it. Give a reporting agent the read-only Access ID.
- **Service Provider fan-out:** `accounts health`, `endpoint-devices posture`, `endpoint-patches exposure`, `threatsync-incidents queue` and `allocations reconcile` exit 6 when any read failed (listed in `fetch_failures`, each row's `errors`, or `incidents_error` / `licenses_error`) and 5 when nothing could be read. WatchGuard documents no separate answer for a product a customer does not have, so that also counts as a failed read. In `accounts health` a failed count is `null`, the row says `complete: false` and is listed first. Report the failures; never read a missing count as zero or healthy.
- Refresh the mirror with `sync --full`, not a bare `sync`: no WatchGuard resource syncs incrementally, and only a full run prunes Fireboxes and rows that disappeared, so `firebox-devices changes` reports them as removed. The Firebox device sync includes every Subscriber account's devices by default (`include_hierarchy=true`).
- Other GET responses are cached on disk for 5 minutes. After a change, re-run a report with `--no-cache` or you get the earlier answer.

## Command Reference

**accounts**  -  Manage accounts

- `watchguard-cli accounts create`  -  Creates a managed account.
- `watchguard-cli accounts delete`  -  Deletes the specified account.
- `watchguard-cli accounts get`  -  Retrieves information for the specified account.
- `watchguard-cli accounts list-managed`  -  Retrieves a list of managed accounts.
- `watchguard-cli accounts update`  -  Updates a managed account.

**activations**  -  Manage activations

- `watchguard-cli activations activate`  -  Activates one or more hardware devices or software licenses.
- `watchguard-cli activations list-recent`  -  Returns activations from the last 30 days for the account.
- `watchguard-cli activations status`  -  Returns activation statuses for the specified batch ID.

**allocations**  -  Manage allocations

- `watchguard-cli allocations allocate`  -  Allocate an asset to a managed account.
- `watchguard-cli allocations deallocate`  -  Remove an allocated asset from a managed account.
- `watchguard-cli allocations list-allocated`  -  Returns a list of allocated assets (devices and software licenses) for a Service Provider's managed accounts.
- `watchguard-cli allocations list-assigned`  -  Returns a list of assets (devices and software licenses) allocated to a specific account.
- `watchguard-cli allocations list-owned`  -  Returns a list of assets (devices and software licenses) owned by a specific account.
- `watchguard-cli allocations summary`  -  Get an inventory summary for a Service Provider account, based on allocation status.
- `watchguard-cli allocations update-hardware`  -  For hardware devices, you can update the allocationExpiryDate and allocationExpiryType of an existing allocation.
- `watchguard-cli allocations update-software`  -  For software products, you can update the quantity, allocationExpiryDate

**audiences**  -  Manage audiences

- `watchguard-cli audiences`  -  Sets and returns the audience parameter value for the specified managed account.

**authpoint**  -  Manage authpoint

- `watchguard-cli authpoint check-policy`  -  Returns whether the user can authenticate and which authentication methods they can use (Push, QR code, and OTP).
- `watchguard-cli authpoint forgot-token`  -  Validates the Forgot Token Verification Code and enables Forgot Token mode for the user.
- `watchguard-cli authpoint otp`  -  Validates the one-time password provided by the user.
- `watchguard-cli authpoint password`  -  Validates a user's password.
- `watchguard-cli authpoint push`  -  Request a QR code be generated or a push notification be sent to the user.
- `watchguard-cli authpoint qrcode`  -  Validates the Verification Code provided by a user for the generated QR code.
- `watchguard-cli authpoint transaction-status`  -  Check the status of the push notification sent to the user.

**endpoint-configurations**  -  Manage endpoint configurations

- `watchguard-cli endpoint-configurations assign`  -  Links devices to a specified configuration.
- `watchguard-cli endpoint-configurations list`  -  Retrieves a list of the specified type of managed configurations associated with your WatchGuard Cloud account.

**endpoint-devices**  -  Manage endpoint devices

- `watchguard-cli endpoint-devices action`  -  Initiates an action on the specified devices. For example, send an action to reboot a device.
- `watchguard-cli endpoint-devices hardware`  -  Retrieves the characteristics of a device BIOS.
- `watchguard-cli endpoint-devices isolate`  -  Isolates the specified devices.
- `watchguard-cli endpoint-devices list`  -  Retrieves a list of devices, and additional information, such as the device IP address and operating system.
- `watchguard-cli endpoint-devices protection-status`  -  Retrieves a list of devices with their protection status and other device information.
- `watchguard-cli endpoint-devices scan`  -  Starts a task to scan the specified devices immediately.
- `watchguard-cli endpoint-devices stop-isolation`  -  Stops isolation on the specified devices.
- `watchguard-cli endpoint-devices uninstall`  -  Uninstalls protection from the specified devices.

**endpoint-installers**  -  Manage endpoint installers

- `watchguard-cli endpoint-installers`  -  Retrieves the URL to download an installation package that installs Panda software with the specified managed

**endpoint-licenses**  -  Manage endpoint licenses

- `watchguard-cli endpoint-licenses`  -  Retrieves license information for Panda products associated with your WatchGuard Cloud account.

**endpoint-patches**  -  Manage endpoint patches

- `watchguard-cli endpoint-patches`  -  Retrieves published patches not installed on the devices on the network.

**endpoint-risks**  -  Manage endpoint risks

- `watchguard-cli endpoint-risks company-summary`  -  Retrieves a summary of the company risk status.
- `watchguard-cli endpoint-risks detected`  -  Retrieves a count of detected devices for each type of risk.
- `watchguard-cli endpoint-risks devices`  -  Retrieves a count of risks detected on each device by risk level.
- `watchguard-cli endpoint-risks get-config`  -  Retrieves the risk configuration.
- `watchguard-cli endpoint-risks statistics`  -  Retrieves an overview of the number and types of risks detected over time.
- `watchguard-cli endpoint-risks update-config`  -  Modifies the risk configuration.

**endpoint-security-events**  -  Manage endpoint security events

- `watchguard-cli endpoint-security-events counters`  -  Retrieves counts of detected security events for the specified types.
- `watchguard-cli endpoint-security-events export`  -  Retrieves a list of security events of the specified type for the specified device.
- `watchguard-cli endpoint-security-events overview`  -  Retrieves counters for security overview.

**endpoint-software**  -  Manage endpoint software

- `watchguard-cli endpoint-software`  -  Retrieves the software installed on the computers on the network.

**endpoint-tasks**  -  Manage endpoint tasks

- `watchguard-cli endpoint-tasks details`  -  Retrieves the details of a specific task.
- `watchguard-cli endpoint-tasks job-results`  -  Retrieves the results of a task specific repetition.
- `watchguard-cli endpoint-tasks job-status`  -  Retrieves the status of task repetitions for a specific task.
- `watchguard-cli endpoint-tasks jobs`  -  Retrieves task repetitions for a specific task.
- `watchguard-cli endpoint-tasks list`  -  Retrieves a list of all tasks created.

**endpoint-unmanaged-devices**  -  Manage endpoint unmanaged devices

- `watchguard-cli endpoint-unmanaged-devices`  -  Retrieves a list of unmanaged devices discovered on the network.

**firebox-aliases**  -  Manage firebox aliases

- `watchguard-cli firebox-aliases get`  -  Retrieves the alias with the specified object ID.
- `watchguard-cli firebox-aliases list`  -  Retrieves a list of aliases for the account or for a specific device or template.

**firebox-auth-groups**  -  Manage firebox auth groups

- `watchguard-cli firebox-auth-groups get`  -  Retrieves the auth group with the specified object ID.
- `watchguard-cli firebox-auth-groups list`  -  Retrieves a list of auth groups for the account or for a specific device or template.

**firebox-authpoint-user-groups**  -  Manage firebox authpoint user groups

- `watchguard-cli firebox-authpoint-user-groups get`  -  Retrieves the AuthPoint user group with the specified object ID.
- `watchguard-cli firebox-authpoint-user-groups list`  -  Retrieves a list of AuthPoint user groups.

**firebox-blocked-sites**  -  Manage firebox blocked sites

- `watchguard-cli firebox-blocked-sites create`  -  Creates a new blocked sites exception.
- `watchguard-cli firebox-blocked-sites delete`  -  Deletes the specified blocked sites exception.
- `watchguard-cli firebox-blocked-sites get`  -  Retrieves the specified blocked sites exception.
- `watchguard-cli firebox-blocked-sites list`  -  Retrieves blocked sites exceptions saved in the specified WatchGuard Cloud account.
- `watchguard-cli firebox-blocked-sites update`  -  Updates the specified blocked sites exception.

**firebox-botnet-exceptions**  -  Manage firebox botnet exceptions

- `watchguard-cli firebox-botnet-exceptions create`  -  Creates a new botnet site exception.
- `watchguard-cli firebox-botnet-exceptions delete`  -  Deletes the specified botnet site exception.
- `watchguard-cli firebox-botnet-exceptions get`  -  Retrieves the specified botnet site exception.
- `watchguard-cli firebox-botnet-exceptions list`  -  Retrieves botnet site exceptions saved in the specified WatchGuard Cloud account.
- `watchguard-cli firebox-botnet-exceptions update`  -  Updates the specified botnet site exception.

**firebox-bovpn-p1-settings**  -  Manage firebox bovpn p1 settings

- `watchguard-cli firebox-bovpn-p1-settings create`  -  Creates a shared P1 settings object. One object can exist per Firebox.
- `watchguard-cli firebox-bovpn-p1-settings get`  -  Retrieves the BOVPN IPSec P1 shared settings object for the given device ID.
- `watchguard-cli firebox-bovpn-p1-settings update`  -  Updates the specified BOVPN IPSec P1 shared settings object for the device.

**firebox-bovpn-tunnels**  -  Manage firebox bovpn tunnels

- `watchguard-cli firebox-bovpn-tunnels create`  -  Creates a BOVPN IPSec tunnel in the given account. All tunnels require at least one cloud-managed Firebox endpoint.
- `watchguard-cli firebox-bovpn-tunnels delete`  -  Deletes the BOVPN IPSec tunnel with the given object ID.
- `watchguard-cli firebox-bovpn-tunnels delete-by-name`  -  Deletes the BOVPN IPSec tunnel with the given tunnel name.
- `watchguard-cli firebox-bovpn-tunnels get`  -  Retrieves the BOVPN IPSec tunnel with the given object ID.
- `watchguard-cli firebox-bovpn-tunnels list`  -  Given an account ID, retrieves a list of BOVPN tunnels for the account or for a specific device.
- `watchguard-cli firebox-bovpn-tunnels patch`  -  Update the routes list or certificate of a specific BOVPN tunnel.
- `watchguard-cli firebox-bovpn-tunnels patch-by-name`  -  Update the routes list or certificate of a specific BOVPN tunnel.
- `watchguard-cli firebox-bovpn-tunnels update`  -  Updates the BOVPN IPSec tunnel with the given object ID.
- `watchguard-cli firebox-bovpn-tunnels update-by-name`  -  Update a BOVPN IPSec tunnel specified by its name through a URL parameter.

**firebox-certificates**  -  Manage firebox certificates

- `watchguard-cli firebox-certificates create`  -  Creates an account-level or device-level certificate.
- `watchguard-cli firebox-certificates delete`  -  Deletes the certificate with the given object ID.
- `watchguard-cli firebox-certificates get`  -  Retrieves the certificate with the given object ID.
- `watchguard-cli firebox-certificates install`  -  Install a certificate on a Firebox or a list of Fireboxes.
- `watchguard-cli firebox-certificates list`  -  Retrieves a list of certificates for the account.

**firebox-content-filtering**  -  Manage firebox content filtering

- `watchguard-cli firebox-content-filtering get`  -  Retrieves the content filtering rule with the specified object ID.
- `watchguard-cli firebox-content-filtering list`  -  Retrieves a list of content filtering rules for the account or for a specific device or template.

**firebox-deployments**  -  Manage firebox deployments

- `watchguard-cli firebox-deployments`  -  Deploys exceptions or full configurations saved in WatchGuard Cloud to the specified devices.

**firebox-devices**  -  Manage firebox devices

- `watchguard-cli firebox-devices`  -  Retrieves a list of Fireboxes, FireClusters, and Access Points for the specified WatchGuard Cloud account.

**firebox-exceptions**  -  Manage firebox exceptions

- `watchguard-cli firebox-exceptions disable-global`  -  Disables the ability to manage and deploy global exceptions with the Firebox Management API for the specified device(s).
- `watchguard-cli firebox-exceptions enable-global`  -  Enables the ability to manage and deploy exceptions with the Firebox Management API for the specified device(s).
- `watchguard-cli firebox-exceptions list`  -  Retrieves exceptions saved in the specified WatchGuard Cloud account.

**firebox-file-exceptions**  -  Manage firebox file exceptions

- `watchguard-cli firebox-file-exceptions create`  -  Creates a new file exception.
- `watchguard-cli firebox-file-exceptions delete`  -  Deletes the specified file exception.
- `watchguard-cli firebox-file-exceptions get`  -  Retrieves the specified file exception.
- `watchguard-cli firebox-file-exceptions list`  -  Retrieves file exceptions saved in the specified WatchGuard Cloud account.
- `watchguard-cli firebox-file-exceptions update`  -  Updates the specified file exception.

**firebox-geo-exceptions**  -  Manage firebox geo exceptions

- `watchguard-cli firebox-geo-exceptions create`  -  Creates a new Geolocation exception.
- `watchguard-cli firebox-geo-exceptions delete`  -  Deletes the specified Geolocation exception.
- `watchguard-cli firebox-geo-exceptions get`  -  Retrieves the specified Geolocation exception.
- `watchguard-cli firebox-geo-exceptions list`  -  Retrieves all Geolocation exceptions saved in the specified WatchGuard Cloud account.
- `watchguard-cli firebox-geo-exceptions update`  -  Updates the specified Geolocation exception.

**firebox-groups**  -  Manage firebox groups

- `watchguard-cli firebox-groups get`  -  Retrieves the group with the specified object ID.
- `watchguard-cli firebox-groups list`  -  Retrieves a list of groups for the account or for a specific device or template.

**firebox-ips-exceptions**  -  Manage firebox ips exceptions

- `watchguard-cli firebox-ips-exceptions create`  -  Creates a new IPS signature exception.
- `watchguard-cli firebox-ips-exceptions delete`  -  Deletes the specified IPS signature exception.
- `watchguard-cli firebox-ips-exceptions get`  -  Retrieves the specified IPS signature exception.
- `watchguard-cli firebox-ips-exceptions list`  -  Retrieves all IPS signature exceptions.
- `watchguard-cli firebox-ips-exceptions update`  -  Updates the specified IPS signature exception.

**firebox-networks**  -  Manage firebox networks

- `watchguard-cli firebox-networks get`  -  Retrieves the network with the specified object ID.
- `watchguard-cli firebox-networks list`  -  Retrieves a list of networks for the account or for a specific device or template.

**firebox-policies**  -  Manage firebox policies

- `watchguard-cli firebox-policies create`  -  Creates a new firewall policy for a device.
- `watchguard-cli firebox-policies delete`  -  Deletes the firewall policy with the specified object ID. Permanent policies cannot be deleted.
- `watchguard-cli firebox-policies get`  -  Retrieves the firewall policy with the specified object ID.
- `watchguard-cli firebox-policies list`  -  Retrieves a list of firewall policies for the account or for a specific device.
- `watchguard-cli firebox-policies set-enabled`  -  Enables or disables the firewall policy with the specified object ID without modifying other settings.
- `watchguard-cli firebox-policies update`  -  Updates the firewall policy with the specified object ID.

**firebox-policy-groups**  -  Manage firebox policy groups

- `watchguard-cli firebox-policy-groups get`  -  Retrieves the policy group with the specified object ID.
- `watchguard-cli firebox-policy-groups list`  -  Retrieves a list of policy groups for the account or for a specific device or template.

**firebox-reports**  -  Manage firebox reports

- `watchguard-cli firebox-reports executive`  -  Retrieves the specified Executive Dashboard records for the specified devices.
- `watchguard-cli firebox-reports security`  -  Retrieves the specified Security Dashboard records for the specified devices.

**firebox-saml-user-groups**  -  Manage firebox saml user groups

- `watchguard-cli firebox-saml-user-groups get`  -  Retrieves the SAML user group with the specified object ID.
- `watchguard-cli firebox-saml-user-groups list`  -  Retrieves a list of SAML user groups.

**firebox-schedules**  -  Manage firebox schedules

- `watchguard-cli firebox-schedules get`  -  Retrieves the schedule with the specified object ID.
- `watchguard-cli firebox-schedules list`  -  Retrieves a list of schedules for the account or for a specific device or template.

**firebox-sdwans**  -  Manage firebox sdwans

- `watchguard-cli firebox-sdwans get`  -  Retrieves the SD-WAN with the specified object ID.
- `watchguard-cli firebox-sdwans list`  -  Retrieves a list of SD-WANs for the account or for a specific device or template.

**firebox-snat-actions**  -  Manage firebox snat actions

- `watchguard-cli firebox-snat-actions get`  -  Retrieves the SNAT action with the specified object ID.
- `watchguard-cli firebox-snat-actions list`  -  Retrieves a list of SNAT actions for the account or for a specific device or template.

**firebox-templates**  -  Manage firebox templates

- `watchguard-cli firebox-templates list-subscriptions`  -  With no query parameters, returns templates available to the account, including inherited templates.
- `watchguard-cli firebox-templates subscribe`  -  Subscribes, unsubscribes, or changes the order of Firebox templates on a Firebox.

**firebox-traffic-shaping-rules**  -  Manage firebox traffic shaping rules

- `watchguard-cli firebox-traffic-shaping-rules get`  -  Retrieves the traffic shaping rule with the specified object ID.
- `watchguard-cli firebox-traffic-shaping-rules list`  -  Retrieves a list of traffic shaping rules for the account or for a specific device or template.

**firebox-traffic-types**  -  Manage firebox traffic types

- `watchguard-cli firebox-traffic-types get`  -  Retrieves the traffic type with the specified object ID.
- `watchguard-cli firebox-traffic-types list`  -  Retrieves a list of traffic types for the account or for a specific device or template.

**firebox-transactions**  -  Manage firebox transactions

- `watchguard-cli firebox-transactions cancel`  -  Deletes the specified transaction record.
- `watchguard-cli firebox-transactions get`  -  Retrieves the specified transaction record, which includes details of a specific deployment.
- `watchguard-cli firebox-transactions list`  -  Retrieves a list of transaction records, which include details of individual deployments.
- `watchguard-cli firebox-transactions update`  -  Updates the specified transaction record.

**firebox-users**  -  Manage firebox users

- `watchguard-cli firebox-users get`  -  Retrieves the user with the specified object ID.
- `watchguard-cli firebox-users list`  -  Retrieves a list of users for the account or for a specific device or template.

**firebox-webblocker-exceptions**  -  Manage firebox webblocker exceptions

- `watchguard-cli firebox-webblocker-exceptions create`  -  Creates a new WebBlocker exception.
- `watchguard-cli firebox-webblocker-exceptions delete`  -  Deletes the specified WebBlocker exception.
- `watchguard-cli firebox-webblocker-exceptions get`  -  Retrieves the specified WebBlocker exception.
- `watchguard-cli firebox-webblocker-exceptions list`  -  Retrieves all WebBlocker exceptions.
- `watchguard-cli firebox-webblocker-exceptions update`  -  Updates the specified WebBlocker exception.

**firecloud-blocked-sites**  -  Manage firecloud blocked sites

- `watchguard-cli firecloud-blocked-sites create`  -  Creates a new blocked sites exception.
- `watchguard-cli firecloud-blocked-sites delete`  -  Deletes the specified blocked sites exception.
- `watchguard-cli firecloud-blocked-sites get`  -  Retrieves the specified blocked sites exception.
- `watchguard-cli firecloud-blocked-sites list`  -  Retrieves all blocked sites exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-blocked-sites update`  -  Updates the specified blocked sites exception.

**firecloud-botnet-exceptions**  -  Manage firecloud botnet exceptions

- `watchguard-cli firecloud-botnet-exceptions create`  -  Creates a new botnet site exception.
- `watchguard-cli firecloud-botnet-exceptions delete`  -  Deletes the specified botnet site exception.
- `watchguard-cli firecloud-botnet-exceptions get`  -  Retrieves the specified botnet site exception.
- `watchguard-cli firecloud-botnet-exceptions list`  -  Retrieves all botnet site exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-botnet-exceptions update`  -  Updates the specified botnet site exception.

**firecloud-exceptions**  -  Manage firecloud exceptions

- `watchguard-cli firecloud-exceptions`  -  Retrieves all exceptions in the specified FireCloud account.

**firecloud-file-exceptions**  -  Manage firecloud file exceptions

- `watchguard-cli firecloud-file-exceptions create`  -  Creates a new file exception.
- `watchguard-cli firecloud-file-exceptions delete`  -  Deletes the specified file exception.
- `watchguard-cli firecloud-file-exceptions get`  -  Retrieves the specified file exception.
- `watchguard-cli firecloud-file-exceptions list`  -  Retrieves all file exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-file-exceptions update`  -  Updates the specified file exception.

**firecloud-geo-exceptions**  -  Manage firecloud geo exceptions

- `watchguard-cli firecloud-geo-exceptions create`  -  Creates a new Geolocation exception.
- `watchguard-cli firecloud-geo-exceptions delete`  -  Deletes the specified Geolocation exception.
- `watchguard-cli firecloud-geo-exceptions get`  -  Retrieves the specified Geolocation exception.
- `watchguard-cli firecloud-geo-exceptions list`  -  Retrieves all Geolocation exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-geo-exceptions update`  -  Updates the specified Geolocation exception.

**firecloud-https-exceptions**  -  Manage firecloud https exceptions

- `watchguard-cli firecloud-https-exceptions create`  -  Creates a new HTTPS decryption exception.
- `watchguard-cli firecloud-https-exceptions delete`  -  Deletes the specified HTTPS decryption exception.
- `watchguard-cli firecloud-https-exceptions get`  -  Retrieves the specified HTTPS decryption exception.
- `watchguard-cli firecloud-https-exceptions list`  -  Retrieves all HTTPS decryption exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-https-exceptions update`  -  Updates the specified HTTPS decryption exception.

**firecloud-ips-exceptions**  -  Manage firecloud ips exceptions

- `watchguard-cli firecloud-ips-exceptions create`  -  Creates a new IPS signature exception.
- `watchguard-cli firecloud-ips-exceptions delete`  -  Deletes the specified IPS signature exception.
- `watchguard-cli firecloud-ips-exceptions get`  -  Retrieves the specified IPS signature exception.
- `watchguard-cli firecloud-ips-exceptions list`  -  Retrieves all IPS signature exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-ips-exceptions update`  -  Updates the specified IPS signature exception.

**firecloud-webblocker-exceptions**  -  Manage firecloud webblocker exceptions

- `watchguard-cli firecloud-webblocker-exceptions create`  -  Creates a new WebBlocker exception.
- `watchguard-cli firecloud-webblocker-exceptions delete`  -  Deletes the specified WebBlocker exception.
- `watchguard-cli firecloud-webblocker-exceptions get`  -  Retrieves the specified WebBlocker exception.
- `watchguard-cli firecloud-webblocker-exceptions list`  -  Retrieves all WebBlocker exceptions in the specified FireCloud account.
- `watchguard-cli firecloud-webblocker-exceptions update`  -  Updates the specified WebBlocker exception.

**ndr-assets**  -  Manage ndr assets

- `watchguard-cli ndr-assets create`  -  Creates an asset.
- `watchguard-cli ndr-assets delete`  -  Deletes an asset specified by asset ID.
- `watchguard-cli ndr-assets get`  -  Retrieves details for an asset specified by asset ID.
- `watchguard-cli ndr-assets list`  -  Retrieves a list of all the assets for the specified tenant.
- `watchguard-cli ndr-assets update`  -  Updates an asset detail specified by asset ID.

**ndr-smart-alerts**  -  Manage ndr smart alerts

- `watchguard-cli ndr-smart-alerts close`  -  Closes a Smart Alert specified by a Smart Alert ID. You can also specify the reason to close the Smart Alert.
- `watchguard-cli ndr-smart-alerts get`  -  Retrieves details of a Smart Alert specified by a Smart Alert ID.

**operators**  -  Manage operators

- `watchguard-cli operators create`  -  Creates new operators in the specified account. In the request body, specify the details of the operators to create.
- `watchguard-cli operators delete`  -  Deletes operators from the specified account. In the request body, specify the details of the operators to delete.
- `watchguard-cli operators list`  -  To get details of all operators for an account, specify the account ID in the query parameter.
- `watchguard-cli operators transaction-status`  -  To get the status of a transaction, specify the transaction ID in the query parameter.
- `watchguard-cli operators update`  -  Updates existing operators in the specified account.

**threatsync-actions**  -  Manage threatsync actions

- `watchguard-cli threatsync-actions list`  -  Retrieves a list of actions.
- `watchguard-cli threatsync-actions perform`  -  Performs a specified action.

**threatsync-comments**  -  Manage threatsync comments

- `watchguard-cli threatsync-comments create`  -  Create comment for a specific incident.
- `watchguard-cli threatsync-comments delete`  -  Deletes a specific comment.
- `watchguard-cli threatsync-comments list`  -  Retrieves a list of comments for a specific incident.
- `watchguard-cli threatsync-comments update`  -  Updates a specific comment.

**threatsync-incident-actions**  -  Manage threatsync incident actions

- `watchguard-cli threatsync-incident-actions get`  -  Retrieves details of a specific action for an incident.
- `watchguard-cli threatsync-incident-actions list`  -  Retrieves a list of actions for a specific incident.
- `watchguard-cli threatsync-incident-actions perform`  -  Performs an action for a specific incident.

**threatsync-incidents**  -  Manage threatsync incidents

- `watchguard-cli threatsync-incidents get`  -  Retrieves a specific incident by incident ID.
- `watchguard-cli threatsync-incidents list`  -  Retrieves a list of specific ThreatSync incidents for an account.
- `watchguard-cli threatsync-incidents set-status`  -  Updates the status of a specific incident.

**threatsync-transactions**  -  Manage threatsync transactions

- `watchguard-cli threatsync-transactions`  -  Submits multiple requests to ThreatSync Management API endpoints at the same time.

**accounts**

- `watchguard-cli accounts health`  -  One row per managed account: Fireboxes offline, endpoints at risk, open incidents by severity and licenses expiring soon (Service Provider accounts).

**allocations**

- `watchguard-cli allocations reconcile`  -  Compare a Service Provider's Firebox allocations against the Fireboxes that actually report in, and summarize owned, allocated and available software licenses.

**endpoint-devices**

- `watchguard-cli endpoint-devices posture`  -  List every endpoint that is unprotected, out of date, isolated, unlicensed or waiting on a reboot, across the accounts a Service Provider manages.

**endpoint-patches**

- `watchguard-cli endpoint-patches exposure`  -  Group missing patches by CVE and criticality across managed accounts (needs WatchGuard Patch Management), showing which customers and devices are exposed.

**firebox-devices**

- `watchguard-cli firebox-devices changes`  -  After a sync, see which Fireboxes, FireClusters and Access Points went offline, came back, appeared, disappeared or changed firmware since the previous sync.

- `watchguard-cli firebox-devices firmware-lag`  -  From synced devices, find Fireboxes and Access Points running older firmware than the newest version seen for the same model in your fleet.

**firebox-transactions**

- `watchguard-cli firebox-transactions wait`  -  Block until a Firebox deployment or certificate-install transaction finishes: exit 0 on complete, 8 if it ended without completing (failed, timed out or canceled), 9 if still running at --max-wait.

**threatsync-incidents**

- `watchguard-cli threatsync-incidents queue`  -  See open ThreatSync incidents across your managed accounts, ranked by severity, threat score and age, with untouched incidents and recommended-but-not-taken actions called out.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
watchguard-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Fleet status with only the fields you need

```bash
watchguard-cli firebox-devices --include-hierarchy --agent --select id,name,state,version,offline_since
```

Every Firebox, FireCluster and Access Point including subscriber devices, narrowed to state and firmware.

### What changed in the fleet since the last sync

```bash
watchguard-cli firebox-devices changes --agent
```

After 'sync --resources firebox_devices --resource-param firebox_devices:include_hierarchy=true --full', lists devices that went offline, came back, appeared, were removed or changed firmware since the previous sync.

### Triage queue across customers

```bash
watchguard-cli threatsync-incidents queue --duration 7D --agent
```

Open ThreatSync incidents from every subscriber ranked by severity and age, with untouched ones flagged.

### Endpoint posture sweep

```bash
watchguard-cli endpoint-devices posture --agent
```

Unprotected, isolated, unlicensed or outdated endpoints across all subscribers in one list.

### Wait for a deployment to land

```bash
watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m
```

Blocks until the device's newest deployment completes (exit 0) or fails (exit 8); exit 9 if still running at --max-wait.

## Auth Setup

Enable API access in WatchGuard Cloud under Administration > Managed Access (Owner or Administrator role). Set WATCHGUARD_CLIENT_ID to the Access ID (read-only or read-write), WATCHGUARD_CLIENT_SECRET to the matching API password, and WATCHGUARD_API_KEY to the API key. The CLI exchanges the Access ID and password for a one-hour bearer token and sends the API key header on every call; 'auth status' shows what is configured. Set WATCHGUARD_ACCOUNT_ID to your account ID (ACC-... or WGC-...) so commands do not need --account-id. Accounts outside North America set WATCHGUARD_BASE_URL to https://api.deu.cloud.watchguard.com (EMEA) or https://api.jpn.cloud.watchguard.com (APAC) - the host Managed Access shows, without its /rest/ suffix - and WATCHGUARD_TOKEN_URL to the same host plus /oauth/token. A Service Provider reaches one managed account's resources with that account's audience: run 'watchguard-cli audiences --account-id ACC-1234567', then export WATCHGUARD_AUDIENCE=<audience> (or let the fan-out commands do it per account).

Run `watchguard-cli doctor` to verify setup.

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
  watchguard-cli accounts get --agent --select accountInfo
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

- Use `--home <dir>` for one invocation, or set `WATCHGUARD_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `WATCHGUARD_CONFIG_DIR`, `WATCHGUARD_DATA_DIR`, `WATCHGUARD_STATE_DIR`, `WATCHGUARD_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `WATCHGUARD_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `watchguard-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "watchguard": {
        "command": "watchguard-mcp",
        "env": {
          "WATCHGUARD_HOME": "/srv/watchguard"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `WATCHGUARD_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `WATCHGUARD_HOME`, or `doctor` will not find credentials left under the former root.

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
watchguard-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "watchguard-cli learnings confirm 12"] }
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
       materially more, record the divergence via `watchguard-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `watchguard-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `watchguard-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
watchguard-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
watchguard-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
watchguard-cli teach-playbook \
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
watchguard-cli playbook amend \
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

`watchguard-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `WATCHGUARD_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
watchguard-cli feedback "the --since flag is inclusive but docs say exclusive"
watchguard-cli feedback --stdin < notes.txt
watchguard-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `WATCHGUARD_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `WATCHGUARD_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
watchguard-cli profile save briefing --json
watchguard-cli --profile briefing accounts get
watchguard-cli profile list --json
watchguard-cli profile show briefing
watchguard-cli profile delete briefing --yes
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
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit). For `firebox-transactions wait`: the deployment ENDED WITHOUT COMPLETING (failed, timed out or canceled; see `status`). |
| 9 | `firebox-transactions wait` only: the transaction was still running when --max-wait elapsed |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `watchguard-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add watchguard-mcp --env WATCHGUARD_CLIENT_ID=<access-id> --env WATCHGUARD_CLIENT_SECRET=<api-password> --env WATCHGUARD_API_KEY=<api-key> --env WATCHGUARD_ACCOUNT_ID=ACC-1234567 -- watchguard-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which watchguard-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   watchguard-cli accounts health --agent
   ```
4. If ambiguous, drill into subcommand help: `watchguard-cli endpoint-devices --help`.
