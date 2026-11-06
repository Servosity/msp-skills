# AWS Billing Intelligence CLI

**Get a non-expert from zero to a Slack-delivered, plain-English, waste-flagged AWS bill breakdown, and cache it so you don't pay per Cost Explorer call.**

A self-contained Go binary (native AWS SDK, no aws CLI needed) that pulls your bill, breaks it down per-account and per-service with month-over-month deltas, hunts real waste (idle instances, orphaned volumes, data-transfer bleed) instead of nagging you to buy Reserved Instances, and posts a smart HTML/PDF report to Slack. The headline feature is the part everyone else skips: a tiered least-privilege IAM setup (copy-paste policy, one-click CloudFormation, or admin bootstrap) so you can actually share it with a colleague.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `aws-billing-cli` and `aws-billing-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.ps1 | iex
   ```
3. Verify: `aws-billing-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=aws-billing). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/aws-billing --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/aws-billing --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `aws-billing-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the aws-billing skill from https://github.com/Servosity/msp-skills/tree/main/skills/aws-billing. The skill defines how its required CLI (`aws-billing-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=aws-billing).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`aws-billing-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `AWS_BILLING_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `AWS_BILLING_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export AWS_BILLING_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
aws-billing-mcp --transport http
```

Set `AWS_BILLING_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Uses the native AWS credential chain  -  environment variables, a shared `--profile`, SSO, or assume-role (EC2 instance-metadata credentials are not used)  -  and signs requests itself (SigV4), so no access keys to paste and no `aws` CLI required. Org-wide cost data lives in the management/payer account; run `iam-setup` to mint exactly the read-only permissions this tool needs, then `doctor` to confirm what's reachable. Resource-level waste scans work in any member account immediately.

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
aws-billing-cli report --post-slack --yes --period last-month

```

## Unique Features

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
aws-billing-cli report --post-slack --yes --period last-month --snark
```

Renders the HTML/PDF breakdown, posts it to Slack, with the opt-in Corey-Quinn voice on the human summary.

## Usage

Run `aws-billing-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `AWS_BILLING_CONFIG_DIR`, `AWS_BILLING_DATA_DIR`, `AWS_BILLING_STATE_DIR`, or `AWS_BILLING_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `AWS_BILLING_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export AWS_BILLING_HOME=/srv/aws-billing
aws-billing-cli doctor
```

On Windows (PowerShell):

```powershell
$env:AWS_BILLING_HOME = "C:\ProgramData\aws-billing"
aws-billing-cli doctor
```

Under `AWS_BILLING_HOME=/srv/aws-billing`, the four dirs resolve to `/srv/aws-billing/config`, `/srv/aws-billing/data`, `/srv/aws-billing/state`, and `/srv/aws-billing/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:AWS_BILLING_HOME = "C:\srv\aws-billing"
aws-billing-cli doctor
```

Under `$env:AWS_BILLING_HOME = "C:\srv\aws-billing"`, the four dirs resolve to `C:\srv\aws-billing\config`, `C:\srv\aws-billing\data`, `C:\srv\aws-billing\state`, and `C:\srv\aws-billing\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "aws-billing": {
      "command": "aws-billing-mcp",
      "env": {
        "AWS_BILLING_HOME": "/srv/aws-billing"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `AWS_BILLING_DATA_DIR` overrides an explicit `--home` for that kind. Use `AWS_BILLING_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `AWS_BILLING_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `aws-billing-cli doctor --fail-on warn` to check path warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. Responses are never cached on disk. The learn journal lives under `learn/` in the state directory. `aws-billing-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `aws-billing-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `AWS_BILLING_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `AWS_BILLING_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### accounts

AWS Organizations member accounts (id, name, status)

- **`aws-billing-cli accounts get`** - Get one synced AWS account
- **`aws-billing-cli accounts list`** - List synced AWS accounts in the organization


### ask


- **`aws-billing-cli ask`** - Answers known questions about your bill (top services, biggest month-over-month mover, per-account total, transfer spend) straight from the local cache; emits the relevant slice as JSON for anything it can't answer directly.


### consolidated


- **`aws-billing-cli consolidated`** - Shows each linked account's bill with names resolved, the management rollup, and an inline month-over-month delta per account in one view.


### iam-setup


- **`aws-billing-cli iam-setup`** - Get from zero to a working billing read-only setup with copy-paste least-privilege policy, a one-click CloudFormation launch-stack, or an admin bootstrap path.


### waste


- **`aws-billing-cli waste rank`** - One table of every idle, orphaned, or over-provisioned resource the hunters found, sorted by estimated monthly dollars wasted, with a grand-total you could save.

- **`aws-billing-cli waste gp2-gp3`** - Lists gp2 EBS volumes and computes the exact monthly dollars saved by converting each to gp3.

- **`aws-billing-cli waste transfer`** - Surfaces the cross-AZ, cross-region, and NAT-gateway data-transfer line items ranked by spend, with the AZ/region pair that's bleeding.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`aws-billing-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`aws-billing-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`aws-billing-cli learnings list`** - Inspect taught rows
- **`aws-billing-cli learnings forget <query>`** - Undo a teach
- **`aws-billing-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`aws-billing-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`aws-billing-cli teach-pattern`** - Install a query/resource template up front
- **`aws-billing-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `AWS_BILLING_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `aws-billing-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
aws-billing-cli accounts list

# JSON for scripting and agents
aws-billing-cli accounts list --json
# Filter to specific fields
aws-billing-cli accounts list --json --select account_id,name,status

# Dry run  -  show the request without sending
aws-billing-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
aws-billing-cli accounts list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited or blocked, `10` config error.

## Health Check

```bash
aws-billing-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `aws-billing-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/aws-billing-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\aws-billing-cli\config.toml` on Windows; `--home`, `AWS_BILLING_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `AWS_BILLING_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `AWS_BILLING_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

## Credential safety

- AWS credentials come from the AWS SDK default chain (environment variables, `~/.aws` shared files, SSO, assume-role, or an instance or task role). This CLI stores none of its own, and `config.toml` never holds one.
- Signed AWS requests go only to the AWS endpoints of the services this CLI calls (Cost Explorer, Organizations, STS, EC2, CloudWatch, S3, and the SSO and sign-in services), over HTTPS. A custom endpoint (`AWS_ENDPOINT_URL`) and every redirect are refused, so a signature or session token never reaches another host.
- The shared credential kit holds every AWS credential this program can read from the moment it starts (the credential variables, `~/.aws/credentials`, the credential keys of `~/.aws/config`, the SSO and login token caches, the web identity and container token files) and every credential the AWS SDK retrieves while it runs. Nothing the CLI or the MCP server prints, writes to a file or to the local store, hands to a helper program, or sends to a host other than those AWS endpoints can carry one, in any encoding: the kit withholds the whole message, file write, store row or request, and prints "[credkit: withheld from <where>: it carried a credential this program holds]" in its place. Responses are never cached on disk.
- Three AWS setups are refused rather than weakened: a custom CA bundle (`AWS_CA_BUNDLE`, or `ca_bundle` in the AWS config) stops with an error, a container credential endpoint that needs an authorization token over plain HTTP (EKS Pod Identity, or ECS with `AWS_CONTAINER_AUTHORIZATION_TOKEN`) is never sent the token, and EC2 instance-metadata (instance role) credentials are not used: nothing is sent to the instance-metadata service, so on EC2 use a profile, SSO or environment keys.
- `--dry-run` sends nothing to AWS. `report --post-slack` only previews the post unless you add `--yes`; over MCP the `report` tool previews until `confirm: true`, and an MCP call never writes a file. The Slack and PDF helper programs start with no AWS variable but the region, and do not start at all when their arguments, input or environment would carry a credential this program holds.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **AccessDenied on cost commands**  -  Run `doctor`  -  it names the exact missing ce: permission and whether you're in a member vs management account; `iam-setup --tier core` emits the policy to add.
- **Cost commands return empty but resource/waste commands work**  -  You're authenticated to a member account; org-wide Cost Explorer data lives in the management account  -  point `--profile` at a management-account profile.
- **Every query costs money / bill from the tool itself**  -  Run `sync` once, then query with `--data-source local` (the default after a sync) so reads hit the SQLite cache instead of the $0.01-per-call Cost Explorer API.
- **report --post-slack only prints a preview**  -  Add --yes to post (without it the command only previews). Posting delegates to `slack-pp-cli`; confirm it's installed and `slack-pp-cli doctor` reports credentials valid, then pass `--slack-channel <id>`.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**aws-cost-cli**](https://github.com/kamranahmedse/aws-cost-cli)  -  TypeScript (2500 stars)
- [**aws-cost-explorer-mcp-server**](https://github.com/aarora79/aws-cost-explorer-mcp-server)  -  Python (300 stars)
- [**aws-finops-mcp-server**](https://github.com/ravikiranvm/aws-finops-mcp-server)  -  Python (120 stars)
- [**ccExplorer**](https://github.com/cduggn/ccExplorer)  -  Go (80 stars)
- [**aws-doctor**](https://github.com/elC0mpa/aws-doctor)  -  Go (40 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
