# Cork MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not put this
> connector behind the `supergateway` bridge or any other stdio-to-HTTP bridge,
> not even on localhost. The bridge adds no password, so anyone who can reach
> its port can use the MCP tools with your credentials. Use the default stdio
> transport. If you set up a bridge or a tunnel for a remote agent, shut it
> down. A fix is on the way.

This page wires the Cork MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `cork-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need a stdio-to-HTTP bridge, which is paused for this
connector until it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `cork-cli`
and `cork-mcp` on your PATH. `cork-mcp` is what the agents talk to.

```bash
cork-mcp --help
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
    "cork": {
      "command": "cork-mcp",
      "env": {
        "CORK_API_KEY": "<your-cork_api_token>",
        "CORK_BASE_URL": "https://api.corkinc.com/api/v1",
        "CORK_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "cork": {
      "type": "stdio",
      "command": "cork-mcp",
      "env": {
        "CORK_API_KEY": "<your-cork_api_token>",
        "CORK_BASE_URL": "https://api.corkinc.com/api/v1",
        "CORK_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
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
    "cork": {
      "command": "cork-mcp",
      "env": {
        "CORK_API_KEY": "<your-cork_api_token>",
        "CORK_BASE_URL": "https://api.corkinc.com/api/v1",
        "CORK_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Cork tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS. `cork-mcp` speaks stdio only, so that would mean
putting a stdio-to-HTTP bridge such as `supergateway` in front of it. Do not do
that with this build: the bridge adds no password, so anyone who can reach its
port can use the MCP tools with your credentials (see the security note at the
top of this page). Do not put it behind a tunnel or reverse proxy either. Use a
local agent above, or the Claude Code / Codex Skill, until the fix release
brings these steps back.

The bridge launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# CORK_API_KEY=<value> npx -y supergateway --stdio "cork-mcp" --port 7777
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add cork -- cork-mcp

# OpenClaw
openclaw mcp set cork '{"command":"cork-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `cork-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
