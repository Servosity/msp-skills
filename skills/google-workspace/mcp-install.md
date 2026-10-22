# Google Workspace MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

This page wires the Google Workspace MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `google-workspace-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `google-workspace-cli`
and `google-workspace-mcp` on your PATH. `google-workspace-mcp` is what the agents talk to.

```bash
google-workspace-mcp --help
```

## Before you wire any agent: register your clients

The MCP server reads the same tenant registry as the CLI, and `tenants add` is not
exposed over MCP. Register each client once from a terminal (full steps in
[README.md](./README.md#authenticate)), then confirm every delegation works:

```bash
google-workspace-cli tenants add acme --domain example.com --admin admin@example.com --key ./acme-sa.json
google-workspace-cli tenants test --all-tenants
```

Every variable in the `env` blocks below is optional and can stay blank, except
`GOOGLE_WORKSPACE_MCP_READ_ONLY`, which is preset to `1`:

| Variable | Set it when |
| --- | --- |
| `GOOGLE_WORKSPACE_TENANT` | More than one tenant is registered. The raw API execute tool and the `sql`, `search`, and `context` tools take no tenant argument and use this one; without it `sql` and `search` refuse to answer. Use a name from `tenants list`. |
| `GOOGLE_WORKSPACE_MCP_READ_ONLY` | You want a read-only server: exactly `1` hides `offboard`, `sync`, `export`, `tenants test`, `tenants remove`, `workflow archive`, `learnings forget`, and `learnings reject`, and refuses every non-GET API call. (`import` is never exposed over MCP: it reads a local file.) `true` or `yes` are ignored. |
| `GOOGLE_WORKSPACE_TOKEN` | You manage ONE organization without registering tenants: an OAuth access token with Admin SDK scopes. `ADMIN_ACCESS_TOKEN` is an alternate name for it. Not used once tenants are registered (a call with no tenant selected is refused), so leave it blank then. |
| `GOOGLE_WORKSPACE_MCP_HTTP_TOKEN` | You run the server with `--transport http`, which is paused for now (see the remote section below). |
| `GOOGLE_WORKSPACE_BASE_URL`, `GOOGLE_WORKSPACE_USER_AGENT`, `PRINTING_PRESS_CLIENT_PROFILE` | Testing against a mock server or overriding the User-Agent; `PRINTING_PRESS_CLIENT_PROFILE` has no effect for this connector. Leave blank. |

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
    "google-workspace": {
      "command": "google-workspace-mcp",
      "env": {
        "GOOGLE_WORKSPACE_TENANT": "",
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "1",
        "GOOGLE_WORKSPACE_TOKEN": "",
        "ADMIN_ACCESS_TOKEN": "",
        "GOOGLE_WORKSPACE_MCP_HTTP_TOKEN": "",
        "GOOGLE_WORKSPACE_BASE_URL": "",
        "GOOGLE_WORKSPACE_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

`GOOGLE_WORKSPACE_MCP_READ_ONLY` is `1` in every JSON config on this page.
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
    "google-workspace": {
      "type": "stdio",
      "command": "google-workspace-mcp",
      "env": {
        "GOOGLE_WORKSPACE_TENANT": "",
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "1",
        "GOOGLE_WORKSPACE_TOKEN": "",
        "ADMIN_ACCESS_TOKEN": "",
        "GOOGLE_WORKSPACE_MCP_HTTP_TOKEN": "",
        "GOOGLE_WORKSPACE_BASE_URL": "",
        "GOOGLE_WORKSPACE_USER_AGENT": "",
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
    "google-workspace": {
      "command": "google-workspace-mcp",
      "env": {
        "GOOGLE_WORKSPACE_TENANT": "",
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "1",
        "GOOGLE_WORKSPACE_TOKEN": "",
        "ADMIN_ACCESS_TOKEN": "",
        "GOOGLE_WORKSPACE_MCP_HTTP_TOKEN": "",
        "GOOGLE_WORKSPACE_BASE_URL": "",
        "GOOGLE_WORKSPACE_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Google Workspace tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `google-workspace-mcp` in HTTP
mode. Do not do that with this build (see the security note at the top of this
page), and do not put this server behind a tunnel or reverse proxy. Use a local
agent above, or the Claude Code / Codex Skill, until the fix release brings
these steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# GOOGLE_WORKSPACE_MCP_HTTP_TOKEN=<a long random secret> GOOGLE_WORKSPACE_MCP_READ_ONLY=1 google-workspace-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add google-workspace -- google-workspace-mcp

# OpenClaw
openclaw mcp set google-workspace '{"command":"google-workspace-mcp"}'
```

Same env vars as the blocks above, including `GOOGLE_WORKSPACE_MCP_READ_ONLY=1`. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `google-workspace-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
