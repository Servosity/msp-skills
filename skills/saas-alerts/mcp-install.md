# SaaS Alerts MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

This page wires the SaaS Alerts MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `saas-alerts-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `saas-alerts-cli`
and `saas-alerts-mcp` on your PATH. `saas-alerts-mcp` is what the agents talk to.

```bash
saas-alerts-mcp --help
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
    "saas-alerts": {
      "command": "saas-alerts-mcp",
      "env": {
        "SAAS_ALERTS_BASE_URL": "",
        "SAAS_ALERTS_API_KEY": "<your-saas_alerts_api_key>",
        "SAAS_ALERTS_USER_AGENT": "",
        "SAAS_ALERTS_MCP_READ_ONLY": "1",
        "SAAS_ALERTS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

`SAAS_ALERTS_MCP_READ_ONLY` is `1` in every JSON config on this page. Remove
this line only when you want the agent to make changes.

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

`SAAS_ALERTS_API_KEY` is the partner API key from **Settings** in the SaaS Alerts
console. The server sends it as the `api_key` header to the Reports, Manage and
Webhooks APIs and exchanges it for the Respond access token itself, so there is no
second credential to paste. `SAAS_ALERTS_BASE_URL` is optional: leave it empty (or
remove the line) to use the SaaS Alerts API host. For an agent that should only read,
keep the `"SAAS_ALERTS_MCP_READ_ONLY"` entry that is already in the `env` block at `"1"`
(here and in any other agent's config below; do not add a second entry, because JSON
keeps the last one): `saas-alerts_execute` then runs GET endpoints only, `contain`,
`rule_triggers_decide` and `subscriptions_health` can only preview (their `confirm`
parameter is refused), and `sync`, `import` and `export` are not offered. The one-click
`.mcpb` extension asks for the same switch as **Read-only MCP server**: enter `1` there.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.vscode/mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "saas-alerts": {
      "type": "stdio",
      "command": "saas-alerts-mcp",
      "env": {
        "SAAS_ALERTS_BASE_URL": "",
        "SAAS_ALERTS_API_KEY": "<your-saas_alerts_api_key>",
        "SAAS_ALERTS_USER_AGENT": "",
        "SAAS_ALERTS_MCP_READ_ONLY": "1",
        "SAAS_ALERTS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Then open Copilot Chat and switch the mode dropdown to **Agent** - MCP tools are
invisible in Ask/Edit mode.

## Gemini CLI (Google)

Edit `~/.gemini/settings.json` (Windows: `%USERPROFILE%\.gemini\settings.json`; Gemini CLI's config) and add the same shape as
Claude Desktop:

```json
{
  "mcpServers": {
    "saas-alerts": {
      "command": "saas-alerts-mcp",
      "env": {
        "SAAS_ALERTS_BASE_URL": "",
        "SAAS_ALERTS_API_KEY": "<your-saas_alerts_api_key>",
        "SAAS_ALERTS_USER_AGENT": "",
        "SAAS_ALERTS_MCP_READ_ONLY": "1",
        "SAAS_ALERTS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the SaaS Alerts tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `saas-alerts-mcp` in HTTP mode.
Do not do that with this build (see the security note at the top of this page),
and do not put this server behind a tunnel or reverse proxy. Use a local agent
above, or the Claude Code / Codex Skill, until the fix release brings these
steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# SAAS_ALERTS_API_KEY=<value> SAAS_ALERTS_MCP_HTTP_TOKEN=<long-random-value> saas-alerts-mcp --transport http
```

```powershell
# $env:SAAS_ALERTS_API_KEY="<value>"; $env:SAAS_ALERTS_MCP_HTTP_TOKEN="<long-random-value>"; saas-alerts-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add saas-alerts -- saas-alerts-mcp

# OpenClaw
openclaw mcp set saas-alerts '{"command":"saas-alerts-mcp"}'
```

Same env vars as the blocks above, including `SAAS_ALERTS_MCP_READ_ONLY=1`. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `saas-alerts-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
