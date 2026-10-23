# ConnectWise Manage MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build has no password,
> listens on every network interface, and predates the browser-rebinding guard,
> so a web page you open can reach it. The MCP tools can also make changes with
> your credentials. Use the default stdio transport. If you set up a tunnel for
> a remote agent, shut it down. A fix is on the way.

This page wires the ConnectWise Manage MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `connectwise-manage-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `connectwise-manage-cli`
and `connectwise-manage-mcp` on your PATH. `connectwise-manage-mcp` is what the agents talk to.

```bash
connectwise-manage-mcp --help
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
    "connectwise-manage": {
      "command": "connectwise-manage-mcp",
      "env": {
        "CONNECTWISE_MANAGE_BASE_URL": "https://api-na.myconnectwise.net/v4_6_release/apis/3.0",
        "CW_API_VERSION": "",
        "CW_CLIENT_ID": "<your-cw_client_id>",
        "CW_COMPANY_ID": "<your-cw_company_id>",
        "CW_PRIVATE_KEY": "<your-cw_private_key>",
        "CW_PUBLIC_KEY": "<your-cw_public_key>",
        "CW_SITE": "<your-cw_site>"
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
    "connectwise-manage": {
      "type": "stdio",
      "command": "connectwise-manage-mcp",
      "env": {
        "CONNECTWISE_MANAGE_BASE_URL": "https://api-na.myconnectwise.net/v4_6_release/apis/3.0",
        "CW_API_VERSION": "",
        "CW_CLIENT_ID": "<your-cw_client_id>",
        "CW_COMPANY_ID": "<your-cw_company_id>",
        "CW_PRIVATE_KEY": "<your-cw_private_key>",
        "CW_PUBLIC_KEY": "<your-cw_public_key>",
        "CW_SITE": "<your-cw_site>"
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
    "connectwise-manage": {
      "command": "connectwise-manage-mcp",
      "env": {
        "CONNECTWISE_MANAGE_BASE_URL": "https://api-na.myconnectwise.net/v4_6_release/apis/3.0",
        "CW_API_VERSION": "",
        "CW_CLIENT_ID": "<your-cw_client_id>",
        "CW_COMPANY_ID": "<your-cw_company_id>",
        "CW_PRIVATE_KEY": "<your-cw_private_key>",
        "CW_PUBLIC_KEY": "<your-cw_public_key>",
        "CW_SITE": "<your-cw_site>"
      }
    }
  }
}
```

Restart Gemini CLI; the ConnectWise Manage tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `connectwise-manage-mcp` in
HTTP mode. Do not do that with this build (see the security note at the top of
this page), and do not put this server behind a tunnel or reverse proxy. Use a
local agent above, or the Claude Code / Codex Skill, until the fix release
brings these steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# CONNECTWISE_MANAGE_BASE_URL=<value> CW_API_VERSION=<value> CW_CLIENT_ID=<value> CW_COMPANY_ID=<value> CW_PRIVATE_KEY=<value> CW_PUBLIC_KEY=<value> CW_SITE=<value> connectwise-manage-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add connectwise-manage -- connectwise-manage-mcp

# OpenClaw
openclaw mcp set connectwise-manage '{"command":"connectwise-manage-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `connectwise-manage-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
