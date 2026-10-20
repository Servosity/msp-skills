# backup-radar skill - governance and safety model

> Unofficial. Community-built skill for the Backup Radar API. Not affiliated with,
> endorsed by, or sponsored by ScalePad or Backup Radar.
> This page tells an MSP owner exactly what the backup-radar skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `backup-radar-cli` binary (and `backup-radar-mcp`) with up to two
keys, and either one alone is enough for its own commands:

- `BACKUP_RADAR_API_KEY` - a ScalePad API key (ScalePad > API tab > New API key, which
  needs an active Administrator user). It authenticates the v3 API: `health`, `devices`,
  `backups ingest`, `backups retire` and `push`. It is read from the environment, or
  from the credentials file that `auth set-token` writes (mode 0600 on macOS and Linux;
  an owner-only DACL on Windows).
- `BACKUP_RADAR_V2_API_KEY` - a Backup Radar API v2 key. It authenticates the read-only
  v2 API: `backups list`, `get`, `results`, `filters`, `inactive`, `retired` and
  `overview`. It is read from the environment only and is never written to disk.

Keys are never logged and are sent only to the Backup Radar API hosts
(`api.scalepad.com` or `eu.api.scalepad.com` for v3, or the v3 `base_url` you configure;
`api.backupradar.com` for v2).
Responses are mirrored into a local SQLite store under your user account; the response
cache key includes a hash of the key that fetched it, so two tenants' keys never share
a cached row.

## Default-safe behavior

- **Only four commands write to Backup Radar.** `backups ingest`, `push` and the generic
  `import backups` post backup results (`import` sends one POST per JSONL record to the
  v3 ingest path; the API expects a root array, so prefer `push`); `backups retire` removes
  backups from monitoring. Everything else reads the API or the local mirror.
- **`--dry-run` is opt-in - use it.** `backups retire --dry-run` asks the API for the
  match set without retiring anything; `push --dry-run` validates every row and shows
  the exact first request body. `stale` only ever emits retire bodies with
  `dry_run: true`. Make your agent's policy: preview, show the exact command, get
  approval, then run the write.
- **Retire bodies are pinned, not guessed.** `stale` pins a mapped device by its
  ScalePad device id and an unmapped one by company, device and job name together; a
  backup it cannot pin gets no body and a `retire_note` explaining why.
- **Read commands are always safe to run** (`check`, `unwatched`, `scorecard`,
  `timeline`, `stale`, `coverage diff`, `health`, `devices`, the v2 `backups` reads,
  `sync`, `search`, `analytics`); they cannot change anything in Backup Radar.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `check`, `unwatched`, `scorecard`, `timeline`, `stale`, `coverage diff`, `health list`, `health get`, `devices`, `backups list`, `backups get`, `backups results`, `backups filters`, `backups inactive`, `backups retired`, `backups overview`, `sync`, `search`, `analytics`, `export` | Allow |
| **Write (routine)** | Posts backup results into Backup Radar. | `backups ingest`, `push`, `import` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Touches the saved ScalePad key. | `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Removes backups from monitoring; can close their PSA tickets. | `backups retire` (with `--close-psa-tickets`, the related tickets are closed too) | Human-in-the-loop only, explicit confirmation, after a `--dry-run` of the same filters |
| **Admin** | Back-office administration. | none in this connector (ScalePad user and policy administration are not in the API) | n/a |

## What the MCP server exposes

- `BACKUP_RADAR_MCP_READ_ONLY=1` (or `true` / `yes`) removes the `backups_ingest` and `backups_retire` tools
  and the `sync`, `export`, `learnings_forget`, `learnings_reject` and `workflow_archive`
  mirrors. The remaining tools still write local state: the response cache, the mirror
  and learning entries.
- `backups_retire` carries the MCP destructive annotation, so hosts that honor it ask
  before calling.
- `push` and `import` are never exposed over MCP (they read local files).
- Flags that read or write local files are refused from MCP callers even when the
  command is exposed: `--devices` on `coverage diff`, `--playbook-file`,
  `--playbook-notes-file` and `--notes-file` on `teach` and `teach-playbook`, and the
  root `--output`, `--db`, `--home`, `--config`, `--deliver`, `--receipt-file` and
  `--audit-dir`. A call to a mirrored command that names one of them is rejected with
  `unknown MCP parameter`. The typed endpoint tools (`health_*`, `devices_list`,
  `backups_*`) have no file flags at all. On the read tools an argument outside the schema
  is forwarded to the Backup Radar API as a query parameter and is never read as a local
  path; the write tools (`backups_ingest`, `backups_retire`) refuse any argument outside
  their schema, so a mistyped preview flag (`dry-run` for `dry_run`) fails instead of
  running live.
- When the server runs with `--transport http`, `BACKUP_RADAR_MCP_HTTP_TOKEN` must be a
  real bearer token; an unresolved `${user_config...}` placeholder is refused.

## How to lock it down

- **Give an agent the v2 key only** when it should never write. The v2 API is read-only
  by design, and `check`, `unwatched`, `stale` and `timeline` still answer from v2 data
  (they list the v3-only reasons as skipped).
- **Scope the ScalePad key** to a user whose role matches the workflow, and keep
  `backups retire` behind a human even with that key.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Rotate a key if it is ever exposed** (for example after bridging the MCP server to a
  public endpoint for ChatGPT - see mcp-install.md). Keys are created and revoked in
  ScalePad (v3) and Backup Radar (v2).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the keys, the binary uses them against the
Backup Radar API, and you can read every line of how it does so. The skill is
read-first, every write can be previewed with `--dry-run` (writes are sent as
asked when you omit it), and it is scoped to your own tenant.
