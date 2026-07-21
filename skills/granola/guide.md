# Granola CLI

**Every Granola feature  -  plus offline SQLite cross-meeting search, attendee timelines, and a MEMO pipeline runner no other Granola tool has.**

granola-cli sources every meeting from Granola’s public surfaces  -  the public REST API (`public-api.granola.ai/v1`) for the meeting list, speaker-labeled transcripts, AI summaries, folders, attendees, and calendar, plus Granola’s official MCP server (`mcp.granola.ai`) for the raw private/human notes the REST API omits  -  and syncs all of it into a local SQLite store. That store powers the cross-meeting queries Granola.ai’s web app and existing community CLIs cannot answer: memo run, memo queue, attendee timeline, recipes coverage, calendar overlay, and talktime are offline data joins no per-meeting tool produces. The sealed desktop store (`cache-v6.json.enc`) is app-private on Granola v7.4x+ and is not used. Works offline once synced; agent-native JSON by default.

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).

## Install

The recommended path installs both the `granola-cli` binary and the `pp-granola` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install granola
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install granola --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install granola --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install granola --agent claude-code
npx -y @mvanhorn/printing-press-library install granola --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.5 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/productivity/granola/cmd/granola-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/granola-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-granola --force
```

Inside a Hermes chat session:

```text
/skills install mvanhorn/printing-press-library/cli-skills/pp-granola --force
```

## Install for OpenClaw

Install both the CLI binary and the focused OpenClaw skill into runtime-visible locations:

```bash
npx -y @mvanhorn/printing-press-library install granola --agent openclaw --bin-dir ~/.local/bin
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/granola-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `GRANOLA_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "granola": {
      "command": "granola-mcp",
      "env": {
        "GRANOLA_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Two surfaces. The public REST API key is the floor that powers everything; Granola’s official MCP is an optional add-on that only supplies your private notes. Neither is “one or the other”  -  the key is required, the MCP is additive.

### `GRANOLA_API_KEY`  -  public REST API (required)

Powers everything: `sync`, meetings, transcripts, summaries, folders, attendees, talk-time, search, and the MEMO pipeline’s transcript + summary. Get it from **Granola Settings → API**. A **Personal** key exposes your own notes; a **Workspace** key exposes workspace-visible notes  -  both work identically with this CLI. Set it in your environment:

```bash
export GRANOLA_API_KEY=<your-key>
```

![Granola Settings → API showing a Personal API key and a Workspace API key](docs/images/granola-api-keys-personal-vs-workspace.png)

*Granola Settings → API. Either a **Personal** key (your notes) or a **Workspace** key (workspace-visible notes) works  -  the CLI treats them identically.*

### Granola’s official MCP  -  private/human notes (optional, additive)

Granola’s REST API omits your raw private/human notes; Granola’s official MCP server (`https://mcp.granola.ai`) is the only source for them. Connecting it is optional  -  everything else works without it, and the commands that show human notes (`notes-show`, `memo`, `extract`, `export`) simply omit the human-notes section until you connect. Connect once, then verify:

```bash
granola-cli mcp-auth login     # browser OAuth (PKCE); tokens saved to the macOS Keychain, never on disk
granola-cli mcp-auth status    # is it connected?
granola-cli mcp-auth verify    # confirm private notes are reachable end-to-end
```

![Connecting Granola’s official MCP server](docs/images/granola-mcp-setup.png)

*Granola’s official MCP setup. `mcp-auth login` connects the CLI to `mcp.granola.ai` as an OAuth client so it can fetch the private notes REST omits.*

> **Two different MCP servers  -  don’t conflate them.** This CLI *exposes its own* MCP server (`granola-mcp`, the agent-native surface that mirrors its commands as tools  -  see [Use with Claude Desktop](#use-with-claude-desktop) above) **and** separately *consumes Granola’s official* MCP server (`mcp.granola.ai`, an external service run by Granola) as an OAuth client to fetch your private notes. `mcp-auth` connects the latter; it has nothing to do with the former.

> **The sealed desktop store is unused.** On Granola v7.4x+ the local store (`~/Library/Application Support/Granola/cache-v6.json.enc`) sits behind Granola’s own macOS Keychain access group and its internal API token behind the same wall, so no third-party binary can decrypt it. This CLI does not try  -  all data flows through the public REST API and the official MCP instead.

## Quick Start

```bash
# Confirm the REST key resolves and show MCP + local-store status.
granola-cli doctor --json

# Populate the local SQLite store from the public REST API
# (meeting list + folders + per-meeting transcripts/attendees;
#  also pulls private notes if Granola's MCP is connected).
granola-cli sync

# Optional: connect Granola's official MCP to add your private/human notes.
granola-cli mcp-auth login

# What’s synced but not yet MEMO’d this week.
granola-cli memo queue --since 7d --json

# Run the full MEMO pipeline on every meeting since yesterday.
granola-cli memo run --since 24h --to ~/Documents/Dev/meeting-transcripts --json

# Every meeting with Trevin in the last 60 days, oldest first, with the recipes applied per row.
granola-cli attendee timeline alice@example.com --since 60d --json --select id,title,started_at,recipes

# Meetings missing the Discovery panel  -  the Friday retro gap.
granola-cli recipes coverage --since 14d --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### MEMO pipeline
- **`memo run`**  -  Run the preflight → extract pipeline on one meeting or every new meeting since a timestamp, emitting the MEMO three-file artifact and an ndjson run-state ledger.

  _Replaces the per-meeting shell loop that drives the MEMO pipeline  -  one call, one ndjson stream, agent-readable._

  ```bash
  granola-cli memo run --since 24h --to ~/Documents/Dev/meeting-transcripts --json
  ```
- **`memo queue`**  -  List every meeting whose transcript is in the cache but whose MEMO triple is not yet on disk.

  _Answers the daily question “what’s still un-MEMO’d?” without the user opening Granola at all._

  ```bash
  granola-cli memo queue --since 7d --json
  ```

### Attendee intelligence
- **`attendee timeline`**  -  Every meeting with a given attendee, ordered oldest→newest, with title, date, folder, and recipe-applied flag per row.

  _Pre-call prep in one command; surfaces the conversation arc with a single person across months of meetings._

  ```bash
  granola-cli attendee timeline alice@example.com --since 60d --json --select id,title,started_at,folder,recipes
  ```
- **`attendee brief`**  -  Pulls the last N meetings with an attendee and stitches together their real cached notes plus real AI panel summaries  -  no synthesis.

  _Eliminates the click-each-meeting copy-paste that account leads do before every external call._

  ```bash
  granola-cli attendee brief alice@example.com --last 3 --panel action-items --json
  ```

### Folders + recipes
- **`folder stream`**  -  ndjson stream of every meeting in a Granola folder (resolved via documentLists + listRules) with notes and a named panel inlined.

  _Replaces the weekly retro workflow of opening a folder and copy-pasting each meeting’s summary into a spreadsheet._

  ```bash
  granola-cli folder stream client-foo --panel summary --json
  ```
- **`recipes coverage`**  -  Surface meetings that did NOT have a named panel template/recipe applied within a date range.

  _Friday retro question “did I run the Discovery recipe on every new-prospect call?” answered in one row per gap._

  ```bash
  granola-cli recipes coverage discovery --since 14d --json
  ```

### Transcript analytics
- **`talktime`**  -  Per-segment-source talk-time for one meeting  -  microphone (you) vs system (everyone else) in minutes.

  _Confidence column lets you grade transcript accuracy; mic vs system split is the input to “am I talking too much” retros._

  ```bash
  granola-cli talktime 196037d9 --json
  ```
- **`talktime`**  -  Lifts the per-source talk-time aggregation across N meetings since a date  -  who-talked-most over time.

  _Time-defrag retro input that no per-meeting tool can produce._

  ```bash
  granola-cli talktime --by participant --since 7d --json
  ```

### Cache-native data
- **`chat list`**  -  List and dump Granola’s AI chat threads anchored to a meeting (entities.chat_thread + entities.chat_message in the cache).

  _Recovers the AI Q&A history a user has accumulated against a meeting  -  useful when chasing what you asked about an account weeks ago._

  ```bash
  granola-cli chat list 196037d9 --json
  ```
- **`calendar overlay`**  -  Left-anti-join meetingsMetadata calendar events with documents.google_calendar_event to find calendared-but-not-recorded meetings.

  _Sarah’s Friday retro and Damien’s “what did I miss” sweep both reduce to this row-level diff._

  ```bash
  granola-cli calendar overlay --week 2026-05-11 --missed-only --json
  ```

### Pipeline hygiene
- **`duplicates scan`**  -  Hash (title, date-bucket, attendee-email-set) across the cache and a meeting-transcripts repo to surface duplicates at scale.

  _Repos accumulate near-duplicate files when meetings are re-extracted; this returns the dupe groups for cleanup._

  ```bash
  granola-cli duplicates scan --root ~/Documents/Dev/meeting-transcripts --json
  ```
- **`tiptap extract`**  -  Render documents[id].notes (TipTap JSON: headings, bullet_list, list_item, bold marks, paragraph_break) to canonical markdown instead of falling back to notes_plain.

  _The MEMO summary file’s quality is bounded by extractor fidelity; granola.py loses sub-list hierarchy and bold runs._

  ```bash
  granola-cli tiptap extract 196037d9 --as markdown
  ```

## Usage

Run `granola-cli --help` for the full command reference and flag list.

## Commands

This CLI exposes 35+ commands. Use `granola-cli --help` for the canonical tree and `granola-cli which "<capability>"` to find the right command from natural language. Grouped overview:

| Group | Commands |
|-------|----------|
| **MEMO pipeline** | `memo run`, `memo queue`, `preflight`, `extract` |
| **Meetings** | `meetings list / get / fetch-batch / delete / restore`, `show` |
| **Three streams** | `notes-show`, `panel get`, `transcript get`, `tiptap extract` |
| **Export** | `export <id> -o FILE`, `export-all --since DATE -o DIR` |
| **Cross-meeting analytics** | `attendee timeline / brief`, `folder stream`, `recipes coverage`, `talktime`, `calendar overlay`, `stats frequency / duration / attendees / calendar`, `collect`, `duplicates scan`, `chat list / get` |
| **Granola entities** | `folders`, `folder list / stream`, `recipes list / describe / coverage`, `workspaces list` |
| **Public API mirrors** | `notes list / get`, `folders` (require `GRANOLA_API_KEY`) |
| **Sync / system** | `sync`, `enrich`, `sync-api`, `mcp-auth login / status / verify / logout`, `doctor`, `auth login / status / set-token / logout`, `which`, `agent-context`, `version`, `import` |
| **GUI bridge (macOS only)** | `warm <id> <query>`  -  prints by default; `--launch` activates the Granola desktop app |

The REST + MCP model adds three commands worth calling out:

- **`sync`**  -  one command to populate the local SQLite store from the public REST API (meeting list + folders), enriching meetings/transcripts/attendees from the per-meeting detail endpoint; also pulls private notes when Granola’s MCP is connected. `--full` enriches the entire library (default: 50 most recent).
- **`enrich [--limit N | --full]`**  -  the detail/transcript/notes enrichment step on its own. `sync` runs it for you; call it directly to refresh more meetings (`--full` for the whole library).
- **`mcp-auth login | status | verify | logout`**  -  connect and inspect Granola’s official MCP for private/human notes. `verify` confirms those notes are reachable end-to-end.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
granola-cli folders

# JSON for scripting and agents
granola-cli folders --json

# Filter to specific fields
granola-cli folders --json --select id,name,status

# Dry run  -  show the request without sending
granola-cli folders --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
granola-cli folders --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default with a narrow opt-in write surface**  -  `meetings delete`, `meetings restore`, `import`, and `warm --launch` mutate state; everything else inspects, exports, syncs, or analyzes
- **Offline-friendly** - `sync` and the `meetings list --query <text>` FTS path use the local SQLite store
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Auto-Refresh

Every command auto-refreshes the local store as its first action. You do not need to run `granola-cli sync` before `meetings list`, `panel get`, or any other read.

Auto-refresh pulls from the public REST API (`public-api.granola.ai`) when `GRANOLA_API_KEY` is set  -  the meeting list and folders plus per-meeting transcript/attendee enrichment  -  and, when Granola’s official MCP is connected, your private/human notes. The sealed desktop store (`cache-v6.json.enc`) is app-private on Granola v7.4x+, so that surface is a no-op. When no API key is configured, auto-refresh is a silent no-op and the underlying command produces its own auth error.

A one-line provenance summary lands on stderr in interactive mode: `auto-refresh: api=ok (1.2s, 47 rows)`. It is suppressed under `--agent`, `--json`, `--compact`, `--quiet`, and when stderr is piped  -  so agent and CI consumers see no chatter on stdout or stderr.

Opt out with `--no-refresh` for a single command, `GRANOLA_NO_AUTO_REFRESH=1` for a shell session or CI job, or by saving a profile with `--no-refresh` (`granola-cli profile save fast --no-refresh`). The skip list (commands that never auto-refresh) is `sync`, `sync-api`, `auth`, `doctor`, `help`, `version`, `completion`, `agent-context`, `profile`, `feedback`, `which`. Run `granola-cli agent-context --json` to see the full contract as structured JSON.

Auto-refresh reads from Granola’s public REST API; it does not poke the desktop app. The freshness ceiling is whatever Granola has published to your account through the REST API and MCP.

## Health Check

```bash
granola-cli doctor
```

`doctor` verifies your setup end-to-end: it makes a real call against the public REST API to confirm `GRANOLA_API_KEY` works, reports whether Granola’s official MCP is connected (so private/human notes are reachable), and reports the sealed desktop store as **app-private** on Granola v7.4x+  -  an INFO line, not an error. It also shows local-store freshness and row counts. Add `--json` for machine-readable output or `--fail-on error` to exit non-zero on failure.

## Configuration

Config file: `~/.config/granola-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `GRANOLA_API_KEY` | per_call | Yes | Public REST API key from Granola Settings → API (Personal or Workspace). Powers `sync` and every data command. |

Granola’s official MCP (for private/human notes) is connected separately with `granola-cli mcp-auth login` (browser OAuth); its tokens live in the macOS Keychain, not in an environment variable.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `granola-cli doctor` to check credentials
- Verify the environment variable is set: `echo $GRANOLA_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **doctor says `key_unavailable`, or "can't read the desktop cache" / "encrypted store app-private"**  -  Expected on Granola v7.4x+. Granola sealed its local store behind its own macOS Keychain access group, so no third-party binary can decrypt it  -  this is not an error. Run `granola-cli sync` to populate the store from the public REST API, and `granola-cli mcp-auth login` to add your private/human notes.
- **Authentication error (exit code 4) / empty results**  -  Make sure `GRANOLA_API_KEY` is set (`echo $GRANOLA_API_KEY`); get a Personal or Workspace key from Granola Settings → API. Run `granola-cli doctor` to confirm the key reaches the REST API.
- **Human/private notes are missing from `notes-show`, `memo`, `extract`, or `export`**  -  The public REST API omits raw private notes. Connect Granola’s official MCP with `granola-cli mcp-auth login`, then `granola-cli mcp-auth verify` to confirm they’re reachable.
- **memo run --since reports duplicate_of**  -  A file with the same title-date-attendees fingerprint already exists in --to. Pick a different `--to` directory, remove the existing file, or `mv` it out of the way.
- **Transcript missing for a recent meeting**  -  Granola may not have processed it server-side yet, or it isn’t in the synced window. Run `granola-cli sync` (or `granola-cli enrich --full` to enrich the entire library), then retry.
- **stats / talktime returns empty rows**  -  The local store is empty or stale. Run `granola-cli sync` to hydrate it from the REST API (add `--full` to pull the whole library). Confirm `GRANOLA_API_KEY` is set via `granola-cli doctor`. If you bypassed auto-refresh with `--no-refresh` or `GRANOLA_NO_AUTO_REFRESH=1`, run `granola-cli sync` manually.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**granola.py**](https://github.com/dstevens/cc-skills)  -  Python
- [**GranolaMCP (pedramamini)**](https://github.com/pedramamini/GranolaMCP)  -  Python
- [**granola-mcp (chrisguillory)**](https://github.com/chrisguillory/granola-mcp)  -  Python
- [**reverse-engineering-granola-api (getprobo)**](https://github.com/getprobo/reverse-engineering-granola-api)  -  Python
- [**granola-claude-mcp (cobblehillmachine)**](https://github.com/cobblehillmachine/granola-claude-mcp)  -  Python
- [**granola-mcp (btn0s)**](https://github.com/btn0s/granola-mcp)  -  TypeScript
- [**granola-mcp-server (EoinFalconer)**](https://github.com/EoinFalconer/granola-mcp-server)  -  TypeScript
- [**granola-ai-mcp-server (maxgerlach1)**](https://github.com/maxgerlach1/granola-ai-mcp-server)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
