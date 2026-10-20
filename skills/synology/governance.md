# synology skill - governance and safety model

> Unofficial. Community-built skill for the Synology DSM API. Not affiliated with,
> endorsed by, or sponsored by Synology Inc.
> This page tells an MSP owner exactly what the synology skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `synology-cli` binary (and `synology-mcp`). DSM has no API
keys: the CLI logs in to each NAS with a DSM account through `SYNO.API.Auth`,
sending the password as a form POST, and then uses the session id and SynoToken
DSM returns. The account needs to be able to open Active Backup for Business (an
administrator, or an account granted the Active Backup for Business application).
A dedicated account per client NAS keeps the MSP's access separate from the
client's own logins and easy to revoke.

- **One NAS.** `SYNOLOGY_BASE_URL` (for example `https://192.168.1.50:5001`),
  `SYNOLOGY_ACCOUNT` and `SYNOLOGY_PASSWORD`.
- **A fleet.** `synology-cli fleet add <name> --url https://<nas>:5001` registers
  each NAS in the local `fleet.json` registry, which holds its URL, client name,
  certificate setting and the names of its credential env vars, never a password.
  Its login comes from `<prefix>_ACCOUNT` and `<prefix>_PASSWORD`; the prefix
  defaults to `SYNOLOGY_<NAME>` and can be set with `--env-prefix`.
- **Two-factor accounts.** The first login takes a one-time code in
  `SYNOLOGY_OTP_CODE` (or `<prefix>_OTP_CODE`). DSM returns a trusted-device token;
  the CLI keeps it with that NAS's cached session and sends it on later logins.
  `SYNOLOGY_DEVICE_ID` (or `<prefix>_DEVICE_ID`) overrides the kept token.

**What is written to disk.** Passwords and one-time codes are never written. The
CLI does cache, per NAS, the DSM session id, the SynoToken and the two-factor
trusted-device token in `session-<host>.json` in its data directory, created
owner-only (mode 0600) on macOS and Linux; on Windows, protect the data directory
with NTFS permissions. Anyone who can read that file can act as the CLI's DSM
session until it expires, and the trusted-device token spares a future login the
second factor. `synology-cli auth logout` forgets the cached session and
trusted-device token for the `SYNOLOGY_BASE_URL` NAS; `fleet remove` does not
touch a fleet NAS's session file, so delete it from the data directory
(`synology-cli doctor` prints the path) when you retire a NAS.
`synology-cli session logout` invalidates the session id on the NAS itself. To
revoke a trusted-device token for good, remove the device from the account's
trusted devices in DSM.

**Certificates.** Most NAS units serve DSM's self-signed certificate. `fleet add
--pin-cert` (or `--tls-fingerprint <sha256>`) and `SYNOLOGY_TLS_FINGERPRINT` pin
that exact certificate and refuse any other, which is the recommended path.
`--insecure` on one fleet NAS and `SYNOLOGY_SKIP_TLS_VERIFY=1` trust any
certificate and expose the password to anyone on the network path: use them only
on a network you control. A plain `http://` address (DSM's port 5000) is accepted
with a warning, because it sends the password and session in clear text; a
certificate pin on an `http://` address is refused rather than sending the
password unprotected.

## Default-safe behavior

- **Almost everything reads.** Every Active Backup for Business, Hyper Backup,
  storage, system, log, package, folder, user and group command is a read, and the
  fleet reports (`fleet status`, `fleet health`, `abb failures`, `abb stale`,
  `abb device`, `abb trend`) read only the local mirror.
- **Three commands act on a NAS, and they do not ask first.** `abb backup` starts
  a backup, `abb cancel` cancels a running one, and `session logout` invalidates
  the CLI's DSM session. Each sends immediately unless you pass `--dry-run`, which
  prints the request without sending it. Make your agent's policy: preview, show
  the exact command, get approval, then run it.
- **An unknown answer is never OK.** A NAS that never synced, whose last sync
  failed, or whose mirror is older than `--max-sync-age` reads as unknown or
  `nas_not_synced`, never as healthy. The report commands exit 6 when anything
  needs attention (`--no-fail` exits 0), and `fleet sync` exits 5 when any NAS
  failed.
- **A read-only MCP server is one variable away.** With `SYNOLOGY_MCP_READ_ONLY=1`
  (any value other than blank, `0`, `false`, `no` or `off`) the MCP server drops
  every tool that is not read-only, including `abb_backup`, `abb_cancel`,
  `session_logout`, `fleet_sync` and the local writers (`sync`, `export`,
  `teach`, learnings and playbook writes).
- **MCP write tools take only their declared arguments.** `abb_backup`,
  `abb_cancel` and `session_logout` refuse any argument outside their schema,
  so an agent's `dry-run` is refused rather than ignored while the write runs.
  The read tools forward extra arguments to DSM as filters but refuse one that
  would override `api`, `method` or `version`, so a read tool can never be
  turned into a write.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no
  write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. Nothing changes on a NAS. | `fleet status`, `fleet health`, `fleet list`, `abb failures`, `abb stale`, `abb device`, `abb trend`, `abb triage`, `abb tasks`, `abb results`, `abb result-detail`, `abb logs`, `abb transfers`, `abb storage`, `abb settings`, `abb hypervisors`, `hyperbackup tasks`, `hyperbackup task-status`, `storage`, `system`, `log`, `package`, `folder`, `user`, `group`, `session apis`, `sync`, `search`, `export` (`sync` fills the local mirror; `export --output` overwrites the file you name) | Allow |
| **Write (routine)** | Starts or stops work on a NAS, or changes local state. | `abb backup`, `abb cancel`, `session logout` (act on a NAS); `fleet add`, `fleet remove` (change only the local registry and mirror); `fleet sync` (logs in to every registered NAS and rewrites the mirror) | Preview with `--dry-run`, then an approved write |
| **Credential** | Logs in, or stores or clears a session. | `auth login` (caches the DSM session and the two-factor trusted-device token), `auth logout` | Human-in-the-loop only |
| **Data egress** | Sends output off your machine. | `--deliver webhook:<url>` on any command (POSTs that command's output to the URL you name), `feedback --send` | Human-in-the-loop - a webhook sink moves client data off your machine |
| **Destructive / Admin** | Deletes, restores, reconfigures or reboots. | None: no command deletes backups, restores data, changes DSM settings or powers a NAS off | Not applicable |

## How to lock it down

- **Give each NAS a dedicated DSM account** with only the access the workflow
  needs, and two-factor sign-in on it. The CLI can do only what that account may
  do in DSM.
- **Run the MCP server read-only for unattended agents.** `SYNOLOGY_MCP_READ_ONLY=1`
  removes every write tool; an agent that only reports never needs them.
- **Keep autonomous agents to Read plus previewed writes.** Have a human approve
  `abb backup`, `abb cancel` and `session logout`: the gate lives in your agent's
  policy, not in the binary's defaults.
- **Pin each NAS certificate** (`fleet add --pin-cert`) instead of turning
  verification off.
- **Protect the data directory.** It holds the per-NAS session cache and the local
  mirror of your clients' backup history.
- **Rotate the DSM password if it is ever exposed**, for example after bridging
  the MCP server to a public endpoint for ChatGPT (see mcp-install.md), and remove
  the CLI's trusted device from the account in DSM.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply each NAS's login through environment
variables, the binary uses it against that NAS only, and you can read every line
of how it does so. The skill reads first, previews writes with `--dry-run`, and
never deletes, restores or reconfigures anything on a NAS.
