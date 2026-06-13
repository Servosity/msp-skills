---
name: knowbe4
description: "Every KnowBe4 KMSAT reporting feature, plus a local SQLite store for the questions the console cannot answer. Trigger phrases: `who keeps clicking phishing in knowbe4`, `knowbe4 repeat clickers`, `knowbe4 risk leaderboard`, `who failed phishing but never trained`, `knowbe4 training coverage gaps`, `build a knowbe4 qbr`, `use knowbe4`, `run knowbe4-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "KnowBe4"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - knowbe4-cli
    install:
      - kind: go
        bins: [knowbe4-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/knowbe4/cmd/knowbe4-cli
---

# KnowBe4  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `knowbe4-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install knowbe4 --cli-only
   ```
2. Verify: `knowbe4-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/knowbe4/cmd/knowbe4-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

knowbe4-cli mirrors the KnowBe4 Reporting API (users, groups, phishing security tests and campaigns, training campaigns and enrollments, policies, and risk-score history) and syncs it all into a local SQLite database. That local store powers transcendence commands no other KnowBe4 tool has: repeat-clickers finds humans who failed multiple phishing tests, untrained-clickers anti-joins phishing failures against training records, risk-drift ranks who deteriorated this quarter, and qbr assembles a full client quarterly review in one shot.

## When to Use This CLI

Use knowbe4-cli when an agent or MSP analyst needs to answer security-awareness-posture questions that span entities or time: who keeps clicking phishing, who failed a test but never trained, whose risk worsened this quarter, which groups drove account risk up, or to assemble a client QBR. It is the right tool whenever the answer requires joining KnowBe4's separate reporting endpoints or diffing risk over time - work the console and a single API call cannot do.

## Anti-triggers

Do not use this CLI for:
- creating or modifying KnowBe4 users, groups, or phishing/training campaigns (the Reporting API surface is read-only)
- launching or scheduling a real phishing test or training enrollment
- any mutation other than pushing/deleting custom user risk events via the events group (separate KNOWBE4_USER_EVENT_API_KEY, --dry-run-previewable)
- mutations outside KnowBe4: ticketing, email, CRM, ordering, purchasing, booking

## Unique Capabilities

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

## Command Reference

**account**  -  Your KnowBe4 account: subscription, seat count, admins, and org-level risk score

- `knowbe4-cli account info`  -  Get account and subscription data (name, subscription level, seats, admins, current risk score)
- `knowbe4-cli account risk-score-history`  -  Get the org-level risk-score history time series

**groups**  -  Groups/Smart Groups  -  membership counts and group-level risk score

- `knowbe4-cli groups get`  -  Get a single group by id
- `knowbe4-cli groups list`  -  List all groups with member counts and risk scores
- `knowbe4-cli groups members`  -  List the users in a group
- `knowbe4-cli groups risk-score-history`  -  Get a group's risk-score history time series

**phishing-campaigns**  -  Phishing campaigns  -  recurring/one-time simulated-phishing programs

- `knowbe4-cli phishing-campaigns get`  -  Get a single phishing campaign by id
- `knowbe4-cli phishing-campaigns list`  -  List all phishing campaigns
- `knowbe4-cli phishing-campaigns security-tests`  -  List the phishing security tests (PSTs) run by a campaign

**phishing-tests**  -  Phishing security tests (PSTs)  -  individual simulated-phishing sends with click/report stats

- `knowbe4-cli phishing-tests get`  -  Get a single phishing security test by id
- `knowbe4-cli phishing-tests list`  -  List all phishing security tests (PSTs) across every campaign
- `knowbe4-cli phishing-tests recipient`  -  Get a single recipient's result within a PST
- `knowbe4-cli phishing-tests recipients`  -  List per-user results for a PST (who opened, clicked, reported, entered data)

**policies**  -  Uploaded policies  -  policy documents assigned for acknowledgement

- `knowbe4-cli policies get`  -  Get a single policy by id
- `knowbe4-cli policies list`  -  List all uploaded policies

**store-purchases**  -  Store purchases  -  training content/modules licensed to your account

- `knowbe4-cli store-purchases get`  -  Get a single store purchase by id
- `knowbe4-cli store-purchases list`  -  List all training store purchases (licensed content/modules)

**training-campaigns**  -  Training campaigns  -  assigned security-awareness training programs

- `knowbe4-cli training-campaigns get`  -  Get a single training campaign by id
- `knowbe4-cli training-campaigns list`  -  List all training campaigns

**training-enrollments**  -  Training enrollments  -  per-user module assignments with completion status

- `knowbe4-cli training-enrollments get`  -  Get a single training enrollment by id
- `knowbe4-cli training-enrollments list`  -  List all training enrollments with completion status and time spent

**users**  -  Users in the KnowBe4 console  -  phish-prone percentage, current risk score, group membership

- `knowbe4-cli users get`  -  Get a single user by id
- `knowbe4-cli users list`  -  List all users with risk score and phish-prone percentage
- `knowbe4-cli users risk-score-history`  -  Get a user's risk-score history time series


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
knowbe4-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Authentication uses a Reporting API key (a Bearer token) generated in the KSAT console under Account Settings -> API. Export it as KNOWBE4_API_KEY. If your tenant lives on a non-US server, set KNOWBE4_REGION to eu, ca, uk, or de so the CLI targets the right regional host. The separate User Event API (push custom risk events) uses its own key, KNOWBE4_USER_EVENT_API_KEY, consumed only by the `events` command group.

Run `knowbe4-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  knowbe4-cli groups list --agent --select id,name,status
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
knowbe4-cli feedback "the --since flag is inclusive but docs say exclusive"
knowbe4-cli feedback --stdin < notes.txt
knowbe4-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/knowbe4-cli/feedback.jsonl`. They are never POSTed unless `KNOWBE4_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `KNOWBE4_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
knowbe4-cli profile save briefing --json
knowbe4-cli --profile briefing groups list
knowbe4-cli profile list --json
knowbe4-cli profile show briefing
knowbe4-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `knowbe4-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/knowbe4/cmd/knowbe4-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add knowbe4-mcp -- knowbe4-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which knowbe4-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   knowbe4-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `knowbe4-cli <command> --help`.
