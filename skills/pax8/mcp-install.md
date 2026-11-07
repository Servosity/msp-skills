# Pax8 MCP - install for every agent that speaks MCP

This page wires the Pax8 MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `pax8-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Web-only AI apps** below.

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
        "PAX8_CLIENT_ID": "<your-pax8_client_id>",
        "PAX8_CLIENT_SECRET": "<your-pax8_client_secret>",
        "PAX8_BASE_URL": "https://api.pax8.com/v1",
        "PAX8_OAUTH_SCOPE": "<your-pax8_oauth_scope>",
        "PAX8_TOKEN_URL": "https://token-manager.pax8.com/oauth/token",
        "PAX8_NO_CONFIG_WRITE": "<your-pax8_no_config_write>",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "PAX8_MCP_READ_ONLY": "",
        "PAX8_NO_LOCAL_DATA": "",
        "PAX8_USER_AGENT": "",
        "PAX8_MCP_HTTP_TOKEN": ""
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
**MCP servers**, add a server, choose **STDIO**, and give `pax8-mcp` as the
command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a `[mcp_servers.pax8]`
table with `command = "pax8-mcp"` and an `env` table); the desktop app reads that
file too.

ChatGPT on the web is a different case: see **Web-only AI apps** below.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.vscode/mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "pax8": {
      "type": "stdio",
      "command": "pax8-mcp",
      "env": {
        "PAX8_AUDIENCE": "api://p8p.client",
        "PAX8_CLIENT_ID": "<your-pax8_client_id>",
        "PAX8_CLIENT_SECRET": "<your-pax8_client_secret>",
        "PAX8_BASE_URL": "https://api.pax8.com/v1",
        "PAX8_OAUTH_SCOPE": "<your-pax8_oauth_scope>",
        "PAX8_TOKEN_URL": "https://token-manager.pax8.com/oauth/token",
        "PAX8_NO_CONFIG_WRITE": "<your-pax8_no_config_write>",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "PAX8_MCP_READ_ONLY": "",
        "PAX8_NO_LOCAL_DATA": "",
        "PAX8_USER_AGENT": "",
        "PAX8_MCP_HTTP_TOKEN": ""
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
        "PAX8_CLIENT_ID": "<your-pax8_client_id>",
        "PAX8_CLIENT_SECRET": "<your-pax8_client_secret>",
        "PAX8_BASE_URL": "https://api.pax8.com/v1",
        "PAX8_OAUTH_SCOPE": "<your-pax8_oauth_scope>",
        "PAX8_TOKEN_URL": "https://token-manager.pax8.com/oauth/token",
        "PAX8_NO_CONFIG_WRITE": "<your-pax8_no_config_write>",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "PAX8_MCP_READ_ONLY": "",
        "PAX8_NO_LOCAL_DATA": "",
        "PAX8_USER_AGENT": "",
        "PAX8_MCP_HTTP_TOKEN": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Pax8 tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the web-only section below.)

---

# Web-only AI apps (not supported yet)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they need a remote server. If your team works in one of those
apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `pax8-mcp` locally (see above).

## HTTP mode in this build (reference)

This build's HTTP mode is documented here for reference; msp-skills does not
support a web-only AI app on it yet. Keep stdio (the default) for every local
agent above. HTTP mode serves `http://127.0.0.1:7777/mcp` by default and refuses
to start until `PAX8_MCP_HTTP_TOKEN` is set; every request must send
`Authorization: Bearer <token>`. Any address other than loopback also needs TLS:

```bash
export PAX8_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
PAX8_AUDIENCE=<value> PAX8_CLIENT_ID=<value> PAX8_CLIENT_SECRET=<value> PAX8_BASE_URL=<value> PAX8_OAUTH_SCOPE=<value> PAX8_TOKEN_URL=<value> PAX8_NO_CONFIG_WRITE=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> PAX8_MCP_READ_ONLY=<value> PAX8_NO_LOCAL_DATA=<value> PAX8_USER_AGENT=<value> pax8-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so every client must send that header. **Treat
the token like a password** - whoever holds it can call every tool with your
Pax8 credentials.

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
