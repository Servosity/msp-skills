---
name: proofpoint
description: "Every TAP Threat Insight endpoint, plus a local threat store that answers the cross-endpoint questions  -  who is both attacked and clicking, what touched this user  -  inside Proofpoint's punishing daily quotas. Trigger phrases: `pull proofpoint siem events`, `who are my VAPs`, `decode this urldefense link`, `proofpoint incident brief`, `top clickers this month`, `use proofpoint`, `run proofpoint-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Proofpoint"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - proofpoint-cli
    install:
      - kind: go
        bins: [proofpoint-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/proofpoint/cmd/proofpoint-cli
---

# Proofpoint TAP  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `proofpoint-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install proofpoint --cli-only
   ```
2. Verify: `proofpoint-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/proofpoint/cmd/proofpoint-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Existing TAP tools are thin per-endpoint wrappers or one-way SIEM shovels. This CLI syncs clicks, messages, campaigns, VAPs, and clickers into SQLite, then answers joined questions locally  -  incident briefs, flat IOC tables, risk overlaps, per-user timelines  -  without re-spending the 1800-per-day SIEM quota or the 50-per-day campaign-ids quota.

## When to Use This CLI

Use this CLI when an agent needs Proofpoint TAP threat data: pulling SIEM click/message events, identifying Very Attacked People and top clickers, investigating a threatId or campaign, extracting IOCs for blocking, or decoding urldefense-rewritten links. It shines when questions repeat or cross endpoints  -  the local store answers joins and re-queries without burning daily API quota.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for Proofpoint Essentials administration (orgs, users, licensing)  -  that is a different API with different credentials
- Do not use this CLI to stream PoD message logs  -  the PoD Log API is a WebSocket service not covered here
- Do not use this CLI to release or manage quarantined messages  -  quarantine actions live in the PPS admin interface, not the TAP API
- Do not use this CLI to send or simulate phishing tests  -  TAP is read-only threat intelligence

## Unique Capabilities

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

## Command Reference

**campaign**  -  Campaign intelligence (actors, malware, families, techniques)

- `proofpoint-cli campaign get`  -  Get campaign detail (actors, malware, techniques, members)
- `proofpoint-cli campaign list-ids`  -  Heavily rate limited (50 requests per rolling 24 hours). Prefer the local synced store for repeated queries.

**forensics**  -  Forensic evidence (IOCs) for threats and campaigns

- `proofpoint-cli forensics`  -  Provide exactly one of threatId or campaignId.

**people**  -  Very Attacked People and top clickers

- `proofpoint-cli people list-top-clickers`  -  List users who clicked the most malicious links in a window
- `proofpoint-cli people list-vap`  -  List Very Attacked People (highest attack index) for a window

**siem**  -  Time-windowed threat event feeds (clicks and messages)

- `proofpoint-cli siem list-all-events`  -  Fetch all click and message threat events in the window
- `proofpoint-cli siem list-clicks-blocked`  -  Fetch clicks to malicious URLs that were blocked in the window
- `proofpoint-cli siem list-clicks-permitted`  -  Fetch clicks to malicious URLs that were permitted in the window
- `proofpoint-cli siem list-issues`  -  Events for clicks to malicious URLs permitted and messages delivered containing a known threat within the window  -  the
- `proofpoint-cli siem list-messages-blocked`  -  Fetch blocked messages that contained a known threat
- `proofpoint-cli siem list-messages-delivered`  -  Fetch delivered messages that contained a known threat

**threat**  -  Per-threat summaries

- `proofpoint-cli threat <threatId>`  -  Get a threat summary (severity, spread, actors, malware, techniques)

**url**  -  Decode TAP-rewritten (urldefense) URLs

- `proofpoint-cli url`  -  Decode TAP-rewritten (urldefense) URLs to their original targets


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
proofpoint-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

TAP uses HTTP Basic auth with a service principal and secret. In the TAP Dashboard go to Settings, then Connected Applications, and create a new credential pair. Export PROOFPOINT_SERVICE_PRINCIPAL and PROOFPOINT_API_SECRET in your shell. The url decode command works without credentials; everything else requires them.

Run `proofpoint-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  proofpoint-cli campaign get <id> --agent --select id,name,status
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
proofpoint-cli feedback "the --since flag is inclusive but docs say exclusive"
proofpoint-cli feedback --stdin < notes.txt
proofpoint-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/proofpoint-cli/feedback.jsonl`. They are never POSTed unless `PROOFPOINT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PROOFPOINT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
proofpoint-cli profile save briefing --json
proofpoint-cli --profile briefing campaign get <id>
proofpoint-cli profile list --json
proofpoint-cli profile show briefing
proofpoint-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `proofpoint-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/proofpoint/cmd/proofpoint-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add proofpoint-mcp -- proofpoint-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which proofpoint-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   proofpoint-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `proofpoint-cli <command> --help`.
