---
name: aws-billing
description: "Get a non-expert from zero to a plain-English, waste-flagged AWS bill breakdown - per-account, month-over-month, with dollar-ranked waste - and cache it locally so you don't pay per Cost Explorer call. Trigger phrases: `what's my aws bill`, `break down my aws costs`, `why did my aws bill go up`, `find aws waste`, `which account is driving my aws spend`, `post my aws bill to slack`, `set up aws billing access`, `use aws-billing`, `run aws-billing-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Amazon Web Services"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - aws-billing-cli
    install:
      - kind: go
        bins: [aws-billing-cli]
        module: github.com/mvanhorn/printing-press-library/library/cloud/aws-billing/cmd/aws-billing-cli
---

# AWS Billing Intelligence  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `aws-billing-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press-library install aws-billing --cli-only
   ```
2. Verify: `aws-billing-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.3 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/aws-billing/cmd/aws-billing-cli@latest
```

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

A self-contained Go binary (native AWS SDK, no aws CLI needed) that pulls your bill, breaks it down per-account and per-service with month-over-month deltas, hunts real waste (idle instances, orphaned volumes, data-transfer bleed) instead of nagging you to buy Reserved Instances, and posts a smart HTML/PDF report to Slack. The headline feature is the part everyone else skips: a tiered least-privilege IAM setup (copy-paste policy, one-click CloudFormation, or admin bootstrap) so you can actually share it with a colleague.

## When to Use This CLI

Reach for this CLI when someone asks what an AWS bill means, why it changed month-over-month, which linked account is driving spend, or where money is being wasted on idle/orphaned resources. It is the right tool for recurring bill reviews, pre-bill cleanup sweeps, and onboarding a non-expert to read their own AWS costs. It is read-only and waste-focused; it does not buy Reserved Instances or modify any AWS resource.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. This printed CLI exposes read-only commands for inspection, export, sync, and analysis.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Onboarding that doesn't make you quit
- **`iam-setup`**  -  Get from zero to a working billing read-only setup with copy-paste least-privilege policy, a one-click CloudFormation launch-stack, or an admin bootstrap path.

  _Reach for this first when sharing the tool with someone who has never touched IAM  -  it produces exactly the permissions this CLI needs and nothing more._

  ```bash
  aws-billing-cli iam-setup --tier core --format cloudformation
  ```
- **`doctor`**  -  Probes the AWS credential chain and, on an AccessDenied, names the exact missing ce:/ec2: permission and whether you're in a member vs management account.

  _Run this whenever a command fails with AccessDenied  -  it tells you precisely what to fix instead of a generic stack trace._

  ```bash
  aws-billing-cli doctor --json
  ```

### Local state that compounds
- **`consolidated`**  -  Shows each linked account's bill with names resolved, the management rollup, and an inline month-over-month delta per account in one view.

  _The fastest answer to 'which account is driving the org bill and is it creeping?'_

  ```bash
  aws-billing-cli consolidated --period this-month --agent
  ```
- **`ask`**  -  Answers known questions about your bill (top services, biggest month-over-month mover, per-account total, transfer spend) straight from the local cache; emits the relevant slice as JSON for anything it can't answer directly.

  _Lets a non-expert get a plain answer about the bill without learning Cost Explorer's dimension grammar._

  ```bash
  aws-billing-cli ask "what changed since last month" --agent
  ```

### Waste-first, not upsell-first
- **`waste rank`**  -  One table of every idle, orphaned, or over-provisioned resource the hunters found, sorted by estimated monthly dollars wasted, with a grand-total you could save.

  _The table to paste into Slack before the monthly bill lands  -  it turns 'I think we're wasting money' into a ranked, actionable number._

  ```bash
  aws-billing-cli waste rank --agent --select rows.resource_id,rows.monthly_waste_usd
  ```
- **`waste gp2-gp3`**  -  Lists gp2 EBS volumes and computes the exact monthly dollars saved by converting each to gp3.

  _A concrete, low-risk savings target you can act on the same day without touching workloads._

  ```bash
  aws-billing-cli waste gp2-gp3 --profile-aws dev
  ```
- **`waste transfer`**  -  Surfaces the cross-AZ, cross-region, and NAT-gateway data-transfer line items ranked by spend, with the AZ/region pair that's bleeding.

  _Data transfer is the most confusing and most over-looked AWS cost; this names exactly where it's leaking._

  ```bash
  aws-billing-cli waste transfer --period last-month
  ```

## Command Reference

**accounts**  -  AWS Organizations member accounts (id, name, status)

- `aws-billing-cli accounts get`  -  Get one synced AWS account
- `aws-billing-cli accounts list`  -  List synced AWS accounts in the organization

Synced cost lines, forecasts, and resource inventory are queried through the
rich commands (`bill`, `consolidated`, `compare`, `forecast`, `waste`) and the
generic `search` / `export` commands.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
aws-billing-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Monday bill-creep check

```bash
aws-billing-cli consolidated --period this-month --agent --select accounts.name,accounts.amount_usd,accounts.delta_pct
```

Per-account spend with month-over-month delta, narrowed to just the three fields that answer 'is it creeping?'

### Pre-bill waste sweep for a dev account

```bash
aws-billing-cli waste rank --profile-aws dev --agent
```

Dollar-ranked idle/orphaned/over-provisioned resources in the member account, ready to act on before the bill lands.

### Find the data-transfer bleed

```bash
aws-billing-cli waste transfer --period last-month
```

Ranks the cross-AZ/region/NAT data-transfer line items so you can see exactly where transfer cost is leaking.

### Decode a confusing line item

```bash
aws-billing-cli explain EUC1-DataTransfer-Out-Bytes
```

Plain-English decode of an opaque usage-type string.

### Auto-report to Slack with attitude

```bash
aws-billing-cli report --post-slack --period last-month --snark
```

Renders the HTML/PDF breakdown, posts it to Slack, with the opt-in Corey-Quinn voice on the human summary.

## Auth Setup

Uses the native AWS credential chain  -  environment variables, a shared profile via `--profile-aws`, SSO, assume-role, or instance metadata  -  and signs requests itself (SigV4), so no access keys to paste and no `aws` CLI required. Org-wide cost data lives in the management/payer account; run `iam-setup` to mint exactly the read-only permissions this tool needs, then `doctor` to confirm what's reachable. Resource-level waste scans work in any member account immediately.

Run `aws-billing-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  aws-billing-cli accounts list --agent --select id,name,status
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
aws-billing-cli feedback "the --since flag is inclusive but docs say exclusive"
aws-billing-cli feedback --stdin < notes.txt
aws-billing-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/aws-billing-cli/feedback.jsonl`. They are never POSTed unless `AWS_BILLING_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AWS_BILLING_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - for example a nightly `consolidated` rollup pinned to the payer-account profile.

```
aws-billing-cli profile save briefing --json
aws-billing-cli --profile briefing accounts list
aws-billing-cli profile list --json
aws-billing-cli profile show briefing
aws-billing-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `aws-billing-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/cloud/aws-billing/cmd/aws-billing-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add aws-billing-mcp -- aws-billing-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which aws-billing-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   aws-billing-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `aws-billing-cli <command> --help`.
