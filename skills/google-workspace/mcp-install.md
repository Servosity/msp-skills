# Google Workspace MCP - install for every agent that speaks MCP

This page wires the Google Workspace MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `google-workspace-mcp` directly on your machine
- no hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio, the Gemini app) need a remote server, which msp-skills does not
support yet; see **Web-only AI apps** below.

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
| `GOOGLE_WORKSPACE_MCP_READ_ONLY` | You want a read-only server: exactly `1` hides `offboard`, `sync`, `export`, `tenants test`, `tenants remove`, `workflow archive`, the raw API execute tool, and every tool that writes the local learning store (`teach`, `teach-pattern`, `teach-lookup`, `teach-playbook`, `playbook amend`, `learnings confirm`, `learnings forget`, and `learnings reject`), so only reads reach Google. (`import` is never exposed over MCP: it reads a local file.) `true` or `yes` are ignored. |
| `GOOGLE_WORKSPACE_TOKEN` | You manage ONE organization without registering tenants: an OAuth access token with Admin SDK scopes. `ADMIN_ACCESS_TOKEN` is an alternate name for it. Not used once tenants are registered (a call with no tenant selected is refused), so leave it blank then. |
| `GOOGLE_WORKSPACE_NO_LOCAL_DATA` | You want no local copy of Google data: exactly `1` sends every read to the API, turns the response cache off, writes nothing new to the local store, and refuses `sync` and `workflow archive`. |
| `GOOGLE_WORKSPACE_MCP_HTTP_TOKEN` | You run the server with `--transport http` (the remote section below). At least 32 characters. |
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
        "GOOGLE_WORKSPACE_NO_LOCAL_DATA": "",
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

## ChatGPT desktop app (OpenAI)

The ChatGPT desktop app runs MCP servers locally (STDIO) and shares MCP
configuration with Codex on the same machine
([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). Open **Settings**, select
**MCP servers**, add a server, choose **STDIO**, and give `google-workspace-mcp`
as the command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a
`[mcp_servers.google-workspace]` table with `command = "google-workspace-mcp"` and
an `env` table); the desktop app reads that file too.

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
    "google-workspace": {
      "type": "stdio",
      "command": "google-workspace-mcp",
      "env": {
        "GOOGLE_WORKSPACE_TENANT": "",
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "1",
        "GOOGLE_WORKSPACE_NO_LOCAL_DATA": "",
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
        "GOOGLE_WORKSPACE_NO_LOCAL_DATA": "",
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
web** needs a remote server - see **Web-only AI apps** below.)

---

# Web-only AI apps (not supported yet)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they need a remote server. If your team works in one of those
apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `google-workspace-mcp` locally (see
above).

## HTTP mode in this build (reference)

This build's HTTP mode is documented here for reference; msp-skills does not
support a web-only AI app on it yet. Keep stdio (the default) for every local
agent above. HTTP mode serves
`http://127.0.0.1:7777/mcp` by default and refuses to start until
`GOOGLE_WORKSPACE_MCP_HTTP_TOKEN` is set to at least 32 characters; every request
must send `Authorization: Bearer <token>`. Any address other than loopback also
needs TLS. `GOOGLE_WORKSPACE_MCP_READ_ONLY=1` is strongly recommended for anything
reachable from outside your machine:

```bash
export GOOGLE_WORKSPACE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
GOOGLE_WORKSPACE_MCP_READ_ONLY=1 google-workspace-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

```powershell
$env:GOOGLE_WORKSPACE_MCP_HTTP_TOKEN = -join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Maximum 256) })
$env:GOOGLE_WORKSPACE_MCP_READ_ONLY = "1"
google-workspace-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`. The server rejects any request without
`Authorization: Bearer <token>`, so every client must send that header. **Treat the token like a password** - whoever
holds it can call every tool with your clients' Google Workspace delegation.

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
