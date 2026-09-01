# Avanan MCP - install for every agent that speaks MCP

This page wires the Avanan MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `avanan-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `avanan-mcp` over HTTPS first.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `avanan-cli`
and `avanan-mcp` on your PATH. `avanan-mcp` is what the agents talk to.

```bash
avanan-mcp --help
```

**Set your region first.** Avanan credentials are region-scoped and cross-region
reads are refused by design. The CLI defaults to the USA host. If your tenant is
anywhere else, run `avanan-cli auth login --region <us|eu|ca|ap|uk|uae|in> --save`
once before wiring up any agent, so the MCP server inherits the right host from
the saved config. `avanan-cli doctor` prints the `base_url` it resolved.

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
    "avanan": {
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-avanan-application-id>",
        "AVANAN_BASE_URL": "https://smart-api-production-1-us.avanan.net",
        "AVANAN_CLIENT_SECRET": "<your-avanan-client-secret>",
        "AVANAN_TOKEN": "<your-avanan_token>",
        "AVANAN_USER_AGENT": "",
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

Create `.mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "avanan": {
      "type": "stdio",
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-avanan-application-id>",
        "AVANAN_BASE_URL": "https://smart-api-production-1-us.avanan.net",
        "AVANAN_CLIENT_SECRET": "<your-avanan-client-secret>",
        "AVANAN_TOKEN": "<your-avanan_token>",
        "AVANAN_USER_AGENT": "",
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
    "avanan": {
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-avanan-application-id>",
        "AVANAN_BASE_URL": "https://smart-api-production-1-us.avanan.net",
        "AVANAN_CLIENT_SECRET": "<your-avanan-client-secret>",
        "AVANAN_TOKEN": "<your-avanan_token>",
        "AVANAN_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Avanan tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (expose the binary over HTTPS first)

All remote agents need `avanan-mcp` reachable as a public **HTTPS** endpoint.
`avanan-mcp` speaks **stdio only** - it has no HTTP mode and opens no listener of
its own - so put a stdio-to-HTTP bridge in front of it:

```bash
# ChatGPT and anything else that accepts SSE:
AVANAN_APP_ID=<value> AVANAN_BASE_URL=<value> AVANAN_CLIENT_SECRET=<value> AVANAN_TOKEN=<value> AVANAN_USER_AGENT=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> npx -y supergateway --stdio "avanan-mcp" --port 7777

# Microsoft 365 Copilot / Copilot Studio, which take Streamable HTTP only:
AVANAN_APP_ID=<value> AVANAN_BASE_URL=<value> AVANAN_CLIENT_SECRET=<value> AVANAN_TOKEN=<value> AVANAN_USER_AGENT=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> npx -y supergateway --stdio "avanan-mcp" --port 7777 \
  --outputTransport streamableHttp --streamableHttpPath /mcp
```

**Pick the transport your consumer actually takes** - that is the whole reason
there are two lines. The first serves the bridged server at
`http://localhost:7777/sse`; the second at `http://localhost:7777/mcp`. Expose
whichever one you started as a public HTTPS URL via a secure tunnel
(Cloudflare Tunnel, ngrok) or your own reverse proxy. **Treat that URL as
sensitive** - it's a key to your MCP server. Never expose it bare on the internet;
gate it behind SSO / Cloudflare Access for team use.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector
pointing at your tunnel's HTTPS URL.

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`avanan-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `avanan-mcp` is hosted over HTTPS (above), the lowest-code route:

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
hermes mcp add avanan -- avanan-mcp

# OpenClaw
openclaw mcp set avanan '{"command":"avanan-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `avanan-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
