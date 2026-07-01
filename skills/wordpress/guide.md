# WordPress CLI

Publish and manage WordPress pages, posts, media, and taxonomies via the REST API  -  built for AI agents authoring landing pages with on-page SEO.

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `wordpress-cli` binary and the `pp-wordpress` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install wordpress
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install wordpress --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install wordpress --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install wordpress --agent claude-code
npx -y @mvanhorn/printing-press-library install wordpress --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/marketing/wordpress/cmd/wordpress-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/wordpress-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install wordpress --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-wordpress --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-wordpress --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install wordpress --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/wordpress-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `WORDPRESS_BASIC_AUTH` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/marketing/wordpress/cmd/wordpress-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "wordpress": {
      "command": "wordpress-mcp",
      "env": {
        "WORDPRESS_BASIC_AUTH": "<your-key>"
      }
    }
  }
}
```

</details>

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Set Up Credentials

Get your API key from your API provider's developer portal. The key typically looks like a long alphanumeric string.

```bash
export WORDPRESS_BASIC_AUTH="<paste-your-key>"
```

To persist credentials, use `wordpress-cli auth set-token <token>`. Stored secrets live in `credentials.toml` under the data directory, not in `config.toml`.

### 3. Verify Setup

```bash
wordpress-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
wordpress-cli categories list
```

## Usage

Run `wordpress-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `WORDPRESS_CONFIG_DIR`, `WORDPRESS_DATA_DIR`, `WORDPRESS_STATE_DIR`, or `WORDPRESS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `WORDPRESS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export WORDPRESS_HOME=/srv/wordpress
wordpress-cli doctor
```

Under `WORDPRESS_HOME=/srv/wordpress`, the four dirs resolve to `/srv/wordpress/config`, `/srv/wordpress/data`, `/srv/wordpress/state`, and `/srv/wordpress/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `WORDPRESS_DATA_DIR` overrides an explicit `--home` for that kind. Use `WORDPRESS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `WORDPRESS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `wordpress-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### categories

List, read, and create post categories

- **`wordpress-cli categories create`** - Create a category
- **`wordpress-cli categories get`** - Get one category
- **`wordpress-cli categories list`** - List categories

### media

Upload, list, read, update, and delete media library items (images, video, audio, PDF)

- **`wordpress-cli media upload`** - Upload an image, video, audio, or document and return its media id
- **`wordpress-cli media delete`** - Delete a media item
- **`wordpress-cli media get`** - Get one media item by id
- **`wordpress-cli media list`** - List media library items
- **`wordpress-cli media update`** - Update media metadata (title, alt text, caption)

### pages

Create, read, update, delete WordPress pages  -  the primary landing-page surface

- **`wordpress-cli pages create`** - Create a page  -  pass full HTML content, optionally a blank/canvas template and SEO meta
- **`wordpress-cli pages delete`** - Delete a page (trash, or force-delete)
- **`wordpress-cli pages get`** - Get one page by id
- **`wordpress-cli pages list`** - List pages (filter by status, search, slug, parent)
- **`wordpress-cli pages update`** - Update a page (WordPress accepts POST for updates)

### posts

Create, read, update, delete blog posts with categories and tags

- **`wordpress-cli posts create`** - Create a post
- **`wordpress-cli posts delete`** - Delete a post
- **`wordpress-cli posts get`** - Get one post by id
- **`wordpress-cli posts list`** - List posts
- **`wordpress-cli posts update`** - Update a post

### settings

Read and update site-level settings (limited; requires admin auth)

- **`wordpress-cli settings get`** - Get site settings
- **`wordpress-cli settings update`** - Update site settings

### tags

List, read, and create post tags

- **`wordpress-cli tags create`** - Create a tag
- **`wordpress-cli tags get`** - Get one tag
- **`wordpress-cli tags list`** - List tags

### users

List and read users (for author assignment)

- **`wordpress-cli users get`** - Get one user by id
- **`wordpress-cli users list`** - List users


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
wordpress-cli categories list

# JSON for scripting and agents
wordpress-cli categories list --json

# Filter to specific fields
wordpress-cli categories list --json --select id,name,status

# Dry run  -  show the request without sending
wordpress-cli categories list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
wordpress-cli categories list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
wordpress-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `wordpress-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/wordpress-cli/config.toml`; `--home`, `WORDPRESS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `WORDPRESS_BASIC_AUTH` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `wordpress-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `wordpress-cli doctor` to check credentials
- Verify the environment variable is set: `echo $WORDPRESS_BASIC_AUTH`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
