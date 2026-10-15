---
name: proxmox
description: "Use when the user asks whether Proxmox VE or Proxmox Backup Server (PBS) backups are good across client clusters: which VMs are unprotected, stale or failed verification, what failed overnight, when a datastore fills, or which restore points exist. Read-only (GET-only) CLI and MCP server with per-guest backup verdicts. Trigger phrases: `are my proxmox backups good`, `which VMs are not backed up`, `check PBS verify failures`, `proxmox backup posture for all clients`, `when will the backup datastore fill up`, `use proxmox`, `run proxmox-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Proxmox"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - proxmox-cli
    install:
      - kind: script
        bins: [proxmox-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1
---

# Proxmox VE + Backup Server  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `proxmox-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1 | iex
   ```
3. Verify: `proxmox-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `proxmox-cli` and `proxmox-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every safe read endpoint of Proxmox VE and Proxmox Backup Server (a few unsafe GETs are excluded by design) as a typed command, with no write calls to Proxmox, so nothing an agent runs can change a cluster or a backup. On top: posture joins job coverage, real archive age and PBS verify state per guest across clusters; backup coverage, verify-health, failures, capacity, restore-points and rpo answer the questions MSPs ask every morning.

## When to Use This CLI

Use this CLI to inspect Proxmox VE clusters and Proxmox Backup Server from scripts or agents without changing them (GET requests only; no configuration, guest or backup is modified): backup posture across many client clusters, verify failures, failed backup tasks, datastore capacity, restore-point lookups and RPO evidence, plus every read-only API endpoint (VMs, containers, nodes, storage, tasks, Ceph, SDN, HA, firewall, access). It is built for MSPs running many small clusters and anyone who wants an agent to watch backups without being able to change them.

## Anti-triggers

Do not use this CLI for:
- Starting, stopping, migrating, creating or deleting VMs or containers: this CLI has no write commands; use the Proxmox web UI, qm/pct, or pvesh on the node
- Running a backup, restore, prune, verify or garbage collection: use vzdump, the PBS UI, or proxmox-backup-manager
- Downloading backup archives or restoring files: use proxmox-backup-client or the file-restore UI
- Managing Proxmox Mail Gateway: not covered

## Unique Capabilities

### Backup assurance across every cluster
- **`posture`**  -  One verdict per guest across every client cluster: covered by an enabled job, age of the newest real restore point, PBS verify state, last backup attempt.

  _Reach for this first when asked whether backups are good; --fail-on-red exits 1 on any red guest or unread cluster, and its vendor-neutral rows merge with other backup vendors' output. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli posture --agent --stale-after 26h
  ```
- **`backup coverage`**  -  Every guest or disk that no enabled backup job includes, with the reason: no job, disabled job only, excluded, or disk excluded.

  _Use after provisioning or migration waves to prove every new guest and disk is in an enabled job. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli backup coverage --include-disks --agent
  ```
- **`failures`**  -  Failed and silently incomplete backup, verify, GC, sync and prune tasks in a time window, one row per failed guest or datastore with the reason line.

  _Use for morning triage instead of reading backup emails; it names the guest and the error, not just the task. It reads at most --max-logs task logs per cluster (default 50); log_cap_hit says when that limit was reached. A PBS task names no PVE cluster, so on a PBS that another configured cluster also writes to its row has attribution `shared: <clusters>` and an empty tenant; do not credit it to one client. A task two profiles read through the same PBS host spelling is listed once. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli failures --since 24h --agent
  ```

### PBS health
- **`verify-health`**  -  Per PBS datastore and backup group: failed, never-verified and outdated snapshots, first failure, newest verified-good point, and namespaces no verify job reaches. Needs the PBS API.

  _Use when verify failures appear, to see how far they spread and which groups still have a good restore point. Needs PROXMOX_PBS_HOST and PROXMOX_PBS_API_TOKEN (or pbs_host in clusters.toml); skipped_clusters lists clusters with no PBS host, which were not checked, and exit 5 means no cluster was checked at all; healthy groups are listed only with --all._

  ```bash
  proxmox-cli verify-health --agent
  ```
- **`capacity`**  -  PBS datastores ranked by days until full, plus every PVE backup storage by percent used (only PBS forecasts), with garbage-collection and prune state and a GC-deadlock warning.

  _Use weekly to catch a datastore before it fills past the point where garbage collection can still run. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli capacity --within 30d --agent
  ```

### Restore readiness
- **`restore-points`**  -  Every restore point of one guest across all storages and PBS namespaces, with verify state, protection, encryption and, for points listed from PVE storage, whether the next prune removes it.

  _Use to answer 'can we restore server X from Tuesday?' with the exact restore points and whether each verified. Exit 3 means no reachable cluster has that guest; exit 5 means no cluster answered, or no match had a readable restore point while some source was not read (see not_read)._

  ```bash
  proxmox-cli restore-points 104 --agent
  ```
- **`rpo`**  -  Per guest over a past window: largest gap between restore points, scheduled runs with no restore point, and days with a verified-good point, rolled up per client.

  _Use for weekly SLA reviews and QBR or compliance evidence that every scheduled backup produced a restore point. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli rpo --days 7 --agent
  ```

## Command Reference

**access**  -  Manage access

- `proxmox-cli access acl`  -  Get Access Control List (ACLs).
- `proxmox-cli access domains`  -  Authentication domain index.
- `proxmox-cli access groups`  -  Group index.
- `proxmox-cli access groups-get`  -  Get group configuration. Required privilege: Sys.Audit, Group.Allocate (any) on /access/groups
- `proxmox-cli access permissions`  -  Retrieve effective permissions of given user/token.
- `proxmox-cli access roles`  -  Role index. Required privilege: any authenticated user
- `proxmox-cli access roles-get`  -  Get role configuration. Required privilege: any authenticated user
- `proxmox-cli access tfa`  -  List TFA configurations of users. Required privilege: Returns all or just the logged-in user, depending on privileges.
- `proxmox-cli access tfa-get`  -  List TFA configurations of users.
- `proxmox-cli access tfa-id-get`  -  Fetch a requested TFA entry if present.
- `proxmox-cli access users`  -  User index.
- `proxmox-cli access users-get`  -  Get user configuration. Required privilege: user or group management privileges on /access
- `proxmox-cli access users-tfa`  -  Get user TFA types (Personal and Realm).
- `proxmox-cli access users-token`  -  Get user API tokens.
- `proxmox-cli access users-token-get`  -  Get specific API token information.

**acme**  -  Manage acme

- `proxmox-cli acme account`  -  ACMEAccount index. Required privilege: any authenticated user
- `proxmox-cli acme account-get`  -  Return existing ACME account information.
- `proxmox-cli acme challenge-schema`  -  Get schema of ACME challenge types. Required privilege: any authenticated user
- `proxmox-cli acme directories`  -  Get named known ACME directory endpoints. Required privilege: any authenticated user

**backup**  -  Manage backup

- `proxmox-cli backup job`  -  List vzdump backup schedule. Required privilege: Sys.Audit on /
- `proxmox-cli backup job-get`  -  Read vzdump backup job definition. Required privilege: Sys.Audit on /
- `proxmox-cli backup job-included-volumes`  -  Returns included guests and the backup status of their disks. Optimized to be used in ExtJS tree views.
- `proxmox-cli backup not-backed-up`  -  Shows all guests which are not covered by any backup job. Required privilege: Sys.Audit on /
- `proxmox-cli backup vzdump-defaults`  -  Get the currently configured vzdump defaults.

**ceph**  -  Manage ceph

- `proxmox-cli ceph cluster-flags`  -  get the status of all ceph flags. Required privilege: Sys.Audit on /
- `proxmox-cli ceph cluster-flags-get`  -  Get the status of a specific ceph flag. Required privilege: Sys.Audit on /
- `proxmox-cli ceph cluster-health-mute`  -  Get the currently muted Ceph health checks. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph cluster-metadata`  -  Get ceph metadata. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph cluster-status`  -  Get ceph status. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph cmd-safety`  -  Heuristical check if it is safe to perform an action. Required privilege: Sys.Audit on /
- `proxmox-cli ceph crush`  -  Get OSD crush map. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph fs`  -  Directory index. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph log`  -  Read ceph log. Required privilege: Sys.Syslog on /nodes/{node}
- `proxmox-cli ceph mds`  -  MDS directory index. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph mgr`  -  MGR directory index. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph mon`  -  Get Ceph monitor list. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph osd`  -  Get Ceph osd list/tree. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph osd-lv-info`  -  Get OSD volume details. Required privilege: Sys.Audit on /
- `proxmox-cli ceph osd-metadata`  -  Get OSD details. Required privilege: Sys.Audit on /
- `proxmox-cli ceph pool`  -  List all pools and their settings (which are settable by the POST/PUT endpoints).
- `proxmox-cli ceph pool-status`  -  Show the current pool status. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph releases`  -  List all known Ceph releases, marking which ones can be installed on this node.
- `proxmox-cli ceph rules`  -  List ceph rules. Required privilege: Sys.Audit, Datastore.Audit (any) on /
- `proxmox-cli ceph status`  -  Get the Ceph cluster status (raw 'ceph status' output).

**cluster**  -  Manage cluster

- `proxmox-cli cluster api-version`  -  API version details, including some parts of the global datacenter config. Required privilege: any authenticated user
- `proxmox-cli cluster config-apiversion`  -  Return the version of the cluster join API available on this node. Required privilege: Sys.Audit on /
- `proxmox-cli cluster config-join`  -  Get information needed to join this cluster over the connected node. Required privilege: Sys.Audit on /
- `proxmox-cli cluster config-nodes`  -  Corosync node list. Required privilege: Sys.Audit on /
- `proxmox-cli cluster config-qdevice`  -  Get QDevice status. Required privilege: Sys.Audit on /
- `proxmox-cli cluster config-totem`  -  Get corosync totem protocol settings. Required privilege: Sys.Audit on /
- `proxmox-cli cluster jobs-realm-sync`  -  List configured realm-sync-jobs. Required privilege: Sys.Audit on /
- `proxmox-cli cluster jobs-realm-sync-get`  -  Read realm-sync job definition. Required privilege: Sys.Audit on /
- `proxmox-cli cluster jobs-schedule-analyze`  -  Returns a list of future schedule runtimes. Required privilege: any authenticated user
- `proxmox-cli cluster log`  -  Read cluster log. Required privilege: The user needs 'Sys.Syslog' on '/' in order to get all logs.
- `proxmox-cli cluster metrics-export`  -  Retrieve metrics of the cluster. Required privilege: Sys.Audit on /
- `proxmox-cli cluster metrics-server`  -  List configured metric servers. Required privilege: Sys.Audit on /
- `proxmox-cli cluster metrics-server-get`  -  Read metric server configuration. Required privilege: Sys.Audit on /
- `proxmox-cli cluster nextid`  -  Get next free VMID. Pass a VMID to assert that its free (at time of check). Required privilege: any authenticated user
- `proxmox-cli cluster qemu-cpu-flags`  -  List of available CPU flags. Currently only implemented for x86_64, returns an empty list for aarch64.
- `proxmox-cli cluster qemu-custom-cpu-models`  -  List all custom CPU model definitions visible to the user.
- `proxmox-cli cluster qemu-custom-cpu-models-get`  -  Retrieve details about a specific custom CPU model.
- `proxmox-cli cluster resources`  -  Resources index (cluster wide). Required privilege: any authenticated user
- `proxmox-cli cluster status`  -  Get cluster status information. Required privilege: Sys.Audit on /

**ct**  -  Manage ct

- `proxmox-cli ct config`  -  Get container configuration. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct feature`  -  Check if feature for virtual machine is available. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-aliases`  -  List aliases. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-aliases-get`  -  Read alias. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-ipset`  -  List IPSets. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-ipset-cidr-get`  -  Read IP or Network settings from IPSet. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-ipset-get`  -  List IPSet content. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-log`  -  Read firewall log. Required privilege: VM.Console on /vms/{vmid}
- `proxmox-cli ct firewall-options`  -  Get VM firewall options. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-refs`  -  Lists possible IPSet/Alias reference which are allowed in source/dest properties.
- `proxmox-cli ct firewall-rules`  -  List rules. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct firewall-rules-get`  -  Get single rule data. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct interfaces`  -  Get IP addresses of the specified container interface. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct list`  -  LXC container index (per node). Required privilege: Only list CTs where you have VM.Audit permission on /vms/<vmid>.
- `proxmox-cli ct migrate`  -  Get preconditions for migration. Required privilege: VM.Migrate on /vms/{vmid}
- `proxmox-cli ct pending`  -  Get container configuration, including pending changes. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct rrd`  -  Read VM RRD statistics (returns PNG) Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct rrddata`  -  Read VM RRD statistics. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct snapshot`  -  List all snapshots. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli ct snapshot-config`  -  Get snapshot configuration. Required privilege: VM.Snapshot, VM.Snapshot.Rollback, VM.Audit (any) on /vms/{vmid}
- `proxmox-cli ct snapshot-get`  -  Read /nodes/{node}/lxc/{vmid}/snapshot/{snapname}. Required privilege: any authenticated user
- `proxmox-cli ct status-current`  -  Get virtual machine status. Required privilege: VM.Audit on /vms/{vmid}

**disks**  -  Manage disks

- `proxmox-cli disks directory`  -  PVE Managed Directory storages. Required privilege: Sys.Audit on /
- `proxmox-cli disks list`  -  List local disks. Required privilege: Sys.Audit on / or Sys.Audit on /nodes/{node}
- `proxmox-cli disks lvm`  -  List LVM Volume Groups. Required privilege: Sys.Audit on /
- `proxmox-cli disks lvmthin`  -  List LVM thinpools. Required privilege: Sys.Audit on /
- `proxmox-cli disks scan`  -  Index of available scan methods. Required privilege: any authenticated user
- `proxmox-cli disks scan-lvm`  -  List local LVM volume groups. Required privilege: Datastore.Allocate on /storage
- `proxmox-cli disks scan-lvmthin`  -  List local LVM Thin Pools. Required privilege: Datastore.Allocate on /storage
- `proxmox-cli disks scan-zfs`  -  Scan zfs pool list on local node. Required privilege: Datastore.Allocate on /storage
- `proxmox-cli disks smart`  -  Get SMART Health of a disk. Required privilege: Sys.Audit on /
- `proxmox-cli disks zfs`  -  List Zpools. Required privilege: Sys.Audit on /
- `proxmox-cli disks zfs-get`  -  Get details about a zpool. Required privilege: Sys.Audit on /

**firewall**  -  Manage firewall

- `proxmox-cli firewall aliases`  -  List aliases. Required privilege: Sys.Audit on /
- `proxmox-cli firewall aliases-get`  -  Read alias. Required privilege: Sys.Audit on /
- `proxmox-cli firewall groups`  -  List security groups. Required privilege: any authenticated user
- `proxmox-cli firewall groups-get`  -  List rules. Required privilege: Sys.Audit on /
- `proxmox-cli firewall groups-pos-get`  -  Get single rule data. Required privilege: Sys.Audit on /
- `proxmox-cli firewall ipset`  -  List IPSets. Required privilege: Sys.Audit on /
- `proxmox-cli firewall ipset-cidr-get`  -  Read IP or Network settings from IPSet. Required privilege: Sys.Audit on /
- `proxmox-cli firewall ipset-get`  -  List IPSet content. Required privilege: Sys.Audit on /
- `proxmox-cli firewall macros`  -  List available macros. Required privilege: any authenticated user
- `proxmox-cli firewall node-log`  -  Read firewall log. Required privilege: Sys.Syslog on /nodes/{node}
- `proxmox-cli firewall node-options`  -  Get host firewall options. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli firewall node-rules`  -  List rules. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli firewall node-rules-get`  -  Get single rule data. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli firewall options`  -  Get Firewall options. Required privilege: Sys.Audit on /
- `proxmox-cli firewall refs`  -  Lists possible IPSet/Alias reference which are allowed in source/dest properties. Required privilege: Sys.Audit on /
- `proxmox-cli firewall rules`  -  List rules. Required privilege: Sys.Audit on /
- `proxmox-cli firewall rules-get`  -  Get single rule data. Required privilege: Sys.Audit on /

**ha**  -  Manage ha

- `proxmox-cli ha groups`  -  Get HA groups. (deprecated in favor of HA rules) Required privilege: Sys.Audit on /
- `proxmox-cli ha groups-get`  -  Read ha group configuration. (deprecated in favor of HA rules) Required privilege: Sys.Audit on /
- `proxmox-cli ha list`  -  Directory index. Required privilege: Sys.Audit on /
- `proxmox-cli ha resources`  -  List HA resources. Required privilege: Sys.Audit on /
- `proxmox-cli ha resources-get`  -  Read resource configuration. Required privilege: Sys.Audit on /
- `proxmox-cli ha rules`  -  Get HA rules. Required privilege: Sys.Audit on /
- `proxmox-cli ha rules-get`  -  Read HA rule. Required privilege: Sys.Audit on /
- `proxmox-cli ha status-current`  -  Get HA manager status. Required privilege: Sys.Audit on /
- `proxmox-cli ha status-manager-status`  -  Get full HA manager status, including LRM status. Required privilege: Sys.Audit on /

**mapping**  -  Manage mapping

- `proxmox-cli mapping dir`  -  List directory mapping.
- `proxmox-cli mapping dir-get`  -  Get directory mapping.
- `proxmox-cli mapping pci`  -  List PCI Hardware Mapping.
- `proxmox-cli mapping pci-get`  -  Get PCI Mapping.
- `proxmox-cli mapping usb`  -  List USB Hardware Mappings.
- `proxmox-cli mapping usb-get`  -  Get USB Mapping.

**nodes**  -  Manage nodes

- `proxmox-cli nodes aplinfo`  -  Get list of appliances. Required privilege: any authenticated user
- `proxmox-cli nodes apt`  -  Directory index for apt (Advanced Package Tool). Required privilege: any authenticated user
- `proxmox-cli nodes apt-changelog`  -  Get package changelogs. Required privilege: Sys.Modify on /nodes/{node}
- `proxmox-cli nodes apt-update`  -  List available updates. Required privilege: Sys.Modify on /nodes/{node}
- `proxmox-cli nodes apt-versions`  -  Get package information for important Proxmox packages. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes capabilities-qemu-cpu`  -  List all custom and default CPU models.
- `proxmox-cli nodes capabilities-qemu-cpu-flags`  -  List of available VM-specific CPU flags.
- `proxmox-cli nodes capabilities-qemu-machines`  -  Get available QEMU/KVM machine types. Required privilege: any authenticated user
- `proxmox-cli nodes capabilities-qemu-migration`  -  Get node-specific QEMU migration capabilities of the node. Requires the 'Sys.Audit' permission on '/nodes/<node>'.
- `proxmox-cli nodes certificates-info`  -  Get information about node's certificates. Required privilege: any authenticated user
- `proxmox-cli nodes config`  -  Get node configuration options. Required privilege: Sys.Audit on /
- `proxmox-cli nodes dns`  -  Read DNS settings. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes hardware`  -  Index of hardware types. Required privilege: any authenticated user
- `proxmox-cli nodes hardware-pci`  -  List local PCI devices. Required privilege: Sys.Audit, Sys.Modify (any) on /
- `proxmox-cli nodes hardware-pci-get`  -  Index of available pci methods. Required privilege: any authenticated user
- `proxmox-cli nodes hardware-pci-mdev`  -  List mediated device types for given PCI device. Required privilege: Sys.Audit, Sys.Modify (any) on /
- `proxmox-cli nodes hardware-usb`  -  List local USB devices. Required privilege: Sys.Modify on /
- `proxmox-cli nodes hosts`  -  Get the content of /etc/hosts. Required privilege: Sys.Audit on /
- `proxmox-cli nodes journal`  -  Read Journal. Required privilege: Sys.Syslog on /nodes/{node}
- `proxmox-cli nodes list`  -  Cluster node index. Required privilege: any authenticated user
- `proxmox-cli nodes netstat`  -  Read tap/vm network device interface counters. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes network`  -  List available networks. Required privilege: any authenticated user
- `proxmox-cli nodes network-get`  -  Read network device configuration. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes rrd`  -  Read node RRD statistics (returns PNG) Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes rrddata`  -  Read node RRD statistics. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes services`  -  Service list. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes services-state`  -  Read service properties. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes status`  -  Read node status. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes syslog`  -  Read system log. Required privilege: Sys.Syslog on /nodes/{node}
- `proxmox-cli nodes time`  -  Read server time and time zone settings. Required privilege: Sys.Audit on /nodes/{node}
- `proxmox-cli nodes version`  -  API version details. Required privilege: any authenticated user

**notifications**  -  Manage notifications

- `proxmox-cli notifications endpoints-gotify`  -  Returns a list of all gotify endpoints. Required privilege: Mapping.Audit on /mapping/notifications
- `proxmox-cli notifications endpoints-gotify-get`  -  Return a specific gotify endpoint.
- `proxmox-cli notifications endpoints-sendmail`  -  Returns a list of all sendmail endpoints.
- `proxmox-cli notifications endpoints-sendmail-get`  -  Return a specific sendmail endpoint.
- `proxmox-cli notifications endpoints-smtp`  -  Returns a list of all smtp endpoints.
- `proxmox-cli notifications endpoints-smtp-get`  -  Return a specific smtp endpoint.
- `proxmox-cli notifications matcher-field-values`  -  Returns known notification metadata fields and their known values.
- `proxmox-cli notifications matcher-fields`  -  Returns known notification metadata fields.
- `proxmox-cli notifications matchers`  -  Returns a list of all matchers.
- `proxmox-cli notifications matchers-get`  -  Return a specific matcher.
- `proxmox-cli notifications targets`  -  Returns a list of all entities that can be used as notification targets.

**pbs_access**  -  Manage pbs access

- `proxmox-cli pbs-access acl`  -  Read Access Control List (ACLs).
- `proxmox-cli pbs-access config-ad`  -  List configured AD realms. Required privilege: Realm.Allocate on /access/domains
- `proxmox-cli pbs-access config-ad-get`  -  Read the AD realm configuration. Required privilege: Sys.Audit on /access/domains
- `proxmox-cli pbs-access config-ldap`  -  List configured LDAP realms. Required privilege: Realm.Allocate on /access/domains
- `proxmox-cli pbs-access config-ldap-get`  -  Read the LDAP realm configuration. Required privilege: Sys.Audit on /access/domains
- `proxmox-cli pbs-access config-pam`  -  Read the PAM realm configuration. Required privilege: Sys.Audit on /access/domains
- `proxmox-cli pbs-access config-pbs`  -  Read the Proxmox Backup authentication server realm configuration. Required privilege: Sys.Audit on /access/domains
- `proxmox-cli pbs-access config-tfa-webauthn`  -  Get the TFA configuration. Required privilege: any authenticated user
- `proxmox-cli pbs-access domains`  -  Authentication domain/realm index.
- `proxmox-cli pbs-access permissions`  -  List permissions of given or currently authenticated user / API token. Optionally limited to specific path.
- `proxmox-cli pbs-access roles`  -  Role list. Required privilege: any authenticated user
- `proxmox-cli pbs-access tfa`  -  List user TFA configuration. Required privilege: Returns all or just the logged-in user, depending on privileges.
- `proxmox-cli pbs-access tfa-get`  -  List a user's TFA entries (types and descriptions; no secrets). Required privilege: Permissions.Modify on /access/users or the user named by {userid}
- `proxmox-cli pbs-access tfa-id-get`  -  Get a single TFA entry. Required privilege: Permissions.Modify on /access/users or the user named by {userid}
- `proxmox-cli pbs-access users`  -  List users. Required privilege: Returns all or just the logged-in user (/API token owner), depending on privileges.
- `proxmox-cli pbs-access users-get`  -  Read user configuration data. Required privilege: Sys.Audit on /access/users or the user named by {userid}
- `proxmox-cli pbs-access users-token`  -  List user's API tokens. Required privilege: Sys.Audit on /access/users or the user named by {userid}
- `proxmox-cli pbs-access users-token-get`  -  Read user's API token metadata. Required privilege: Sys.Audit on /access/users or the user named by {userid}

**pbs_config**  -  Manage pbs config

- `proxmox-cli pbs-config acme-account`  -  List ACME accounts. Required privilege: Sys.Modify on /system/certificates
- `proxmox-cli pbs-config acme-account-get`  -  Return existing ACME account information. Required privilege: Sys.Modify on /system/certificates
- `proxmox-cli pbs-config acme-challenge-schema`  -  Get the ACME challenge plugin schema. Required privilege: any authenticated user
- `proxmox-cli pbs-config acme-directories`  -  Get named known ACME directory endpoints. Required privilege: any authenticated user
- `proxmox-cli pbs-config admin-metrics`  -  List configured metric servers. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-config admin-traffic-control`  -  Show current traffic for all traffic control rules. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-config encryption-keys`  -  List configured encryption keys. Required privilege: List configured encryption keys filtered by Sys.Audit privileges
- `proxmox-cli pbs-config metrics-influxdb-udp`  -  List configured InfluxDB udp metric servers. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-config metrics-influxdb-udp-get`  -  Read the InfluxDB udp server configuration. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-config remote`  -  List all remotes. Required privilege: List configured remotes filtered by Remote.Audit privileges
- `proxmox-cli pbs-config remote-get`  -  Read remote configuration data. Required privilege: Remote.Audit on /remote/{name}
- `proxmox-cli pbs-config remote-scan`  -  List datastores of a remote.cfg entry. Required privilege: Remote.Audit on /remote/{name}
- `proxmox-cli pbs-config remote-scan-groups`  -  List groups of a remote.cfg entry's datastore. Required privilege: Remote.Audit on /remote/{name}
- `proxmox-cli pbs-config remote-scan-namespaces`  -  List namespaces of a datastore of a remote.cfg entry. Required privilege: Remote.Audit on /remote/{name}
- `proxmox-cli pbs-config s3`  -  List all s3 client configurations. Required privilege: List configured s3 endpoints filtered by Sys.Audit privileges
- `proxmox-cli pbs-config s3-get`  -  Read an s3 client configuration. Required privilege: Sys.Audit on /system/s3-endpoint/{id}
- `proxmox-cli pbs-config s3-list-buckets`  -  List buckets accessible by given s3 client configuration. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-config traffic-control`  -  List traffic control rules. Required privilege: Sys.Modify on /
- `proxmox-cli pbs-config traffic-control-get`  -  Read traffic control configuration data. Required privilege: Sys.Audit on /

**pbs_datastore**  -  Manage pbs datastore

- `proxmox-cli pbs-datastore active-operations`  -  Read datastore stats. Required privilege: Datastore.Audit on /datastore/{store}
- `proxmox-cli pbs-datastore catalog`  -  Get the entries of the given path of the catalog.
- `proxmox-cli pbs-datastore config`  -  List all datastores. Required privilege: any authenticated user
- `proxmox-cli pbs-datastore config-get`  -  Read a datastore configuration. Required privilege: Datastore.Audit on /datastore/{name}
- `proxmox-cli pbs-datastore files`  -  List snapshot files.
- `proxmox-cli pbs-datastore gc`  -  Garbage collection status. Required privilege: Datastore.Audit on /datastore/{store}
- `proxmox-cli pbs-datastore group-notes`  -  Get 'notes' for a backup group.
- `proxmox-cli pbs-datastore groups`  -  List backup groups.
- `proxmox-cli pbs-datastore list`  -  Datastore list. Required privilege: any authenticated user
- `proxmox-cli pbs-datastore namespace`  -  List the namespaces of a datastore.
- `proxmox-cli pbs-datastore notes`  -  Get 'notes' for a specific backup.
- `proxmox-cli pbs-datastore protected`  -  Query protection for a specific backup.
- `proxmox-cli pbs-datastore rrd`  -  Read datastore stats. Required privilege: Datastore.Audit, Datastore.Backup (any) on /datastore/{store}
- `proxmox-cli pbs-datastore snapshots`  -  List backup snapshots.
- `proxmox-cli pbs-datastore status`  -  Get datastore status.
- `proxmox-cli pbs-datastore usage`  -  List Datastore usages and estimates. Required privilege: any authenticated user

**pbs_jobs**  -  Manage pbs jobs

- `proxmox-cli pbs-jobs gc`  -  List all GC jobs (max one per datastore) Required privilege: Requires Datastore.Audit or Datastore.Modify on datastore.
- `proxmox-cli pbs-jobs gc-get`  -  List all GC jobs (max one per datastore) Required privilege: Requires Datastore.Audit or Datastore.Modify on datastore.
- `proxmox-cli pbs-jobs prune`  -  List all prune jobs. Required privilege: Requires Datastore.Audit or Datastore.Modify on datastore.
- `proxmox-cli pbs-jobs prune-config`  -  List all scheduled prune jobs. Required privilege: Requires Datastore.Audit.
- `proxmox-cli pbs-jobs prune-config-get`  -  Read a prune job configuration. Required privilege: Requires Datastore.Audit or Datastore.Verify on job's datastore.
- `proxmox-cli pbs-jobs sync`  -  List all configured sync jobs.
- `proxmox-cli pbs-jobs sync-config`  -  List all sync jobs.
- `proxmox-cli pbs-jobs sync-config-get`  -  Read a sync job configuration.
- `proxmox-cli pbs-jobs verify`  -  List all verification jobs. Required privilege: Requires Datastore.Audit or Datastore.Verify on datastore.
- `proxmox-cli pbs-jobs verify-config`  -  List all verification jobs. Required privilege: Requires Datastore.Audit or Datastore.Verify on datastore.
- `proxmox-cli pbs-jobs verify-config-get`  -  Read a verification job configuration.

**pbs_nodes**  -  Manage pbs nodes

- `proxmox-cli pbs-nodes api-version`  -  Proxmox Backup Server API version. Required privilege: any authenticated user
- `proxmox-cli pbs-nodes apt-changelog`  -  Retrieve the changelog of the specified package. Required privilege: Sys.Modify on /
- `proxmox-cli pbs-nodes apt-update`  -  List available APT updates. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-nodes apt-versions`  -  Get package information for important Proxmox Backup Server packages. Required privilege: Sys.Audit on /
- `proxmox-cli pbs-nodes certificates-info`  -  Get certificate info. Required privilege: Sys.Modify on /system/certificates
- `proxmox-cli pbs-nodes disks-directory`  -  List systemd datastore mount units. Required privilege: Sys.Audit on /system/disks
- `proxmox-cli pbs-nodes disks-list`  -  List local disks. Required privilege: Sys.Audit on /system/disks
- `proxmox-cli pbs-nodes disks-smart`  -  Get SMART attributes and health of a disk. Required privilege: Sys.Audit on /system/disks
- `proxmox-cli pbs-nodes disks-zfs`  -  List zfs pools. Required privilege: Sys.Audit on /system/disks
- `proxmox-cli pbs-nodes disks-zfs-get`  -  Get zpool status details. Required privilege: Sys.Audit on /system/disks
- `proxmox-cli pbs-nodes dns`  -  Read DNS settings. Required privilege: Sys.Audit on /system/network/dns
- `proxmox-cli pbs-nodes identity`  -  Returns a unique server identity. The ID is derived from `/etc/machine-id`. Required privilege: any authenticated user
- `proxmox-cli pbs-nodes journal`  -  Read syslog entries. Required privilege: Sys.Audit on /system/log
- `proxmox-cli pbs-nodes list`  -  List Nodes (only for compatibility)
- `proxmox-cli pbs-nodes network`  -  List network interfaces. Required privilege: Sys.Audit on /system/network/interfaces
- `proxmox-cli pbs-nodes network-get`  -  Read a network interface configuration. Required privilege: Sys.Audit on /system/network/interfaces/{iface}
- `proxmox-cli pbs-nodes ping`  -  Dummy method which replies with `{ 'pong': True }`.
- `proxmox-cli pbs-nodes rrd`  -  Read node stats. Required privilege: Sys.Audit on /system/status
- `proxmox-cli pbs-nodes services`  -  Service list. Required privilege: Sys.Audit on /system/services
- `proxmox-cli pbs-nodes services-state`  -  Read service properties. Required privilege: Sys.Audit on /system/services/{service}
- `proxmox-cli pbs-nodes status`  -  Read node memory, CPU and (root) disk usage. Required privilege: Sys.Audit on /system/status
- `proxmox-cli pbs-nodes status-metrics`  -  Return backup server metrics.
- `proxmox-cli pbs-nodes syslog`  -  Read syslog entries. Required privilege: Sys.Audit on /system/log
- `proxmox-cli pbs-nodes tasks`  -  List tasks. Required privilege: Users can only see their own tasks, unless they have Sys.Audit on /system/tasks.
- `proxmox-cli pbs-nodes tasks-log`  -  Read the task log. Required privilege: Users can access their own tasks, or need Sys.Audit on /system/tasks.
- `proxmox-cli pbs-nodes tasks-status`  -  Get task status. Required privilege: Users can access their own tasks, or need Sys.Audit on /system/tasks.
- `proxmox-cli pbs-nodes time`  -  Read server time and time zone settings. Required privilege: any authenticated user

**pbs_notifications**  -  Manage pbs notifications

- `proxmox-cli pbs-notifications endpoints-gotify`  -  List all gotify endpoints. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications endpoints-gotify-get`  -  Get a gotify endpoint. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications endpoints-sendmail`  -  List all sendmail endpoints. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications endpoints-sendmail-get`  -  Get a sendmail endpoint. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications endpoints-smtp`  -  List all smtp endpoints. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications endpoints-smtp-get`  -  Get a smtp endpoint. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications matcher-field-values`  -  List all known, matchable metadata field values. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications matcher-fields`  -  Get all known metadata fields. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications matchers`  -  List all notification matchers. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications matchers-get`  -  Get a notification matcher. Required privilege: Sys.Audit on /system/notifications
- `proxmox-cli pbs-notifications targets`  -  List all notification targets. Required privilege: Sys.Audit on /system/notifications

**pbs_tape**  -  Manage pbs tape

- `proxmox-cli pbs-tape backup`  -  List all tape backup jobs. Required privilege: List configured tape jobs filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape backup-job-config`  -  List all tape backup jobs. Required privilege: List configured tape jobs filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape backup-job-config-get`  -  Read a tape backup job configuration. Required privilege: Tape.Audit on /tape/job/{id}
- `proxmox-cli pbs-tape changer`  -  List changers. Required privilege: List configured tape changer filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape changer-config`  -  List changers. Required privilege: List configured tape changer filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape changer-config-get`  -  Get tape changer configuration. Required privilege: Tape.Audit on /tape/device/{name}
- `proxmox-cli pbs-tape drive`  -  List drives. Required privilege: List configured tape drives filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape drive-cartridge-memory`  -  Read Cartridge Memory (Medium auxiliary memory attributes) Required privilege: Tape.Audit on /tape/device/{drive}
- `proxmox-cli pbs-tape drive-config`  -  List drives. Required privilege: List configured tape drives filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape drive-config-get`  -  Get drive configuration. Required privilege: Tape.Audit on /tape/device/{name}
- `proxmox-cli pbs-tape drive-status`  -  Get drive/media status. Required privilege: Tape.Audit on /tape/device/{drive}
- `proxmox-cli pbs-tape drive-volume-statistics`  -  Read Volume Statistics (SCSI log page 17h) Required privilege: Tape.Audit on /tape/device/{drive}
- `proxmox-cli pbs-tape encryption-keys`  -  List existing keys. Required privilege: Tape.Audit on /tape/pool
- `proxmox-cli pbs-tape encryption-keys-get`  -  Get key config (public key part) Required privilege: Tape.Audit on /tape/pool
- `proxmox-cli pbs-tape media-content`  -  List media content. Required privilege: List content filtered by Tape.Audit privilege on pool
- `proxmox-cli pbs-tape media-list-status`  -  Get current media status.
- `proxmox-cli pbs-tape media-media-sets`  -  List Media sets. Required privilege: List of media sets filtered by Tape.Audit privileges on pool
- `proxmox-cli pbs-tape media-pool-config`  -  List media pools. Required privilege: List configured media pools filtered by Tape.Audit privileges
- `proxmox-cli pbs-tape media-pool-config-get`  -  Get media pool configuration. Required privilege: Tape.Audit on /tape/pool/{name}
- `proxmox-cli pbs-tape scan-changers`  -  Scan for SCSI tape changers.
- `proxmox-cli pbs-tape scan-drives`  -  Scan tape drives.

**pools**  -  Manage pools

- `proxmox-cli pools get`  -  Get pool configuration (deprecated, no support for nested pools, use 'GET /pools/?poolid={poolid}').
- `proxmox-cli pools list`  -  List pools or get pool configuration.

**replication**  -  Manage replication

- `proxmox-cli replication get`  -  Read replication job configuration. Required privilege: Requires the VM.Audit permission on /vms/<vmid>.
- `proxmox-cli replication list`  -  List replication jobs.
- `proxmox-cli replication node`  -  List status of all replication jobs on this node. Required privilege: Requires the VM.Audit permission on /vms/<vmid>.
- `proxmox-cli replication node-log`  -  Read replication job log.
- `proxmox-cli replication node-status`  -  Get replication job status. Required privilege: Requires the VM.Audit permission on /vms/<vmid>.

**sdn**  -  Manage sdn

- `proxmox-cli sdn controllers`  -  SDN controllers index.
- `proxmox-cli sdn controllers-get`  -  Read sdn controller configuration. Required privilege: SDN.Allocate on /sdn/controllers/{controller}
- `proxmox-cli sdn fabrics-all`  -  SDN Fabrics Index.
- `proxmox-cli sdn fabrics-fabric`  -  SDN Fabrics Index.
- `proxmox-cli sdn fabrics-fabric-get`  -  Read a fabric's configuration. (The upstream schema describes this GET as 'Update a fabric.'; it is the read.)
- `proxmox-cli sdn fabrics-node`  -  SDN Fabrics Index.
- `proxmox-cli sdn fabrics-node-get`  -  SDN Fabrics Index. Required privilege: Only returns nodes where you have 'Sys.Audit' or 'Sys.Modify' permissions.
- `proxmox-cli sdn fabrics-node-node-id-get`  -  Get a node.
- `proxmox-cli sdn ipams-status`  -  List PVE IPAM Entries.
- `proxmox-cli sdn list`  -  Directory index. Required privilege: SDN.Audit on /sdn
- `proxmox-cli sdn node-fabrics-interfaces`  -  Get all interfaces for a fabric. Required privilege: SDN.Audit on /sdn/fabrics/{fabric}
- `proxmox-cli sdn node-fabrics-neighbors`  -  Get all neighbors for a fabric. Required privilege: SDN.Audit on /sdn/fabrics/{fabric}
- `proxmox-cli sdn node-fabrics-routes`  -  Get all routes for a fabric. Required privilege: SDN.Audit on /sdn/fabrics/{fabric}
- `proxmox-cli sdn node-vnets-mac-vrf`  -  Get the MAC VRF for a VNet in an EVPN zone.
- `proxmox-cli sdn node-zones`  -  Get status for all zones. Required privilege: Only list entries where you have 'SDN.Audit'
- `proxmox-cli sdn node-zones-bridges`  -  Get a list of all bridges (vnets) that are part of a zone, as well as the ports that are members of that bridge.
- `proxmox-cli sdn node-zones-content`  -  List zone content. Required privilege: SDN.Audit on /sdn/zones/{zone}
- `proxmox-cli sdn node-zones-ip-vrf`  -  Get the IP VRF of an EVPN zone. Required privilege: SDN.Audit on /sdn/zones/{zone}
- `proxmox-cli sdn prefix-lists`  -  List Prefix Lists.
- `proxmox-cli sdn prefix-lists-entries`  -  List Prefix List Entries. Required privilege: SDN.Audit on /sdn/prefix-lists/{id}
- `proxmox-cli sdn prefix-lists-entries-get`  -  Get Prefix List Entry. Required privilege: SDN.Audit on /sdn/prefix-lists/{id}
- `proxmox-cli sdn prefix-lists-get`  -  Get Prefix List. Required privilege: SDN.Audit on /sdn/prefix-lists/{id}
- `proxmox-cli sdn route-maps`  -  List Route Maps. Required privilege: Only returns route maps where you have 'SDN.Audit' or 'SDN.Allocate' permissions.
- `proxmox-cli sdn route-maps-entries`  -  Lists all route map entries.
- `proxmox-cli sdn route-maps-entries-entry-get`  -  Get Route Map Entry. Required privilege: SDN.Audit, SDN.Allocate (any) on /sdn/route-maps/{route-map-id}
- `proxmox-cli sdn route-maps-entries-get`  -  List all entries for a given Route Map.
- `proxmox-cli sdn vnets`  -  SDN vnets index.
- `proxmox-cli sdn vnets-firewall-options`  -  Get vnet firewall options. Required privilege: Needs SDN.Audit or SDN.Allocate permissions on '/sdn/zones/<zone>/<vnet>'
- `proxmox-cli sdn vnets-firewall-rules`  -  List rules. Required privilege: Needs SDN.Audit or SDN.Allocate permissions on '/sdn/zones/<zone>/<vnet>'
- `proxmox-cli sdn vnets-firewall-rules-get`  -  Get single rule data. Required privilege: Needs SDN.Audit or SDN.Allocate permissions on '/sdn/zones/<zone>/<vnet>'
- `proxmox-cli sdn vnets-get`  -  Read sdn vnet configuration.
- `proxmox-cli sdn vnets-subnets`  -  SDN subnets index.
- `proxmox-cli sdn vnets-subnets-get`  -  Read sdn subnet configuration.
- `proxmox-cli sdn zones`  -  SDN zones index.
- `proxmox-cli sdn zones-get`  -  Read sdn zone configuration. Required privilege: SDN.Allocate on /sdn/zones/{zone}

**storage**  -  Manage storage

- `proxmox-cli storage get`  -  Read storage configuration. Required privilege: Datastore.Allocate on /storage/{storage}
- `proxmox-cli storage list`  -  Storage index.
- `proxmox-cli storage node`  -  Get status for all datastores.
- `proxmox-cli storage node-content`  -  List storage content. Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}
- `proxmox-cli storage node-content-get`  -  Get volume attributes. Required privilege: You need read access for the volume.
- `proxmox-cli storage node-identity`  -  Return identity information for this storage instance.
- `proxmox-cli storage node-import-metadata`  -  Get the base parameters for creating a guest which imports data from a foreign importable guest, like an ESXi VM.
- `proxmox-cli storage node-prunebackups`  -  Get prune information for backups.
- `proxmox-cli storage node-rrd`  -  Read storage RRD statistics (returns PNG).
- `proxmox-cli storage node-rrddata`  -  Read storage RRD statistics. Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}
- `proxmox-cli storage node-status`  -  Read storage status. Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}

**tasks**  -  Manage tasks

- `proxmox-cli tasks cluster`  -  List recent tasks (cluster wide). Required privilege: any authenticated user
- `proxmox-cli tasks get`  -  Read /nodes/{node}/tasks/{upid}. Required privilege: any authenticated user
- `proxmox-cli tasks list`  -  Read task list for one node (finished tasks).
- `proxmox-cli tasks log`  -  Read task log.
- `proxmox-cli tasks status`  -  Read task status.

**vm**  -  Manage vm

- `proxmox-cli vm agent-get-fsinfo`  -  Execute get-fsinfo. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-host-name`  -  Execute get-host-name. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-memory-block-info`  -  Execute get-memory-block-info. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-memory-blocks`  -  Execute get-memory-blocks. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-osinfo`  -  Execute get-osinfo. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-time`  -  Execute get-time. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-timezone`  -  Execute get-timezone. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-users`  -  Execute get-users. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-get-vcpus`  -  Execute get-vcpus. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-info`  -  Execute info. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm agent-network-get-interfaces`  -  Execute network-get-interfaces. Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- `proxmox-cli vm cloudinit`  -  Get the cloudinit configuration with both current and pending values. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm config`  -  Get the virtual machine configuration with pending configuration changes applied.
- `proxmox-cli vm feature`  -  Check if feature for virtual machine is available. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-aliases`  -  List aliases. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-aliases-get`  -  Read alias. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-ipset`  -  List IPSets. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-ipset-cidr-get`  -  Read IP or Network settings from IPSet. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-ipset-get`  -  List IPSet content. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-log`  -  Read firewall log. Required privilege: VM.Console on /vms/{vmid}
- `proxmox-cli vm firewall-options`  -  Get VM firewall options. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-refs`  -  Lists possible IPSet/Alias reference which are allowed in source/dest properties.
- `proxmox-cli vm firewall-rules`  -  List rules. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm firewall-rules-get`  -  Get single rule data. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm list`  -  Virtual machine index (per node). Required privilege: Only list VMs where you have VM.Audit permissions on /vms/<vmid>.
- `proxmox-cli vm migrate`  -  Get preconditions for migration. Required privilege: VM.Migrate on /vms/{vmid}
- `proxmox-cli vm pending`  -  Get the virtual machine configuration with both current and pending values. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm rrd`  -  Read VM RRD statistics (returns PNG) Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm rrddata`  -  Read VM RRD statistics. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm snapshot`  -  List all snapshots. Required privilege: VM.Audit on /vms/{vmid}
- `proxmox-cli vm snapshot-config`  -  Get snapshot configuration. Required privilege: VM.Snapshot, VM.Snapshot.Rollback, VM.Audit (any) on /vms/{vmid}
- `proxmox-cli vm snapshot-get`  -  Read /nodes/{node}/qemu/{vmid}/snapshot/{snapname}. Required privilege: any authenticated user
- `proxmox-cli vm status-current`  -  Get virtual machine status. Required privilege: VM.Audit on /vms/{vmid}


**backup**


- `proxmox-cli backup coverage`  -  Every guest or disk that no enabled backup job includes, with the reason: no job, disabled job only, excluded, or disk excluded.


**capacity**


- `proxmox-cli capacity`  -  PBS datastores ranked by days until full, plus every PVE backup storage by percent used (only PBS forecasts), with garbage-collection and prune state and a GC-deadlock warning.


**failures**


- `proxmox-cli failures`  -  Failed and silently incomplete backup, verify, GC, sync and prune tasks in a time window, one row per failed guest or datastore with the reason line.


**posture**


- `proxmox-cli posture`  -  One verdict per guest across every client cluster: covered by an enabled job, age of the newest real restore point, PBS verify state, last backup attempt.


**restore-points**


- `proxmox-cli restore-points`  -  Every restore point of one guest across all storages and PBS namespaces, with verify state, protection, encryption and, for points listed from PVE storage, whether the next prune removes it.


**rpo**


- `proxmox-cli rpo`  -  Per guest over a past window: largest gap between restore points, scheduled runs with no restore point, and days with a verified-good point, rolled up per client.


**verify-health**


- `proxmox-cli verify-health`  -  Per PBS datastore and backup group: failed, never-verified and outdated snapshots, first failure, newest verified-good point, and namespaces no verify job reaches. Needs the PBS API.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
proxmox-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Red guests only, compact for an agent

```bash
proxmox-cli posture --only red --agent --select rows.cluster,rows.asset_name,rows.verdict,rows.age_hours,rows.verify_state
```

Only red verdicts, narrowed to the fields an agent needs to open tickets.

### Per-client weekly backup line as CSV

```bash
proxmox-cli posture --by-client --csv
```

One row per client with counts of fresh, stale, never and uncovered guests.

### How far has a verify failure spread

```bash
proxmox-cli verify-health --store main --agent
```

Failed and never-verified snapshot counts per backup group on one datastore.

### Can we restore VM 104 from Tuesday

```bash
proxmox-cli restore-points 104 --cluster acme
```

Every restore point of guest 104 with verify state and prune marks.

### Datastores filling within a month or carrying a risk flag

```bash
proxmox-cli capacity --within 30d
```

Ranked by days to full; also lists storage flagged gc-deadlock-risk or unpruned regardless of free space.

## Auth Setup

Create a privilege-separated API token. PVE: PVEAuditor on /. Proxmox VE lists backup volumes only to a token that also holds Datastore.AllocateSpace and VM.Backup (an audit-only token sees an empty list, which this CLI reports as an unread storage and an unproven guest, never as "no backups"), so when backups live on dir, NFS or CIFS storage, or PBS is not configured here, add a custom role with those two privileges on /; the CLI never sends a request that uses them. PBS: the Audit role on / (read-only; its Sys.Audit on /system/tasks is what lets failures see verify, GC, sync and prune tasks). When PROXMOX_PBS_HOST is set (spelled as in the PVE storage's server field: the same IP or hostname), PBS-backed archives are read straight from the PBS API and the PVE token can stay audit-only. Set PROXMOX_API_TOKEN to USER@REALM!TOKENID=SECRET and PROXMOX_HOST to the PVE host (or PROXMOX_BASE_URL for a full https://host:port/api2/json); for PBS set PROXMOX_PBS_API_TOKEN to USER@REALM!TOKENID:SECRET (colon) and PROXMOX_PBS_HOST. Proxmox ships self-signed certificates: pin them with PROXMOX_TLS_FINGERPRINT and PROXMOX_PBS_TLS_FINGERPRINT (the SHA-256 fingerprint shown under the node's Certificates; a malformed pin is an error, never ignored); there is no insecure mode. For many clients, add one entry per cluster with 'clusters add' (clusters.toml in the config directory, or PROXMOX_CLUSTERS_FILE) and select one with --cluster or PROXMOX_CLUSTER. clusters.toml stores only the names of the env vars that hold tokens; 'auth set-token' is the only path that saves a token, to credentials.toml.

Run `proxmox-cli doctor` to verify the PROXMOX_* setup, and `proxmox-cli clusters check` to probe each configured cluster's PVE and PBS (host, certificate pin, token).

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
  proxmox-cli ct list pve1 --agent --select cpu,cpus,disk
  ```
- **Previewable**  -  `--dry-run` on endpoint commands shows the request without sending; fleet commands (posture, failures, ...) only confirm what would run
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear

### Response envelope

Under `--agent`, commands that read from the local store or the API wrap output in a provenance envelope (`--json` alone emits the bare payload; the fleet commands' own `rows`/`fetch_failures` object sits inside `.results`):

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `PROXMOX_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `PROXMOX_CONFIG_DIR`, `PROXMOX_DATA_DIR`, `PROXMOX_STATE_DIR`, `PROXMOX_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `PROXMOX_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `proxmox-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "proxmox": {
        "command": "proxmox-mcp",
        "env": {
          "PROXMOX_HOME": "/srv/proxmox"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `PROXMOX_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `PROXMOX_HOME`, or `doctor` will not find credentials left under the former root.

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
proxmox-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "proxmox-cli learnings confirm 12"] }
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
       materially more, record the divergence via `proxmox-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `proxmox-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `proxmox-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
proxmox-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
proxmox-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
proxmox-cli teach-playbook \
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
proxmox-cli playbook amend \
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

`proxmox-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `PROXMOX_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
proxmox-cli feedback "the --since flag is inclusive but docs say exclusive"
proxmox-cli feedback --stdin < notes.txt
proxmox-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `PROXMOX_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PROXMOX_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
proxmox-cli profile save briefing --json
proxmox-cli --profile briefing ct list pve1
proxmox-cli profile list --json
proxmox-cli profile show briefing
proxmox-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Per-cluster local store and CSV

Each `--cluster` (or `PROXMOX_CLUSTER`) keeps its own local store, so synced rows of one client never answer for another. The MCP `sql` and `search` tools take no cluster argument: they read the store of the `PROXMOX_CLUSTER` set in the MCP server's environment (the default store when unset), so set it there to query one client's mirror. `--csv` is a flat projection of rows: read `evidence`, `not_read` and `fetch_failures` from the JSON output when completeness matters.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Gate tripped (`posture --fail-on-red`, `doctor --fail-on`, a failed `clusters check`) |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

The fleet commands (posture, backup coverage, verify-health, failures, capacity, restore-points, rpo) report a rejected token as that cluster's fetch failure: they exit 5 when no cluster answered, never 4.

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `proxmox-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add proxmox-mcp -- proxmox-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which proxmox-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   proxmox-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `proxmox-cli <command> --help`.
