# Windows Server CLI

**Agentless Windows Server health checks over WinRM: one verdict per server, a diffable inventory, and no arbitrary PowerShell.**

Every check is a fixed, parameterized PowerShell script sent over WinRM HTTPS with certificate verification always on. health rolls the checks into one verdict per server, inventory diff shows drift between runs, and services down explains why a service stopped. The MCP server can never pick a host or run its own script.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `windows-server-cli` and `windows-server-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1 | iex
   ```
3. Verify: `windows-server-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=windows-server). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/windows-server --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/windows-server --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `windows-server-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the windows-server skill from https://github.com/Servosity/msp-skills/tree/main/skills/windows-server. The skill defines how its required CLI (`windows-server-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle, Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=windows-server).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `WINDOWS_SERVER_USERNAME` and `WINDOWS_SERVER_PASSWORD` (the shared WinRM account) when Claude Desktop prompts you.
4. The bundle reads the same config file as the CLI, so the `[[servers]]` list must already exist at the default path (see Authentication); per-server `servers login` credentials are used too.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1 | iex            # Windows
```

Store each server's password first with `windows-server-cli servers login <server>` (see Authentication); the MCP server reads the same config file and credential stores. Then add it to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "windows-server": {
      "command": "windows-server-mcp"
    }
  }
}
```

Add an `env` block with `WINDOWS_SERVER_CONFIG` only when the config file is not at the default path.

</details>

### Remote MCP over HTTP

`windows-server-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `WINDOWS_SERVER_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `WINDOWS_SERVER_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export WINDOWS_SERVER_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
windows-server-mcp --transport http
```

Set `WINDOWS_SERVER_MCP_READ_ONLY=1` (exactly `1`) to remove every tool not marked read-only: `services_restart`, `services_start`, `health`, `inventory_snapshot`, `sync`, `export`, `workflow_archive`, and the learning writers (`teach`, `teach_playbook`, `teach_pattern`, `teach_lookup`, `playbook_amend`, `learnings_confirm`, `learnings_forget`, `learnings_reject`). There is no tool that runs an arbitrary script, and no MCP tool takes a host.

## Authentication

Windows Server has no API key. The CLI signs in to WinRM over HTTPS (port 5986 by default) with a domain or local account. Each server is a [[servers]] entry in the config file (default ~/.config/windows-server-cli/config.toml; override with --config or WINDOWS_SERVER_CONFIG): name, host, optional port, auth (ntlm, the default, or basic), username, and cert_sha256 or ca_file when its certificate does not chain to a system root. Other users must not be able to change that file (chmod 600), or the CLI refuses its servers. Store a password per server with servers login <name>, which reads it from stdin, or share one account through WINDOWS_SERVER_USERNAME and WINDOWS_SERVER_PASSWORD (auth set-credentials also saves the shared account, but takes the password as an argument, so it lands in shell history). Passwords shorter than 8 characters are refused. With NTLM, a local account other than the built-in Administrator gets HTTP 401 unless LocalAccountTokenFilterPolicy is 1 on the server: use a domain account, or auth = "basic" over HTTPS. Keep passwords out of config.toml; a legacy server_password there is still read for compatibility.

## Server setup

A server entry looks like this (`--server` takes the `name`, never a host):

```toml
[[servers]]
name = "fs01"
host = "fs01.corp.example"
auth = "ntlm"                  # or "basic"
username = "CORP\\svc-monitor"
cert_sha256 = "4B:EF:..."      # or ca_file = "/path/to/ca.pem"
default = true
```

## Quick Start

```bash
# After adding fs01 to the config file's [[servers]] list, store its password (read from stdin, never an argument)
windows-server-cli servers login fs01 --username 'CORP\svc-monitor' < password.txt

# Check every configured server and where its credential comes from (never the value)
windows-server-cli servers list --agent

# One verdict per configured server across every check; this is the real connectivity test
windows-server-cli health --agent

# Which automatic services are stopped, and why
windows-server-cli services down --agent

# Record roles, shares, admins, certificates and updates; run it again later and inventory diff shows what changed
windows-server-cli inventory snapshot --agent

```

## Unique Features

### Safe actions
- **`services restart`**  -  Restart one running service with services restart, or start a stopped automatic one with services start; each previews first and returns the service state before and after.

  _Reach for this when a ticket needs one specific service back up; it previews by default and only acts with confirmation._

  ```bash
  windows-server-cli services restart Spooler --server fs01 --agent
  ```

### Daily server check
- **`health`**  -  One ok, warn, crit or unknown verdict per server, with the reason for every failing check and how long it has been failing.

  _Use this first for any 'is this server healthy' question instead of running checks one by one._

  ```bash
  windows-server-cli health --agent
  ```
- **`services down`**  -  Stopped automatic services on every server, each paired with the Service Control Manager event that explains the stop.

  _Use this on a 'server is down' ticket before restarting anything._

  ```bash
  windows-server-cli services down --agent
  ```
- **`eventlog new`**  -  System and Application errors and warnings whose source and event ID are new on a server compared with its own history.

  _Use this during triage to separate a new failure from an error that fires every night._

  ```bash
  windows-server-cli eventlog new --window 24h --baseline 7d --agent
  ```

### Change tracking
- **`inventory diff`**  -  What changed in roles, features, shares, local administrators, certificates, installed updates and VMs between the two newest inventory snapshot runs.

  _Use this for drift reviews and change tickets: it names exactly what was added, removed or changed._

  ```bash
  windows-server-cli inventory diff --agent
  ```
- **`local-admins changes`**  -  Who added or removed each local Administrators member, when, and whether they are still a member.

  _Use this when a new local admin appears and you need the author and time._

  ```bash
  windows-server-cli local-admins changes --hours 168 --agent
  ```
- **`events list`**  -  Poll health-state changes between health runs with a sequence cursor.

  _Use this on every polling cycle to learn what changed since your last cursor._

  ```bash
  windows-server-cli events list --since 0 --agent
  ```

## Recipes

### Daily server check, narrowed

```bash
windows-server-cli health --agent --select server,verdict,checks.name,checks.status,checks.reason
```

One verdict per server with only the fields an agent needs from each check.

### Down ticket

```bash
windows-server-cli services down --server fs01 --agent
```

Lists stopped automatic services on fs01 with the Service Control Manager event behind each stop.

### Restart after preview

```bash
windows-server-cli services restart Spooler --server fs01 --yes
```

Run the same command without --yes first: it prints the preview and exits 2 with nothing changed. With --yes it restarts the Print Spooler on fs01 and returns state_before and state_after.

### Weekly drift review

```bash
windows-server-cli inventory diff --agent
```

Added, removed and changed inventory rows between the two newest inventory snapshot runs; run inventory snapshot on a schedule to feed it.

### Poll health changes

```bash
windows-server-cli events list --since 0 --agent
```

Returns health-state change events and the next cursor to poll from; a server's first health run records only the checks already failing.

## Limits to know before you run

- **Previews exit 2.** `services restart` and `services start` without `--yes` (or `confirm: true` over MCP) print the preview and exit 2 with nothing changed; a refused action also exits 2. Exit 0 means the change happened.
- **One server or every server.** health, services down, eventlog new, local-admins changes and inventory snapshot read every configured server. The single-resource reads, `services restart`, `services start` and `sync` act on one server; with several servers and none marked `default = true`, pass `--server`.
- **History needs earlier runs.** `events list` records what changed between `health` runs; a server's first run records only the checks already failing. `inventory diff` needs two `inventory snapshot` runs and exits 3 ("no inventory recorded yet") before the first.
- **Optional roles exit 3.** `hyperv-vms` on a server without the Hyper-V role, and `defender` on one without Microsoft Defender Antivirus, exit 3 (not found) and name what is missing; `health` reports those checks as n/a.
- **Audit policy.** `local-admins changes` only sees changes when the "Security Group Management" audit subcategory logs Success; otherwise each server returns a row with a warning. The policy is read from `auditpol` output, which is in the server's display language: on a non-English server the warning can appear although auditing is on.
- **Stored passwords follow the config file and the address.** `servers login` saves a password for that server name, under that config file, at its configured host and port; another config's server of the same name never gets it. Move the config, or change the host or port, and run `servers login` again.
- **No local data.** With `WINDOWS_SERVER_NO_LOCAL_DATA=1`, `health` prints its verdicts without recording them (no `failing_since` history, no events), and `inventory snapshot` is refused.
- **Basic needs HTTPS.** `auth = "basic"` with `transport = "http"` is refused: the password would cross the network in clear text.
- **Slow checks.** `pending-updates` takes 30-60 seconds, and `health` runs the same search.
- **MCP read-only mode.** With `WINDOWS_SERVER_MCP_READ_ONLY=1`, `health` and `inventory snapshot` are not offered (they write the local store), nor are `services restart` and `services start`.
- **MCP read tools have no server argument.** `certificates_list`, `volumes_list` and the other single-resource read tools always read the default server. The mirrored tools (`health`, `services_down`, `eventlog_new`, `local_admins_changes`, `services_restart`, `services_start`) take a `server` name.

## Usage

Run `windows-server-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml` (the shared account), `server-credentials.toml` (per-server logins, mode 0600), and `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `WINDOWS_SERVER_CONFIG_DIR`, `WINDOWS_SERVER_DATA_DIR`, `WINDOWS_SERVER_STATE_DIR`, or `WINDOWS_SERVER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `WINDOWS_SERVER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export WINDOWS_SERVER_HOME=/srv/windows-server
windows-server-cli doctor
```

On Windows (PowerShell):

```powershell
$env:WINDOWS_SERVER_HOME = "C:\ProgramData\windows-server"
windows-server-cli doctor
```

Under `WINDOWS_SERVER_HOME=/srv/windows-server`, the four dirs resolve to `/srv/windows-server/config`, `/srv/windows-server/data`, `/srv/windows-server/state`, and `/srv/windows-server/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:WINDOWS_SERVER_HOME = "C:\srv\windows-server"
windows-server-cli doctor
```

Under `$env:WINDOWS_SERVER_HOME = "C:\srv\windows-server"`, the four dirs resolve to `C:\srv\windows-server\config`, `C:\srv\windows-server\data`, `C:\srv\windows-server\state`, and `C:\srv\windows-server\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "windows-server": {
      "command": "windows-server-mcp",
      "env": {
        "WINDOWS_SERVER_HOME": "/srv/windows-server"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `WINDOWS_SERVER_DATA_DIR` overrides an explicit `--home` for that kind. Use `WINDOWS_SERVER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `WINDOWS_SERVER_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `windows-server-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory, and `health` and `inventory snapshot` record their runs there. Server data is never written to the HTTP response cache. Credentials live in `credentials.toml` and `server-credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `windows-server-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `windows-server-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `WINDOWS_SERVER_NO_LOCAL_DATA=1`. Reads go to the server over WinRM (`--data-source live`; `--data-source local` still reads an existing store), nothing new is written to the store by reads, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `WINDOWS_SERVER_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

Single-resource reads run one fixed script on one server: the one `--server` names, or the one marked `default = true`.

- `windows-server-cli certificates`: List certificates in a LocalMachine certificate store with expiry and days left (`--store`, `--expiring-days`)
- `windows-server-cli defender`: Read whether Defender real-time protection is on and how old its signatures are
- `windows-server-cli eventlog`: List recent critical, error and warning events from the System or Application log (`--log`, `--level`, `--hours`, `--max`)
- `windows-server-cli features`: List installed Windows Server roles, role services and features
- `windows-server-cli hyperv-vms`: List Hyper-V virtual machines with state, uptime, checkpoints and replication health
- `windows-server-cli installed-updates`: List installed Windows updates (hotfixes) with install date
- `windows-server-cli local-admins`: List the direct members of the local Administrators group with their SIDs
- `windows-server-cli pending-updates`: List software updates the Windows Update Agent finds applicable but not installed (takes 30-60 seconds)
- `windows-server-cli scheduled-tasks`: List scheduled tasks with state, last run time and last result code (`--failed`, `--include-microsoft`)
- `windows-server-cli services list`: List Windows services with state, start mode and service account (`--state`, `--start-mode`, `--stopped-automatic`)
- `windows-server-cli services get <name>`: Read one Windows service by its service name
- `windows-server-cli shares`: List SMB shares with path and whether each is an administrative share
- `windows-server-cli system`: Read the server's operating system, last boot time, uptime and whether a reboot is pending
- `windows-server-cli time-sync`: Read the Windows Time service state, time source, stratum and last successful sync
- `windows-server-cli volumes`: List fixed-disk volumes with size, free space and percent free

Fleet commands run on every configured server unless `--server` names one:

- `windows-server-cli health`: One ok, warn, crit or unknown verdict per server, with the reason for every failing check and how long it has been failing. Records each run in the local store.
- `windows-server-cli services down`: Stopped automatic services on every server, each paired with the Service Control Manager event that explains the stop (`--days`)
- `windows-server-cli eventlog new`: System and Application errors whose source and event ID are new on a server compared with its own history (`--window`, `--baseline`)
- `windows-server-cli local-admins changes`: Who added or removed each local Administrators member, when, and whether they are still a member (`--hours`)
- `windows-server-cli inventory snapshot`: Record the inventory of every server in the local store for inventory diff

Local store only (no server is contacted):

- `windows-server-cli inventory diff`: What changed in roles, features, shares, local administrators, certificates, installed updates and VMs between the two newest inventory snapshot runs.
- `windows-server-cli events list`: Poll health-state changes between health runs with a sequence cursor (`--since`, `--type`, `--limit`)

Service actions act on one server and only preview unless you pass `--yes` (CLI) or `confirm: true` (MCP):

- `windows-server-cli services restart <service-name>`: Restart one running service. Refused: WinRM and the services it depends on, a service with running dependents, and a service that is not running.
- `windows-server-cli services start <service-name>`: Start one stopped automatic-start service. Refused: a service that is not stopped, a manual or disabled service, and WinRM and the services it depends on.

Setup:

- `windows-server-cli servers list`: List configured servers and where each one's credential comes from (never the value)
- `windows-server-cli servers login <server>`: Store a credential for one server; the password is read from stdin, and `--username` defaults to the server's configured username
- `windows-server-cli servers logout <server>`: Remove one server's stored credential
- `windows-server-cli auth set-credentials <username> <password>`: Save the shared account used by every server without its own login (the password is an argument, so prefer `servers login` or `WINDOWS_SERVER_PASSWORD`)
- `windows-server-cli auth status`: Show whether a shared credential is configured
- `windows-server-cli auth logout`: Clear the stored shared credential (per-server logins stay until `servers logout`)


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`windows-server-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`windows-server-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`windows-server-cli learnings list`** - Inspect taught rows
- **`windows-server-cli learnings forget <query>`** - Undo a teach
- **`windows-server-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`windows-server-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`windows-server-cli teach-pattern`** - Install a query/resource template up front
- **`windows-server-cli teach-lookup`** - Add an entity mapping for pattern substitution (rarely needed here: server and service names are used verbatim)

Pass `--no-learn` or set `WINDOWS_SERVER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `windows-server-cli` opens the database, older binaries refuse it with a version error, so upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
windows-server-cli certificates

# JSON for scripting and agents
windows-server-cli certificates --json
# Filter to specific fields
windows-server-cli certificates --json --select days_left,dns_names,has_private_key

# Dry run: contact no server and print the placeholder request the command would answer
windows-server-cli certificates --dry-run

# Agent mode: JSON + compact + no prompts in one flag
windows-server-cli certificates --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` contacts no server: hand-written commands print a "would run ... on <target>" summary, and the single-resource reads print the placeholder request they would answer. For `services restart` and `services start`, the real preview is running the command without `--yes` (it exits 2 with nothing changed)
- **Reads only, with two exceptions** - `services restart` and `services start` change one service on one server, and only with `--yes` (CLI) or `confirm: true` (MCP)
- **Offline-friendly** - `sync`, `search` and `analytics` work from the local SQLite store; `sync` reads one server (`--server` or the default)
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success; for `services restart` and `services start`, the change was made and the service is Running |
| 2 | Usage error (bad flag or argument, a `--server` value that is not a server name; on hand-written commands also an unknown server name, or several servers with none marked `default = true`), a `services restart`/`start` preview without `--yes`, or a refused action |
| 3 | Not found: an unknown service name, or `inventory diff` before any snapshot ("no inventory recorded yet") |
| 4 | WinRM rejected the credentials, or the account was denied on the server; single-resource reads also return 4 when the server has no credential |
| 5 | WinRM or script failure on the server, or the service is not Running after a restart or start; single-resource reads also return 5 for config problems such as "no servers configured" |
| 10 | Config error on hand-written commands: no servers configured, a config file that cannot be read or that other users can change, or a server with no credential |

Exit 7 (rate limited) is not produced: a throttled WinRM listener surfaces as exit 5. Commands that read every server (health, services down, eventlog new, local-admins changes, inventory snapshot) print one row per server and exit 0 when at least one server answered; when none did, they exit with the first server's code.

## Health Check

```bash
windows-server-cli servers list --agent
windows-server-cli health --server fs01 --agent
```

`servers list` shows every configured server and where its credential comes from; `health --server <name>` is the real connectivity and credential test. `windows-server-cli doctor` shows resolved paths, the configured servers and whether each one has a credential (shared or from `servers login`), and local data, but it does not contact any server or check a certificate pin, so its api row reads `not verified here`.

## Configuration

Run `windows-server-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/windows-server-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\windows-server-cli\config.toml` on Windows; `--config <file>`, `WINDOWS_SERVER_CONFIG`, `--home`, `WINDOWS_SERVER_HOME`, and per-kind env vars can relocate it.

WinRM certificate trust is set per server in its `[[servers]]` entry: `cert_sha256` pins the exact certificate (the SHA-256 fingerprint, not the SHA-1 Windows thumbprint), `ca_file` replaces the system roots with a PEM bundle, and `tls_server_name` is the name checked when the certificate does not match `host`. With neither `cert_sha256` nor `ca_file`, the system roots are used. Verification is never skipped. The root `--ca-cert` and `--tls-server-name` flags (and `WINDOWS_SERVER_CA_CERT`, `WINDOWS_SERVER_TLS_SERVER_NAME`) do not apply to WinRM connections.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `WINDOWS_SERVER_USERNAME` | per_call | No (shared account) | WinRM account name for every server without its own `servers login` (a local or domain account in the server's Administrators group) |
| `WINDOWS_SERVER_PASSWORD` | per_call | No (shared account) | Password for that shared WinRM account |
| `WINDOWS_SERVER_CONFIG` | path | No | Config file to read instead of the default `config.toml` |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `windows-server-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie; the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `windows-server-cli servers list --agent` to see where each server's credential comes from
- Store a server's password again with `windows-server-cli servers login <server>` (stdin)
**Not found errors (exit code 3)**
- `services get` was given a service name the server does not have; check it with `windows-server-cli services list --agent`
- `inventory diff` says "no inventory recorded yet": run `windows-server-cli inventory snapshot` twice

### API-specific
- **x509: certificate signed by unknown authority**  -  Set cert_sha256 (the SHA-256 fingerprint of the server certificate; the 40-character Windows thumbprint is SHA-1 and is refused) or ca_file for that server in the config file, and tls_server_name when the certificate names a different host
- **plain HTTP refused**  -  Use an HTTPS listener on 5986. Plain HTTP needs transport = "http" on that server plus the CLI-only --allow-http flag, and is always refused under MCP
- **401 from WinRM**  -  With auth = "ntlm" (the default), a local account other than the built-in Administrator gets 401 unless LocalAccountTokenFilterPolicy is 1 on the server: use a domain account, or auth = "basic" over HTTPS with WinRM Basic enabled (winrm get winrm/config/service/auth). Otherwise check that server's account with servers list and store it again with servers login
- **pending-updates is slow**  -  The Windows Update Agent search takes 30-60 seconds on a server; it is read-only and never installs. health runs the same search, so health can take as long

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**pywinrm**](https://github.com/diyan/pywinrm): Python (1142 stars)
- [**masterzen/winrm**](https://github.com/masterzen/winrm): Go (480 stars)
- [**winrm-cli**](https://github.com/masterzen/winrm-cli): Go (171 stars)
- [**powershell-mcp**](https://github.com/IMRRD/powershell-mcp): TypeScript (1 stars)
- [**winrm-mcp**](https://github.com/bigbatmanorg/winrm-mcp): Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
