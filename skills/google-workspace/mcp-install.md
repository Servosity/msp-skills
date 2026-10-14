# Google Workspace MCP - install for every agent that speaks MCP

This page wires the Google Workspace MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `google-workspace-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `google-workspace-mcp` over HTTPS first.

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

Every variable in the `env` blocks below is optional and can stay blank:

| Variable | Set it when |
| --- | --- |
| `GOOGLE_WORKSPACE_TENANT` | More than one tenant is registered. The raw API execute tool and the `sql`, `search`, and `context` tools take no tenant argument and use this one; without it `sql` and `search` refuse to answer. Use a name from `tenants list`. |
| `GOOGLE_WORKSPACE_MCP_READ_ONLY` | You want a read-only server: exactly `1` hides `offboard`, `sync`, `import`, `export`, and `tenants remove`, and refuses every non-GET API call. `true` or `yes` are ignored. |
| `GOOGLE_WORKSPACE_TOKEN` | You manage ONE organization without registering tenants: an OAuth access token with Admin SDK scopes. `ADMIN_ACCESS_TOKEN` is an alternate name for it. Leave it blank once tenants are registered, or untargeted API calls act on its organization. |
| `GOOGLE_WORKSPACE_MCP_HTTP_TOKEN` | You run the server with `--transport http` (the remote section below). |
| `GOOGLE_WORKSPACE_BASE_URL`, `GOOGLE_WORKSPACE_USER_AGENT`, `PRINTING_PRESS_CLIENT_PROFILE` | Testing against a mock server, overriding the User-Agent, or binding a saved client profile. Leave blank. |

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
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "",
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
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "",
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
        "GOOGLE_WORKSPACE_MCP_READ_ONLY": "",
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

# Remote agents (expose the binary over HTTPS first)

All remote agents need `google-workspace-mcp` reachable as a public **HTTPS** endpoint. Run it
in HTTP mode (it listens on 127.0.0.1:7777 by default and refuses a non-loopback bind
without TLS, so do not pass `--addr :7777`). HTTP mode will not start without
`GOOGLE_WORKSPACE_MCP_HTTP_TOKEN`, and every caller must send it as an
`Authorization: Bearer` header. `GOOGLE_WORKSPACE_MCP_READ_ONLY=1` is strongly
recommended for anything reachable from outside your machine:

```bash
GOOGLE_WORKSPACE_MCP_HTTP_TOKEN=<a long random secret> GOOGLE_WORKSPACE_MCP_READ_ONLY=1 google-workspace-mcp --transport http
```

Then expose `http://localhost:7777/mcp` as a public HTTPS URL via a secure tunnel
(Cloudflare Tunnel, ngrok) or your own reverse proxy. The path is part of the
endpoint: the server answers Streamable HTTP at `/mcp` and returns 404 at the
bare root, so a connector pointed at the root URL never handshakes. **Treat that
URL as sensitive** - it's a key to your MCP server. Never expose it bare on the
internet; gate it behind SSO / Cloudflare Access for team use.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector
pointing at your tunnel's HTTPS URL.

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`google-workspace-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `google-workspace-mcp` is hosted over HTTPS (above), the lowest-code route:

1. In **Copilot Studio**, open your agent > **Tools** > **Add a tool** > **Model
   Context Protocol**.
2. Enter a **Server name**, the **Server URL** (your HTTPS endpoint), and auth
   (OAuth 2.0 or API key). Copilot Studio builds the Power Platform connector behind
   the scenes; generative orchestration must be **on**.
3. Publish the agent into Microsoft 365 Copilot.

Alternative (dev-ish): build a **declarative agent** with the Microsoft 365 Agents
Toolkit in VS Code (**Add an Action > Start with an MCP Server**, point at the remote
URL), then sideload - requires admin-enabled Custom App Upload + Copilot Access.

Microsoft docs: https://learn.microsoft.com/en-us/microsoft-copilot-studio/agent-extend-action-mcp

## Gemini app / web (Google)

Same remote pattern as ChatGPT - point Gemini's connector at your hosted HTTPS
endpoint. For a local, no-hosting path on Google, use **Gemini CLI** (above) instead.

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

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `google-workspace-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
