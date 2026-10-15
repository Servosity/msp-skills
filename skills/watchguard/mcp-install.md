# WatchGuard MCP - install for every agent that speaks MCP

This page wires the WatchGuard MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `watchguard-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `watchguard-mcp` over HTTPS first.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `watchguard-cli`
and `watchguard-mcp` on your PATH. `watchguard-mcp` is what the agents talk to.

```bash
watchguard-mcp --help
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
    "watchguard": {
      "command": "watchguard-mcp",
      "env": {
        "WATCHGUARD_CLIENT_ID": "<your-watchguard_client_id>",
        "WATCHGUARD_CLIENT_SECRET": "<your-watchguard_client_secret>",
        "WATCHGUARD_API_KEY": "<your-watchguard_api_key>",
        "WATCHGUARD_ACCOUNT_ID": "<your-watchguard_account_id>",
        "WATCHGUARD_BASE_URL": "",
        "WATCHGUARD_TOKEN_URL": "",
        "WATCHGUARD_AUDIENCE": "",
        "WATCHGUARD_OAUTH_SCOPE": "",
        "WATCHGUARD_USER_AGENT": "",
        "WATCHGUARD_MCP_READ_ONLY": "",
        "WATCHGUARD_MCP_HTTP_TOKEN": "",
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
    "watchguard": {
      "type": "stdio",
      "command": "watchguard-mcp",
      "env": {
        "WATCHGUARD_CLIENT_ID": "<your-watchguard_client_id>",
        "WATCHGUARD_CLIENT_SECRET": "<your-watchguard_client_secret>",
        "WATCHGUARD_API_KEY": "<your-watchguard_api_key>",
        "WATCHGUARD_ACCOUNT_ID": "<your-watchguard_account_id>",
        "WATCHGUARD_BASE_URL": "",
        "WATCHGUARD_TOKEN_URL": "",
        "WATCHGUARD_AUDIENCE": "",
        "WATCHGUARD_OAUTH_SCOPE": "",
        "WATCHGUARD_USER_AGENT": "",
        "WATCHGUARD_MCP_READ_ONLY": "",
        "WATCHGUARD_MCP_HTTP_TOKEN": "",
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
    "watchguard": {
      "command": "watchguard-mcp",
      "env": {
        "WATCHGUARD_CLIENT_ID": "<your-watchguard_client_id>",
        "WATCHGUARD_CLIENT_SECRET": "<your-watchguard_client_secret>",
        "WATCHGUARD_API_KEY": "<your-watchguard_api_key>",
        "WATCHGUARD_ACCOUNT_ID": "<your-watchguard_account_id>",
        "WATCHGUARD_BASE_URL": "",
        "WATCHGUARD_TOKEN_URL": "",
        "WATCHGUARD_AUDIENCE": "",
        "WATCHGUARD_OAUTH_SCOPE": "",
        "WATCHGUARD_USER_AGENT": "",
        "WATCHGUARD_MCP_READ_ONLY": "",
        "WATCHGUARD_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the WatchGuard tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (expose the binary over HTTPS first)

All remote agents need `watchguard-mcp` reachable as a public **HTTPS** endpoint. Run it
in HTTP mode with your credentials in the environment (it serves `http://127.0.0.1:7777/mcp` by
default; press 4.32+ refuses a non-loopback bind without TLS, so do not pass `--addr :7777`):

```bash
WATCHGUARD_CLIENT_ID=<value> WATCHGUARD_CLIENT_SECRET=<value> WATCHGUARD_API_KEY=<value> WATCHGUARD_ACCOUNT_ID=<value> WATCHGUARD_MCP_HTTP_TOKEN=<value> watchguard-mcp --transport http
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
`watchguard-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `watchguard-mcp` is hosted over HTTPS (above), the lowest-code route:

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
hermes mcp add watchguard -- watchguard-mcp

# OpenClaw
openclaw mcp set watchguard '{"command":"watchguard-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `watchguard-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
