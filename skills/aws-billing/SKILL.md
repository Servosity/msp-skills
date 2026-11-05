---
name: aws-billing
description: "Get a non-expert from zero to a Slack-delivered, plain-English, waste-flagged AWS bill breakdown, and cache it so you don't pay per Cost Explorer call. Trigger phrases: `what's my aws bill`, `break down my aws costs`, `why did my aws bill go up`, `find aws waste`, `which account is driving my aws spend`, `post my aws bill to slack`, `set up aws billing access`, `use aws-billing`, `run aws-billing-cli`."
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
      - kind: script
        bins: [aws-billing-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.ps1
---

# AWS Billing Intelligence - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `aws-billing-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `aws-billing-cli` and `aws-billing-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A self-contained Go binary (native AWS SDK, no aws CLI needed) that pulls your bill, breaks it down per-account and per-service with month-over-month deltas, hunts real waste (idle instances, orphaned volumes, data-transfer bleed) instead of nagging you to buy Reserved Instances, and posts a smart HTML/PDF report to Slack. The headline feature is the part everyone else skips: a tiered least-privilege IAM setup (copy-paste policy, one-click CloudFormation, or admin bootstrap) so you can actually share it with a colleague.

## When to Use This CLI

Reach for this CLI when someone asks what an AWS bill means, why it changed month-over-month, which linked account is driving spend, or where money is being wasted on idle/orphaned resources. It is the right tool for recurring bill reviews, pre-bill cleanup sweeps, and onboarding a non-expert to read their own AWS costs. It is read-only and waste-focused; it does not buy Reserved Instances or modify any AWS resource.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. This printed CLI exposes read-only commands for inspection, export, sync, and analysis.

## Unique Capabilities

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


**ask**


- `aws-billing-cli ask`  -  Answers known questions about your bill (top services, biggest month-over-month mover, per-account total, transfer spend) straight from the local cache; emits the relevant slice as JSON for anything it can't answer directly.


**consolidated**


- `aws-billing-cli consolidated`  -  Shows each linked account's bill with names resolved, the management rollup, and an inline month-over-month delta per account in one view.


**iam-setup**


- `aws-billing-cli iam-setup`  -  Get from zero to a working billing read-only setup with copy-paste least-privilege policy, a one-click CloudFormation launch-stack, or an admin bootstrap path.


**waste**


- `aws-billing-cli waste rank`  -  One table of every idle, orphaned, or over-provisioned resource the hunters found, sorted by estimated monthly dollars wasted, with a grand-total you could save.

- `aws-billing-cli waste gp2-gp3`  -  Lists gp2 EBS volumes and computes the exact monthly dollars saved by converting each to gp3.

- `aws-billing-cli waste transfer`  -  Surfaces the cross-AZ, cross-region, and NAT-gateway data-transfer line items ranked by spend, with the AZ/region pair that's bleeding.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
aws-billing-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

Uses the native AWS credential chain  -  environment variables, a shared `--profile`, SSO, or assume-role (EC2 instance-metadata credentials are not used)  -  and signs requests itself (SigV4), so no access keys to paste and no `aws` CLI required. Org-wide cost data lives in the management/payer account; run `iam-setup` to mint exactly the read-only permissions this tool needs, then `doctor` to confirm what's reachable. Resource-level waste scans work in any member account immediately.

Run `aws-billing-cli doctor` to verify setup.

## Credential safety

- AWS credentials come from the AWS SDK default chain (environment variables, `~/.aws` shared files, SSO, assume-role, or an instance or task role). This CLI stores none of its own, and `config.toml` never holds one.
- Signed AWS requests go only to the AWS endpoints of the services this CLI calls (Cost Explorer, Organizations, STS, EC2, CloudWatch, S3, and the SSO and sign-in services), over HTTPS. A custom endpoint (`AWS_ENDPOINT_URL`) and every redirect are refused, so a signature or session token never reaches another host.
- The shared credential kit holds every AWS credential this program can read from the moment it starts (the credential variables, `~/.aws/credentials`, the credential keys of `~/.aws/config`, the SSO and login token caches, the web identity and container token files) and every credential the AWS SDK retrieves while it runs. Nothing the CLI or the MCP server prints, writes to a file or to the local store, hands to a helper program, or sends to a host other than those AWS endpoints can carry one, in any encoding: the kit withholds the whole message, file write, store row or request, and prints "[credkit: withheld from <where>: it carried a credential this program holds]" in its place. Responses are never cached on disk.
- Three AWS setups are refused rather than weakened: a custom CA bundle (`AWS_CA_BUNDLE`, or `ca_bundle` in the AWS config) stops with an error, a container credential endpoint that needs an authorization token over plain HTTP (EKS Pod Identity, or ECS with `AWS_CONTAINER_AUTHORIZATION_TOKEN`) is never sent the token, and EC2 instance-metadata (instance role) credentials are not used: nothing is sent to the instance-metadata service, so on EC2 use a profile, SSO or environment keys.
- `--dry-run` sends nothing to AWS. `report --post-slack` only previews the post unless you add `--yes`; over MCP the `report` tool previews until `confirm: true`, and an MCP call never writes a file. The Slack and PDF helper programs start with no AWS variable but the region, and do not start at all when their arguments, input or environment would carry a credential this program holds.

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
  aws-billing-cli accounts list --agent --select account_id,name,status
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

- Use `--home <dir>` for one invocation, or set `AWS_BILLING_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `AWS_BILLING_CONFIG_DIR`, `AWS_BILLING_DATA_DIR`, `AWS_BILLING_STATE_DIR`, `AWS_BILLING_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `AWS_BILLING_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains durable local data such as `data.db`. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Run `aws-billing-cli doctor --fail-on warn` to surface path warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `AWS_BILLING_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `AWS_BILLING_HOME`, or `doctor` will not find files left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; Responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `AWS_BILLING_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `AWS_BILLING_DATA_SOURCE` sets the `--data-source` default.

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
aws-billing-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "aws-billing-cli learnings confirm 12"] }
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
       materially more, record the divergence via `aws-billing-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `aws-billing-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `aws-billing-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
aws-billing-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
aws-billing-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
aws-billing-cli teach-playbook \
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
aws-billing-cli playbook amend \
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

`aws-billing-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `AWS_BILLING_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
aws-billing-cli feedback "the --since flag is inclusive but docs say exclusive"
aws-billing-cli feedback --stdin < notes.txt
aws-billing-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `AWS_BILLING_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AWS_BILLING_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 7 | Rate limited or blocked by a firewall or bot challenge (back off; do not retry immediately) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `aws-billing-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add aws-billing-mcp -- aws-billing-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `aws-billing-mcp --transport http` requires `AWS_BILLING_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `AWS_BILLING_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `AWS_BILLING_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which aws-billing-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   aws-billing-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `aws-billing-cli <command> --help`.
