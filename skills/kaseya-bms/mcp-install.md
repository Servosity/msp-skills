# Kaseya BMS MCP - install for every agent that speaks MCP

This page wires the Kaseya BMS MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `kaseya-bms-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Remote agents** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `kaseya-bms-cli`
and `kaseya-bms-mcp` on your PATH. `kaseya-bms-mcp` is what the agents talk to.

```bash
kaseya-bms-mcp --help
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
    "kaseya-bms": {
      "command": "kaseya-bms-mcp",
      "env": {
        "KASEYA_BMS_BASE_URL": "https://api.bms.kaseya.com",
        "KASEYA_BMS_BEARER_AUTH": "<your-kaseya_bms_bearer_auth>",
        "KASEYA_BMS_PASSWORD": "<your-kaseya_bms_password>",
        "KASEYA_BMS_TENANT": "<your-kaseya_bms_tenant>",
        "KASEYA_BMS_TOKEN": "<your-kaseya_bms_token>",
        "KASEYA_BMS_USERNAME": "<your-kaseya_bms_username>",
        "KASEYA_BMS_MCP_READ_ONLY": "",
        "KASEYA_BMS_NO_LOCAL_DATA": "",
        "KASEYA_BMS_MCP_HTTP_TOKEN": "",
        "KASEYA_BMS_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
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
**MCP servers**, add a server, choose **STDIO**, and give `kaseya-bms-mcp` as the
command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a
`[mcp_servers.kaseya-bms]` table with `command = "kaseya-bms-mcp"` and an `env`
table); the desktop app reads that file too.

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
    "kaseya-bms": {
      "type": "stdio",
      "command": "kaseya-bms-mcp",
      "env": {
        "KASEYA_BMS_BASE_URL": "https://api.bms.kaseya.com",
        "KASEYA_BMS_BEARER_AUTH": "<your-kaseya_bms_bearer_auth>",
        "KASEYA_BMS_PASSWORD": "<your-kaseya_bms_password>",
        "KASEYA_BMS_TENANT": "<your-kaseya_bms_tenant>",
        "KASEYA_BMS_TOKEN": "<your-kaseya_bms_token>",
        "KASEYA_BMS_USERNAME": "<your-kaseya_bms_username>",
        "KASEYA_BMS_MCP_READ_ONLY": "",
        "KASEYA_BMS_NO_LOCAL_DATA": "",
        "KASEYA_BMS_MCP_HTTP_TOKEN": "",
        "KASEYA_BMS_USER_AGENT": "",
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
    "kaseya-bms": {
      "command": "kaseya-bms-mcp",
      "env": {
        "KASEYA_BMS_BASE_URL": "https://api.bms.kaseya.com",
        "KASEYA_BMS_BEARER_AUTH": "<your-kaseya_bms_bearer_auth>",
        "KASEYA_BMS_PASSWORD": "<your-kaseya_bms_password>",
        "KASEYA_BMS_TENANT": "<your-kaseya_bms_tenant>",
        "KASEYA_BMS_TOKEN": "<your-kaseya_bms_token>",
        "KASEYA_BMS_USERNAME": "<your-kaseya_bms_username>",
        "KASEYA_BMS_MCP_READ_ONLY": "",
        "KASEYA_BMS_NO_LOCAL_DATA": "",
        "KASEYA_BMS_MCP_HTTP_TOKEN": "",
        "KASEYA_BMS_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Kaseya BMS tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (not supported yet)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they would need `kaseya-bms-mcp` running in HTTP mode behind
HTTPS, which msp-skills does not set up for you. Do not put this server behind a
tunnel or reverse proxy. If your team works in one
of those apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `kaseya-bms-mcp` locally (see above).

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# KASEYA_BMS_BASE_URL=<value> KASEYA_BMS_BEARER_AUTH=<value> KASEYA_BMS_PASSWORD=<value> KASEYA_BMS_TENANT=<value> KASEYA_BMS_TOKEN=<value> KASEYA_BMS_USERNAME=<value> KASEYA_BMS_MCP_READ_ONLY=<value> KASEYA_BMS_NO_LOCAL_DATA=<value> KASEYA_BMS_USER_AGENT=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> kaseya-bms-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add kaseya-bms -- kaseya-bms-mcp

# OpenClaw
openclaw mcp set kaseya-bms '{"command":"kaseya-bms-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `kaseya-bms-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
