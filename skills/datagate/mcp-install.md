# DataGate MCP - install for every agent that speaks MCP

This page wires the DataGate MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `datagate-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `datagate-mcp` over HTTPS first.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `datagate-cli`
and `datagate-mcp` on your PATH. `datagate-mcp` is what the agents talk to.

```bash
datagate-mcp --help
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
    "datagate": {
      "command": "datagate-mcp",
      "env": {
        "DATAGATE_API_KEY": "<your-datagate-bearer-token>",
        "DATAGATE_CLIENT_ID": "<your-datagate-clientid-guid>",
        "DATAGATE_BASE_URL": "",
        "DATAGATE_USER_AGENT": "",
        "DATAGATE_MCP_HTTP_TOKEN": "",
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
    "datagate": {
      "type": "stdio",
      "command": "datagate-mcp",
      "env": {
        "DATAGATE_API_KEY": "<your-datagate-bearer-token>",
        "DATAGATE_CLIENT_ID": "<your-datagate-clientid-guid>",
        "DATAGATE_BASE_URL": "",
        "DATAGATE_USER_AGENT": "",
        "DATAGATE_MCP_HTTP_TOKEN": "",
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
    "datagate": {
      "command": "datagate-mcp",
      "env": {
        "DATAGATE_API_KEY": "<your-datagate-bearer-token>",
        "DATAGATE_CLIENT_ID": "<your-datagate-clientid-guid>",
        "DATAGATE_BASE_URL": "",
        "DATAGATE_USER_AGENT": "",
        "DATAGATE_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the DataGate tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (expose the binary over HTTPS first)

All remote agents need `datagate-mcp` reachable as a public **HTTPS** endpoint. Run
it in HTTP mode with your credentials in the environment:

```bash
DATAGATE_API_KEY=<value> DATAGATE_CLIENT_ID=<value> DATAGATE_MCP_HTTP_TOKEN=<value> datagate-mcp --transport http --addr :7777
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
`datagate-mcp` you installed is not enough on its own. You also need a **Copilot
Studio license** and a **tenant admin** to enable it. This is a build-and-host task,
not a self-serve install.

Once `datagate-mcp` is hosted over HTTPS (above), the lowest-code route:

1. In **Copilot Studio**, open your agent > **Tools** > **Add a tool** > **Model
   Context Protocol**.
2. Enter a **Server name**, the **Server URL** (your HTTPS endpoint), and auth
   (API key). Copilot Studio builds the Power Platform connector behind the
   scenes; generative orchestration must be **on**.
3. Publish the agent into Microsoft 365 Copilot.

Microsoft docs: https://learn.microsoft.com/en-us/microsoft-copilot-studio/agent-extend-action-mcp

## Gemini app / web (Google)

Same remote pattern as ChatGPT - point Gemini's connector at your hosted HTTPS
endpoint. For a local, no-hosting path on Google, use **Gemini CLI** (above) instead.

---

## Troubleshooting

- `datagate-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.
- `doctor` reports auth not configured: confirm both `DATAGATE_API_KEY` and
  `DATAGATE_CLIENT_ID` are set - DataGate requires both together, not just one.

For the full CLI command reference, see [guide.md](./guide.md).
