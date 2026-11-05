# Keeper Admin and MSP Console CLI

**Read-only Keeper MSP posture across every client, with history, a change feed and no path to a vault secret.**

Every managed company's 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seats in one run through Keeper Commander Service Mode. Runs are stored locally, so posture diff and events list show what changed since last time. A fixed allowlist of read-only reports is the whole surface: it cannot read a record or change a setting.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `keeper-cli` and `keeper-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex
   ```
3. Verify: `keeper-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=keeper). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/keeper --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/keeper --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `keeper-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the keeper skill from https://github.com/Servosity/msp-skills/tree/main/skills/keeper. The skill defines how its required CLI (`keeper-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=keeper).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `KEEPER_BASE_URL` (your Service Mode host) and `KEEPER_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "keeper": {
      "command": "keeper-mcp",
      "env": {
        "KEEPER_BASE_URL": "https://keeper-svc.example.internal:9090",
        "KEEPER_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`keeper-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `KEEPER_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `KEEPER_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export KEEPER_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
keeper-mcp --transport http
```

Set `KEEPER_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own local stores (the learn store, and the posture store `posture report` fills), stay available.

## Authentication

This CLI talks to Keeper Commander Service Mode, which you run on your own host (service-create). Set KEEPER_BASE_URL to that host (for example https://keeper-svc.example.internal:9090; https only, so run Service Mode with TLS: service-create -crtf and -crtp, and KEEPER_CA_CERT trusts a private certificate) and give it the Service Mode API key: export KEEPER_API_KEY, or store it once with 'keeper-cli auth set-token' (reads the key from stdin into the 0600 credential store). 'keeper-cli auth status' shows which credential source is in use without printing it, and 'keeper-cli auth logout' removes the stored key. Commander on that host must be logged in to your Keeper region (US, EU, AU, CA, JP or GOV) as an MSP or enterprise admin. Set the key's own command list to exactly the commands 'allowlist' prints.

## Quick Start

```bash
# Confirm the CLI is installed (dry run: sends nothing)
keeper-cli doctor --dry-run

# See the read-only commands this CLI can send; set Service Mode's -c list to the same
keeper-cli allowlist

# Every managed company with plan and seats
keeper-cli companies list --agent

# Record posture for every client
keeper-cli posture report --days 90 --agent

# Users without 2FA across every client
keeper-cli users risk --kind no-2fa --agent

# What changed since the previous run
keeper-cli posture diff --agent

```

## Unique Features

### Cross-client posture and history
- **`posture report`**  -  Fetch 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seat usage for every managed company in one run, and record it.

  _Reach for this when asked how every client is doing on Keeper security posture._

  ```bash
  keeper-cli posture report --days 90 --agent
  ```
- **`posture diff`**  -  Compare two stored posture runs (needs two 'posture report' runs): per-company metric deltas and per-user transitions such as 2FA turned off or newly at risk.

  _Use for a QBR 'since last time' answer without re-reading every client._

  ```bash
  keeper-cli posture diff --agent
  ```
- **`events list`**  -  Read posture change events (user.twofa_disabled, posture.weak_passwords_increased and more) after a cursor, once each.

  _Use to react to posture changes from automation without re-diffing._

  ```bash
  keeper-cli events list --since 0 --agent
  ```
- **`users find`**  -  Find which managed company a user belongs to, with status, 2FA and last login, across every client; reads the newest stored 'posture report' run (run it first).

  _Use when a ticket names only an email address._

  ```bash
  keeper-cli users find jane@example.com --agent
  ```
- **`users hygiene`**  -  List stale invites (with how long they have been pending), locked, inactive and pending-transfer users across every client; reads the newest stored 'posture report' run (run it first).

  _Use for weekly offboarding and cleanup sweeps._

  ```bash
  keeper-cli users hygiene --kind invited,inactive --agent
  ```
- **`users risk`**  -  List users without 2FA (admins outside any 2FA-enforcing role first), BreachWatch at-risk users and users with weak or reused passwords across every client; reads the newest stored 'posture report' run (run it first).

  _Use to answer which clients have users without 2FA._

  ```bash
  keeper-cli users risk --kind no-2fa --agent
  ```
- **`posture runs`**  -  List stored posture runs with their time, status and inactivity window.

  _Use to pick run ids for posture diff._

  ```bash
  keeper-cli posture runs --agent
  ```

### Read-only admin and MSP reports
- **`companies list`**  -  List every managed company with plan, seats allocated and active, storage and add-ons.

  _Use for seat and plan questions across clients._

  ```bash
  keeper-cli companies list --agent
  ```
- **`companies get`**  -  Show one managed company's plan, seats and add-ons by id.

  _Use for one client's licensing._

  ```bash
  keeper-cli companies get 4421 --agent
  ```
- **`products list`**  -  List MSP products, add-ons and file plans with pricing.

  _Use for pricing questions._

  ```bash
  keeper-cli products list --agent
  ```
- **`permits list`**  -  Show which products and add-ons the MSP may allocate and the maximum file plan.

  _Use before quoting a plan change._

  ```bash
  keeper-cli permits list --agent
  ```
- **`billing report`**  -  Show the monthly consumption billing statement, optionally by company and by date.

  _Use for the monthly billing close._

  ```bash
  keeper-cli billing report --month 2026-09 --by-company --agent
  ```
- **`users list`**  -  List an enterprise's or a managed company's users with status, transfer status, node, roles and 2FA.

  _Use for one client's user roster._

  ```bash
  keeper-cli users list --mc 4421 --agent
  ```
- **`users last-login`**  -  Show each user's last login within a window.

  _Use to check one client's activity live._

  ```bash
  keeper-cli users last-login --mc 4421 --days 90 --agent
  ```
- **`nodes list`**  -  List the enterprise node tree with user, team and role counts.

  _Use to understand a tenant's structure._

  ```bash
  keeper-cli nodes list --agent
  ```
- **`teams list`**  -  List teams with node and user count.

  _Use for team membership questions._

  ```bash
  keeper-cli teams list --mc 4421 --agent
  ```
- **`roles list`**  -  List roles with admin flag, default role, node and user count.

  _Use to find admin roles._

  ```bash
  keeper-cli roles list --mc 4421 --agent
  ```
- **`security-audit report`**  -  Show each user's weak, fair, medium, strong, reused and unique password counts, security score and 2FA.

  _Use for one client's password health._

  ```bash
  keeper-cli security-audit report --mc 4421 --agent
  ```
- **`breachwatch summary`**  -  Show each user's BreachWatch at-risk, passed and ignored counts.

  _Use for one client's breach exposure._

  ```bash
  keeper-cli breachwatch summary --mc 4421 --agent
  ```
- **`audit-trail events`**  -  Read raw audit events, newest first, filtered by period, user and event type.

  _Use to investigate who did what._

  ```bash
  keeper-cli audit-trail events --created last_7_days --limit 50 --agent
  ```
- **`audit-trail summary`**  -  Count audit events by type per day, week, month or the whole period.

  _Use for activity trends._

  ```bash
  keeper-cli audit-trail summary --period day --created last_7_days --agent
  ```
- **`allowlist`**  -  Print the read-only Commander commands this CLI can send and the matching Service Mode -c value.

  _Use to configure Service Mode's own allowlist for defense in depth._

  ```bash
  keeper-cli allowlist --agent
  ```

## Recipes

### Seats per client, narrowed

```bash
keeper-cli companies list --agent --select company_id,company_name,allocated,active
```

Allocated against active seats for every managed company, only the columns that matter.

### Quarterly posture snapshot

```bash
keeper-cli posture report --days 90 --agent
```

One run across every client; stored for the next diff.

### What changed since last run

```bash
keeper-cli posture diff --agent
```

Per-company deltas and per-user transitions between the two newest runs.

### Admins without 2FA

```bash
keeper-cli users risk --kind no-2fa --admins-only --agent
```

Admin users with 2FA off, across every client.

### Change feed for automation

```bash
keeper-cli events list --since 0 --agent
```

Typed events after a cursor; pass the returned cursor next time.

## Usage

Run `keeper-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml` and the posture store `data.db` (or `data-<hash>.db`) |
| `state` | Runtime state such as the learn journal and `teach.log` |
| `cache` | Regenerable files (the HTTP response cache is off in this CLI) |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `KEEPER_CONFIG_DIR`, `KEEPER_DATA_DIR`, `KEEPER_STATE_DIR`, or `KEEPER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `KEEPER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export KEEPER_HOME=/srv/keeper
keeper-cli doctor
```

On Windows (PowerShell):

```powershell
$env:KEEPER_HOME = "C:\ProgramData\keeper"
keeper-cli doctor
```

Under `KEEPER_HOME=/srv/keeper`, the four dirs resolve to `/srv/keeper/config`, `/srv/keeper/data`, `/srv/keeper/state`, and `/srv/keeper/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:KEEPER_HOME = "C:\srv\keeper"
keeper-cli doctor
```

Under `$env:KEEPER_HOME = "C:\srv\keeper"`, the four dirs resolve to `C:\srv\keeper\config`, `C:\srv\keeper\data`, `C:\srv\keeper\state`, and `C:\srv\keeper\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "keeper": {
      "command": "keeper-mcp",
      "env": {
        "KEEPER_HOME": "/srv/keeper"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `KEEPER_DATA_DIR` overrides an explicit `--home` for that kind. Use `KEEPER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `KEEPER_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `keeper-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Typed reads are not saved: they go to Service Mode and print. `posture report` saves each run (per-company metrics and per-user facts: email, name, status, 2FA, last login, password-health counts) and its change events to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory; `posture diff`, `posture runs`, `events list` and `users find`, `users hygiene` and `users risk` read it. The HTTP response cache is off. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `keeper-cli doctor` prints the resolved paths and every store file.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `keeper-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `KEEPER_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `posture report` refuses to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `KEEPER_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Security model

- **Read-only by construction.** The CLI can send only the Commander commands `keeper-cli allowlist` prints (msp-info, msp-billing-report, enterprise-info, user-report, security-audit-report, audit-report, switch-to-mc, switch-to-msp), each with fixed read-only flags. Every value you pass (a managed company id, a day count, a month, an email, an event type) is checked against a strict pattern and sent as one token, so no argument can add a flag or another command; anything else is refused before a request is made.
- **Vault record contents are out of scope.** Nothing here reads records, passwords, attachments, shared-folder secrets or one-time shares: an admin and MSP view does not need them, and leaving them out means an agent with this CLI cannot leak them. Report rows are cut to known fields, and a response that carries anything shaped like record content is refused whole.
- **Defense in depth: set Service Mode's own allowlist.** Create the Service Mode key with exactly the commands above (`service-create -c 'msp-info,msp-billing-report,enterprise-info,user-report,security-audit-report,audit-report,switch-to-mc,switch-to-msp'`, or `service-config-add`), so Service Mode itself refuses anything else even if another tool shares the host.
- **Managed-company context is shared.** Commander's managed-company context is one global per Service Mode process. Reads return the session to the MSP first; `--mc` reads switch in, check Keeper confirmed the switch, read, and switch back. Every run of this CLI for this user on the machine (the CLI and the MCP server alike, whatever `--home` or environment it runs with) holds one lock per Service Mode host for each read, so this CLI's own reads never interleave, and every read checks afterwards that the session is still where it was: a read another client disturbed is discarded with an error, never shown or stored. Run this CLI against a Service Mode instance no other tool is switching at the same time (a separate API key does not isolate it, and another OS user or another machine running this CLI counts as another tool).
- **The MCP server writes almost nothing.** `posture_report` saves its run to the local store, and it returns a preview and sends nothing until it is called with `confirm` set to boolean true. The learning commands (`teach`, `learnings confirm`, `learnings forget`, `playbook amend`) are CLI-only, not MCP tools. Read-only tools run directly and write nothing, not even a usage row.
- **The API key.** `KEEPER_API_KEY` is sent only to `KEEPER_BASE_URL` (scheme, host and port); no redirect is ever followed, and plain `http` is refused on every host, localhost included: run Service Mode with TLS (`service-create -crtf ... -crtp ...`), and trust a private certificate with `KEEPER_CA_CERT`. The key is withheld from every output, log, error, file and MCP result, in every encoding. It is stored only in the 0600 credential store.

## Commands

### health

Check that the Service Mode host is up

- **`keeper-cli health`** - Liveness probe. Answers {"status":"ok"} with no authentication and is exempt from the Service Mode rate limit (keepercommander/service/api/routes.py init_routes health_check).

### queue

Read the Service Mode request queue

- **`keeper-cli queue`** - Service Mode queue status: the whole queue's size plus active and completed request counts, the request currently processing and whether the worker runs. Requires Commander 17.1.7 or later with queue mode on (the default); a host in legacy mode answers 404 here and serves only /api/v1. Source: keepercommander/service/api/command.py get_queue_status, keepercommander/service/core/request_queue.py.


### allowlist


- **`keeper-cli allowlist`** - Print the read-only Commander commands this CLI can send and the matching Service Mode -c value.


### audit-trail


- **`keeper-cli audit-trail events`** - Read raw audit events, newest first, filtered by period, user and event type.

- **`keeper-cli audit-trail summary`** - Count audit events by type per day, week, month or the whole period.


### billing


- **`keeper-cli billing report`** - Show the monthly consumption billing statement, optionally by company and by date.


### breachwatch


- **`keeper-cli breachwatch summary`** - Show each user's BreachWatch at-risk, passed and ignored counts.


### companies


- **`keeper-cli companies list`** - List every managed company with plan, seats allocated and active, storage and add-ons.

- **`keeper-cli companies get`** - Show one managed company's plan, seats and add-ons by id.


### events


- **`keeper-cli events list`** - Read posture change events (user.twofa_disabled, posture.weak_passwords_increased and more) after a cursor, once each.


### nodes


- **`keeper-cli nodes list`** - List the enterprise node tree with user, team and role counts.


### permits


- **`keeper-cli permits list`** - Show which products and add-ons the MSP may allocate and the maximum file plan.


### posture


- **`keeper-cli posture report`** - Fetch 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seat usage for every managed company in one run, and record it.

- **`keeper-cli posture diff`** - See what changed between two posture runs: per-company metric deltas and per-user transitions such as 2FA turned off or newly at risk.

- **`keeper-cli posture runs`** - List stored posture runs with their time, status and inactivity window.


### products


- **`keeper-cli products list`** - List MSP products, add-ons and file plans with pricing.


### roles


- **`keeper-cli roles list`** - List roles with admin flag, default role, node and user count.


### security-audit


- **`keeper-cli security-audit report`** - Show each user's weak, fair, medium, strong, reused and unique password counts, security score and 2FA.


### teams


- **`keeper-cli teams list`** - List teams with node and user count.


### users


- **`keeper-cli users find`** - Find which managed company a user belongs to, with status, 2FA and last login, across every client at once.

- **`keeper-cli users hygiene`** - List stale invites (with how long they have been pending), locked users, inactive users and pending account transfers across every client.

- **`keeper-cli users risk`** - List users without 2FA (admins outside any 2FA-enforcing role first), BreachWatch at-risk users and users with weak or reused passwords across every client.

- **`keeper-cli users list`** - List an enterprise's or a managed company's users with status, transfer status, node, roles and 2FA.

- **`keeper-cli users last-login`** - Show each user's last login within a window.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`keeper-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`keeper-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`keeper-cli learnings list`** - Inspect taught rows
- **`keeper-cli learnings forget "$QUERY" --all`** - Undo a teach
- **`keeper-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`keeper-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`keeper-cli teach-pattern`** - Install a query/resource template up front
- **`keeper-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `KEEPER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `keeper-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
keeper-cli health

# JSON for scripting and agents
keeper-cli health --json
# Filter to specific fields
keeper-cli health --json --select status

# Dry run  -  show the request without sending
keeper-cli health --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
keeper-cli health --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - `posture diff`, `posture runs`, `events list` and `users find`, `users hygiene` and `users risk` read the local SQLite store
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
keeper-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `keeper-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/keeper-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\keeper-cli\config.toml` on Windows; `--home`, `KEEPER_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `KEEPER_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `KEEPER_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `KEEPER_API_KEY` | per_call | Yes | The Keeper Commander Service Mode API key (service-create prints it). |
| `KEEPER_BASE_URL` | per_call | Yes | Your Service Mode host, e.g. `https://keeper-svc.example.internal:9090` (default: this machine, port 9090; https only). |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `keeper-cli doctor` reports `agentcookie: detected` and `keeper-cli auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `keeper-cli doctor` to check credentials
- Check which credential source is in use (the key is never printed): `keeper-cli auth status`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `keeper-cli companies list` to see managed company ids

### API-specific
- **403 Not permitted to perform this function**  -  Add the command to this key's Service Mode allowlist: service-config-add -c with the list 'allowlist' prints
- **No active session. Please log in through the CLI first**  -  Log Commander in on the Service Mode host (login, then service-start)
- **Service Mode answered without a JSON object**  -  Turn off response encryption (-ek) for the Service Mode host this CLI uses
- **This command is restricted to Keeper Enterprise administrators**  -  Log Service Mode in with an enterprise or MSP admin account
- **request refused: it would leave the configured Keeper Service Mode host**  -  Set KEEPER_BASE_URL to the Service Mode host; redirects to other hosts are never followed
