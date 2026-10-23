# CloudAlly CLI

**CloudAlly SaaS backup for Microsoft 365 and Google Workspace: partner, reseller and customer accounts, backup tasks and their status, users, billing and recovery jobs through the Management API v2.**

Drives CloudAlly backups of Microsoft 365 and Google Workspace for one company account or, with a partner login, every customer: backup status across customers, backup tasks and their entities, running, pausing and cancelling backups, restoring one entity from a backup date, accounts, resellers, users and billing.

Learn more at [CloudAlly](https://www.cloudally.com).

Created by [@sonofskywalker3](https://github.com/sonofskywalker3) (Jeff Moore).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `cloudally-cli` and `cloudally-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.ps1 | iex
   ```
3. Verify: `cloudally-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cloudally). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/cloudally --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/cloudally --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cloudally-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cloudally skill from https://github.com/Servosity/msp-skills/tree/main/skills/cloudally. The skill defines how its required CLI (`cloudally-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle - Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cloudally).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CLOUDALLY_CLIENT_ID` and `CLOUDALLY_CLIENT_SECRET` when Claude Desktop prompts you. Leave `CLOUDALLY_ACCESS_TOKEN` empty: run `cloudally-cli auth login --partner` (or `auth login` for one company) in a terminal once instead. The MCP server reads the saved token and refreshes it.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "cloudally": {
      "command": "cloudally-mcp",
      "env": {
        "CLOUDALLY_CLIENT_ID": "<your CloudAlly API client id>",
        "CLOUDALLY_CLIENT_SECRET": "<your CloudAlly API client secret>"
      }
    }
  }
}
```

</details>

## Authentication

CloudAlly needs three things on every request: a client-id header, a client-secret header (the API key pair created under Integrations > REST API in the partner portal) and a Bearer access token. The key pair identifies the integration; what the CLI can reach depends only on which login you use. Set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, then set CLOUDALLY_EMAIL and CLOUDALLY_PASSWORD to one company's CloudAlly account and run `cloudally-cli auth login`: that is the default, and every command is limited to that one company. For partner-wide work (status across customers, the `distributor-*` and `reseller-*` commands, switching between customers with `auth use-account <accountId>`), set CLOUDALLY_PARTNER_EMAIL and CLOUDALLY_PARTNER_PASSWORD and run `cloudally-cli auth login --partner`; that login can see and change every customer account. `auth use-account --partner` switches back to the partner token. Tokens are stored in the credentials file and refreshed automatically.

## Quick Start

```bash
# Confirm the client headers and token are configured and the API is reachable
cloudally-cli doctor

# List the company's Microsoft 365 backup tasks to get the task id
cloudally-cli tasks list --source MS365 --agent

# See every mailbox in that task and its backup status
cloudally-cli tasks get 64b0e9 --agent --select entities.entity,entities.status

# Preview a restore of one mailbox from its latest backup date (shown by tasks get)
cloudally-cli recovery restore --task-id 64b0e9 --account jane.doe@contoso.com --date 20261001021500 --dry-run

```

## Recipes

### Backup status across every customer (partner login)

```bash
cloudally-cli distributor-accounts backup-statuses --all --agent
```

One call returns the latest backup status of every active backup across the partner's customer accounts. Resellers use `reseller-accounts backup-statuses`; a distributor with resellers can use `distributor-resellers all-backup-statuses`.

### List one customer's backup tasks

```bash
cloudally-cli auth use-account 5f1c2a
cloudally-cli tasks list --type BACKUP --all --agent --select id,source,status
```

`auth use-account` makes the customer's account-scoped token active, so `tasks` and `user` act inside that account. `auth use-account --partner` switches back.

### Status of every task in the current account

```bash
cloudally-cli user status --all --agent
```

### Run a backup now and follow it

```bash
cloudally-cli tasks backup-now 64b0e9
cloudally-cli tasks get 64b0e9 --agent --select entities.entity,entities.status
```

### Follow restore jobs

```bash
cloudally-cli tasks list --type RESTORE --agent --select id,status
```

### Bills across resellers

```bash
cloudally-cli distributor-resellers all-bills --agent
```

## How CloudAlly behaves (read before restoring)

These were observed against a production CloudAlly partner tenant, not taken from the API reference.

- **The documented API reports only the latest backup.** `tasks get` and the status feeds (`backup-statuses`, `user status`) report each entity's most recent backup per sub-source. The Management API v2 has no call that lists older backup dates, so `recovery restore --date` can only be filled from the latest backup date the task reports, or from a date the operator already knows.
- **The documented export endpoint does nothing.** `POST .../recovery/export` (the generated `recovery export`, `distributor-recovery export` and `reseller-recovery export` commands) answers 200 with an empty body and creates no job (seen live on 2026-10-15). Do not rely on it to create a download; use the CloudAlly portal for downloads.
- **`tasks delete` deletes backed-up data.** `tasks delete` (and the distributor and reseller variants) is `DELETE .../backup-tasks/{id}`: it deletes a backup task together with its backed-up data. To stop a running backup, restore or download job, use `tasks cancel`; never point `tasks delete` at a job id.
- **Restores write into live data.** `recovery restore` and its distributor and reseller variants write back into the customer's live mailbox, site or drive. Preview with `--dry-run` and confirm the target before sending.

## Usage

Run `cloudally-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CLOUDALLY_CONFIG_DIR`, `CLOUDALLY_DATA_DIR`, `CLOUDALLY_STATE_DIR`, or `CLOUDALLY_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CLOUDALLY_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CLOUDALLY_HOME=/srv/cloudally
cloudally-cli doctor
```

Under `CLOUDALLY_HOME=/srv/cloudally`, the four dirs resolve to `/srv/cloudally/config`, `/srv/cloudally/data`, `/srv/cloudally/state`, and `/srv/cloudally/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "cloudally": {
      "command": "cloudally-mcp",
      "env": {
        "CLOUDALLY_HOME": "/srv/cloudally"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CLOUDALLY_DATA_DIR` overrides an explicit `--home` for that kind. Use `CLOUDALLY_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CLOUDALLY_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `cloudally-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### distributor

Manage distributor

- **`cloudally-cli distributor bills`** - bills
- **`cloudally-cli distributor create-user`** - Create a secondary partner under a specific reseller. Use role='administrator' for full permissions or role='custom' with a permissions block. Set accessType='all' for access to all accounts, or 'custom' with accountIds for specific accounts.
- **`cloudally-cli distributor delete-user`** - Delete a secondary user from a partner.
- **`cloudally-cli distributor details`** - Returns partner details

### distributor_account_tasks

Manage distributor account tasks

- **`cloudally-cli distributor-account-tasks activate`** - Activate a backup task to run on schedule, if it was paused.
- **`cloudally-cli distributor-account-tasks activate-all`** - Activate all accounts in a task for a backup.
- **`cloudally-cli distributor-account-tasks activate-entity`** - Activate a specific account in a task for a backup.
- **`cloudally-cli distributor-account-tasks add-entity`** - Manually add a site or account for a backup in a backup task.
- **`cloudally-cli distributor-account-tasks backup-entity-now`** - Run a backup for the specified account or site.
- **`cloudally-cli distributor-account-tasks backup-now`** - Run a backup task with all the activated accounts or sites in it.
- **`cloudally-cli distributor-account-tasks cancel`** - Cancel the specified running task (an entire backup, restore or download tasks).
- **`cloudally-cli distributor-account-tasks cancel-entity`** - Cancel the specified running task of a specified account or a site.
- **`cloudally-cli distributor-account-tasks delete`** - Delete a backup task, removing all the data.
- **`cloudally-cli distributor-account-tasks get`** - Get information for the specified task
- **`cloudally-cli distributor-account-tasks list`** - List all tasks of a specified type: backup, restore, download (paginated).
- **`cloudally-cli distributor-account-tasks pause`** - Pause execution by schedule for all accounts or sites of a backup task.
- **`cloudally-cli distributor-account-tasks pause-entity`** - Pause execution by schedule of a specified account or a site.
- **`cloudally-cli distributor-account-tasks remove-entity`** - Deactivate an account or site from a backup, removing all the coorresponding data.

### distributor_accounts

Manage distributor accounts

- **`cloudally-cli distributor-accounts access-token`** - Access token
- **`cloudally-cli distributor-accounts backup-statuses`** - Get all active backup statuses.
- **`cloudally-cli distributor-accounts create`** - Create a Cloudally account (trial status).
- **`cloudally-cli distributor-accounts list`** - Get all users belonging to the partner.
- **`cloudally-cli distributor-accounts sign-in-token`** - Sign in token
- **`cloudally-cli distributor-accounts tasks`** - tasks
- **`cloudally-cli distributor-accounts unsubscribe`** - Cancel subscription and delete a Cloudally account.

### distributor_recovery

Manage distributor recovery

- **`cloudally-cli distributor-recovery export`** - The documented export call. On the live API it answers 200 with an empty body and creates no job; do not rely on it.
- **`cloudally-cli distributor-recovery restore`** - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. One call restores one entity; repeat per entity for a batch. It writes back into the customer's live mailbox, site or drive.

### distributor_resellers

Manage distributor resellers

- **`cloudally-cli distributor-resellers accounts`** - accounts
- **`cloudally-cli distributor-resellers all-accounts`** - All accounts
- **`cloudally-cli distributor-resellers all-backup-statuses`** - Get backup status for all resellers
- **`cloudally-cli distributor-resellers all-bills`** - Get bills for all resellers
- **`cloudally-cli distributor-resellers all-tasks`** - Get backup tasks for all resellers
- **`cloudally-cli distributor-resellers backup-statuses`** - Get all active backup statuses.
- **`cloudally-cli distributor-resellers bills`** - bills
- **`cloudally-cli distributor-resellers create`** - Create a reseller
- **`cloudally-cli distributor-resellers create-account`** - Create a Cloudally account (trial status).
- **`cloudally-cli distributor-resellers create-user`** - Create a secondary user under a specific reseller. Use role='administrator' for full permissions or role='custom' with a permissions block. Set accessType='all' for access to all accounts, or 'custom' with accountIds for specific accounts.
- **`cloudally-cli distributor-resellers delete-user`** - Delete a secondary user from a reseller.
- **`cloudally-cli distributor-resellers get`** - Returns reseller by partnerId
- **`cloudally-cli distributor-resellers list`** - Returns list of current distributor's resellers
- **`cloudally-cli distributor-resellers tasks`** - tasks
- **`cloudally-cli distributor-resellers unsubscribe-account`** - Cancel subscription and delete a Cloudally account.
- **`cloudally-cli distributor-resellers update`** - update a reseller

### recovery

Manage recovery

- **`cloudally-cli recovery export`** - The documented export call. On the live API it answers 200 with an empty body and creates no job; do not rely on it.
- **`cloudally-cli recovery restore`** - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. One call restores one entity; repeat per entity for a batch. It writes back into the customer's live mailbox, site or drive.

### reseller

Manage reseller

- **`cloudally-cli reseller bills`** - bills
- **`cloudally-cli reseller create-user`** - Create a secondary user under the reseller. Use role='administrator' for full permissions or role='custom' with a permissions block. Set accessType='all' for access to all accounts, or 'custom' with accountIds for specific accounts.
- **`cloudally-cli reseller delete-user`** - Delete a secondary user from a reseller.
- **`cloudally-cli reseller details`** - Returns partner details

### reseller_account_tasks

Manage reseller account tasks

- **`cloudally-cli reseller-account-tasks activate`** - Activate a backup task to run on schedule, if it was paused.
- **`cloudally-cli reseller-account-tasks activate-all`** - Activate all accounts in a task for a backup.
- **`cloudally-cli reseller-account-tasks activate-entity`** - Activate a specific account in a task for a backup.
- **`cloudally-cli reseller-account-tasks add-entity`** - Manually add a site or account for a backup in a backup task.
- **`cloudally-cli reseller-account-tasks backup-entity-now`** - Run a backup for the specified account or site.
- **`cloudally-cli reseller-account-tasks backup-now`** - Run a backup task with all the activated accounts or sites in it.
- **`cloudally-cli reseller-account-tasks cancel`** - Cancel the specified running task (an entire backup, restore or download tasks).
- **`cloudally-cli reseller-account-tasks cancel-entity`** - Cancel the specified running task of a specified account or a site.
- **`cloudally-cli reseller-account-tasks delete`** - Delete a backup task, removing all the data.
- **`cloudally-cli reseller-account-tasks get`** - Get information for the specified task
- **`cloudally-cli reseller-account-tasks list`** - List all tasks of a specified type: backup, restore, download (paginated).
- **`cloudally-cli reseller-account-tasks pause`** - Pause execution by schedule for all accounts or sites of a backup task.
- **`cloudally-cli reseller-account-tasks pause-entity`** - Pause execution by schedule of a specified account or a site.
- **`cloudally-cli reseller-account-tasks remove-entity`** - Deactivate an account or site from a backup, removing all the coorresponding data.

### reseller_accounts

Manage reseller accounts

- **`cloudally-cli reseller-accounts access-token`** - Access token
- **`cloudally-cli reseller-accounts backup-statuses`** - Get all active backup statuses.
- **`cloudally-cli reseller-accounts create`** - Create a Cloudally account (trial status).
- **`cloudally-cli reseller-accounts list`** - Get all users belonging to the partner.
- **`cloudally-cli reseller-accounts sign-in-token`** - Sign in token
- **`cloudally-cli reseller-accounts tasks`** - tasks
- **`cloudally-cli reseller-accounts unsubscribe`** - Cancel subscription and delete a Cloudally account.

### reseller_recovery

Manage reseller recovery

- **`cloudally-cli reseller-recovery export`** - The documented export call. On the live API it answers 200 with an empty body and creates no job; do not rely on it.
- **`cloudally-cli reseller-recovery restore`** - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. One call restores one entity; repeat per entity for a batch. It writes back into the customer's live mailbox, site or drive.

### session

Manage session

- **`cloudally-cli session login`** - Returns the access and refresh tokens.
- **`cloudally-cli session partner-login`** - Returns the access and refresh tokens.
- **`cloudally-cli session refresh`** - Returns the access and refresh tokens.

### tasks

Manage tasks

- **`cloudally-cli tasks activate`** - Activate a backup task to run on schedule, if it was paused.
- **`cloudally-cli tasks activate-all`** - Activate all accounts in a task for a backup.
- **`cloudally-cli tasks activate-entity`** - Activate a specific account in a task for a backup.
- **`cloudally-cli tasks add-entity`** - Manually add a site or account for a backup in a backup task.
- **`cloudally-cli tasks backup-entity-now`** - Run a backup for the specified account or site.
- **`cloudally-cli tasks backup-now`** - Run a backup task with all the activated accounts or sites in it.
- **`cloudally-cli tasks cancel`** - Cancel the specified running task (an entire backup, restore or download tasks).
- **`cloudally-cli tasks cancel-entity`** - Cancel the specified running task of a specified account or a site.
- **`cloudally-cli tasks delete`** - Delete a backup task, removing all the data.
- **`cloudally-cli tasks get`** - Get information for the specified task
- **`cloudally-cli tasks list`** - List all tasks of a specified type: backup, restore, download (paginated).
- **`cloudally-cli tasks pause`** - Pause execution by schedule for all accounts or sites of a backup task.
- **`cloudally-cli tasks pause-entity`** - Pause execution by schedule of a specified account or a site.
- **`cloudally-cli tasks remove-entity`** - Deactivate an account or site from a backup, removing all the coorresponding data.

### user

Manage user

- **`cloudally-cli user bills`** - Get current billing forecast.
- **`cloudally-cli user create-primary`** - Create a Cloudally account (trial status).
- **`cloudally-cli user create-secondary`** - Create a Cloudally user in a Cloudally account.
- **`cloudally-cli user delete`** - Cancel subscription and delete a Cloudally account.
- **`cloudally-cli user me`** - Get Cloudally Account settings.
- **`cloudally-cli user secondaries`** - Get list of the Cloudally users of a Cloudally account.
- **`cloudally-cli user status`** - Get status for all tasks of user.
- **`cloudally-cli user update-password`** - Update password
- **`cloudally-cli user update-permissions`** - Update permission setting of a Cloudally account user.
- **`cloudally-cli user update-settings`** - Update Cloudally account settings.


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cloudally-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cloudally-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cloudally-cli learnings list`** - Inspect taught rows
- **`cloudally-cli learnings forget <query>`** - Undo a teach
- **`cloudally-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cloudally-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cloudally-cli teach-pattern`** - Install a query/resource template up front
- **`cloudally-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CLOUDALLY_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cloudally-cli` opens the database, older binaries refuse it with a version error - upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cloudally-cli tasks list

# JSON for scripting and agents
cloudally-cli tasks list --json
# Filter to specific fields
cloudally-cli tasks list --json --select id,source,status

# Dry run - show the request without sending
cloudally-cli tasks list --dry-run

# Agent mode - JSON + compact + no prompts in one flag
cloudally-cli tasks list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Health Check

```bash
cloudally-cli doctor
```

Checks configuration and paths, then verifies the stored token with one authenticated read (`GET /v2/user`): it reports the credential as valid, rejected (run `auth login` again) or not verified, and turns the API row red when `base_url` is not the CloudAlly API. Add `--fail-on error` to make a failed check exit non-zero.

## Configuration

Run `cloudally-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cloudally-cli/config.toml` (`%USERPROFILE%\.config\cloudally-cli\config.toml` on Windows); `--home`, `CLOUDALLY_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CLOUDALLY_CLIENT_ID` | credential | Yes | API application client id, sent as the `client-id` header on every request (partner portal > Integrations > REST API). |
| `CLOUDALLY_CLIENT_SECRET` | credential | Yes | API application client secret, sent as the `client-secret` header on every request. |
| `CLOUDALLY_ACCESS_TOKEN` | credential | No | Bearer token override. Leave it unset and run `auth login`, which saves and refreshes the token; when set it overrides the stored token on every command. |
| `CLOUDALLY_EMAIL` | login | No | Company account login email used by `auth login` (the default login, limited to that one company). |
| `CLOUDALLY_PASSWORD` | login | No | Company account login password used by `auth login`. |
| `CLOUDALLY_PARTNER_EMAIL` | login | No | Partner portal login email used by `auth login --partner` (reaches every customer account; needed for the `distributor-*` and `reseller-*` commands and `auth use-account`). |
| `CLOUDALLY_PARTNER_PASSWORD` | login | No | Partner portal login password used by `auth login --partner`. |
| `CLOUDALLY_BASE_URL` | endpoint | No (default `https://api.cloudally.com`) | Sends every API call to this base URL instead; for tests and mock servers. |
| `CLOUDALLY_CONFIG` | path | No | Config file path (default `~/.config/cloudally-cli/config.toml`). |
| `CLOUDALLY_MCP_READ_ONLY` | MCP | No | Set to `1` so the MCP server hides tools that change CloudAlly state and refuses non-GET execute calls. Tools still write local state: the response cache and learning entries. |
| `CLOUDALLY_MCP_HTTP_TOKEN` | MCP | Only with `--transport http` | Bearer token HTTP MCP callers must send; the HTTP transport refuses to start without it. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cloudally-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cloudally-cli doctor` to check credentials
- Run `cloudally-cli auth login --partner` (or `auth login`) again; a set `CLOUDALLY_ACCESS_TOKEN` overrides the stored token, so unset it
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 403 with an empty body on every command** - The client-id or client-secret header is missing or wrong: set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, then run cloudally-cli doctor
- **401 The user is not logged in** - The access token expired and could not be refreshed: run cloudally-cli auth login again
- **auth use-account says no partner token available** - Set CLOUDALLY_PARTNER_EMAIL and CLOUDALLY_PARTNER_PASSWORD and run cloudally-cli auth login --partner
- **auth use-account or a stored login seems ignored** - CLOUDALLY_ACCESS_TOKEN in the environment overrides the stored token on every command; unset it
- **distributor-recovery export (or recovery export) succeeds but no download appears** - That documented endpoint answers 200 with an empty body and creates no job. Use the CloudAlly portal for downloads.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**cloudally (Ruby gem)**](https://github.com/jancotanis/cloudally) - Ruby
- [**n8n-nodes-cloudally**](https://github.com/davejlong/n8n-nodes-cloudally) - TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
