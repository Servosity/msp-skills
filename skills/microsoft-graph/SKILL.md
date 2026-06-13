---
name: microsoft-graph
description: "The maintained single-binary successor to the retiring mgc  -  every MSP-relevant Microsoft Graph surface, plus an offline store that finds wasted licenses, privileged-access risks, and stale devices no single API call can. Trigger phrases: `find unused microsoft 365 licenses`, `who has global admin in this tenant`, `triage microsoft defender alerts`, `list non-compliant intune devices`, `microsoft graph tenant snapshot`, `use microsoft-graph`, `run microsoft-graph`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Microsoft Graph"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - microsoft-graph-cli
    install:
      - kind: go
        bins: [microsoft-graph-cli]
        module: github.com/mvanhorn/printing-press-library/library/cloud/microsoft-graph/cmd/microsoft-graph-cli
---

# Microsoft Graph  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `microsoft-graph-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install microsoft-graph --cli-only
   ```
2. Verify: `microsoft-graph-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/microsoft-graph/cmd/microsoft-graph-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Microsoft is retiring the Microsoft Graph CLI (mgc) in August 2026, leaving M365 admins and MSPs without a lightweight, scriptable replacement scoped to the directory, security, licensing, and device core. This is that replacement: one cross-platform Go binary (no .NET or PowerShell runtime), with a local SQLite store that powers cross-entity answers  -  licenses waste, admins audit, security triage, managed-devices drift, tenant snapshot  -  that no single Graph endpoint returns.

## When to Use This CLI

Reach for this CLI for read-side Microsoft 365 / Entra tenant administration from a terminal, script, or agent: directory lookups (users, groups, roles), licensing and cost questions, security-alert triage, and Intune device-compliance reporting. It is the right tool when you want one cross-platform binary instead of the retiring mgc, the PowerShell Microsoft.Graph module, or the M365 admin portals  -  and especially when the question spans entities (waste, orphaned licenses, privileged access, compliance drift, tenant posture) that no single Graph call answers. It is read-focused; apart from the explicit `import` command (a JSONL create path, previewable with `--dry-run`), it does not create, update, or delete directory objects. The cross-entity analytics commands (licenses waste/orphans/map, admins audit, security triage, managed-devices drift, tenant snapshot, groups risk) read the LOCAL SQLite store  -  run `microsoft-graph-cli pull` first to populate it, or they will honestly return empty results with a stderr sync hint.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to create, update, or delete directory objects, send mail, or change tenant state  -  the only write path is the explicit `import` command (JSONL create, previewable with `--dry-run`).
- Do not use the cross-entity analytics commands against a never-synced store and treat empty output as a real answer  -  run `pull` first; an unsynced store returns honest empties with a stderr hint.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### License cost intelligence
- **`licenses waste`**  -  Surfaces every tenant SKU where you are paying for more seats than you use, ranked by unused seats.

  _Reach for this to find recoverable M365 license spend across a tenant in one call instead of exporting SKU CSVs from the admin center._

  ```bash
  microsoft-graph-cli licenses waste --agent
  ```
- **`licenses orphans`**  -  Lists disabled and guest accounts that still hold paid SKUs  -  licenses you are paying for but nobody is using.

  _Use before a license true-up to reclaim seats assigned to disabled or guest identities._

  ```bash
  microsoft-graph-cli licenses orphans --json
  ```
- **`licenses map`**  -  Lists every user consuming a specific SKU, with account-enabled state and guest flags, so you can plan seat reclamation and reassignment.

  _Reach for this when you need to know exactly who holds a given SKU before reclaiming or reassigning seats._

  ```bash
  microsoft-graph-cli licenses map ENTERPRISEPACK --agent
  ```

### Security & privileged-access
- **`admins audit`**  -  Lists every holder of a privileged directory role with the role name, account-enabled state, and guest/disabled risk flags.

  _Run this for the monthly privileged-access review  -  it is the fastest answer to 'who can administer this tenant right now'._

  ```bash
  microsoft-graph-cli admins audit --agent
  ```
- **`security triage`**  -  Counts the open security alerts created in a recent time window, grouped by severity and detection source.

  _Reach for this every morning to answer 'what is new and still open since yesterday' without portal pagination._

  ```bash
  microsoft-graph-cli security triage --since 24h --agent
  ```
- **`groups risk`**  -  Flags ownerless, empty, and guest-heavy groups across the tenant in one pass.

  _Use this for tenant governance reviews when no single Graph filter can surface risky groups._

  ```bash
  microsoft-graph-cli groups risk --agent
  ```

### Device & tenant posture
- **`managed-devices drift`**  -  Flags Intune devices that are non-compliant, unencrypted, or have not checked in within a time window, attributed to their assigned user.

  _Use to build the weekly device-compliance ticket queue in one command instead of a portal-to-spreadsheet ETL._

  ```bash
  microsoft-graph-cli managed-devices drift --days 30 --json
  ```
- **`tenant snapshot`**  -  One agent-readable summary of the tenant: user and guest counts, license waste, admin count, open high-severity alerts, and non-compliant device count.

  _Reach for this first when you pick up a tenant  -  it is the 'where does this tenant stand' answer an MSP needs before drilling in._

  ```bash
  microsoft-graph-cli tenant snapshot --agent
  ```

## Command Reference

**devices**  -  Entra ID registered/joined device objects

- `microsoft-graph-cli devices get`  -  Get an Entra device by object id
- `microsoft-graph-cli devices list`  -  List Entra-registered devices

**directory-roles**  -  Entra ID directory roles (admin roles) and their members

- `microsoft-graph-cli directory-roles get`  -  Get a directory role by object id
- `microsoft-graph-cli directory-roles list`  -  List activated directory roles in the tenant
- `microsoft-graph-cli directory-roles members`  -  List the members assigned to a directory role

**groups**  -  Entra ID groups  -  list, get, members, and owners

- `microsoft-graph-cli groups get`  -  Get a group by object id
- `microsoft-graph-cli groups list`  -  List groups in the tenant
- `microsoft-graph-cli groups members`  -  List a group's members
- `microsoft-graph-cli groups owners`  -  List a group's owners

**licenses**  -  Tenant commercial subscriptions (subscribedSkus)

- `microsoft-graph-cli licenses sku`  -  Get a single subscribed SKU by id
- `microsoft-graph-cli licenses skus`  -  List the commercial subscriptions (SKUs) the tenant owns

**managed-devices**  -  Intune-managed devices and their compliance posture

- `microsoft-graph-cli managed-devices get`  -  Get an Intune-managed device by id
- `microsoft-graph-cli managed-devices list`  -  List Intune-managed devices (requires an Intune license)

**security**  -  Microsoft Defender / Sentinel security alerts and incidents

- `microsoft-graph-cli security alert`  -  Get a security alert by id
- `microsoft-graph-cli security alerts`  -  List security alerts (alerts_v2)
- `microsoft-graph-cli security incident`  -  Get a security incident by id
- `microsoft-graph-cli security incidents`  -  List security incidents

**users**  -  Entra ID (Azure AD) users  -  list, get, mail, and license details

- `microsoft-graph-cli users get`  -  Get a user by object id or userPrincipalName
- `microsoft-graph-cli users licenses`  -  List the SKUs/licenses assigned to a user
- `microsoft-graph-cli users list`  -  List users in the tenant
- `microsoft-graph-cli users me`  -  Get the signed-in user (delegated tokens only; app-only tokens have no /me)
- `microsoft-graph-cli users messages`  -  List a user's mail messages


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
microsoft-graph-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Find recoverable license spend

```bash
microsoft-graph-cli licenses waste --agent
```

Ranks SKUs by unused paid seats so you can right-size subscriptions at renewal.

### Monthly privileged-access review

```bash
microsoft-graph-cli admins audit --agent
```

Lists every directory-role holder with risk flags for guest or disabled admin accounts.

### Morning alert triage

```bash
microsoft-graph-cli security triage --since 24h --agent
```

Groups open alerts from the last day by severity and detection source.

### Device compliance ticket queue

```bash
microsoft-graph-cli managed-devices drift --days 30 --agent
```

Surfaces non-compliant, unencrypted, or stale-sync Intune devices mapped to their user.

### Trim a large user payload to just the fields you need

```bash
microsoft-graph-cli users list --top 50 --agent --select id,displayName,userPrincipalName,accountEnabled
```

Pairs --agent with --select to keep agent context small when a Graph user object would otherwise return dozens of properties.

## Auth Setup

Microsoft Graph uses OAuth2 bearer tokens. For unattended MSP use, run `auth login --tenant <tenant-id> --client-id <app-id> --client-secret <secret>` to mint and cache an app-only token via the client-credentials flow. Alternatively, export a pre-minted token as `MICROSOFT_GRAPH_TOKEN` (for example from `az account get-access-token --scope https://graph.microsoft.com/.default --query accessToken -o tsv` or Graph Explorer). Read scopes such as User.Read.All, Directory.Read.All, RoleManagement.Read.Directory, SecurityAlert.Read.All, and DeviceManagementManagedDevices.Read.All must be granted and admin-consented on the app registration.

Run `microsoft-graph-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  microsoft-graph-cli devices list --agent --select id,name,status
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
microsoft-graph-cli feedback "the --since flag is inclusive but docs say exclusive"
microsoft-graph-cli feedback --stdin < notes.txt
microsoft-graph-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/microsoft-graph-cli/feedback.jsonl`. They are never POSTed unless `MICROSOFT_GRAPH_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MICROSOFT_GRAPH_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
microsoft-graph-cli profile save briefing --json
microsoft-graph-cli --profile briefing devices list
microsoft-graph-cli profile list --json
microsoft-graph-cli profile show briefing
microsoft-graph-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `microsoft-graph-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/cloud/microsoft-graph/cmd/microsoft-graph-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add microsoft-graph-mcp -- microsoft-graph-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which microsoft-graph-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   microsoft-graph-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `microsoft-graph-cli <command> --help`.
