# Avanan MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not put this
> connector behind the `supergateway` bridge or any other stdio-to-HTTP bridge,
> not even on localhost. The bridge adds no password, so anyone who can reach
> its port can use the MCP tools with your credentials. Use the default stdio
> transport. If you set up a bridge or a tunnel for a remote agent, shut it
> down. A fix is on the way.

This page wires the Avanan MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `avanan-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need a stdio-to-HTTP bridge, which is paused for this
connector until it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `avanan-cli`
and `avanan-mcp` on your PATH. `avanan-mcp` is what the agents talk to.

```bash
avanan-mcp --help
```

**Set your region first.** Avanan credentials are region-scoped and cross-region
reads are refused by design. The CLI defaults to the USA host. If your tenant is
anywhere else, run `avanan-cli auth login --region <us|eu|ca|ap|uk|uae|in> --save`
once before wiring up any agent, so the MCP server inherits the right host from
the saved config. `avanan-cli doctor` prints the `base_url` it resolved.

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
    "avanan": {
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-avanan-application-id>",
        "AVANAN_BASE_URL": "https://smart-api-production-1-us.avanan.net",
        "AVANAN_CLIENT_SECRET": "<your-avanan-client-secret>",
        "AVANAN_TOKEN": "<your-avanan_token>",
        "AVANAN_USER_AGENT": "",
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
    "avanan": {
      "type": "stdio",
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-avanan-application-id>",
        "AVANAN_BASE_URL": "https://smart-api-production-1-us.avanan.net",
        "AVANAN_CLIENT_SECRET": "<your-avanan-client-secret>",
        "AVANAN_TOKEN": "<your-avanan_token>",
        "AVANAN_USER_AGENT": "",
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
    "avanan": {
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-avanan-application-id>",
        "AVANAN_BASE_URL": "https://smart-api-production-1-us.avanan.net",
        "AVANAN_CLIENT_SECRET": "<your-avanan-client-secret>",
        "AVANAN_TOKEN": "<your-avanan_token>",
        "AVANAN_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Avanan tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS. `avanan-mcp` speaks stdio only, so that would mean
putting a stdio-to-HTTP bridge such as `supergateway` in front of it. Do not do
that with this build: the bridge adds no password, so anyone who can reach its
port can use the MCP tools with your credentials (see the security note at the
top of this page). Do not put it behind a tunnel or reverse proxy either. Use a
local agent above, or the Claude Code / Codex Skill, until the fix release
brings these steps back.

The bridge launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# AVANAN_APP_ID=<value> AVANAN_BASE_URL=<value> AVANAN_CLIENT_SECRET=<value> AVANAN_TOKEN=<value> AVANAN_USER_AGENT=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> npx -y supergateway --stdio "avanan-mcp" --port 7777
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add avanan -- avanan-mcp

# OpenClaw
openclaw mcp set avanan '{"command":"avanan-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `avanan-mcp: command not found`: the install dir is not on your PATH (the
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
