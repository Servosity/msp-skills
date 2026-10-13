---
name: granola
description: "Use when the user asks to search, export, or analyze their Granola meetings across calls - attendee timelines, talk-time roll-ups, topic search, and bulk transcript export from an offline SQLite mirror no per-meeting view offers. Trigger phrases: `every meeting with this client`, `talk time across last month's meetings`, `export last week's calls`, `search my granola meetings`, `what's in granola but not yet memo'd`, `attendee timeline for a contact`, `use granola`, `run granola`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Granola"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - granola-cli
    install:
      - kind: script
        bins: [granola-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.ps1
---

<!-- // PATCH(skill-doc-auth-rewrite): Auth Setup section rewritten for the
     public REST + official-MCP model. Granola v7.4x+ sealed cache-v6.json.enc
     behind its own Keychain access group, so the CLI now sources data from the
     public REST API (GRANOLA_API_KEY, required) plus Granola's official MCP
     (mcp-auth login, optional  -  private/human notes only). -->

# Granola  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `granola-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.ps1 | iex
   ```
3. Verify: `granola-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `granola-cli` and `granola-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

## When to Use This CLI

Reach for granola-cli when you need to answer cross-meeting questions Granola.ai’s web app and the GUI cannot - attendee timelines, MEMO pipeline state, talk-time aggregation, duplicate detection. It is the right tool for an agent processing transcripts in a loop, a CSM doing pre-call prep, or a consultant running a weekly retro. Pair the --json default with --select dotted paths to keep agent context lean.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. This printed CLI exposes read-only commands for inspection, export, sync, and analysis.

## Platform Notes

Every command is cross-platform except `mcp-auth`, which keeps Granola's official-MCP OAuth tokens in the macOS Keychain and is therefore macOS-only. Without it the CLI still works everywhere; the commands that show human notes simply omit that section.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### MEMO pipeline
- **`memo run`**  -  Run the preflight → extract pipeline on one meeting or every new meeting since a timestamp, emitting the MEMO three-file artifact and an ndjson run-state ledger.

  _Replaces the per-meeting shell loop that drives the MEMO pipeline  -  one call, one ndjson stream, agent-readable._

  ```bash
  granola-cli memo run --since 24h --to ~/Documents/meeting-transcripts --root ~/Documents/meeting-transcripts --json
  ```
- **`memo queue`** - List every meeting whose transcript is in the local store but whose MEMO triple is not yet on disk.

  _Answers the daily question “what’s still un-MEMO’d?” without the user opening Granola at all._

  ```bash
  granola-cli memo queue --since 7d --root ~/Documents/meeting-transcripts --json
  ```

### Transcript source labels, exit codes, and drift

Granola's public REST API labels each transcript segment `speaker.source`:
**`microphone`** (you, the near end) and **`speaker`** (the far end). The sealed
pre-7.4x desktop cache used `system`/`mixed`; that store is app-private on
Granola v7.4x+ and unreadable by any third-party tool, so those labels are
treated as **unrecognized** rather than silently counted.

`preflight` exit codes:

| Exit | Meaning |
| --- | --- |
| `0` | ready to extract |
| `2` | no transcript, or below the threshold (needs >= 5 `microphone` segments AND >= 10 CLASSIFIED segments) |
| `3` | a file in `--root` already covers this meeting - the meeting id is in its filename, or a file whose name carries the meeting title declares that id (a `Meeting ID:` line, a `Meeting Link:` to notes.granola.ai, or frontmatter `id:`), or the title AND day match for a file that declares no comparable id - **or** the meeting is unknown to both the local store and the Granola API (preflight first tries to fetch a meeting sync has not enriched yet; stderr says which case it is). A same-title file from a different day is a recurring meeting, reported as `title_collision`, and does not block. `--root` defaults to `~/Documents/Dev/meeting-transcripts`; pass the folder your memos actually live in. |
| `4` | transcript source labels are unrecognized, so the gate cannot classify the transcript -- upstream changed its grammar. **Not the same as exit 2:** the text is probably there, the channels just can't be read. |
| `5` | the meeting is not in the local store and the Granola API refused or failed the lookup (bad key, outage). The meeting may exist; fix the key or retry. |

Unclassified segments never count toward the threshold, so an upstream relabel
cannot manufacture a passing verdict.

**`one_sided`** is reported, never gated. A solo voice memo legitimately has zero
far-end segments and passes. The key is **omitted** when any label failed to
classify, because a relabelled far end and a genuine solo memo are then
indistinguishable -- absence means "cannot tell", not `false`.

`memo run` status values: `new`, `duplicate`, `missing_transcript` (absent OR
below threshold), `unknown_source_label` (grammar drift, distinct from missing),
`skipped`, `error`. Drift is reported in `unknown_sources` even on a memo that
succeeded.

`memo queue` emits a **union**: ready-to-memo rows, plus diagnostic rows carrying
`status:"skipped"` and `reason` for meetings whose labels could not be
classified. Filter diagnostics out when building a work list:

```bash
granola-cli memo queue --last 7d | jq 'select(.status != "skipped")'
```

They exist so a fleet-wide relabel cannot present as an empty queue.

### Credentials

Pass the token via the **`GRANOLA_API_KEY` environment variable**, or save it once
with `auth set-token`, which reads the secret from stdin (or from `GRANOLA_API_KEY`)
and refuses a command-line argument, so it never lands in shell history or `ps`
output:

```bash
printf %s "$TOKEN" | granola-cli auth set-token
```

### Attendee intelligence
- **`attendee timeline`** - Every meeting with a given attendee, ordered oldest→newest, with title and date per row.

  _Pre-call prep in one command; surfaces the conversation arc with a single person across months of meetings._

  ```bash
  granola-cli attendee timeline alice@example.com --since 60d --json --select id,title,started_at
  ```
- **`attendee brief`** - Pulls the last N meetings with an attendee and stitches together their real human notes plus real AI panel summaries - no synthesis.

  _Eliminates the click-each-meeting copy-paste that account leads do before every external call._

  ```bash
  granola-cli attendee brief alice@example.com --last 3 --json
  ```

### Transcript analytics
- **`talktime`** - Per-segment-source talk-time for one meeting - microphone (you) vs everyone else (reported as `system_seconds`) in minutes.

  _Granola's REST API does not report per-segment confidence, so `confidence_avg` is always 0; the mic vs system split is the input to “am I talking too much” retros._

  ```bash
  granola-cli talktime not_06Yq6JtogRihEr --json
  ```
- **`talktime --by participant`** - Lifts the per-source talk-time aggregation across N meetings since a date - your share of the talking (your microphone) versus everyone else, over time. The public API names no meeting owner, so this is a you-versus-everyone split, not a per-person ranking.

  _Time-defrag retro input that no per-meeting tool can produce._

  ```bash
  granola-cli talktime --by participant --since 7d --json
  ```

### Pipeline hygiene
- **`duplicates scan`** - Groups the local store and a meeting-transcripts repo by meeting id (from the store, the file name, or the id a file declares); a file with no id is matched by title plus the date in its name, and only when exactly one stored meeting fits.

  _Repos accumulate near-duplicate files when meetings are re-extracted; this returns the dupe groups for cleanup._

  ```bash
  granola-cli duplicates scan --root ~/Documents/meeting-transcripts --json
  ```

### REST + MCP backbone
- **`enrich`** - Populate meetings, transcripts, and attendees from the public REST detail endpoint (`/v1/notes/{id}?include=transcript`). Needed on Granola v7.4x+ where the desktop cache is sealed; `sync` runs it for you, or call it directly with `--full` for the whole library.

  _Hydrates the rich local tables straight from `public-api.granola.ai` - no desktop app, no decryption._

  ```bash
  granola-cli enrich --full --json
  ```
- **`mcp-auth login`** - Connect Granola's official MCP (OAuth + PKCE) - the only source of the private/human notes the public REST API omits.

  _Adds the one stream REST cannot return; tokens land in the macOS Keychain, never on disk._

  ```bash
  granola-cli mcp-auth login
  ```
- **`notes-show`** - Show only the human-typed notes (rich-text notes rendered to markdown) for a meeting - the human stream, addressable separately from AI panels and transcript.

  _Isolates what a person actually wrote from the AI summary, so agents can route the two streams differently._

  ```bash
  granola-cli notes-show not_06Yq6JtogRihEr --json
  ```

## Command Reference

This CLI exposes 35+ commands. The full tree is too long to inline; ask the CLI for the canonical list:

<!-- cli-claims:ignore -->
```bash
granola-cli --help                              # top-level commands
granola-cli <command> --help                    # subcommands + flags
granola-cli agent-context --json                # machine-readable command tree for agents
```

Quick orientation by group:

| Group | Commands | Purpose |
|-------|----------|---------|
| **MEMO pipeline** | `memo run`, `memo queue`, `preflight`, `extract` | Composed three-stream pipeline; reads the REST-fed store (plus Granola's MCP for human notes) and writes the MEMO triple |
| **Meetings** | `meetings list`, `meetings get`, `meetings fetch-batch`, `show` | List and fetch meetings (read-only; `get`/`fetch-batch` fall back to the public REST API) |
| **Streams** | `notes-show`, `panel get`, `transcript get` | The three streams - human notes, AI panels, transcript - addressable separately |
| **Export** | `export`, `export-all` | Combined three-stream markdown export, single or bulk |
| **Cross-meeting analytics** | `attendee timeline`, `attendee brief`, `talktime`, `stats frequency`, `stats duration`, `stats attendees`, `stats calendar`, `collect`, `duplicates scan` | Queries no per-meeting tool can answer |
| **Folders** | `folders` | Granola folders via the public REST API |
| **Public-API mirrors** | `notes list`, `notes get`, `folders` | Typed Bearer-key endpoints |
| **Sync / system** | `sync`, `enrich`, `sync-api`, `mcp-auth login / status / verify / logout`, `doctor`, `auth setup`, `auth status`, `auth set-token`, `auth logout`, `which`, `agent-context`, `version` | REST-driven store hydration + enrichment, Granola-MCP connection, auth, capability discovery |

Three commands anchor the REST + MCP model:

- **`sync`**  -  one command to populate the local SQLite store from the public REST API (meeting list + folders), enriching meetings/transcripts/attendees from the per-meeting detail endpoint; also pulls private notes when Granola's MCP is connected. `--full` enriches the entire library (default: 50 most recent).
- **`enrich [--limit N | --full]`**  -  the detail/transcript/notes enrichment step alone. `sync` runs it for you; call it directly to refresh more meetings (`--full` for the whole library).
- **`mcp-auth login | status | verify | logout`**  -  connect and inspect Granola's official MCP for private/human notes. `verify` confirms those notes are reachable end-to-end.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
granola-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes


### Daily MEMO loop

```bash
granola-cli memo run --since 24h --to ~/Documents/meeting-transcripts --root ~/Documents/meeting-transcripts --json
```

Process every new meeting since yesterday into the MEMO triple format and yield only the new artifacts.

### Pre-call attendee brief

```bash
granola-cli attendee brief alice@example.com --last 3 --json
```

Pull the last three meetings with Alice, one JSON line each: id, title, date, your notes (`notes_human`), and the AI summary (`panel_summary`).

### Weekly talk-time retro

```bash
granola-cli talktime --by participant --since 7d --json
```

How much of this week's meetings was you talking versus everyone else - the time-defrag retro input no per-meeting tool can produce.

### Repo-wide duplicate scrub

```bash
granola-cli duplicates scan --root ~/Documents/meeting-transcripts --json
```

Find duplicate-meeting clusters across the MEMO output repo for cleanup.

### Bulk-export the week

```bash
granola-cli export-all --out ~/Documents/meeting-transcripts --last 7d
```

Write a combined three-stream markdown file (human notes + AI summary + transcript) for every meeting in the last 7 days.

## Auth Setup

Two surfaces. The public REST API key is required and powers everything; Granola's official MCP is optional and only supplies your private/human notes.

1. **`GRANOLA_API_KEY`  -  public REST API (required).** Powers `sync`, meetings, transcripts, summaries, folders, attendees, talk-time, search, and the MEMO pipeline's transcript + summary. Get it from **Granola Settings → API**  -  a **Personal** key (your notes) or a **Workspace** key (workspace-visible notes) both work identically. Set it:

   ```bash
   export GRANOLA_API_KEY=<your-key>
   ```

   ![Granola Settings → API showing a Personal API key and a Workspace API key](docs/images/granola-api-keys-personal-vs-workspace.png)

   *Granola Settings → API. A Personal or a Workspace key both work  -  the CLI treats them identically.*

2. **Granola's official MCP  -  private/human notes (optional, additive).** The REST API omits your raw private/human notes; Granola's official MCP server (`https://mcp.granola.ai`) is the only source for them. Everything else works without it  -  the commands that show human notes (`notes-show`, `memo`, `extract`, `export`) just omit the human-notes section until you connect. Connect once, then verify:

   ```bash
   granola-cli mcp-auth login     # browser OAuth (PKCE); tokens saved to the macOS Keychain, never on disk
   granola-cli mcp-auth status    # is it connected?
   granola-cli mcp-auth verify    # confirm private notes are reachable end-to-end
   granola-cli enrich             # add private notes to meetings you already synced now (enrich --full for all)
   ```

   ![Connecting Granola's official MCP server](docs/images/granola-mcp-setup.png)

   *Granola's official MCP setup. `mcp-auth login` connects the CLI to `mcp.granola.ai` as an OAuth client to fetch the private notes REST omits.*

3. **Two different MCP servers  -  keep them distinct.** This CLI *exposes its own* MCP server (`granola-mcp`, the agent-native surface that mirrors its commands as tools  -  see "MCP Server Installation" below) **and** separately *consumes Granola's official* MCP server (`mcp.granola.ai`, an external service run by Granola) as an OAuth client for private notes. `mcp-auth` connects the latter; it has nothing to do with the former.

4. **The sealed desktop store is unused.** On Granola v7.4x+ the local store (`~/Library/Application Support/Granola/cache-v6.json.enc`) and the internal API token sit behind Granola's own macOS Keychain access group, so no third-party binary can decrypt them. The CLI does not try  -  all data flows through the public REST API and the official MCP instead.

Run `granola-cli doctor` to verify setup: it makes a real REST call to confirm the key and reports whether Granola's MCP is connected. The sealed desktop store is not read at all, so doctor does not report on it.

### Troubleshooting

| `doctor` says... | What to do |
|---|---|
| `auth: not configured` (hint `Run 'granola-cli auth setup' for credential setup steps.`) | Set `GRANOLA_API_KEY` from Granola Settings → API (Personal or Workspace). The key powers `sync` and every data command. |
| `api: reachable (GET /v1/notes answered)` + `credentials: verified` | REST is wired up. `sync` will hydrate the local store; row counts and freshness show in `--json` output. |
| `api: ERROR base_url is not the Granola public API root` | `GRANOLA_BASE_URL` (or `base_url` in the config file) points somewhere that is not the Granola API. Unset it to use `https://public-api.granola.ai`. |
| `granola_mcp: connected (private/human notes available)` | Granola's official MCP is connected  -  human notes will appear in `notes-show`, `memo`, `extract`, `export`. |
| `granola_mcp: not connected` | Optional. Private/human notes are omitted until you run `granola-cli mcp-auth login`, then `granola-cli mcp-auth verify`. |

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  granola-cli folders --agent --select folders.id,folders.name
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  `sync` fills the local SQLite store; `meetings list --query <text>` matches titles, notes, and transcript text there
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only against Granola** - every command inspects, exports, syncs, or analyzes; it never mutates remote Granola state. The only writes are local: saved credentials (`auth set-token`), Granola-MCP tokens (`mcp-auth login`), MEMO/export files on disk, and the local teach/learning store

### Response envelope

Under `--agent` (the mode this skill uses), commands that return one JSON document wrap it in a provenance envelope. With plain `--json`, the cross-meeting commands (`meetings list`, `attendee timeline`, `talktime`, `stats`) print a bare JSON array instead. The streaming commands (`memo queue`, `memo run`, `attendee brief`) emit one JSON object per line (ndjson) in every mode, unwrapped, and ignore `--select`:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

For enveloped output, parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal  -  piped/agent consumers get pure JSON on stdout.

## Auto-Refresh

Every command auto-refreshes the local store as its first action. You do **not** need to run `granola-cli sync` before `meetings list`, `panel get`, or any other read  -  the CLI handles that for you on every invocation.

**Where it refreshes from:**

| Surface | What runs | When it fires |
|---------|-----------|---------------|
| Public REST API | `sync-api` (public-api.granola.ai → SQLite) + per-meeting detail enrichment | When `GRANOLA_API_KEY` is set |
| Granola's official MCP | private/human notes pulled into the store, plus a backfill of up to 25 meetings per command that were synced before MCP was connected (or whose MCP pass failed) | When Granola's MCP is connected (`mcp-auth login`) |
| Desktop encrypted cache | no-op | Sealed (app-private) on Granola v7.4x+  -  the CLI does not read it |

When `GRANOLA_API_KEY` is not configured, auto-refresh is a silent no-op and your underlying command produces its own auth error.

**Freshness ceiling.** Auto-refresh reads from Granola's public REST API (and the official MCP for private notes); it does **not** poke the desktop app. The freshness ceiling is whatever Granola has published to your account through the REST API and MCP.

**Provenance line.** When stderr is a TTY and you are not in `--agent` / `--json` / `--compact` / `--quiet` mode, a one-liner like `auto-refresh: api=ok (1.2s, 47 rows)` lands on stderr after the refresh. Agent and JSON consumers see no chatter on stdout.

**Failures are non-fatal.** A refresh that fails prints `api=failed: <short reason>` on stderr and the command proceeds against whatever data is already in the store. Run `granola-cli doctor` to investigate persistent refresh failures.

**Opt out** (precedence: flag wins over env):

```bash
# Single command:
granola-cli meetings list --no-refresh

# For a shell session / CI job:
export GRANOLA_NO_AUTO_REFRESH=1

# Saved per-profile via the existing profile mechanism:
granola-cli profile save "fast" --no-refresh
granola-cli --profile fast meetings list
```

**Skipped commands.** Auto-refresh never fires for `sync`, `sync-api`, `auth*`, `doctor`, `help`, `version`, `completion`, `agent-context`, `profile*`, `feedback*`, or `which`. These either do not read data or cannot operate before auth is established. `agent-context --json` exposes the full skip list under `auto_refresh.skip_list` for introspecting agents.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
granola-cli feedback "the --since flag is inclusive but docs say exclusive"
granola-cli feedback --stdin < notes.txt
granola-cli feedback list --json --limit 10
```

Entries are stored locally in the CLI data directory's `feedback.jsonl` (`~/.local/share/granola-cli/` by default). They are never POSTed unless `GRANOLA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `GRANOLA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
granola-cli profile save briefing --json
granola-cli --profile briefing folders
granola-cli profile list --json
granola-cli profile show briefing
granola-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `granola-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add granola-mcp -- granola-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which granola-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   granola-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `granola-cli <command> --help`.
