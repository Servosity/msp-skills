# runZero MCP - install for every agent that speaks MCP

This page wires the runZero MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `runzero-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Remote agents** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `runzero-cli`
and `runzero-mcp` on your PATH. `runzero-mcp` is what the agents talk to.

```bash
runzero-mcp --help
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
    "runzero": {
      "command": "runzero-mcp",
      "env": {
        "RUNZERO_API_KEY": "<your-runzero_api_key>",
        "RUNZERO_BASE_URL": "<your-runzero_base_url>",
        "RUNZERO_USER_AGENT": "",
        "RUNZERO_MCP_READ_ONLY": "",
        "RUNZERO_NO_LOCAL_DATA": "",
        "RUNZERO_MCP_UPLOAD_ROOT": "",
        "RUNZERO_MCP_HTTP_TOKEN": "",
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
**MCP servers**, add a server, choose **STDIO**, and give `runzero-mcp` as the
command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a `[mcp_servers.runzero]`
table with `command = "runzero-mcp"` and an `env` table); the desktop app reads
that file too.

ChatGPT on the web is a different case: see **Remote agents** below.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.vscode/mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "runzero": {
      "type": "stdio",
      "command": "runzero-mcp",
      "env": {
        "RUNZERO_API_KEY": "<your-runzero_api_key>",
        "RUNZERO_BASE_URL": "<your-runzero_base_url>",
        "RUNZERO_USER_AGENT": "",
        "RUNZERO_MCP_READ_ONLY": "",
        "RUNZERO_NO_LOCAL_DATA": "",
        "RUNZERO_MCP_UPLOAD_ROOT": "",
        "RUNZERO_MCP_HTTP_TOKEN": "",
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
    "runzero": {
      "command": "runzero-mcp",
      "env": {
        "RUNZERO_API_KEY": "<your-runzero_api_key>",
        "RUNZERO_BASE_URL": "<your-runzero_base_url>",
        "RUNZERO_USER_AGENT": "",
        "RUNZERO_MCP_READ_ONLY": "",
        "RUNZERO_NO_LOCAL_DATA": "",
        "RUNZERO_MCP_UPLOAD_ROOT": "",
        "RUNZERO_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the runZero tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they would need `runzero-mcp` hosted in HTTP mode. If your team
works in one of those apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `runzero-mcp` locally (see above).

For reference, HTTP mode serves `http://127.0.0.1:7777/mcp` by default and refuses
to start until `RUNZERO_MCP_HTTP_TOKEN` is set to 32 or more characters; every
request must send `Authorization: Bearer <token>`, and any address other than
loopback also needs `--tls-cert` and `--tls-key`. **Treat the token like a
password** - whoever holds it can call every tool with your runZero credentials.

The launch command, for reference:

```bash
export RUNZERO_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
RUNZERO_API_KEY=<value> RUNZERO_BASE_URL=<value> RUNZERO_MCP_READ_ONLY= RUNZERO_NO_LOCAL_DATA= RUNZERO_MCP_UPLOAD_ROOT= PRINTING_PRESS_CLIENT_PROFILE= RUNZERO_USER_AGENT= runzero-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add runzero -- runzero-mcp

# OpenClaw
openclaw mcp set runzero '{"command":"runzero-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `runzero-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
