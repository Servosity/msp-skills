---
name: riverside-fm
description: "Use when the user asks to download, back up, export, or search their own Riverside.com (Riverside.fm) podcast or video account: bulk-export a studio's transcripts, audio, and video; grab a single recording; convert a transcript to VTT/SRT/Markdown; compute per-speaker talktime; or harvest Magic Clips with fresh URLs. Works on Pro/Live/Webinar accounts with no Business-plan API key. Trigger phrases: `download my riverside transcripts`, `back up my riverside studio`, `bulk export riverside studio`, `search riverside transcripts`, `convert riverside transcript to vtt`, `harvest magic clips`, `use riverside-fm`, `run riverside-fm`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Riverside"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - riverside-fm-cli
    install:
      - kind: script
        bins: [riverside-fm-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.ps1
---

# Riverside - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `riverside-fm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.ps1 | iex
   ```
3. Verify: `riverside-fm-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `riverside-fm-cli` and `riverside-fm-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Riverside.com makes you click through Studio → Project → Take → Transcript for every download, and locks the official API behind a custom-priced Business plan. This CLI imports your logged-in browser cookies and reaches the same internal API the web app uses, giving you priority-fallback grab, bulk studio export with resume, transcript search over your whole archive, and Magic Clips harvest with CloudFront URL refresh  -  features Riverside has never shipped to Pro users.

## When to Use This CLI

Use this CLI when an agent needs to programmatically access a Riverside.com account on Pro / Live / Webinar tier (i.e., no Business API key available). It's the right choice for backing up a creator's archive, batch-downloading transcripts/audio/video by date range, full-text-searching a podcast catalog, harvesting Magic Clips when AI generation finishes, or waiting on a recording to be ready before firing downstream automation. It is NOT the right choice for write operations (creating studios, inviting guests, posting webhooks)  -  those surfaces exist in the API but weren't exercised in this print.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. This printed CLI exposes read-only commands for inspection, export, sync, and analysis.

## Unique Capabilities

### Priority-aware downloads

- **`grab`**  -  Get whichever exists for a recording in priority order: transcript first, then audio tracks, then HLS video  -  your stated goal as a single command.

  _When an agent has a recording session ID, this is the one-shot command that gets the most useful asset that exists without polling three separate endpoints first._

  ```bash
  riverside-fm-cli grab bf487406-af40-4bb4-b7f9-a6b49047b55d --agent
  ```
- **`bulk export`**  -  Walk every project and take in a studio (or date range) and save each take's transcript (JSON and text), asset list and HLS manifests, with a resume cursor in `<out>/.resume.json`. It downloads no audio or video: use `grab` for those. The killer workflow no Pro-tier tool offers.

  _For an agent backing up a creator's archive, this is the only way to get every asset out of Riverside without 6-12 manual clicks per take._

  ```bash
  riverside-fm-cli bulk export --studio damien-stevenss-studio --since 2026-04-01 --out ./archive
  ```
- **`media refresh`**  -  Re-walks production media + clip-assets to refresh short-lived CloudFront signed URLs; --prefetch downloads every asset body before TTL expires.

  _When an agent needs to archive a project's media assets, this command races the TTL clock instead of failing midway through a download chain._

  ```bash
  riverside-fm-cli media refresh --project 69fcda9fba030a19ae93a526 --prefetch --out ./media
  ```

### Transcript intelligence

- **`transcripts convert`**  -  Convert Riverside's editableWithVoiceActivity JSON (speakers + voice-activity timestamps) to VTT, SRT, plain text, JSON, or speaker-grouped Markdown  -  formats Riverside's own UI doesn't expose.

  _When an agent needs a transcript in WebVTT for a web player or JSON for downstream NLP, this command converts the locally-cached transcript without re-hitting the API._

  ```bash
  riverside-fm-cli transcripts convert bf487406-af40-4bb4-b7f9-a6b49047b55d --format vtt --out episode-12.vtt
  ```
- **`search`**  -  SQLite FTS5 over locally-cached transcription bodies; speaker filter joins the speakers array; output names the session, project, matched line, and timestamp.

  _Agents finding a quote or moment in a creator's backlog use this instead of opening Riverside studios one by one._

  ```bash
  riverside-fm-cli search "compounding loop" --json
  ```
- **`transcripts talktime`**  -  From the cached voice-activity timestamps, compute seconds spoken per speaker, % of total, longest monologue, and interrupt count.

  _Agents grading interview pacing or producer-host balance use this to answer talktime questions without re-watching takes._

  ```bash
  riverside-fm-cli transcripts talktime bf487406-af40-4bb4-b7f9-a6b49047b55d --json
  ```

### Production ops

- **`ready`**  -  List every take across all your synced studios that is fully ready: cloud backup done, transcription finished, no participant track still uploading.

  _Agents helping a producer with multiple shows need one call to see what's ready to cut  -  not 4 studios x N projects of manual clicking._

  ```bash
  riverside-fm-cli ready --studio my-studio --json --select studio,project,take_id,duration_min
  ```
- **`wait`**  -  Block until a take's backup, transcription, and/or AI generation are done; --include selects which facets to wait on; --timeout caps the wait. Exits 0 on ready, 2 on timeout.

  _Lets an agent pipeline depend on Riverside readiness without busy-loops or hardcoded sleeps._

  ```bash
  riverside-fm-cli wait bf487406-af40-4bb4-b7f9-a6b49047b55d --include transcript,assets,ai --timeout 30m
  ```
- **`clips list`**  -  List a project's "Made for you" Magic Clips with Riverside's own AI rating (calculated_score 0-100), the raw model score, and each clip's start/end inside the take.

  _When an agent automates social-clip distribution, this is the one command to pull every Magic Clip with fresh, downloadable URLs._

  ```bash
  riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --download --out ./clips
  ```
- **`clips harvest`**  -  Gates on AI-generation-status=ready, lists the project's Magic Clips with AI scores + take in/out points, refreshes any rendered clip's signed URL, optionally downloads MP4s.

  _When an agent automates social-clip distribution, this is the one command to pull every Magic Clip with fresh, downloadable URLs._

  ```bash
  riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --download --out ./clips
  ```
- **`stale`**  -  Cross-studio query: which recordings have been stuck in uploading / processing / transcribing past N hours? Catches dropped guest-wifi uploads and abandoned takes.

  _Production agents catching stuck takes before the host has moved on use this to triage what needs guest re-upload._

  ```bash
  riverside-fm-cli stale --studio my-studio --days 1 --json
  ```

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

## Discovery Signals

This CLI was generated with browser-observed traffic context.
- Capture coverage: 76 API entries from 76 total network entries
- Protocols: rest_json (75% confidence)
- Auth signals: cookie_session
- Generation hints: browser_http_transport, requires_protected_client
- Candidate command ideas: create_logs  -  Derived from observed POST /api/logs traffic.; create_migrate  -  Derived from observed POST /api/v4/global-search/migrate traffic.; get_assets  -  Derived from observed GET /api/v4/take/{uuid}/assets traffic.; get_clip_assets  -  Derived from observed GET /api/v4/take/{uuid}/clip/69fcda9fba030a19ae93a63c/clip-assets traffic.; get_damienstevens_fqd4a6ckh  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-fqd4a6ckh traffic.; get_damienstevens_lmb0ml5mk  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-lmb0ml5mk traffic.; get_damienstevens_ms1to36g5  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-ms1to36g5 traffic.; get_damienstevens_nev5txur8  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-nev5txur8 traffic.
- Caveats: empty_payload: API-looking request returned an empty or null payload; schema confidence is weak.; error_status_cluster: Endpoint cluster only observed error HTTP statuses.

`--ca-cert`, `RIVERSIDE_FM_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `RIVERSIDE_FM_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. `doctor` reports the trust in use and names the setting that fixes a chain or hostname failure.

## Command Reference

**ai**  -  AI/Magic features status checks

- `riverside-fm-cli ai can-create-event`  -  Check whether the workspace can create scheduled events for a studio.
- `riverside-fm-cli ai can-generate`  -  Check whether the workspace can run AI generation (magic clips, magic episodes) for a studio.

**clips**  -  Clips (Magic Clips, Magic Segments, manual edits)

- `riverside-fm-cli clips get`  -  Get a clip with full take + clip metadata (export status, AI generation info, transcription language).
- `riverside-fm-cli clips get-patches`  -  Get edit patches applied to a clip.

**productions**  -  Workspace-level productions

- `riverside-fm-cli productions <productionId>`  -  List media board items (sound effects, intros, jingles) for a production. Includes signed CloudFront URLs.

**projects**  -  Riverside projects (episodes / sessions inside a studio)

- `riverside-fm-cli projects ai-generation-status`  -  Get the AI generation status (magic clips, magic episodes) for a project.
- `riverside-fm-cli projects get`  -  Get a single project with full metadata (title, scheduled events, AI generation status).
- `riverside-fm-cli projects list-by-studio`  -  List projects (episodes) in a studio.
- `riverside-fm-cli projects list-clips`  -  List every clip in a project - the 'Made for you' Magic Clips, Magic Hooks and Magic Episodes.
- `riverside-fm-cli projects list-exports`  -  List RENDERED exports (finished MP4/WAV render jobs) for a project.
- `riverside-fm-cli projects list-takes`  -  List takes (recording sessions) in a project. Each take includes participant recordings and transcription session ID.

**recordings**  -  Individual recording files (per-participant audio + video)

- `riverside-fm-cli recordings <recordingId>`  -  Get the cloud backup status for a single recording. Status values - none, processing, done.

**studios**  -  Riverside studios (top-level workspaces for a series of content)

- `riverside-fm-cli studios get`  -  Get the studio overview by slug (includes production ID, members, recent activity).
- `riverside-fm-cli studios get-v3`  -  Get the v3 studio detail by slug (legacy endpoint, returns richer config).

**takes**  -  Riverside takes (a single recording attempt grouping per-participant tracks + a session-level transcription)

- `riverside-fm-cli takes get-assets`  -  Get all track assets for a take by session ID (filenames, resolution, recording status, device info).
- `riverside-fm-cli takes get-clip-assets`  -  Get clip-specific assets for a take + clip pair.

**transcriptions**  -  Per-session transcripts with speaker labels and voice activity timestamps

- `riverside-fm-cli transcriptions <sessionId>`  -  Get the editable transcript with voice activity for a take. Returns speakers, segments, timestamps.

**user**  -  Current authenticated Riverside user

- `riverside-fm-cli user`  -  Get the current authenticated user profile (id, role, account, plan flags).

**vod**  -  HLS video-on-demand manifests per participant per take

- `riverside-fm-cli vod <sessionId> <participantHandle>`  -  Get the HLS m3u8 manifest for a participant's video. Used to stream or transcode.


**bulk**


- `riverside-fm-cli bulk export`  -  Walk every project and take in a studio (or date range) and save each take's transcript (JSON and text), asset list and HLS manifests, with a resume cursor in `<out>/.resume.json`. It downloads no audio or video: use `grab` for those. The killer workflow no Pro-tier tool offers.


**clips**


- `riverside-fm-cli clips list`  -  List a project's 'Made for you' Magic Clips with Riverside's own AI rating (calculated_score 0-100), the raw model score, and each clip's start/end inside the take.

- `riverside-fm-cli clips harvest`  -  Gates on AI-generation-status=ready, lists the project's Magic Clips with AI scores + take in/out points, refreshes any rendered clip's signed URL, optionally downloads MP4s.


**grab**


- `riverside-fm-cli grab`  -  Get whichever exists for a recording in priority order: transcript first, then audio tracks, then HLS video  -  your stated goal as a single command.


**media**


- `riverside-fm-cli media refresh`  -  Re-walks production media + clip-assets to refresh short-lived CloudFront signed URLs; --prefetch downloads every asset body before TTL expires.


**ready**


- `riverside-fm-cli ready`  -  List every take across all your synced studios that is fully ready: cloud backup done, transcription finished, no participant track still uploading.


**search**


- `riverside-fm-cli search`  -  SQLite FTS5 over locally-cached transcription bodies; speaker filter joins the speakers array; output names the session, project, matched line, and timestamp.


**stale**


- `riverside-fm-cli stale`  -  Cross-studio query: which recordings have been stuck in uploading / processing / transcribing past N hours? Catches dropped guest-wifi uploads and abandoned takes.


**transcripts**


- `riverside-fm-cli transcripts convert`  -  Convert Riverside's editableWithVoiceActivity JSON (speakers + voice-activity timestamps) to VTT, SRT, plain text, JSON, or speaker-grouped Markdown  -  formats Riverside's own UI doesn't expose.

- `riverside-fm-cli transcripts talktime`  -  From the cached voice-activity timestamps, compute seconds spoken per speaker, % of total, longest monologue, and interrupt count.


**wait**


- `riverside-fm-cli wait`  -  Block until a take's backup, transcription, and/or AI generation are done; --include selects which facets to wait on; --timeout caps the wait. Exits 0 on ready, 2 on timeout.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
riverside-fm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes


### Bulk-export a whole studio's archive

```bash
riverside-fm-cli bulk export --studio damien-stevenss-studio --since 2026-04-01 --out ./archive
```

Walks projects, takes, transcripts, asset metadata, HLS manifests; resumes if interrupted.

### Find every mention of a topic across years of podcasts

```bash
riverside-fm-cli search "network effects" --json --select id,resource_type,hit
```

FTS5 across every cached transcript with snippet context.

### Wait until a take is ready, then harvest its Magic Clips

```bash
riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --wait --download --out ./clips
```

Blocks on AI-generation-status, then refreshes signed URLs and downloads every Magic Clip.

### Compute per-speaker talktime stats

```bash
riverside-fm-cli transcripts talktime bf487406-af40-4bb4-b7f9-a6b49047b55d --json
```

Computes seconds, % of total, longest monologue, and interrupt count per speaker from voice-activity timestamps.

### Pull a clean WebVTT transcript for a take

```bash
riverside-fm-cli transcripts convert bf487406-af40-4bb4-b7f9-a6b49047b55d --format vtt --out ep12.vtt
```

Converts the voice-activity JSON to WebVTT (a format Riverside's UI doesn't expose) for embedding in a web player.

## Auth Setup

Riverside Pro / Live / Webinar tiers don't get an API key  -  but the web app you log into uses an internal API gated only by HttpOnly session cookies. Run `riverside-fm-cli auth login --chrome` once: the CLI reads `riverside_auth_access`, `riverside_auth_refresh`, `sweetsesh`, and `cloudfront_signed_url` from your local Chrome profile and reuses them. The Business API at platform.riverside.fm is NOT used by this CLI  -  it requires a Bearer key that Pro plans cannot issue, and rejects cookies outright.

Run `riverside-fm-cli doctor` to verify setup.

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
  riverside-fm-cli clips get mock-value --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending. A completed dry run exits 0 with `"success": true` and `"dry_run": true` and no HTTP `status`; validation failures still exit non-zero with `"success": false`.
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

- Use `--home <dir>` for one invocation, or set `RIVERSIDE_FM_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `RIVERSIDE_FM_CONFIG_DIR`, `RIVERSIDE_FM_DATA_DIR`, `RIVERSIDE_FM_STATE_DIR`, `RIVERSIDE_FM_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `RIVERSIDE_FM_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `riverside-fm-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "riverside-fm": {
        "command": "riverside-fm-mcp",
        "env": {
          "RIVERSIDE_FM_HOME": "/srv/riverside-fm"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `RIVERSIDE_FM_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `RIVERSIDE_FM_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `RIVERSIDE_FM_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `RIVERSIDE_FM_DATA_SOURCE` sets the `--data-source` default.

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
riverside-fm-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "riverside-fm-cli learnings confirm 12"] }
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
       materially more, record the divergence via `riverside-fm-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `riverside-fm-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
riverside-fm-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
riverside-fm-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
riverside-fm-cli teach-playbook \
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
riverside-fm-cli playbook amend \
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

`riverside-fm-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `RIVERSIDE_FM_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
riverside-fm-cli feedback "the --since flag is inclusive but docs say exclusive"
riverside-fm-cli feedback --stdin < notes.txt
riverside-fm-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `RIVERSIDE_FM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `RIVERSIDE_FM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
riverside-fm-cli profile save briefing --json
riverside-fm-cli --profile briefing clips get mock-value
riverside-fm-cli profile list --json
riverside-fm-cli profile show briefing
riverside-fm-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `riverside-fm-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add riverside-fm-mcp -- riverside-fm-mcp
```

Verify: `claude mcp list`

For a remote host, `riverside-fm-mcp --transport http` requires `RIVERSIDE_FM_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `RIVERSIDE_FM_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `RIVERSIDE_FM_MCP_READ_ONLY=1` hides `compute_per_speaker_talktime_stats` and every tool that writes the local learning store; no tool changes anything in your Riverside account either way.

## Direct Use

1. Check if installed: `which riverside-fm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   riverside-fm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `riverside-fm-cli <command> --help`.

## Magic Clips - the "Made for you" tab

**The clips live on `GET /api/v4/projects/{projectId}/clips`.** They are *not* on
`/clips/exports` - that route lists RENDERED export jobs and stays `{"clips":[],"total":0}`
until a human presses Export in the Riverside UI. That was the 2026-09-15 bug where
`clips harvest` printed `[]` on a project whose `magicClipsCount` was 21.

```bash
# every Magic Clip / Hook / Episode, ranked by Riverside's own AI rating
riverside-fm-cli clips list --project 6aa84b6e11ceb0f1f3b6de53 --agent

# just the 9:16 clips Riverside rated 85+
riverside-fm-cli clips list --project 6aa84b6e11ceb0f1f3b6de53 --type Clip --min-score 85 --agent

# one take only, and skip the per-clip detail call (faster, loses calculated_score)
riverside-fm-cli clips list --project <id> --take 3ad1cc93-06fd-407e-8d8d-68878988f31f --no-detail --agent
```

### Metadata Riverside exposes per clip

| Field | Meaning |
|---|---|
| `calculated_score` | **The rating.** 0-100 integer, the number the dashboard ranks by. Observed 70-99. Only on `GET /api/v4/clip/{id}`, so `--no-detail` drops it. |
| `score_type` | `logprob` for `magicType: Clip`, `mean_prob_cutoff` for `magicType: Hook`. The two scales are NOT comparable - use `calculated_score` to rank across types. |
| `raw_score` / `normalized_score` | The model's own score. `logprob` clips run ~ -5 to -9 (higher = better); `mean_prob_cutoff` hooks run 0-1. `normalized_score` is normalised within `batch_id`. |
| `batch_id` | The generation batch - `session_<sessionId>_<code>`. Clips from one run share it. |
| `ai_variation` | The model/prompt variant, e.g. `mc__rolling_delta_1067` (clips), `mt__arctic_molecule_1007` (hooks). |
| `magic_type` | `Clip` (Magic Clip), `Hook` (Magic Hook), or `Episode` (composed Magic Episode - unscored). |
| `start_seconds` / `end_seconds` | **In/out points inside the source take**, derived from the timeline cuts. This is the join key against your own clip picks and against the transcript. |
| `duration_ms` / `duration_seconds` | Final clip length *after* the inner disfluency / pause-removal cuts, so it is shorter than `end - start`. |
| `aspect_ratio`, `take_id`, `session_id`, `take_name`, `created_at`, `magic_clip_id` | Context. `session_id` is what `takes get-assets`, `vod`, and `transcriptions` want. |
| `render_status` | `composedRecording.status`. `draft` = no media rendered yet. |

There is no separate "virality" field - `calculated_score` *is* the rating, and `raw_score`
is the underlying model number it is bucketed from.

### Why `--download` usually says `not-rendered`

A Magic Clip is an **edit decision list, not a file**. Riverside materialises no MP4 until
somebody exports it, so `exports.*.status` is `none` and `composedRecording.status` is
`draft` for every freshly generated clip - there is no signed URL to hand back.
`clips harvest --download` therefore returns `status: "not-rendered"` with the take id and
`start_seconds` / `end_seconds`, which is what you need to cut the clip yourself from take
media you can already fetch (`grab`, `takes get-assets`, `vod`). When a clip *has* been
exported, the old behaviour is unchanged: the signed URL is refreshed and downloaded.

```bash
riverside-fm-cli clips harvest --project <id> --type Clip --limit 1 --download --out ./clips --agent
```

Cut points cross-check: the Hook `6aa85a41379e572048976ea2` reports `115.85s -> 136.218s`
(20.368s) and `duration_ms: 15955` - the 4.413s difference is exactly its two inner
`manual` cuts. The derivation handles both timeline shapes Riverside uses:
`rstlTimeline.syncedCuts` (ticks, `magicType: Clip`) and `timeline.sections[].effects`
(milliseconds, `Hook`/`Episode`, detail call only).
