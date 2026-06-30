# aws-billing skill - governance and safety model

> Unofficial. Community-built skill for the Amazon Web Services API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the aws-billing skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `aws-billing-cli` binary (and `aws-billing-mcp`),
authenticating with the native AWS credential chain documented in mcp-install.md -
environment variables, a shared `--profile-aws`, SSO, assume-role, or instance
metadata. It signs its own requests (SigV4); there are no access keys to paste and
no `aws` CLI dependency. Credentials are read from the environment only - never
written to disk, never logged, never sent anywhere except the AWS API.

## Default-safe behavior

- **Read-only against AWS.** Every command inspects cost and inventory data; the
  CLI never stops an instance, deletes a volume, modifies a resource, or buys a
  commitment. `waste gp2-gp3` even prints the `aws ec2 modify-volume` command you
  would run rather than running it.
- **`--dry-run` previews any live request.** Pass `--dry-run` on a command that
  makes a live Cost Explorer / EC2 call to see the request without sending it.
- **The only writes are local.** `sync` writes the SQLite cache; `report` writes an
  HTML/PDF file. Neither leaves your machine.
- **The only outbound action is opt-in.** `report --post-slack` posts a summary to
  Slack (delegating to the `slack-pp-cli` binary) and fires only when you pass the
  flag. Without it, `report` just writes a local file.
- **Agent mode is explicit.** `--agent` produces JSON for scripting; it does not
  loosen anything, because there is nothing to mutate. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **the full read surface plus the
local-write commands**; nothing in this CLI mutates AWS.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Bill breakdowns, rollups, comparisons, forecasts, waste scans, search. No change anywhere. | `bill`, `consolidated`, `compare`, `forecast`, `waste rank`, `waste transfer`, `ask`, `explain`, `dimensions`, `doctor`, `iam-setup` | Allow |
| **Write (local / opt-in)** | Writes to your machine, or posts to Slack only when asked. Never touches AWS. | `sync` (local cache), `report` (local file), `report --post-slack` (Slack post) | Allow; the Slack post fires only with `--post-slack` |
| **Credential / security** | Touches tokens, keys, MFA. | (none) | n/a - this CLI never handles secrets beyond reading the AWS credential chain |
| **Destructive** | Irreversible data or config loss. | (none) | n/a - the CLI never deletes or modifies an AWS resource |
| **Admin** | Back-office administration. | (none) | n/a |

## How to lock it down

- **Scope the credential** to read-only. `iam-setup --tier core` emits exactly the
  Cost Explorer + Organizations read permissions the bill commands need;
  `--tier waste` adds the EC2/CloudWatch/S3 describes for the waste hunters.
  Attach it in the management (payer) account for org-wide data; the waste tier
  works in any account.
- **A read/report workflow needs nothing more than the read-only policy above** -
  there is no destructive or credential tier to grant.
- **Treat the Slack post as the one explicit action.** If you don't want an agent
  posting to Slack autonomously, simply don't grant `--post-slack`; the rest of the
  surface is inert against the outside world.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the AWS API read-only, and you can read every line of how it does so. The skill is
read-first, local-by-default, and scoped to your own account.
