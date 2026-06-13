---
name: itglue
description: "Every IT Glue resource, plus an offline SQLite mirror, fleet-wide cross-resource search, and documentation-hygiene analytics no other IT Glue tool offers. Trigger phrases: `find this device across all my IT Glue clients`, `which IT Glue organizations are under-documented`, `audit stale passwords in IT Glue`, `sync IT Glue to a local database`, `what changed in IT Glue since last week`, `use itglue`, `run itglue`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "IT Glue"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - itglue-cli
    install:
      - kind: go
        bins: [itglue-cli]
        module: github.com/mvanhorn/printing-press-library/library/productivity/itglue/cmd/itglue-cli
---

# IT Glue  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `itglue-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install itglue --cli-only
   ```
2. Verify: `itglue-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/productivity/itglue/cmd/itglue-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

IT Glue's API is a thin per-call JSON:API surface guarded by a 3000-requests/5-minute rate ceiling, and every existing tool just wraps one endpoint at a time. This CLI mirrors your organizations, contacts, passwords, configurations, and documents into a local SQLite store, then answers the questions the live API can't: `search` finds anything across every client at once, `coverage` ranks clients by documentation completeness, and `passwords stale` audits credential rotation fleet-wide without burning your rate budget.

## When to Use This CLI

Reach for this CLI when an agent or technician needs to answer relational, fleet-wide questions about an MSP's IT Glue documentation  -  finding a device or contact across all clients, auditing credential rotation or documentation completeness, or assembling a one-shot client brief  -  without hammering IT Glue's rate-limited API. It is the right tool for read-heavy investigation and hygiene reporting backed by a local mirror, and for non-destructive create/update of contacts, passwords, configurations, and documents (it never deletes).

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet-wide answers from the local mirror
- **`search`**  -  Search every synced organization, contact, password (metadata), configuration, and document at once, with the owning client shown for each hit.

  _When a ticket only gives you a serial number, device name, or contact, reach for this to find which client it belongs to without paging the live API across every tenant._

  ```bash
  itglue-cli search "Fortinet" --agent
  ```
- **`changes`**  -  Show every record updated since a timestamp across all five resources, newest first, optionally narrowed to one resource type.

  _Use for a weekly remediation sweep to see what changed across the fleet since your last review._

  ```bash
  itglue-cli changes --since 2026-05-01 --agent
  ```

### Documentation & credential hygiene
- **`coverage`**  -  Rank organizations by documentation completeness  -  counts of configurations, contacts, passwords, and documents per client, thinnest first.

  _Use before a QBR or onboarding review to surface clients with missing runbooks, contacts, or credentials before they complain._

  ```bash
  itglue-cli coverage --below 1 --agent
  ```
- **`passwords stale`**  -  List credentials whose last update is older than a threshold, grouped by client and sorted oldest-first  -  metadata only, the secret value is never read.

  _Use for SOC2 / cyber-insurance credential-hygiene attestation and to generate rotation tickets across all clients in one pass._

  ```bash
  itglue-cli passwords stale --days 365 --agent
  ```
- **`contacts dupes`**  -  Find contacts that share a normalized name or email within (or across) an organization  -  the duplicates that overlapping PSA syncs leave behind.

  _Run after a PSA contact sync to clean up duplicate people before they pollute client documentation._

  ```bash
  itglue-cli contacts dupes --org 12345 --agent
  ```
- **`orphans`**  -  Find configurations, contacts, passwords, and documents whose organization-id no longer resolves to a synced organization.

  _Run after client offboarding or org renames to catch documentation left dangling before it misleads a technician._

  ```bash
  itglue-cli orphans --agent
  ```

### One-call relational views
- **`org show`**  -  Assemble everything known about one client  -  contacts, configurations, password metadata, and documents  -  in a single offline read.

  _Use at the start of a client call or ticket to pull the full picture instantly instead of clicking through the web UI._

  ```bash
  itglue-cli org show 12345 --agent
  ```

## Command Reference

**configurations**  -  Manage configurations

- `itglue-cli configurations create`  -  Create a configuration
- `itglue-cli configurations get`  -  Retrieve a configuration
- `itglue-cli configurations list`  -  Use stable fields such as organization id, name, hostname, serial number, or asset tag before creating records.
- `itglue-cli configurations update`  -  Update a configuration

**contacts**  -  Manage contacts

- `itglue-cli contacts create`  -  Idempotent callers should first search by organization id and email address.
- `itglue-cli contacts get`  -  Retrieve a contact
- `itglue-cli contacts list`  -  Use filter[email] or filter[organization_id] before creating contacts to avoid duplicates.
- `itglue-cli contacts update`  -  Use PATCH for contact deactivation; this spec intentionally does not expose DELETE.

**documents**  -  Manage documents

- `itglue-cli documents create`  -  Create a document
- `itglue-cli documents get`  -  Retrieve a document
- `itglue-cli documents list`  -  List documents
- `itglue-cli documents update`  -  Update a document

**organizations**  -  Manage organizations

- `itglue-cli organizations get`  -  Retrieve one organization
- `itglue-cli organizations list`  -  List organizations

**passwords**  -  Manage passwords

- `itglue-cli passwords create`  -  Creates either a general password or an embedded password when resource-id and resource-type are supplied.
- `itglue-cli passwords get`  -  Retrieve a password metadata record
- `itglue-cli passwords list`  -  Use filter[name] plus filter[organization_id] before creating password mirror records.
- `itglue-cli passwords update`  -  Update a password


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
itglue-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Mirror then audit credential hygiene

```bash
itglue-cli sync --full && itglue-cli passwords stale --days 180 --agent
```

Sync once, then list every password not rotated in six months across all clients without touching the live API again.

### Narrow a verbose configuration list for an agent

```bash
itglue-cli configurations list --agent --select name,serial-number,primary-ip,configuration-type-name
```

Configuration records carry dozens of attributes; --select with --agent returns only the fields an agent needs, keeping the payload small.

### Find which client a device belongs to

```bash
itglue-cli search "SN-ABC123" --agent
```

One FTS query maps a serial number to its owning organization and resource, no per-org clicking.

### Pre-QBR documentation sweep

```bash
itglue-cli coverage --below 1 --agent
```

List clients missing an entire documentation category so you can assign remediation before the review.

### Preview a write before it lands

```bash
itglue-cli contacts create --data-attributes-organization-id 12345 --data-attributes-first-name Jane --data-attributes-last-name Doe --dry-run
```

Print the exact JSON:API payload that would be sent without creating anything.

## Auth Setup

Authentication is a single IT Glue API key sent as the `x-api-key` header. Set `ITGLUE_API_KEY` in your environment, then pick your data region by setting `ITGLUE_BASE_URL` (or `base_url` in `~/.config/itglue-cli/config.toml`) to US `https://api.itglue.com`, EU `https://api.eu.itglue.com`, or AU `https://api.au.itglue.com`. Generate the key in IT Glue account settings; it needs read (and, for create/update commands, write) permissions. Verify with `doctor`.

Run `itglue-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  itglue-cli configurations list --agent --select id,name,status
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
itglue-cli feedback "the --since flag is inclusive but docs say exclusive"
itglue-cli feedback --stdin < notes.txt
itglue-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/itglue-cli/feedback.jsonl`. They are never POSTed unless `ITGLUE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ITGLUE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
itglue-cli profile save briefing --json
itglue-cli --profile briefing configurations list
itglue-cli profile list --json
itglue-cli profile show briefing
itglue-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `itglue-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/productivity/itglue/cmd/itglue-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add itglue-mcp -- itglue-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which itglue-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   itglue-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `itglue-cli <command> --help`.
