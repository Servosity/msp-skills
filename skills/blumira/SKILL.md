---
name: blumira
description: "Every Blumira finding, detection, and agent across your direct org and every MSP sub-account  -  in one offline-searchable store with cross-account triage and over-time trends no single API call can answer. Trigger phrases: `blumira findings`, `triage blumira across accounts`, `what changed in blumira since yesterday`, `blumira detection coverage`, `blumira domain controller exposure`, `blumira MTTR report`, `use blumira`, `run blumira`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Blumira"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - blumira-cli
    install:
      - kind: go
        bins: [blumira-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/blumira/cmd/blumira-cli
---

# Blumira  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `blumira-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install blumira --cli-only
   ```
2. Verify: `blumira-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/blumira/cmd/blumira-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Blumira's API and the community MCP return per-account, point-in-time snapshots. This CLI syncs findings, evidence, detection rules, and agent devices from your org and every MSP client account into a local SQLite store, so you get one ranked cross-account triage queue (triage), what-changed-since-last-sync (drift), MTTR and velocity trends (velocity), SLA aging (sla), detection coverage drift vs the MSP basis (coverage), and domain-controller exposure (exposure). It also mints and auto-refreshes its own JWT from your Client ID and Secret instead of making you bring your own.

## When to Use This CLI

Use this CLI when an agent or analyst needs Blumira security data across more than one client account or across time  -  cross-account triage, what-changed diffs, MTTR/velocity reporting, SLA aging, detection-coverage drift, or domain-controller exposure. Prefer it over raw API calls whenever the question spans accounts (MSP rollup) or spans syncs (trends, drift, re-fire), since those answers do not exist in any single Blumira endpoint.

## Anti-triggers

Do not use this CLI for:
- Raw log or event search (the API exposes findings/rules/agents, not the log stream - use the Blumira console)
- Agent installation or sensor deployment (agents commands are read-only inventory)
- Non-Blumira SIEM/EDR data (use the matching vendor's connector)
- Modifying detection rules (public API is read-only for rules - tune in the Blumira console)

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-account & over-time intelligence
- **`triage`**  -  One globally-ranked open-findings queue across every MSP client account.

  _Reach for this to start MSP triage: it answers 'what is on fire across all my clients right now' in one ranked list instead of one account at a time._

  ```bash
  blumira-cli triage --priority high --status open --agent
  ```
- **`drift`**  -  New, status-changed, and newly-resolved findings since the last sync, per account.

  _The shift-handoff / morning-standup answer  -  pick this when the user asks what is new or changed, not the full current state._

  ```bash
  blumira-cli drift --since 24h --agent
  ```
- **`velocity`**  -  Mean-time-to-resolve and open-rate per account or overall.

  _Use for reporting and trend tracking  -  it is the only way to get response-time trends Blumira's API does not expose._

  ```bash
  blumira-cli velocity --window 30d --by account --agent
  ```
- **`sla`**  -  Findings about to breach an age-based SLA across all accounts, ranked.

  _Pick this to catch findings slipping past SLA before they breach, instead of discovering breaches after the fact._

  ```bash
  blumira-cli sla --breach-in 4h --priority high --agent
  ```
- **`coverage`**  -  Detection rules missing or disabled in your org versus the MSP basis ruleset.

  _Use to find detection rules that silently drifted away from the MSP basis baseline._

  ```bash
  blumira-cli coverage --against basis --agent
  ```
- **`exposure`**  -  Cross-account agent rollup that flags stale or unprotected domain controllers.

  _Reach for this for billing/coverage reviews and to spot unprotected domain controllers across the whole MSP fleet._

  ```bash
  blumira-cli exposure --flag-dc-stale --agent
  ```
- **`audit`**  -  Findings that were resolved and then re-fired (reopened).

  _Use to catch premature or low-quality resolutions that re-fired._

  ```bash
  blumira-cli audit --min-reopens 2 --agent
  ```
- **`recurring`**  -  The same detection firing repeatedly across accounts and time.

  _Pick this to find the noisy detection or unfixed root cause that keeps generating findings._

  ```bash
  blumira-cli recurring --window 90d --min-count 3 --agent
  ```
- **`overview`**  -  One-screen per-account rollup: open findings by priority, agent coverage, and detection-coverage drift across every client.

  _Run this first each morning: it answers 'which client needs attention' before triage answers 'which finding'._

  ```bash
  blumira-cli overview --agent
  ```
- **`reconcile`**  -  Flat finding-to-org-to-assignee-to-status table for diffing against ConnectWise, Jira, or Zendesk.

  _Use when reconciling Blumira findings against an external ticketing system - one command replaces the manual cross-account export._

  ```bash
  blumira-cli reconcile --status open --csv
  ```
- **`evidence-search`**  -  Full-text search over synced finding evidence to find which findings mention an IOC, hostname, or user.

  _Reach for this during threat hunting when you have an indicator but not a finding ID._

  ```bash
  blumira-cli evidence-search "rdp brute force" --agent
  ```
- **`dc-roster`**  -  Every domain controller across all accounts with last check-in and protected/stale state.

  _Use for fleet and billing reviews needing the full DC inventory; use exposure instead for the prioritized stale/unprotected action list._

  ```bash
  blumira-cli dc-roster --agent
  ```
- **`workload`**  -  Open assigned findings grouped by owner across all accounts, with age buckets.

  _Use when balancing analyst assignments or reviewing team load before sprint or shift planning._

  ```bash
  blumira-cli workload --agent
  ```

### Access that beats the incumbent
- **`auth login`**  -  Mints, caches, and auto-refreshes the Blumira JWT from a Client ID + Secret.

  _Use once to authenticate; every other command then just works without manually exchanging or refreshing tokens._

  ```bash
  blumira-cli auth login --client-id "$BLUMIRA_CLIENT_ID" --client-secret "$BLUMIRA_CLIENT_SECRET"
  ```

## Command Reference

**health**  -  Manage health

- `blumira-cli health`  -  Get API health

**msp**  -  Manage msp

- `blumira-cli msp add-account-finding-comment`  -  Add a comment to a finding for your MSP sub-account
- `blumira-cli msp get`  -  Get a specific MSP sub-account by ID
- `blumira-cli msp get-account-agents-devices`  -  Get or search agents devices for your MSP sub-account
- `blumira-cli msp get-account-agents-keys`  -  Get or search agents keys for your MSP sub-account
- `blumira-cli msp get-account-finding`  -  Get a specific finding by ID for your MSP sub-account
- `blumira-cli msp get-account-finding-comments`  -  Get comments for a specific finding for your MSP sub-account
- `blumira-cli msp get-account-finding-evidence`  -  Returns evidence keys and a paginated first page of evidence rows for the finding in one call.
- `blumira-cli msp get-account-findings`  -  Get or search findings for your MSP sub-account
- `blumira-cli msp get-accounts`  -  Get or search sub-accounts for your MSP
- `blumira-cli msp get-accounts-findings`  -  Get or search findings for all your MSP sub-accounts
- `blumira-cli msp get-agents-device`  -  Get a specific agents device by ID for your MSP sub-account
- `blumira-cli msp get-agents-key`  -  Get a specific agents key by ID for your MSP sub-account
- `blumira-cli msp get-detection-rule-by-account`  -  Get a detection rule by ID for your MSP sub-account
- `blumira-cli msp get-detection-rules-by-account`  -  List detection rules for your MSP sub-account
- `blumira-cli msp get-msp-detection-rule`  -  Get a basis detection rule by ID (for MSP bulk detail)
- `blumira-cli msp get-msp-detection-rules`  -  List basis detection rules (catalog) for MSP bulk
- `blumira-cli msp list-users`  -  Get or search users for your MSP sub-account
- `blumira-cli msp resolve-finding`  -  Resolve a finding for your MSP sub-account
- `blumira-cli msp set-finding-owners`  -  Assign owners to a finding for your MSP sub-account

**org**  -  Manage org

- `blumira-cli org controller-direct-add-comment`  -  Add a comment to a finding in the org
- `blumira-cli org controller-direct-get`  -  Get a specific finding by ID
- `blumira-cli org controller-direct-get-agents-device`  -  Get a specific agents device by ID
- `blumira-cli org controller-direct-get-agents-devices`  -  Get or search agents devices
- `blumira-cli org controller-direct-get-agents-key`  -  Get a specific agents key by ID
- `blumira-cli org controller-direct-get-agents-keys`  -  Get or search agents keys
- `blumira-cli org controller-direct-get-by`  -  Get or search findings for your organization
- `blumira-cli org controller-direct-get-comments`  -  Get comments for a specific finding
- `blumira-cli org controller-direct-get-details`  -  Get details for a specific finding
- `blumira-cli org controller-direct-get-detection-rule`  -  Get a detection rule by ID
- `blumira-cli org controller-direct-get-detection-rules-by`  -  List detection rules for your organization
- `blumira-cli org controller-direct-get-evidence`  -  Returns evidence keys (schema) and a paginated first page of evidence rows for the finding in one call.
- `blumira-cli org controller-direct-list-users`  -  Get or search users for your organization
- `blumira-cli org controller-direct-resolve-finding`  -  Resolve a finding
- `blumira-cli org controller-direct-set-owners`  -  Assign owners to a finding

**resolutions**  -  Manage resolutions

- `blumira-cli resolutions`  -  Get resolution options for findings


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
blumira-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning MSP triage

```bash
blumira-cli sync --full && blumira-cli triage --priority high --status open --json
```

Sync every account, then pull the ranked high-priority open queue across all clients as JSON for a dashboard or agent.

### Shift-handoff drift report

```bash
blumira-cli drift --since 12h --json --select account,name,change
```

Show only what changed since the last sync with a narrow field set so an agent does not parse full finding payloads.

### Monthly velocity by account

```bash
blumira-cli velocity --window 30d --by account --csv
```

Export per-account MTTR and open-rate for a 30-day report.

### Find unprotected domain controllers

```bash
blumira-cli exposure --flag-dc-stale --json
```

List domain controllers across all accounts whose agent check-in is stale or missing.

### Hunt an indicator across every client's evidence

```bash
blumira-cli evidence-search "rdp brute force" --agent --select matches.finding_id,matches.name
```

Greps the locally cached evidence corpus plus synced finding payloads for an IOC/hostname/term and returns just the fields an agent needs  -  impossible upstream, where evidence is only retrievable per known finding ID. Pass --fetch to populate the evidence cache live first (bounded by --max-fetch).

## Auth Setup

Blumira uses OAuth2 client-credentials. Generate a Client ID + Client Secret in Settings > Organization > Generate API Credentials, then run `auth login --client-id <id> --client-secret <secret>`  -  the CLI exchanges them at https://auth.blumira.com/oauth/token (audience=public-api) for a ~30-day JWT, caches it, and refreshes it automatically. You can also export a pre-minted token as BLUMIRA_API_TOKEN. Keys are read-write by default; scope them read-only in the Blumira UI for safe read access. MSP tenants address sub-orgs by account_id; nothing is baked in.

Run `blumira-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  blumira-cli health --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
blumira-cli feedback "the --since flag is inclusive but docs say exclusive"
blumira-cli feedback --stdin < notes.txt
blumira-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/blumira-cli/feedback.jsonl`. They are never POSTed unless `BLUMIRA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BLUMIRA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
blumira-cli profile save briefing --json
blumira-cli --profile briefing health
blumira-cli profile list --json
blumira-cli profile show briefing
blumira-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `blumira-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/blumira/cmd/blumira-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add blumira-mcp -- blumira-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which blumira-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   blumira-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `blumira-cli <command> --help`.
