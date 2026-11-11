# Hook Security CLI

**Read Hook Security phishing and training data, and answer the cross-client questions an MSP actually asks.**

Every read-only Hook API endpoint as a command and an MCP tool. client-risk ranks clients by click rate, clicked-untrained joins simulation results to training, overdue-training groups late courses per client, and qbr assembles one client's review data.

Learn more at [Hook Security](https://api.hooksecurity.co/v1/docs).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `hook-security-cli` and `hook-security-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hook-security/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hook-security/install.ps1 | iex
   ```
3. Verify: `hook-security-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=hook-security). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/hook-security --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/hook-security --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `hook-security-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the hook-security skill from https://github.com/Servosity/msp-skills/tree/main/skills/hook-security. The skill defines how its required CLI (`hook-security-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=hook-security).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `HOOK_SECURITY_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hook-security/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hook-security/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "hook-security": {
      "command": "hook-security-mcp",
      "env": {
        "HOOK_SECURITY_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`hook-security-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `HOOK_SECURITY_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `HOOK_SECURITY_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export HOOK_SECURITY_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
hook-security-mcp --transport http
```

```powershell
$env:HOOK_SECURITY_MCP_HTTP_TOKEN = -join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Maximum 256) })
hook-security-mcp --transport http
```

Set `HOOK_SECURITY_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

An admin creates a key in Hook under Settings > API keys. An organization key reads that organization. An MSP key reads the MSP and all its clients, including new ones. Set it as HOOK_SECURITY_API_KEY.

## Quick Start

```bash
# Confirm the install without a key or network.
hook-security-cli doctor --dry-run

# See the organizations your key covers.
hook-security-cli organizations list --type client --agent

# Rank clients by click rate.
hook-security-cli client-risk --top 10 --agent

# Count overdue training per client.
hook-security-cli overdue-training --counts-only --agent

# Assemble one client's review data.
hook-security-cli qbr org_4521 --days 90 --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### MSP rollups
- **`client-risk`**  -  Rank every client of your MSP by phishing click rate, with a fleet rate computed from totals.

  _Reach for this when asked which clients are riskiest or what the click rate is across all clients._

  ```bash
  hook-security-cli client-risk --top 10 --agent
  ```
- **`qbr`**  -  Build one client's quarterly review data: simulation and training totals, monthly trends, recent campaigns and overdue training.

  _Reach for this when preparing a client business review on security awareness._

  ```bash
  hook-security-cli qbr org_4521 --days 90 --agent
  ```

### Simulation to training
- **`clicked-untrained`**  -  List people who clicked a simulation and have not finished training, with course status and due date.

  _Reach for this when asked who fell for a phish and still needs training._

  ```bash
  hook-security-cli clicked-untrained --campaign cmp_88213 --agent
  ```
- **`overdue-training`**  -  See overdue training grouped by client, with the people and courses behind each count.

  _Reach for this when asked who is overdue on training._

  ```bash
  hook-security-cli overdue-training --org org_4521 --agent
  ```

## Recipes

### Riskiest clients, narrow output

```bash
hook-security-cli client-risk --top 5 --agent --select clients.organization_name,clients.click_rate,fleet.click_rate
```

Keep only client names, click rates and the fleet rate.

### Follow up on one campaign

```bash
hook-security-cli clicked-untrained --campaign cmp_88213 --agent
```

List who clicked and still has training open.

### Overdue training for one client

```bash
hook-security-cli overdue-training --org org_4521 --agent
```

Show each overdue person and course for that client.

### Quarterly review data

```bash
hook-security-cli qbr org_4521 --days 90 --agent
```

Combine totals, trends, campaigns and overdue training.

## Usage

Run `hook-security-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `HOOK_SECURITY_CONFIG_DIR`, `HOOK_SECURITY_DATA_DIR`, `HOOK_SECURITY_STATE_DIR`, or `HOOK_SECURITY_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `HOOK_SECURITY_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export HOOK_SECURITY_HOME=/srv/hook-security
hook-security-cli doctor
```

On Windows (PowerShell):

```powershell
$env:HOOK_SECURITY_HOME = "C:\ProgramData\hook-security"
hook-security-cli doctor
```

Under `HOOK_SECURITY_HOME=/srv/hook-security`, the four dirs resolve to `/srv/hook-security/config`, `/srv/hook-security/data`, `/srv/hook-security/state`, and `/srv/hook-security/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:HOOK_SECURITY_HOME = "C:\srv\hook-security"
hook-security-cli doctor
```

Under `$env:HOOK_SECURITY_HOME = "C:\srv\hook-security"`, the four dirs resolve to `C:\srv\hook-security\config`, `C:\srv\hook-security\data`, `C:\srv\hook-security\state`, and `C:\srv\hook-security\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "hook-security": {
      "command": "hook-security-mcp",
      "env": {
        "HOOK_SECURITY_HOME": "/srv/hook-security"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `HOOK_SECURITY_DATA_DIR` overrides an explicit `--home` for that kind. Use `HOOK_SECURITY_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `HOOK_SECURITY_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `hook-security-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `hook-security-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `hook-security-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `HOOK_SECURITY_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `HOOK_SECURITY_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### billing

Billable users per account on the new Hook platform and month.

- **`hook-security-cli billing`** - Billable users for each account on the new Hook platform the key covers, for one calendar month, counted only from Hook's own records. An account created on the new platform bills the highest daily count of active people on a day it was active, the rule Hook's monthly usage report uses. An account that moved there from the legacy platform has no daily count yet: it bills the people in Hook's records at any point in the month, including anyone removed during it, who are not set inactive today. It is listed from the month its first simulation on the new platform started or its first training there was assigned, when it has such people and is not still suspended from before the month. Its rows are recomputed from today's records, so store the figures you invoice from. created_on_new_platform says which rule applies. Both start in October 2026; earlier months have no recorded counts. Which accounts are on the new platform is decided by their state today. Organizations still on the legacy platform are not listed. A client deleted during or after the month is still listed for it (organization_state deleted). Clients are grouped by today's organization tree: a client moved to another MSP is listed under its current MSP only. Without month, the current month so far; a cursor belongs to its month and filters. An MSP's own staff are free NFR seats from December 2026, up to 100 people: nfr_users says how many of an MSP row's billable_users that covers.

### campaigns

Phishing simulation campaigns and per-person results.

- **`hook-security-cli campaigns get`** - Get a campaign
- **`hook-security-cli campaigns list`** - Phishing simulation campaigns on the new Hook platform in the key's coverage. Counts equal Hook's own campaign report from the campaign's first send to today (UTC).

### courses

Manage courses

- **`hook-security-cli courses`** - Hook's published course catalog, the same for every key. Pass a course's ID as course_id on enrollment lists. A course since unpublished can still appear on older enrollments.

### enrollment-batches

Manage enrollment batches

- **`hook-security-cli enrollment-batches get-enrollment-batch`** - Get an enrollment batch
- **`hook-security-cli enrollment-batches list`** - Training assignments on the new Hook platform. Counts equal Hook's own enrollment report from the batch's first assignment to today (UTC).

### enrollments

Manage enrollments

- **`hook-security-cli enrollments`** - One row per learner per batch, including cancelled enrollments.

### groups

Manage groups

- **`hook-security-cli groups`** - Groups of people in Hook's own records. Pass a group's ID as group_id on the people list to read its members.

### organizations

The organizations a key covers.

- **`hook-security-cli organizations get`** - Get an organization
- **`hook-security-cli organizations list`** - Every organization the key covers: an MSP's whole tree, or the key's own organization. Filter by name to find one.

### people

People and groups in Hook's own records.

- **`hook-security-cli people get-person`** - Get a person
- **`hook-security-cli people list`** - People in Hook's own records, removed people left out. Find one person across every covered organization by email, or within one organization by name.


### clicked-untrained


- **`hook-security-cli clicked-untrained`** - List people who clicked a simulation and have not finished training, with course status and due date.


### client-risk


- **`hook-security-cli client-risk`** - Rank every client of your MSP by phishing click rate, with a fleet rate computed from totals.


### overdue-training


- **`hook-security-cli overdue-training`** - See overdue training grouped by client, with the people and courses behind each count.


### qbr


- **`hook-security-cli qbr`** - Build one client's quarterly review data: simulation and training totals, monthly trends, recent campaigns and overdue training.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`hook-security-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`hook-security-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`hook-security-cli learnings list`** - Inspect taught rows
- **`hook-security-cli learnings forget <query>`** - Undo a teach
- **`hook-security-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`hook-security-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`hook-security-cli teach-pattern`** - Install a query/resource template up front
- **`hook-security-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `HOOK_SECURITY_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `hook-security-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
hook-security-cli campaigns list

# JSON for scripting and agents
hook-security-cli campaigns list --json
# Filter to specific fields
hook-security-cli campaigns list --json --select created_at,engagement_measured,id

# Dry run  -  show the request without sending
hook-security-cli campaigns list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
hook-security-cli campaigns list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
hook-security-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `hook-security-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/hook-security-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\hook-security-cli\config.toml` on Windows; `--home`, `HOOK_SECURITY_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `HOOK_SECURITY_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `HOOK_SECURITY_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `HOOK_SECURITY_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `hook-security-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `hook-security-cli doctor` to check credentials
- Verify the environment variable is set: `echo $HOOK_SECURITY_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 401 unauthorized or key_revoked**  -  Set HOOK_SECURITY_API_KEY to a current key from Settings > API keys in Hook.
- **HTTP 403 coverage_too_large**  -  The key's organization tree is too large for one request; scope the command with --org org_4521.
- **HTTP 429 rate_limited**  -  Wait for the Retry-After period, then rerun with a smaller scope.
- **A client shows too_large with no rates**  -  Run qbr for that client with a shorter window, for example --days 30.
