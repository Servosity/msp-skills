# Proofpoint TAP CLI

**Every TAP Threat Insight endpoint, plus a local threat store that answers the cross-endpoint questions  -  who is both attacked and clicking, what touched this user  -  inside Proofpoint's punishing daily quotas.**

Existing TAP tools are thin per-endpoint wrappers or one-way SIEM shovels. This CLI syncs clicks, messages, campaigns, VAPs, and clickers into SQLite, then answers joined questions locally  -  incident briefs, flat IOC tables, risk overlaps, per-user timelines  -  without re-spending the 1800-per-day SIEM quota or the 50-per-day campaign-ids quota.

Learn more at [Proofpoint TAP](https://help.proofpoint.com/Threat_Insight_Dashboard/API_Documentation).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `proofpoint-cli` binary and the `pp-proofpoint` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install proofpoint
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install proofpoint --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install proofpoint --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install proofpoint --agent claude-code
npx -y @mvanhorn/printing-press-library install proofpoint --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/proofpoint/cmd/proofpoint-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/proofpoint-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install proofpoint --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-proofpoint --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-proofpoint --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install proofpoint --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/proofpoint-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PROOFPOINT_SERVICE_PRINCIPAL` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/proofpoint/cmd/proofpoint-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "proofpoint": {
      "command": "proofpoint-mcp",
      "env": {
        "PROOFPOINT_SERVICE_PRINCIPAL": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

TAP uses HTTP Basic auth with a service principal and secret. In the TAP Dashboard go to Settings, then Connected Applications, and create a new credential pair. Export PROOFPOINT_SERVICE_PRINCIPAL and PROOFPOINT_API_SECRET in your shell. The url decode command works without credentials; everything else requires them.

## Quick Start

```bash
# Verify the binary, config, and credential wiring before touching the API
proofpoint-cli doctor --dry-run

# Pull the last hour of SIEM events, people, and campaigns into the local store
proofpoint-cli sync --since 1h

# Reconstruct a full day of events by auto-looping the API's 1-hour window limit
proofpoint-cli backfill --since 24h

# Who is both Very Attacked and a top clicker  -  the highest-risk people
proofpoint-cli risk-overlap --window 30 --agent

# Full-text search the synced events offline
proofpoint-cli search "phish" --limit 10

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Quota-aware threat ops
- **`backfill`**  -  Reconstruct up to 7 days of SIEM threat events in one command  -  the CLI auto-loops the API's mandatory 1-hour windows and persists every page locally.

  _Reach for this when an agent needs more than the last hour of click/message events; single API calls cannot exceed a 1-hour window._

  ```bash
  proofpoint-cli backfill --since 48h --agent
  ```
- **`campaign-threats`**  -  Expand one campaign into the threats inside it, enriched with severity and family from the local threat store.

  _Use this to pivot from a campaign to its member threats when quota is tight  -  campaign detail is the one unlimited TAP endpoint._

  ```bash
  proofpoint-cli campaign-threats "campaign-xyz789" --agent
  ```

### Incident response
- **`incident`**  -  Turn a threatId into a single incident brief: severity, actors, malware, techniques, forensic evidence, and every local event that touched it.

  _Use this for alert triage when you have a threatId and need the full picture in one shot instead of three separate calls._

  ```bash
  proofpoint-cli incident "threat-abc123" --agent
  ```
- **`iocs`**  -  Flatten TAP's nested forensic evidence tree into a paste-ready indicator table: hashes, URLs, domains, IPs, files, registry keys, processes.

  _Use this when the goal is indicators for blocking or hunting, not a narrative brief  -  output pipes straight into an EDR or blocklist._

  ```bash
  proofpoint-cli iocs --threat-id "threat-abc123" --csv
  ```

### People risk
- **`risk-overlap`**  -  List the people who are both Very Attacked AND top clickers  -  attack index beside click count  -  your highest-risk humans.

  _The single best list for security-awareness targeting: highly attacked people who also click._

  ```bash
  proofpoint-cli risk-overlap --window 30 --agent
  ```
- **`user`**  -  Everything the local store knows about one person: clicks, threat messages, VAP status, and clicker status in one view.

  _Use this during investigations to answer 'show me every event touching this user' without burning SIEM quota on re-queries._

  ```bash
  proofpoint-cli user "jane.doe@example.com" --agent
  ```

## Recipes


### Morning triage: what got through overnight

```bash
proofpoint-cli backfill --since 12h
```

Backfill loops the 1-hour windows for you; afterwards search and user answer questions offline.

### One-shot incident brief from an alert

```bash
proofpoint-cli incident "threat-abc123" --agent --select summary.severity,summary.malware,iocs
```

Severity, attribution, and evidence for a threatId in a single agent-shaped payload.

### Blocklist-ready indicators for a campaign

```bash
proofpoint-cli iocs --campaign-id "campaign-xyz789" --csv
```

Flat CSV of hashes, URLs, and IPs ready for an EDR import.

### Awareness-training cohort

```bash
proofpoint-cli risk-overlap --window 90 --csv
```

The people who are both heavily attacked and clicking, with both metrics side by side.

### Decode a batch of urldefense links

```bash
proofpoint-cli url --urls "https://urldefense.com/v3/__https://example.com__;!!abc" --agent --select urls.decodedUrl,urls.success
```

Returns the real target URLs; works even without credentials.

## Usage

Run `proofpoint-cli --help` for the full command reference and flag list.

## Commands

### campaign

Campaign intelligence (actors, malware, families, techniques)

- **`proofpoint-cli campaign get`** - Get campaign detail (actors, malware, techniques, members)
- **`proofpoint-cli campaign list-ids`** - Heavily rate limited (50 requests per rolling 24 hours). Prefer the local synced store for repeated queries.

### forensics

Forensic evidence (IOCs) for threats and campaigns

- **`proofpoint-cli forensics`** - Provide exactly one of threatId or campaignId. threatId queries are limited to 50 per rolling 24 hours; campaignId queries to 1800.

### people

Very Attacked People and top clickers

- **`proofpoint-cli people list-top-clickers`** - List users who clicked the most malicious links in a window
- **`proofpoint-cli people list-vap`** - List Very Attacked People (highest attack index) for a window

### siem

Time-windowed threat event feeds (clicks and messages)

- **`proofpoint-cli siem list-all-events`** - Fetch all click and message threat events in the window
- **`proofpoint-cli siem list-clicks-blocked`** - Fetch clicks to malicious URLs that were blocked in the window
- **`proofpoint-cli siem list-clicks-permitted`** - Fetch clicks to malicious URLs that were permitted in the window
- **`proofpoint-cli siem list-issues`** - Events for clicks to malicious URLs permitted and messages delivered containing a known threat within the window  -  the subset that got through and needs response.
- **`proofpoint-cli siem list-messages-blocked`** - Fetch blocked messages that contained a known threat
- **`proofpoint-cli siem list-messages-delivered`** - Fetch delivered messages that contained a known threat

### threat

Per-threat summaries

- **`proofpoint-cli threat <threatId>`** - Get a threat summary (severity, spread, actors, malware, techniques)

### url

Decode TAP-rewritten (urldefense) URLs

- **`proofpoint-cli url`** - Decode TAP-rewritten (urldefense) URLs to their original targets


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
proofpoint-cli campaign get <id>

# JSON for scripting and agents
proofpoint-cli campaign get <id> --json

# Filter to specific fields
proofpoint-cli campaign get <id> --json --select id,name,status

# Dry run  -  show the request without sending
proofpoint-cli campaign get <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
proofpoint-cli campaign get <id> --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
proofpoint-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/proofpoint-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PROOFPOINT_SERVICE_PRINCIPAL` | per_call | Yes |  |
| `PROOFPOINT_API_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `proofpoint-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `proofpoint-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PROOFPOINT_SERVICE_PRINCIPAL`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Service Id / Credentials authentication failed**  -  Re-export PROOFPOINT_SERVICE_PRINCIPAL and PROOFPOINT_API_SECRET; verify the pair in TAP Dashboard Settings under Connected Applications, then run doctor
- **HTTP 429 on siem or campaign commands**  -  You hit a rolling 24h quota (SIEM 1800/day, campaign ids 50/day). Query the local store instead  -  sync once, then use search, sql, user, or risk-overlap offline
- **400 when requesting more than an hour of SIEM data**  -  The API caps each request at a 1-hour window. Use backfill --since 24h to auto-loop windows instead of widening sinceSeconds
- **risk-overlap or user returns nothing**  -  The local store is empty for those tables. Run sync first (people resources sync with window 30 by default)

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**tap-api-cli**](https://github.com/pfptcommunity/tap-api-cli)  -  Python
- [**tap-api-python**](https://github.com/pfptcommunity/tap-api-python)  -  Python
- [**proofpoint_tap**](https://github.com/drizzo-tech/proofpoint_tap)  -  Python
- [**ProofpointTAP**](https://github.com/lambdac0de/ProofpointTAP)  -  PowerShell
- [**proofpoint-mcp**](https://github.com/wyre-technology/proofpoint-mcp)  -  TypeScript
- [**TA-proofpoint_TAP**](https://github.com/wcmc-its/TA-proofpoint_TAP)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
