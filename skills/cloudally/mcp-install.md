# CloudAlly MCP - install for every agent that speaks MCP

This page wires the CloudAlly MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `cloudally-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `cloudally-mcp` over HTTPS first.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `cloudally-cli`
and `cloudally-mcp` on your PATH. `cloudally-mcp` is what the agents talk to.

```bash
cloudally-mcp --help
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
    "cloudally": {
      "command": "cloudally-mcp",
      "env": {
        "CLOUDALLY_CLIENT_ID": "<your CloudAlly API client id>",
        "CLOUDALLY_CLIENT_SECRET": "<your CloudAlly API client secret>",
        "CLOUDALLY_ACCESS_TOKEN": "",
        "CLOUDALLY_EMAIL": "",
        "CLOUDALLY_PASSWORD": "",
        "CLOUDALLY_PARTNER_EMAIL": "",
        "CLOUDALLY_PARTNER_PASSWORD": "",
        "CLOUDALLY_BASE_URL": "",
        "CLOUDALLY_PORTAL_URL": "",
        "CLOUDALLY_MCP_READ_ONLY": "",
        "CLOUDALLY_MCP_HTTP_TOKEN": "",
        "CLOUDALLY_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Leave `CLOUDALLY_ACCESS_TOKEN` empty: run `cloudally-cli auth login --partner` (or `cloudally-cli auth login` for one company) once in a terminal, which saves the token where the MCP server reads it and refreshes it. A pasted token expires and keeps overriding the saved one. The login email and password fields are only needed if you want the server's environment to carry them; leave them and the other empty values empty unless you need them.

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
    "cloudally": {
      "type": "stdio",
      "command": "cloudally-mcp",
      "env": {
        "CLOUDALLY_CLIENT_ID": "<your CloudAlly API client id>",
        "CLOUDALLY_CLIENT_SECRET": "<your CloudAlly API client secret>",
        "CLOUDALLY_ACCESS_TOKEN": "",
        "CLOUDALLY_EMAIL": "",
        "CLOUDALLY_PASSWORD": "",
        "CLOUDALLY_PARTNER_EMAIL": "",
        "CLOUDALLY_PARTNER_PASSWORD": "",
        "CLOUDALLY_BASE_URL": "",
        "CLOUDALLY_PORTAL_URL": "",
        "CLOUDALLY_MCP_READ_ONLY": "",
        "CLOUDALLY_MCP_HTTP_TOKEN": "",
        "CLOUDALLY_USER_AGENT": "",
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
    "cloudally": {
      "command": "cloudally-mcp",
      "env": {
        "CLOUDALLY_CLIENT_ID": "<your CloudAlly API client id>",
        "CLOUDALLY_CLIENT_SECRET": "<your CloudAlly API client secret>",
        "CLOUDALLY_ACCESS_TOKEN": "",
        "CLOUDALLY_EMAIL": "",
        "CLOUDALLY_PASSWORD": "",
        "CLOUDALLY_PARTNER_EMAIL": "",
        "CLOUDALLY_PARTNER_PASSWORD": "",
        "CLOUDALLY_BASE_URL": "",
        "CLOUDALLY_PORTAL_URL": "",
        "CLOUDALLY_MCP_READ_ONLY": "",
        "CLOUDALLY_MCP_HTTP_TOKEN": "",
        "CLOUDALLY_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the CloudAlly tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (expose the binary over HTTPS first)

All remote agents need `cloudally-mcp` reachable as a public **HTTPS** endpoint. Run it
in HTTP mode with your credentials in the environment (it serves `http://127.0.0.1:7777/mcp` by
default and refuses to start until `CLOUDALLY_MCP_HTTP_TOKEN` is set; press 4.32+ refuses a non-loopback bind without TLS, so do not pass `--addr :7777`):

```bash
CLOUDALLY_CLIENT_ID=<value> CLOUDALLY_CLIENT_SECRET=<value> CLOUDALLY_MCP_HTTP_TOKEN=<a long random value> cloudally-mcp --transport http
```

On Windows, set the same three variables in PowerShell first (`$env:CLOUDALLY_CLIENT_ID = "<value>"`, and likewise `CLOUDALLY_CLIENT_SECRET` and `CLOUDALLY_MCP_HTTP_TOKEN`), then start the server in HTTP mode the same way.

Then expose `http://localhost:7777/mcp` as a public HTTPS URL via a secure tunnel
(Cloudflare Tunnel, ngrok) or your own reverse proxy. The path is part of the
endpoint: the server answers Streamable HTTP at `/mcp` and returns 404 at the
bare root, so a connector pointed at the root URL never handshakes. **Treat that
URL as sensitive** - it's a key to your MCP server. Never expose it bare on the
internet; gate it behind SSO / Cloudflare Access for team use.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector.

ChatGPT connectors sign in with OAuth or with no authentication. They cannot send
the fixed `Authorization: Bearer` token that `cloudally-mcp` requires, so a connector
pointed straight at the tunnel gets HTTP 401 on every request. Put an authenticating
proxy in front: one that signs ChatGPT in with OAuth and adds
`Authorization: Bearer <your CLOUDALLY_MCP_HTTP_TOKEN>` to each request it forwards to
`http://localhost:7777/mcp`. Point the connector at that proxy's HTTPS URL.

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`cloudally-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `cloudally-mcp` is hosted over HTTPS (above), the lowest-code route:

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

Same remote pattern as ChatGPT, including the authenticating proxy - point
Gemini's connector at the proxy's HTTPS URL. For a local, no-hosting path on Google, use **Gemini CLI** (above) instead.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add cloudally -- cloudally-mcp

# OpenClaw
openclaw mcp set cloudally '{"command":"cloudally-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `cloudally-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
