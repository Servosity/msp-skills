# SkyKick MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build has no password,
> listens on every network interface, and predates the browser-rebinding guard,
> so a web page you open can reach it. The MCP tools can also make changes with
> your credentials. Use the default stdio transport. If you set up a tunnel for
> a remote agent, shut it down. A fix is on the way.

This page wires the SkyKick MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `skykick-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `skykick-cli`
and `skykick-mcp` on your PATH. `skykick-mcp` is what the agents talk to.

```bash
skykick-mcp --help
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
    "skykick": {
      "command": "skykick-mcp",
      "env": {
        "SKYKICK_BASE_URL": "https://apis.cloudservices.connectwise.com",
        "SKYKICK_CLIENT_ID": "<your-skykick_client_id>",
        "SKYKICK_CLIENT_SECRET": "<your-skykick_client_secret>",
        "SKYKICK_OAUTH_SCOPE": "<your-skykick_oauth_scope>",
        "SKYKICK_TOKEN_URL": "https://apis.cloudservices.connectwise.com/auth/token",
        "SKYKICK_NO_CONFIG_WRITE": ""
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
    "skykick": {
      "type": "stdio",
      "command": "skykick-mcp",
      "env": {
        "SKYKICK_BASE_URL": "https://apis.cloudservices.connectwise.com",
        "SKYKICK_CLIENT_ID": "<your-skykick_client_id>",
        "SKYKICK_CLIENT_SECRET": "<your-skykick_client_secret>",
        "SKYKICK_OAUTH_SCOPE": "<your-skykick_oauth_scope>",
        "SKYKICK_TOKEN_URL": "https://apis.cloudservices.connectwise.com/auth/token",
        "SKYKICK_NO_CONFIG_WRITE": ""
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
    "skykick": {
      "command": "skykick-mcp",
      "env": {
        "SKYKICK_BASE_URL": "https://apis.cloudservices.connectwise.com",
        "SKYKICK_CLIENT_ID": "<your-skykick_client_id>",
        "SKYKICK_CLIENT_SECRET": "<your-skykick_client_secret>",
        "SKYKICK_OAUTH_SCOPE": "<your-skykick_oauth_scope>",
        "SKYKICK_TOKEN_URL": "https://apis.cloudservices.connectwise.com/auth/token",
        "SKYKICK_NO_CONFIG_WRITE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the SkyKick tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `skykick-mcp` in HTTP mode. Do
not do that with this build (see the security note at the top of this page), and
do not put this server behind a tunnel or reverse proxy. Use a local agent
above, or the Claude Code / Codex Skill, until the fix release brings these
steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# SKYKICK_BASE_URL=<value> SKYKICK_CLIENT_ID=<value> SKYKICK_CLIENT_SECRET=<value> SKYKICK_OAUTH_SCOPE=<value> SKYKICK_TOKEN_URL=<value> skykick-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add skykick -- skykick-mcp

# OpenClaw
openclaw mcp set skykick '{"command":"skykick-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `skykick-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
