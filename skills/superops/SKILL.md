---
name: superops
description: "Every SuperOps PSA+RMM entity in your terminal, plus a local SQLite mirror that answers cross-entity questions the web UI can't. Trigger phrases: `list superops tickets`, `which tickets are breaching sla`, `show unbilled worklog in superops`, `assets missing patches with open tickets`, `client 360 in superops`, `use superops`, `run superops`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "SuperOps"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - superops-cli
---

# SuperOps  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `superops-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install superops --cli-only
   ```
2. Verify: `superops-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

SuperOps unifies PSA and RMM on one relational database; this CLI syncs your whole tenant into local SQLite so you can grep, jq, and join across tickets, assets, clients, contracts, and invoices offline. Match every entity the GraphQL API exposes, then transcend with commands like sla-watch, unbilled, at-risk-assets, and alert-coverage that no single SuperOps call answers.

## When to Use This CLI

Use this CLI when you need to query or manage a SuperOps tenant from the terminal or an AI agent: triaging the ticket queue, checking endpoint patch posture, assembling a client 360 before a QBR, reconciling unbilled worklog at month-end, or feeding a triage agent a grounded context pack. It shines for cross-entity questions that span PSA and RMM, which the web UI answers only one entity at a time.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. This printed CLI exposes read-only commands for inspection, export, sync, and analysis.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-entity insight from local state
- **`sla-watch`**  -  See which open tickets are breaching or about to breach SLA, grouped by technician or client.

  _Reach for this to answer 'who is about to miss SLA and on whose queue' in one call instead of five filtered views._

  ```bash
  superops-cli sla-watch --by tech --window 4h --agent
  ```
- **`unbilled`**  -  Find logged worklog time that never landed on an invoice, totaled in dollars per client.

  _Reach for this at month-end to surface revenue leaking out of the billing pipeline._

  ```bash
  superops-cli unbilled --since 2026-05-01 --agent
  ```
- **`at-risk-assets`**  -  List assets missing a critical patch that also have an active (unresolved) alert.

  _Reach for this to prioritize remediation on endpoints that are both vulnerable and actively alerting._

  ```bash
  superops-cli at-risk-assets --client acme --agent
  ```
- **`alert-coverage`**  -  Partition alerts into open (uncovered) vs resolved, grouped by client.

  _Reach for this to catch clients with alerts still sitting unhandled  -  work nobody is tracking._

  ```bash
  superops-cli alert-coverage --client acme --agent
  ```
- **`client-360`**  -  One offline bundle of a client plus its sites, users, contracts, open tickets, assets, and open invoices.

  _Reach for this before a QBR or escalation to load the full client picture in one command._

  ```bash
  superops-cli client-360 <client> --agent
  ```
- **`stale-tickets`**  -  Open tickets with no conversation, note, or worklog activity in N days.

  _Reach for this to catch neglected tickets before they turn into SLA misses or angry clients._

  ```bash
  superops-cli stale-tickets --days 7 --agent
  ```

### Agent-native plumbing
- **`context-ticket`**  -  Assemble a ticket plus its worklogs, client, and SLA into one agent-shaped JSON blob (conversation/notes fetched live).

  _Reach for this as an AI triage agent's single read to ground a decision without six round-trips._

  ```bash
  superops-cli context-ticket 12345 --agent --select ticket.subject,client.name,sla.name
  ```

## Command Reference

**alerts**  -  Manage RMM alerts

- `superops-cli alerts`  -  List alerts

**assets**  -  Manage SuperOps assets and endpoints

- `superops-cli assets <id>`  -  Get an asset by ID
- `superops-cli assets`  -  List assets

**clients**  -  Manage SuperOps clients (accounts)

- `superops-cli clients <id>`  -  Get a client by account ID
- `superops-cli clients`  -  List clients

**contracts**  -  Manage client contracts

- `superops-cli contracts`  -  List client contracts

**invoices**  -  Manage invoices

- `superops-cli invoices <id>`  -  Get an invoice by ID
- `superops-cli invoices`  -  List invoices

**it-docs**  -  Manage IT documentation

- `superops-cli it-docs`  -  List IT documentation

**kb**  -  Manage knowledge base articles

- `superops-cli kb`  -  List knowledge base items

**service-items**  -  Manage service catalog items

- `superops-cli service-items`  -  List service items

**sites**  -  Manage client sites

- `superops-cli sites`  -  List client sites

**tasks**  -  Manage tasks

- `superops-cli tasks <id>`  -  Get a task by ID
- `superops-cli tasks`  -  List tasks

**technicians**  -  Manage technicians

- `superops-cli technicians`  -  List technicians

**tickets**  -  Manage SuperOps tickets

- `superops-cli tickets <id>`  -  Get a ticket by display ID
- `superops-cli tickets`  -  List tickets

**users**  -  Manage client users (contacts)

- `superops-cli users`  -  List client users

**worklogs**  -  Manage worklog time entries

- `superops-cli worklogs`  -  List worklog entries


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
superops-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning SLA triage by technician

```bash
superops-cli sla-watch --by tech --window 4h
```

Groups at-risk tickets per tech so the service desk knows where to push first.

### Month-end revenue leak check

```bash
superops-cli unbilled --agent --select client.name,worklog.minutes,worklog.amount
```

Lists unbilled worklog per client with just the fields billing needs.

### Patch remediation priorities

```bash
superops-cli at-risk-assets --client acme
```

Endpoints that are both missing critical patches and carrying an active (unresolved) alert.

### Agent context for a ticket

```bash
superops-cli context-ticket 12345 --agent --select ticket.subject,ticket.status,client.name,asset.hostName,sla.name
```

Pairs --agent with --select on a deeply nested bundle so an AI agent gets only the fields it needs instead of a multi-KB blob.

### Offline full-text search

```bash
superops-cli search 'disk full' --agent
```

FTS5 over synced tickets, assets, clients, and KB with no live API call.

## Auth Setup

Authenticate with a SuperOps API token (Settings - My Profile - API token) plus your tenant subdomain (Settings - MSP Information). Set SUPEROPS_API_TOKEN, SUPEROPS_SUBDOMAIN, and optionally SUPEROPS_REGION (us or eu). Every GraphQL request sends Authorization: Bearer <token> and the CustomerSubDomain header.

Run `superops-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  superops-cli alerts --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  do not use this CLI for create, update, delete, publish, comment, upvote, invite, order, send, or other mutating requests

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
superops-cli feedback "the --since flag is inclusive but docs say exclusive"
superops-cli feedback --stdin < notes.txt
superops-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/superops-cli/feedback.jsonl`. They are never POSTed unless `SUPEROPS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SUPEROPS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
superops-cli profile save briefing --json
superops-cli --profile briefing alerts
superops-cli profile list --json
superops-cli profile show briefing
superops-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

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

1. **Empty, `help`, or `--help`** → show `superops-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add superops-mcp -- superops-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which superops-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   superops-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `superops-cli <command> --help`.
