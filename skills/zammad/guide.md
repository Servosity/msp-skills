# Zammad CLI

**Every Zammad ticket, article, and Knowledge Base operation as one CLI and MCP server  -  plus a team-management layer (agent load, customer health, aging backlog, escalation triage, churn risk, feedback mining) the Zammad API can't answer in a single call.**

A single Go binary for the whole Zammad REST + Knowledge Base surface with an offline SQLite store, structured `--json`/`--select` output, and `--dry-run` on every write. On top of parity it adds a team-management layer computed from your synced tickets: `agent-load` and `agent-trend` for workload, `customer-health` and `churn-risk` for account risk, `overdue` for aging backlog, `escalate` for sentiment triage, and `feedback` for mining what customers ask for. Works against any Zammad instance  -  set your instance URL and token.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `zammad-cli` and `zammad-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zammad/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zammad/install.ps1 | iex
   ```
3. Verify: `zammad-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=zammad). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/zammad --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/zammad --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `zammad-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the zammad skill from https://github.com/Servosity/msp-skills/tree/main/skills/zammad. The skill defines how its required CLI (`zammad-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=zammad).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ZAMMAD_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

> **Interim note:** check any `.mcpb` bundle before you trust it ([#287](https://github.com/Servosity/msp-skills/issues/287)). Its `manifest.json` launches `${__dirname}/bin/zammad-pp-mcp`, while the builder stores the release binaries in `bin/` under their platform-suffixed names - `zammad-mcp-darwin-arm64`, `-darwin-amd64`, `-linux-arm64`, `-linux-amd64`, `-windows-amd64.exe`. Run `unzip -l <file>.mcpb | grep bin/`: if the name the manifest launches is not among them, Claude Desktop has nothing to run - use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zammad/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zammad/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "zammad": {
      "command": "zammad-mcp",
      "env": {
        "ZAMMAD_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Zammad uses a personal access token sent as `Authorization: Token token=<TOKEN>`. Create one in your Zammad instance under Profile → Token Access, then set `ZAMMAD_API_TOKEN`. Point the CLI at your instance with `ZAMMAD_URL` (e.g. https://support.example.com). The token's permissions cap what the CLI can do; a token without write scopes keeps every command read-only regardless of flags.

## Quick Start

```bash
# Health check  -  confirms instance URL and token are wired before any live call.
zammad-cli doctor --dry-run

# Confirm auth by fetching the authenticated agent.
zammad-cli users me --json

# Pull tickets, organizations, users, and groups into the local store.
zammad-cli sync --json

# Search open tickets with Zammad's query syntax.
zammad-cli tickets search --query "state:open" --json

# See current ticket load per agent.
zammad-cli agent-load --json

# Find tickets open too long.
zammad-cli overdue --days 3 --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Team management
- **`agent-load`**  -  See each agent's current ticket load broken out by state (open/pending/backlog) so you can balance work before it piles up.

  _Reach for this when asked who is overloaded or how work is distributed across the support team right now._

  ```bash
  zammad-cli agent-load --json
  ```
- **`agent-trend`**  -  Show whether each agent's queue is growing or shrinking across the last N weeks, with opened-vs-closed deltas.

  _Reach for this when asked if the team is keeping up or falling behind over time._

  ```bash
  zammad-cli agent-trend --weeks 2 --json
  ```
- **`customer-health`**  -  Rank organizations by a health signal built from open count, oldest-open age, reopen rate, velocity, and last activity.

  _Reach for this to spot which customers are struggling before they complain._

  ```bash
  zammad-cli customer-health --at-risk --json
  ```
- **`overdue`**  -  Find aging tickets still in new/open/pending past a threshold, weighted by priority so the worst rise to the top.

  _Reach for this to catch tickets about to breach SLA or go stale._

  ```bash
  zammad-cli overdue --days 3 --json
  ```
- **`escalate`**  -  Surface active tickets whose recent customer messages read as upset (heuristic negative-signal scan), ranked by hits, age, and priority  -  with the matched snippets shown.

  _Reach for this to find customers who sound angry and should be escalated; it is keyword triage with snippets, so you make the final call._

  ```bash
  zammad-cli escalate --json
  ```
- **`churn-risk`**  -  Score each organization's churn risk from open backlog, overdue tickets, unanswered pending, and negative-sentiment customer messages, listing the contributing factors.

  _Reach for this to get ahead of churn; it surfaces the factors so you can judge and act._

  ```bash
  zammad-cli churn-risk --json
  ```
- **`feedback-scan`**  -  Bucket ticket and article text into feature-request, pricing, compliance, and bug themes with the source ticket refs and snippets.

  _Reach for this to mine what customers keep asking for across features, pricing, and compliance._

  ```bash
  zammad-cli feedback-scan --bucket pricing --json
  ```

### Knowledge Base
- **`kb browse`**  -  Print the Knowledge Base as a category/answer tree parsed from the init bundle.

  _Reach for this to see the KB structure before reading or searching answers._

  ```bash
  zammad-cli kb browse
  ```
- **`kb search`**  -  Text search over KB answer titles and bodies from the live KB init bundle.

  _Reach for this to find the right KB answer by keyword._

  ```bash
  zammad-cli kb search "restore" --json
  ```
- **`kb get`**  -  Resolve a KB answer id from the init bundle to its full translated body.

  _Reach for this to read a KB answer's full content._

  ```bash
  zammad-cli kb get 42
  ```

### Ticket ergonomics
- **`ticket note`**  -  Add an internal or partner-visible note to a ticket in one line, with correct content-type defaults.

  _Reach for this to log work on a ticket without hand-building the article payload._

  ```bash
  zammad-cli ticket note 12345 --body "Investigated, awaiting logs" --internal --dry-run
  ```

## Recipes

### Balance the queue at standup

```bash
zammad-cli agent-load --json --select agent,open,pending,backlog
```

One row per agent with open, pending, and total backlog counts.

### Find at-risk accounts

```bash
zammad-cli customer-health --at-risk --json --select organization,open,oldest_open_days,last_activity_days,signal
```

Organizations flagged at-risk with the health factors that triggered the flag, using --select to keep the payload small on a deeply aggregated response.

### Triage who to escalate

```bash
zammad-cli escalate --json
```

Ranked tickets with matched negative-signal snippets so you judge true sentiment.

### Mine pricing feedback

```bash
zammad-cli feedback-scan --bucket pricing --json
```

Tickets and article snippets bucketed as pricing feedback with source refs.

### Read the KB before answering

```bash
zammad-cli kb search "backup restore" --json
```

Keyword search over Knowledge Base answers (from the live KB init bundle).

## Usage

Run `zammad-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ZAMMAD_CONFIG_DIR`, `ZAMMAD_DATA_DIR`, `ZAMMAD_STATE_DIR`, or `ZAMMAD_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ZAMMAD_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ZAMMAD_HOME=/srv/zammad
zammad-cli doctor
```

Under `ZAMMAD_HOME=/srv/zammad`, the four dirs resolve to `/srv/zammad/config`, `/srv/zammad/data`, `/srv/zammad/state`, and `/srv/zammad/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "zammad": {
      "command": "zammad-mcp",
      "env": {
        "ZAMMAD_HOME": "/srv/zammad"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `ZAMMAD_DATA_DIR` overrides an explicit `--home` for that kind. Use `ZAMMAD_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ZAMMAD_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `zammad-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### articles

Read and add ticket articles (notes and emails)

- **`zammad-cli articles by-ticket`** - List all articles for a ticket
- **`zammad-cli articles create`** - Add an article (note or email) to a ticket
- **`zammad-cli articles get`** - Get a single article by id

### groups

Ticket routing groups

- **`zammad-cli groups get`** - Get a group by id
- **`zammad-cli groups list`** - List all groups

### kb

Knowledge Base categories and answers

- **`zammad-cli kb answer-create`** - Create a KB answer. Pass translations_attributes as JSON, then publish or mark internal.
- **`zammad-cli kb answer-delete`** - Delete a KB answer (requires KB editor permission)
- **`zammad-cli kb answer-internal`** - Mark a KB answer internal (agent-only)
- **`zammad-cli kb answer-publish`** - Publish a KB answer (make it public)
- **`zammad-cli kb category-create`** - Create a KB category. Pass translations_attributes as JSON.
- **`zammad-cli kb init`** - Fetch the full Knowledge Base bundle (categories, answers, translations). Used by 'kb browse/search/get'.

### organizations

Read and manage organizations (customers / partners)

- **`zammad-cli organizations create`** - Create an organization
- **`zammad-cli organizations get`** - Get an organization by id
- **`zammad-cli organizations list`** - List organizations (paginated)
- **`zammad-cli organizations search`** - Search organizations by name or custom field
- **`zammad-cli organizations update`** - Update an organization

### overviews

Agent ticket overviews (saved views)

- **`zammad-cli overviews get`** - Get an overview (and its tickets) by id
- **`zammad-cli overviews list`** - List all overviews

### priorities

Ticket priority reference (id -> name map)

- **`zammad-cli priorities`** - List ticket priorities

### states

Ticket state reference (id -> name map)

- **`zammad-cli states`** - List ticket states

### tags

Ticket tags

- **`zammad-cli tags add`** - Add a tag to an object
- **`zammad-cli tags all`** - List every tag defined in the instance (vocabulary)
- **`zammad-cli tags list`** - List tags on an object (defaults to a ticket)
- **`zammad-cli tags remove`** - Remove a tag from an object

### tickets

Read, search, create, and change Zammad tickets

- **`zammad-cli tickets create`** - Create a ticket. Provide title, group, a customer, and an initial article.
- **`zammad-cli tickets delete`** - Delete a ticket permanently (requires admin / delete permission)
- **`zammad-cli tickets get`** - Get a ticket by id
- **`zammad-cli tickets list`** - List tickets (paginated)
- **`zammad-cli tickets search`** - Search tickets with Zammad query syntax (e.g. 'state:open owner_id:3')
- **`zammad-cli tickets update`** - Update a ticket (state, priority, owner, title, organization)

### users

Read and manage users (agents and customers)

- **`zammad-cli users create`** - Create a user
- **`zammad-cli users get`** - Get a user by id
- **`zammad-cli users list`** - List users (paginated)
- **`zammad-cli users me`** - Get the authenticated user (auth self-check)
- **`zammad-cli users search`** - Search users by email, name, or query (e.g. organization_id:5)
- **`zammad-cli users update`** - Update a user


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`zammad-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`zammad-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`zammad-cli learnings list`** - Inspect taught rows
- **`zammad-cli learnings forget <query>`** - Undo a teach
- **`zammad-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`zammad-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`zammad-cli teach-pattern`** - Install a query/resource template up front
- **`zammad-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ZAMMAD_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `zammad-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
zammad-cli articles get 12345

# JSON for scripting and agents
zammad-cli articles get 12345 --json

# Filter to specific fields
zammad-cli articles get 12345 --json --select id,name,status

# Dry run  -  show the request without sending
zammad-cli articles get 12345 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
zammad-cli articles get 12345 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
zammad-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `zammad-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/zammad-cli/config.toml`; `--home`, `ZAMMAD_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ZAMMAD_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `zammad-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `zammad-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ZAMMAD_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **403 Authentication required**  -  Set ZAMMAD_API_TOKEN to a valid Zammad access token (Profile → Token Access).
- **Requests hit the wrong host or return HTML**  -  Set ZAMMAD_URL to your instance base (no trailing slash), e.g. https://support.example.com.
- **tickets search returns ids instead of full objects**  -  The CLI passes expand=true by default; if you disabled it, re-add --expand.
- **Team-management commands look empty or stale**  -  Run `zammad-cli sync` first; escalate/feedback also read article bodies, so use `sync --articles` for full coverage.
- **403 on create/update/delete**  -  Your token lacks write permission; that is expected for a read-only token.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**zammad-api-client-ruby**](https://github.com/zammad/zammad-api-client-ruby)  -  Ruby
- [**zammad_py**](https://github.com/joeirimpan/zammad_py)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
