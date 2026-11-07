# AppDirect MCP - install for every agent that speaks MCP

This page wires the AppDirect MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `appdirect-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Web-only AI apps** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `appdirect-cli`
and `appdirect-mcp` on your PATH. `appdirect-mcp` is what the agents talk to.

```bash
appdirect-mcp --help
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
    "appdirect": {
      "command": "appdirect-mcp",
      "env": {
        "APPDIRECT_BASE_URL": "https://marketplace.appdirect.com/api",
        "APPDIRECT_CLIENT_ID": "<your-appdirect_client_id>",
        "APPDIRECT_CLIENT_SECRET": "<your-appdirect_client_secret>",
        "APPDIRECT_OAUTH_SCOPE": "<your-appdirect_oauth_scope>",
        "APPDIRECT_TOKEN_URL": "https://marketplace.appdirect.com/oauth2/token",
        "APPDIRECT_NO_CONFIG_WRITE": "",
        "APPDIRECT_MCP_READ_ONLY": "",
        "APPDIRECT_NO_LOCAL_DATA": "",
        "APPDIRECT_USER_AGENT": "",
        "APPDIRECT_MCP_UPLOAD_ROOT": "",
        "APPDIRECT_MCP_HTTP_TOKEN": "",
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
**MCP servers**, add a server, choose **STDIO**, and give `appdirect-mcp` as the
command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a `[mcp_servers.appdirect]`
table with `command = "appdirect-mcp"` and an `env` table); the desktop app reads
that file too.

ChatGPT on the web is a different case: see **Web-only AI apps** below.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "appdirect": {
      "type": "stdio",
      "command": "appdirect-mcp",
      "env": {
        "APPDIRECT_BASE_URL": "https://marketplace.appdirect.com/api",
        "APPDIRECT_CLIENT_ID": "<your-appdirect_client_id>",
        "APPDIRECT_CLIENT_SECRET": "<your-appdirect_client_secret>",
        "APPDIRECT_OAUTH_SCOPE": "<your-appdirect_oauth_scope>",
        "APPDIRECT_TOKEN_URL": "https://marketplace.appdirect.com/oauth2/token",
        "APPDIRECT_NO_CONFIG_WRITE": "",
        "APPDIRECT_MCP_READ_ONLY": "",
        "APPDIRECT_NO_LOCAL_DATA": "",
        "APPDIRECT_USER_AGENT": "",
        "APPDIRECT_MCP_UPLOAD_ROOT": "",
        "APPDIRECT_MCP_HTTP_TOKEN": "",
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
    "appdirect": {
      "command": "appdirect-mcp",
      "env": {
        "APPDIRECT_BASE_URL": "https://marketplace.appdirect.com/api",
        "APPDIRECT_CLIENT_ID": "<your-appdirect_client_id>",
        "APPDIRECT_CLIENT_SECRET": "<your-appdirect_client_secret>",
        "APPDIRECT_OAUTH_SCOPE": "<your-appdirect_oauth_scope>",
        "APPDIRECT_TOKEN_URL": "https://marketplace.appdirect.com/oauth2/token",
        "APPDIRECT_NO_CONFIG_WRITE": "",
        "APPDIRECT_MCP_READ_ONLY": "",
        "APPDIRECT_NO_LOCAL_DATA": "",
        "APPDIRECT_USER_AGENT": "",
        "APPDIRECT_MCP_UPLOAD_ROOT": "",
        "APPDIRECT_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the AppDirect tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Web-only AI apps (not supported yet)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they need a remote server. If your team works in one of those
apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `appdirect-mcp` locally (see above).

## HTTP mode in this build (reference)

This build's HTTP mode is documented here for reference; msp-skills does not
support a web-only AI app on it yet. Keep stdio (the default) for every local
agent above. HTTP mode serves `http://127.0.0.1:7777/mcp` by default and refuses
to start until `APPDIRECT_MCP_HTTP_TOKEN` is set; every request must send
`Authorization: Bearer <token>`. Any address other than loopback also needs TLS:

```bash
export APPDIRECT_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
APPDIRECT_BASE_URL=<value> APPDIRECT_CLIENT_ID=<value> APPDIRECT_CLIENT_SECRET=<value> APPDIRECT_OAUTH_SCOPE=<value> APPDIRECT_TOKEN_URL=<value> APPDIRECT_NO_CONFIG_WRITE=<value> APPDIRECT_MCP_READ_ONLY=<value> APPDIRECT_NO_LOCAL_DATA=<value> APPDIRECT_USER_AGENT=<value> APPDIRECT_MCP_UPLOAD_ROOT=<value> APPDIRECT_MCP_HTTP_TOKEN=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> appdirect-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp`. The server rejects any request without
`Authorization: Bearer <token>`, so every client must send that header. **Treat
the token like a password** - whoever holds it can call every tool with your AppDirect
credentials.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add appdirect -- appdirect-mcp

# OpenClaw
openclaw mcp set appdirect '{"command":"appdirect-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `appdirect-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
