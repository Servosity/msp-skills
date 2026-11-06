# Afi CLI

**The first CLI for Afi SaaS backup  -  full public-API coverage plus the fleet-wide coverage, staleness, and offboarding answers the rate-limited API can't serve live.**

An open-source CLI and MCP server for Afi, whose vendor explicitly discourages API polling. This CLI walks the whole hierarchy into local SQLite once (fleet-sync), then answers fleet-wide questions (coverage-gaps, fleet-health, backup-stale, reconcile-licenses) offline. A guarded offboard command runs the vendor's own archive-then-release sequence with a verification gate the portal lacks.

Learn more at [Afi](https://afi.ai/docs/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `afi-cli` and `afi-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex
   ```
3. Verify: `afi-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=afi). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/afi --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/afi --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `afi-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the afi skill from https://github.com/Servosity/msp-skills/tree/main/skills/afi. The skill defines how its required CLI (`afi-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=afi).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `AFI_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `afi-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the installer above first (or set `AFI_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "afi": {
      "command": "afi-mcp",
      "env": {
        "AFI_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`afi-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `AFI_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `AFI_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export AFI_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
afi-mcp --transport http
```

Set `AFI_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Create an Application and API key in the Afi portal (org level: Configuration → Apps; tenant level: Service → Settings → Apps), then set AFI_API_KEY. The key is sent as the raw Authorization header value (Afi keys look like appkey-...; no Bearer prefix). Each Application supports two keys for rotation. Keys inherit the Application's installation scope  -  if a tenant or org is missing from results, check where the Application is installed.

## Quick Start

```bash
# Verify the binary, config, and connectivity plan before touching the API
afi-cli doctor --dry-run

# One respectful hierarchy walk pulls orgs, tenants, resources, protections, policies, archives, quotas, and task stats into local SQLite  -  then every question below runs offline
afi-cli fleet-sync

# The Monday sweep: task failures and quota breaches across every tenant in one table
afi-cli fleet-health --json

# List resources with no backup protection  -  the blind spots
afi-cli coverage-gaps --json

# Catch protected resources whose backups quietly stopped landing
afi-cli backup-stale --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`fleet-sync`**  -  Walk the whole Afi hierarchy  -  installations, orgs, tenants, then each tenant's resources, protections, policies, archives, quotas, and task stats  -  into local SQLite in one respectful pass.

  _Run this first (and on a schedule): every fleet question afterward  -  coverage, staleness, licensing, scorecards  -  answers offline without tripping Afi's rate limits._

  ```bash
  afi-cli fleet-sync
  ```
- **`coverage-gaps`**  -  Find resources in a tenant that have no backup protection applied  -  the backup blind spots.

  _Reach for this when asked whether every mailbox, site, or drive is actually being backed up  -  it answers fleet-coverage questions one API can't._

  ```bash
  afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --agent
  ```
- **`fleet-health`**  -  One table across all synced tenants: task success/failure counts, failed-task list, and quota breaches.

  _Use this for the Monday-morning 'is the whole fleet green' sweep instead of walking tenants one portal tab at a time._

  ```bash
  afi-cli fleet-health --since 24h --agent
  ```
- **`resolve`**  -  Map a Microsoft 365 / Google Workspace ID, email, or name to the canonical Afi resource or tenant, including Multi-Geo fan-out.

  _Run this before any offboard or audit to be certain which Afi object corresponds to an external-system identity._

  ```bash
  afi-cli resolve jane.doe@example.com --agent
  ```
- **`reconcile-licenses`**  -  Compare purchased subscription quantities against actually-protected resource counts per tenant to flag over- and under-provisioning.

  _Use this for the weekly licensing drift check that today requires manual spreadsheet stitching across two portal screens._

  ```bash
  afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
  ```
- **`backup-stale`**  -  Flag protected resources that are not getting backed up (no archive at all, or no backup task finished in their tenant in the synced window)  -  protected-but-silently-failing resources.

  _Use this to catch the silent-failure class: resources that have a policy attached but whose backups quietly stopped landing._

  ```bash
  afi-cli backup-stale --agent
  ```
- **`tenant-scorecard`**  -  One tenant's full backup posture: coverage percentage, last task status, newest/oldest archive age, and quota state.

  _Generate this card when prepping a customer ticket or QBR  -  the per-tenant deep dive that complements fleet-health._

  ```bash
  afi-cli tenant-scorecard 01F000000000000411Z1101G1Y --agent --select tenant_id,coverage_pct,quotas_exceeded
  ```

### Guarded workflows
- **`offboard`**  -  Safely back up a departing user's resource, verify the archive landed, then release the protection  -  refusing the irreversible step until the backup is confirmed.

  _Use this when offboarding an employee so the final backup is verified before the M365/GWS seat is released  -  never lose a leaver's data._

  ```bash
  afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --reason "employee departure"
  ```

## Recipes

### Monday fleet sweep

```bash
afi-cli fleet-sync --skip-archives && afi-cli fleet-health --failed-only --agent
```

Refresh the fleet store (skipping the heavy archives walk), then list only the tenants with failed tasks or quota breaches.

### Find backup blind spots in one tenant

```bash
afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --agent --select items.name,items.kind,items.external_id
```

LEFT JOIN of resources against protections, narrowed to the three fields an agent needs to open tickets.

### Offboard a departed employee safely

```bash
afi-cli resolve jane.doe@example.com && afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --reason "employee departure" --dry-run
```

Resolve the ambiguous email to the canonical Afi resource, then preview the guarded final-backup-verify-release sequence before running it for real.

### Catch silently failing backups

```bash
afi-cli backup-stale --json
```

Protected resources with no archive at all, or in a tenant where no backup task finished in the synced window  -  the failure class no portal screen surfaces.

### Licensing drift check

```bash
afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
```

Purchased subscription quantities vs actually-protected resource counts, per tenant.

## Usage

Run `afi-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `AFI_CONFIG_DIR`, `AFI_DATA_DIR`, `AFI_STATE_DIR`, or `AFI_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `AFI_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export AFI_HOME=/srv/afi
afi-cli doctor
```

On Windows (PowerShell):

```powershell
$env:AFI_HOME = "C:\ProgramData\afi"
afi-cli doctor
```

Under `AFI_HOME=/srv/afi`, the four dirs resolve to `/srv/afi/config`, `/srv/afi/data`, `/srv/afi/state`, and `/srv/afi/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:AFI_HOME = "C:\srv\afi"
afi-cli doctor
```

Under `$env:AFI_HOME = "C:\srv\afi"`, the four dirs resolve to `C:\srv\afi\config`, `C:\srv\afi\data`, `C:\srv\afi\state`, and `C:\srv\afi\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "afi": {
      "command": "afi-mcp",
      "env": {
        "AFI_HOME": "/srv/afi"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `AFI_DATA_DIR` overrides an explicit `--home` for that kind. Use `AFI_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `AFI_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `afi-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `afi-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `afi-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `AFI_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `AFI_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### applications

Manage applications

- **`afi-cli applications`** - Lists installations for the application identified by the authentication key.
Use the `limit` and `page_token` query parameters to paginate through all installations.

### orgs

Manage orgs

- **`afi-cli orgs create`** - Creates a new child organization.
- **`afi-cli orgs get`** - Retrieves an organization by its ID.

### tenants

Manage tenants

- **`afi-cli tenants <id>`** - Retrieves a tenant by its ID.


### backup-stale


- **`afi-cli backup-stale`** - Flag protected resources that are not getting backed up (no archive at all, or no backup task finished in their tenant in the synced window)  -  protected-but-silently-failing resources.


### coverage-gaps


- **`afi-cli coverage-gaps`** - Find resources in a tenant that have no backup protection applied  -  the backup blind spots.


### fleet-health


- **`afi-cli fleet-health`** - One table across all synced tenants: task success/failure counts, failed-task list, and quota breaches.


### fleet-sync


- **`afi-cli fleet-sync`** - Walk the whole Afi hierarchy  -  installations, orgs, tenants, then each tenant's resources, protections, policies, archives, quotas, and task stats  -  into local SQLite in one respectful pass.


### offboard


- **`afi-cli offboard`** - Safely back up a departing user's resource, verify the archive landed, then release the protection  -  refusing the irreversible step until the backup is confirmed.


### reconcile-licenses


- **`afi-cli reconcile-licenses`** - Compare purchased subscription quantities against actually-protected resource counts per tenant to flag over- and under-provisioning.


### resolve


- **`afi-cli resolve`** - Map a Microsoft 365 / Google Workspace ID, email, or name to the canonical Afi resource or tenant, including Multi-Geo fan-out.


### tenant-scorecard


- **`afi-cli tenant-scorecard`** - One tenant's full backup posture: coverage percentage, last task status, newest/oldest archive age, and quota state.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`afi-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`afi-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`afi-cli learnings list`** - Inspect taught rows
- **`afi-cli learnings forget <query>`** - Undo a teach
- **`afi-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`afi-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`afi-cli teach-pattern`** - Install a query/resource template up front
- **`afi-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `AFI_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `afi-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
afi-cli orgs get mock-value

# JSON for scripting and agents
afi-cli orgs get mock-value --json
# Filter to specific fields
afi-cli orgs get mock-value --json --select external_id,id,kind

# Dry run  -  show the request without sending
afi-cli orgs get mock-value --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
afi-cli orgs get mock-value --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
afi-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `afi-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/afi-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\afi-cli\config.toml` on Windows; `--home`, `AFI_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `AFI_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `AFI_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `AFI_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `afi-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The API key goes only to the Afi API host. `AFI_API_KEY` (sent raw in the `Authorization` header) goes only to the scheme, host and port of `https://papi.afi.ai`, or of `AFI_BASE_URL` when set. A redirect to any other host is refused, and a request that would carry the key anywhere but that header (a query value, an org name, another header) is refused before it is sent.
- Any output, file or MCP result that would repeat the key, or a credential-named value from a response, prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, every tool that changes Afi data (`orgs_create`, `tenants_archives_delete`, `tenants_jobs_tasks-trigger`, `tenants_resources_protections-protect`, `tenants_resources_protections-unprotect`, `offboard`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write (so `import`, which reads a local file, is CLI-only).

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `afi-cli doctor` to check credentials
- Verify the environment variable is set without printing it: `[ -n "$AFI_API_KEY" ] && echo set` (PowerShell: `if ($env:AFI_API_KEY) { 'set' }`)
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set AFI_API_KEY to a valid Application key (looks like appkey-...). The value is sent raw in the Authorization header  -  do not add a Bearer prefix.
- **429 Too Many Requests during fleet-sync**  -  Afi rate-limits aggressively and asks for exponential backoff. Re-run 'fleet-sync' later, scope it with --tenants, or add --skip-archives; prefer local queries over repeated live calls.
- **A tenant or org is missing from fleet-sync output**  -  The Application only sees orgs/tenants where it is installed. Check installations with 'afi-cli applications' and install the app in the missing org via the Afi portal.
- **coverage-gaps, fleet-health, or resolve returns nothing**  -  These commands read the local store. Run 'afi-cli fleet-sync' first; the stderr hints will tell you when the store is unsynced or stale.
