# windows-server skill - governance and safety model

> Unofficial. Community-built skill for Windows Server. Not affiliated with,
> endorsed by, or sponsored by Microsoft Corporation.
> This page tells an MSP owner exactly what the windows-server skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `windows-server-cli` binary (and `windows-server-mcp`). It signs in
to each server's WinRM HTTPS listener (port 5986 by default) with a local or domain
account in that server's Administrators group, using NTLM (the default) or Basic over
HTTPS. Servers are `[[servers]]` entries in the config file, chosen by name; the host,
port and certificate pin come from that file only, never from an agent or an MCP tool
argument.

Credentials come from two places: a password stored per server with
`windows-server-cli servers login <server>` (read from stdin, kept only in the CLI's
0600 credential store), or one shared account in `WINDOWS_SERVER_USERNAME` and
`WINDOWS_SERVER_PASSWORD`. A password is never written to the config file, the local
store, the cache, the learn journal or any output; output that would contain a held
password in any encoding is withheld.

## Default-safe behavior

- **No arbitrary PowerShell.** Every command runs one fixed script that ships inside the
  binary and is checked by SHA-256 on the server before it runs. Arguments (a service
  name, a log name, a number of hours) are validated against a strict pattern and passed
  as typed data, never pasted into script text.
- **Certificate verification is always on**: a system root, a CA file (`ca_file`), or a
  pinned certificate fingerprint (`cert_sha256`). Plain HTTP (5985) is refused unless the
  operator passes `--allow-http` on the CLI; the MCP server never allows it, and Basic
  auth over HTTP is always refused.
- **Service actions preview first.** `services restart` and `services start` print a
  preview and exit 2 with nothing changed unless the CLI gets `--yes` or the MCP tool gets
  `confirm: true`. `services start` only starts a stopped automatic service.
- **Read commands cannot change a server.** They read state with fixed scripts.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating - the preview-then-approve policy still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **every read, plus local history**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read (server)** | Reads server state over WinRM with fixed scripts. No change. | `health`, `system`, `volumes`, `services list`, `services get`, `services down`, `eventlog`, `eventlog new`, `installed-updates`, `pending-updates`, `features`, `local-admins`, `local-admins changes`, `shares`, `scheduled-tasks`, `certificates`, `defender`, `time-sync`, `hyperv-vms` | Allow |
| **Read (local)** | Reads the local store or config only. | `events list`, `inventory diff`, `servers list`, `search`, `analytics` | Allow |
| **Write (local only)** | Writes this machine's local store; nothing on a server. | `inventory snapshot`, `health` (records history and events), `sync`, `local-data purge` (preview unless `--yes`) | Allow; review `local-data purge` |
| **Credential** | Stores or removes a server password. | `servers login`, `servers logout`, `auth set-credentials`, `auth logout` | Human-in-the-loop only |
| **Server action** | Changes a customer server: restarts or starts a service. | `services restart`, `services start` | Human-in-the-loop only: read the preview, then run with `--yes` (CLI) or `confirm: true` (MCP) |
| **Destructive** | Irreversible data or config loss on a server. | (none: the binary has no such command) | - |

`WINDOWS_SERVER_MCP_READ_ONLY=1` removes `health`, `inventory snapshot`,
`services restart` and `services start` from the MCP server (the first two write the
local store); every read stays available.

## How to lock it down

- **Scope the account.** The checks need an account in the server's Administrators
  group (WinRM and several reads require it). Use a dedicated monitoring account per
  client, not a domain admin, and rotate it like any service account.
- **Keep autonomous agents to reads.** Set `WINDOWS_SERVER_MCP_READ_ONLY=1` for an
  agent that should never restart a service, and have a human approve every
  `services restart` and `services start`.
- **Never let an agent run the Credential tier unattended.**
- **Pin certificates.** For a self-signed WinRM listener, set `cert_sha256` in the
  server entry rather than turning on plain HTTP.
- **Rotate the password if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0), including every PowerShell script it can run
(`cli/internal/winsrv/scripts/`). You supply the account, the binary uses it only
against the servers in your config, and you can read every line of how it does so.
