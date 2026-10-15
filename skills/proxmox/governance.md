# proxmox skill - governance and safety model

> Unofficial. Community-built skill for the Proxmox VE and Proxmox Backup Server
> APIs. Not affiliated with, endorsed by, or sponsored by Proxmox Server
> Solutions GmbH.
> This page tells an MSP owner exactly what the proxmox skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## The short version

Every request this skill sends to Proxmox VE (PVE) or Proxmox Backup Server
(PBS) is a read (HTTP GET). The CLI and MCP server contain no command that
creates, changes, starts, stops, migrates, restores, prunes or deletes anything
in Proxmox. That is a design choice, not a default you can flip: the write
endpoints were never generated into the binary. A handful of read endpoints that
return stored secrets (ACME DNS plugin credentials, SDN IPAM and DNS plugin
tokens, webhook notification targets, the raw Ceph config, cloud-init user data)
or that read a file inside a guest (guest-agent file read) are excluded too.
The other guest-agent reads (`vm agent-get-osinfo`, `agent-get-users`,
`agent-network-get-interfaces` and the like) remain: each runs a read-only
query inside the guest through the QEMU guest agent.

## What it authenticates as

The skill drives the `proxmox-cli` binary (and `proxmox-mcp`):

- **PVE:** `PROXMOX_API_TOKEN` (`USER@REALM!TOKENID=SECRET`, sent as
  `PVEAPIToken=...`) against `PROXMOX_HOST` (port 8006) or `PROXMOX_BASE_URL`.
- **PBS (optional):** `PROXMOX_PBS_API_TOKEN` (`USER@REALM!TOKENID:SECRET`, sent
  as `PBSAPIToken=...`) against `PROXMOX_PBS_HOST` (port 8007). The PVE token is
  never sent to the PBS host, and the PBS token never to the PVE host.
- **Many clients:** `clusters.toml` holds one entry per client cluster with its
  hosts, certificate fingerprints, and the *names* of the environment variables
  that hold its tokens. The tokens themselves are never written to that file.
- **Saved token:** `auth set-token` is the only command that writes a token to
  disk (`credentials.toml`, owner-only permissions).

Certificates are trusted by pinned SHA-256 fingerprint
(`PROXMOX_TLS_FINGERPRINT`, `PROXMOX_PBS_TLS_FINGERPRINT`, or per cluster in
`clusters.toml`), or by normal certificate validation when no pin is set. Each
pin is bound to its own service. There is no option to skip certificate checks.

## Default-safe behavior

- **Read commands cannot change Proxmox.** `posture`, `backup coverage`,
  `failures`, `verify-health`, `capacity`, `restore-points`, `rpo`, and every
  endpoint command only read.
- **One side effect to know about.** Reading a storage's status or content lets
  Proxmox activate (mount) an enabled storage, the same activation its own
  `pvestatd` daemon performs every 10 seconds.
- **Assurance reads are live.** The backup commands above read Proxmox on every
  run and never answer from the response cache, so a second run is never a
  stale copy of the first.
- **Agent mode is explicit.** `--agent` produces JSON for scripting; it does not
  widen anything. See AGENTS.md.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read (Proxmox)** | Reports, rollups, inventory. No change in Proxmox. | `posture`, `backup coverage`, `failures`, `verify-health`, `capacity`, `restore-points`, `rpo`, and every `vm`, `ct`, `nodes`, `storage`, `tasks`, `cluster`, `access` and `pbs-*` command | Allow |
| **Local writes** | Writes files on this machine only. | `sync`, `workflow archive`, `export`, `profile save`/`delete`, `teach`, `feedback` (local file) | Allow |
| **Credential routing (local)** | Decides where a token is sent and which certificate is trusted, or saves a token. | `clusters add` (host, TLS pin and token env var per client), `auth set-token`, `auth logout` | Human runs these. None of them is an MCP tool, so an agent connected over MCP cannot point a token at a host of its choosing |
| **Data egress** | Sends output off this machine to a destination you choose. | `--deliver webhook:<url>` or `--deliver file:<path>` on any command, `feedback --send`, bare `feedback` when `PROXMOX_FEEDBACK_ENDPOINT` is set and `PROXMOX_FEEDBACK_AUTO_SEND=true`, and the MCP server bridged to a public HTTPS endpoint | Human-in-the-loop. Read output is still your clients' data |
| **Proxmox writes** | Start, stop, migrate, restore, prune, delete, reconfigure. | None. The skill has no such command. | Not possible with this skill |

The MCP server refuses `--deliver` from an MCP client, so the webhook path is
reachable only from the CLI. `sync --path-context` cannot change `host` or
`pbs_host` either: tokens, pins and origin checks are bound to the configured
hosts before any request is built.

## How to lock it down

- **Use privilege-separated tokens with the smallest role that works.**
  - **PBS:** the `Audit` role on `/` (read-only). It includes `Sys.Audit` on
    `/system/tasks`, which `failures` needs to see verify, GC, sync and prune
    tasks.
  - **PVE:** `PVEAuditor` on `/`. Proxmox VE lists a backup file only to a
    token that also holds `Datastore.AllocateSpace` (on the storage) and
    `VM.Backup` (on the guest); an audit-only token gets an empty list, which
    this skill reports as "storage not read" and the guest as `unproven`, never
    as "no backups". If your backups live on Proxmox Backup Server and
    `PROXMOX_PBS_HOST` is set (spelled as in the PVE storage's server field), the
    skill reads them from the PBS API and the PVE token can stay audit-only.
    If they live on dir, NFS or CIFS storage, grant those two privileges as
    well. They would let the token create or restore a backup through the
    API; this skill never sends such a request, and the trade-off is yours.
    On a stock install the `local` storage also accepts backups; with an
    audit-only token it stays a fetch failure (so `posture --fail-on-red`
    exits 1, and a guest with no readable or only an old restore point is
    unproven rather than never or stale) until you remove "VZDump backup
    file" from its content types.

  ```bash
  pveum user add monitor@pve
  pveum role add BackupReader --privs "Datastore.AllocateSpace VM.Backup"   # only for backups on PVE storage
  pveum acl modify / --users monitor@pve --roles PVEAuditor,BackupReader
  pveum user token add monitor@pve agent --privsep 1
  pveum acl modify / --tokens 'monitor@pve!agent' --roles PVEAuditor,BackupReader
  ```

  A privilege-separated token never has more than its user, so the user needs
  the same roles. Keep the grant on `/` with propagation and add no NoAccess
  or narrower ACL below it for this token: Proxmox VE silently leaves out of
  every list the guests and storages a token cannot see. The skill reports a
  missing, non-propagating or narrowed grant as a fetch failure, but an
  explicit NoAccess on a single guest leaves no trace in any read. With an
  audit-only role on both products, Proxmox itself would refuse a write even
  if one were ever sent.
- **One PBS namespace per client cluster** on a shared Backup Server,
  declared as that entry's `pbs_namespace` (a datastore per client alone does
  not scope a cluster that cannot be read). VMIDs repeat across
  clusters; where two configured clusters write the same namespace (or one
  writes inside another's declared namespace), the skill reports it and
  counts none of those restore points for either. The check covers every
  configured cluster, even when `--cluster` or `--client` narrows the report.
  A cluster that cannot be read is assumed to write anywhere in its declared
  `pbs_namespace` on the PBS host (or certificate) its entry names (on any
  PBS, if it names none), or anywhere on that PBS when it declares none.
  `verify-health` keeps such a namespace's groups but marks them:
  `attribution` is `shared` on the store row and `shared: <clusters>` on each
  group row, whose tenant is left empty. `failures` marks a PBS task the same
  way.
- **One token per client cluster**, referenced by env var name in
  `clusters.toml`, so revoking one client never touches another.
- **Pin every certificate** with its SHA-256 fingerprint.
- **Never let an agent run the Data egress tier unattended.**
- **Rotate a token if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the tokens, the binary uses them only to
read the Proxmox APIs, and you can read every line of how it does so.
