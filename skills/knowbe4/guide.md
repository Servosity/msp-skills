# KnowBe4 CLI

**Every KnowBe4 KMSAT reporting feature, plus a local SQLite store for the questions the console cannot answer.**

knowbe4-cli mirrors the KnowBe4 Reporting API (users, groups, phishing security tests and campaigns, training campaigns and enrollments, policies, and risk-score history) and syncs it all into a local SQLite database. That local store powers transcendence commands no other KnowBe4 tool has: repeat-clickers finds humans who failed multiple phishing tests, untrained-clickers anti-joins phishing failures against training records, risk-drift ranks who deteriorated this quarter, and qbr assembles a full client quarterly review in one shot.

## Install

The recommended path installs both the `knowbe4-cli` binary and the `pp-knowbe4` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install knowbe4
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install knowbe4 --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install knowbe4 --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install knowbe4 --agent claude-code
npx -y @mvanhorn/printing-press-library install knowbe4 --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/knowbe4/cmd/knowbe4-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/knowbe4-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install knowbe4 --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-knowbe4 --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-knowbe4 --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install knowbe4 --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/knowbe4-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `KNOWBE4_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/knowbe4/cmd/knowbe4-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

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

These capabilities aren't available in any other tool for this API.

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


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
knowbe4-cli groups list

# JSON for scripting and agents
knowbe4-cli groups list --json

# Filter to specific fields
knowbe4-cli groups list --json --select id,name,status

# Dry run  -  show the request without sending
knowbe4-cli groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
knowbe4-cli groups list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
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

Config file: `~/.config/knowbe4-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `KNOWBE4_REGION` | endpoint | Yes |  |
| `KNOWBE4_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `knowbe4-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

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
- **Transcendence commands return empty results**  -  Run `knowbe4-cli sync` first  -  repeat-clickers, untrained-clickers, and risk-leaderboard read the local store, including a hand-built per-PST recipient sync.
- **events commands fail with 401**  -  The User Event API uses a separate key. Export KNOWBE4_USER_EVENT_API_KEY (User Event API Management Console -> API key).

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**leshk0de/KnowBe4ReportingClient**](https://github.com/leshk0de/KnowBe4ReportingClient)  -  Python
- [**joshuadavidthomas/kb4.py**](https://github.com/joshuadavidthomas/kb4.py)  -  Python
- [**demisto/content KnowBe4KMSAT**](https://github.com/demisto/content/tree/master/Packs/KnowBe4KMSAT)  -  Python
- [**compuvin/KnowBe4**](https://github.com/compuvin/KnowBe4)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
