# ConnectWise RMM MCP - install for every agent that speaks MCP

This page wires the ConnectWise RMM MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `connectwise-rmm-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `connectwise-rmm-mcp` over HTTPS first.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `connectwise-rmm-cli`
and `connectwise-rmm-mcp` on your PATH. `connectwise-rmm-mcp` is what the agents talk to.

```bash
connectwise-rmm-mcp --help
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
    "connectwise-rmm": {
      "command": "connectwise-rmm-mcp",
      "env": {
        "CONNECTWISE_RMM_CLIENT_ID": "<your-connectwise_rmm_client_id>",
        "CONNECTWISE_RMM_CLIENT_SECRET": "<your-connectwise_rmm_client_secret>",
        "CONNECTWISE_RMM_OAUTH_SCOPE": "<your-connectwise_rmm_oauth_scope>",
        "CONNECTWISE_RMM_BASE_URL": "<your-connectwise_rmm_base_url>",
        "CONNECTWISE_RMM_TOKEN_URL": "<your-connectwise_rmm_token_url>",
        "CONNECTWISE_RMM_USER_AGENT": "<your-connectwise_rmm_user_agent>",
        "CONNECTWISE_RMM_MCP_READ_ONLY": "<your-connectwise_rmm_mcp_read_only>",
        "CONNECTWISE_RMM_MCP_HTTP_TOKEN": "<your-connectwise_rmm_mcp_http_token>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>"
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

Create `.mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "connectwise-rmm": {
      "type": "stdio",
      "command": "connectwise-rmm-mcp",
      "env": {
        "CONNECTWISE_RMM_CLIENT_ID": "<your-connectwise_rmm_client_id>",
        "CONNECTWISE_RMM_CLIENT_SECRET": "<your-connectwise_rmm_client_secret>",
        "CONNECTWISE_RMM_OAUTH_SCOPE": "<your-connectwise_rmm_oauth_scope>",
        "CONNECTWISE_RMM_BASE_URL": "<your-connectwise_rmm_base_url>",
        "CONNECTWISE_RMM_TOKEN_URL": "<your-connectwise_rmm_token_url>",
        "CONNECTWISE_RMM_USER_AGENT": "<your-connectwise_rmm_user_agent>",
        "CONNECTWISE_RMM_MCP_READ_ONLY": "<your-connectwise_rmm_mcp_read_only>",
        "CONNECTWISE_RMM_MCP_HTTP_TOKEN": "<your-connectwise_rmm_mcp_http_token>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>"
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
    "connectwise-rmm": {
      "command": "connectwise-rmm-mcp",
      "env": {
        "CONNECTWISE_RMM_CLIENT_ID": "<your-connectwise_rmm_client_id>",
        "CONNECTWISE_RMM_CLIENT_SECRET": "<your-connectwise_rmm_client_secret>",
        "CONNECTWISE_RMM_OAUTH_SCOPE": "<your-connectwise_rmm_oauth_scope>",
        "CONNECTWISE_RMM_BASE_URL": "<your-connectwise_rmm_base_url>",
        "CONNECTWISE_RMM_TOKEN_URL": "<your-connectwise_rmm_token_url>",
        "CONNECTWISE_RMM_USER_AGENT": "<your-connectwise_rmm_user_agent>",
        "CONNECTWISE_RMM_MCP_READ_ONLY": "<your-connectwise_rmm_mcp_read_only>",
        "CONNECTWISE_RMM_MCP_HTTP_TOKEN": "<your-connectwise_rmm_mcp_http_token>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>"
      }
    }
  }
}
```

Restart Gemini CLI; the ConnectWise RMM tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (expose the binary over HTTPS first)

All remote agents need `connectwise-rmm-mcp` reachable as a public **HTTPS** endpoint. Run it
in HTTP mode with your credentials in the environment (it listens on 127.0.0.1:7777 by
default and refuses a non-loopback bind without `--tls-cert`/`--tls-key`, so do not pass `--addr :7777`).
Only the client ID, the client secret and `CONNECTWISE_RMM_MCP_HTTP_TOKEN` are required; leave out
any optional variable you are not setting:

```bash
CONNECTWISE_RMM_CLIENT_ID=<value> CONNECTWISE_RMM_CLIENT_SECRET=<value> CONNECTWISE_RMM_OAUTH_SCOPE=<value> CONNECTWISE_RMM_BASE_URL=<value> CONNECTWISE_RMM_TOKEN_URL=<value> CONNECTWISE_RMM_USER_AGENT=<value> CONNECTWISE_RMM_MCP_READ_ONLY=<value> CONNECTWISE_RMM_MCP_HTTP_TOKEN=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> connectwise-rmm-mcp --transport http
```

Then expose `http://localhost:7777/mcp` as a public HTTPS URL via a secure tunnel
(Cloudflare Tunnel, ngrok) or your own reverse proxy. The server answers
Streamable HTTP at `/mcp`; use that URL, and send the token as
`Authorization: Bearer <CONNECTWISE_RMM_MCP_HTTP_TOKEN>` - requests without it get HTTP 401. **Treat that
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
`connectwise-rmm-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `connectwise-rmm-mcp` is hosted over HTTPS (above), the lowest-code route:

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
hermes mcp add connectwise-rmm -- connectwise-rmm-mcp

# OpenClaw
openclaw mcp set connectwise-rmm '{"command":"connectwise-rmm-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `connectwise-rmm-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
