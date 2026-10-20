# SaaS Alerts MCP - install for every agent that speaks MCP

This page wires the SaaS Alerts MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `saas-alerts-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `saas-alerts-mcp` over HTTPS first.

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
        "SAAS_ALERTS_MCP_READ_ONLY": "",
        "SAAS_ALERTS_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

`SAAS_ALERTS_API_KEY` is the partner API key from **Settings** in the SaaS Alerts
console. The server sends it as the `api_key` header to the Reports, Manage and
Webhooks APIs and exchanges it for the Respond access token itself, so there is no
second credential to paste. `SAAS_ALERTS_BASE_URL` is optional: leave it empty (or
remove the line) to use the SaaS Alerts API host. For an agent that should only read,
set the `"SAAS_ALERTS_MCP_READ_ONLY"` entry that is already in the `env` block to `"1"`
(here or in any other agent's config below; do not add a second entry, because JSON
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
        "SAAS_ALERTS_MCP_READ_ONLY": "",
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
        "SAAS_ALERTS_MCP_READ_ONLY": "",
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

# Remote agents (expose the binary over HTTPS first)

All remote agents need `saas-alerts-mcp` reachable as a public **HTTPS** endpoint. Run it
in HTTP mode with your credentials in the environment (it serves `http://127.0.0.1:7777/mcp` by
default; the server refuses a non-loopback bind without `--tls-cert`/`--tls-key`, so do not
pass `--addr :7777`):

```bash
SAAS_ALERTS_API_KEY=<value> SAAS_ALERTS_MCP_HTTP_TOKEN=<long-random-value> saas-alerts-mcp --transport http
```

On Windows (PowerShell):

```powershell
$env:SAAS_ALERTS_API_KEY="<value>"; $env:SAAS_ALERTS_MCP_HTTP_TOKEN="<long-random-value>"; saas-alerts-mcp --transport http
```

`SAAS_ALERTS_MCP_HTTP_TOKEN` is required: the server refuses to start in HTTP mode
without it, and rejects any request that does not carry `Authorization: Bearer <that value>`.
If your remote client cannot send that header, have your reverse proxy add it to the
requests it forwards, and put your own sign-in in front of the proxy.

Then expose `http://localhost:7777/mcp` as a public HTTPS URL via a secure tunnel
(Cloudflare Tunnel, ngrok) or your own reverse proxy. The path is part of the
endpoint: put `/mcp` in the connector URL, which is the Streamable HTTP endpoint this
server documents. **Treat the token and the URL as sensitive.** Never expose the
endpoint bare on the internet; gate it behind SSO / Cloudflare Access for team use.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector
pointing at your tunnel's HTTPS URL.

At the time of writing, ChatGPT's custom connectors sign in with OAuth or use no
authentication; they do not send a fixed bearer header. So `saas-alerts-mcp` answers a
direct ChatGPT connector with 401. Put a gateway in front that ChatGPT can authenticate to
with OAuth and that adds `Authorization: Bearer <your SAAS_ALERTS_MCP_HTTP_TOKEN>` to the
requests it forwards. Never expose the endpoint without authentication, and run it with
`SAAS_ALERTS_MCP_READ_ONLY=1` unless ChatGPT must act on Respond triggers or accounts.

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`saas-alerts-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `saas-alerts-mcp` is hosted over HTTPS (above), the lowest-code route:

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
hermes mcp add saas-alerts -- saas-alerts-mcp

# OpenClaw
openclaw mcp set saas-alerts '{"command":"saas-alerts-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `saas-alerts-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
