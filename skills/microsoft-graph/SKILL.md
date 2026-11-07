---
name: microsoft-graph
description: "The maintained single-binary successor to the retiring mgc  -  every MSP-relevant Microsoft Graph surface, plus an offline store that finds wasted licenses, privileged-access risks, over-privileged third-party app consents, and stale devices no single API call can. Trigger phrases: `find unused microsoft 365 licenses`, `who has global admin in this tenant`, `audit third-party app consent`, `which apps can read our mail`, `triage microsoft defender alerts`, `list non-compliant intune devices`, `microsoft graph tenant snapshot`, `list teams phone numbers`, `download my teams meeting transcript`, `use microsoft-graph`, `run microsoft-graph`."
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
      - kind: script
        bins: [microsoft-graph-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1
---

# Microsoft Graph - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `microsoft-graph-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex
   ```
3. Verify: `microsoft-graph-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `microsoft-graph-cli` and `microsoft-graph-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Microsoft is retiring the Microsoft Graph CLI (mgc) in August 2026, leaving M365 admins and MSPs without a lightweight, scriptable replacement scoped to the directory, security, licensing, and device core. This is that replacement: one cross-platform Go binary (no .NET or PowerShell runtime), with a local SQLite store that powers cross-entity answers  -  licenses waste, admins audit, security triage, managed-devices drift, tenant snapshot  -  that no single Graph endpoint returns.

## When to Use This CLI

Reach for this CLI for read-side Microsoft 365 / Entra tenant administration from a terminal, script, or agent: directory lookups (users, groups, roles), licensing and cost questions, security-alert triage, Intune device-compliance reporting, third-party app consent, and Teams Phone reads (telephone numbers, per-user voice configuration and effective Teams policy assignments). It is the right tool when you want one cross-platform binary instead of the retiring mgc, the PowerShell Microsoft.Graph module, or the M365 admin portals, and especially when the question spans entities (waste, orphaned licenses, privileged access, compliance drift, tenant posture) that no single Graph call answers. It is read-only: it does not create, update, or delete anything in a tenant. The cross-entity analytics commands (licenses waste/orphans/map, admins audit, security triage, managed-devices drift, tenant snapshot, groups risk) read the LOCAL SQLite store: run `microsoft-graph-cli pull` first to populate it, or they will honestly return empty results with a stderr sync hint. Teams meeting transcripts are own-tenant only: an MSP must not read its clients' meetings, so the `transcripts` commands refuse any token issued for another tenant (a GDAP or guest sign-in), are never part of a cross-tenant run, and keep nothing on disk (no local store, response cache or learn journal).

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to create, update, or delete directory objects, assign phone numbers or policies, send mail, or change tenant state: it is read-only.
- Do not use the cross-entity analytics commands against a never-synced store and treat empty output as a real answer: run `pull` first; an unsynced store returns honest empties with a stderr hint.
- Do not use the `transcripts` commands to read a client tenant's meetings: they are own-tenant only and refuse a GDAP, guest or other-tenant token.

## Unique Capabilities

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
- **`apps consent`**  -  Inventories every non-Microsoft enterprise application consented into the tenant and the access it holds, risk-ranked with over-privileged, admin-consented, privilege-escalation and user-consented flags.

  _Run this for the quarterly app-consent review: it is the one-command answer to which third-party apps can read or change this tenant._

  ```bash
  microsoft-graph-cli apps consent --agent
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

### Teams meetings (own tenant)

Why own tenant only: an MSP's credentials can reach its clients' tenants (GDAP, multi-tenant apps), but a client's meetings are never the MSP's to read. So the `transcripts` commands need `MICROSOFT_GRAPH_HOME_TENANT` set to your own tenant id and refuse, before any request, a token for another tenant, a GDAP or guest sign-in, an unreadable token, and an app-only token unless `MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1` (with a Teams application access policy, `Grant-CsApplicationAccessPolicy`). They keep nothing on disk (no local store, response cache or learn journal). Through MCP they are read-only, return the text instead of writing a file, and `MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1` removes them.

- **`transcripts list`**  -  Lists the transcripts of one Teams meeting the signed-in user can already open, in the credential's own tenant only.

  _Use it to find the transcript of your own MSP's meeting; it refuses a client tenant by design._

  ```bash
  microsoft-graph-cli transcripts list --meeting <meeting-id> --agent
  ```
- **`transcripts download`**  -  Downloads one Teams meeting transcript as WebVTT to stdout or a named file, in the credential's own tenant only; nothing is kept on disk by the CLI.

  _Reach for this to pull your own meeting's transcript into notes or an agent without copying it out of Teams._

  ```bash
  microsoft-graph-cli transcripts download --meeting <meeting-id> --transcript <transcript-id> --out meeting.vtt
  ```
- **`transcripts all`**  -  Lists every transcript from the meetings one user organized, in the credential's own tenant only (Graph offers this to application tokens only, behind an explicit opt-in).

  _Use it to see which of your own team's recent meetings have transcripts, without opening each meeting._

  ```bash
  microsoft-graph-cli transcripts all --organizer <user-id> --since 7d --agent
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

**phone-numbers**  -  Teams Phone telephone numbers in the tenant and what each is assigned to (read-only)

- `microsoft-graph-cli phone-numbers get`  -  Get one Teams telephone number assignment by its id (needs TeamsTelephoneNumber.Read.All)
- `microsoft-graph-cli phone-numbers list`  -  List the tenant's Teams telephone numbers with type, activation and assignment (needs TeamsTelephoneNumber.Read.All)

**security**  -  Microsoft Defender / Sentinel security alerts and incidents

- `microsoft-graph-cli security alert`  -  Get a security alert by id
- `microsoft-graph-cli security alerts`  -  List security alerts (alerts_v2)
- `microsoft-graph-cli security incident`  -  Get a security incident by id
- `microsoft-graph-cli security incidents`  -  List security incidents

**teams-users**  -  Teams user configuration: phone numbers, Enterprise Voice and effective Teams policy assignments per user (read-only)

- `microsoft-graph-cli teams-users get`  -  Get one user's Teams configuration: numbers
- `microsoft-graph-cli teams-users list`  -  List Teams user configurations: numbers

**users**  -  Entra ID (Azure AD) users  -  list, get, mail, and license details

- `microsoft-graph-cli users get`  -  Get a user by object id or userPrincipalName
- `microsoft-graph-cli users licenses`  -  List the SKUs/licenses assigned to a user
- `microsoft-graph-cli users list`  -  List users in the tenant
- `microsoft-graph-cli users me`  -  Get the signed-in user (delegated tokens only; app-only tokens have no /me)
- `microsoft-graph-cli users messages`  -  List a user's mail messages


**admins**


- `microsoft-graph-cli admins audit`  -  Lists every holder of a privileged directory role with the role name, account-enabled state, and guest/disabled risk flags.


**apps**


- `microsoft-graph-cli apps consent`  -  Inventories every non-Microsoft enterprise application consented into the tenant and the access it holds, risk-ranked with over-privileged, admin-consented, privilege-escalation and user-consented flags.


**groups**


- `microsoft-graph-cli groups risk`  -  Flags ownerless, empty, and guest-heavy groups across the tenant in one pass.


**licenses**


- `microsoft-graph-cli licenses waste`  -  Surfaces every tenant SKU where you are paying for more seats than you use, ranked by unused seats.

- `microsoft-graph-cli licenses orphans`  -  Lists disabled and guest accounts that still hold paid SKUs  -  licenses you are paying for but nobody is using.

- `microsoft-graph-cli licenses map`  -  Lists every user consuming a specific SKU, with account-enabled state and guest flags, so you can plan seat reclamation and reassignment.


**managed-devices**


- `microsoft-graph-cli managed-devices drift`  -  Flags Intune devices that are non-compliant, unencrypted, or have not checked in within a time window, attributed to their assigned user.


**security**


- `microsoft-graph-cli security triage`  -  Counts the open security alerts created in a recent time window, grouped by severity and detection source.


**tenant**


- `microsoft-graph-cli tenant snapshot`  -  One agent-readable summary of the tenant: user and guest counts, license waste, admin count, open high-severity alerts, and non-compliant device count.


**transcripts**


- `microsoft-graph-cli transcripts list`  -  Lists the transcripts of one Teams meeting the signed-in user can already open, in the credential's own tenant only.

- `microsoft-graph-cli transcripts download`  -  Downloads one Teams meeting transcript as WebVTT to stdout or a named file, in the credential's own tenant only; nothing is kept on disk by the CLI.

- `microsoft-graph-cli transcripts all`  -  Lists every transcript from the meetings one user organized, in the credential's own tenant only (Graph offers this to application tokens only, behind an explicit opt-in).




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
microsoft-graph-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

Microsoft Graph uses OAuth2 bearer tokens. For unattended MSP use, run `auth login --tenant <tenant-id> --client-id <app-id> --client-secret <secret>` to mint and cache an app-only token via the client-credentials flow. Alternatively, export a pre-minted token as `MICROSOFT_GRAPH_TOKEN` (for example from `az account get-access-token --scope https://graph.microsoft.com/.default --query accessToken -o tsv` or Graph Explorer). Read scopes such as User.Read.All, Directory.Read.All, RoleManagement.Read.Directory, SecurityAlert.Read.All, and DeviceManagementManagedDevices.Read.All must be granted and admin-consented on the app registration. Teams Phone reads need TeamsTelephoneNumber.Read.All and TeamsUserConfiguration.Read.All. The `transcripts` commands are own-tenant only, because an MSP must not read its clients' meetings: set MICROSOFT_GRAPH_HOME_TENANT to your own tenant id, and a token for any other tenant (a client tenant, or a GDAP or guest sign-in) is refused before a request is sent. They default to a delegated (signed-in user) token with OnlineMeetingTranscript.Read.All and read only meetings that user can already open; an app-only token is refused unless MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 and a Teams application access policy (Grant-CsApplicationAccessPolicy) limits the app to named users. They keep nothing on disk, and MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1 removes them from the MCP server.

Run `microsoft-graph-cli doctor` to verify setup.

## Credential safety

- Requests go only to the scheme, host and port of `MICROSOFT_GRAPH_BASE_URL` (default `https://graph.microsoft.com/v1.0`): a redirect, or an `@odata.nextLink`, naming anywhere else is refused or fetched from the configured host, so the token never reaches another host. `auth login` sends the client secret only to `login.microsoftonline.com` and refuses a redirect.
- Any output line, file, local-store record, MCP result or outbound request that would carry the token, the client secret or a credential from a response is withheld whole, and "[credkit: withheld from <where>: it carried a credential this program holds]" prints in its place. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential; `auth login` and `auth set-token` write it.
- MCP tools only read: any tool that could change the tenant previews and runs only with `confirm: true`, and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  microsoft-graph-cli devices list --agent --select id,displayName,operatingSystem
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

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `MICROSOFT_GRAPH_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `MICROSOFT_GRAPH_CONFIG_DIR`, `MICROSOFT_GRAPH_DATA_DIR`, `MICROSOFT_GRAPH_STATE_DIR`, `MICROSOFT_GRAPH_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `MICROSOFT_GRAPH_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `microsoft-graph-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "microsoft-graph": {
        "command": "microsoft-graph-mcp",
        "env": {
          "MICROSOFT_GRAPH_HOME": "/srv/microsoft-graph"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `MICROSOFT_GRAPH_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `MICROSOFT_GRAPH_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `MICROSOFT_GRAPH_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `MICROSOFT_GRAPH_DATA_SOURCE` sets the `--data-source` default.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
microsoft-graph-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "microsoft-graph-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `microsoft-graph-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `microsoft-graph-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `microsoft-graph-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
microsoft-graph-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
microsoft-graph-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
microsoft-graph-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
microsoft-graph-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`microsoft-graph-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `MICROSOFT_GRAPH_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
microsoft-graph-cli feedback "the --since flag is inclusive but docs say exclusive"
microsoft-graph-cli feedback --stdin < notes.txt
microsoft-graph-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `MICROSOFT_GRAPH_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MICROSOFT_GRAPH_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

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

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add microsoft-graph-mcp -- microsoft-graph-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `microsoft-graph-mcp --transport http` requires `MICROSOFT_GRAPH_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `MICROSOFT_GRAPH_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `MICROSOFT_GRAPH_MCP_READ_ONLY=1` drops every tool not marked read-only, including the local-write learn tools that only touch the CLI's own learn store.

## Direct Use

1. Check if installed: `which microsoft-graph-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   microsoft-graph-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `microsoft-graph-cli <command> --help`.
