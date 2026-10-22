# Pax8 MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build has no password,
> listens on every network interface, and predates the browser-rebinding guard,
> so a web page you open can reach it. The MCP tools can also make changes with
> your credentials. Use the default stdio transport. If you set up a tunnel for
> a remote agent, shut it down. A fix is on the way.

This page wires the Pax8 MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `pax8-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `pax8-cli`
and `pax8-mcp` on your PATH. `pax8-mcp` is what the agents talk to.

```bash
pax8-mcp --help
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
    "pax8": {
      "command": "pax8-mcp",
      "env": {
        "PAX8_AUDIENCE": "api://p8p.client",
        "PAX8_BASE_URL": "https://api.pax8.com/v1",
        "PAX8_CLIENT_ID": "<your-pax8_client_id>",
        "PAX8_CLIENT_SECRET": "<your-pax8_client_secret>",
        "PAX8_OAUTH_SCOPE": "<your-pax8_oauth_scope>",
        "PAX8_TOKEN_URL": "https://token-manager.pax8.com/oauth/token",
        "PAX8_NO_CONFIG_WRITE": ""
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
    "pax8": {
      "type": "stdio",
      "command": "pax8-mcp",
      "env": {
        "PAX8_AUDIENCE": "api://p8p.client",
        "PAX8_BASE_URL": "https://api.pax8.com/v1",
        "PAX8_CLIENT_ID": "<your-pax8_client_id>",
        "PAX8_CLIENT_SECRET": "<your-pax8_client_secret>",
        "PAX8_OAUTH_SCOPE": "<your-pax8_oauth_scope>",
        "PAX8_TOKEN_URL": "https://token-manager.pax8.com/oauth/token",
        "PAX8_NO_CONFIG_WRITE": ""
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
    "pax8": {
      "command": "pax8-mcp",
      "env": {
        "PAX8_AUDIENCE": "api://p8p.client",
        "PAX8_BASE_URL": "https://api.pax8.com/v1",
        "PAX8_CLIENT_ID": "<your-pax8_client_id>",
        "PAX8_CLIENT_SECRET": "<your-pax8_client_secret>",
        "PAX8_OAUTH_SCOPE": "<your-pax8_oauth_scope>",
        "PAX8_TOKEN_URL": "https://token-manager.pax8.com/oauth/token",
        "PAX8_NO_CONFIG_WRITE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Pax8 tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `pax8-mcp` in HTTP mode. Do not
do that with this build (see the security note at the top of this page), and do
not put this server behind a tunnel or reverse proxy. Use a local agent above,
or the Claude Code / Codex Skill, until the fix release brings these steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# PAX8_AUDIENCE=<value> PAX8_BASE_URL=<value> PAX8_CLIENT_ID=<value> PAX8_CLIENT_SECRET=<value> PAX8_OAUTH_SCOPE=<value> PAX8_TOKEN_URL=<value> pax8-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add pax8 -- pax8-mcp

# OpenClaw
openclaw mcp set pax8 '{"command":"pax8-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `pax8-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
