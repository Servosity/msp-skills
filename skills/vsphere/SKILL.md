---
name: vsphere
description: "Read-only vCenter inventory for migration planning: blockers, snapshots, per-core licence exposure and Proxmox mapping, as JSON an agent can use. Trigger phrases: `inventory our vcenter`, `what blocks the vmware migration`, `how many cores will broadcom charge us`, `find old vmware snapshots`, `export rvtools from vcenter`, `use vsphere`, `run vsphere-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "VMware"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - vsphere-cli
    install:
      - kind: script
        bins: [vsphere-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1
---

# VMware vSphere - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `vsphere-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `vsphere-cli` and `vsphere-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Reads a vCenter Server through both vendor JSON APIs and answers the questions an MSP asks before leaving VMware. The `migration blockers`, `snapshots audit` and `licence exposure` commands replace an afternoon of RVTools tabs and spreadsheet math, and `rvtools export` writes the CSV layout assessment tools expect, all from macOS or Linux. Every command is read-only.

## When to Use This CLI

Use this CLI to inventory a VMware vCenter before a migration: VM sizing and hardware, snapshot hygiene, datastores, host CPU sockets and cores for Broadcom per-core licensing, and Proxmox VE mapping hints. It is strictly read-only, so it is safe to point at a production vCenter with a Read-Only account. Limits: snapshots audit and licence exposure need vCenter 8.0 Update 1 or later and fail on older vCenters; on those, migration blockers reports the snapshot checks as unknown and migration capacity and rvtools export leave the VI/JSON figures empty. Guest data (DNS name, IPs, filesystems, guest-used storage) needs VMware Tools running, and a self-signed vCenter certificate needs VSPHERE_TLS_FINGERPRINT.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to power, create, clone, snapshot, migrate or delete VMs; it has no write commands. Use govc or PowerCLI for changes.
- Do not use it for live performance metrics or alarms; use vCenter, Aria Operations or a monitoring tool.
- Do not use it to configure the Proxmox side; use the Proxmox CLI or API.

## Unique Capabilities

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

## Command Reference

**appliance**  -  Manage appliance

- `vsphere-cli appliance`  -  Get the version. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

**cluster**  -  Manage cluster

- `vsphere-cli cluster get`  -  Retrieves information about the cluster corresponding to cluster.
- `vsphere-cli cluster list`  -  Returns information about at most 1000 visible (subject to permission checks) clusters in vCenter matching the Cluster.

**datacenter**  -  Manage datacenter

- `vsphere-cli datacenter get`  -  Retrieves information about the datacenter corresponding to datacenter.
- `vsphere-cli datacenter list`  -  Returns information about at most 1000 visible (subject to permission checks)

**datastore**  -  Manage datastore

- `vsphere-cli datastore get`  -  Retrieves information about the datastore indicated by datastore.
- `vsphere-cli datastore list`  -  Returns information about at most 2500 visible (subject to permission checks)

**folder**  -  Manage folder

- `vsphere-cli folder`  -  Returns information about at most 1000 visible (subject to permission checks) folders in vCenter matching the Folder.

**host**  -  Manage host

- `vsphere-cli host`  -  Returns information about at most 2500 visible (subject to permission checks) hosts in vCenter matching the Host.

**network**  -  Manage network

- `vsphere-cli network`  -  Returns information about at most 1000 visible (subject to permission checks) networks in vCenter matching the Network.

**resource_pool**  -  Manage resource pool

- `vsphere-cli resource-pool get`  -  Retrieves information about the resource pool indicated by resourcePool.
- `vsphere-cli resource-pool list`  -  Returns information about at most 1000 visible (subject to permission checks)

**session**  -  Manage session

- `vsphere-cli session`  -  Show the current vCenter session (user, created and last-accessed time); the CLI logs in for you.

**tagging_association**  -  Manage tagging association

- `vsphere-cli tagging-association`  -  Returns tag associations that match the specified iteration spec.

**tagging_category**  -  Manage tagging category

- `vsphere-cli tagging-category get`  -  Fetches the category information for the given category identifier.
- `vsphere-cli tagging-category list`  -  Enumerates the categories in the system. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

**tagging_tag**  -  Manage tagging tag

- `vsphere-cli tagging-tag get`  -  Fetches the tag information for the given tag identifier.
- `vsphere-cli tagging-tag list`  -  Enumerates the tags in the system. Needs the vSphere Automation API (vCenter 7 Update 2 or later).

**vi_datastore**  -  Manage vi datastore

- `vsphere-cli vi-datastore <moId>`  -  Global properties of the datastore.

**vi_host**  -  Manage vi host

- `vsphere-cli vi-host <moId>`  -  Basic information about the host, including connection state.

**vi_service_instance**  -  Manage vi service instance

- `vsphere-cli vi-service-instance`  -  The properties of the ServiceInstance managed object.

**vi_vm**  -  Manage vi vm

- `vsphere-cli vi-vm layout-ex`  -  Detailed information about the files that comprise this virtual machine.
- `vsphere-cli vi-vm snapshot`  -  Current snapshot and tree.
- `vsphere-cli vi-vm storage`  -  Storage space used by the virtual machine, split by datastore.
- `vsphere-cli vi-vm summary`  -  Basic information about this virtual machine.

**vm**  -  Manage vm

- `vsphere-cli vm get`  -  Returns information about a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- `vsphere-cli vm list`  -  Returns information about at most 4000 visible (subject to permission checks)

**vm_guest**  -  Manage vm guest

- `vsphere-cli vm-guest identity`  -  Return information about the guest. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- `vsphere-cli vm-guest local-filesystem`  -  Returns details of the local file systems in the guest operating system.
- `vsphere-cli vm-guest network-interfaces`  -  Returns information about the networking interfaces in the guest operating system.

**vm_hardware**  -  Manage vm hardware

- `vsphere-cli vm-hardware boot`  -  Returns the boot-related settings of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- `vsphere-cli vm-hardware cdroms`  -  Returns commonly used information about the virtual CD-ROM devices belonging to the virtual machine.
- `vsphere-cli vm-hardware cpu`  -  Returns the CPU-related settings of a virtual machine. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- `vsphere-cli vm-hardware disk`  -  Returns information about a virtual disk. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- `vsphere-cli vm-hardware disks`  -  Returns commonly used information about the virtual disks belonging to the virtual machine.
- `vsphere-cli vm-hardware get`  -  Returns the virtual hardware settings of a virtual machine.
- `vsphere-cli vm-hardware memory`  -  Returns the memory-related settings of a virtual machine.
- `vsphere-cli vm-hardware nic`  -  Returns information about a virtual Ethernet adapter. Needs the vSphere Automation API (vCenter 7 Update 2 or later).
- `vsphere-cli vm-hardware nics`  -  Returns commonly used information about the virtual Ethernet adapters belonging to the virtual machine.
- `vsphere-cli vm-hardware nvme-adapters`  -  Returns commonly used information about the virtual NVMe adapters belonging to the virtual machine.
- `vsphere-cli vm-hardware sata-adapters`  -  Returns commonly used information about the virtual SATA adapters belonging to the virtual machine.
- `vsphere-cli vm-hardware scsi-adapters`  -  Returns commonly used information about the virtual SCSI adapters belonging to the virtual machine.

**vm_power**  -  Manage vm power

- `vsphere-cli vm-power <vm>`  -  Returns the power state information of a virtual machine.

**vm_tools**  -  Manage vm tools

- `vsphere-cli vm-tools <vm>`  -  Get the properties of VMware Tools. Needs the vSphere Automation API (vCenter 7 Update 2 or later).


**licence**


- `vsphere-cli licence exposure`  -  Licensable cores per host, cluster and vCenter under Broadcom's 16-cores-per-CPU minimum, next to current licence editions and usage with keys masked.


**migration**


- `vsphere-cli migration blockers`  -  One verdict per VM (blocked, review, ready, or unknown when a check could not run) with reason codes: snapshots, vCenter's consolidation-needed flag, delta chains without snapshots, vSAN-backed disks, encrypted disks, attached ISOs, VMware Tools missing or stopped, PVSCSI controllers, NVMe disks, EFI firmware and VMXNET NICs.

- `vsphere-cli migration capacity`  -  Per-cluster sizing totals: VMs, vCPU, vRAM, physical cores and RAM with overcommit ratios, and provisioned versus committed versus guest-used storage, each with the count of items it covers.

- `vsphere-cli migration proxmox-map`  -  Each VM's sizing, firmware, disks, NICs and guest OS beside the Proxmox VE qm settings they map to (bios, efidisk0, ostype, sockets, cores, memory, scsihw, netN with model and MAC, disk slots, boot order, smbios1), following Proxmox's own VMware importer.


**rvtools**


- `vsphere-cli rvtools export`  -  Writes the RVTools CSV tabs that AWS Transform and other assessment tools read (vInfo, vCPU, vMemory, vDisk, vPartition, vNetwork, vTools, vHost, vSource, plus vSnapshot) from macOS or Linux; without --out it prints them as JSON.


**snapshots**


- `vsphere-cli snapshots audit`  -  Every snapshot in the vCenter as a row with age, tree depth and quiesced flag, plus per-VM delta-disk and snapshot-file totals and the VMs vCenter says need consolidation.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
vsphere-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

Set VSPHERE_BASE_URL to your vCenter (https://vcenter.example.com; https only) and VSPHERE_USERNAME / VSPHERE_PASSWORD to an SSO account in user@domain form, such as a read-only user in the vsphere.local domain; a Read-Only role is enough. The CLI never sends the password on data calls: it logs in once per API (POST /api/session for the Automation API, SessionManager Login for the VI/JSON API) and sends the returned session id. Session ids are kept in memory for the life of one process and never written to disk, so each CLI invocation logs in once (bulk work belongs in the MCP server or one process); `auth logout` clears stored credentials. vCenter usually serves a self-signed VMCA certificate: pin it with VSPHERE_TLS_FINGERPRINT (SHA-256, comma-separated for several vCenters). Snapshots, host core counts, licences and the consolidation flag need vCenter 8.0 Update 1 or later (VI/JSON API); guest DNS names, IP addresses and filesystems need VMware Tools running in the guest.

Run `vsphere-cli doctor` to verify setup.

## Credential safety

- Your password goes only to vCenter's two login endpoints (`POST /api/session`, and the VI/JSON `SessionManager` Login on vCenter 8.0 Update 1 or later); every data call carries the session id vCenter returns. Session ids are kept in memory for the run and never written to disk, so each CLI invocation logs in once; for bulk work use the MCP server or one long-running process.
- Requests go only to the scheme, host and port of `VSPHERE_BASE_URL`: a redirect to anywhere else is refused before it is followed. Credentials are refused over plain `http://`, a loopback address included; for a local test server use https with `VSPHERE_CA_CERT`.
- Any output, file, request or MCP result that would repeat the password, a session id or a credential-named value from a response is withheld whole and replaced by a notice such as "[credkit: withheld from stdout: it carried a credential this program holds]". The same goes for 8 or more characters in a row of any of them, in the encodings the check knows. Because a login holds user:password as one value, this includes 8 or more characters of your user name in output after a login: if a command you expect to work prints that notice, a read-only account with a name that does not appear in your inventory avoids it. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- `credentials.toml` (mode 0600) holds the credential `auth set-credentials` writes. A hand-written `config.toml` (0600) may hold one too, and with an `.agentcookie-managed` marker `auth set-credentials` keeps it in `config.toml`.

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
  vsphere-cli cluster list --agent --select cluster,drs_enabled,ha_enabled
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  do not use this CLI for create, update, delete, publish, comment, upvote, invite, order, send, or other mutating requests

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

- Use `--home <dir>` for one invocation, or set `VSPHERE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `VSPHERE_CONFIG_DIR`, `VSPHERE_DATA_DIR`, `VSPHERE_STATE_DIR`, `VSPHERE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `VSPHERE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, the `data-<hash>.db` stores (one per vCenter address and credential), cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `vsphere-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `VSPHERE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `VSPHERE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `VSPHERE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `VSPHERE_DATA_SOURCE` sets the `--data-source` default.

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
vsphere-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "vsphere-cli learnings confirm 12"] }
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
       materially more, record the divergence via `vsphere-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `vsphere-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "web" matching both web01 and web02). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `vsphere-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
vsphere-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "running") satisfies future queries under another alias (e.g., "powered on", "POWERED_ON") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
vsphere-cli teach \
  --query "$QUERY" \
  --resource-type vm \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
vsphere-cli teach-playbook \
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
vsphere-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between vCenter releases, a category label that the API now returns lower-cased).

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

`vsphere-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `VSPHERE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
vsphere-cli feedback "the --since flag is inclusive but docs say exclusive"
vsphere-cli feedback --stdin < notes.txt
vsphere-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `VSPHERE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `VSPHERE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
vsphere-cli profile save briefing --json
vsphere-cli --profile briefing cluster list
vsphere-cli profile list --json
vsphere-cli profile show briefing
vsphere-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `vsphere-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add vsphere-mcp --env "VSPHERE_BASE_URL=https://vcenter.example.com" --env "VSPHERE_USERNAME=readonly@vsphere.local" --env "VSPHERE_PASSWORD=your-password" -- vsphere-mcp
   # self-signed vCenter certificate: also --env "VSPHERE_TLS_FINGERPRINT=AB:CD:...:EF"
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which vsphere-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   vsphere-cli migration blockers --agent
   ```
4. If ambiguous, drill into subcommand help, e.g. `vsphere-cli migration blockers --help`.
