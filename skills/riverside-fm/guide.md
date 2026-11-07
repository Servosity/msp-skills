# Riverside CLI

**The first programmatic way to download every transcript, audio track, and video from your own Riverside.com account  -  no Business plan or API key required.**

Riverside.com makes you click through Studio → Project → Take → Transcript for every download, and locks the official API behind a custom-priced Business plan. This CLI imports your logged-in browser cookies and reaches the same internal API the web app uses, giving you priority-fallback grab, bulk studio export with resume, transcript search over your whole archive, and Magic Clips harvest with CloudFront URL refresh  -  features Riverside has never shipped to Pro users.

Learn more at [Riverside](https://riverside.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `riverside-fm-cli` and `riverside-fm-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=riverside-fm). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/riverside-fm --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/riverside-fm --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `riverside-fm-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the riverside-fm skill from https://github.com/Servosity/msp-skills/tree/main/skills/riverside-fm. The skill defines how its required CLI (`riverside-fm-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local browser session  -  set it up first if you haven't:

```bash
riverside-fm-cli auth login --chrome
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=riverside-fm).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "riverside-fm": {
      "command": "riverside-fm-mcp"
    }
  }
}
```

</details>

### Remote MCP over HTTP

`riverside-fm-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `RIVERSIDE_FM_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `RIVERSIDE_FM_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export RIVERSIDE_FM_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
riverside-fm-mcp --transport http
```

Set `RIVERSIDE_FM_MCP_READ_ONLY=1` (exactly `1`) to hide `compute_per_speaker_talktime_stats` and every tool that writes the local learning store (`teach`, `teach-pattern`, `teach-lookup`, `teach-playbook`, `playbook amend`, `learnings confirm`, `learnings forget` and `learnings reject`). No tool changes anything in your Riverside account either way.

## Authentication

Riverside Pro / Live / Webinar tiers don't get an API key  -  but the web app you log into uses an internal API gated only by HttpOnly session cookies. Run `riverside-fm-cli auth login --chrome` once: the CLI reads `riverside_auth_access`, `riverside_auth_refresh`, `sweetsesh`, and `cloudfront_signed_url` from your local Chrome profile and reuses them. The Business API at platform.riverside.fm is NOT used by this CLI  -  it requires a Bearer key that Pro plans cannot issue, and rejects cookies outright.

## Quick Start

```bash
# Import your Riverside session cookies from local Chrome (no password, no API key)
riverside-fm-cli auth login --chrome


# Verify the cookies authenticate against /user
riverside-fm-cli doctor


# Priority-fallback download: transcript first, then audio, then video
riverside-fm-cli grab bf487406-af40-4bb4-b7f9-a6b49047b55d


# FTS over every cached transcript
riverside-fm-cli search "compounding loops" --json

```

## Unique Features

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

## Usage

Run `riverside-fm-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `RIVERSIDE_FM_CONFIG_DIR`, `RIVERSIDE_FM_DATA_DIR`, `RIVERSIDE_FM_STATE_DIR`, or `RIVERSIDE_FM_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `RIVERSIDE_FM_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export RIVERSIDE_FM_HOME=/srv/riverside-fm
riverside-fm-cli doctor
```

On Windows (PowerShell):

```powershell
$env:RIVERSIDE_FM_HOME = "C:\ProgramData\riverside-fm"
riverside-fm-cli doctor
```

Under `RIVERSIDE_FM_HOME=/srv/riverside-fm`, the four dirs resolve to `/srv/riverside-fm/config`, `/srv/riverside-fm/data`, `/srv/riverside-fm/state`, and `/srv/riverside-fm/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:RIVERSIDE_FM_HOME = "C:\srv\riverside-fm"
riverside-fm-cli doctor
```

Under `$env:RIVERSIDE_FM_HOME = "C:\srv\riverside-fm"`, the four dirs resolve to `C:\srv\riverside-fm\config`, `C:\srv\riverside-fm\data`, `C:\srv\riverside-fm\state`, and `C:\srv\riverside-fm\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `RIVERSIDE_FM_DATA_DIR` overrides an explicit `--home` for that kind. Use `RIVERSIDE_FM_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `RIVERSIDE_FM_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `riverside-fm-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `riverside-fm-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `riverside-fm-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `RIVERSIDE_FM_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `RIVERSIDE_FM_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### ai

AI/Magic features status checks

- **`riverside-fm-cli ai can-create-event`** - Check whether the workspace can create scheduled events for a studio.
- **`riverside-fm-cli ai can-generate`** - Check whether the workspace can run AI generation (magic clips, magic episodes) for a studio.

### clips

Clips (Magic Clips, Magic Segments, manual edits)

- **`riverside-fm-cli clips get`** - Get a clip with full take + clip metadata (export status, AI generation info, transcription language).
- **`riverside-fm-cli clips get-patches`** - Get edit patches applied to a clip.

### productions

Workspace-level productions

- **`riverside-fm-cli productions <productionId>`** - List media board items (sound effects, intros, jingles) for a production. Includes signed CloudFront URLs.

### projects

Riverside projects (episodes / sessions inside a studio)

- **`riverside-fm-cli projects ai-generation-status`** - Get the AI generation status (magic clips, magic episodes) for a project.
- **`riverside-fm-cli projects get`** - Get a single project with full metadata (title, scheduled events, AI generation status).
- **`riverside-fm-cli projects list-by-studio`** - List projects (episodes) in a studio.
- **`riverside-fm-cli projects list-clips`** - List every clip in a project - the "Made for you" Magic Clips, Magic Hooks and Magic Episodes. Each record carries the AI rating (calculatedScore 0-100 via clips.get), scoreData (scoreType/rawScore/normalizedScore/batchId), aiVariation, duration, aspectRatio, the parent take, and the timeline cuts that give the clip's in/out points inside the take. NOTE - this is NOT projects.list_exports, which only lists RENDERED export jobs and returns an empty array until a human presses Export.
- **`riverside-fm-cli projects list-exports`** - List RENDERED exports (finished MP4/WAV render jobs) for a project. Empty for projects whose Magic Clips have never been exported - use projects.list_clips for the Magic Clips themselves.
- **`riverside-fm-cli projects list-takes`** - List takes (recording sessions) in a project. Each take includes participant recordings and transcription session ID.

### recordings

Individual recording files (per-participant audio + video)

- **`riverside-fm-cli recordings <recordingId>`** - Get the cloud backup status for a single recording. Status values - none, processing, done.

### studios

Riverside studios (top-level workspaces for a series of content)

- **`riverside-fm-cli studios get`** - Get the studio overview by slug (includes production ID, members, recent activity).
- **`riverside-fm-cli studios get-v3`** - Get the v3 studio detail by slug (legacy endpoint, returns richer config).

### takes

Riverside takes (a single recording attempt grouping per-participant tracks + a session-level transcription)

- **`riverside-fm-cli takes get-assets`** - Get all track assets for a take by session ID (filenames, resolution, recording status, device info).
- **`riverside-fm-cli takes get-clip-assets`** - Get clip-specific assets for a take + clip pair.

### transcriptions

Per-session transcripts with speaker labels and voice activity timestamps

- **`riverside-fm-cli transcriptions <sessionId>`** - Get the editable transcript with voice activity for a take. Returns speakers, segments, timestamps. This is the raw data the CLI converts to TXT/SRT/VTT/JSON output.

### user

Current authenticated Riverside user

- **`riverside-fm-cli user`** - Get the current authenticated user profile (id, role, account, plan flags).

### vod

HLS video-on-demand manifests per participant per take

- **`riverside-fm-cli vod <sessionId> <participantHandle>`** - Get the HLS m3u8 manifest for a participant's video. Used to stream or transcode.


### bulk


- **`riverside-fm-cli bulk export`** - Walk every project and take in a studio (or date range) and save each take's transcript (JSON and text), asset list and HLS manifests, with a resume cursor in `<out>/.resume.json`. It downloads no audio or video: use `grab` for those. The killer workflow no Pro-tier tool offers.


### clips


- **`riverside-fm-cli clips list`** - List a project's 'Made for you' Magic Clips with Riverside's own AI rating (calculated_score 0-100), the raw model score, and each clip's start/end inside the take.

- **`riverside-fm-cli clips harvest`** - Gates on AI-generation-status=ready, lists the project's Magic Clips with AI scores + take in/out points, refreshes any rendered clip's signed URL, optionally downloads MP4s.


### grab


- **`riverside-fm-cli grab`** - Get whichever exists for a recording in priority order: transcript first, then audio tracks, then HLS video  -  your stated goal as a single command.


### media


- **`riverside-fm-cli media refresh`** - Re-walks production media + clip-assets to refresh short-lived CloudFront signed URLs; --prefetch downloads every asset body before TTL expires.


### ready


- **`riverside-fm-cli ready`** - List every take across all your synced studios that is fully ready: cloud backup done, transcription finished, no participant track still uploading.


### search


- **`riverside-fm-cli search`** - SQLite FTS5 over locally-cached transcription bodies; speaker filter joins the speakers array; output names the session, project, matched line, and timestamp.


### stale


- **`riverside-fm-cli stale`** - Cross-studio query: which recordings have been stuck in uploading / processing / transcribing past N hours? Catches dropped guest-wifi uploads and abandoned takes.


### transcripts


- **`riverside-fm-cli transcripts convert`** - Convert Riverside's editableWithVoiceActivity JSON (speakers + voice-activity timestamps) to VTT, SRT, plain text, JSON, or speaker-grouped Markdown  -  formats Riverside's own UI doesn't expose.

- **`riverside-fm-cli transcripts talktime`** - From the cached voice-activity timestamps, compute seconds spoken per speaker, % of total, longest monologue, and interrupt count.


### wait


- **`riverside-fm-cli wait`** - Block until a take's backup, transcription, and/or AI generation are done; --include selects which facets to wait on; --timeout caps the wait. Exits 0 on ready, 2 on timeout.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`riverside-fm-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`riverside-fm-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`riverside-fm-cli learnings list`** - Inspect taught rows
- **`riverside-fm-cli learnings forget <query>`** - Undo a teach
- **`riverside-fm-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`riverside-fm-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`riverside-fm-cli teach-pattern`** - Install a query/resource template up front
- **`riverside-fm-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `RIVERSIDE_FM_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `riverside-fm-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
riverside-fm-cli clips get mock-value

# JSON for scripting and agents
riverside-fm-cli clips get mock-value --json
# Filter to specific fields by name
riverside-fm-cli clips get mock-value --json --select <field>[,<field>...]

# Dry run  -  show the request without sending
riverside-fm-cli clips get mock-value --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
riverside-fm-cli clips get mock-value --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
riverside-fm-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `riverside-fm-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/riverside-fm-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\riverside-fm-cli\config.toml` on Windows; `--home`, `RIVERSIDE_FM_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `RIVERSIDE_FM_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `RIVERSIDE_FM_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

### Keeping credentials off disk (optional)

By default the connector writes the browser session cookie back to the config
file after every response, because Riverside rotates `riverside_auth_access` on
each reply (a sliding session) and the next run needs the freshest value. Set
`RIVERSIDE_FM_NO_CONFIG_WRITE=1` to turn that off: the rotated cookie is kept in
memory for the rest of the run and nothing is written. Use it when the config
file is managed by something else (agentcookie, a secrets launcher that
materialises it at process start) and you do not want the CLI rewriting a
plaintext copy on disk. There is no environment variable that carries the
cookie, so the switch does not remove the need for a config file; it only stops
the CLI updating it.

It gates only credential writes. It does not change any other write the CLI makes
(the local SQLite store, the response cache, receipts), and it does not erase a
config file that already exists: an existing file is still read, just never
updated. Remove the file yourself, or run `auth logout`, if you want it gone.

| What changes | With `RIVERSIDE_FM_NO_CONFIG_WRITE=1` |
| --- | --- |
| Session cookie cache | Not written. The cookie Riverside rotates on every response is kept in memory for the rest of the run only; the next run starts again from the cookies in the config file. |
| `auth login --chrome` | Refuses, naming the variable, instead of reporting a save that did not happen. |
| `auth logout` | Still clears an existing config file. The erase is not a credential write. |
| MCP server | Honours the same variable, so a Claude Desktop install gets no plaintext token cache either. |

Set it in the shell for CLI use, or through the install prompt of the same name
for the MCP server. Any value other than blank, `0`, `false`, `no` or `off`
turns it on. See issue #270.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `riverside-fm-cli doctor` to check credentials
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **401 'User is not authenticated'**  -  Cookies expired  -  re-run `auth login --chrome` after reopening riverside.com in Chrome
- **403 on download URL after a successful list**  -  CloudFront signed URL TTL expired  -  run `media refresh --project <id>` to fetch fresh signed URLs
- **Transcription returns empty speakers[]**  -  Take is too short to transcribe or transcription is still processing  -  run `wait <session-id> --include transcript`
- **Business API 401 "The Api Key is required"**  -  This CLI uses the cookie-authed riverside.com surface, not the Business API. You don't need an API key. If you see this, you're hitting platform.riverside.fm by mistake  -  file a bug.

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

TLS certificates are verified by default. For a trusted development or self-signed endpoint only, pass `--insecure` for one invocation, set `RIVERSIDE_FM_SKIP_TLS_VERIFY=true` for the current environment, or set `skip_tls_verify = true` in the config file for a persistent override. `--insecure` overrides `--ca-cert` and `--tls-server-name`.

## Discovery Signals

This CLI was generated with browser-captured traffic analysis.
- Target observed: https://riverside.com/dashboard
- Capture coverage: 76 API entries from 76 total network entries
- Reachability: browser_http (78% confidence)
- Protocols: rest_json (75% confidence)
- Auth signals: cookie_session
- Protection signals: cloudflare (90% confidence)
- Generation hints: browser_http_transport, requires_protected_client
- Candidate command ideas: create_logs  -  Derived from observed POST /api/logs traffic.; create_migrate  -  Derived from observed POST /api/v4/global-search/migrate traffic.; get_assets  -  Derived from observed GET /api/v4/take/{uuid}/assets traffic.; get_clip_assets  -  Derived from observed GET /api/v4/take/{uuid}/clip/69fcda9fba030a19ae93a63c/clip-assets traffic.; get_damienstevens_fqd4a6ckh  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-fqd4a6ckh traffic.; get_damienstevens_lmb0ml5mk  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-lmb0ml5mk traffic.; get_damienstevens_ms1to36g5  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-ms1to36g5 traffic.; get_damienstevens_nev5txur8  -  Derived from observed GET /api/v4/vod/{uuid}/damienstevens-nev5txur8 traffic.

Warnings from discovery:
- empty_payload: API-looking request returned an empty or null payload; schema confidence is weak.
- error_status_cluster: Endpoint cluster only observed error HTTP statuses.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PodInvite/riverside-api-docs**](https://github.com/PodInvite/riverside-api-docs)  -  YAML
- [**dlt source: riverside**](https://dlthub.com/context/source/riverside)  -  Python
- [**Make.com Riverside integration**](https://apps.make.com/riverside-goh4mb)  -  JavaScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
