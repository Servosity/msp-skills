---
name: afi
description: "The first CLI for Afi SaaS backup  -  full public-API coverage plus the fleet-wide coverage, staleness, and offboarding answers the rate-limited API can't serve live. Trigger phrases: `check afi backup coverage`, `which mailboxes aren't backed up in afi`, `afi fleet health`, `offboard a user from afi backup`, `afi backup stale check`, `use afi`, `run afi-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Afi"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - afi-cli
    install:
      - kind: go
        bins: [afi-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/afi/cmd/afi-cli
---

# Afi  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `afi-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install afi --cli-only
   ```
2. Verify: `afi-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/afi/cmd/afi-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Afi has no other CLI, SDK, or MCP server  -  the only alternative is the web portal, and the vendor explicitly discourages API polling. This CLI walks the whole hierarchy into local SQLite once (fleet-sync), then answers fleet-wide questions (coverage-gaps, fleet-health, backup-stale, reconcile-licenses) offline. A guarded offboard command runs the vendor's own archive-then-release sequence with a verification gate the portal lacks.

## When to Use This CLI

Use this CLI for any programmatic question about Afi SaaS backup fleets: backup coverage, nightly task health, quota breaches, archive staleness, license reconciliation, and the guarded archive-and-offboard sequence for departing employees. It is the right choice for MSP-scale questions across many tenants, where the portal forces a per-tenant walk and the rate-limited API punishes polling  -  sync once, then query the local store.

## Anti-triggers

Do not use this CLI for:
- Performing restores, exports, or browsing actual backup data  -  the Afi public API does not expose restore/export initiation or data download; use the Afi web portal
- Creating or editing backup policies  -  policies are read-only in the public API; configure them in the portal
- Real-time monitoring or continuous polling of task status  -  Afi explicitly throttles and may suspend polling applications; use scheduled syncs instead
- Managing Afi portal user accounts, SSO, or notification settings  -  not part of the public API surface

## Unique Capabilities

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
- **`backup-stale`**  -  Flag protected resources whose most recent backup archive is older than a threshold  -  protected-but-silently-failing resources.

  _Use this to catch the silent-failure class: resources that have a policy attached but whose backups quietly stopped landing._

  ```bash
  afi-cli backup-stale --max-age 48h --agent
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

## Command Reference

**applications**  -  Manage applications

- `afi-cli applications`  -  Lists installations for the application identified by the authentication key.

**orgs**  -  Manage orgs

- `afi-cli orgs create`  -  Creates a new child organization.
- `afi-cli orgs get`  -  Retrieves an organization by its ID.

**tenants**  -  Manage tenants

- `afi-cli tenants <id>`  -  Retrieves a tenant by its ID.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
afi-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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
afi-cli backup-stale --max-age 48h --json
```

Protected resources whose newest archive is older than two days  -  the failure class no portal screen surfaces.

### Licensing drift check

```bash
afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
```

Purchased subscription quantities vs actually-protected resource counts, per tenant.

## Auth Setup

Create an Application and API key in the Afi portal (org level: Configuration → Apps; tenant level: Service → Settings → Apps), then set AFI_API_KEY. The key is sent as the raw Authorization header value (Afi keys look like appkey-...; no Bearer prefix). Each Application supports two keys for rotation. Keys inherit the Application's installation scope  -  if a tenant or org is missing from results, check where the Application is installed.

Run `afi-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  afi-cli orgs get <org-id> --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

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
afi-cli feedback "the --since flag is inclusive but docs say exclusive"
afi-cli feedback --stdin < notes.txt
afi-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/afi-cli/feedback.jsonl`. They are never POSTed unless `AFI_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AFI_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration.

```
afi-cli profile save briefing --json
afi-cli --profile briefing orgs get <org-id>
afi-cli profile list --json
afi-cli profile show briefing
afi-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `afi-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/afi/cmd/afi-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add afi-mcp -- afi-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which afi-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   afi-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `afi-cli <command> --help`.
