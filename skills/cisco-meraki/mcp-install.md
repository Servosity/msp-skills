# Cisco Meraki MCP - install for every agent that speaks MCP

This page wires the Cisco Meraki MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `cisco-meraki-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Web-only AI apps** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `cisco-meraki-cli`
and `cisco-meraki-mcp` on your PATH. `cisco-meraki-mcp` is what the agents talk to.

```bash
cisco-meraki-mcp --help
```

---

# Local agents (launch the binary directly)

## Claude Desktop

Edit your Claude Desktop config:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add (or merge with your existing `mcpServers` block). The API key comes from Meraki
Dashboard > My profile > API access. Every other variable is optional and can stay
blank: set `CISCO_MERAKI_BASE_URL` only for a regional dashboard
(`https://api.meraki.ca/api/v1` Canada, `https://api.meraki.cn/api/v1` China,
`https://api.meraki.in/api/v1` India, `https://api.gov-meraki.com/api/v1` FedRAMP), and
`CISCO_MERAKI_MCP_READ_ONLY` is preset to `1`, which makes the server refuse every write.
Set `CISCO_MERAKI_NO_LOCAL_DATA` to `1` to keep no local copy of API responses. The same
applies to the configs below.

```json
{
  "mcpServers": {
    "cisco-meraki": {
      "command": "cisco-meraki-mcp",
      "env": {
        "MERAKI_DASHBOARD_API_KEY": "<your-meraki-dashboard-api-key>",
        "CISCO_MERAKI_BASE_URL": "",
        "CISCO_MERAKI_MCP_READ_ONLY": "1",
        "CISCO_MERAKI_NO_LOCAL_DATA": "",
        "CISCO_MERAKI_USER_AGENT": "",
        "CISCO_MERAKI_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

`CISCO_MERAKI_MCP_READ_ONLY` is `1` in every JSON config on this page. Remove
this line only when you want the agent to make changes.

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## ChatGPT desktop app (OpenAI)

The ChatGPT desktop app runs MCP servers locally (STDIO) and shares MCP
configuration with Codex on the same machine
([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). Open **Settings**, select
**MCP servers**, add a server, choose **STDIO**, and give `cisco-meraki-mcp` as
the command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a
`[mcp_servers.cisco-meraki]` table with `command = "cisco-meraki-mcp"` and an
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
    "cisco-meraki": {
      "type": "stdio",
      "command": "cisco-meraki-mcp",
      "env": {
        "MERAKI_DASHBOARD_API_KEY": "<your-meraki-dashboard-api-key>",
        "CISCO_MERAKI_BASE_URL": "",
        "CISCO_MERAKI_MCP_READ_ONLY": "1",
        "CISCO_MERAKI_NO_LOCAL_DATA": "",
        "CISCO_MERAKI_USER_AGENT": "",
        "CISCO_MERAKI_MCP_HTTP_TOKEN": "",
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
    "cisco-meraki": {
      "command": "cisco-meraki-mcp",
      "env": {
        "MERAKI_DASHBOARD_API_KEY": "<your-meraki-dashboard-api-key>",
        "CISCO_MERAKI_BASE_URL": "",
        "CISCO_MERAKI_MCP_READ_ONLY": "1",
        "CISCO_MERAKI_NO_LOCAL_DATA": "",
        "CISCO_MERAKI_USER_AGENT": "",
        "CISCO_MERAKI_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Cisco Meraki tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Web-only AI apps (not supported yet)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they need a remote server. If your team works in one of those
apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `cisco-meraki-mcp` locally (see above).

## HTTP mode in this build (reference)

This build's HTTP mode is documented here for reference; msp-skills does not
support a web-only AI app on it yet. Keep stdio (the default) for every local
agent above. HTTP mode serves `http://127.0.0.1:7777/mcp` by default and refuses
to start until `CISCO_MERAKI_MCP_HTTP_TOKEN` is set; every request must send
`Authorization: Bearer <token>`. Any address other than loopback also needs TLS:

```bash
export CISCO_MERAKI_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
MERAKI_DASHBOARD_API_KEY=<value> CISCO_MERAKI_MCP_READ_ONLY=1 cisco-meraki-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so every client must send that header. **Treat
the token like a password** - whoever holds it can call every tool with your Cisco
Meraki credentials.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add cisco-meraki -- cisco-meraki-mcp

# OpenClaw
openclaw mcp set cisco-meraki '{"command":"cisco-meraki-mcp"}'
```

Same env vars as the blocks above, including `CISCO_MERAKI_MCP_READ_ONLY=1`. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `cisco-meraki-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
