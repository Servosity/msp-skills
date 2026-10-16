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
- **Backup answers are live.** The CLI keeps no response cache, and the backup
  commands above read Proxmox on every run, so a second run is never a stale
  copy of the first. The per-endpoint read commands default to `--data-source
  auto`: when Proxmox cannot be reached they answer from the local mirror
  (which `sync` and their own earlier reads fill, comments included) and say
  so: a warning on stderr in every output form, and `meta.source: local` in
  JSON. Pass `--data-source live` to refuse a mirror answer.
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
  explicit NoAccess on a single guest leaves no trace in any read. The same
  holds for a backup group whose owner file the Backup Server cannot read (a
  damaged datastore): it drops the group from every list and only logs it on
  the server. With an audit-only role on both products, Proxmox itself would
  refuse a write even if one were ever sent.
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
  An entry with a `pbs_host` and no Proxmox VE host belongs to its own
  client (the same `client`, or the same entry name when `client` is unset;
  compared without case). In `verify-health` it is credited with each
  namespace one of that client's clusters writes alone; for a whole
  datastore (`capacity`) and for Backup Server tasks (`failures`), only when
  a single cluster of that client writes there. Anything else is shared.
  Such an entry is not itself counted as a writer: give another client's
  PBS-only entry (host backups through proxmox-backup-client, say) its own
  datastore, or that datastore's capacity, verify and garbage-collection
  results may be credited to a client whose cluster writes there too.
  `verify-health` keeps such a namespace's groups but marks them:
  `attribution` is `shared` on the store row and `shared: <clusters>` on each
  group row, whose tenant is left empty. `failures` marks a PBS task the same
  way. Two references to a Backup Server are matched when they name the same
  host and port, or carry the same certificate fingerprint. A pin on one side
  does not reach a second name left unpinned: when one server is reached under
  two names (a CA-signed certificate), every storage and clusters.toml entry
  that names it must carry its fingerprint, or it is not matched and
  colliding VMIDs are credited (Proxmox VE already requires the fingerprint
  for the Backup Server's default self-signed certificate). The reverse also
  holds: two clients' separate servers that their clusters reach at the same
  private address and port count as one server, which a per-client
  `pbs_namespace` keeps apart.
- **One backup export per client cluster** for NFS, SMB/CIFS, CephFS or
  GlusterFS backup storage. A vzdump archive carries only a VMID, so where two
  configured clusters mount the same export or share (or a configured cluster
  could not be read), the skill counts none of its archives for either and
  says so. It matches a share by server, export, share and subdir as the
  storage config spells them; the same server reached by name on one cluster
  and by address on another, a `dir` storage over a mounted share, and a
  hyperconverged CephFS another cluster mounts through its monitors are not
  matched, because no Proxmox read says what backs them.
- **Do not reuse a VMID while the old guest's backups are retained.** A
  backup carries only the VMID. For a VM the skill reads the creation time
  Proxmox VE records in its config since qemu-server 7.0-18 (Proxmox VE
  7.1): `meta` `ctime`, which a restore keeps. It credits the VM with no
  backup or task from before then and judges no scheduled run from before
  then. A backup dated less than an hour before it (the clock-skew tolerance
  between nodes) may be either guest's: it is neither credited nor taken as
  proof that none exists, and restore-points lists it with
  `may_be_earlier_guest`. If a VM's config cannot be read (an error, or its
  node is offline), none of its backups is credited: they are listed the
  same way, the VM reads unproven, and the failed read is a fetch failure.
  Where the record is missing or does not help, a guest that reuses a
  deleted guest's VMID is credited with that guest's retained backups: a
  container (it has no such record), a VM created before then, a VM whose
  creation time is dated in the future (the creating node's clock ran ahead,
  so it bounds nothing), a VM cloned from a template or another VM, migrated
  in from another cluster, or restored from another guest's backup (each
  keeps its source's older creation time), and a VM created on a node whose
  clock ran behind (an earlier guest's backup taken just before the VMID was
  reused can be dated after the new creation time). The reverse also
  happens: if the creating node's clock ran more than an hour ahead and was
  corrected later, the VM's first backups read as an earlier guest's.
- **One token per client cluster**, referenced by env var name in
  `clusters.toml`, so revoking one client never touches another.
- **Pin every certificate** with its SHA-256 fingerprint.
- **Offsite means another server.** posture's `offsite_last_success_at`
  counts a PBS push sync only when its remote is read and is not this Backup
  Server (the same host:port, a loopback name, or a fingerprint this PBS is
  pinned to). The same server reached under another name is caught only by
  the fingerprint, so pin the PBS fingerprint. A remote is judged by where it
  points now: re-pointing one after its last run (PBS keeps no history of a
  remote's host) is not detected.
- **Never let an agent run the Data egress tier unattended.**
- **Rotate a token if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the tokens, the binary uses them only to
read the Proxmox APIs, and you can read every line of how it does so.
