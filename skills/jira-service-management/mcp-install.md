# Jira Service Management MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

This page wires the Jira Service Management MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `jira-service-management-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `jira-service-management-cli`
and `jira-service-management-mcp` on your PATH. `jira-service-management-mcp` is what the agents talk to.

```bash
jira-service-management-mcp --help
```

**Which variables you need.** `JIRA_SERVICE_MANAGEMENT_SITE` (your site subdomain: `acme` for acme.atlassian.net), `JIRA_SERVICE_MANAGEMENT_EMAIL` and `JIRA_SERVICE_MANAGEMENT_API_TOKEN` (an Atlassian API token) are required. `JIRA_SERVICE_MANAGEMENT_CLOUD_ID` and `JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID` are only for Assets (CMDB) commands: leave them out if you do not use Assets, or run `jira-service-management-cli cmdb discover` to print them. Every other variable in the blocks below is optional; leave it out unless you need it.

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
    "jira-service-management": {
      "command": "jira-service-management-mcp",
      "env": {
        "JIRA_SERVICE_MANAGEMENT_API_TOKEN": "<your-jira_service_management_api_token>",
        "JIRA_SERVICE_MANAGEMENT_CLOUD_ID": "<your-jira_service_management_cloud_id>",
        "JIRA_SERVICE_MANAGEMENT_EMAIL": "<your-jira_service_management_email>",
        "JIRA_SERVICE_MANAGEMENT_SITE": "<your-jira_service_management_site>",
        "JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID": "<your-jira_service_management_workspace_id>",
        "JIRA_SERVICE_MANAGEMENT_BASE_URL": "<your-jira_service_management_base_url>",
        "JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY": "1",
        "JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN": "<your-jira_service_management_mcp_http_token>",
        "JIRA_SERVICE_MANAGEMENT_USER_AGENT": "<your-jira_service_management_user_agent>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>"
      }
    }
  }
}
```

`JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY` is `1` in every config on this page.
Remove this line only when you want the agent to make changes.

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.vscode/mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "jira-service-management": {
      "type": "stdio",
      "command": "jira-service-management-mcp",
      "env": {
        "JIRA_SERVICE_MANAGEMENT_API_TOKEN": "<your-jira_service_management_api_token>",
        "JIRA_SERVICE_MANAGEMENT_CLOUD_ID": "<your-jira_service_management_cloud_id>",
        "JIRA_SERVICE_MANAGEMENT_EMAIL": "<your-jira_service_management_email>",
        "JIRA_SERVICE_MANAGEMENT_SITE": "<your-jira_service_management_site>",
        "JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID": "<your-jira_service_management_workspace_id>",
        "JIRA_SERVICE_MANAGEMENT_BASE_URL": "<your-jira_service_management_base_url>",
        "JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY": "1",
        "JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN": "<your-jira_service_management_mcp_http_token>",
        "JIRA_SERVICE_MANAGEMENT_USER_AGENT": "<your-jira_service_management_user_agent>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>"
      }
    }
  }
}
```

Then open Copilot Chat and switch the mode dropdown to **Agent** - MCP tools are
invisible in Ask/Edit mode.

## Gemini CLI (Google)

Edit `~/.gemini/settings.json` (`%USERPROFILE%\.gemini\settings.json` on Windows) (Gemini CLI's config) and add the same shape as
Claude Desktop:

```json
{
  "mcpServers": {
    "jira-service-management": {
      "command": "jira-service-management-mcp",
      "env": {
        "JIRA_SERVICE_MANAGEMENT_API_TOKEN": "<your-jira_service_management_api_token>",
        "JIRA_SERVICE_MANAGEMENT_CLOUD_ID": "<your-jira_service_management_cloud_id>",
        "JIRA_SERVICE_MANAGEMENT_EMAIL": "<your-jira_service_management_email>",
        "JIRA_SERVICE_MANAGEMENT_SITE": "<your-jira_service_management_site>",
        "JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID": "<your-jira_service_management_workspace_id>",
        "JIRA_SERVICE_MANAGEMENT_BASE_URL": "<your-jira_service_management_base_url>",
        "JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY": "1",
        "JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN": "<your-jira_service_management_mcp_http_token>",
        "JIRA_SERVICE_MANAGEMENT_USER_AGENT": "<your-jira_service_management_user_agent>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>"
      }
    }
  }
}
```

Restart Gemini CLI; the Jira Service Management tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `jira-service-management-mcp`
in HTTP mode. Do not do that with this build (see the security note at the top
of this page), and do not put this server behind a tunnel or reverse proxy. Use
a local agent above, or the Claude Code / Codex Skill, until the fix release
brings these steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN=<long-random-token> JIRA_SERVICE_MANAGEMENT_SITE=<value> JIRA_SERVICE_MANAGEMENT_EMAIL=<value> JIRA_SERVICE_MANAGEMENT_API_TOKEN=<value> jira-service-management-mcp --transport http
```

```powershell
# $env:JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN="<long-random-token>"; $env:JIRA_SERVICE_MANAGEMENT_SITE="<value>"; $env:JIRA_SERVICE_MANAGEMENT_EMAIL="<value>"; $env:JIRA_SERVICE_MANAGEMENT_API_TOKEN="<value>"; jira-service-management-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add jira-service-management -- jira-service-management-mcp

# OpenClaw
openclaw mcp set jira-service-management '{"command":"jira-service-management-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `jira-service-management-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
