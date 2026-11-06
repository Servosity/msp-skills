# KnowBe4 CLI

**Every KnowBe4 KMSAT reporting feature, plus a local SQLite store for the questions the console cannot answer.**

knowbe4-cli mirrors the KnowBe4 Reporting API (users, groups, phishing security tests and campaigns, training campaigns and enrollments, policies, and risk-score history) and syncs it all into a local SQLite database. That local store powers transcendence commands no other KnowBe4 tool has: repeat-clickers finds humans who failed multiple phishing tests, untrained-clickers anti-joins phishing failures against training records, risk-drift ranks who deteriorated this quarter, and qbr assembles a full client quarterly review in one shot.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `knowbe4-cli` and `knowbe4-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex
   ```
3. Verify: `knowbe4-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=knowbe4). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/knowbe4 --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/knowbe4 --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `knowbe4-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the knowbe4 skill from https://github.com/Servosity/msp-skills/tree/main/skills/knowbe4. The skill defines how its required CLI (`knowbe4-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=knowbe4).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `KNOWBE4_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "knowbe4": {
      "command": "knowbe4-mcp",
      "env": {
        "KNOWBE4_REGION": "<region>",
        "KNOWBE4_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`knowbe4-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `KNOWBE4_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `KNOWBE4_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export KNOWBE4_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
knowbe4-mcp --transport http
```

Set `KNOWBE4_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Authentication uses a Reporting API key (a Bearer token) generated in the KSAT console under Account Settings -> API. Export it as KNOWBE4_API_KEY. If your tenant lives on a non-US server, set KNOWBE4_REGION to eu, ca, uk, or de so the CLI targets the right regional host. The separate User Event API (push custom risk events) uses its own key, KNOWBE4_USER_EVENT_API_KEY, consumed only by the `events` command group.

## Quick Start

```bash
# Confirm KNOWBE4_API_KEY is set and the Reporting API is reachable for your region
knowbe4-cli doctor

# Pull users, groups, phishing tests/campaigns, training, and policies into local SQLite
knowbe4-cli sync

# List users with risk score and phish-prone percentage
knowbe4-cli users list --per-page 100 --agent

# Rank the highest-risk users with click/report/training context attached
knowbe4-cli risk-leaderboard --top 25 --agent

# Hunt the humans who clicked the bait in multiple phishing tests
knowbe4-cli repeat-clickers --min-clicks 2 --since 90d --agent

# Assemble a full client quarterly review in one command
knowbe4-cli qbr --since 90d --format md

```

## Unique Features

### Phishing behavior intelligence
- **`repeat-clickers`**  -  Surface users who clicked the bait in two or more distinct phishing security tests within a window  -  your hardest-to-train humans.

  _Reach for this when an agent needs the riskiest 5% of humans to target for enforced training instead of blasting everyone._

  ```bash
  knowbe4-cli repeat-clickers --min-clicks 2 --since 90d --top 25 --agent
  ```
- **`report-rate`**  -  Rank users and groups by how often they report simulated phish versus falling for them - surfacing who never uses the Phish Alert Button.

  _Reach for this when the question is about reporting behavior (good security instincts), not just failure counts._

  ```bash
  knowbe4-cli report-rate --bottom 25 --agent
  ```

### Risk posture that compounds
- **`risk-drift`**  -  Rank users and groups by how much their risk score worsened (or improved) between a prior snapshot and now.

  _Pick this for QBR/board prep  -  it turns a wall of risk numbers into the short list of who is deteriorating._

  ```bash
  knowbe4-cli risk-drift --window 90d --worsened --top 20 --agent
  ```
- **`phish-prone-trend`**  -  Plot a group's phish-prone percentage across sequential phishing tests to show whether training is actually working.

  _Produces the headline QBR chart ('phish-prone went 28% to 6%') per group from your own synced history._

  ```bash
  knowbe4-cli phish-prone-trend --group "Finance" --since 12mo --agent
  ```
- **`risk-leaderboard`**  -  Highest-risk users ranked, each enriched with click history, report rate, and overdue-training context.

  _Reach for this when a risk number needs context  -  clicked twice, reported never, two trainings past due  -  so the remediation writes itself._

  ```bash
  knowbe4-cli risk-leaderboard --top 25 --agent
  ```
- **`group-risk-contribution`**  -  Attribute account-level risk movement to the specific groups (departments) driving it up or down.

  _Answers 'why did our risk go up' by department instead of hand-waving  -  turning a board question into a targeted action._

  ```bash
  knowbe4-cli group-risk-contribution --window 90d --top 10 --agent
  ```

### Coverage anti-joins
- **`untrained-clickers`**  -  Anti-join: users who clicked a phish but have no Passed training enrollment to show for it.

  _The single most actionable remediation list a vCISO owns: people who proved they're vulnerable and have zero completed training._

  ```bash
  knowbe4-cli untrained-clickers --since 180d --agent
  ```
- **`coverage-gaps`**  -  Find active users enrolled in zero phishing campaigns or zero training campaigns  -  the people your program silently misses.

  _Use when closing compliance holes  -  uncovered users (including unenrolled new hires) are unmeasured risk._

  ```bash
  knowbe4-cli coverage-gaps --type training --joined-within 30d --agent
  ```

### Reporting rituals
- **`qbr`**  -  One command that assembles the full quarterly review: risk trend, phish-prone trend, training completion, and the top-risk humans.

  _Collapses an afternoon of CSV exports and pivots into one command  -  the biggest recurring time win for an MSP._

  ```bash
  knowbe4-cli qbr --since 90d --format md
  ```

### Local state that compounds
- **`freshness`**  -  See per-table sync freshness and whether the cross-entity commands have the data they need - before trusting a clicker hunt.

  _Run this first when results look empty or stale; it names the exact sync command that fixes the gap._

  ```bash
  knowbe4-cli freshness --agent
  ```

## Recipes

### Sync then hunt repeat clickers

```bash
knowbe4-cli sync && knowbe4-cli repeat-clickers --min-clicks 2 --since 180d --agent
```

Pull everything local, then surface users who failed two or more phishing tests in the last 180 days.

### Narrow a deeply-nested phishing test payload

```bash
knowbe4-cli phishing-tests get 16142 --agent --select pst_id,name,phish_prone_percentage,clicked_count,reported_count,groups.name
```

PST payloads are large and nested; --select with dotted paths returns only the fields an agent needs.

### Find vulnerable, untrained users

```bash
knowbe4-cli untrained-clickers --since 180d --agent --select user_email,clicked_psts,current_risk_score
```

Anti-join of phishing failures against training records  -  the next enrollment campaign, ready to act on.

### Build a client QBR

```bash
knowbe4-cli qbr --since 90d --format md
```

Assemble risk trend, phish-prone trend, training completion, and top-risk humans into one markdown report.

### Free-text search the synced store

```bash
knowbe4-cli search "finance" --agent
```

Full-text search across synced users, groups, and campaigns without a round-trip to the API.

### Find who never reports phish

```bash
knowbe4-cli report-rate --bottom 25 --agent
```

Rank the users with the worst report-vs-click behavior - the people who never use the Phish Alert Button.

### Check the store before you trust a hunt

```bash
knowbe4-cli freshness --agent
```

Per-table sync freshness plus input-readiness for every cross-entity command - names the sync command that fixes any gap.

## Usage

Run `knowbe4-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `KNOWBE4_CONFIG_DIR`, `KNOWBE4_DATA_DIR`, `KNOWBE4_STATE_DIR`, or `KNOWBE4_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `KNOWBE4_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export KNOWBE4_HOME=/srv/knowbe4
knowbe4-cli doctor
```

On Windows (PowerShell):

```powershell
$env:KNOWBE4_HOME = "C:\ProgramData\knowbe4"
knowbe4-cli doctor
```

Under `KNOWBE4_HOME=/srv/knowbe4`, the four dirs resolve to `/srv/knowbe4/config`, `/srv/knowbe4/data`, `/srv/knowbe4/state`, and `/srv/knowbe4/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:KNOWBE4_HOME = "C:\srv\knowbe4"
knowbe4-cli doctor
```

Under `$env:KNOWBE4_HOME = "C:\srv\knowbe4"`, the four dirs resolve to `C:\srv\knowbe4\config`, `C:\srv\knowbe4\data`, `C:\srv\knowbe4\state`, and `C:\srv\knowbe4\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "knowbe4": {
      "command": "knowbe4-mcp",
      "env": {
        "KNOWBE4_HOME": "/srv/knowbe4"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `KNOWBE4_DATA_DIR` overrides an explicit `--home` for that kind. Use `KNOWBE4_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `KNOWBE4_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `knowbe4-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `knowbe4-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `knowbe4-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `KNOWBE4_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `KNOWBE4_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### account

Your KnowBe4 account: subscription, seat count, admins, and org-level risk score

- **`knowbe4-cli account info`** - Get account and subscription data (name, subscription level, seats, admins, current risk score)
- **`knowbe4-cli account risk-score-history`** - Get the org-level risk-score history time series

### groups

Groups/Smart Groups  -  membership counts and group-level risk score

- **`knowbe4-cli groups get`** - Get a single group by id
- **`knowbe4-cli groups list`** - List all groups with member counts and risk scores
- **`knowbe4-cli groups members`** - List the users in a group
- **`knowbe4-cli groups risk-score-history`** - Get a group's risk-score history time series

### phishing-campaigns

Phishing campaigns  -  recurring/one-time simulated-phishing programs

- **`knowbe4-cli phishing-campaigns get`** - Get a single phishing campaign by id
- **`knowbe4-cli phishing-campaigns list`** - List all phishing campaigns
- **`knowbe4-cli phishing-campaigns security-tests`** - List the phishing security tests (PSTs) run by a campaign

### phishing-tests

Phishing security tests (PSTs)  -  individual simulated-phishing sends with click/report stats

- **`knowbe4-cli phishing-tests get`** - Get a single phishing security test by id
- **`knowbe4-cli phishing-tests list`** - List all phishing security tests (PSTs) across every campaign
- **`knowbe4-cli phishing-tests recipient`** - Get a single recipient's result within a PST
- **`knowbe4-cli phishing-tests recipients`** - List per-user results for a PST (who opened, clicked, reported, entered data)

### policies

Uploaded policies  -  policy documents assigned for acknowledgement

- **`knowbe4-cli policies get`** - Get a single policy by id
- **`knowbe4-cli policies list`** - List all uploaded policies

### store-purchases

Store purchases  -  training content/modules licensed to your account

- **`knowbe4-cli store-purchases get`** - Get a single store purchase by id
- **`knowbe4-cli store-purchases list`** - List all training store purchases (licensed content/modules)

### training-campaigns

Training campaigns  -  assigned security-awareness training programs

- **`knowbe4-cli training-campaigns get`** - Get a single training campaign by id
- **`knowbe4-cli training-campaigns list`** - List all training campaigns

### training-enrollments

Training enrollments  -  per-user module assignments with completion status

- **`knowbe4-cli training-enrollments get`** - Get a single training enrollment by id
- **`knowbe4-cli training-enrollments list`** - List all training enrollments with completion status and time spent

### users

Users in the KnowBe4 console  -  phish-prone percentage, current risk score, group membership

- **`knowbe4-cli users get`** - Get a single user by id
- **`knowbe4-cli users list`** - List all users with risk score and phish-prone percentage
- **`knowbe4-cli users risk-score-history`** - Get a user's risk-score history time series


### coverage-gaps


- **`knowbe4-cli coverage-gaps`** - Find active users enrolled in zero phishing campaigns or zero training campaigns  -  the people your program silently misses.


### freshness


- **`knowbe4-cli freshness`** - See per-table sync freshness and whether the cross-entity commands have the data they need - before trusting a clicker hunt.


### group-risk-contribution


- **`knowbe4-cli group-risk-contribution`** - Attribute account-level risk movement to the specific groups (departments) driving it up or down.


### phish-prone-trend


- **`knowbe4-cli phish-prone-trend`** - Plot a group's phish-prone percentage across sequential phishing tests to show whether training is actually working.


### qbr


- **`knowbe4-cli qbr`** - One command that assembles the full quarterly review: risk trend, phish-prone trend, training completion, and the top-risk humans.


### repeat-clickers


- **`knowbe4-cli repeat-clickers`** - Surface users who clicked the bait in two or more distinct phishing security tests within a window  -  your hardest-to-train humans.


### report-rate


- **`knowbe4-cli report-rate`** - Rank users and groups by how often they report simulated phish versus falling for them - surfacing who never uses the Phish Alert Button.


### risk-drift


- **`knowbe4-cli risk-drift`** - Rank users and groups by how much their risk score worsened (or improved) between a prior snapshot and now.


### risk-leaderboard


- **`knowbe4-cli risk-leaderboard`** - Highest-risk users ranked, each enriched with click history, report rate, and overdue-training context.


### untrained-clickers


- **`knowbe4-cli untrained-clickers`** - Anti-join: users who clicked a phish but have no Passed training enrollment to show for it.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`knowbe4-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`knowbe4-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`knowbe4-cli learnings list`** - Inspect taught rows
- **`knowbe4-cli learnings forget <query>`** - Undo a teach
- **`knowbe4-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`knowbe4-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`knowbe4-cli teach-pattern`** - Install a query/resource template up front
- **`knowbe4-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `KNOWBE4_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `knowbe4-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
knowbe4-cli groups list

# JSON for scripting and agents
knowbe4-cli groups list --json
# Filter to specific fields
knowbe4-cli groups list --json --select id,name,group_type

# Dry run  -  show the request without sending
knowbe4-cli groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
knowbe4-cli groups list --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `KNOWBE4_REGION` resolves `{region}`

Base URL: `https://{region}.api.knowbe4.com/v1`

## Health Check

```bash
knowbe4-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `knowbe4-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/knowbe4-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\knowbe4-cli\config.toml` on Windows; `--home`, `KNOWBE4_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `KNOWBE4_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `KNOWBE4_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `KNOWBE4_REGION` | endpoint | Yes |  |
| `KNOWBE4_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `knowbe4-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The Reporting API key (`Authorization: Bearer <KNOWBE4_API_KEY>`) and every Reporting API request go only to the scheme, host and port of the configured base URL (`https://<region>.api.knowbe4.com/v1` built from `KNOWBE4_REGION`, default `us`, or `KNOWBE4_BASE_URL`). A redirect to any other host is refused, so the key never reaches it, and a region that is not a single host-name label is refused before anything is sent.
- The User Event API key (`KNOWBE4_USER_EVENT_API_KEY`, used only by the `events` commands) goes only to the User Event API host for your region (`https://api.events.knowbe4.com`, or `https://api-<region>.events.knowbe4.com`; `KNOWBE4_USER_EVENT_BASE_URL` overrides it), with the same redirect refusal. Neither key is ever sent to the other API.
- Both programs hold every credential they know from the first byte of output: both keys, the MCP HTTP token, and the credentials in `config.toml`, `credentials.toml` and the run profiles. Any output that would carry one of them, in any encoding, is replaced whole by a notice naming where it was going, for example "[credkit: withheld from stdout: it carried a credential this program holds]" (the sink is stdout, stderr, mcp, file, sqlite, outbound request or child process). A request that would carry a held credential to a host it does not belong to, over plain http, or across a redirect to another host is refused before it is sent. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, `events_create` and `events_delete` (the only commands that change KnowBe4 data) return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `knowbe4-cli doctor` to check credentials
- Verify the environment variable is set: `echo $KNOWBE4_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  KNOWBE4_API_KEY is missing or is a non-Reporting key. Generate a Reporting API key in KSAT Account Settings -> API and re-export it.
- **403 Forbidden / feature not enabled**  -  The Reporting API requires a Platinum, Diamond, or SAT Foundations/Advanced subscription. Confirm your tier with your KnowBe4 admin.
- **Calls hit the wrong server or return no data**  -  Set KNOWBE4_REGION to match your console host (us, eu, ca, uk, de). The default is us.
- **Transcendence commands return empty results**  -  Run `knowbe4-cli sync` first (with phishing-tests, users or groups in scope): besides the tables it walks every synced phishing test's recipients into the store and takes a risk snapshot, and it fails if every recipient fetch failed. risk-drift and group-risk-contribution compare snapshots, so they need syncs on two different days. `knowbe4-cli freshness` shows what each command has to work with.
- **events commands fail with 401**  -  The User Event API uses a separate key. Export KNOWBE4_USER_EVENT_API_KEY (User Event API Management Console -> API key).

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**leshk0de/KnowBe4ReportingClient**](https://github.com/leshk0de/KnowBe4ReportingClient)  -  Python
- [**joshuadavidthomas/kb4.py**](https://github.com/joshuadavidthomas/kb4.py)  -  Python
- [**demisto/content KnowBe4KMSAT**](https://github.com/demisto/content/tree/master/Packs/KnowBe4KMSAT)  -  Python
- [**compuvin/KnowBe4**](https://github.com/compuvin/KnowBe4)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
