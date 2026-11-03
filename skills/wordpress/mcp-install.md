# WordPress MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

This page wires the WordPress MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `wordpress-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Remote agents** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `wordpress-cli`
and `wordpress-mcp` on your PATH. `wordpress-mcp` is what the agents talk to.

```bash
wordpress-mcp --help
```

---

# Local agents (launch the binary directly)

## Claude Desktop

Edit your Claude Desktop config:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add (or merge with your existing `mcpServers` block):

```json
{
  "mcpServers": {
    "wordpress": {
      "command": "wordpress-mcp",
      "env": {
        "WORDPRESS_BASE_URL": "https://wordpress.org/news/wp-json/wp/v2",
        "WORDPRESS_BASIC_AUTH": "<your-wordpress_basic_auth>",
        "WORDPRESS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "WORDPRESS_USER_AGENT": ""
      }
    }
  }
}
```

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## ChatGPT desktop app (OpenAI)

The ChatGPT desktop app runs MCP servers locally (STDIO) and shares MCP
configuration with Codex on the same machine
([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). Open **Settings**, select
**MCP servers**, add a server, choose **STDIO**, and give `wordpress-mcp` as the
command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a `[mcp_servers.wordpress]`
table with `command = "wordpress-mcp"` and an `env` table); the desktop app reads
that file too.

ChatGPT on the web is a different case: see **Remote agents** below.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "wordpress": {
      "type": "stdio",
      "command": "wordpress-mcp",
      "env": {
        "WORDPRESS_BASE_URL": "https://wordpress.org/news/wp-json/wp/v2",
        "WORDPRESS_BASIC_AUTH": "<your-wordpress_basic_auth>",
        "WORDPRESS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "WORDPRESS_USER_AGENT": ""
      }
    }
  }
}
```

Then open Copilot Chat and switch the mode dropdown to **Agent** - MCP tools are
invisible in Ask/Edit mode.

## Gemini CLI (Google)

Edit `~/.gemini/settings.json` (Gemini CLI's config) and add the same shape as
Claude Desktop:

```json
{
  "mcpServers": {
    "wordpress": {
      "command": "wordpress-mcp",
      "env": {
        "WORDPRESS_BASE_URL": "https://wordpress.org/news/wp-json/wp/v2",
        "WORDPRESS_BASIC_AUTH": "<your-wordpress_basic_auth>",
        "WORDPRESS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "WORDPRESS_USER_AGENT": ""
      }
    }
  }
}
```

Restart Gemini CLI; the WordPress tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they would need `wordpress-mcp` running in HTTP mode. Do not
run HTTP mode with this build (see the security note at the top of this page), and
do not put this server behind a tunnel or reverse proxy. If your team works in one
of those apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `wordpress-mcp` locally (see above).

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# WORDPRESS_BASE_URL=<value> WORDPRESS_BASIC_AUTH=<value> WORDPRESS_MCP_HTTP_TOKEN=<value> wordpress-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add wordpress -- wordpress-mcp

# OpenClaw
openclaw mcp set wordpress '{"command":"wordpress-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `wordpress-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
