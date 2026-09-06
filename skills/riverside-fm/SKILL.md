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
---

# Riverside  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `riverside-fm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press install riverside-fm --cli-only
   ```
2. Verify: `riverside-fm-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Riverside.com makes you click through Studio → Project → Take → Transcript for every download, and locks the official API behind a custom-priced Business plan. This CLI imports your logged-in browser cookies and reaches the same internal API the web app uses, giving you priority-fallback grab, bulk studio export with resume, transcript search over your whole archive, and Magic Clips harvest with CloudFront URL refresh  -  features Riverside has never shipped to Pro users.

## When to Use This CLI

Use this CLI when an agent needs to programmatically access a Riverside.com account on Pro / Live / Webinar tier (i.e., no Business API key available). It's the right choice for backing up a creator's archive, batch-downloading transcripts/audio/video by date range, full-text-searching a podcast catalog, harvesting Magic Clips when AI generation finishes, or waiting on a recording to be ready before firing downstream automation. It is NOT the right choice for write operations (creating studios, inviting guests, posting webhooks)  -  those surfaces exist in the API but weren't exercised in this print.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. The Riverside data commands are read-only (inspection, export, sync, analysis); the only mutating command is the generic `import`, which POSTs records from a local file and is out of scope for this skill.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Priority-aware downloads
- **`grab`**  -  Get whichever exists for a recording in priority order: transcript first, then audio tracks, then HLS video  -  your stated goal as a single command.

  _When an agent has a recording session ID, this is the one-shot command that gets the most useful asset that exists without polling three separate endpoints first._

  ```bash
  riverside-fm-cli grab <session-id> --agent
  ```
- **`bulk export`**  -  Walk every project / take / asset in a studio (or date range) and download transcripts + per-participant audio + HLS manifests with a resume cursor in `<out>/.resume.json`. The killer workflow no Pro-tier tool offers.

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
  riverside-fm-cli transcripts convert <session-id> --format vtt --out episode-12.vtt
  ```
- **`search`**  -  SQLite FTS5 over locally-cached transcription bodies; speaker filter joins the speakers array; output names the session, project, matched line, and timestamp.

  _Agents finding a quote or moment in a creator's backlog use this instead of opening Riverside studios one by one._

  ```bash
  riverside-fm-cli search "compounding loop" --json
  ```
- **`transcripts talktime`**  -  From the cached voice-activity timestamps, compute seconds spoken per speaker, % of total, longest monologue, and interrupt count.

  _Agents grading interview pacing or producer-host balance use this to answer talktime questions without re-watching takes._

  ```bash
  riverside-fm-cli transcripts talktime <session-id> --json
  ```

### Production ops
- **`ready`**  -  List every take in a studio (the required `--studio <slug>`) that is fully ready: cloud backup done, transcription finished, no participant track still uploading.

  _Agents helping a producer scope a studio need one call to see what's ready to cut  -  not N projects of manual clicking._

  ```bash
  riverside-fm-cli ready --studio my-studio --json --select studio,project,take_id,duration
  ```
- **`wait`**  -  Block until a take's backup, transcription, and/or AI generation are done; --include selects which facets to wait on (`ai` requires `--project`); --timeout caps the wait. Exits 0 on ready, 2 on timeout.

  _Lets an agent pipeline depend on Riverside readiness without busy-loops or hardcoded sleeps._

  ```bash
  riverside-fm-cli wait <session-id> --include transcript,assets,ai --project <project-id> --timeout 30m
  ```
- **`clips harvest`**  -  Gates on AI-generation-status=ready, lists Magic Clip exports for a project, refreshes each clip's signed URL, optionally downloads MP4s.

  _When an agent automates social-clip distribution, this is the one command to pull every Magic Clip with fresh, downloadable URLs._

  ```bash
  riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --download --out ./clips
  ```
- **`stale`**  -  Scan the locally synced store for items with no update in N days (`--days`, default 30). A quick way to surface old or abandoned records once you have synced; it filters by last-update age, not by upload/processing status.

  _Production agents triaging an archive use this to surface records that haven't changed in a while._

  ```bash
  riverside-fm-cli stale --days 1 --json
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
- `riverside-fm-cli projects list-exports`  -  List exports (rendered MP4/WAV files) for a project.
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

- `riverside-fm-cli transcriptions <sessionId>`  -  Get the editable transcript with voice activity for a take. Returns speakers, segments, timestamps. This is the raw...

**user**  -  Current authenticated Riverside user

- `riverside-fm-cli user`  -  Get the current authenticated user profile (id, role, account, plan flags).

**vod**  -  HLS video-on-demand manifests per participant per take

- `riverside-fm-cli vod <sessionId> <participantHandle>`  -  Get the HLS m3u8 manifest for a participant's video. Used to stream or transcode.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
riverside-fm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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
riverside-fm-cli transcripts talktime <session-id> --json
```

Computes seconds, % of total, longest monologue, and interrupt count per speaker from voice-activity timestamps.

### Pull a clean WebVTT transcript for a take

```bash
riverside-fm-cli transcripts convert <session-id> --format vtt --out ep12.vtt
```

Converts the voice-activity JSON to WebVTT (a format Riverside's UI doesn't expose) for embedding in a web player.

## Auth Setup

Riverside Pro / Live / Webinar tiers don't get an API key  -  but the web app you log into uses an internal API gated only by HttpOnly session cookies. Run `riverside-fm-cli auth login --chrome` once: the CLI reads `riverside_auth_access`, `riverside_auth_refresh`, `sweetsesh`, and `cloudfront_signed_url` from your local Chrome profile and reuses them. The Business API at platform.riverside.fm is NOT used by this CLI  -  it requires a Bearer key that Pro plans cannot issue, and rejects cookies outright.

Run `riverside-fm-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  riverside-fm-cli clips get <clip-id> --agent --select id,name,status
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

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal  -  piped/agent consumers get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
riverside-fm-cli feedback "the --since flag is inclusive but docs say exclusive"
riverside-fm-cli feedback --stdin < notes.txt
riverside-fm-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.riverside-fm-cli/feedback.jsonl`. They are never POSTed unless `RIVERSIDE_FM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `RIVERSIDE_FM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
riverside-fm-cli profile save briefing --json
riverside-fm-cli --profile briefing clips get <clip-id>
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

## Direct Use

1. Check if installed: `which riverside-fm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   riverside-fm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `riverside-fm-cli <command> --help`.
