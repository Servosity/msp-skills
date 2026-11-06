# VMware vSphere CLI

**Read-only vCenter inventory for migration planning: blockers, snapshots, per-core licence exposure and Proxmox mapping, as JSON an agent can use.**

Reads a vCenter Server through both vendor JSON APIs and answers the questions an MSP asks before leaving VMware. The `migration blockers`, `snapshots audit` and `licence exposure` commands replace an afternoon of RVTools tabs and spreadsheet math, and `rvtools export` writes the CSV layout assessment tools expect, all from macOS or Linux. Every command is read-only.

Learn more at [VMware vSphere](https://developer.broadcom.com/xapis/vsphere-automation-api/latest/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `vsphere-cli` and `vsphere-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1 | iex
   ```
3. Verify: `vsphere-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=vsphere). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine vsphere-cli`. On Unix, mark it executable: `chmod +x vsphere-cli`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/vsphere --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/vsphere --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `vsphere-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the vsphere skill from https://github.com/Servosity/msp-skills/tree/main/skills/vsphere. The skill defines how its required CLI (`vsphere-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=vsphere).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in your vCenter URL (`https://<vcenter>`), user name, password and, for a self-signed certificate, its SHA-256 fingerprint when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "vsphere": {
      "command": "vsphere-mcp",
      "env": {
        "VSPHERE_BASE_URL": "https://vcenter.example.com",
        "VSPHERE_USERNAME": "readonly@vsphere.local",
        "VSPHERE_PASSWORD": "<password>",
        "VSPHERE_TLS_FINGERPRINT": "<sha-256 fingerprint, only for a self-signed certificate>"
      }
    }
  }
}
```

</details>

## Authentication

Set VSPHERE_BASE_URL to your vCenter (https://vcenter.example.com; https only) and VSPHERE_USERNAME / VSPHERE_PASSWORD to an SSO account in user@domain form, such as a read-only user in the vsphere.local domain; a Read-Only role is enough. The CLI never sends the password on data calls: it logs in once per API (POST /api/session for the Automation API, SessionManager Login for the VI/JSON API) and sends the returned session id. Session ids stay in memory for the life of the process and are never written to disk, so each run logs in again; `auth logout` clears stored credentials. A saved password belongs to the vCenter it was saved for: setting VSPHERE_BASE_URL to another vCenter refuses it rather than sending it there, and each vCenter keeps its own local store. vCenter usually serves a self-signed VMCA certificate: pin it with VSPHERE_TLS_FINGERPRINT (SHA-256, comma-separated for several vCenters). Snapshots, host core counts, licences and the consolidation flag need vCenter 8.0 Update 1 or later (VI/JSON API); guest DNS names, IP addresses and filesystems need VMware Tools running in the guest.

## Quick Start

```bash
# Confirm the binary runs (dry run, no network); run doctor without --dry-run for the live report
vsphere-cli doctor --dry-run

# vCenter version and build; answers before login and shows whether the VI/JSON API exists
vsphere-cli vi-service-instance

# Powered-on VMs with vCPU and memory
vsphere-cli vm list --power-states POWERED_ON --agent

# Per-VM go/no-go for the migration
vsphere-cli migration blockers --agent

# Licensable cores under the 16-cores-per-CPU minimum
vsphere-cli licence exposure --agent

```

## Unique Features

### Migration planning
- **`migration blockers`**  -  One verdict per VM (blocked, review, ready, or unknown when a check could not run) with reason codes: snapshots, vCenter's consolidation-needed flag, delta chains without snapshots, vSAN-backed disks, encrypted disks, attached ISOs, VMware Tools missing or stopped, PVSCSI controllers, NVMe disks, EFI firmware and VMXNET NICs.

  _Reach for this before a cutover wave to get every VM's blockers in one call instead of five RVTools tabs._

  ```bash
  vsphere-cli migration blockers --cluster domain-c8 --agent
  ```
- **`snapshots audit`**  -  Every snapshot in the vCenter as a row with age, tree depth and quiesced flag, plus per-VM delta-disk and snapshot-file totals and the VMs vCenter says need consolidation.

  _Use it for the weekly snapshot sweep: the oldest snapshots and the VMs holding the most delta-disk space, in one view. Needs vCenter 8.0 Update 1 or later._

  ```bash
  vsphere-cli snapshots audit --older-than 7d --agent
  ```
- **`migration proxmox-map`**  -  Each VM's sizing, firmware, disks, NICs and guest OS beside the Proxmox VE qm settings they map to (bios, efidisk0, ostype, sockets, cores, memory, scsihw, netN with model and MAC, disk slots, boot order, smbios1), following Proxmox's own VMware importer.

  _Use it to pre-stage target VMs: the output keys are the Proxmox parameter names._

  ```bash
  vsphere-cli migration proxmox-map --vm vm-1021 --agent
  ```
- **`rvtools export`**  -  Writes the RVTools CSV tabs that AWS Transform and other assessment tools read (vInfo, vCPU, vMemory, vDisk, vPartition, vNetwork, vTools, vHost, vSource, plus vSnapshot) from macOS or Linux; without --out it prints them as JSON.

  _Use it when an assessment or migration tool asks for an RVTools export._

  ```bash
  vsphere-cli rvtools export --out ./acme-rvtools
  ```

### Cost and sizing
- **`licence exposure`**  -  Licensable cores per host, cluster and vCenter under Broadcom's 16-cores-per-CPU minimum, next to current licence editions and usage with keys masked.

  _Use it for renewal and QBR prep: the per-core number Broadcom bills against, without a spreadsheet. Needs vCenter 8.0 Update 1 or later._

  ```bash
  vsphere-cli licence exposure --price-per-core 350 --agent
  ```
- **`migration capacity`**  -  Per-cluster sizing totals: VMs, vCPU, vRAM, physical cores and RAM with overcommit ratios, and provisioned versus committed versus guest-used storage, each with the count of items it covers.

  _Use it to size the Proxmox target hardware from the numbers that actually have to move._

  ```bash
  vsphere-cli migration capacity --powered-on-only --agent
  ```

## Recipes

### Narrow the VM list for an agent

```bash
vsphere-cli vm list --clusters domain-c8 --agent --select vm,name,power_state,cpu_count,memory_size_MiB
```

Keeps only the sizing columns so a large estate fits in context.

### Snapshots older than a week

```bash
vsphere-cli snapshots audit --older-than 7d --agent
```

Snapshot rows sorted oldest first, plus per-VM delta-disk totals in vms[].

### What blocks this wave

```bash
vsphere-cli migration blockers --cluster domain-c8 --agent
```

Verdict and reason codes per VM; checks that need vCenter 8.0 Update 1 come back as unknown on older vCenters.

### Broadcom renewal number

```bash
vsphere-cli licence exposure --price-per-core 350 --agent
```

Licensable cores with the 16-core-per-CPU floor and an estimate from the price you supply.

### RVTools-format export

```bash
vsphere-cli rvtools export --out ./acme-rvtools
```

CSV tabs named like an RVTools export for assessment tools.

## Usage

Run `vsphere-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, the `data-<hash>.db` stores, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `VSPHERE_CONFIG_DIR`, `VSPHERE_DATA_DIR`, `VSPHERE_STATE_DIR`, or `VSPHERE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `VSPHERE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export VSPHERE_HOME=/srv/vsphere
vsphere-cli doctor
```

On Windows (PowerShell):

```powershell
$env:VSPHERE_HOME = "C:\ProgramData\vsphere"
vsphere-cli doctor
```

Under `VSPHERE_HOME=/srv/vsphere`, the four dirs resolve to `/srv/vsphere/config`, `/srv/vsphere/data`, `/srv/vsphere/state`, and `/srv/vsphere/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:VSPHERE_HOME = "C:\srv\vsphere"
vsphere-cli doctor
```

Under `$env:VSPHERE_HOME = "C:\srv\vsphere"`, the four dirs resolve to `C:\srv\vsphere\config`, `C:\srv\vsphere\data`, `C:\srv\vsphere\state`, and `C:\srv\vsphere\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "vsphere": {
      "command": "vsphere-mcp",
      "env": {
        "VSPHERE_HOME": "/srv/vsphere"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `VSPHERE_DATA_DIR` overrides an explicit `--home` for that kind. Use `VSPHERE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `VSPHERE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `vsphere-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store in the data directory: one `data-<hash>.db` per vCenter address and credential, so one vCenter's records are never read as another's. Responses are never cached on disk (the response cache is off on every client). Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `vsphere-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `vsphere-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `VSPHERE_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `VSPHERE_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### appliance

Manage appliance

- **`vsphere-cli appliance`** - Get the version. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### cluster

Manage cluster

- **`vsphere-cli cluster get`** - Retrieves information about the cluster corresponding to cluster. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli cluster list`** - Returns information about at most 1000 visible (subject to permission checks) clusters in vCenter matching the Cluster.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### datacenter

Manage datacenter

- **`vsphere-cli datacenter get`** - Retrieves information about the datacenter corresponding to datacenter. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli datacenter list`** - Returns information about at most 1000 visible (subject to permission checks) datacenters in vCenter matching the Datacenter.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### datastore

Manage datastore

- **`vsphere-cli datastore get`** - Retrieves information about the datastore indicated by datastore. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli datastore list`** - Returns information about at most 2500 visible (subject to permission checks) datastores in vCenter matching the Datastore.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### folder

Manage folder

- **`vsphere-cli folder`** - Returns information about at most 1000 visible (subject to permission checks) folders in vCenter matching the Folder.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### host

Manage host

- **`vsphere-cli host`** - Returns information about at most 2500 visible (subject to permission checks) hosts in vCenter matching the Host.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### network

Manage network

- **`vsphere-cli network`** - Returns information about at most 1000 visible (subject to permission checks) networks in vCenter matching the Network.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### resource_pool

Manage resource pool

- **`vsphere-cli resource-pool get`** - Retrieves information about the resource pool indicated by resourcePool. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli resource-pool list`** - Returns information about at most 1000 visible (subject to permission checks) resource pools in vCenter matching the ResourcePool.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### session

Manage session

- **`vsphere-cli session`** - Show the current vCenter session (user, created and last-accessed time); the CLI logs in for you. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### tagging_association

Manage tagging association

- **`vsphere-cli tagging-association`** - Returns tag associations that match the specified iteration spec. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### tagging_category

Manage tagging category

- **`vsphere-cli tagging-category get`** - Fetches the category information for the given category identifier. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli tagging-category list`** - Enumerates the categories in the system. Needs the vSphere Automation API (vCenter 7 Update 2 or later). Returns identifiers only; read each one with the matching get command.

### tagging_tag

Manage tagging tag

- **`vsphere-cli tagging-tag get`** - Fetches the tag information for the given tag identifier. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli tagging-tag list`** - Enumerates the tags in the system. Needs the vSphere Automation API (vCenter 7 Update 2 or later). Returns identifiers only; read each one with the matching get command.

### vi_datastore

Manage vi datastore

- **`vsphere-cli vi-datastore <moId>`** - Global properties of the datastore. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session.

### vi_host

Manage vi host

- **`vsphere-cli vi-host <moId>`** - Basic information about the host, including connection state. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session.

### vi_service_instance

Manage vi service instance

- **`vsphere-cli vi-service-instance`** - The properties of the ServiceInstance managed object. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session. Unauthenticated: answers before login.

### vi_vm

Manage vi vm

- **`vsphere-cli vi-vm layout-ex`** - Detailed information about the files that comprise this virtual machine. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session.
- **`vsphere-cli vi-vm snapshot`** - Current snapshot and tree. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session.
- **`vsphere-cli vi-vm storage`** - Storage space used by the virtual machine, split by datastore. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session.
- **`vsphere-cli vi-vm summary`** - Basic information about this virtual machine. Virtual Infrastructure JSON API: needs vCenter 8 Update 1 or later; the CLI logs in with its own VI/JSON session.

### vm

Manage vm

- **`vsphere-cli vm get`** - Returns information about a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm list`** - Returns information about at most 4000 visible (subject to permission checks) virtual machines in vCenter matching the VM.FilterSpec. Needs the vSphere Automation API (vCenter 7 Update 2 or later). vCenter refuses (unable_to_allocate_resource) when more than 4000 VMs match; filter by cluster or host on large estates.

### vm_guest

Manage vm guest

- **`vsphere-cli vm-guest identity`** - Return information about the guest. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-guest local-filesystem`** - Returns details of the local file systems in the guest operating system. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-guest network-interfaces`** - Returns information about the networking interfaces in the guest operating system. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### vm_hardware

Manage vm hardware

- **`vsphere-cli vm-hardware boot`** - Returns the boot-related settings of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware cdroms`** - Returns commonly used information about the virtual CD-ROM devices belonging to the virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware cpu`** - Returns the CPU-related settings of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware disk`** - Returns information about a virtual disk. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware disks`** - Returns commonly used information about the virtual disks belonging to the virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware get`** - Returns the virtual hardware settings of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware memory`** - Returns the memory-related settings of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware nic`** - Returns information about a virtual Ethernet adapter. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware nics`** - Returns commonly used information about the virtual Ethernet adapters belonging to the virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware nvme-adapters`** - Returns commonly used information about the virtual NVMe adapters belonging to the virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware sata-adapters`** - Returns commonly used information about the virtual SATA adapters belonging to the virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- **`vsphere-cli vm-hardware scsi-adapters`** - Returns commonly used information about the virtual SCSI adapters belonging to the virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### vm_power

Manage vm power

- **`vsphere-cli vm-power <vm>`** - Returns the power state information of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

### vm_tools

Manage vm tools

- **`vsphere-cli vm-tools <vm>`** - Get the properties of VMware Tools. Needs the vSphere Automation API (vCenter 7 Update 2 or later).


### licence


- **`vsphere-cli licence exposure`** - Licensable cores per host, cluster and vCenter under Broadcom's 16-cores-per-CPU minimum, next to current licence editions and usage with keys masked.


### migration


- **`vsphere-cli migration blockers`** - One verdict per VM (blocked, review, ready, or unknown when a check could not run) with reason codes: snapshots, vCenter's consolidation-needed flag, delta chains without snapshots, vSAN-backed disks, encrypted disks, attached ISOs, VMware Tools missing or stopped, PVSCSI controllers, NVMe disks, EFI firmware and VMXNET NICs.

- **`vsphere-cli migration capacity`** - Per-cluster sizing totals: VMs, vCPU, vRAM, physical cores and RAM with overcommit ratios, and provisioned versus committed versus guest-used storage, each with the count of items it covers.

- **`vsphere-cli migration proxmox-map`** - Each VM's sizing, firmware, disks, NICs and guest OS beside the Proxmox VE qm settings they map to (bios, efidisk0, ostype, sockets, cores, memory, scsihw, netN with model and MAC, disk slots, boot order, smbios1), following Proxmox's own VMware importer.


### rvtools


- **`vsphere-cli rvtools export`** - Writes the RVTools CSV tabs that AWS Transform and other assessment tools read (vInfo, vCPU, vMemory, vDisk, vPartition, vNetwork, vTools, vHost, vSource, plus vSnapshot) from macOS or Linux; without --out it prints them as JSON.


### snapshots


- **`vsphere-cli snapshots audit`** - Every snapshot in the vCenter as a row with age, tree depth and quiesced flag, plus per-VM delta-disk and snapshot-file totals and the VMs vCenter says need consolidation.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`vsphere-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`vsphere-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`vsphere-cli learnings list`** - Inspect taught rows
- **`vsphere-cli learnings forget <query>`** - Undo a teach
- **`vsphere-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`vsphere-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`vsphere-cli teach-pattern`** - Install a query/resource template up front
- **`vsphere-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `VSPHERE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `vsphere-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
vsphere-cli cluster list

# JSON for scripting and agents
vsphere-cli cluster list --json
# Filter to specific fields
vsphere-cli cluster list --json --select cluster,drs_enabled,ha_enabled

# Dry run  -  show the request without sending
vsphere-cli cluster list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
vsphere-cli cluster list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
vsphere-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `vsphere-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/vsphere-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\vsphere-cli\config.toml` on Windows; `--home`, `VSPHERE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `VSPHERE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `VSPHERE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `VSPHERE_USERNAME` | per_call | Yes | vCenter SSO user, e.g. readonly@vsphere.local. A Read-Only role is enough. |
| `VSPHERE_PASSWORD` | per_call | Yes | Password for VSPHERE_USERNAME. |
| `VSPHERE_BASE_URL` | config | Yes | Your vCenter, `https://<vcenter>` (https only). |
| `VSPHERE_TLS_FINGERPRINT` | config | No | SHA-256 certificate pins (comma-separated) for self-signed vCenter certificates. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `vsphere-cli doctor` reports `agentcookie: detected` and `vsphere-cli auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- Your password goes only to vCenter's two login endpoints (`POST /api/session`, and the VI/JSON `SessionManager` Login on vCenter 8.0 Update 1 or later); every data call carries the session id vCenter returns. Session ids are kept in memory for the run and never written to disk, so each CLI invocation logs in once; for bulk work use the MCP server or one long-running process.
- Requests go only to the scheme, host and port of `VSPHERE_BASE_URL`: a redirect to anywhere else is refused before it is followed. Credentials are refused over plain `http://`, a loopback address included; for a local test server use https with `VSPHERE_CA_CERT`.
- Any output, file, request or MCP result that would repeat the password, a session id or a credential-named value from a response is withheld whole and replaced by a notice such as "[credkit: withheld from stdout: it carried a credential this program holds]". The same goes for 8 or more characters in a row of any of them, in the encodings the check knows. Because a login holds user:password as one value, this includes 8 or more characters of your user name in output after a login: if a command you expect to work prints that notice, a read-only account with a name that does not appear in your inventory avoids it. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- `credentials.toml` (mode 0600) holds the credential `auth set-credentials` writes. A hand-written `config.toml` (0600) may hold one too, and with an `.agentcookie-managed` marker `auth set-credentials` keeps it in `config.toml`.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `vsphere-cli doctor` to check credentials
- Verify the environment variable is set: `echo $VSPHERE_USERNAME`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **x509: certificate signed by unknown authority**  -  Pin the vCenter certificate: export VSPHERE_TLS_FINGERPRINT=$(openssl s_client -connect vcenter.example.com:443 </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256 | cut -d= -f2)
- **TLS_PIN_MISMATCH error**  -  The vCenter certificate changed or the pin is for another vCenter; re-read the fingerprint with the openssl command above and update VSPHERE_TLS_FINGERPRINT
- **HTTP 401 from vCenter login**  -  Check VSPHERE_USERNAME includes the SSO domain (user@domain, for example a user in vsphere.local, not the bare user name) and VSPHERE_PASSWORD is current; run vsphere-cli auth status
- **no vCenter configured**  -  export VSPHERE_BASE_URL=https://<your-vcenter> (https only)
- **snapshots audit or licence exposure fails with "needs the VI/JSON API"**  -  The VI/JSON API needs vCenter 8.0 Update 1 or later; on 7.0 use migration blockers (reports those checks as unknown) and the /api commands
- **vm list fails with unable_to_allocate_resource**  -  More than 4000 VMs matched; filter with --clusters or --hosts (the novel commands already list per host)

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**rcourtman/Pulse**](https://github.com/rcourtman/Pulse)  -  Go (6838 stars)
- [**zw008/VMware-Monitor**](https://github.com/zw008/VMware-Monitor)  -  Python (15 stars)
- [**awslabs/export-for-vcenter**](https://github.com/awslabs/export-for-vcenter)  -  Python (8 stars)
- [**domotz/custom-driver**](https://github.com/domotz/custom-driver)  -  JavaScript
- [**vmware/govmomi**](https://github.com/vmware/govmomi)  -  Go
- [**michaelrice/vcenter-mcp**](https://github.com/michaelrice/vcenter-mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
