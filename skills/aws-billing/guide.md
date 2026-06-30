# AWS Billing Intelligence CLI

**Get a non-expert from zero to a Slack-delivered, plain-English, waste-flagged AWS bill breakdown  -  and cache it so you don't pay per Cost Explorer call.**

A self-contained Go binary (native AWS SDK, no aws CLI needed) that pulls your bill, breaks it down per-account and per-service with month-over-month deltas, hunts real waste (idle instances, orphaned volumes, data-transfer bleed) instead of nagging you to buy Reserved Instances, and posts a smart HTML/PDF report to Slack. The headline feature is the part everyone else skips: a tiered least-privilege IAM setup (copy-paste policy, one-click CloudFormation, or admin bootstrap) so you can actually share it with a colleague.

## Install

The recommended path installs both the `aws-billing-cli` binary and the `pp-aws-billing` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install aws-billing
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install aws-billing --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install aws-billing --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install aws-billing --agent claude-code
npx -y @mvanhorn/printing-press-library install aws-billing --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.3 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/aws-billing/cmd/aws-billing-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/aws-billing-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-aws-billing --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-aws-billing --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-aws-billing skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-aws-billing. The skill defines how its required CLI can be installed.
```

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/aws-billing-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/aws-billing/cmd/aws-billing-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "aws-billing": {
      "command": "aws-billing-mcp"
    }
  }
}
```

</details>

## Authentication

Uses the native AWS credential chain  -  environment variables, a shared `--profile`, SSO, assume-role, or instance metadata  -  and signs requests itself (SigV4), so no access keys to paste and no `aws` CLI required. Org-wide cost data lives in the management/payer account; run `iam-setup` to mint exactly the read-only permissions this tool needs, then `doctor` to confirm what's reachable. Resource-level waste scans work in any member account immediately.

## Quick Start

```bash
# First: get the least-privilege billing read policy (or a one-click CloudFormation template) to share with whoever holds admin.
aws-billing-cli iam-setup --tier core

# Confirm credentials resolve and see exactly which permissions are reachable.
aws-billing-cli doctor --profile-aws dev

# Pull cost data once into the local cache so later queries don't re-bill Cost Explorer.
aws-billing-cli sync --months 3 --profile-aws prod

# Per-account + aggregate org breakdown with month-over-month deltas.
aws-billing-cli consolidated --period this-month

# Dollar-ranked waste rollup for the account  -  works without management access.
aws-billing-cli waste rank --profile-aws dev

# Render the HTML/PDF report and post the smart breakdown to Slack.
aws-billing-cli report --post-slack --period last-month

```

## Unique Features

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

## Usage

Run `aws-billing-cli --help` for the full command reference and flag list.

## Commands

### accounts

AWS Organizations member accounts (id, name, status)

- **`aws-billing-cli accounts get`** - Get one synced AWS account
- **`aws-billing-cli accounts list`** - List synced AWS accounts in the organization

Synced cost lines, forecasts, and resource inventory are surfaced through the
rich commands  -  `bill`, `consolidated`, `compare`, `forecast`, and the `waste`
family  -  plus the generic `search` and `export` commands.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
aws-billing-cli accounts list

# JSON for scripting and agents
aws-billing-cli accounts list --json

# Filter to specific fields
aws-billing-cli accounts list --json --select id,name,status

# Dry run  -  show the request without sending
aws-billing-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
aws-billing-cli accounts list --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
aws-billing-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Config file: `~/.config/aws-billing-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **AccessDenied on cost commands**  -  Run `doctor`  -  it names the exact missing ce: permission and whether you're in a member vs management account; `iam-setup --tier core` emits the policy to add.
- **Cost commands return empty but resource/waste commands work**  -  You're authenticated to a member account; org-wide Cost Explorer data lives in the management account  -  point `--profile` at a management-account profile.
- **Every query costs money / bill from the tool itself**  -  Run `sync` once, then query with `--data-source local` (the default after a sync) so reads hit the SQLite cache instead of the $0.01-per-call Cost Explorer API.
- **report --post-slack does nothing**  -  Posting delegates to `slack-pp-cli`; confirm it's installed and `slack-pp-cli doctor` reports credentials valid, then pass `--slack-channel <id>`.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**aws-cost-cli**](https://github.com/kamranahmedse/aws-cost-cli)  -  TypeScript (2500 stars)
- [**aws-cost-explorer-mcp-server**](https://github.com/aarora79/aws-cost-explorer-mcp-server)  -  Python (300 stars)
- [**aws-finops-mcp-server**](https://github.com/ravikiranvm/aws-finops-mcp-server)  -  Python (120 stars)
- [**ccExplorer**](https://github.com/cduggn/ccExplorer)  -  Go (80 stars)
- [**aws-doctor**](https://github.com/elC0mpa/aws-doctor)  -  Go (40 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
