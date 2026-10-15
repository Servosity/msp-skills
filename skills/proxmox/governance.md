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
or that act inside a guest (guest-agent file read) are excluded too.

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
| **Local writes** | Writes files on this machine only. | `clusters add`, `sync`, `workflow archive`, `profile save`/`delete`, `teach`, `feedback` (local file) | Allow |
| **Credential (local)** | Saves or removes a stored token. | `auth set-token`, `auth logout` | Human runs these |
| **Data egress** | Sends output off this machine to a destination you choose. | `--deliver webhook:<url>` or `--deliver file:<path>` on any command, `feedback --send`, bare `feedback` when `PROXMOX_FEEDBACK_ENDPOINT` is set and `PROXMOX_FEEDBACK_AUTO_SEND=true`, and the MCP server bridged to a public HTTPS endpoint | Human-in-the-loop. Read output is still your clients' data |
| **Proxmox writes** | Start, stop, migrate, restore, prune, delete, reconfigure. | None. The skill has no such command. | Not possible with this skill |

The MCP server refuses `--deliver` from an MCP client, so the webhook path is
reachable only from the CLI.

## How to lock it down

- **Use an audit-only, privilege-separated token.** PVE: a token with the
  `PVEAuditor` role on `/`. PBS: a token with the `Audit` role (or
  `DatastoreAudit` on `/datastore`). With those roles Proxmox itself would
  refuse a write even if one were sent.
- **One token per client cluster**, referenced by env var name in
  `clusters.toml`, so revoking one client never touches another.
- **Pin every certificate** with its SHA-256 fingerprint.
- **Never let an agent run the Data egress tier unattended.**
- **Rotate a token if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply an auditor token, the binary uses it only
to read the Proxmox APIs, and you can read every line of how it does so.
