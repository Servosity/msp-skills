# QuickBooks Online MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build has no password,
> listens on every network interface, and predates the browser-rebinding guard,
> so a web page you open can reach it. The MCP tools can also make changes with
> your credentials. Use the default stdio transport. If you set up a tunnel for
> a remote agent, shut it down. A fix is on the way.

This page wires the QuickBooks Online MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `quickbooks-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `quickbooks-cli`
and `quickbooks-mcp` on your PATH. `quickbooks-mcp` is what the agents talk to.

```bash
quickbooks-mcp --help
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
    "quickbooks": {
      "command": "quickbooks-mcp",
      "env": {
        "QUICKBOOKS_ACCESS_TOKEN": "<your-quickbooks_access_token>",
        "QUICKBOOKS_BASE_URL": "",
        "QUICKBOOKS_CLIENT_ID": "",
        "QUICKBOOKS_CLIENT_SECRET": "",
        "QUICKBOOKS_ENVIRONMENT": "production",
        "QUICKBOOKS_REALM_ID": "<your-quickbooks_realm_id>",
        "QUICKBOOKS_REFRESH_TOKEN": "",
        "QUICKBOOKS_TOKEN_URL": "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"
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
    "quickbooks": {
      "type": "stdio",
      "command": "quickbooks-mcp",
      "env": {
        "QUICKBOOKS_ACCESS_TOKEN": "<your-quickbooks_access_token>",
        "QUICKBOOKS_BASE_URL": "",
        "QUICKBOOKS_CLIENT_ID": "",
        "QUICKBOOKS_CLIENT_SECRET": "",
        "QUICKBOOKS_ENVIRONMENT": "production",
        "QUICKBOOKS_REALM_ID": "<your-quickbooks_realm_id>",
        "QUICKBOOKS_REFRESH_TOKEN": "",
        "QUICKBOOKS_TOKEN_URL": "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"
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
    "quickbooks": {
      "command": "quickbooks-mcp",
      "env": {
        "QUICKBOOKS_ACCESS_TOKEN": "<your-quickbooks_access_token>",
        "QUICKBOOKS_BASE_URL": "",
        "QUICKBOOKS_CLIENT_ID": "",
        "QUICKBOOKS_CLIENT_SECRET": "",
        "QUICKBOOKS_ENVIRONMENT": "production",
        "QUICKBOOKS_REALM_ID": "<your-quickbooks_realm_id>",
        "QUICKBOOKS_REFRESH_TOKEN": "",
        "QUICKBOOKS_TOKEN_URL": "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"
      }
    }
  }
}
```

Restart Gemini CLI; the QuickBooks Online tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `quickbooks-mcp` in HTTP mode.
Do not do that with this build (see the security note at the top of this page),
and do not put this server behind a tunnel or reverse proxy. Use a local agent
above, or the Claude Code / Codex Skill, until the fix release brings these
steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# QUICKBOOKS_ACCESS_TOKEN=<value> QUICKBOOKS_BASE_URL=<value> QUICKBOOKS_CLIENT_ID=<value> QUICKBOOKS_CLIENT_SECRET=<value> QUICKBOOKS_ENVIRONMENT=<value> QUICKBOOKS_REALM_ID=<value> QUICKBOOKS_REFRESH_TOKEN=<value> QUICKBOOKS_TOKEN_URL=<value> quickbooks-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add quickbooks -- quickbooks-mcp

# OpenClaw
openclaw mcp set quickbooks '{"command":"quickbooks-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `quickbooks-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).

## Request discovery

The endpoint search tool returns `params_schema` alongside each result. Read its
`properties` and `required` fields before calling the executor. `x-location` identifies
where an input is sent; `x-wire-name`, when present, names its API parameter.
Keep JSON body key capitalization and nested objects exactly as shown. Known enum
values are included in the schema and can be used as search terms.
