# ThreatLocker MCP - install for every agent that speaks MCP

This page wires the ThreatLocker MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `threatlocker-mcp` directly on your machine -
no hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio, the Gemini app) need a remote server, which msp-skills does not
support yet; see **Web-only AI apps** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `threatlocker-cli`
and `threatlocker-mcp` on your PATH. `threatlocker-mcp` is what the agents talk to.

```bash
threatlocker-mcp --help
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
    "threatlocker": {
      "command": "threatlocker-mcp",
      "env": {
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "THREATLOCKER_API_KEY": "<your-threatlocker_api_key>",
        "THREATLOCKER_BASE_URL": "https://portalapi.g.threatlocker.com/portalapi",
        "THREATLOCKER_ORG_ID": "",
        "THREATLOCKER_USER_AGENT": "",
        "THREATLOCKER_MCP_READ_ONLY": "",
        "THREATLOCKER_NO_LOCAL_DATA": "",
        "THREATLOCKER_MCP_HTTP_TOKEN": ""
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
**MCP servers**, add a server, choose **STDIO**, and give `threatlocker-mcp` as
the command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a
`[mcp_servers.threatlocker]` table with `command = "threatlocker-mcp"` and an
`env` table); the desktop app reads that file too.

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
    "threatlocker": {
      "type": "stdio",
      "command": "threatlocker-mcp",
      "env": {
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "THREATLOCKER_API_KEY": "<your-threatlocker_api_key>",
        "THREATLOCKER_BASE_URL": "https://portalapi.g.threatlocker.com/portalapi",
        "THREATLOCKER_ORG_ID": "",
        "THREATLOCKER_USER_AGENT": "",
        "THREATLOCKER_MCP_READ_ONLY": "",
        "THREATLOCKER_NO_LOCAL_DATA": "",
        "THREATLOCKER_MCP_HTTP_TOKEN": ""
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
    "threatlocker": {
      "command": "threatlocker-mcp",
      "env": {
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "THREATLOCKER_API_KEY": "<your-threatlocker_api_key>",
        "THREATLOCKER_BASE_URL": "https://portalapi.g.threatlocker.com/portalapi",
        "THREATLOCKER_ORG_ID": "",
        "THREATLOCKER_USER_AGENT": "",
        "THREATLOCKER_MCP_READ_ONLY": "",
        "THREATLOCKER_NO_LOCAL_DATA": "",
        "THREATLOCKER_MCP_HTTP_TOKEN": ""
      }
    }
  }
}
```

Restart Gemini CLI; the ThreatLocker tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Web-only AI apps (not supported yet)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they need a remote server. If your team works in one of those
apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `threatlocker-mcp` locally (see above).

## HTTP mode in this build (reference)

This build's HTTP mode is documented here for reference; msp-skills does not
support a web-only AI app on it yet. Keep stdio (the default) for every local
agent above. HTTP mode serves `http://127.0.0.1:7777/mcp` by default and refuses
to start until `THREATLOCKER_MCP_HTTP_TOKEN` is set; every request must send
`Authorization: Bearer <token>`. Any address other than loopback also needs TLS:

```bash
export THREATLOCKER_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
PRINTING_PRESS_CLIENT_PROFILE=<value> THREATLOCKER_API_KEY=<value> THREATLOCKER_BASE_URL=<value> THREATLOCKER_ORG_ID=<value> THREATLOCKER_USER_AGENT=<value> THREATLOCKER_MCP_READ_ONLY=<value> THREATLOCKER_NO_LOCAL_DATA=<value> threatlocker-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so every client must send that header. **Treat
the token like a password** - whoever holds it can call every tool with your
ThreatLocker credentials.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add threatlocker -- threatlocker-mcp

# OpenClaw
openclaw mcp set threatlocker '{"command":"threatlocker-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `threatlocker-mcp: command not found`: the install dir is not on your PATH (the
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
