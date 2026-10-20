# synology skill - governance and safety model

> Unofficial. Community-built skill for the Synology DSM API. Not affiliated with,
> endorsed by, or sponsored by Synology Inc.
> This page tells an MSP owner exactly what the synology skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `synology-cli` binary (and `synology-mcp`). DSM has no API
keys: the CLI logs in to each NAS with a DSM account through `SYNO.API.Auth`,
sending the password as a form POST, and then uses the session id and SynoToken
DSM returns. Use a DSM account in the administrators group: Active Backup for
Business management and Storage Manager calls need administrator rights, and
Active Backup's own Operator and Manager delegation roles are not tested with
this CLI. A dedicated account per client NAS keeps the MSP's access separate
from the client's own logins and easy to revoke.

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

**The password and the session go only to a DSM.** Before the first request
that carries a password, one-time code, trusted-device token or session (a
login, a cached session, or a raw session id in the config), the CLI asks the
address for `SYNO.API.Info` without credentials, once per address per run.
Only a host that answers the way DSM does gets the account, password or
session; anything else (a
mistyped address, the client's router, a web page, a redirect) fails with "does
not answer as a DSM WebAPI ... the password was not sent" (or "... the session
was not sent" for a cached or configured session). No DSM request ever follows a
redirect (DSM's WebAPI never redirects), so a DSM that redirects `http://` to
`https://` must be registered by its `https://` address. The probe proves the
host is a DSM, not that it is the NAS you meant: check each address you register.

**What is written to disk.** The CLI never writes a password or one-time code.
Your MCP client's config file does, if you put `SYNOLOGY_PASSWORD` (or a fleet
NAS's `<prefix>_PASSWORD`) in its `env` block: Claude Desktop, VS Code and
Gemini CLI store that file in plain text, so keep it private or set the
variables in the environment the client starts from. For the single NAS the CLI
caches the DSM session id, the SynoToken and the two-factor trusted-device token
in a per-NAS session file (`session-<scheme>-<host>_<port>-<hash>.json`) in its
data directory; for a fleet NAS, `fleet sync`
logs out when it finishes and keeps only the trusted-device token there. Session
files are created
owner-only (mode 0600) on macOS and Linux. On Windows the data directory sits
under your user profile (`%USERPROFILE%\.local\share\synology-cli`), which
Windows already limits to your account, SYSTEM and Administrators; keep it there
rather than under `C:\ProgramData` or a shared folder, or lock a relocated one
with `icacls <dir> /inheritance:r /grant:r "${env:USERNAME}:(OI)(CI)F"` in
PowerShell (`"%USERNAME%:(OI)(CI)F"` in cmd.exe). `export` shows
secret-named fields (passwords, tokens, encrypted share passwords) as
`[redacted]`, as the local store and the response cache do; and if a NAS answer
ever carries one of the CLI's own secret values (the password, a one-time code,
the trusted-device token, the session id or SynoToken) as a text value, every
output, the cache, the store and exports show `[redacted]` in its place (a whole
text field of any length; inside longer text, values of 8 characters or more).
Numbers are never rewritten. A cached session is reused only for the exact NAS
address and account it was issued for, and its cookies stay bound to that
address. Anyone who can read
the session file can act as the CLI's DSM
session until it expires, and the trusted-device token spares a future login the
second factor. `synology-cli auth logout` forgets the whole session file (every
account's session id, SynoToken, cookies and trusted-device token) of the NAS in
`SYNOLOGY_BASE_URL`, of a registered NAS with `--nas <name>`, or of every NAS
with `--all`, and lists each file it removed; with no NAS named it refuses and
changes nothing. It removes only files it can prove it wrote (its own session
file name and record format); anything else named like a session cache, for
example in a shared config folder, is listed as skipped and kept. It works locally and does not contact the NAS. `fleet remove`
does not touch a NAS's session file, so run `synology-cli auth logout --nas
<name>` before you retire a fleet NAS.
`synology-cli session logout` invalidates the session id on the NAS itself. To
revoke a trusted-device token for good, remove the device from the account's
trusted devices in DSM.

**Certificates.** Most NAS units serve DSM's self-signed certificate. `fleet add
--pin-cert` (or `--tls-fingerprint <sha256>`) and `SYNOLOGY_TLS_FINGERPRINT` pin
that exact certificate and refuse any other, which is the recommended path.
`--insecure` on one fleet NAS and `SYNOLOGY_SKIP_TLS_VERIFY=1` trust any
certificate and expose the password to anyone on the network path: use them only
on a network you control. A plain `http://` address (DSM's port 5000) is accepted,
but every login over it prints a warning on stderr and `doctor` reports its
Transport row as WARN, because it sends the password and session in clear text; a
certificate pin on an `http://` address is refused rather than sending the
password unprotected. When a NAS renews its certificate (a DSM self-signed
renewal, or Let's Encrypt every 90 days or so), the pin stops matching and every
call to that NAS fails with "the NAS certificate does not match the pinned
SHA-256 fingerprint", naming the fingerprint the NAS presented. Check that
fingerprint in DSM (Control Panel > Security > Certificate), then re-pin with
the value you checked: `synology-cli fleet add <name> --tls-fingerprint <sha256>`
for a registered NAS (keeps its address, settings and mirrored history) or a new
`SYNOLOGY_TLS_FINGERPRINT` for a single NAS. A pin mismatch is also what an
interception looks like, so never re-pin with `--pin-cert` (which trusts
whatever certificate is presented) without that check.

## Default-safe behavior

- **Almost everything reads.** Every Active Backup for Business command except
  `abb backup` and `abb cancel`, and every Hyper Backup, storage, system, log,
  package, folder, user and group command, is a read, and the
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
  turned into a write. The same rule covers `sync --param`, `--global-param` and
  `--resource-param` on the CLI and over MCP: they may add filters, never choose
  the DSM operation or session.
- **Agents cannot choose where a password or a file goes.** `fleet add`,
  `fleet remove` and the streaming `tail` command are CLI only, not MCP tools;
  `fleet_sync` over MCP reaches only NAS units the operator registered. MCP
  `teach` takes its playbook inline, never from a file path, and the output
  destination flags (`--deliver`, `--output`, `--db`, `--config`, `--home`) are
  refused over MCP.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no
  write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. Nothing changes on a NAS. | `fleet status`, `fleet health`, `fleet list`, `abb failures`, `abb stale`, `abb device`, `abb trend`, `abb triage`, `abb tasks`, `abb results`, `abb result-detail`, `abb logs`, `abb transfers`, `abb storage`, `abb settings`, `abb hypervisors`, `hyperbackup tasks`, `hyperbackup task-status`, `storage`, `system`, `log`, `package`, `folder`, `user`, `group`, `session apis`, `sync`, `search`, `export` (`sync` fills the local mirror; `export --output` overwrites the file you name) | Allow |
| **Write (routine)** | Starts or stops work on a NAS, or refreshes the mirror. | `abb backup`, `abb cancel`, `session logout` (act on a NAS); `fleet sync` (sends each registered NAS's env-var login to that NAS's registered URL, logs out when done, keeps the two-factor trusted-device token, and rewrites the mirror) | Preview with `--dry-run`, then an approved write |
| **Credential** | Sends a password, or stores or clears a session. | `auth login` (sends the password to `SYNOLOGY_BASE_URL` and caches the DSM session and the two-factor trusted-device token), `auth logout` | Human-in-the-loop only |
| **Admin (operator only)** | Decides which URL each NAS password is sent to. | `fleet add` (registers a NAS URL and the env vars whose login is sent there; `--check` logs in once), `fleet remove` (unregisters a NAS and deletes its mirror rows). CLI only: neither is offered as an MCP tool | Operator only, never an agent |
| **Data egress** | Sends output off your machine. | `--deliver webhook:<url>` on any command (POSTs that command's output to the URL you name), `feedback --send` | Human-in-the-loop - a webhook sink moves client data off your machine |
| **Destructive / Admin** | Deletes, restores, reconfigures or reboots. | None: no command deletes backups, restores data, changes DSM settings or powers a NAS off | Not applicable |

## How to lock it down

- **Give each NAS a dedicated DSM account** with only the access the workflow
  needs, and two-factor sign-in on it. The CLI can do only what that account may
  do in DSM.
- **Run the MCP server read-only for unattended agents.** `SYNOLOGY_MCP_READ_ONLY=1`
  removes every write tool; an agent that only reports never needs them.
- **Register NAS units yourself.** `fleet add` decides where each NAS password is
  sent, so it is CLI only and never offered to an agent over MCP; check each
  `--url` before you register it.
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
