---
name: veeam
description: "Use when the user asks to check Veeam backup health across customers, find failing or stale backup jobs, surface workloads past their RPO, triage VSPC alarms, or report per-tenant license usage from a Veeam Service Provider Console. Syncs every tenant into a local SQLite mirror so it answers multi-tenant backup questions the per-tenant console can't. Trigger phrases: `check Veeam backup health`, `which Veeam jobs are failing`, `stale Veeam backups across customers`, `Veeam alarms triage`, `Veeam license usage by company`, `use veeam`, `run veeam`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Veeam"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - veeam-cli
---

# Veeam Service Provider Console Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `veeam-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/veeam/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/veeam/install.ps1 | iex
   ```
3. Verify: `veeam-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Drive the full VSPC v3 REST surface  -  companies, backup servers, jobs, backup agents, protected workloads, alarms, licensing, and billing  -  from one CLI built for AI agents. A local SQLite mirror powers offline search and cross-tenant commands the web console can't: fleet-health, stale-backups, at-risk, alarms-triage, license-usage, company-overview, and since (drift).

## When to Use This CLI

Reach for this CLI when an agent or operator needs to inspect or manage Veeam-protected estates across many tenants from a Veeam Service Provider Console: checking backup job health, finding stale or at-risk protection, triaging alarms, reviewing license consumption, or scripting bulk reads against the VSPC v3 API. The local mirror makes fleet-wide questions fast and offline.

## Anti-triggers

Do not use this CLI for:
- Driving a single standalone Veeam Backup & Replication server with no VSPC in front of it (that is the separate VBR REST API)
- Restoring data or running production backup jobs as the system of record  -  use the Veeam console for irreversible recovery operations
- Managing Microsoft 365 backups directly on a VB365 server that is not registered to the console

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-tenant rollups that compound
- **`fleet-health`**  -  One-pane cross-company backup health: jobs by last status, agents online/offline, and active alarms per tenant.

  _The MSP morning-coffee command: reach for it to see every tenant's backup posture in one shot before triaging._

  ```bash
  veeam-cli fleet-health --agent --select companies.company,companies.failed,companies.agents_offline
  ```
- **`stale-backups`**  -  Every backup job and agent job whose last successful run is older than N days, across all tenants, sorted by staleness.

  _The #1 MSP risk question. Use it to catch silently-failing protection before a customer needs a restore._

  ```bash
  veeam-cli stale-backups --days 3 --agent
  ```
- **`company-overview`**  -  Single-tenant 360: backup servers, job status breakdown, agents, active alarms, protected workloads, and license usage assembled from the local store.

  _Answers 'how is this one customer doing?' without a six-tab tour of the web console._

  ```bash
  veeam-cli company-overview --company "Contoso Ltd" --agent
  ```

### Risk and SLA answers
- **`at-risk`**  -  Protected workloads whose latest restore point is older than the RPO threshold (or missing entirely)  -  the data that would be lost on failure today.

  _Turns raw restore-point timestamps into the SLA answer an MSP actually owns._

  ```bash
  veeam-cli at-risk --rpo 24h --agent
  ```
- **`alarms-triage`**  -  Active alarms grouped by company and severity with first-seen / last-seen from local history, deduped for fleet-wide triage.

  _Cuts a noisy multi-tenant alarm feed down to the handful of distinct problems worth acting on._

  ```bash
  veeam-cli alarms-triage --severity Error --agent
  ```
- **`since`**  -  What changed across the fleet inside a time window: backup jobs that failed or warned, alarms that fired, and agents that newly activated.

  _Replaces re-reading the whole console with a focused 'what's new and bad' delta for the shift handoff._

  ```bash
  veeam-cli since 24h --agent
  ```

### Billing and capacity
- **`license-usage`**  -  Per-organization license consumption (used points) with the delta since the previous run.

  _Billing-relevant: spot tenants approaching their license ceiling before overage, in one command._

  ```bash
  veeam-cli license-usage --agent --select organizations.organization,organizations.used_points,organizations.delta
  ```

## Command Reference

**alarms**  -  This resource collection represents Veeam Service Provider Console alarms.

- `veeam-cli alarms acknowledge-active`  -  Assigns the Acknowledged status to a triggered alarm with the specified UID.
- `veeam-cli alarms clone`  -  Creates a clone of an alarm template with the specified UID.
- `veeam-cli alarms delete`  -  Deletes an alarm template with the specified UID.
- `veeam-cli alarms delete-active`  -  Deletes a triggered alarm with the specified UID.
- `veeam-cli alarms get`  -  Returns a collection resource representation of all Veeam Service Provider Console alarm templates.
- `veeam-cli alarms get-active`  -  Returns collection resource representation of all Veeam Service Provider Console triggered alarms.
- `veeam-cli alarms get-active-active`  -  Returns a resource representation of a triggered alarm with the specified UID.
- `veeam-cli alarms get-active-by`  -  Returns all status changes of a triggered alarm with the specified template UID.
- `veeam-cli alarms get-active-history`  -  Returns a collection resource representation of all status changes of a triggered alarm with the specified UID in
- `veeam-cli alarms get-templates`  -  Returns a resource representation of an alarm template with the specified UID.
- `veeam-cli alarms resolve-active`  -  Resolves a triggered alarm with the specified UID.

**async-actions**  -  This resource collection represents Veeam Service Provider Console REST API async actions.

- `veeam-cli async-actions cancel`  -  Cancels an action with the specified UID.
- `veeam-cli async-actions get-config`  -  Returns a configuration of an async action service.
- `veeam-cli async-actions get-info`  -  Returns a collection resource representation of all active async actions of the current user.
- `veeam-cli async-actions get-info-asyncactions`  -  Returns a resource representation of an async action with the specified UID.

**authentication**  -  This resource collection allows to authenticate to Veeam Service Provider Console REST API.

- `veeam-cli authentication decrypt-pkcs12-container`  -  Decrypts an encrypted PKCS#12 container.
- `veeam-cli authentication generate-new-pkcs12-key-pair`  -  Issues a PKCS#12 container with an RSA private key and an X.509 certificate.
- `veeam-cli authentication generate-new-rsa-key-pair`  -  Issues an RSA key pair. > You can specify the key size if needed.
- `veeam-cli authentication generate-totp-secret`  -  Issues a TOTP secret and a URL-encoded secret.

**configuration**  -  This resource collection represents security certificate installed on Veeam Service Provider Console server.

- `veeam-cli configuration copy-backup-policy`  -  Creates a copy of a backup policy with the specified UID.
- `veeam-cli configuration create-linux-backup-policy`  -  Creates a backup policy for Linux computers.
- `veeam-cli configuration create-mac-backup-policy`  -  Creates a backup policy for Mac computers.
- `veeam-cli configuration create-windows-backup-policy`  -  Creates a backup policy for Microsoft Windows computers.
- `veeam-cli configuration delete-backup-policy`  -  Deletes a backup policy with the specified UID.
- `veeam-cli configuration get-all-available-certificates`  -  Returns a collection resource representation of security cerificates on Veeam Service Provider Console server that are
- `veeam-cli configuration get-backup-policies`  -  Returns a collection resource representation of all backup policies.
- `veeam-cli configuration get-backup-policies-to-assign`  -  Returns a collection resource representation of all backup policies that can be assigned to agents.
- `veeam-cli configuration get-backup-policy`  -  Returns a resource representation of a backup policy with the specified UID.
- `veeam-cli configuration get-certificate`  -  Returns a Veeam Service Provider Console server security certificate.
- `veeam-cli configuration get-linux-backup-policies`  -  Returns a collection resource representation of all backup policies configured for Linux computers.
- `veeam-cli configuration get-linux-backup-policy`  -  Returns a resource representation of a Linux computer backup policy with the specified UID.
- `veeam-cli configuration get-mac-backup-policies`  -  Returns a collection resource representation of all backup policies configured for Mac computers.
- `veeam-cli configuration get-mac-backup-policy`  -  Returns a resource representation of a Mac computer backup policy with the specified UID.
- `veeam-cli configuration get-windows-backup-policies`  -  Returns a collection resource representation of all backup policies configured for Microsoft Windows computers.
- `veeam-cli configuration get-windows-backup-policy`  -  Returns a resource representation of a Windows computer backup policy with the specified UID.
- `veeam-cli configuration install-certificate`  -  Installs a Veeam Service Provider Console server security certificate.
- `veeam-cli configuration patch-backup-policy`  -  Modifies a backup policy with the specified UID.
- `veeam-cli configuration patch-linux-backup-policy`  -  Modifies a Linux computer backup policy with the specified UID.
- `veeam-cli configuration patch-mac-backup-policy`  -  Modifies a Mac computer backup policy with the specified UID.
- `veeam-cli configuration patch-windows-backup-policy`  -  Modifies a Windows computer backup policy with the specified UID.

**deployment**  -  This resource collection represents deployment tasks.

- `veeam-cli deployment get-backup-server-configuration-xml`  -  Returns a resource representation of an example for Veeam Backup & Replication server deployment configuration in the
- `veeam-cli deployment get-log`  -  Returns a resource representation of deployment task status.
- `veeam-cli deployment get-task`  -  Returns a resource representation of a deployment task with the specified UID.
- `veeam-cli deployment get-tasks`  -  Returns a collection resource representation of all deployment tasks.
- `veeam-cli deployment get-vone-server-configuration-xml`  -  Returns a resource representation of an example for Veeam ONE server deployment configuration in the `XML` format.
- `veeam-cli deployment wait-task`  -  Initiates an async action that waits for a deployment task with the specified UID to complete.

**discovery**  -  This resource collection represents discovery rules and discovered computers.

- `veeam-cli discovery create-linux-custom-rule`  -  Creates a Linux rule based on a list of IP addresses and DNS names.
- `veeam-cli discovery create-linux-network-based-rule`  -  Creates a Linux network-based discovery rule.
- `veeam-cli discovery create-windows-active-directory-based-rule`  -  Create Microsoft Entra ID Discovery Rule for Windows
- `veeam-cli discovery create-windows-custom-rule`  -  Creates a Windows rule based on a list of IP addresses and DNS names.
- `veeam-cli discovery create-windows-network-based-rule`  -  Creates a Windows network-based discovery rule.
- `veeam-cli discovery delete-rule`  -  Deletes a discovery rule with the specified UID.
- `veeam-cli discovery delete-scheduled-deployment-task-for-computer`  -  Deletes a computer scheduled deployment task with the specified UID.
- `veeam-cli discovery get-computer`  -  Returns a resource representation of a discovered computer with the specified UID.
- `veeam-cli discovery get-computers`  -  Returns a collection resource representation of all discovered computers.
- `veeam-cli discovery get-computers-by-rule`  -  Returns a collection resource representation of all computers discovered with a rule with the specified UID.
- `veeam-cli discovery get-linux-custom-rule`  -  Returns a resource representation of a Linux discovery rule based on a list of IP addresses and DNS names.
- `veeam-cli discovery get-linux-custom-rules`  -  Returns a collection resource representation of all Linux discovery rules based on lists of IP addresses and DNS names.
- `veeam-cli discovery get-linux-network-based-rule`  -  Returns a resource representation of a Linux network-based discovery rule.
- `veeam-cli discovery get-linux-network-based-rules`  -  Returns a collection resource representation of all Linux network-based discovery rules.
- `veeam-cli discovery get-linux-rule`  -  Returns a resource representation of a Linux discovery rule with the specified UID.
- `veeam-cli discovery get-linux-rules`  -  Returns a collection resource representation of all Linux discovery rules.
- `veeam-cli discovery get-rule`  -  Returns a resource representation of a discovery rule with the specified UID.
- `veeam-cli discovery get-rules`  -  Returns a collection resource representation of all discovery rules.
- `veeam-cli discovery get-scheduled-deployment-task-for-computer`  -  Returns a resource representation of a computer scheduled deployment task with the specified UID.
- `veeam-cli discovery get-scheduled-deployment-tasks-for-computer`  -  Returns a collection resource representation of all deployment tasks scheduled for a computer with the specified UID.
- `veeam-cli discovery get-time-zones`  -  Returns a collection resource representation of all time zones.
- `veeam-cli discovery get-windows-active-directory-based-rule`  -  Returns a resource representation of an Microsoft Entra ID discovery rule with the specified UID.
- `veeam-cli discovery get-windows-active-directory-based-rules`  -  Returns a collection resource representation of all Microsoft Entra ID discovery rules.
- `veeam-cli discovery get-windows-custom-rule`  -  Returns a resource representation of a Windows discovery rule based on a list of IP addresses and DNS names.
- `veeam-cli discovery get-windows-custom-rules`  -  Returns a collection resource representation of all Windows discovery rules based on lists of IP addresses and DNS
- `veeam-cli discovery get-windows-network-based-rule`  -  Returns a resource representation of a Windows network-based discovery rule.
- `veeam-cli discovery get-windows-network-based-rules`  -  Returns a collection resource representation of all Windows network-based discovery rules.
- `veeam-cli discovery get-windows-rule`  -  Returns a resource representation of a Windows discovery rule with the specified UID.
- `veeam-cli discovery get-windows-rules`  -  Returns a collection resource representation of all Windows discovery rules.
- `veeam-cli discovery install-backup-agent-on-computer`  -  Deploys Veeam backup agent and management agent on a discovered computer with the specified UID.
- `veeam-cli discovery install-backup-server-on-computer`  -  Installs Veeam Backup & Replication and management agent on a discovered computer with the specified UID.
- `veeam-cli discovery install-linux-backup-agent-on-computer`  -  Deploy Veeam backup agent and management agent on a discovered Linux computer with the specified UID.
- `veeam-cli discovery install-vone-server-on-computer`  -  Installs Veeam ONE and management agent on a discovered computer with the specified UID.
- `veeam-cli discovery patch-linux-custom-rule`  -  Modifies a Linux discovery rule based on a list of IP addresses and DNS names.
- `veeam-cli discovery patch-linux-network-based-rule`  -  Modifies a Linux network-based discovery rule with the specified UID.
- `veeam-cli discovery patch-linux-rule`  -  Modifies a Linux discovery rule with the specified UID.
- `veeam-cli discovery patch-rule`  -  Modifies a discovery rule with the specified UID.
- `veeam-cli discovery patch-scheduled-deployment-task-for-computer`  -  Modifies a computer scheduled deployment task with the specified UID.
- `veeam-cli discovery patch-windows-active-directory-based-rule`  -  Modifies an Microsoft Entra ID discovery rule with the specified UID.
- `veeam-cli discovery patch-windows-custom-rule`  -  Modifies a Windows discovery rule based on a list of IP addresses and DNS names.
- `veeam-cli discovery patch-windows-network-based-rule`  -  Modifies a Windows network-based discovery rule with the specified UID.
- `veeam-cli discovery patch-windows-rule`  -  Modifies a Windows discovery rule with the specified UID.
- `veeam-cli discovery reboot-computer`  -  Reboots Veeam backup agent on a discovered computer with the specified UID.
- `veeam-cli discovery reset-computers-by-rule`  -  Resets results of discovery by a discovery rule with the specified UID.
- `veeam-cli discovery schedule-install-backup-server-on-computer`  -  Creates a sheduled task that installs Veeam Backup & Replication and management agent on a discovered computer with the
- `veeam-cli discovery schedule-install-vone-server-on-computer`  -  Creates a sheduled task that installs Veeam ONE and management agent on a discovered computer with the specified UID.
- `veeam-cli discovery start-rule`  -  Run discovery by a discovery rule with the specified UID.
- `veeam-cli discovery start-scheduled-deployment-task-for-computer`  -  Starts a computer scheduled deployment task with the specified UID.
- `veeam-cli discovery stop-rule`  -  Stop discovery by a discovery rule with the specified UID.

**event-logs**  -  This resource collection represents event log entries.

- `veeam-cli event-logs get-activity-logs`  -  Returns a collection resource representation of all activity log records.
- `veeam-cli event-logs get-management-agent-tasks`  -  Returns a collection resource representation of all management agent task log records.

**infrastructure**  -  Manage infrastructure

- `veeam-cli infrastructure abort-backup-server-multipart-patch`  -  Aborts an upload of a patch to Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure abort-linux-backup-server-multipart-patch`  -  Aborts an upload of a patch to Veeam Backup & Replication linux server with the specified UID.
- `veeam-cli infrastructure abort-public-cloud-multipart-patch`  -  Aborts an upload of a patch to Veeam Backup for Public Clouds appliances registered on a Veeam Cloud Connect site with
- `veeam-cli infrastructure abort-vone-server-multipart-patch`  -  Aborts upload of a Veeam ONE patch to a server with the specified UID.
- `veeam-cli infrastructure accept-unverified-agent`  -  Accepts an unverified management agent with the specified UID.
- `veeam-cli infrastructure activate-backup-agent`  -  Changes management mode of a Veeam backup agent with the specified UID to ManagedByConsole.
- `veeam-cli infrastructure activate-vb365-server`  -  Activates an unactivated Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli infrastructure add-existing-public-cloud-aws-appliance`  -  Connects an existing Veeam Backup for AWS appliance registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure add-existing-public-cloud-azure-account`  -  Registers an existing Microsoft Azure on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure add-existing-public-cloud-azure-appliance`  -  Connect an existing Veeam Backup for Microsoft Azure appliance registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure add-existing-public-cloud-google-appliance`  -  Connects an existing Veeam Backup for Google Cloud appliance registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure assign-backup-server-job`  -  Assign a job with the specified UID to a company.
- `veeam-cli infrastructure assign-linux-policy-on-backup-agent`  -  Assigns a backup policy to a Veeam Agent for Linux with the specified UID.
- `veeam-cli infrastructure assign-mac-policy-on-backup-agent`  -  Assigns a backup policy to a Veeam Agent for Mac with the specified UID.
- `veeam-cli infrastructure assign-policy-on-backup-agent`  -  Assigns a backup policy to a Veeam Agent for Microsoft Windows with the specified UID.
- `veeam-cli infrastructure assign-tenant`  -  Assigns a cloud tenant with the specified UID to a company.
- `veeam-cli infrastructure check-vb365-microsoft365-organization-certificate-existence-in-the-storage`  -  Checks whether a security certificate is available for a Microsoft 365 organization application with the specified UID.
- `veeam-cli infrastructure complete-backup-server-multipart-patch`  -  Finalizes an upload of a patch to Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure complete-linux-backup-server-multipart-patch`  -  Finalizes an upload of a patch to Veeam Backup & Replication linux server with the specified UID.
- `veeam-cli infrastructure complete-public-cloud-multipart-patch`  -  Finalizes an upload of a patch to Veeam Backup for Public Clouds appliances registered on a Veeam Cloud Connect site
- `veeam-cli infrastructure complete-vone-server-multipart-patch`  -  Finalizes upload of a Veeam ONE patch to a server with the specified UID.
- `veeam-cli infrastructure create-azure-device-code`  -  Generates a device code to log in to a Microsoft organization.
- `veeam-cli infrastructure create-backup-server-backup-vm-vcd-job`  -  Creates a VMware Cloud Director VM backup job on a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-backup-server-backup-vm-vsphere-job`  -  Creates a VMware VSphere VM backup job on a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-backup-server-encryption-password`  -  Creates a new encryption password on a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-backup-server-linux-credentials`  -  Adds Linux credentials for a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-backup-server-multipart-patch`  -  Initiates an upload of a patch to a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-backup-server-standard-credentials`  -  Adds standard credentials for Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-backup-server-veeam-vault-repository`  -  Creates a Veeam Data Cloud Vault backup repository connected to a Veeam Backup & Replication server with the specified
- `veeam-cli infrastructure create-backup-server-win-local-repository`  -  Creates a Windows local backup repository connected to a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure create-linux-backup-agent-job-configuration`  -  Creates configuration of a Veeam backup agent job protecting Linux computer with the specified UID.
- `veeam-cli infrastructure create-linux-backup-server-multipart-patch`  -  Initiates an upload of a patch to a Veeam Backup & Replication linux server with the specified UID.
- `veeam-cli infrastructure create-mac-backup-agent-job-configuration`  -  Creates a configuration of a Veeam backup agent job protecting Mac computer with the specified UID.
- `veeam-cli infrastructure create-new-public-cloud-aws-appliance`  -  Creates a new Veeam Backup for AWS appliance registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-new-public-cloud-azure-appliance`  -  Creates a new Veeam Backup for Microsoft Azure appliance regitered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-new-public-cloud-google-appliance`  -  Creates a new Veeam Backup for Google Cloud appliance registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-aws-account`  -  Creates an AWS account in a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-aws-connection`  -  Adds Amazon connection in a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-aws-key`  -  Creates an Amazon encryption key.
- `veeam-cli infrastructure create-public-cloud-azure-account`  -  Creates a new Microsoft Azure account registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-azure-connection`  -  Creates a new Microsoft Azure connection registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-azure-key`  -  Creates a Microsoft Azure cryptographic key registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-google-account`  -  Creates a Google Cloud account in a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-public-cloud-guest-os-credentials`  -  Creates guest OS credentials for public cloud VMs.
- `veeam-cli infrastructure create-public-cloud-mapping`  -  Maps a Veeam Backup for Public Clouds appliance to an organization.
- `veeam-cli infrastructure create-public-cloud-multipart-patch`  -  Initiates an upload of a patch to Veeam Backup for Public Clouds appliances registered on a Veeam Cloud Connect site
- `veeam-cli infrastructure create-public-cloud-sql-account`  -  Creates a new public cloud SQL account.
- `veeam-cli infrastructure create-tenant`  -  Creates a new tenant on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure create-tenant-backup-resource`  -  Allocate a new cloud backup resource to a tenant with the specified UID.
- `veeam-cli infrastructure create-tenant-replication-resource`  -  Allocates a new cloud replication resource to a tenant.
- `veeam-cli infrastructure create-tenant-vcd-replication-resource`  -  Allocates a new VMware Cloud Director replication resource to a tenant with the specified UID.
- `veeam-cli infrastructure create-vb365-backup-job`  -  Creates a new Veeam Backup for Microsoft 365 backup job.
- `veeam-cli infrastructure create-vb365-copy-job`  -  Creates a new Veeam Backup for Microsoft 365 backup copy job.
- `veeam-cli infrastructure create-vb365-microsoft365-organization`  -  Creates a new Microsoft 365 organization on a Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli infrastructure create-vb365-organization-to-company-mapping`  -  Maps a Microsoft organization to a company.
- `veeam-cli infrastructure create-vone-server-multipart-patch`  -  Initiates upload of a Veeam ONE patch to a server with the specified UID.
- `veeam-cli infrastructure create-windows-backup-agent-job-configuration`  -  Creates a configuration of a Veeam backup agent job protecting a Windows computer with the specified UID.
- `veeam-cli infrastructure deactivate-backup-agent`  -  Changes management mode of Veeam backup agent with the specified UID to UnManaged and deletes management agent from the
- `veeam-cli infrastructure delete-backup-agent`  -  Deletes Veeam backup agent with the specified UID.
- `veeam-cli infrastructure delete-backup-server-credentials`  -  Deletes Veeam Backup & Replication server credentials record with the specified UID.
- `veeam-cli infrastructure delete-backup-server-encryption-password`  -  Deletes a Veeam Backup & Replication server encryption password with the specified UID.
- `veeam-cli infrastructure delete-backup-server-job`  -  Delete a job with the specified UID.
- `veeam-cli infrastructure delete-backup-server-repository`  -  Deletes a backup repository with the specified UID.
- `veeam-cli infrastructure delete-linux-backup-agent-job`  -  Deletes a Veeam Agent for Linux job with the specified UID.
- `veeam-cli infrastructure delete-mac-backup-agent-job`  -  Deletes a Veeam Agent for Mac job with the specified UID.
- `veeam-cli infrastructure delete-management-agent`  -  Deletes a management agent with the specified UID.
- `veeam-cli infrastructure delete-management-agent-credentials`  -  Deletes credentials for Veeam Backup & Replication and Veeam backup agent installation configured on a management agent
- `veeam-cli infrastructure delete-public-cloud-appliance`  -  Removes a Veeam Backup for Public Clouds appliance with the specified UID registered on a Veeam Cloud Connect site.
- `veeam-cli infrastructure delete-public-cloud-aws-connection`  -  Deletes Amazon connection with the specified UID.
- `veeam-cli infrastructure delete-public-cloud-azure-connection`  -  Deletes a Microsoft Azure connection with the specified UID.
- `veeam-cli infrastructure delete-public-cloud-policy`  -  Deletes a Veeam Backup for Public Clouds policy with the specified UID.
- `veeam-cli infrastructure delete-public-cloud-sql-account`  -  Deletes a public cloud SQL account with the specified ID.
- `veeam-cli infrastructure delete-scheduled-deployment-task-for-agent`  -  Deletes a management agent scheduled deployment task with the specified UID.
- `veeam-cli infrastructure delete-tenant`  -  Deletes a tenant with the specified UID.
- `veeam-cli infrastructure delete-tenant-backup-resource`  -  Deletes a tenant cloud backup resource with the specified UID.
- `veeam-cli infrastructure delete-vb365-job`  -  Deletes a Veeam Backup for Microsoft 365 job with the specified UID.
- `veeam-cli infrastructure delete-vb365-organization`  -  Deletes a Microsoft organization with the specified UID.
- `veeam-cli infrastructure delete-vb365-organization-to-company-mapping`  -  Deletes a Microsoft organization to company mapping with the specified UID.
- `veeam-cli infrastructure delete-vb365-server`  -  Removes a Veeam Backup for Microsoft 365 server with the specified UID from the Veeam Service Provider Console database.
- `veeam-cli infrastructure delete-vone-server`  -  Delete data of a Veeam ONE server with the specified UID from a Veeam Service Provider Console server.
- `veeam-cli infrastructure delete-windows-backup-agent-job`  -  Deletes a Veeam Agent for Microsoft Windows job with the specified UID.
- `veeam-cli infrastructure disable-public-cloud-policy`  -  Disables a Veeam Backup for Public Clouds policy with the specified UID.
- `veeam-cli infrastructure disable-tenant`  -  Disables a cloud tenant with the specified UID.
- `veeam-cli infrastructure disable-vb365-job`  -  Disables a Veeam Backup for Microsoft 365 job with the specified UID.
- `veeam-cli infrastructure discover-active-directory-tree`  -  Returns a collection resource representation of organizational units of an Active Directory infrastructure where a
- `veeam-cli infrastructure download-linux-management-package`  -  Returns a download link to a management agent setup file in the shell script format for Linux computers.
- `veeam-cli infrastructure download-mac-management-package`  -  Returns a download link to a management agent setup file in the shell script format for macOS computers.
- `veeam-cli infrastructure download-windows-management-package`  -  Returns a download link to a management agent setup file in the `EXE` or `MSI` format for Microsoft Windows computers.
- `veeam-cli infrastructure enable-public-cloud-policy`  -  Enables a Veeam Backup for Public Clouds policy with the specified UID.
- `veeam-cli infrastructure enable-tenant`  -  Enables a cloud tenant with the specified UID.
- `veeam-cli infrastructure enable-vb365-job`  -  Enables a Veeam Backup for Microsoft 365 job with the specified UID.
- `veeam-cli infrastructure expand-backup-server-cloud-director-object-containers`  -  Returns a collection resource representation of VMs in specified VMware Cloud Director containers.
- `veeam-cli infrastructure expand-backup-server-virtual-server-object-containers`  -  Returns a collection resource representation of VMs included in the specified VMware vSphere VM containers.
- `veeam-cli infrastructure force-collect-backup-agent`  -  Forces data collection from a Veeam backup agent with the specified UID.
- `veeam-cli infrastructure force-collect-backup-server`  -  Forces data collection from a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure force-collect-data-from-backup-server-public-cloud-appliance`  -  Initiates data collection from a Veeam Backup for Public Clouds appliance with the specified UID.
- `veeam-cli infrastructure force-collect-data-from-public-cloud-appliance`  -  Initiates data collection from a Veeam Backup for Public Clouds appliance with the specified UID registered on a Veeam
- `veeam-cli infrastructure force-collect-enterprise-manager`  -  Forces data collection from an Enterprise Manager server with the specified UID.
- `veeam-cli infrastructure force-collect-vb365-server`  -  Enforces data collection from a Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli infrastructure force-collect-vone-server`  -  Enforces data collection from a connected Veeam ONE server with the specified UID.
- `veeam-cli infrastructure get-all-backup-agent-assigned-policies`  -  Returns a collection resource representation of all backup policies assigned to Veeam Agents for Microsoft Windows.
- `veeam-cli infrastructure get-all-linux-backup-agent-assigned-policies`  -  Returns a collection resource representation of all backup policies assigned to Veeam Agents for Linux.
- `veeam-cli infrastructure get-all-mac-backup-agent-assigned-policies`  -  Returns a collection resource representation of all backup policies assigned to Veeam Agents for Mac.
- `veeam-cli infrastructure get-azure-device-code-status`  -  Returns a resource representation of a device code status.
- `veeam-cli infrastructure get-backup-agent`  -  Returns a resource representation of a Veeam backup agent with the specified UID.
- `veeam-cli infrastructure get-backup-agent-assigned-policies`  -  Returns a collection resource representation of all backup policies assigned to Veeam Agent for Microsoft Windows with
- `veeam-cli infrastructure get-backup-agent-jobs`  -  Returns a collection resource representation of all Veeam backup agent jobs protecting Windows computers.
- `veeam-cli infrastructure get-backup-agent-settings`  -  Returns a resource representation of settings configured for Veeam Agent for Microsoft Windows with the specified UID.
- `veeam-cli infrastructure get-backup-agents`  -  Returns a collection resource representation of all Veeam backup agents.
- `veeam-cli infrastructure get-backup-agents-settings`  -  Returns a collection resource representation of settings configured for all Veeam backup agents installed on Windows
- `veeam-cli infrastructure get-backup-failover-plan`  -  Returns a resource representation of a failover plan with the specified UID.
- `veeam-cli infrastructure get-backup-failover-plan-objects`  -  Returns a collection resource representation of all VMs included in a failover plan with the specified UID.
- `veeam-cli infrastructure get-backup-failover-plans`  -  Returns a collection resource representation of all failover plans.
- `veeam-cli infrastructure get-backup-failover-plans-by-server`  -  Returns a collection resource representation of all failover plans configured on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-failover-plans-objects`  -  Returns a collection resource representation of VMs included in all failover plans.
- `veeam-cli infrastructure get-backup-failover-plans-objects-by-server`  -  Returns a collection resource representation of VMs included in all failover plans configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-hardware-plan`  -  Returns a resource representation of a hardware plan with the specified UID.
- `veeam-cli infrastructure get-backup-hardware-plan-storages`  -  Returns a collection resource representation of all storage entities in a hardware plan with the specified UID.
- `veeam-cli infrastructure get-backup-hardware-plans`  -  Returns a collection resource representation of all hardware plans.
- `veeam-cli infrastructure get-backup-hardware-plans-by-site`  -  Returns a collection resource representation of all hardware plans configured on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-backup-hardware-plans-storages`  -  Returns a collection resource representation of all storage entities in all hardware plans.
- `veeam-cli infrastructure get-backup-proxies`  -  Returns a collection resource representation of all backup proxies.
- `veeam-cli infrastructure get-backup-proxies-by-server`  -  Returns a collection resource representation of all backup proxies connected to a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-proxy`  -  Returns a resource representation of a backup proxy with the specified UID.
- `veeam-cli infrastructure get-backup-repositories`  -  Returns a collection resource representation of all backup repositories.
- `veeam-cli infrastructure get-backup-repositories-by-server`  -  Returns a collection resource representation of all backup repositories connected to a Veeam Backup & Replication
- `veeam-cli infrastructure get-backup-repository`  -  Returns a resource representation of a backup repository with the specified UID.
- `veeam-cli infrastructure get-backup-server`  -  Returns a resource representation of a Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure get-backup-server-agent`  -  Returns a resource representation of a Veeam backup agent with the specified UID managed by a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-agent-job`  -  Returns a resource representation of a backup agent job with the specified UID.
- `veeam-cli infrastructure get-backup-server-agent-job-objects`  -  Returns a collection resource representation of all objects of a backup agent job with the specified UID.
- `veeam-cli infrastructure get-backup-server-agent-jobs`  -  Returns a collection resource representation of all backup agent jobs.
- `veeam-cli infrastructure get-backup-server-agent-jobs-by-server`  -  Returns a collection resource representation of all backup agent jobs configured on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-server-agent-jobs-objects`  -  Returns a collection resource representation of all backup agent job objects.
- `veeam-cli infrastructure get-backup-server-agent-protection-groups`  -  Returns a collection resource representation of all protection groups configured on managed Veeam Backup & Replication
- `veeam-cli infrastructure get-backup-server-agent-protection-groups-by-server`  -  Returns a collection resource representation of all protection groups configured on a managed Veeam Backup &
- `veeam-cli infrastructure get-backup-server-agents-by-server`  -  Returns a collection resource representation of all Veeam backup agents managed by a connected Veeam Backup &
- `veeam-cli infrastructure get-backup-server-backup-copy-job`  -  Returns a resource representation of a periodic backup copy or legacy periodic backup copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-backup-copy-job-linked-job-objects`  -  Returns a collection resource representation of all objects of a periodic backup copy or legacy periodic backup copy
- `veeam-cli infrastructure get-backup-server-backup-copy-jobs`  -  Returns a collection resource representation of all periodic backup copy and legacy periodic backup copy jobs.
- `veeam-cli infrastructure get-backup-server-backup-copy-jobs-by-server`  -  Returns a collection resource representation of all periodic backup copy and legacy periodic backup copy jobs
- `veeam-cli infrastructure get-backup-server-backup-copy-jobs-linked-job-objects`  -  Returns a collection resource representation of objects of all periodic backup copy and legacy periodic backup copy
- `veeam-cli infrastructure get-backup-server-backup-copy-jobs-linked-job-objects-by-server`  -  Returns a collection resource representation of objects of all periodic backup copy and legacy periodic backup copy
- `veeam-cli infrastructure get-backup-server-backup-tape-job`  -  Returns a resource representation of a backup to tape job with the specified UID.
- `veeam-cli infrastructure get-backup-server-backup-tape-job-linked-job-objects`  -  Returns a collection resource representation of all jobs processed by a backup to tape job with the specified UID.
- `veeam-cli infrastructure get-backup-server-backup-tape-job-linked-repository-objects`  -  Returns a collection resource representation of all repositories processed by a backup to tape job with the specified
- `veeam-cli infrastructure get-backup-server-backup-tape-jobs`  -  Returns a collection resource representation of all backup to tape jobs.
- `veeam-cli infrastructure get-backup-server-backup-tape-jobs-by-server`  -  Returns a collection resource representation of all backup to tape jobs configured on a Veeam Backup & Replication
- `veeam-cli infrastructure get-backup-server-backup-tape-jobs-linked-job-objects`  -  Returns a collection resource representation of all jobs processed by backup to tape jobs.
- `veeam-cli infrastructure get-backup-server-backup-tape-jobs-linked-job-objects-by-server`  -  Returns a collection resource representation of all jobs that are processed by backup to tape jobs configured on a
- `veeam-cli infrastructure get-backup-server-backup-tape-jobs-linked-repository-objects`  -  Returns a collection resource representation of all repositories processed by backup to tape jobs.
- `veeam-cli infrastructure get-backup-server-backup-tape-jobs-linked-repository-objects-by-server`  -  Returns a collection resource representation of all repositories that are processed by backup to tape jobs configured
- `veeam-cli infrastructure get-backup-server-backup-vm-job`  -  Returns a resource representation of a backup job with the specified UID that protects VMs.
- `veeam-cli infrastructure get-backup-server-backup-vm-job-objects`  -  Returns a collection resource representation of all objects of a VM backup job with the specified UID.
- `veeam-cli infrastructure get-backup-server-backup-vm-jobs`  -  Returns a collection resource representation of all backup jobs that protect VMs.
- `veeam-cli infrastructure get-backup-server-backup-vm-jobs-by-server`  -  Returns a collection resource representation of all VM backup jobs configured on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-server-backup-vm-jobs-objects`  -  Returns a collection resource representation of all VM backup job objects.
- `veeam-cli infrastructure get-backup-server-backup-vm-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of VM backup jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-backup-vm-vcd-job-configuration`  -  Returns a resource representation of a configuration of a VMware Cloud Director VM backup job with the specified UID.
- `veeam-cli infrastructure get-backup-server-backup-vm-vsphere-job-configuration`  -  Returns a resource representation of a configuration of a VMware vSphere VM backup job with the specified UID.
- `veeam-cli infrastructure get-backup-server-cdp-replication-job`  -  Returns a resource representation of a CDP replication job with the specified UID.
- `veeam-cli infrastructure get-backup-server-cdp-replication-job-objects`  -  Returns a collection resource representation of all objects of a CDP replication job with the specified UID.
- `veeam-cli infrastructure get-backup-server-cdp-replication-jobs`  -  Returns a collection resource representation of all CDP replication jobs.
- `veeam-cli infrastructure get-backup-server-cdp-replication-jobs-by-server`  -  Returns a collection resource representation of all CDP replication jobs configured on a Veeam Backup & Replication
- `veeam-cli infrastructure get-backup-server-cdp-replication-jobs-objects`  -  Returns a collection resource representation of all CDP replication job objects.
- `veeam-cli infrastructure get-backup-server-cdp-replication-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of CDP replication jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-credentials-by-server`  -  Returns a collection resource representation of all credentials stored on Veeam Backup & Replication server with the
- `veeam-cli infrastructure get-backup-server-encryption-passwords-by-server`  -  Returns a collection resource representation of all encryption passwords created on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-server-file-copy-job`  -  Returns a resource representation of a file copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-copy-jobs`  -  Returns a collection resource representation of all file copy jobs.
- `veeam-cli infrastructure get-backup-server-file-copy-jobs-by-server`  -  Returns a collection resource representation of all file copy jobs configured on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-server-file-share-copy-job`  -  Returns a resource representation of a file share backup copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-share-copy-job-objects`  -  Returns a collection resource representation of all objects of a file share backup copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-share-copy-jobs`  -  Returns a collection resource representation of all backup copy jobs for file shares.
- `veeam-cli infrastructure get-backup-server-file-share-copy-jobs-by-server`  -  Returns a collection resource representation of all file share backup copy jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-file-share-copy-jobs-objects`  -  Returns a collection resource representation of all file share backup copy job objects.
- `veeam-cli infrastructure get-backup-server-file-share-copy-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of file share backup copy jobs configured on a Veeam Backup
- `veeam-cli infrastructure get-backup-server-file-share-job`  -  Returns a resource representation of a file share job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-share-job-objects`  -  Returns a collection resource representation of all objects of a file share job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-share-jobs`  -  Returns a collection resource representation of all file share jobs.
- `veeam-cli infrastructure get-backup-server-file-share-jobs-by-server`  -  Returns a collection resource representation of all file share jobs configured on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-server-file-share-jobs-objects`  -  Returns a collection resource representation of all file share job objects.
- `veeam-cli infrastructure get-backup-server-file-share-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of file share jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-file-tape-job`  -  Returns a resource representation of a file to tape job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-tape-job-objects`  -  Returns a collection resource representation of all objects of a file to tape job with the specified UID.
- `veeam-cli infrastructure get-backup-server-file-tape-jobs`  -  Returns a collection resource representation of all file to tape jobs.
- `veeam-cli infrastructure get-backup-server-file-tape-jobs-by-server`  -  Returns a collection resource representation of all file to tape jobs configured on a Veeam Backup & Replication server
- `veeam-cli infrastructure get-backup-server-file-tape-jobs-objects`  -  Returns a collection resource representation of all file to tape job objects.
- `veeam-cli infrastructure get-backup-server-file-tape-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of file to tape jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-host`  -  Returns a resource representation of a server with the specified UID connected to a Veeam Backup & Replication server.
- `veeam-cli infrastructure get-backup-server-hosts`  -  Returns a collection resource representation of all servers connected to all Veeam Backup & Replication servers.
- `veeam-cli infrastructure get-backup-server-hosts-by-server`  -  Returns a collection resource representation of all servers connected to a Veeam Backup & Replication server with the
- `veeam-cli infrastructure get-backup-server-job`  -  Returns a resource representation of a job with the specified UID.
- `veeam-cli infrastructure get-backup-server-job-by-server`  -  Returns a collection resource representation of all jobs configured on a Veeam Backup & Replication server with the
- `veeam-cli infrastructure get-backup-server-jobs`  -  Returns a collection resource representation of all jobs on all Veeam Backup & Replication servers.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-copy-job`  -  Returns a resource representation of a object storage backup backup copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-copy-job-objects`  -  Returns a collection resource representation of all objects of a object storage backup backup copy job with the
- `veeam-cli infrastructure get-backup-server-object-storage-backup-copy-jobs`  -  Returns a collection resource representation of all backup copy jobs for object storage backups.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-copy-jobs-by-server`  -  Returns a collection resource representation of all object storage backup backup copy jobs configured on a Veeam Backup
- `veeam-cli infrastructure get-backup-server-object-storage-backup-copy-jobs-objects`  -  Returns a collection resource representation of all object storage backup backup copy job objects.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-copy-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of object storage backup backup copy jobs configured on a
- `veeam-cli infrastructure get-backup-server-object-storage-backup-job`  -  Returns a resource representation of a object storage backup job with the specified UID.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-job-objects`  -  Returns a collection resource representation of all objects of a object storage backup job with the specified UID.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-jobs`  -  Returns a collection resource representation of all object storage backup jobs.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-jobs-by-server`  -  Returns a collection resource representation of all object storage backup jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-object-storage-backup-jobs-objects`  -  Returns a collection resource representation of all object storage backup job objects.
- `veeam-cli infrastructure get-backup-server-object-storage-backup-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of object storage backup jobs configured on a Veeam Backup
- `veeam-cli infrastructure get-backup-server-public-cloud-appliance`  -  Returns a resource representation of a Veeam Backup for Public Clouds appliance with the specified UID.
- `veeam-cli infrastructure get-backup-server-public-cloud-appliances`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds appliances.
- `veeam-cli infrastructure get-backup-server-public-cloud-appliances-by-server`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds appliances connected to a Veeam
- `veeam-cli infrastructure get-backup-server-replication-vm-job`  -  Returns a resource representation of a replication job with the specified UID that protects VMs.
- `veeam-cli infrastructure get-backup-server-replication-vm-job-objects`  -  Returns a collection resource representation of all objects of a VM replication job with the specified UID.
- `veeam-cli infrastructure get-backup-server-replication-vm-jobs`  -  Returns a resource representation of all replication jobs that protects VMs.
- `veeam-cli infrastructure get-backup-server-replication-vm-jobs-by-server`  -  Returns a collection resource representation of all VM replication jobs configured on a Veeam Backup & Replication
- `veeam-cli infrastructure get-backup-server-replication-vm-jobs-objects`  -  Returns a collection resource representation of all VM replication job objects.
- `veeam-cli infrastructure get-backup-server-replication-vm-jobs-objects-by-server`  -  Returns a collection resource representation of all objects of VM replication jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-simple-backup-copy-job`  -  Returns a resource representation of an immediate backup copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-simple-backup-copy-job-linked-job-objects`  -  Returns a collection resource representation of all objects of an immediate backup copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-simple-backup-copy-jobs`  -  Returns a collection resource representation of all immediate backup copy jobs.
- `veeam-cli infrastructure get-backup-server-simple-backup-copy-jobs-by-server`  -  Returns a collection resource representation of all immediate backup copy jobs configured on a Veeam Backup &
- `veeam-cli infrastructure get-backup-server-simple-backup-copy-jobs-linked-job-objects`  -  Returns a collection resource representation of all immediate backup copy job objects.
- `veeam-cli infrastructure get-backup-server-simple-backup-copy-jobs-linked-job-objects-by-server`  -  Returns a collection resource representation of all objects of immediate backup copy jobs configured on a Veeam Backup
- `veeam-cli infrastructure get-backup-server-veeam-vault-repository`  -  Returns a resource representation of a Veeam Data Cloud Vault backup repository with the specified UID.
- `veeam-cli infrastructure get-backup-server-virtual-server-tag-virtual-machines`  -  Returns a resource representation of all VMs marked with a vCenter Server tag with the specified URN.
- `veeam-cli infrastructure get-backup-server-virtual-server-tags`  -  Returns a collection resource representation of tags collected from a vCenter Server with the specified UID connected
- `veeam-cli infrastructure get-backup-server-vm-copy-job`  -  Returns a resource representation of a VM copy job with the specified UID.
- `veeam-cli infrastructure get-backup-server-vm-copy-jobs`  -  Returns a collection resource representation of all VM copy jobs.
- `veeam-cli infrastructure get-backup-server-vm-copy-jobs-by-server`  -  Returns a collection resource representation of all VM copy jobs configured on a Veeam Backup & Replication server with
- `veeam-cli infrastructure get-backup-server-win-local-repository`  -  Returns a resource representation of a Windows local backup repository with the specified UID.
- `veeam-cli infrastructure get-backup-servers`  -  Returns a collection resource representation of all Veeam Backup & Replication servers.
- `veeam-cli infrastructure get-backup-servers-agents`  -  Returns a collection resource representation of all Veeam backup agents managed by connected Veeam Backup & Replication
- `veeam-cli infrastructure get-backup-servers-by-enterprise-manager`  -  Returns a collection resource representation of all Veeam Backup & Replication servers that are managed by a Veeam
- `veeam-cli infrastructure get-backup-wan-accelerator`  -  Returns a resource representation of a WAN accelerator with the specified UID.
- `veeam-cli infrastructure get-backup-wan-accelerators`  -  Returns a collection resource representation of all WAN accelerators.
- `veeam-cli infrastructure get-backup-wan-accelerators-by-server`  -  Returns a collection resource representation of all WAN accelerators connected to a Veeam Backup & Replication server
- `veeam-cli infrastructure get-cloud-backup`  -  Returns a resource representation of a backup with the specified UID created by a Veeam Cloud Connect site.
- `veeam-cli infrastructure get-cloud-backups`  -  Returns a collection resource representation of backups created by all Veeam Cloud Connect sites.
- `veeam-cli infrastructure get-cloud-backups-by-site`  -  Returns a collection resource representation of all backups created by a Veeam Cloud Connect site with the specified
- `veeam-cli infrastructure get-cloud-gateway`  -  Returns a resource representation of a cloud gateway with the specified UID.
- `veeam-cli infrastructure get-cloud-gateway-pool`  -  Returns a resource representation of a cloud gateway pool with the specified UID.
- `veeam-cli infrastructure get-cloud-gateway-pools`  -  Returns a collection resource representation of all cloud gateway pools.
- `veeam-cli infrastructure get-cloud-gateway-pools-by-site`  -  Returns a collection resource representation of all cloud gateway pools configured for a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-cloud-gateway-pools-by-tenant`  -  Returns a collection resource representation of all cloud gateway pools assigned to a tenant with the specified UID.
- `veeam-cli infrastructure get-cloud-gateways`  -  Returns a collection resource representation of all cloud gateways.
- `veeam-cli infrastructure get-cloud-gateways-by-pool`  -  Returns a collection resource representation of all cloud gateways included in a pool with the specified UID.
- `veeam-cli infrastructure get-cloud-gateways-by-site`  -  Returns a collection resource representation of all cloud gateways configured for a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-cloud-tenants-products`  -  Returns a collection resource representation of tenant Veeam products that generated cloud data the latest.
- `veeam-cli infrastructure get-cloud-tenants-products-by-site`  -  Returns a collection resource representation of tenant Veeam products that generated the latest cloud data managed by a
- `veeam-cli infrastructure get-default-mount-server`  -  Returns a collection resource representation of all default mount servers managed by a Veeam Backup & Replication
- `veeam-cli infrastructure get-enterprise-manager`  -  Returns a resource representation of a Veeam Backup Enterprise Manager server with the specified UID.
- `veeam-cli infrastructure get-enterprise-managers`  -  Returns a collection resource representation of all Veeam Backup Enterprise Manager servers.
- `veeam-cli infrastructure get-hosted-agent-linux`  -  Returns a resource representation of parameters for management agent deployment on hosted Linux computers.
- `veeam-cli infrastructure get-job-session-heatmap`  -  Returns a resource representation of the Session States dashboard.
- `veeam-cli infrastructure get-linux-backup-agent`  -  Returns a resource representation of a Veeam Agent for Linux with the specified UID.
- `veeam-cli infrastructure get-linux-backup-agent-assigned-policies`  -  Returns a collection resource representation of all backup policies assigned to a Veeam Agent for Linux with the
- `veeam-cli infrastructure get-linux-backup-agent-job`  -  Returns a resource representation of a Veeam Agent for Linux job with the specified UID.
- `veeam-cli infrastructure get-linux-backup-agent-job-configuration`  -  Returns a resource representation of a configuration of a Veeam Agent for Linux job with the specified UID.
- `veeam-cli infrastructure get-linux-backup-agent-jobs`  -  Returns a collection resource representation of all Veeam Agent jobs protecting Linux computers.
- `veeam-cli infrastructure get-linux-backup-agent-jobs-by-agent`  -  Returns a collection resource representation of all jobs configured for a Veeam Agent for Linux with the specified UID.
- `veeam-cli infrastructure get-linux-backup-agents`  -  Returns a collection resource representation of all Veeam Agents for Linux.
- `veeam-cli infrastructure get-mac-backup-agent`  -  Returns a resource representation of a Veeam Agent for Mac with the specified UID.
- `veeam-cli infrastructure get-mac-backup-agent-assigned-policies`  -  Returns a collection resource representation of all backup policies assigned to a Veeam Agent for Mac with the
- `veeam-cli infrastructure get-mac-backup-agent-job`  -  Returns a resource representation of a Veeam Agent for Mac job with the specified UID.
- `veeam-cli infrastructure get-mac-backup-agent-job-configuration`  -  Returns a resource representation of a configuration of a Veeam Agent for Mac job with the specified UID.
- `veeam-cli infrastructure get-mac-backup-agent-jobs`  -  Returns a collection resource representation of all Veeam Agent for Mac jobs.
- `veeam-cli infrastructure get-mac-backup-agent-jobs-by-agent`  -  Returns a collection resource representation of all jobs configured for Veeam Agent for Mac with the specified UID.
- `veeam-cli infrastructure get-mac-backup-agents`  -  Returns a collection resource representation of all Veeam Agents for Mac.
- `veeam-cli infrastructure get-management-agent`  -  Returns a resource representation of a management agent with the specified UID.
- `veeam-cli infrastructure get-management-agent-credentials`  -  Get a resource representation of credentials for Veeam Backup & Replication and Veeam backup agent installation
- `veeam-cli infrastructure get-management-agent-installed-proxyable-products`  -  Returns a collection resource representation of Veeam products with available request proxying installed on the server
- `veeam-cli infrastructure get-management-agents`  -  Returns a collection resource representation of all management agents.
- `veeam-cli infrastructure get-mount-server`  -  Returns a resource representation of a mount server with the specified UID.
- `veeam-cli infrastructure get-mount-servers`  -  Returns a collection resource representation of all mount servers managed by a Veeam Backup & Replication server with
- `veeam-cli infrastructure get-public-cloud-appliance`  -  Returns a resource representation of a Veeam Backup for Public Clouds appliance with the specified UID registered on a
- `veeam-cli infrastructure get-public-cloud-appliance-certificate`  -  Returns a resource representation of a security certificate of a Veeam Backup for Public Clouds appliance with the
- `veeam-cli infrastructure get-public-cloud-appliances`  -  Returns a collection resource representation of Veeam Backup for Public Clouds appliances registered on all Veeam Cloud
- `veeam-cli infrastructure get-public-cloud-appliances-by-server`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds appliances registered on a Veeam
- `veeam-cli infrastructure get-public-cloud-appliances-for-sql-accounts`  -  Returns a collection of resource representation of all Veeam Backup for Public Clouds appliances on which SQL accounts
- `veeam-cli infrastructure get-public-cloud-aws-account`  -  Returns a resource representation of an AWS account with the specified UID.
- `veeam-cli infrastructure get-public-cloud-aws-accounts`  -  Returns a collection resource representation of all AWS accounts managed by a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-public-cloud-aws-appliance`  -  Returns a resource representation of an Veeam Backup for AWS appliance with the specified UID.
- `veeam-cli infrastructure get-public-cloud-aws-backup-server-ip-addresses`  -  Returns a collection resource representation of IP addresses assigned in AWS to a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-public-cloud-aws-data-centers`  -  Returns a collection resource representation of all AWS datacenters registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-public-cloud-aws-elastic-ip-addresses`  -  Returns a collection resource representation of all Elastic IP addresses available to AWS accounts registered on a
- `veeam-cli infrastructure get-public-cloud-aws-keys`  -  Returns a collection resource representation of all Amazon encryption keys.
- `veeam-cli infrastructure get-public-cloud-aws-networks`  -  Returns a collection resource representation of all AWS networks registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-public-cloud-aws-regions`  -  Returns a collection resource representation of all AWS regions.
- `veeam-cli infrastructure get-public-cloud-aws-security-groups`  -  Returns a collection resource representation of all AWS security groups registered on a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-aws-sub-networks`  -  Returns a collection resource representation of all AWS subnets registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-public-cloud-aws-virtual-machines`  -  Returns a collection resource representation of all AWS VMs registered on a Veeam Cloud Connect site with the specified
- `veeam-cli infrastructure get-public-cloud-azure-account`  -  Returns a resource representation of a Microsoft Azure account with the specified UID.
- `veeam-cli infrastructure get-public-cloud-azure-accounts`  -  Returns a collection resource representation of all Microsoft Azure accounts registered on a Veeam Cloud Connect site
- `veeam-cli infrastructure get-public-cloud-azure-appliance`  -  Returns a resource representation of a Veeam Backup for Microsoft Azure appliance with the specified UID.
- `veeam-cli infrastructure get-public-cloud-azure-backup-server-ip-addresses`  -  Returns a resource representation of all IP addresses assigned in Microsoft Azure to a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-azure-data-centers`  -  Returns a collection resource representation of all Microsoft Azure datacenters registered on a Veeam Cloud Connect
- `veeam-cli infrastructure get-public-cloud-azure-device-code`  -  Returns a resource representation of a Microsoft Azure device authentication code.
- `veeam-cli infrastructure get-public-cloud-azure-existing-ip-addresses`  -  Returns a collection resource representation of all existing IP addresses available to Microsoft Azure accounts
- `veeam-cli infrastructure get-public-cloud-azure-keys`  -  Returns a collection resource representation of all Microsoft Azure cryptographic keys registered on a Veeam Cloud
- `veeam-cli infrastructure get-public-cloud-azure-networks`  -  Returns a collection resource representation of all Microsoft Azure virtual networks registered on a Veeam Cloud
- `veeam-cli infrastructure get-public-cloud-azure-resource-groups`  -  Returns a collection resource representation of all Microsoft Azure resource groups registered on a Veeam Cloud Connect
- `veeam-cli infrastructure get-public-cloud-azure-security-groups`  -  Returns a collection resource representation of all Microsoft Azure network security groups registered on a Veeam Cloud
- `veeam-cli infrastructure get-public-cloud-azure-subscriptions`  -  Returns a collection resource representation of all Microsoft Azure subscriptions registered on a Veeam Cloud Connect
- `veeam-cli infrastructure get-public-cloud-azure-virtual-machines`  -  Returns a collection resource representation of all Microsoft Azure VMs registered on a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-database-policies`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds database policies.
- `veeam-cli infrastructure get-public-cloud-database-policies-by-server`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds database policies configured on a
- `veeam-cli infrastructure get-public-cloud-database-policies-objects`  -  Returns a collection resource representation of objects of all Veeam Backup for Public Clouds database policies.
- `veeam-cli infrastructure get-public-cloud-database-policies-objects-by-server`  -  Returns a collection resource representation of objects of all Veeam Backup for Public Clouds database policies
- `veeam-cli infrastructure get-public-cloud-database-policy`  -  Returns a resource representation of a Veeam Backup for Public Clouds database policy with the specified UID.
- `veeam-cli infrastructure get-public-cloud-database-policy-objects`  -  Returns a collection resource representation of all objects of a Veeam Backup for Public Clouds database policy with
- `veeam-cli infrastructure get-public-cloud-file-share-policies`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds file share policies.
- `veeam-cli infrastructure get-public-cloud-file-share-policies-by-server`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds file share policies configured on a
- `veeam-cli infrastructure get-public-cloud-file-share-policies-objects`  -  Returns a collection resource representation of objects of all Veeam Backup for Public Clouds file share policies.
- `veeam-cli infrastructure get-public-cloud-file-share-policies-objects-by-server`  -  Returns a collection resource representation of objects of all Veeam Backup for Public Clouds file share policies
- `veeam-cli infrastructure get-public-cloud-file-share-policy`  -  Returns a resource representation of a Veeam Backup for Public Clouds file share policy.
- `veeam-cli infrastructure get-public-cloud-file-share-policy-objects`  -  Returns a collection resource representation of all objects of a Veeam Backup for Public Clouds file share policy with
- `veeam-cli infrastructure get-public-cloud-google-account`  -  Returns a resource representation of a Google Cloud account with the specified UID.
- `veeam-cli infrastructure get-public-cloud-google-accounts`  -  Returns a collection resource representation of all Google Cloud accounts managed by a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-google-appliance`  -  Returns a resource representation of a Veeam Backup for Google Cloud appliance with the specified UID.
- `veeam-cli infrastructure get-public-cloud-google-availability-zones`  -  Returns a collection resource representation of all Google Cloud zones.
- `veeam-cli infrastructure get-public-cloud-google-backup-server-ip-addresses`  -  Returns a collection resource representation of IP addresses specified in Google Cloud for a Veeam Cloud Connect site
- `veeam-cli infrastructure get-public-cloud-google-data-centers`  -  Returns a collection resource representation of all Google Cloud datacenters managed by Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-google-existing-ip-addresses`  -  Returns a collection resource representation of all existing IP addresses available to Google Cloud accounts registered
- `veeam-cli infrastructure get-public-cloud-google-network-tags`  -  Returns a collection resource representation of all Google Cloud network tags registered on a Veeam Cloud Connect site
- `veeam-cli infrastructure get-public-cloud-google-networks`  -  Returns a collection resource representation of all Google Cloud networks managed by a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-google-sub-networks`  -  Returns a collection resource representation of all Google Cloud subnets registered on a Veeam Cloud Connect site with
- `veeam-cli infrastructure get-public-cloud-google-virtual-machines`  -  Returns a collection resource representation of all Google Cloud VMs registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-public-cloud-guest-os-credentials`  -  Returns a collection resource representation of all guest OS credentials for public cloud VMs.
- `veeam-cli infrastructure get-public-cloud-guest-os-credentials-by-id`  -  Returns a resource representation of public cloud guest OS credentials record with the specified UID.
- `veeam-cli infrastructure get-public-cloud-mapping`  -  Returns a resource representation of organization mapping of a Veeam Backup for Public Clouds appliance with the
- `veeam-cli infrastructure get-public-cloud-network-policies`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds network policies.
- `veeam-cli infrastructure get-public-cloud-network-policies-by-server`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds network policies configured on a
- `veeam-cli infrastructure get-public-cloud-network-policy`  -  Returns a resource representation of a Veeam Backup for Public Clouds network policy with the specified UID.
- `veeam-cli infrastructure get-public-cloud-policies`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds policies.
- `veeam-cli infrastructure get-public-cloud-policy`  -  Returns a resource representation of a Veeam Backup for Public Clouds policy with the specified UID.
- `veeam-cli infrastructure get-public-cloud-repositories`  -  Returns a collection resource representation of all public cloud repositories.
- `veeam-cli infrastructure get-public-cloud-repositories-by-appliance-uid`  -  Returns a collection resource representation of repositories created by a Veeam Backup for Public Clouds appliance with
- `veeam-cli infrastructure get-public-cloud-sql-account`  -  Returns a resource representation of a public cloud SQL account with the specified ID.
- `veeam-cli infrastructure get-public-cloud-sql-accounts`  -  Returns a collection resource representation of all available public cloud SQL accounts.
- `veeam-cli infrastructure get-public-cloud-time-zones`  -  Returns a collection resource representation of all time zones of public cloud VMs registered on a Veeam Cloud Connect
- `veeam-cli infrastructure get-public-cloud-virtual-machine-policies`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds VM policies.
- `veeam-cli infrastructure get-public-cloud-virtual-machine-policies-by-server`  -  Returns a collection resource representation of all Veeam Backup for Public Clouds VM policies configured on a Veeam
- `veeam-cli infrastructure get-public-cloud-virtual-machine-policies-objects`  -  Returns a collection resource representation of objects of all Veeam Backup for Public Clouds VM policies.
- `veeam-cli infrastructure get-public-cloud-virtual-machine-policies-objects-by-server`  -  Returns a collection resource representation of objects of all Veeam Backup for Public Clouds VM policies configured on
- `veeam-cli infrastructure get-public-cloud-virtual-machine-policy`  -  Returns a resource representation of a Veeam Backup for Public Clouds VM policy with the specified UID.
- `veeam-cli infrastructure get-public-cloud-virtual-machine-policy-objects`  -  Returns a collection resource representation of all objects of a Veeam Backup for Public Clouds VM policy with the
- `veeam-cli infrastructure get-scheduled-deployment-task-for-agent`  -  Returns a resource representation of a management agent scheduled deployment task with the specified UID.
- `veeam-cli infrastructure get-scheduled-deployment-tasks-for-agent`  -  Returns a collection resource representation of all deployment tasks scheduled for a management agent with the
- `veeam-cli infrastructure get-site`  -  Returns a resource representation of a cloud agent installed on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure get-site-vcd-organization`  -  Returns a resource representation of a VMware Cloud Director organization with the specified UID managed by Veeam Cloud
- `veeam-cli infrastructure get-site-vcd-organization-users-by-organization`  -  Returns a collection resource representation of all users of a VMware Cloud Director organization with the specified
- `veeam-cli infrastructure get-site-vcd-organization-vdc`  -  Returns a resource representation of an organization VDC with the specified UID managed by a Veeam Cloud Connect site.
- `veeam-cli infrastructure get-site-vcd-server`  -  Returns a resource representation of a VMware Cloud Director server with the specified UID managed by a Veeam Cloud
- `veeam-cli infrastructure get-site-wan-accelerator-resource`  -  Returns a resource representation of a WAN accelerator with the specified UID.
- `veeam-cli infrastructure get-site-wan-accelerator-resources`  -  Returns a collection resource representation of all WAN accelerators.
- `veeam-cli infrastructure get-site-wan-accelerator-resources-by-site`  -  Returns a collection resource representation of all WAN accelerators configured for a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-sites`  -  Returns a collection resource representation of all cloud agents installed on sites.
- `veeam-cli infrastructure get-sites-vcd-organization-users`  -  Returns a collection resource representation of users of VMware Cloud Director organizations managed by all Veeam Cloud
- `veeam-cli infrastructure get-sites-vcd-organization-vdcs`  -  Returns a collection resource representation of organization VDCs managed by all Veeam Cloud Connect Sites.
- `veeam-cli infrastructure get-sites-vcd-organization-vdcs-by-organization`  -  Returns a collection resource representation of all VDCs that provide resources to a VMware Cloud Director organization
- `veeam-cli infrastructure get-sites-vcd-organizations`  -  Returns a collection resource representation of VMware Cloud Director organizations managed by all Veeam Cloud Connect
- `veeam-cli infrastructure get-sites-vcd-organizations-by-vcd`  -  Returns a collection resource representation of all VMware Cloud Director organizations configured on a VMware Cloud
- `veeam-cli infrastructure get-sites-vcd-servers`  -  Returns a collection resource representation of VMware Cloud Director servers managed by all Veeam Cloud Connect Sites.
- `veeam-cli infrastructure get-sub-tenant`  -  Returns a resource representation of a subtenant with the specified UID.
- `veeam-cli infrastructure get-sub-tenants`  -  Returns a collection resource representation of all subtenants.
- `veeam-cli infrastructure get-sub-tenants-by-site`  -  Returns a collection resource representation of all subtenants provided with resources of a Veeam Cloud Connect site
- `veeam-cli infrastructure get-sub-tenants-by-tenant`  -  Returns a collection resource representation of all subtenants registered by a tenant with the specified UID.
- `veeam-cli infrastructure get-tenant`  -  Returns a resource representation of a tenant with the specified UID.
- `veeam-cli infrastructure get-tenant-backup-resource`  -  Returns a resource representation of tenant cloud backup resource with the specified UID.
- `veeam-cli infrastructure get-tenant-backup-resources`  -  Returns a collection resource representation of all cloud backup resources allocated to a tenant with the specified UID.
- `veeam-cli infrastructure get-tenant-backup-resources-usage`  -  Returns a resource representation of cloud backup resource usage by a tenant with the specified UID.
- `veeam-cli infrastructure get-tenant-replication-resource-hardware-plan`  -  Returns a resource representation of a tenant hardware plan with the specified UID.
- `veeam-cli infrastructure get-tenant-replication-resources`  -  Returns a collection resource representation of all cloud replication resources allocated to a tenant with the
- `veeam-cli infrastructure get-tenant-replication-resources-network-appliance`  -  Returns a resource representation of a tenant network extension appliance with the specified UID.
- `veeam-cli infrastructure get-tenant-replication-resources-network-appliances`  -  Returns a collection resource representation of a network extension appliances configured for a tenant with the
- `veeam-cli infrastructure get-tenant-replication-resources-usage`  -  Returns a collection resource representation of usage of all cloud replication resources allocated to a tenant with the
- `veeam-cli infrastructure get-tenant-traffic-resource`  -  Returns a resource representation of a cloud traffic quota configured for a tenant with the specified UID.
- `veeam-cli infrastructure get-tenant-vcd-replication-resource-data-center`  -  Returns a resource representation of a tenant organization VDC with the specified UID.
- `veeam-cli infrastructure get-tenant-vcd-replication-resources`  -  Returns a collection resource representation of all VMware Cloud Director replication resources allocated to a tenant
- `veeam-cli infrastructure get-tenant-vcd-replication-resources-network-appliance`  -  Returns a resource representation of a tenant network extension appliance with the specified UID.
- `veeam-cli infrastructure get-tenant-vcd-replication-resources-network-appliances`  -  Returns a collection resource representation of all network extension appliances configured for a tenant with the
- `veeam-cli infrastructure get-tenant-vcd-replication-resources-usage`  -  Returns a collection resource representation of usage of all VMware Cloud Director replication resources allocated to a
- `veeam-cli infrastructure get-tenants`  -  Returns a collection resource representation of all tenants.
- `veeam-cli infrastructure get-tenants-backup-resources`  -  Returns a collection resource representation of all tenant cloud backup resources.
- `veeam-cli infrastructure get-tenants-backup-resources-usages`  -  Returns a collection resource representation of cloud backup resource usage by all tenants.
- `veeam-cli infrastructure get-tenants-by-site`  -  Returns a collection resource representation of tenants registered on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure get-tenants-replication-resources`  -  Returns a collection resource representation of all tenant cloud replication resources.
- `veeam-cli infrastructure get-tenants-replication-resources-usages`  -  Returns a collection resource representation of usage of all tenant replication resources.
- `veeam-cli infrastructure get-tenants-vcd-replication-resources`  -  Returns a collection resource representation of all tenant VMware Cloud Director replication resources.
- `veeam-cli infrastructure get-tenants-vcd-replication-resources-usages`  -  Returns a collection resource representation of VMware Cloud Director replication resource usage by all tenants.
- `veeam-cli infrastructure get-unactivated-vb365-server`  -  Returns an resource representation of an Unactivated Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli infrastructure get-unactivated-vb365-servers`  -  Returns a collection resource representation of all unactivated Veeam Backup for Microsoft 365 servers.
- `veeam-cli infrastructure get-unverified-agent`  -  Returns a resource representation of an unverified management agent with the specified UID.
- `veeam-cli infrastructure get-unverified-agents`  -  Returns a collection resource representation of all unverified management agents.
- `veeam-cli infrastructure get-vb365-backup-job`  -  Returns a resource representation of a Veeam Backup for Microsoft 365 backup job with the specified UID.
- `veeam-cli infrastructure get-vb365-backup-job-available-backup-repositories`  -  Returns a collection resource representation of backup repositories that can be selected as target repositories of a
- `veeam-cli infrastructure get-vb365-backup-jobs`  -  Returns a collection resource representation of all Veeam Backup for Microsoft 365 backup jobs.
- `veeam-cli infrastructure get-vb365-backup-proxies`  -  Returns a collection resource representation of all backup proxies connected to a Veeam Backup for Microsoft 365 server
- `veeam-cli infrastructure get-vb365-backup-proxy-pools`  -  Returns a collection resource representation of all backup proxy pools connected to a Veeam Backup for Microsoft 365
- `veeam-cli infrastructure get-vb365-backup-repositories`  -  Returns a collection resource representation of all backup repositories connected to a Veeam Backup for Microsoft 365
- `veeam-cli infrastructure get-vb365-copy-job`  -  Returns a resource representation of a Veeam Backup for Microsoft 365 backup copy job with the specified UID.
- `veeam-cli infrastructure get-vb365-copy-jobs`  -  Returns a collection resource representation of all Veeam Backup for Microsoft 365 backup copy jobs.
- `veeam-cli infrastructure get-vb365-jobs`  -  Returns a collection resource representation of all Veeam Backup for Microsoft 365 jobs.
- `veeam-cli infrastructure get-vb365-microsoft365-organization`  -  Returns a resource representation of a Microsoft 365 organization with the specified UID.
- `veeam-cli infrastructure get-vb365-microsoft365-organizations`  -  Returns a collection resource representation of all Microsoft 365 organizations managed by a Veeam Backup for Microsoft
- `veeam-cli infrastructure get-vb365-organization`  -  Returns a resource representation of a Microsoft organization with the specified UID.
- `veeam-cli infrastructure get-vb365-organization-groups`  -  Returns a collection resource representation of all groups in a Microsoft organization with the specified UID.
- `veeam-cli infrastructure get-vb365-organization-sites`  -  Returns a collection resource representation of all sites of a Microsoft organization with the specified UID.
- `veeam-cli infrastructure get-vb365-organization-teams`  -  Returns a collection resource representation of all teams of a Microsoft organization with the specified UID.
- `veeam-cli infrastructure get-vb365-organization-to-company-mapping`  -  Returns a resource representation of a Microsoft organization to company mapping with the specified UID.
- `veeam-cli infrastructure get-vb365-organization-users`  -  Returns a collection resource representation of all users of a Microsoft organization with the specified UID.
- `veeam-cli infrastructure get-vb365-organizations`  -  Returns a collection resource representation of all Microsoft organizations.
- `veeam-cli infrastructure get-vb365-organizations-by-vb365-server`  -  Returns a collection resource representation of all Microsoft organizations managed by Veeam Backup for Microsoft 365
- `veeam-cli infrastructure get-vb365-organizations-to-company-mappings`  -  Returns a collection resource representation of all Microsoft organization mappings to companies.
- `veeam-cli infrastructure get-vb365-server`  -  Returns a resource representation of a connected Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli infrastructure get-vb365-servers`  -  Returns a collection resource representation of all connected Veeam Backup for Microsoft 365 servers.
- `veeam-cli infrastructure get-vcd-organization`  -  Returns a resource representation of a VMware Cloud Director organization with the specified UID.
- `veeam-cli infrastructure get-vcd-organization-users`  -  Returns a collection resource representation of users of all VMware Cloud Director organizations.
- `veeam-cli infrastructure get-vcd-organization-users-by-backup-server`  -  Returns a collection resource representation of users of all VMware Cloud Director organizations managed by a Veeam
- `veeam-cli infrastructure get-vcd-organization-users-by-organization`  -  Returns a collection resource representation of all users of a VMware Cloud Director organization with the specified
- `veeam-cli infrastructure get-vcd-organization-users-by-site`  -  Returns a collection resource representation of users of all VMware Cloud Director organizations managed by a Veeam
- `veeam-cli infrastructure get-vcd-organization-vapps-by-vcd`  -  Returns a collection resource representation of all vApps configured on a VMware Cloud Director server with the
- `veeam-cli infrastructure get-vcd-organization-vdc`  -  Returns a resource representation of an organization VDC with the specified UID.
- `veeam-cli infrastructure get-vcd-organization-vdcs`  -  Returns a collection resource representation of all organization VDCs.
- `veeam-cli infrastructure get-vcd-organization-vdcs-by-backup-server`  -  Returns a collection resource representation of all organization VDCs managed by a Veeam Backup & Replication server
- `veeam-cli infrastructure get-vcd-organization-vdcs-by-organization`  -  Returns a collection resource representation of all VDCs that provide resources to a VMware Cloud Director organization
- `veeam-cli infrastructure get-vcd-organization-vdcs-by-site`  -  Returns a collection resource representation of all organization VDCs managed by a Veeam Cloud Connect site with the
- `veeam-cli infrastructure get-vcd-organization-vdcs-by-vcd`  -  Returns a collection resource representation of all organization VDCs on a VMware Cloud Director Server with the
- `veeam-cli infrastructure get-vcd-organization-virtual-machines-by-vcd`  -  Returns a collection resource representation of all VMs configured on a VMware Cloud Director server with the specified
- `veeam-cli infrastructure get-vcd-organizations`  -  Returns a collection resource representation of all VMware Cloud Director organizations.
- `veeam-cli infrastructure get-vcd-organizations-by-backup-server`  -  Returns a collection resource representation of all VMware Cloud Director organizations managed by a Veeam Backup &
- `veeam-cli infrastructure get-vcd-organizations-by-site`  -  Returns a collection resource representation of all VMware Cloud Director organizations managed by a Veeam Cloud
- `veeam-cli infrastructure get-vcd-organizations-by-vcd`  -  Returns a collection resource representation of all VMware Cloud Director organizations configured on a VMware Cloud
- `veeam-cli infrastructure get-vcd-server`  -  Returns a resource representation of a VMware Cloud Director server with the specified UID.
- `veeam-cli infrastructure get-vcd-servers`  -  Returns a collection resource representation of all VMware Cloud Director servers.
- `veeam-cli infrastructure get-vcd-servers-by-backup-server`  -  Returns a collection resource representation of all VMware Cloud Director servers managed by a Veeam Backup &
- `veeam-cli infrastructure get-vcd-servers-by-site`  -  Returns a collection resource representation of all VMware Cloud Director servers managed by a Veeam Cloud Connect site
- `veeam-cli infrastructure get-vone-server`  -  Returns a resource representation of a connected Veeam ONE server with the specified UID.
- `veeam-cli infrastructure get-vone-server-settings`  -  Returns a resource representation of alarm options configured for a Veeam ONE server with the specified UID.
- `veeam-cli infrastructure get-vone-servers`  -  Returns a collection resource representation of all Veeam ONE servers connected to Veeam Service Provider Console.
- `veeam-cli infrastructure get-windows-backup-agent`  -  Returns a resource representation of a Veeam Agent for Microsoft Windows with the specified UID.
- `veeam-cli infrastructure get-windows-backup-agent-job`  -  Returns a resource representation of a Veeam Agent for Microsoft Windows job with the specified UID.
- `veeam-cli infrastructure get-windows-backup-agent-job-configuration`  -  Returns a resource representation of a configuration of the Veeam Agent for Microsoft Windows job with the specified
- `veeam-cli infrastructure get-windows-backup-agent-jobs`  -  Returns a collection resource representation of all Veeam backup agent jobs protecting Windows computers.
- `veeam-cli infrastructure get-windows-backup-agent-jobs-by-agent`  -  Returns a collection resource representation of all jobs configured for Veeam backup agent for Microsoft Windows with
- `veeam-cli infrastructure get-windows-backup-agents`  -  Returns a collection resource representation of all Veeam backup agents installed on Windows computers.
- `veeam-cli infrastructure grant-public-cloud-appliance-platform-credentials-for-migration`  -  Grants permissions necessary to update an Veeam Backup for AWS appliance to an account with the specified UID.
- `veeam-cli infrastructure install-cbt-driver`  -  Installs the Veeam CBT driver on a Windows computer that runs Veeam Agent with the specified UID.
- `veeam-cli infrastructure patch-backup-agent`  -  Modifies Veeam backup agent with the specified UID.
- `veeam-cli infrastructure patch-backup-agent-settings`  -  Modifies settings configured for Veeam Agent for Microsoft Windows with the specified UID.
- `veeam-cli infrastructure patch-backup-failover-plan`  -  Modifies a failover plan with the specified UID. > Operation is performed asynchronously and cannot be tracked.
- `veeam-cli infrastructure patch-backup-server`  -  Patches Veeam Backup & Replication on a server with the specified UID.
- `veeam-cli infrastructure patch-backup-server-backup-vm-vcd-job-configuration`  -  Modifies a configuration of a VMware Cloud Director VM backup job with the specified UID.
- `veeam-cli infrastructure patch-backup-server-backup-vm-vsphere-job-configuration`  -  Modifies a configuration of a VMware vSphere VM backup job with the specified UID.
- `veeam-cli infrastructure patch-backup-server-job`  -  Modifies a job with the specified UID.
- `veeam-cli infrastructure patch-backup-server-veeam-vault-repository`  -  Modifies a Veeam Data Cloud Vault backup repository with the specified UID.
- `veeam-cli infrastructure patch-backup-server-win-local-repository`  -  Modifies a Windows local backup repository with the specified UID.
- `veeam-cli infrastructure patch-linux-backup-agent-job-configuration`  -  Modifies Veeam Agent for Linux job configuration with the specified UID.
- `veeam-cli infrastructure patch-mac-backup-agent-job-configuration`  -  Modifies configuration of a Veeam Agent for Mac job with the specified UID.
- `veeam-cli infrastructure patch-management-agent`  -  Modifies a management agent with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-aws-account`  -  Modifies an AWS account with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-aws-appliance`  -  Modifies an Veeam Backup for AWS appliance with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-azure-account`  -  Modifies a Microsoft Azure account with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-azure-appliance`  -  Modifies a Veeam Backup for Microsoft Azure appliance with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-google-account`  -  Modifies a Google Cloud account with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-google-appliance`  -  Modifies a Veeam Backup for Google Cloud appliance with the specified UID.
- `veeam-cli infrastructure patch-public-cloud-guest-os-credentials`  -  Modifies public cloud guest OS credentials record with the specified UID.
- `veeam-cli infrastructure patch-scheduled-deployment-task-for-agent`  -  Modifies a management agent scheduled deployment task with the specified UID.
- `veeam-cli infrastructure patch-site`  -  Modifies a cloud agent installed on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure patch-tenant`  -  Modifies a tenant with the specified UID.
- `veeam-cli infrastructure patch-tenant-backup-resource`  -  Modifies a tenant cloud backup resource with the specified UID.
- `veeam-cli infrastructure patch-tenant-replication-resource`  -  Modifies a cloud replication resource allocated to a tenant with the specified UID.
- `veeam-cli infrastructure patch-tenant-replication-resources-network-appliance`  -  Modifies a tenant network extension appliance with the specified UID.
- `veeam-cli infrastructure patch-tenant-traffic-resource`  -  Modifies cloud traffic quota configured for a tenant with the specified UID.
- `veeam-cli infrastructure patch-tenant-vcd-replication-resource`  -  Modifies a VMware Cloud Director replication resource allocated to a tenant with the specified UID.
- `veeam-cli infrastructure patch-tenant-vcd-replication-resources-network-appliance`  -  Modifies a tenant network extension appliance with the specified UID.
- `veeam-cli infrastructure patch-vb365-backup-job`  -  Modifies a Veeam Backup for Microsoft 365 backup job with the specified UID.
- `veeam-cli infrastructure patch-vb365-copy-job`  -  Modifies a Veeam Backup for Microsoft 365 backup copy job with the specified UID.
- `veeam-cli infrastructure patch-vb365-microsoft365-organization`  -  Modifies a Microsoft 365 organization with the specified UID.
- `veeam-cli infrastructure patch-windows-backup-agent-job`  -  Modifies a Veeam Agent for Microsoft Windows job with the specified UID.
- `veeam-cli infrastructure patch-windows-backup-agent-job-configuration`  -  Modifies a configuration of a Veeam Agent for Microsoft Windows job with the specified UID.
- `veeam-cli infrastructure predownload-backup-server-iso`  -  Downloads the Veeam Backup & Replication upgrade setup file for further installation on a Veeam Backup & Replication
- `veeam-cli infrastructure predownload-backup-server-iso-on-discovery-computer`  -  Downloads the Veeam Backup & Replication disk image file for further installation on a discovered computer with the
- `veeam-cli infrastructure predownload-vone-server-iso`  -  Downloads the Veeam ONE upgrade setup file for further installation to a server with the specified UID.
- `veeam-cli infrastructure predownload-vone-server-iso-on-discovery-computer`  -  Downloads the Veeam ONE disk image file for further installation on a discovered computer with the specified UID.
- `veeam-cli infrastructure reboot-system-on-management-agent`  -  Runs system reboot on a computer where management agent with the specified UID is installed.
- `veeam-cli infrastructure remove-public-cloud-aws-account`  -  Delete an AWS account with the specified UID.
- `veeam-cli infrastructure remove-public-cloud-azure-account`  -  Deletes a Microsoft Azure account with the specified UID.
- `veeam-cli infrastructure remove-public-cloud-google-account`  -  Delete a Google Cloud account with the specified UID.
- `veeam-cli infrastructure remove-public-cloud-guest-os-credentials`  -  Deletes public cloud guest OS credentials record with the specified UID.
- `veeam-cli infrastructure remove-public-cloud-mapping`  -  Deletes an organization mapping of a Veeam Backup for Public Clouds appliance with the specified UID.
- `veeam-cli infrastructure renew-public-cloud-azure-account`  -  Renews client secret of a Microsoft Azure account with the specified UID.
- `veeam-cli infrastructure renew-vone-server-installation-uid`  -  Generates a new Veeam ONE installation UID for a Veeam ONE server with the specified UID.
- `veeam-cli infrastructure restart-backup-agent-service`  -  Restarts Veeam backup agent with the specified UID.
- `veeam-cli infrastructure restart-management-agent`  -  Restarts a management agent with the specified UID.
- `veeam-cli infrastructure retry-backup-server-job`  -  Retries a job with the specified UID.
- `veeam-cli infrastructure schedule-patch-backup-server`  -  Creates a scheduled task that installes Veeam Backup & Replication patch on a server with the specified UID.
- `veeam-cli infrastructure schedule-upgrade-backup-server`  -  Creates a scheduled task that installs the latest Veeam Backup & Replication update on a server with the specified UID.
- `veeam-cli infrastructure schedule-upgrade-vone-server`  -  Creates a scheduled task that installs the latest Veeam ONE update on a server with the specified UID.
- `veeam-cli infrastructure set-backup-agent-settings`  -  Replaces settings configured for Veeam Agent for Microsoft Windows with the specified UID.
- `veeam-cli infrastructure set-management-agent-credentials`  -  Configure credentials for Veeam Backup & Replication and Veeam backup agent installation on a management agent with the
- `veeam-cli infrastructure set-site-maintenance-mode`  -  Enables or disables the maintenance mode for a site with the specified UID.
- `veeam-cli infrastructure set-site-tenant-management-mode`  -  Enables or disables tenant management on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli infrastructure start-backup-agent-job`  -  Starts a Veeam backup agent job with the specified UID.
- `veeam-cli infrastructure start-backup-failover-plan`  -  Starts a failover plan with the specified UID.
- `veeam-cli infrastructure start-backup-server-job`  -  Starts a job with the specified UID.
- `veeam-cli infrastructure start-linux-backup-agent-job`  -  Starts a Veeam Agent for Linux job with the specified UID.
- `veeam-cli infrastructure start-mac-backup-agent-job`  -  Starts a Veeam Agent for Mac job with the specified UID.
- `veeam-cli infrastructure start-public-cloud-policy`  -  Starts a Veeam Backup for Public Clouds policy with the specified UID.
- `veeam-cli infrastructure start-scheduled-deployment-task-for-agent`  -  Starts a management agent scheduled deployment task with the specified UID.
- `veeam-cli infrastructure start-vb365-job`  -  Starts a Veeam Backup for Microsoft 365 job with the specified UID.
- `veeam-cli infrastructure start-windows-backup-agent-job`  -  Starts a Veeam Agent for Microsoft Windows job with the specified UID.
- `veeam-cli infrastructure stop-backup-agent-job`  -  Stops a Veeam backup agent job with the specified UID.
- `veeam-cli infrastructure stop-backup-server-job`  -  Stops a job with the specified UID.
- `veeam-cli infrastructure stop-linux-backup-agent-job`  -  Stops a Veeam Agent for Linux job with the specified UID.
- `veeam-cli infrastructure stop-mac-backup-agent-job`  -  Stops a Veeam Agent for Mac job with the specified UID.
- `veeam-cli infrastructure stop-public-cloud-policy`  -  Stops a Veeam Backup for Public Clouds policy with the specified UID.
- `veeam-cli infrastructure stop-vb365-job`  -  Stops a Veeam Backup for Microsoft 365 job with the specified UID.
- `veeam-cli infrastructure stop-windows-backup-agent-job`  -  Stops a Veeam Agent for Microsoft Windows job with the specified UID.
- `veeam-cli infrastructure sync-public-cloud-accounts`  -  Initiates synchronization of public cloud account data between Veeam Service Provider Console and Veeam Backup for
- `veeam-cli infrastructure sync-public-cloud-guest-os-credentials`  -  Initiates synchronization of public cloud guest OS credentials data in Veeam Service Provider Console with Veeam Backup
- `veeam-cli infrastructure sync-public-cloud-sql-accounts`  -  Retrieves and updates data of public cloud SQL accounts.
- `veeam-cli infrastructure unassign-backup-server-job`  -  Unassigns a job with the specified UID from a company.
- `veeam-cli infrastructure undo-backup-failover-plan`  -  Undoes a failover plan with the specified UID.
- `veeam-cli infrastructure uninstall-cbt-driver`  -  Uninstalls the Veeam CBT driver from a Windows computer that runs Veeam Agent with the specified UID.
- `veeam-cli infrastructure update-linux-backup-agent`  -  Updates a Veeam Agent for Linux with the specified UID.
- `veeam-cli infrastructure update-mac-backup-agent`  -  Updates a Veeam Agent for Mac with the specified UID.
- `veeam-cli infrastructure update-public-cloud-sql-account`  -  Modifies a public cloud SQL account with the specified ID.
- `veeam-cli infrastructure update-vone-server-settings`  -  Updates alarm options configured for a Veeam ONE server with the specified UID.
- `veeam-cli infrastructure update-windows-backup-agent`  -  Updates Veeam Agent for Microsoft Windows with the specified UID.
- `veeam-cli infrastructure upgrade-backup-server`  -  Installs the latest update of Veeam Backup & Replication on a server with the specified UID.
- `veeam-cli infrastructure upgrade-public-cloud-appliance`  -  Upgrades a Veeam Backup for Public Clouds appliance with the specified UID registered on a Veeam Cloud Connect site.
- `veeam-cli infrastructure upgrade-vone-server`  -  Installs the latest Veeam ONE update on a server with the specified UID.
- `veeam-cli infrastructure upload-backup-server-multipart-patch`  -  Uploads a patch file chunk to Veeam Backup & Replication server with the specified UID.
- `veeam-cli infrastructure upload-linux-backup-server-multipart-patch`  -  Uploads a patch file chunk to Veeam Backup & Replication linux server with the specified UID.
- `veeam-cli infrastructure upload-public-cloud-multipart-patch`  -  Uploads a patch file chunk to Veeam Backup for Public Clouds appliances registered on a Veeam Cloud Connect site with
- `veeam-cli infrastructure upload-vone-server-multipart-patch`  -  Uploads a patch file chunk to Veeam ONE server with the specified UID.
- `veeam-cli infrastructure validate-existing-public-cloud-aws-appliance-connection`  -  Validates an existing Veeam Backup for AWS appliance connection registered on a Veeam Cloud Connect site with the
- `veeam-cli infrastructure validate-public-cloud-appliance-platform-credentials-for-migration`  -  Verifies that account used to access a backup device with the specified UID has permissions to install backup device
- `veeam-cli infrastructure verify-public-cloud-appliance-certificate`  -  Verifies a security certificate of a Veeam Backup for Public Clouds appliance with the specified UID.

**licensing**  -  This resource collection represents licenses in Veeam Service Provider Console.

- `veeam-cli licensing delete-backup-server-license`  -  Deletes a license from a managed Veeam Backup & Replication server with the specified UID.
- `veeam-cli licensing delete-vone-server-license`  -  Deletes a license from a managed Veeam One server with the specified UID.
- `veeam-cli licensing download-report-csv`  -  Returns a download link to a license usage report with the specified ID in the `CSV` format.
- `veeam-cli licensing finalize-reports`  -  Finalize the Veeam license usage reports.
- `veeam-cli licensing get-backup-server-license`  -  Returns a resource representation of a license installed on a managed Veeam Backup & Replication server with the
- `veeam-cli licensing get-backup-servers-licenses`  -  Returns a collection resource representation of the licenses installed on all managed Veeam Backup & Replication
- `veeam-cli licensing get-console-license-information`  -  Returns resource representation of the currently installed Veeam Service Provider Console license.
- `veeam-cli licensing get-console-license-settings`  -  Returns a resource representation of the currently installed Veeam Service Provider Console license settings.
- `veeam-cli licensing get-latest-reports`  -  Returns a collection resource representation of the latest Veeam license usage reports.
- `veeam-cli licensing get-organizations-license-usage`  -  Returns a collection resource representation of license usage by all organizations.
- `veeam-cli licensing get-reports`  -  Returns a collection resource representation of the Veeam license usage reports.
- `veeam-cli licensing get-reports-for-date`  -  Returns a collection resource representation of the Veeam license usage reports with the specified generation date.
- `veeam-cli licensing get-reports-settings`  -  Returns a resource representation of license usage report settings.
- `veeam-cli licensing get-site-license`  -  Returns a resource representation of a license installed on the Veeam Cloud Connect site with the specified UID.
- `veeam-cli licensing get-site-licenses`  -  Returns a collection resource representation of all licenses installed on the Veeam Cloud Connect sites.
- `veeam-cli licensing get-vb365-server-license`  -  Returns a resource representation of a license installed on a managed Veeam Backup for Microsoft 365 server with the
- `veeam-cli licensing get-vb365-servers-licenses`  -  Returns a collection resource representation of the licenses installed on all managed Veeam Backup for Microsoft 365
- `veeam-cli licensing get-vone-server-license`  -  Returns a resource representation of a license installed on a managed Veeam One server with the specified UID.
- `veeam-cli licensing get-vone-servers-licenses`  -  Returns a collection resource representation of the licenses installed on all managed Veeam One servers.
- `veeam-cli licensing install-backup-server-license`  -  Install a license on a managed Veeam Backup & Replication server with the specified UID.
- `veeam-cli licensing install-console-license`  -  Installs Veeam Service Provider Console license.
- `veeam-cli licensing install-site-license`  -  Installs a license on the Veeam Cloud Connect site with the specified UID.
- `veeam-cli licensing install-vb365-server-license`  -  Install a license on a managed Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli licensing install-vone-server-license`  -  Install a license on a managed Veeam One server with the specified UID.
- `veeam-cli licensing patch-backup-server-license`  -  Modifies a license on a managed Veeam Backup & Replication server with the specified UID.
- `veeam-cli licensing patch-reports-settings`  -  Modifies the Veeam licenses usage reports settings.
- `veeam-cli licensing patch-site-license`  -  Modifies a license on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli licensing patch-vb365-server-license`  -  Modifies a license on a managed Veeam Backup for Microsoft 365 server with the specified UID.
- `veeam-cli licensing patch-vone-server-license`  -  Modifies a license on a managed Veeam One server with the specified UID.
- `veeam-cli licensing update-backup-server-license`  -  Downloads a new license from the Internet and installs it on a Veeam Backup & Replication server.
- `veeam-cli licensing update-console-license`  -  Downloads a new Veeam Service Provider Console license from the Internet and installs it.
- `veeam-cli licensing update-site-license`  -  Downloads a license from the Internet and installs it on the Veeam Cloud Connect site with the specified UID.
- `veeam-cli licensing update-vb365-server-license`  -  Downloads a new license from the Internet and installs it on a Veeam Backup for Microsoft 365 server.
- `veeam-cli licensing update-vone-server-license`  -  Downloads a new license from the Internet and installs it on a Veeam One server.

**misc**  -  This resource collection represents useful miscellaneous data.

- `veeam-cli misc get-countries`  -  Returns a collection resource representation of all countries.
- `veeam-cli misc get-currencies`  -  Returns a collection resource representation of all currencies.
- `veeam-cli misc get-usa-states`  -  Returns a collection resource representation of all USA states.

**organizations**  -  This resource collection represent organizations in Veeam Service Provider Console. Organizations include service provider, resellers and companies.

- `veeam-cli organizations assign-company-to-reseller`  -  Assigns a company with the specified UID to a reseller.
- `veeam-cli organizations change-invoice-paid-state`  -  Changes payment status of an invoice with the specified UID.
- `veeam-cli organizations check-uniqueness-for-identity-provider-name`  -  Checks whether the specified name of an identity provider is unique.
- `veeam-cli organizations create-company`  -  Creates a new company managed in Veeam Service Provider Console.
- `veeam-cli organizations create-company-hosted-vbr-backup-resource`  -  Allocates a new Veeam Backup & Replication repository resource to a company on a hosted Veeam Backup & Replication
- `veeam-cli organizations create-company-hosted-vbr-resource`  -  Allocates a hosted Veeam Backup & Replication server resource to a company with the specified UID.
- `veeam-cli organizations create-company-hosted-vbr-tag-resource`  -  Allocates a new tag resource to a company on a hosted Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations create-company-hosted-vbr-vcd-mapping`  -  Maps a VMware Cloud Director organization to a company with an assigned hosted resource with the specified UID.
- `veeam-cli organizations create-company-vb365-backup-resource`  -  Allocates a new backup resource to a company on a Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations create-company-vb365-resource`  -  Allocates a Veeam Backup for Microsoft 365 resource to a company with the specified UID.
- `veeam-cli organizations create-location`  -  Creates a new organization location.
- `veeam-cli organizations create-org-container`  -  Creates a new organization container with specific properties.
- `veeam-cli organizations create-reseller`  -  Creates a new reseller with specific properties.
- `veeam-cli organizations create-reseller-site-backup-resource`  -  Creates a reseller cloud backup resource on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli organizations create-reseller-site-replication-resource`  -  Creates a reseller replication resource on a Veeam Cloud Connect site with the specified UID.
- `veeam-cli organizations create-reseller-site-resource`  -  Creates a managed company quota for a reseller with the specified UID.
- `veeam-cli organizations create-reseller-site-vcd-replication-resource`  -  Creates a VMware Cloud Director replication resource allocated to a reseller on a Veeam Cloud Connect site with the
- `veeam-cli organizations create-reseller-site-wan-accelerator-resource`  -  Creates a reseller cloud WAN accelerator resource on a site with the specified UID.
- `veeam-cli organizations create-reseller-vb365-resource`  -  Allocates a Veeam Backup for Microsoft 365 resource to a reseller with the specified UID.
- `veeam-cli organizations create-reseller-vbr-resource`  -  Allocates a Veeam Backup & Replication server resource to a reseller with the specified UID.
- `veeam-cli organizations delete-company`  -  Deletes a company with the specified UID.
- `veeam-cli organizations delete-company-hosted-vbr-backup-resource`  -  Deletes a company hosted Veeam Backup & Replication repository resource with the specified UID.
- `veeam-cli organizations delete-company-hosted-vbr-resource`  -  Deletes a company hosted Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations delete-company-hosted-vbr-tag-resource`  -  Deletes a company tag resource with the specified UID.
- `veeam-cli organizations delete-company-hosted-vbr-vcd-mapping`  -  Deletes a VMware Cloud Director organization to company mapping with the specified UID.
- `veeam-cli organizations delete-company-vb365-backup-resource`  -  Deletes a company Veeam Backup for Microsoft 365 backup resource with the specified UID.
- `veeam-cli organizations delete-company-vb365-resource`  -  Deletes a company Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations delete-invoice`  -  Deletes an invoice with the specified UID.
- `veeam-cli organizations delete-location`  -  Deletes an organization location with the specified UID.
- `veeam-cli organizations delete-org-container`  -  Deletes an organization container with the specified UID.
- `veeam-cli organizations delete-reseller`  -  Deletes a reseller with the specified UID.
- `veeam-cli organizations delete-reseller-site-backup-resource`  -  Deletes a reseller cloud backup resource with the specified UID.
- `veeam-cli organizations delete-reseller-site-replication-resource`  -  Deletes a reseller cloud replication resource with the specified UID.
- `veeam-cli organizations delete-reseller-site-resource`  -  Deletes a managed companies quota configured for a reseller on the Veeam Cloud Connect site with the specified UID.
- `veeam-cli organizations delete-reseller-site-vcd-replication-resource`  -  Deletes a reseller VMware Cloud Director replication resource with the specified UID.
- `veeam-cli organizations delete-reseller-site-wan-accelerator-resource`  -  Deletes a reseller cloud WAN accelerator resource with the specified UID.
- `veeam-cli organizations delete-reseller-vb365-repository-resource`  -  Deletes a reseller Veeam Backup for Microsoft 365 repository with the specified UID.
- `veeam-cli organizations delete-reseller-vb365-resource`  -  Deletes a reseller Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations delete-reseller-vbr-resource`  -  Deletes a reseller Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations download-invoice-pdf`  -  Returns a download link to an invoice with the specified UID in the `PDF` format.
- `veeam-cli organizations generate-invoice`  -  Initiates invoice generation for a company with the specified UID.
- `veeam-cli organizations generate-quota-usage-report`  -  Initiates quota usage report for a company with the specified UID.
- `veeam-cli organizations get`  -  Returns a collection resource representation of all organizations (service provider, resellers, companies).
- `veeam-cli organizations get-assigned-to-company-subscription-plan`  -  Returns a resource representation of a subscription plan assigned to a company with the specified UID.
- `veeam-cli organizations get-billing-settings`  -  Returns a collection resource representation of billing settings configured for all companies.
- `veeam-cli organizations get-billing-settings-by-company`  -  Returns a resource representation of billing settings configured for a company with the specified UID.
- `veeam-cli organizations get-companies`  -  Returns a collection resource representation of all companies managed in Veeam Service Provider Console.
- `veeam-cli organizations get-companies-aggregated-usage`  -  Returns a collection resource representation of services consumed by companies.
- `veeam-cli organizations get-companies-by-reseller`  -  Returns a collection resource representation of companies managed by a reseller with the specified UID.
- `veeam-cli organizations get-companies-hosted-vbr-backup-resources`  -  Returns a collection resource representation of hosted Veeam Backup & Replication repository resources allocated to all
- `veeam-cli organizations get-companies-hosted-vbr-resources`  -  Returns a collection resource representation of hosted Veeam Backup & Replication server resources allocated to all
- `veeam-cli organizations get-companies-hosted-vbr-tag-resources`  -  Returns a collection resource representation of all company tag resources.
- `veeam-cli organizations get-companies-hosted-vbr-vcd-mappings`  -  Returns a collection resource representation of mappings of VMware Cloud Director organizations to all companies with
- `veeam-cli organizations get-companies-vb365-backup-resources`  -  Returns a collection resource representation of all company Veeam Backup for Microsoft 365 backup resources.
- `veeam-cli organizations get-companies-vb365-resources`  -  Returns a collection resource representation of Veeam Backup for Microsoft 365 resources allocated to all companies.
- `veeam-cli organizations get-company`  -  Returns a resource representation of a company with the specified UID.
- `veeam-cli organizations get-company-aggregated-usage`  -  Returns a collection resource representation of services consumed by a company with the specified UID.
- `veeam-cli organizations get-company-hosted-vbr-backup-resource`  -  Returns a resource representation of a company hosted Veeam Backup & Replication repository resource with the specified
- `veeam-cli organizations get-company-hosted-vbr-backup-resources`  -  Returns a collection resource representation of all Veeam Backup & Replication repository resources allocated to a
- `veeam-cli organizations get-company-hosted-vbr-resource`  -  Returns a resource representation of a company hosted Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations get-company-hosted-vbr-resources`  -  Returns a collection resource representation of hosted Veeam Backup & Replication server resources allocated to a
- `veeam-cli organizations get-company-hosted-vbr-tag-resource`  -  Returns a resource representation of a company tag resource with the specified UID.
- `veeam-cli organizations get-company-hosted-vbr-tag-resources`  -  Returns a collection resource representation of all tag resources allocated to a company on a hosted Veeam Backup &
- `veeam-cli organizations get-company-hosted-vbr-vcd-mapping`  -  Returns a resource representation of a VMware Cloud Director organization to company mapping with the specified UID.
- `veeam-cli organizations get-company-hosted-vbr-vcd-mappings`  -  Returns a collection resource representation of all mappings of VMware Cloud Director organizations to a company with
- `veeam-cli organizations get-company-site-resources`  -  Returns a collection resource representation of all cloud tenants assigned to a company with the specified UID.
- `veeam-cli organizations get-company-vb365-backup-resource`  -  Returns a resource representation of a company Veeam Backup for Microsoft 365 backup resource with the specified UID.
- `veeam-cli organizations get-company-vb365-backup-resources`  -  Returns a collection resource representation of all backup resources allocated to a company on a Veeam Backup for
- `veeam-cli organizations get-company-vb365-resource`  -  Returns a resource representation of company Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations get-company-vb365-resources`  -  Returns a collection resource representation of Veeam Backup for Microsoft 365 resources allocated to a company with
- `veeam-cli organizations get-custom-welcome-email-templates`  -  Returns a collection resource representation of all custom settings configured for email notifications.
- `veeam-cli organizations get-identity-providers`  -  Returns a collection resource representation of all identity providers.
- `veeam-cli organizations get-identity-providers-rules`  -  Returns a collection resource representation of all mapping rules.
- `veeam-cli organizations get-invoice`  -  Returns a resource representation of an invoice with the specified UID.
- `veeam-cli organizations get-invoices`  -  Returns a collection resource representation of invoices generated for all companies.
- `veeam-cli organizations get-invoices-by-company`  -  Returns a resource representation of all invoices generated for a company with the specified UID.
- `veeam-cli organizations get-location`  -  Returns a resource representation of a organization location with the specified UID.
- `veeam-cli organizations get-locations`  -  Returns a collection resource representation of all organization locations.
- `veeam-cli organizations get-org-container`  -  Returns a resource representation of an organization container with the specified UID.
- `veeam-cli organizations get-org-containers`  -  Returns a collection resource representation of all organization containers.
- `veeam-cli organizations get-organizationuid`  -  Returns a resource representation of an organization with the specified UID.
- `veeam-cli organizations get-provider`  -  Returns a resource representation of a service provider.
- `veeam-cli organizations get-provider-companies`  -  Returns a collection resource representation of all companies managed by a service provider.
- `veeam-cli organizations get-reseller`  -  Returns a resource representation of a reseller with the specified UID.
- `veeam-cli organizations get-reseller-aggregated-usage`  -  Returns a collection resource representation of services consumed by a reseller with the specified UID.
- `veeam-cli organizations get-reseller-license-resource`  -  Returns a resource representation of a license management resource allocated to a reseller with the specified UID.
- `veeam-cli organizations get-reseller-site-backup-resource`  -  Returns a resource representation of a reseller cloud backup resource with the specified UID.
- `veeam-cli organizations get-reseller-site-backup-resources`  -  Returns a collection resource representation of all cloud backup resources allocated to a reseller on a Veeam Cloud
- `veeam-cli organizations get-reseller-site-backup-resources-usage`  -  Returns a resource representation of usage of all cloud backup resources allocated to a reseller on a Veeam Cloud
- `veeam-cli organizations get-reseller-site-replication-resource`  -  Returns a resource representation of a reseller cloud replication resource with the specified UID.
- `veeam-cli organizations get-reseller-site-replication-resources`  -  Returns a collection resource representation of all cloud replication resources allocated to a reseller on a Veeam
- `veeam-cli organizations get-reseller-site-replication-resources-usage`  -  Returns a resource representation of cloud replication resource usage by a reseller on a Veeam Cloud Connect site with
- `veeam-cli organizations get-reseller-site-resource`  -  Returns a resource representation of a managed companies quota configured for a reseller on a Veeam Cloud Connect site
- `veeam-cli organizations get-reseller-site-resources`  -  Returns a collection resource representation of managed company quotas configured for a reseller with the specified UID
- `veeam-cli organizations get-reseller-site-vcd-replication-resource`  -  Returns a resource representation of a reseller VMware Cloud Director replication resource with the specified UID.
- `veeam-cli organizations get-reseller-site-vcd-replication-resources`  -  Returns a collection resource representation of all VMware Cloud Director replication resources allocated to a reseller
- `veeam-cli organizations get-reseller-site-wan-accelerator-resource`  -  Returns a resource representation of a reseller cloud WAN accelerator resource with the specified UID.
- `veeam-cli organizations get-reseller-site-wan-accelerator-resources`  -  Returns a collection resource representation of all cloud WAN accelerator resources allocated to a reseller on a site
- `veeam-cli organizations get-reseller-vb365-resource`  -  Returns a resource representation of a reseller Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations get-reseller-vb365-resources`  -  Returns a collection resource representation of all Veeam Backup for Microsoft 365 resources allocated to a reseller
- `veeam-cli organizations get-reseller-vbr-resource`  -  Returns a resource representation of a reseller Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations get-reseller-vbr-resources`  -  Returns a collection resource representation of all Veeam Backup & Replication server resources allocated to a reseller
- `veeam-cli organizations get-resellers`  -  Returns a collection resource representation of all resellers.
- `veeam-cli organizations get-resellers-aggregated-usage`  -  Returns a collection resource representation of services consumed by resellers.
- `veeam-cli organizations get-resellers-license-resources`  -  Returns a collection resource representation of all license management resources allocated to resellers.
- `veeam-cli organizations get-resellers-site-backup-resources`  -  Returns a collection resource representation of all reseller cloud backup resources.
- `veeam-cli organizations get-resellers-site-backup-resources-usages`  -  Returns a collection resource representation of cloud backup resource usage by all resellers.
- `veeam-cli organizations get-resellers-site-replication-resources`  -  Returns a collection resource representation of all reseller cloud replication resources.
- `veeam-cli organizations get-resellers-site-replication-resources-usages`  -  Returns a collection resource representation of Veeam Cloud Connect site replication resource usage by all resellers.
- `veeam-cli organizations get-resellers-site-resources`  -  Returns a collection resource representation of managed company quotas configured for all resellers.
- `veeam-cli organizations get-resellers-site-vcd-replication-resources`  -  Returns a collection resource representation of all reseller VMware Cloud Director replication resources.
- `veeam-cli organizations get-resellers-site-wan-accelerator-resources`  -  Returns a collection resource representation of cloud WAN accelerator resources of all resellers.
- `veeam-cli organizations get-resellers-vb365-resources`  -  Returns a collection resource representation of Veeam Backup for Microsoft 365 resources allocated to all resellers.
- `veeam-cli organizations get-resellers-vbr-resources`  -  Returns a collection resource representation of Veeam Backup & Replication server resources allocated to all resellers.
- `veeam-cli organizations get-users-by-location`  -  Returns a collection resource representation of all users that are assigned to a location with the specified UID.
- `veeam-cli organizations patch`  -  Modifies an organization with the specified UID.
- `veeam-cli organizations patch-billing-settings`  -  Modifies billing settings for a company wth the specified UID.
- `veeam-cli organizations patch-company`  -  Modifies a company managed in Veeam Service Provider Console.
- `veeam-cli organizations patch-company-hosted-vbr-backup-resource`  -  Modifies a company hosted Veeam Backup & Replication repository resource with the specified UID.
- `veeam-cli organizations patch-company-hosted-vbr-resource`  -  Modifies a company hosted Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations patch-company-vb365-backup-resource`  -  Modifies a company Veeam Backup for Microsoft 365 backup resource with the specified UID.
- `veeam-cli organizations patch-company-vb365-resource`  -  Modifies a company Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations patch-location`  -  Modifies an organization location.
- `veeam-cli organizations patch-org-container`  -  Modifies an organization container with the specified UID.
- `veeam-cli organizations patch-reseller`  -  Modifies a reseller with the specified UID.
- `veeam-cli organizations patch-reseller-license-resource`  -  Modifies a license management resource allocated to a reseller with the specified UID.
- `veeam-cli organizations patch-reseller-site-backup-resource`  -  Modifies a reseller cloud backup resource with the specified UID.
- `veeam-cli organizations patch-reseller-site-replication-resource`  -  Modifies a reseller cloud replication resource with the specified UID.
- `veeam-cli organizations patch-reseller-site-resource`  -  Modifies a managed companies quota configured for a reseller on the Veeam Cloud Connect site with the specified UID.
- `veeam-cli organizations patch-reseller-site-wan-accelerator-resource`  -  Modifies a reseller cloud WAN accelerator resource with the specified UID.
- `veeam-cli organizations patch-reseller-vb365-resource`  -  Modifies a reseller Veeam Backup for Microsoft 365 resource with the specified UID.
- `veeam-cli organizations patch-reseller-vbr-resource`  -  Modifies a reseller Veeam Backup & Replication server resource with the specified UID.
- `veeam-cli organizations send-invoice`  -  Sends an invoice with the specified UID.
- `veeam-cli organizations send-welcome-email-to-company`  -  Sends a welcome email to a company with the specified UID.
- `veeam-cli organizations send-welcome-email-to-reseller`  -  Sends a welcome email to a reseller with the specified UID.
- `veeam-cli organizations unassign-company-from-reseller`  -  Unassigns a company with the specified UID from a reseller.

**permissions**  -  Manage permissions

- `veeam-cli permissions get-entity`  -  Returns a resource representation of permissions provided to a Veeam Service Provider Console entity with the specified
- `veeam-cli permissions patch-entity`  -  Modifies permissions provided to a Veeam Service Provider Console entity with the specified UID.

**plugins**  -  This resource collection represents plugins for Veeam Service Provider Console.

- `veeam-cli plugins get`  -  Returns a resource representation of a currently installed plugin with the specified ID.
- `veeam-cli plugins get-all`  -  Returns a collection resource representation of all installed plugins.
- `veeam-cli plugins get-charges`  -  Returns a collection resource representation of charges for all registered plugins.
- `veeam-cli plugins get-user-is-logged`  -  Returns a resource representation of information on plugin user account and server. > Should be used only by plugins.
- `veeam-cli plugins uninstall`  -  Uninstalls a currently installed plugin with the specified ID.

**protected-workloads**  -  This resource collection represents workloads managed in Veeam Service Provider Console.

- `veeam-cli protected-workloads get-protected-cloud-database-backups-by-database`  -  Returns a collection resource representation of all backups of a protected cloud database with the specified UID.
- `veeam-cli protected-workloads get-protected-cloud-databases`  -  Returns a collection resource representation of all protected cloud databases.
- `veeam-cli protected-workloads get-protected-cloud-databases-backups`  -  Returns a collection resource representation of all backups of protected cloud databases.
- `veeam-cli protected-workloads get-protected-cloud-file-share-backups`  -  Returns a collection resource representation of all backups created for protected cloud file shares.
- `veeam-cli protected-workloads get-protected-cloud-file-share-backups-by-share`  -  Returns a collection resource representation of all backups created for a protected cloud file share with the specified
- `veeam-cli protected-workloads get-protected-cloud-file-shares`  -  Returns a collection resource representation of all protected cloud file shares.
- `veeam-cli protected-workloads get-protected-cloud-networks`  -  Returns a collection resource representation of all protected cloud networks.
- `veeam-cli protected-workloads get-protected-cloud-virtual-machine-backups-by-vm`  -  Returns a collection resource representation of all backups of a protected cloud VM with the specified UID.
- `veeam-cli protected-workloads get-protected-cloud-virtual-machines`  -  Returns a collection resource representation of all protected cloud VMs.
- `veeam-cli protected-workloads get-protected-cloud-virtual-machines-backups`  -  Returns a collection resource representation of all backups of protected cloud VMs.
- `veeam-cli protected-workloads get-protected-computer-managed-by-backup-server-restore-points`  -  Returns a collection resource representation of all restore points created for a protected computer managed by Veeam
- `veeam-cli protected-workloads get-protected-computer-managed-by-console-restore-points`  -  Returns a collection resource representation of restore points created for a protected computer managed by Veeam
- `veeam-cli protected-workloads get-protected-computers-managed-by-backup-server`  -  Returns a collection resource representation of all protected computers managed by Veeam Backup & Replication.
- `veeam-cli protected-workloads get-protected-computers-managed-by-backup-server-backups`  -  Returns a collection resource representation of all backups of computers managed by Veeam Backup & Replication.
- `veeam-cli protected-workloads get-protected-computers-managed-by-backup-server-backups-by-backup-agent`  -  Returns a collection resource representation of all backups of a computer managed by Veeam Backup & Replication with
- `veeam-cli protected-workloads get-protected-computers-managed-by-backup-server-latest-restore-points`  -  Returns a collection resource representation of the latest restore points of computers managed by Veeam Backup &
- `veeam-cli protected-workloads get-protected-computers-managed-by-backup-server-restore-points`  -  Returns a collection resource representation of all restore points created for computers managed by Veeam Backup &
- `veeam-cli protected-workloads get-protected-computers-managed-by-console`  -  Returns a collection resource representation of all protected computers managed by Veeam Service Provider Console.
- `veeam-cli protected-workloads get-protected-computers-managed-by-console-jobs`  -  Returns a collection resource representation of all jobs that protect computers managed by Veeam Service Provider
- `veeam-cli protected-workloads get-protected-computers-managed-by-console-jobs-by-backup-agent`  -  Returns a collection resource representation of jobs that protect a computer managed by Veeam Service Provider Console
- `veeam-cli protected-workloads get-protected-computers-managed-by-console-latest-restore-points`  -  Returns a collection resource representation of latest restore points of computers managed by Veeam Service Provider
- `veeam-cli protected-workloads get-protected-computers-managed-by-console-restore-points`  -  Returns a collection resource representation of all restore points of computers managed by Veeam Service Provider
- `veeam-cli protected-workloads get-protected-file-share-restore-points`  -  Returns a collection resource representation of all restore points created for a protected computer managed by Veeam
- `veeam-cli protected-workloads get-protected-file-shares-backups`  -  Returns a collection resource representation of all backups of computers managed by Veeam Backup & Replication.
- `veeam-cli protected-workloads get-protected-file-shares-backups-by-file-share`  -  Returns a collection resource representation of all backups of a file share with the specified UID.
- `veeam-cli protected-workloads get-protected-file-shares-restore-points`  -  Returns a collection resource representation of all restore points created for computers managed by Veeam Backup &
- `veeam-cli protected-workloads get-protected-object-storage-restore-points`  -  Returns a collection resource representation of all restore points created for a protected computer managed by Veeam
- `veeam-cli protected-workloads get-protected-object-storages`  -  Returns a collection resource representation of all protected object storages.
- `veeam-cli protected-workloads get-protected-object-storages-backups`  -  Returns a collection resource representation of all backups of computers managed by Veeam Backup & Replication.
- `veeam-cli protected-workloads get-protected-object-storages-backups-by-object-storage`  -  Returns a collection resource representation of all backups of a computer managed by Veeam Backup & Replication with
- `veeam-cli protected-workloads get-protected-object-storages-restore-points`  -  Returns a collection resource representation of all restore points created for computers managed by Veeam Backup &
- `veeam-cli protected-workloads get-protected-on-premises-file-shares`  -  Returns a collection resource representation of all protected file shares.
- `veeam-cli protected-workloads get-protected-virtual-machine-backup-restore-points`  -  Returns a collection resource representation of backup restore points created for a protected VM with the specified UID.
- `veeam-cli protected-workloads get-protected-virtual-machine-backups`  -  Returns a collection resource representation of all backups of protected VMs.
- `veeam-cli protected-workloads get-protected-virtual-machine-backups-by-vm`  -  Returns a collection resource representation of all backups of a protected VM with the specified UID.
- `veeam-cli protected-workloads get-protected-virtual-machine-replica-restore-points`  -  Returns a collection resource representation of replication restore points created for a protected VM with the
- `veeam-cli protected-workloads get-protected-virtual-machines`  -  Returns a collection resource representation of all protected VMs.
- `veeam-cli protected-workloads get-protected-virtual-machines-latest-restore-points`  -  Returns a collection resource representation of latest restore points created for VMs.
- `veeam-cli protected-workloads get-vb365-protected-objects`  -  Returns a collection resource representation of all objects protected by Veeam Backup for Microsoft 365.
- `veeam-cli protected-workloads get-vb365-restore-points`  -  Returns a collection resource representation of all restore points of an object with the specified UID protected by

**proxy**  -  Manage proxy

- `veeam-cli proxy`  -  Returns a collection resource representation of active proxy sessions.

**pulse**  -  This resource collection represents VCSP Pulse integration support.

- `veeam-cli pulse copy-license`  -  Creates a copy of a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse create-company-by-tenant`  -  Creates a company based on VCSP Pulse tenant with the specified UID.
- `veeam-cli pulse create-license`  -  Adds a new license configuration with the specified parameters to VCSP Pulse.
- `veeam-cli pulse create-tenant-by-company`  -  Creates a VCSP Pulse tenant based on a specific company.
- `veeam-cli pulse delete-license`  -  Deletes a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse download-license`  -  Downloads a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse get-configuration`  -  Returns a resource representation of VCSP Pulse plugin configuration.
- `veeam-cli pulse get-license`  -  Rerurns a resource representation of a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse get-license-contracts`  -  Returns a collection resource representation of all rental agreement contracts.
- `veeam-cli pulse get-license-products`  -  Returns a collection resource representation of all Veeam products managed in VCSP Pulse.
- `veeam-cli pulse get-licenses`  -  Rerurns a collection resource representation of all licenses managed in VCSP Pulse.
- `veeam-cli pulse get-tenant`  -  Returns a resource representation of a VCSP Pulse tenant with the specified UID.
- `veeam-cli pulse get-tenants`  -  Returns a collection resource representation of all VCSP Pulse tenants.
- `veeam-cli pulse install-license`  -  Installs a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse patch-configuration`  -  Modifies VCSP Pulse plugin configuration. > To disconnect the plugin, replace the `token` property value with `null`.
- `veeam-cli pulse patch-license`  -  Modifies a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse patch-tenant`  -  Modifies a VCSP Pulse tenant with the specified UID.
- `veeam-cli pulse revoke-license`  -  Revokes a license managed in VCSP Pulse with the specified UID.
- `veeam-cli pulse sync-data`  -  Initiates synchronization with the VCSP Pulse portal.

**subscription-plans**  -  This resource collection represents subscription plans configured in Veeam Service Provider Console.

- `veeam-cli subscription-plans create`  -  Creates a subscription plan.
- `veeam-cli subscription-plans delete`  -  Deletes a subscription plan with the specified UID.
- `veeam-cli subscription-plans get`  -  Returns a collection resource representation of all subscription plans.
- `veeam-cli subscription-plans get-subscriptionplans`  -  Returns a resource representation of a subscription plan with the specified UID.
- `veeam-cli subscription-plans patch`  -  Modifies a subscription plan with the specified UID.

**token**  -  Manage token

- `veeam-cli token`  -  Performs authentication using the OAuth 2.0 Authorization Framework.

**users**  -  Manage users

- `veeam-cli users complete-reset-password-request`  -  Completes a request for password reset.
- `veeam-cli users create`  -  Creates a new user with specific properties.
- `veeam-cli users create-local-rule`  -  Creates a new Administrator Portal user or group.
- `veeam-cli users create-reset-password-request`  -  Resets a password of a specific user.
- `veeam-cli users delete`  -  Deletes a user with the specified UID.
- `veeam-cli users delete-local-rule`  -  Deletes an Administrator Portal user or group with the specified UID.
- `veeam-cli users discover-objects-for-local-rule`  -  Discovers users and groups in the domain and on the machine on which Veeam Service Provider Console is installed.
- `veeam-cli users get`  -  Returns a collection resource representation of all users.
- `veeam-cli users get-backup-resources`  -  Returns a collection resource representation of subtenant user backup resources.
- `veeam-cli users get-current`  -  Returns a resource representation of a currently logged in user.
- `veeam-cli users get-local-rule`  -  Returns a resource representation of an Administrator Portal user or group with the specified UID.
- `veeam-cli users get-local-rules`  -  Returns a collection resource representation of all Administrator Portal users and groups.
- `veeam-cli users get-logins`  -  Returns a collection resource representation of all user identities.
- `veeam-cli users get-useruid`  -  Returns a resource representation of a user with the specified UID.
- `veeam-cli users patch`  -  Modifies a user with the specified UID.
- `veeam-cli users patch-local-rule`  -  Modifies an Administrator Portal user or group with the specified UID.

**vdc-vault**  -  Manage vdc vault

- `veeam-cli vdc-vault assign-vdc-storage-vault-to-backup-server`  -  Assigns a storage vault to a Veeam Backup & Replication server with the specified UID.
- `veeam-cli vdc-vault complete-registration`  -  Completes Veeam Data Cloud Vault registration.
- `veeam-cli vdc-vault create-backup-server-assigned-vault-folder`  -  Creates a folder to store data on a storage vault with the specified UID.
- `veeam-cli vdc-vault create-tenant`  -  Adds a new Veeam Data Cloud Vault tenant.
- `veeam-cli vdc-vault create-tenant-mapping`  -  Creates a mapping to a company for a Veeam Data Cloud tenant with the specified UID.
- `veeam-cli vdc-vault create-vdc-storage-vault`  -  Adds a new storage vault.
- `veeam-cli vdc-vault get-backup-server-assigned-vault`  -  Returns a resource representation of a storage vault with the specified UID that is assigned to a Veeam Backup &
- `veeam-cli vdc-vault get-backup-server-assigned-vault-folders`  -  Returns a resource representation of a list of folders that are used to store data on a storage vault with the
- `veeam-cli vdc-vault get-backup-server-assigned-vaults`  -  Returns a collection resource representation of all storage vaults assigned to a Veeam Backup & Replication server with
- `veeam-cli vdc-vault get-configuration`  -  Returns a resource representation of a Veeam Data Cloud Vault configurations status.
- `veeam-cli vdc-vault get-registered-backup-server`  -  Returns a resource representation of a Veeam Backup & Replication server that is registered Veeam Data Cloud Vault and
- `veeam-cli vdc-vault get-registered-backup-servers`  -  Returns a collection resource representation of all Veeam Backup & Replication servers registered in Veeam Data Cloud
- `veeam-cli vdc-vault get-registration-data`  -  Returns a resource representation of registration data of a connected Veeam Data Cloud Vault.
- `veeam-cli vdc-vault get-subscription`  -  Returns a resource representation of a Veeam Data Cloud Vault subscription with the specified UID.
- `veeam-cli vdc-vault get-subscription-countries`  -  Returns a collection resource representation of all countries registered for a Veeam Data Cloud Vault subscription with
- `veeam-cli vdc-vault get-subscriptions`  -  Returns a collection resource representation of all Veeam Data Cloud Vault subscriptions.
- `veeam-cli vdc-vault get-tenant`  -  Returns a resource representation of a Veeam Data Cloud Vault tenant with the specified UID.
- `veeam-cli vdc-vault get-tenants`  -  Returns a collection resource representation of all Veeam Data Cloud Vault tenants.
- `veeam-cli vdc-vault get-vdc-storage-vault`  -  Returns a resource representation of a storage vault with the specified UID.
- `veeam-cli vdc-vault get-vdc-storage-vaults`  -  Returns a collection resource representation of all storage vaults.
- `veeam-cli vdc-vault patch-vdc-storage-vault`  -  Modifies a storage vault with the specified UID.
- `veeam-cli vdc-vault register-backup-server`  -  Registers a specified Veeam Backup & Replication server in Veeam Data Cloud Vault.
- `veeam-cli vdc-vault remove-registration`  -  Removes registration of a Veeam Data Cloud Vault.
- `veeam-cli vdc-vault remove-tenant-mapping`  -  Deletes a mapping to a company for a Veeam Data Cloud tenant with the specified UID.
- `veeam-cli vdc-vault remove-vdc-storage-vault`  -  Deletes a storage vault with the specified UID.
- `veeam-cli vdc-vault sync`  -  Synchronizes Veeam Data Cloud Vault data.
- `veeam-cli vdc-vault unassign-vdc-storage-vault-from-backup-server`  -  Unassigns a storage vault with the specified UID from a Veeam Backup & Replication server.
- `veeam-cli vdc-vault unregister-backup-server`  -  Cancels the registration of a Veeam Backup & Replication server with the specified UID in Veeam Data Cloud Vault.

**veeam-service-provider-about**  -  Manage veeam service provider about

- `veeam-cli veeam-service-provider-about`  -  Returns general information about the currently installed version of Veeam Service Provider Console.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
veeam-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Fleet backup posture for an agent

```bash
veeam-cli fleet-health --agent --select companies.company,companies.failed,companies.agents_offline
```

Narrow the cross-tenant rollup to just the columns an agent needs to decide where to look first.

### Stale protection as JSON

```bash
veeam-cli stale-backups --days 7 --agent
```

Every job with no successful run in a week, machine-readable for a ticket-creation workflow.

### Who is out of RPO right now

```bash
veeam-cli at-risk --rpo 24h --agent
```

Protected workloads whose newest restore point is older than 24h  -  the SLA breaches to act on.

### Shift-handoff delta

```bash
veeam-cli since 12h --agent
```

Only what newly broke across the fleet in the last 12 hours.

### One customer, full picture

```bash
veeam-cli company-overview --company "Contoso Ltd" --agent
```

A single-tenant 360 assembled from the local mirror instead of six console tabs.

## Auth Setup

VSPC authenticates with a short-lived Bearer access token. Obtain one from your appliance: POST https://<host>:1280/api/v3/token with grant_type=password, username, and password (the response carries access_token and refresh_token). Set VEEAM_TOKEN to the access_token and VEEAM_BASE_URL to your console's API root (e.g. https://vspc.example.com:1280/api/v3). Tokens expire roughly hourly; re-mint when calls start returning 401.

Run `veeam-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  veeam-cli alarms get --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
veeam-cli feedback "the --since flag is inclusive but docs say exclusive"
veeam-cli feedback --stdin < notes.txt
veeam-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/veeam-cli/feedback.jsonl`. They are never POSTed unless `VEEAM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `VEEAM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration.

```
veeam-cli profile save briefing --json
veeam-cli --profile briefing alarms get
veeam-cli profile list --json
veeam-cli profile show briefing
veeam-cli profile delete briefing --yes
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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `veeam-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add veeam-mcp -- veeam-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which veeam-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   veeam-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `veeam-cli <command> --help`.
