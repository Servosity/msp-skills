---
name: wordpress
description: "Use when the user asks to publish, update, search, or clean up WordPress pages, posts, or media from the terminal or an AI agent - drives the WordPress REST API and keeps a local SQLite mirror for offline full-text search. Trigger phrases: `publish a wordpress page`, `create a wordpress post`, `list draft pages`, `upload media to wordpress`, `search my wordpress site`, `use wordpress`, `run wordpress`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "WordPress"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - wordpress-cli
    install:
      - kind: script
        bins: [wordpress-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.ps1
---

# WordPress CLI and MCP server

## Prerequisites: Install the CLI

This skill drives the `wordpress-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.ps1 | iex
   ```
3. Verify: `wordpress-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer places the `wordpress-cli` and `wordpress-mcp` binaries on your PATH. It does not
register anything with your agent - see [mcp-install.md](./mcp-install.md) for the
MCP wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Publish and manage WordPress pages, posts, media, and taxonomies via the REST API  -  built for AI agents authoring landing pages with on-page SEO.

## Command Reference

**categories**  -  List, read, and create post categories

- `wordpress-cli categories create`  -  Create a category
- `wordpress-cli categories get`  -  Get one category
- `wordpress-cli categories list`  -  List categories

**media**  -  Upload, list, read, update, and delete media library items (images, video, audio, PDF)

- `wordpress-cli media upload`  -  Upload an image, video, audio, or document and return its media id
- `wordpress-cli media delete`  -  Delete a media item
- `wordpress-cli media get`  -  Get one media item by id
- `wordpress-cli media list`  -  List media library items
- `wordpress-cli media update`  -  Update media metadata (title, alt text, caption)

**pages**  -  Create, read, update, delete WordPress pages  -  the primary landing-page surface

- `wordpress-cli pages create`  -  Create a page  -  pass full HTML content, optionally a blank/canvas template and SEO meta
- `wordpress-cli pages delete`  -  Delete a page (trash, or force-delete)
- `wordpress-cli pages get`  -  Get one page by id
- `wordpress-cli pages list`  -  List pages (filter by status, search, slug, parent)
- `wordpress-cli pages update`  -  Update a page (WordPress accepts POST for updates)

**posts**  -  Create, read, update, delete blog posts with categories and tags

- `wordpress-cli posts create`  -  Create a post
- `wordpress-cli posts delete`  -  Delete a post
- `wordpress-cli posts get`  -  Get one post by id
- `wordpress-cli posts list`  -  List posts
- `wordpress-cli posts update`  -  Update a post

**settings**  -  Read and update site-level settings (limited; requires admin auth)

- `wordpress-cli settings get`  -  Get site settings
- `wordpress-cli settings update`  -  Update site settings

**tags**  -  List, read, and create post tags

- `wordpress-cli tags create`  -  Create a tag
- `wordpress-cli tags get`  -  Get one tag
- `wordpress-cli tags list`  -  List tags

**users**  -  List and read users (for author assignment)

- `wordpress-cli users get`  -  Get one user by id
- `wordpress-cli users list`  -  List users


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
wordpress-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Auth Setup
Run `wordpress-cli auth setup` to print the URL and steps for getting a key (add `--launch` to open the URL). Then set:

```bash
export WORDPRESS_BASIC_AUTH="<your-key>"
```

To persist credentials, use `wordpress-cli auth set-token <token>`. Stored secrets live in `credentials.toml` under the data dir, not in `config.toml`.

Run `wordpress-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  wordpress-cli categories list --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

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

- Use `--home <dir>` for one invocation, or set `WORDPRESS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `WORDPRESS_CONFIG_DIR`, `WORDPRESS_DATA_DIR`, `WORDPRESS_STATE_DIR`, `WORDPRESS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `WORDPRESS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `wordpress-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "wordpress": {
        "command": "wordpress-mcp",
        "env": {
          "WORDPRESS_HOME": "/srv/wordpress"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `WORDPRESS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `WORDPRESS_HOME`, or `doctor` will not find credentials left under the former root.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
wordpress-cli feedback "the --since flag is inclusive but docs say exclusive"
wordpress-cli feedback --stdin < notes.txt
wordpress-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `WORDPRESS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `WORDPRESS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - for example a nightly job that always lists draft pages on one client's site.

```
wordpress-cli profile save briefing --json
wordpress-cli --profile briefing categories list
wordpress-cli profile list --json
wordpress-cli profile show briefing
wordpress-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `wordpress-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add wordpress-mcp -- wordpress-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which wordpress-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   wordpress-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `wordpress-cli <command> --help`.
