# Proxmox VE + Backup Server CLI

**Read-only Proxmox VE + Backup Server CLI: per-guest backup verdicts across every cluster, no write calls to Proxmox.**

Every safe read endpoint of Proxmox VE and Proxmox Backup Server (a few unsafe GETs are excluded by design) as a typed command, with no write calls to Proxmox, so nothing an agent runs can change a cluster or a backup. On top: posture joins job coverage, real archive age and PBS verify state per guest across clusters; backup coverage, verify-health, failures, capacity, restore-points and rpo answer the questions MSPs ask every morning.

Learn more at [Proxmox VE + Backup Server](https://www.proxmox.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `proxmox-cli` and `proxmox-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=proxmox). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/proxmox --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/proxmox --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `proxmox-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the proxmox skill from https://github.com/Servosity/msp-skills/tree/main/skills/proxmox. The skill defines how its required CLI (`proxmox-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=proxmox).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in the prompts: `PROXMOX_API_TOKEN` and `PROXMOX_HOST`, plus `PROXMOX_TLS_FINGERPRINT` for a self-signed certificate. The PBS host, token and fingerprint are optional.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "proxmox": {
      "command": "proxmox-mcp",
      "env": {
        "PROXMOX_HOST": "<pve-host>",
        "PROXMOX_API_TOKEN": "<USER@REALM!TOKENID=SECRET>",
        "PROXMOX_TLS_FINGERPRINT": "<PVE certificate SHA-256>",
        "PROXMOX_PBS_HOST": "<optional: pbs-host>",
        "PROXMOX_PBS_API_TOKEN": "<optional: USER@REALM!TOKENID:SECRET>",
        "PROXMOX_PBS_TLS_FINGERPRINT": "<optional: PBS certificate SHA-256>"
      }
    }
  }
}
```

</details>

## Authentication

Create a privilege-separated API token. PVE: PVEAuditor on /. Proxmox VE lists backup volumes only to a token that also holds Datastore.AllocateSpace and VM.Backup (an audit-only token sees an empty list, which this CLI reports as an unread storage, never as "no backups"), so when backups live on dir, NFS or CIFS storage, or PBS is not configured here, add a custom role with those two privileges on /; the CLI never sends a request that uses them. PBS: the Audit role on / (read-only; its Sys.Audit on /system/tasks is what lets failures see verify, GC, sync and prune tasks). When PROXMOX_PBS_HOST is set, PBS-backed archives are read straight from the PBS API and the PVE token can stay audit-only. Set PROXMOX_API_TOKEN to USER@REALM!TOKENID=SECRET and PROXMOX_HOST to the PVE host (or PROXMOX_BASE_URL for a full https://host:port/api2/json); for PBS set PROXMOX_PBS_API_TOKEN to USER@REALM!TOKENID:SECRET (colon) and PROXMOX_PBS_HOST. Proxmox ships self-signed certificates: pin them with PROXMOX_TLS_FINGERPRINT and PROXMOX_PBS_TLS_FINGERPRINT (the SHA-256 fingerprint shown under the node's Certificates); there is no insecure mode. For many clients, add one entry per cluster with 'clusters add' (clusters.toml in the config directory, or PROXMOX_CLUSTERS_FILE) and select one with --cluster or PROXMOX_CLUSTER. clusters.toml stores only the names of the env vars that hold tokens; 'auth set-token' is the only path that saves a token, to credentials.toml.

## Quick Start

```bash
# Health report: config, credentials, certificate pins and reachability for PVE and PBS
proxmox-cli doctor

# Register one client cluster; fingerprints are the SHA-256 values shown under each node's Certificates (stock Proxmox is self-signed); the file stores env var names, never tokens
proxmox-cli clusters add --client "Acme Dental" --pve-host pve1.acme.lan --pve-token-env ACME_PVE_TOKEN --pve-fingerprint "$ACME_PVE_FP" --pbs-host pbs.acme.lan --pbs-token-env ACME_PBS_TOKEN --pbs-fingerprint "$ACME_PBS_FP" acme

# One backup verdict per guest across every configured cluster
proxmox-cli posture --agent

# Last night's failed or incomplete backup, verify, GC and sync work
proxmox-cli failures --since 24h

# Guests and disks no enabled job includes, with the reason
proxmox-cli backup coverage --cluster acme

```

## Unique Features

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

  _Use for morning triage instead of reading backup emails; it names the guest and the error, not just the task. It reads at most --max-logs task logs per cluster (default 50); log_cap_hit says when that limit was reached. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli failures --since 24h --agent
  ```

### PBS health
- **`verify-health`**  -  Per PBS datastore and backup group: failed, never-verified and outdated snapshots, first failure, newest verified-good point, and namespaces no verify job reaches. Needs the PBS API.

  _Use when verify failures appear, to see how far they spread and which groups still have a good restore point. Needs PROXMOX_PBS_HOST and PROXMOX_PBS_API_TOKEN (or pbs_host in clusters.toml); empty groups with a non-empty skipped_clusters means nothing was checked, not that nothing failed._

  ```bash
  proxmox-cli verify-health --agent
  ```
- **`capacity`**  -  Every PBS datastore and PVE backup storage ranked by days until full, with garbage-collection and prune state and a GC-deadlock warning.

  _Use weekly to catch a datastore before it fills past the point where garbage collection can still run. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli capacity --within 30d --agent
  ```

### Restore readiness
- **`restore-points`**  -  Every restore point of one guest across all storages and PBS namespaces, with verify state, protection, encryption and whether the next prune removes it.

  _Use to answer 'can we restore server X from Tuesday?' with the exact restore points and whether each verified. Exit 3 means no reachable cluster has that guest; exit 5 means no cluster answered._

  ```bash
  proxmox-cli restore-points 104 --agent
  ```
- **`rpo`**  -  Per guest over a past window: largest gap between restore points, scheduled runs with no restore point, and days with a verified-good point, rolled up per client.

  _Use for weekly SLA reviews and QBR or compliance evidence that every scheduled backup produced a restore point. fetch_failures lists clusters or calls that did not answer; when no cluster answers it exits 5._

  ```bash
  proxmox-cli rpo --days 7 --agent
  ```

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

## Usage

Run `proxmox-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `PROXMOX_CONFIG_DIR`, `PROXMOX_DATA_DIR`, `PROXMOX_STATE_DIR`, or `PROXMOX_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `PROXMOX_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export PROXMOX_HOME=/srv/proxmox
proxmox-cli doctor
```

Under `PROXMOX_HOME=/srv/proxmox`, the four dirs resolve to `/srv/proxmox/config`, `/srv/proxmox/data`, `/srv/proxmox/state`, and `/srv/proxmox/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `PROXMOX_DATA_DIR` overrides an explicit `--home` for that kind. Use `PROXMOX_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `PROXMOX_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `proxmox-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### access

Manage access

- **`proxmox-cli access acl`** - Get Access Control List (ACLs).

Required privilege: The returned list is restricted to objects where you have rights to modify permissions.
- **`proxmox-cli access domains`** - Authentication domain index.

Required privilege: Anyone can access that, because we need that list for the login box (before the user is authenticated).
- **`proxmox-cli access groups`** - Group index.
- **`proxmox-cli access groups-get`** - Get group configuration.

Required privilege: Sys.Audit, Group.Allocate (any) on /access/groups
- **`proxmox-cli access permissions`** - Retrieve effective permissions of given user/token.

Required privilege: Each user/token is allowed to dump their own permissions (or that of owned tokens). A user can dump the permissions of another user or their tokens if they have 'Sys.Audit' permission on /access.
- **`proxmox-cli access roles`** - Role index.

Required privilege: any authenticated user
- **`proxmox-cli access roles-get`** - Get role configuration.

Required privilege: any authenticated user
- **`proxmox-cli access tfa`** - List TFA configurations of users.

Required privilege: Returns all or just the logged-in user, depending on privileges.
- **`proxmox-cli access tfa-get`** - List TFA configurations of users.

Required privilege: the user named by {userid} (self) or user or group management privileges on /access
- **`proxmox-cli access tfa-id-get`** - Fetch a requested TFA entry if present.

Required privilege: the user named by {userid} (self) or user or group management privileges on /access
- **`proxmox-cli access users`** - User index.
- **`proxmox-cli access users-get`** - Get user configuration.

Required privilege: user or group management privileges on /access
- **`proxmox-cli access users-tfa`** - Get user TFA types (Personal and Realm).

Required privilege: the user named by {userid} (self) or user or group management privileges on /access
- **`proxmox-cli access users-token`** - Get user API tokens.

Required privilege: the user named by {userid} (self) or user or group management privileges on /access
- **`proxmox-cli access users-token-get`** - Get specific API token information.

Required privilege: the user named by {userid} (self) or user or group management privileges on /access

### acme

Manage acme

- **`proxmox-cli acme account`** - ACMEAccount index.

Required privilege: any authenticated user
- **`proxmox-cli acme account-get`** - Return existing ACME account information.
- **`proxmox-cli acme challenge-schema`** - Get schema of ACME challenge types.

Required privilege: any authenticated user
- **`proxmox-cli acme directories`** - Get named known ACME directory endpoints.

Required privilege: any authenticated user

### backup

Manage backup

- **`proxmox-cli backup job`** - List vzdump backup schedule.

Required privilege: Sys.Audit on /
- **`proxmox-cli backup job-get`** - Read vzdump backup job definition.

Required privilege: Sys.Audit on /
- **`proxmox-cli backup job-included-volumes`** - Returns included guests and the backup status of their disks. Optimized to be used in ExtJS tree views.

Required privilege: Sys.Audit on /
- **`proxmox-cli backup not-backed-up`** - Shows all guests which are not covered by any backup job.

Required privilege: Sys.Audit on /
- **`proxmox-cli backup vzdump-defaults`** - Get the currently configured vzdump defaults.

### ceph

Manage ceph

- **`proxmox-cli ceph cluster-flags`** - get the status of all ceph flags.

Required privilege: Sys.Audit on /
- **`proxmox-cli ceph cluster-flags-get`** - Get the status of a specific ceph flag.

Required privilege: Sys.Audit on /
- **`proxmox-cli ceph cluster-health-mute`** - Get the currently muted Ceph health checks.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph cluster-metadata`** - Get ceph metadata.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph cluster-status`** - Get ceph status.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph cmd-safety`** - Heuristical check if it is safe to perform an action.

Required privilege: Sys.Audit on /
- **`proxmox-cli ceph crush`** - Get OSD crush map.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph fs`** - Directory index.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph log`** - Read ceph log.

Required privilege: Sys.Syslog on /nodes/{node}
- **`proxmox-cli ceph mds`** - MDS directory index.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph mgr`** - MGR directory index.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph mon`** - Get Ceph monitor list.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph osd`** - Get Ceph osd list/tree.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph osd-lv-info`** - Get OSD volume details.

Required privilege: Sys.Audit on /
- **`proxmox-cli ceph osd-metadata`** - Get OSD details.

Required privilege: Sys.Audit on /
- **`proxmox-cli ceph pool`** - List all pools and their settings (which are settable by the POST/PUT endpoints).
- **`proxmox-cli ceph pool-status`** - Show the current pool status.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph releases`** - List all known Ceph releases, marking which ones can be installed on this node.
- **`proxmox-cli ceph rules`** - List ceph rules.

Required privilege: Sys.Audit, Datastore.Audit (any) on /
- **`proxmox-cli ceph status`** - Get the Ceph cluster status (raw 'ceph status' output). The response is cluster-wide and identical to /cluster/ceph/status; this node-level alias exists for operator convenience.

Required privilege: Sys.Audit, Datastore.Audit (any) on /

### cluster

Manage cluster

- **`proxmox-cli cluster api-version`** - API version details, including some parts of the global datacenter config.

Required privilege: any authenticated user
- **`proxmox-cli cluster config-apiversion`** - Return the version of the cluster join API available on this node.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster config-join`** - Get information needed to join this cluster over the connected node.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster config-nodes`** - Corosync node list.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster config-qdevice`** - Get QDevice status.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster config-totem`** - Get corosync totem protocol settings.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster jobs-realm-sync`** - List configured realm-sync-jobs.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster jobs-realm-sync-get`** - Read realm-sync job definition.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster jobs-schedule-analyze`** - Returns a list of future schedule runtimes.

Required privilege: any authenticated user
- **`proxmox-cli cluster log`** - Read cluster log.

Required privilege: The user needs 'Sys.Syslog' on '/' in order to get all logs.
- **`proxmox-cli cluster metrics-export`** - Retrieve metrics of the cluster.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster metrics-server`** - List configured metric servers.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster metrics-server-get`** - Read metric server configuration.

Required privilege: Sys.Audit on /
- **`proxmox-cli cluster nextid`** - Get next free VMID. Pass a VMID to assert that its free (at time of check).

Required privilege: any authenticated user
- **`proxmox-cli cluster qemu-cpu-flags`** - List of available CPU flags. Currently only implemented for x86_64, returns an empty list for aarch64.

Required privilege: Sys.Audit on /nodes or Mapping.Audit, Mapping.Use, Mapping.Modify (any) on /mapping/cpu
- **`proxmox-cli cluster qemu-custom-cpu-models`** - List all custom CPU model definitions visible to the user.

Required privilege: Only lists entries where the user has 'Mapping.Modify', 'Mapping.Use' or 'Mapping.Audit' permissions on '/mapping/cpu/<name>'.
- **`proxmox-cli cluster qemu-custom-cpu-models-get`** - Retrieve details about a specific custom CPU model.
- **`proxmox-cli cluster resources`** - Resources index (cluster wide).

Required privilege: any authenticated user
- **`proxmox-cli cluster status`** - Get cluster status information.

Required privilege: Sys.Audit on /

### ct

Manage ct

- **`proxmox-cli ct config`** - Get container configuration.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct feature`** - Check if feature for virtual machine is available.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-aliases`** - List aliases.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-aliases-get`** - Read alias.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-ipset`** - List IPSets.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-ipset-cidr-get`** - Read IP or Network settings from IPSet.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-ipset-get`** - List IPSet content.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-log`** - Read firewall log.

Required privilege: VM.Console on /vms/{vmid}
- **`proxmox-cli ct firewall-options`** - Get VM firewall options.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-refs`** - Lists possible IPSet/Alias reference which are allowed in source/dest properties.
- **`proxmox-cli ct firewall-rules`** - List rules.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct firewall-rules-get`** - Get single rule data.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct interfaces`** - Get IP addresses of the specified container interface.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct list`** - LXC container index (per node).

Required privilege: Only list CTs where you have VM.Audit permission on /vms/<vmid>.
- **`proxmox-cli ct migrate`** - Get preconditions for migration.

Required privilege: VM.Migrate on /vms/{vmid}
- **`proxmox-cli ct pending`** - Get container configuration, including pending changes.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct rrd`** - Read VM RRD statistics (returns PNG)

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct rrddata`** - Read VM RRD statistics.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct snapshot`** - List all snapshots.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli ct snapshot-config`** - Get snapshot configuration.

Required privilege: VM.Snapshot, VM.Snapshot.Rollback, VM.Audit (any) on /vms/{vmid}
- **`proxmox-cli ct snapshot-get`** - Read /nodes/{node}/lxc/{vmid}/snapshot/{snapname}.

Required privilege: any authenticated user
- **`proxmox-cli ct status-current`** - Get virtual machine status.

Required privilege: VM.Audit on /vms/{vmid}

### disks

Manage disks

- **`proxmox-cli disks directory`** - PVE Managed Directory storages.

Required privilege: Sys.Audit on /
- **`proxmox-cli disks list`** - List local disks.

Required privilege: Sys.Audit on / or Sys.Audit on /nodes/{node}
- **`proxmox-cli disks lvm`** - List LVM Volume Groups.

Required privilege: Sys.Audit on /
- **`proxmox-cli disks lvmthin`** - List LVM thinpools.

Required privilege: Sys.Audit on /
- **`proxmox-cli disks scan`** - Index of available scan methods.

Required privilege: any authenticated user
- **`proxmox-cli disks scan-lvm`** - List local LVM volume groups.

Required privilege: Datastore.Allocate on /storage
- **`proxmox-cli disks scan-lvmthin`** - List local LVM Thin Pools.

Required privilege: Datastore.Allocate on /storage
- **`proxmox-cli disks scan-zfs`** - Scan zfs pool list on local node.

Required privilege: Datastore.Allocate on /storage
- **`proxmox-cli disks smart`** - Get SMART Health of a disk.

Required privilege: Sys.Audit on /
- **`proxmox-cli disks zfs`** - List Zpools.

Required privilege: Sys.Audit on /
- **`proxmox-cli disks zfs-get`** - Get details about a zpool.

Required privilege: Sys.Audit on /

### firewall

Manage firewall

- **`proxmox-cli firewall aliases`** - List aliases.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall aliases-get`** - Read alias.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall groups`** - List security groups.

Required privilege: any authenticated user
- **`proxmox-cli firewall groups-get`** - List rules.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall groups-pos-get`** - Get single rule data.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall ipset`** - List IPSets.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall ipset-cidr-get`** - Read IP or Network settings from IPSet.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall ipset-get`** - List IPSet content.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall macros`** - List available macros.

Required privilege: any authenticated user
- **`proxmox-cli firewall node-log`** - Read firewall log.

Required privilege: Sys.Syslog on /nodes/{node}
- **`proxmox-cli firewall node-options`** - Get host firewall options.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli firewall node-rules`** - List rules.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli firewall node-rules-get`** - Get single rule data.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli firewall options`** - Get Firewall options.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall refs`** - Lists possible IPSet/Alias reference which are allowed in source/dest properties.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall rules`** - List rules.

Required privilege: Sys.Audit on /
- **`proxmox-cli firewall rules-get`** - Get single rule data.

Required privilege: Sys.Audit on /

### ha

Manage ha

- **`proxmox-cli ha groups`** - Get HA groups. (deprecated in favor of HA rules)

Required privilege: Sys.Audit on /
- **`proxmox-cli ha groups-get`** - Read ha group configuration. (deprecated in favor of HA rules)

Required privilege: Sys.Audit on /
- **`proxmox-cli ha list`** - Directory index.

Required privilege: Sys.Audit on /
- **`proxmox-cli ha resources`** - List HA resources.

Required privilege: Sys.Audit on /
- **`proxmox-cli ha resources-get`** - Read resource configuration.

Required privilege: Sys.Audit on /
- **`proxmox-cli ha rules`** - Get HA rules.

Required privilege: Sys.Audit on /
- **`proxmox-cli ha rules-get`** - Read HA rule.

Required privilege: Sys.Audit on /
- **`proxmox-cli ha status-current`** - Get HA manager status.

Required privilege: Sys.Audit on /
- **`proxmox-cli ha status-manager-status`** - Get full HA manager status, including LRM status.

Required privilege: Sys.Audit on /

### mapping

Manage mapping

- **`proxmox-cli mapping dir`** - List directory mapping.
- **`proxmox-cli mapping dir-get`** - Get directory mapping.
- **`proxmox-cli mapping pci`** - List PCI Hardware Mapping.
- **`proxmox-cli mapping pci-get`** - Get PCI Mapping.
- **`proxmox-cli mapping usb`** - List USB Hardware Mappings.
- **`proxmox-cli mapping usb-get`** - Get USB Mapping.

### nodes

Manage nodes

- **`proxmox-cli nodes aplinfo`** - Get list of appliances.

Required privilege: any authenticated user
- **`proxmox-cli nodes apt`** - Directory index for apt (Advanced Package Tool).

Required privilege: any authenticated user
- **`proxmox-cli nodes apt-changelog`** - Get package changelogs.

Required privilege: Sys.Modify on /nodes/{node}
- **`proxmox-cli nodes apt-update`** - List available updates.

Required privilege: Sys.Modify on /nodes/{node}
- **`proxmox-cli nodes apt-versions`** - Get package information for important Proxmox packages.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes capabilities-qemu-cpu`** - List all custom and default CPU models.

Required privilege: Custom models are filtered to those the current user has any of Mapping.{Audit,Use,Modify} on /mapping/cpu/<name>; Sys.Audit on /nodes continues to grant visibility of all custom models for back-compat.
- **`proxmox-cli nodes capabilities-qemu-cpu-flags`** - List of available VM-specific CPU flags. Returns an empty list for 'aarch64' as no VM-specific flags are defined for it yet.

Required privilege: any authenticated user
- **`proxmox-cli nodes capabilities-qemu-machines`** - Get available QEMU/KVM machine types.

Required privilege: any authenticated user
- **`proxmox-cli nodes capabilities-qemu-migration`** - Get node-specific QEMU migration capabilities of the node. Requires the 'Sys.Audit' permission on '/nodes/<node>'.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes certificates-info`** - Get information about node's certificates.

Required privilege: any authenticated user
- **`proxmox-cli nodes config`** - Get node configuration options.

Required privilege: Sys.Audit on /
- **`proxmox-cli nodes dns`** - Read DNS settings.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes hardware`** - Index of hardware types.

Required privilege: any authenticated user
- **`proxmox-cli nodes hardware-pci`** - List local PCI devices.

Required privilege: Sys.Audit, Sys.Modify (any) on /
- **`proxmox-cli nodes hardware-pci-get`** - Index of available pci methods.

Required privilege: any authenticated user
- **`proxmox-cli nodes hardware-pci-mdev`** - List mediated device types for given PCI device.

Required privilege: Sys.Audit, Sys.Modify (any) on /
- **`proxmox-cli nodes hardware-usb`** - List local USB devices.

Required privilege: Sys.Modify on /
- **`proxmox-cli nodes hosts`** - Get the content of /etc/hosts.

Required privilege: Sys.Audit on /
- **`proxmox-cli nodes journal`** - Read Journal.

Required privilege: Sys.Syslog on /nodes/{node}
- **`proxmox-cli nodes list`** - Cluster node index.

Required privilege: any authenticated user
- **`proxmox-cli nodes netstat`** - Read tap/vm network device interface counters.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes network`** - List available networks.

Required privilege: any authenticated user
- **`proxmox-cli nodes network-get`** - Read network device configuration.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes rrd`** - Read node RRD statistics (returns PNG)

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes rrddata`** - Read node RRD statistics.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes services`** - Service list.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes services-state`** - Read service properties.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes status`** - Read node status.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes syslog`** - Read system log.

Required privilege: Sys.Syslog on /nodes/{node}
- **`proxmox-cli nodes time`** - Read server time and time zone settings.

Required privilege: Sys.Audit on /nodes/{node}
- **`proxmox-cli nodes version`** - API version details.

Required privilege: any authenticated user

### notifications

Manage notifications

- **`proxmox-cli notifications endpoints-gotify`** - Returns a list of all gotify endpoints.

Required privilege: Mapping.Audit on /mapping/notifications
- **`proxmox-cli notifications endpoints-gotify-get`** - Return a specific gotify endpoint.
- **`proxmox-cli notifications endpoints-sendmail`** - Returns a list of all sendmail endpoints.
- **`proxmox-cli notifications endpoints-sendmail-get`** - Return a specific sendmail endpoint.
- **`proxmox-cli notifications endpoints-smtp`** - Returns a list of all smtp endpoints.
- **`proxmox-cli notifications endpoints-smtp-get`** - Return a specific smtp endpoint.
- **`proxmox-cli notifications matcher-field-values`** - Returns known notification metadata fields and their known values.
- **`proxmox-cli notifications matcher-fields`** - Returns known notification metadata fields.
- **`proxmox-cli notifications matchers`** - Returns a list of all matchers.
- **`proxmox-cli notifications matchers-get`** - Return a specific matcher.
- **`proxmox-cli notifications targets`** - Returns a list of all entities that can be used as notification targets.

### pbs_access

Manage pbs access

- **`proxmox-cli pbs-access acl`** - Read Access Control List (ACLs).
- **`proxmox-cli pbs-access config-ad`** - List configured AD realms.

Required privilege: Realm.Allocate on /access/domains
- **`proxmox-cli pbs-access config-ad-get`** - Read the AD realm configuration.

Required privilege: Sys.Audit on /access/domains
- **`proxmox-cli pbs-access config-ldap`** - List configured LDAP realms.

Required privilege: Realm.Allocate on /access/domains
- **`proxmox-cli pbs-access config-ldap-get`** - Read the LDAP realm configuration.

Required privilege: Sys.Audit on /access/domains
- **`proxmox-cli pbs-access config-pam`** - Read the PAM realm configuration.

Required privilege: Sys.Audit on /access/domains
- **`proxmox-cli pbs-access config-pbs`** - Read the Proxmox Backup authentication server realm configuration.

Required privilege: Sys.Audit on /access/domains
- **`proxmox-cli pbs-access config-tfa-webauthn`** - Get the TFA configuration.

Required privilege: any authenticated user
- **`proxmox-cli pbs-access domains`** - Authentication domain/realm index.

Required privilege: Anyone can access this, because we need that list for the login box (before the user is authenticated).
- **`proxmox-cli pbs-access permissions`** - List permissions of given or currently authenticated user / API token.

Optionally limited to specific path.

Required privilege: Requires Sys.Audit on '/access', limited to own privileges otherwise.
- **`proxmox-cli pbs-access roles`** - Role list.

Required privilege: any authenticated user
- **`proxmox-cli pbs-access tfa`** - List user TFA configuration.

Required privilege: Returns all or just the logged-in user, depending on privileges.
- **`proxmox-cli pbs-access tfa-get`** - Add a TOTP secret to the user.

Required privilege: Permissions.Modify on /access/users or the user named by {userid}
- **`proxmox-cli pbs-access tfa-id-get`** - Get a single TFA entry.

Required privilege: Permissions.Modify on /access/users or the user named by {userid}
- **`proxmox-cli pbs-access users`** - List users.

Required privilege: Returns all or just the logged-in user (/API token owner), depending on privileges.
- **`proxmox-cli pbs-access users-get`** - Read user configuration data.

Required privilege: Sys.Audit on /access/users or the user named by {userid}
- **`proxmox-cli pbs-access users-token`** - List user's API tokens.

Required privilege: Sys.Audit on /access/users or the user named by {userid}
- **`proxmox-cli pbs-access users-token-get`** - Read user's API token metadata.

Required privilege: Sys.Audit on /access/users or the user named by {userid}

### pbs_config

Manage pbs config

- **`proxmox-cli pbs-config acme-account`** - List ACME accounts.

Required privilege: Sys.Modify on /system/certificates
- **`proxmox-cli pbs-config acme-account-get`** - Return existing ACME account information.

Required privilege: Sys.Modify on /system/certificates
- **`proxmox-cli pbs-config acme-challenge-schema`** - Get named known ACME directory endpoints.

Required privilege: any authenticated user
- **`proxmox-cli pbs-config acme-directories`** - Get named known ACME directory endpoints.

Required privilege: any authenticated user
- **`proxmox-cli pbs-config admin-metrics`** - List configured metric servers.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-config admin-traffic-control`** - Show current traffic for all traffic control rules.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-config encryption-keys`** - List configured encryption keys.

Required privilege: List configured encryption keys filtered by Sys.Audit privileges
- **`proxmox-cli pbs-config metrics-influxdb-udp`** - List configured InfluxDB udp metric servers.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-config metrics-influxdb-udp-get`** - Read the InfluxDB udp server configuration.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-config remote`** - List all remotes.

Required privilege: List configured remotes filtered by Remote.Audit privileges
- **`proxmox-cli pbs-config remote-get`** - Read remote configuration data.

Required privilege: Remote.Audit on /remote/{name}
- **`proxmox-cli pbs-config remote-scan`** - List datastores of a remote.cfg entry.

Required privilege: Remote.Audit on /remote/{name}
- **`proxmox-cli pbs-config remote-scan-groups`** - List groups of a remote.cfg entry's datastore.

Required privilege: Remote.Audit on /remote/{name}
- **`proxmox-cli pbs-config remote-scan-namespaces`** - List namespaces of a datastore of a remote.cfg entry.

Required privilege: Remote.Audit on /remote/{name}
- **`proxmox-cli pbs-config s3`** - List all s3 client configurations.

Required privilege: List configured s3 endpoints filtered by Sys.Audit privileges
- **`proxmox-cli pbs-config s3-get`** - Read an s3 client configuration.

Required privilege: Sys.Audit on /system/s3-endpoint/{id}
- **`proxmox-cli pbs-config s3-list-buckets`** - List buckets accessible by given s3 client configuration.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-config traffic-control`** - List traffic control rules.

Required privilege: Sys.Modify on /
- **`proxmox-cli pbs-config traffic-control-get`** - Read traffic control configuration data.

Required privilege: Sys.Audit on /

### pbs_datastore

Manage pbs datastore

- **`proxmox-cli pbs-datastore active-operations`** - Read datastore stats.

Required privilege: Datastore.Audit on /datastore/{store}
- **`proxmox-cli pbs-datastore catalog`** - Get the entries of the given path of the catalog.

Required privilege: Requires on /datastore/{store}[/{namespace}] either DATASTORE_READ for any or DATASTORE_BACKUP and being the owner of the group
- **`proxmox-cli pbs-datastore config`** - List all datastores.

Required privilege: any authenticated user
- **`proxmox-cli pbs-datastore config-get`** - Read a datastore configuration.

Required privilege: Datastore.Audit on /datastore/{name}
- **`proxmox-cli pbs-datastore files`** - List snapshot files.

Required privilege: Requires on /datastore/{store}[/{namespace}] either DATASTORE_AUDIT or DATASTORE_READ for any or DATASTORE_BACKUP and being the owner of the group
- **`proxmox-cli pbs-datastore gc`** - Garbage collection status.

Required privilege: Datastore.Audit on /datastore/{store}
- **`proxmox-cli pbs-datastore group-notes`** - Get "notes" for a backup group.

Required privilege: Requires on /datastore/{store}[/{namespace}] either DATASTORE_AUDIT for any or DATASTORE_BACKUP and being the owner of the group
- **`proxmox-cli pbs-datastore groups`** - List backup groups.

Required privilege: Requires DATASTORE_AUDIT for all or DATASTORE_BACKUP for owned groups on /datastore/{store}[/{namespace}]
- **`proxmox-cli pbs-datastore list`** - Datastore list.

Required privilege: any authenticated user
- **`proxmox-cli pbs-datastore namespace`** - List the namespaces of a datastore.

Required privilege: Requires DATASTORE_AUDIT, DATASTORE_MODIFY or DATASTORE_BACKUP /datastore/{store}[/{parent}]
- **`proxmox-cli pbs-datastore notes`** - Get "notes" for a specific backup.

Required privilege: Requires on /datastore/{store}[/{namespace}] either DATASTORE_AUDIT for any or DATASTORE_BACKUP and being the owner of the group
- **`proxmox-cli pbs-datastore protected`** - Query protection for a specific backup.

Required privilege: Requires on /datastore/{store}[/{namespace}] either DATASTORE_AUDIT for any or DATASTORE_BACKUP and being the owner of the group
- **`proxmox-cli pbs-datastore rrd`** - Read datastore stats.

Required privilege: Datastore.Audit, Datastore.Backup (any) on /datastore/{store}
- **`proxmox-cli pbs-datastore snapshots`** - List backup snapshots.

Required privilege: Requires on /datastore/{store}[/{namespace}] either DATASTORE_AUDIT for any or DATASTORE_BACKUP and being the owner of the group
- **`proxmox-cli pbs-datastore status`** - Get datastore status.

Required privilege: Requires on /datastore/{store} either DATASTORE_AUDIT or DATASTORE_BACKUP for the full statistics. Counts of accessible groups are always returned, if any
- **`proxmox-cli pbs-datastore usage`** - List Datastore usages and estimates.

Required privilege: any authenticated user

### pbs_jobs

Manage pbs jobs

- **`proxmox-cli pbs-jobs gc`** - List all GC jobs (max one per datastore)

Required privilege: Requires Datastore.Audit or Datastore.Modify on datastore.
- **`proxmox-cli pbs-jobs gc-get`** - List all GC jobs (max one per datastore)

Required privilege: Requires Datastore.Audit or Datastore.Modify on datastore.
- **`proxmox-cli pbs-jobs prune`** - List all prune jobs.

Required privilege: Requires Datastore.Audit or Datastore.Modify on datastore.
- **`proxmox-cli pbs-jobs prune-config`** - List all scheduled prune jobs.

Required privilege: Requires Datastore.Audit.
- **`proxmox-cli pbs-jobs prune-config-get`** - Read a prune job configuration.

Required privilege: Requires Datastore.Audit or Datastore.Verify on job's datastore.
- **`proxmox-cli pbs-jobs sync`** - List all configured sync jobs.
- **`proxmox-cli pbs-jobs sync-config`** - List all sync jobs.
- **`proxmox-cli pbs-jobs sync-config-get`** - Read a sync job configuration.
- **`proxmox-cli pbs-jobs verify`** - List all verification jobs.

Required privilege: Requires Datastore.Audit or Datastore.Verify on datastore.
- **`proxmox-cli pbs-jobs verify-config`** - List all verification jobs.

Required privilege: Requires Datastore.Audit or Datastore.Verify on datastore.
- **`proxmox-cli pbs-jobs verify-config-get`** - Read a verification job configuration.

### pbs_nodes

Manage pbs nodes

- **`proxmox-cli pbs-nodes api-version`** - Proxmox Backup Server API version.

Required privilege: any authenticated user
- **`proxmox-cli pbs-nodes apt-changelog`** - Retrieve the changelog of the specified package.

Required privilege: Sys.Modify on /
- **`proxmox-cli pbs-nodes apt-update`** - List available APT updates.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-nodes apt-versions`** - Get package information for important Proxmox Backup Server packages.

Required privilege: Sys.Audit on /
- **`proxmox-cli pbs-nodes certificates-info`** - Get certificate info.

Required privilege: Sys.Modify on /system/certificates
- **`proxmox-cli pbs-nodes disks-directory`** - List systemd datastore mount units.

Required privilege: Sys.Audit on /system/disks
- **`proxmox-cli pbs-nodes disks-list`** - List local disks.

Required privilege: Sys.Audit on /system/disks
- **`proxmox-cli pbs-nodes disks-smart`** - Get SMART attributes and health of a disk.

Required privilege: Sys.Audit on /system/disks
- **`proxmox-cli pbs-nodes disks-zfs`** - List zfs pools.

Required privilege: Sys.Audit on /system/disks
- **`proxmox-cli pbs-nodes disks-zfs-get`** - Get zpool status details.

Required privilege: Sys.Audit on /system/disks
- **`proxmox-cli pbs-nodes dns`** - Read DNS settings.

Required privilege: Sys.Audit on /system/network/dns
- **`proxmox-cli pbs-nodes identity`** - Returns a unique server identity. The ID is derived from `/etc/machine-id`.

Required privilege: any authenticated user
- **`proxmox-cli pbs-nodes journal`** - Read syslog entries.

Required privilege: Sys.Audit on /system/log
- **`proxmox-cli pbs-nodes list`** - List Nodes (only for compatibility)
- **`proxmox-cli pbs-nodes network`** - List all datastores.

Required privilege: Sys.Audit on /system/network/interfaces
- **`proxmox-cli pbs-nodes network-get`** - Read a network interface configuration.

Required privilege: Sys.Audit on /system/network/interfaces/{iface}
- **`proxmox-cli pbs-nodes ping`** - Dummy method which replies with `{ "pong": True }`.

Required privilege: Anyone can access this, because it's used for a cheap check if the API daemon is online.
- **`proxmox-cli pbs-nodes rrd`** - Read node stats.

Required privilege: Sys.Audit on /system/status
- **`proxmox-cli pbs-nodes services`** - Service list.

Required privilege: Sys.Audit on /system/services
- **`proxmox-cli pbs-nodes services-state`** - Read service properties.

Required privilege: Sys.Audit on /system/services/{service}
- **`proxmox-cli pbs-nodes status`** - Read node memory, CPU and (root) disk usage.

Required privilege: Sys.Audit on /system/status
- **`proxmox-cli pbs-nodes status-metrics`** - Return backup server metrics.
- **`proxmox-cli pbs-nodes syslog`** - Read syslog entries.

Required privilege: Sys.Audit on /system/log
- **`proxmox-cli pbs-nodes tasks`** - List tasks.

Required privilege: Users can only see their own tasks, unless they have Sys.Audit on /system/tasks.
- **`proxmox-cli pbs-nodes tasks-log`** - Read the task log.

Required privilege: Users can access their own tasks, or need Sys.Audit on /system/tasks.
- **`proxmox-cli pbs-nodes tasks-status`** - Get task status.

Required privilege: Users can access their own tasks, or need Sys.Audit on /system/tasks.
- **`proxmox-cli pbs-nodes time`** - Read server time and time zone settings.

Required privilege: any authenticated user

### pbs_notifications

Manage pbs notifications

- **`proxmox-cli pbs-notifications endpoints-gotify`** - List all gotify endpoints.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications endpoints-gotify-get`** - Get a gotify endpoint.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications endpoints-sendmail`** - List all sendmail endpoints.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications endpoints-sendmail-get`** - Get a sendmail endpoint.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications endpoints-smtp`** - List all smtp endpoints.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications endpoints-smtp-get`** - Get a smtp endpoint.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications matcher-field-values`** - List all known, matchable metadata field values.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications matcher-fields`** - Get all known metadata fields.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications matchers`** - List all notification matchers.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications matchers-get`** - Get a notification matcher.

Required privilege: Sys.Audit on /system/notifications
- **`proxmox-cli pbs-notifications targets`** - List all notification targets.

Required privilege: Sys.Audit on /system/notifications

### pbs_tape

Manage pbs tape

- **`proxmox-cli pbs-tape backup`** - List all tape backup jobs.

Required privilege: List configured tape jobs filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape backup-job-config`** - List all tape backup jobs.

Required privilege: List configured tape jobs filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape backup-job-config-get`** - Read a tape backup job configuration.

Required privilege: Tape.Audit on /tape/job/{id}
- **`proxmox-cli pbs-tape changer`** - List changers.

Required privilege: List configured tape changer filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape changer-config`** - List changers.

Required privilege: List configured tape changer filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape changer-config-get`** - Get tape changer configuration.

Required privilege: Tape.Audit on /tape/device/{name}
- **`proxmox-cli pbs-tape drive`** - List drives.

Required privilege: List configured tape drives filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape drive-cartridge-memory`** - Read Cartridge Memory (Medium auxiliary memory attributes)

Required privilege: Tape.Audit on /tape/device/{drive}
- **`proxmox-cli pbs-tape drive-config`** - List drives.

Required privilege: List configured tape drives filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape drive-config-get`** - Get drive configuration.

Required privilege: Tape.Audit on /tape/device/{name}
- **`proxmox-cli pbs-tape drive-status`** - Get drive/media status.

Required privilege: Tape.Audit on /tape/device/{drive}
- **`proxmox-cli pbs-tape drive-volume-statistics`** - Read Volume Statistics (SCSI log page 17h)

Required privilege: Tape.Audit on /tape/device/{drive}
- **`proxmox-cli pbs-tape encryption-keys`** - List existing keys.

Required privilege: Tape.Audit on /tape/pool
- **`proxmox-cli pbs-tape encryption-keys-get`** - Get key config (public key part)

Required privilege: Tape.Audit on /tape/pool
- **`proxmox-cli pbs-tape media-content`** - List media content.

Required privilege: List content filtered by Tape.Audit privilege on pool
- **`proxmox-cli pbs-tape media-list-status`** - Get current media status.
- **`proxmox-cli pbs-tape media-media-sets`** - List Media sets.

Required privilege: List of media sets filtered by Tape.Audit privileges on pool
- **`proxmox-cli pbs-tape media-pool-config`** - List media pools.

Required privilege: List configured media pools filtered by Tape.Audit privileges
- **`proxmox-cli pbs-tape media-pool-config-get`** - Get media pool configuration.

Required privilege: Tape.Audit on /tape/pool/{name}
- **`proxmox-cli pbs-tape scan-changers`** - Scan for SCSI tape changers.
- **`proxmox-cli pbs-tape scan-drives`** - Scan tape drives.

### pools

Manage pools

- **`proxmox-cli pools get`** - Get pool configuration (deprecated, no support for nested pools, use 'GET /pools/?poolid={poolid}').

Required privilege: Pool.Audit on /pool/{poolid}
- **`proxmox-cli pools list`** - List pools or get pool configuration.

### replication

Manage replication

- **`proxmox-cli replication get`** - Read replication job configuration.

Required privilege: Requires the VM.Audit permission on /vms/<vmid>.
- **`proxmox-cli replication list`** - List replication jobs.
- **`proxmox-cli replication node`** - List status of all replication jobs on this node.

Required privilege: Requires the VM.Audit permission on /vms/<vmid>.
- **`proxmox-cli replication node-log`** - Read replication job log.
- **`proxmox-cli replication node-status`** - Get replication job status.

Required privilege: Requires the VM.Audit permission on /vms/<vmid>.

### sdn

Manage sdn

- **`proxmox-cli sdn controllers`** - SDN controllers index.
- **`proxmox-cli sdn controllers-get`** - Read sdn controller configuration.

Required privilege: SDN.Allocate on /sdn/controllers/{controller}
- **`proxmox-cli sdn fabrics-all`** - SDN Fabrics Index.
- **`proxmox-cli sdn fabrics-fabric`** - SDN Fabrics Index.
- **`proxmox-cli sdn fabrics-fabric-get`** - Read a fabric's configuration. (The upstream schema describes this GET as 'Update a fabric.'; it is the read.)
- **`proxmox-cli sdn fabrics-node`** - SDN Fabrics Index.
- **`proxmox-cli sdn fabrics-node-get`** - SDN Fabrics Index.

Required privilege: Only returns nodes where you have 'Sys.Audit' or 'Sys.Modify' permissions.
- **`proxmox-cli sdn fabrics-node-node-id-get`** - Get a node.
- **`proxmox-cli sdn ipams-status`** - List PVE IPAM Entries.
- **`proxmox-cli sdn list`** - Directory index.

Required privilege: SDN.Audit on /sdn
- **`proxmox-cli sdn node-fabrics-interfaces`** - Get all interfaces for a fabric.

Required privilege: SDN.Audit on /sdn/fabrics/{fabric}
- **`proxmox-cli sdn node-fabrics-neighbors`** - Get all neighbors for a fabric.

Required privilege: SDN.Audit on /sdn/fabrics/{fabric}
- **`proxmox-cli sdn node-fabrics-routes`** - Get all routes for a fabric.

Required privilege: SDN.Audit on /sdn/fabrics/{fabric}
- **`proxmox-cli sdn node-vnets-mac-vrf`** - Get the MAC VRF for a VNet in an EVPN zone.
- **`proxmox-cli sdn node-zones`** - Get status for all zones.

Required privilege: Only list entries where you have 'SDN.Audit'
- **`proxmox-cli sdn node-zones-bridges`** - Get a list of all bridges (vnets) that are part of a zone, as well as the ports that are members of that bridge.

Required privilege: SDN.Audit on /sdn/zones/{zone}
- **`proxmox-cli sdn node-zones-content`** - List zone content.

Required privilege: SDN.Audit on /sdn/zones/{zone}
- **`proxmox-cli sdn node-zones-ip-vrf`** - Get the IP VRF of an EVPN zone.

Required privilege: SDN.Audit on /sdn/zones/{zone}
- **`proxmox-cli sdn prefix-lists`** - List Prefix Lists.
- **`proxmox-cli sdn prefix-lists-entries`** - List Prefix List Entries.

Required privilege: SDN.Audit on /sdn/prefix-lists/{id}
- **`proxmox-cli sdn prefix-lists-entries-get`** - Get Prefix List Entry.

Required privilege: SDN.Audit on /sdn/prefix-lists/{id}
- **`proxmox-cli sdn prefix-lists-get`** - Get Prefix List.

Required privilege: SDN.Audit on /sdn/prefix-lists/{id}
- **`proxmox-cli sdn route-maps`** - List Route Maps.

Required privilege: Only returns route maps where you have 'SDN.Audit' or 'SDN.Allocate' permissions.
- **`proxmox-cli sdn route-maps-entries`** - Lists all route map entries.
- **`proxmox-cli sdn route-maps-entries-entry-get`** - Get Route Map Entry.

Required privilege: SDN.Audit, SDN.Allocate (any) on /sdn/route-maps/{route-map-id}
- **`proxmox-cli sdn route-maps-entries-get`** - List all entries for a given Route Map.
- **`proxmox-cli sdn vnets`** - SDN vnets index.
- **`proxmox-cli sdn vnets-firewall-options`** - Get vnet firewall options.

Required privilege: Needs SDN.Audit or SDN.Allocate permissions on '/sdn/zones/<zone>/<vnet>'
- **`proxmox-cli sdn vnets-firewall-rules`** - List rules.

Required privilege: Needs SDN.Audit or SDN.Allocate permissions on '/sdn/zones/<zone>/<vnet>'
- **`proxmox-cli sdn vnets-firewall-rules-get`** - Get single rule data.

Required privilege: Needs SDN.Audit or SDN.Allocate permissions on '/sdn/zones/<zone>/<vnet>'
- **`proxmox-cli sdn vnets-get`** - Read sdn vnet configuration.
- **`proxmox-cli sdn vnets-subnets`** - SDN subnets index.
- **`proxmox-cli sdn vnets-subnets-get`** - Read sdn subnet configuration.
- **`proxmox-cli sdn zones`** - SDN zones index.
- **`proxmox-cli sdn zones-get`** - Read sdn zone configuration.

Required privilege: SDN.Allocate on /sdn/zones/{zone}

### storage

Manage storage

- **`proxmox-cli storage get`** - Read storage configuration.

Required privilege: Datastore.Allocate on /storage/{storage}
- **`proxmox-cli storage list`** - Storage index.
- **`proxmox-cli storage node`** - Get status for all datastores.
- **`proxmox-cli storage node-content`** - List storage content.

Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}
- **`proxmox-cli storage node-content-get`** - Get volume attributes.

Required privilege: You need read access for the volume.
- **`proxmox-cli storage node-identity`** - Return identity information for this storage instance.
- **`proxmox-cli storage node-import-metadata`** - Get the base parameters for creating a guest which imports data from a foreign importable guest, like an ESXi VM.

Required privilege: You need read access for the volume.
- **`proxmox-cli storage node-prunebackups`** - Get prune information for backups. NOTE: this is only a preview and might not be what a subsequent prune call does if backups are removed/added in the meantime.

Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}
- **`proxmox-cli storage node-rrd`** - Read storage RRD statistics (returns PNG).
- **`proxmox-cli storage node-rrddata`** - Read storage RRD statistics.

Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}
- **`proxmox-cli storage node-status`** - Read storage status.

Required privilege: Datastore.Audit, Datastore.AllocateSpace (any) on /storage/{storage}

### tasks

Manage tasks

- **`proxmox-cli tasks cluster`** - List recent tasks (cluster wide).

Required privilege: any authenticated user
- **`proxmox-cli tasks get`** - Read /nodes/{node}/tasks/{upid}.

Required privilege: any authenticated user
- **`proxmox-cli tasks list`** - Read task list for one node (finished tasks).

Required privilege: List task associated with the current user, or all task the user has 'Sys.Audit' permissions on /nodes/<node> (the <node> the task runs on).
- **`proxmox-cli tasks log`** - Read task log.
- **`proxmox-cli tasks status`** - Read task status.

### vm

Manage vm

- **`proxmox-cli vm agent-get-fsinfo`** - Execute get-fsinfo.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-host-name`** - Execute get-host-name.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-memory-block-info`** - Execute get-memory-block-info.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-memory-blocks`** - Execute get-memory-blocks.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-osinfo`** - Execute get-osinfo.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-time`** - Execute get-time.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-timezone`** - Execute get-timezone.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-users`** - Execute get-users.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-get-vcpus`** - Execute get-vcpus.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-info`** - Execute info.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm agent-network-get-interfaces`** - Execute network-get-interfaces.

Required privilege: VM.GuestAgent.Audit, VM.GuestAgent.Unrestricted (any) on /vms/{vmid}
- **`proxmox-cli vm cloudinit`** - Get the cloudinit configuration with both current and pending values.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm config`** - Get the virtual machine configuration with pending configuration changes applied. Set the 'current' parameter to get the current configuration instead.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm feature`** - Check if feature for virtual machine is available.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-aliases`** - List aliases.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-aliases-get`** - Read alias.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-ipset`** - List IPSets.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-ipset-cidr-get`** - Read IP or Network settings from IPSet.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-ipset-get`** - List IPSet content.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-log`** - Read firewall log.

Required privilege: VM.Console on /vms/{vmid}
- **`proxmox-cli vm firewall-options`** - Get VM firewall options.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-refs`** - Lists possible IPSet/Alias reference which are allowed in source/dest properties.
- **`proxmox-cli vm firewall-rules`** - List rules.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm firewall-rules-get`** - Get single rule data.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm list`** - Virtual machine index (per node).

Required privilege: Only list VMs where you have VM.Audit permissions on /vms/<vmid>.
- **`proxmox-cli vm migrate`** - Get preconditions for migration.

Required privilege: VM.Migrate on /vms/{vmid}
- **`proxmox-cli vm pending`** - Get the virtual machine configuration with both current and pending values.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm rrd`** - Read VM RRD statistics (returns PNG)

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm rrddata`** - Read VM RRD statistics.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm snapshot`** - List all snapshots.

Required privilege: VM.Audit on /vms/{vmid}
- **`proxmox-cli vm snapshot-config`** - Get snapshot configuration.

Required privilege: VM.Snapshot, VM.Snapshot.Rollback, VM.Audit (any) on /vms/{vmid}
- **`proxmox-cli vm snapshot-get`** - Read /nodes/{node}/qemu/{vmid}/snapshot/{snapname}.

Required privilege: any authenticated user
- **`proxmox-cli vm status-current`** - Get virtual machine status.

Required privilege: VM.Audit on /vms/{vmid}


### backup


- **`proxmox-cli backup coverage`** - Every guest or disk that no enabled backup job includes, with the reason: no job, disabled job only, excluded, or disk excluded.


### capacity


- **`proxmox-cli capacity`** - Every PBS datastore and PVE backup storage ranked by days until full, with garbage-collection and prune state and a GC-deadlock warning.


### failures


- **`proxmox-cli failures`** - Failed and silently incomplete backup, verify, GC, sync and prune tasks in a time window, one row per failed guest or datastore with the reason line.


### posture


- **`proxmox-cli posture`** - One verdict per guest across every client cluster: covered by an enabled job, age of the newest real restore point, PBS verify state, last backup attempt.


### restore-points


- **`proxmox-cli restore-points`** - Every restore point of one guest across all storages and PBS namespaces, with verify state, protection, encryption and whether the next prune removes it.


### rpo


- **`proxmox-cli rpo`** - Per guest over a past window: largest gap between restore points, scheduled runs with no restore point, and days with a verified-good point, rolled up per client.


### verify-health


- **`proxmox-cli verify-health`** - Per PBS datastore and backup group: failed, never-verified and outdated snapshots, first failure, newest verified-good point, and namespaces no verify job reaches. Needs the PBS API.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`proxmox-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`proxmox-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`proxmox-cli learnings list`** - Inspect taught rows
- **`proxmox-cli learnings forget <query>`** - Undo a teach
- **`proxmox-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`proxmox-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`proxmox-cli teach-pattern`** - Install a query/resource template up front
- **`proxmox-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `PROXMOX_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `proxmox-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
proxmox-cli ct list pve1

# JSON for scripting and agents
proxmox-cli ct list pve1 --json
# Filter to specific fields
proxmox-cli ct list pve1 --json --select cpu,cpus,disk

# Dry run  -  show the request without sending
proxmox-cli ct list pve1 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
proxmox-cli ct list pve1 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` on endpoint commands shows the request without sending; fleet commands (posture, failures, ...) only confirm what would run
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` gate tripped (`posture --fail-on-red`, `doctor --fail-on`), `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `PROXMOX_PBS_HOST` resolves `{pbs_host}`
- `PROXMOX_HOST` resolves `{host}`

Base URL: `https://{host}:8006/api2/json`

## Health Check

```bash
proxmox-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `proxmox-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/proxmox-cli/config.toml`; `--home`, `PROXMOX_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PROXMOX_API_TOKEN` | per_call | Yes | Proxmox VE API token `USER@REALM!TOKENID=SECRET` (sent as `PVEAPIToken=...`). |
| `PROXMOX_HOST` | endpoint | Yes (or `PROXMOX_BASE_URL`) | Proxmox VE host name or IP; port 8006. |
| `PROXMOX_BASE_URL` | endpoint | No | Full PVE API base, e.g. `https://pve1:8006/api2/json` (overrides `PROXMOX_HOST`). |
| `PROXMOX_TLS_FINGERPRINT` | tls | No | SHA-256 fingerprint of the PVE certificate to pin (stock Proxmox is self-signed). |
| `PROXMOX_PBS_HOST` | endpoint | No | Proxmox Backup Server host, `host:port`, or `https://host:port` (default port 8007); needed only for `pbs-*` commands and PBS data. |
| `PROXMOX_PBS_API_TOKEN` | per_call | No | PBS API token `USER@REALM!TOKENID:SECRET` (colon before the secret). |
| `PROXMOX_PBS_TLS_FINGERPRINT` | tls | No | SHA-256 fingerprint of the PBS certificate to pin. |
| `PROXMOX_CLUSTER` | selection | No | Default cluster from clusters.toml (same as `--cluster`). |
| `PROXMOX_CLUSTERS_FILE` | config | No | Path to clusters.toml (default: the config directory). |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `proxmox-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `proxmox-cli doctor` to check credentials
- Verify the token is set without printing it: `test -n "$PROXMOX_API_TOKEN" && echo set`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `proxmox-cli cluster resources` to see guests, nodes, and storage

### API-specific
- **x509: certificate signed by unknown authority**  -  Pin the self-signed certificate: export PROXMOX_TLS_FINGERPRINT=<SHA-256 from Node > System > Certificates> (PBS: PROXMOX_PBS_TLS_FINGERPRINT)
- **Lists come back empty but the token authenticates**  -  Privilege-separated tokens need their own ACL: pveum acl modify / --tokens 'user@pam!cli' --roles PVEAuditor, then check with 'proxmox-cli access permissions'
- **Proxmox Backup Server host not set**  -  export PROXMOX_PBS_HOST=pbs.example.lan (or set pbs_host for the cluster in clusters.toml)
- **401 on PBS commands only**  -  PBS tokens use a colon: PROXMOX_PBS_API_TOKEN=user@pbs!tokenid:SECRET (PVE uses '=')
- **posture shows a guest as uncovered though a job lists it**  -  The job is disabled; 'proxmox-cli backup coverage' prints reason disabled-job-only
- **posture or failures exits 5 with 'No data: N of N cluster(s) unreachable'**  -  No cluster answered: check host, token and fingerprint with 'proxmox-cli doctor' (or --cluster <name> doctor)

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**prometheus-pve-exporter**](https://github.com/prometheus-pve/prometheus-pve-exporter)  -  Python (1468 stars)
- [**ProxmoxMCP-Plus**](https://github.com/RekklesNA/ProxmoxMCP-Plus)  -  Python (585 stars)
- [**proxmox-mcp**](https://github.com/GethosTheWalrus/proxmox-mcp)  -  Python (207 stars)
- [**cv4pve-cli**](https://github.com/Corsinvest/cv4pve-cli)  -  C# (86 stars)
- [**pvecontrol**](https://github.com/enix/pvecontrol)  -  Python (81 stars)
- [**pbs-exporter**](https://github.com/natrontech/pbs-exporter)  -  Go (66 stars)
- [**proximo**](https://github.com/john-broadway/proximo)  -  Python (52 stars)
- [**homelab-pbs-mcp**](https://github.com/ahmetem/homelab-pbs-mcp)  -  Python (1 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
