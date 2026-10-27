# Cove Data Protection MCP - install for every agent that speaks MCP

This page wires the Cove Data Protection MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `cove-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint; see **Remote agents** below for what this build supports.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `cove-cli`
and `cove-mcp` on your PATH. `cove-mcp` is what the agents talk to.

```bash
cove-mcp --help
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
    "cove": {
      "command": "cove-mcp",
      "env": {
        "COVE_PASSWORD": "<your-cove_password>",
        "COVE_BASE_URL": "<your-cove_base_url>",
        "COVE_PARTNER": "<your-cove_partner>",
        "COVE_USERNAME": "<your-cove_username>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "COVE_MCP_READ_ONLY": "<your-cove_mcp_read_only>",
        "COVE_NO_LOCAL_DATA": "<your-cove_no_local_data>",
        "COVE_MCP_HTTP_TOKEN": "<your-cove_mcp_http_token>",
        "COVE_USER_AGENT": "<your-cove_user_agent>"
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
    "cove": {
      "type": "stdio",
      "command": "cove-mcp",
      "env": {
        "COVE_PASSWORD": "<your-cove_password>",
        "COVE_BASE_URL": "<your-cove_base_url>",
        "COVE_PARTNER": "<your-cove_partner>",
        "COVE_USERNAME": "<your-cove_username>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "COVE_MCP_READ_ONLY": "<your-cove_mcp_read_only>",
        "COVE_NO_LOCAL_DATA": "<your-cove_no_local_data>",
        "COVE_MCP_HTTP_TOKEN": "<your-cove_mcp_http_token>",
        "COVE_USER_AGENT": "<your-cove_user_agent>"
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
    "cove": {
      "command": "cove-mcp",
      "env": {
        "COVE_PASSWORD": "<your-cove_password>",
        "COVE_BASE_URL": "<your-cove_base_url>",
        "COVE_PARTNER": "<your-cove_partner>",
        "COVE_USERNAME": "<your-cove_username>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "COVE_MCP_READ_ONLY": "<your-cove_mcp_read_only>",
        "COVE_NO_LOCAL_DATA": "<your-cove_no_local_data>",
        "COVE_MCP_HTTP_TOKEN": "<your-cove_mcp_http_token>",
        "COVE_USER_AGENT": "<your-cove_user_agent>"
      }
    }
  }
}
```

Restart Gemini CLI; the Cove Data Protection tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (host the binary over HTTPS first)

Remote agents need `cove-mcp` reachable over **HTTPS** with a bearer token. Keep
stdio (the default) for every local agent. HTTP mode serves
`http://127.0.0.1:7777/mcp` by default and refuses to start until `COVE_MCP_HTTP_TOKEN` is
set; every request must send `Authorization: Bearer <token>`. Any address other
than loopback also needs TLS:

```bash
export COVE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
COVE_PASSWORD=<value> COVE_BASE_URL=<value> COVE_PARTNER=<value> COVE_USERNAME=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> COVE_MCP_READ_ONLY=<value> COVE_NO_LOCAL_DATA=<value> COVE_USER_AGENT=<value> cove-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so a remote agent works only if its connector
settings can send that header; check yours first. **Treat the token like a password** -
whoever holds it can call every tool with your Cove Data Protection credentials.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector
pointing at your HTTPS endpoint, if its authentication settings can send the bearer
token (see above).

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`cove-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `cove-mcp` is hosted over HTTPS (above), the lowest-code route:

1. In **Copilot Studio**, open your agent > **Tools** > **Add a tool** > **Model
   Context Protocol**.
2. Enter a **Server name**, the **Server URL** (your HTTPS endpoint), and auth.
   The server needs `Authorization: Bearer <token>` on every request, so this
   works only if the auth option you pick sends it. Copilot Studio builds the Power
   Platform connector behind the scenes; generative orchestration must be **on**.
3. Publish the agent into Microsoft 365 Copilot.

Alternative (dev-ish): build a **declarative agent** with the Microsoft 365 Agents
Toolkit in VS Code (**Add an Action > Start with an MCP Server**, point at the remote
URL), then sideload - requires admin-enabled Custom App Upload + Copilot Access.

Microsoft docs: https://learn.microsoft.com/en-us/microsoft-copilot-studio/agent-extend-action-mcp

## Gemini app / web (Google)

Same remote pattern as ChatGPT - point Gemini's connector at your hosted HTTPS
endpoint, if it can send the bearer token. For a local, no-hosting path on Google, use
**Gemini CLI** (above) instead.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add cove -- cove-mcp

# OpenClaw
openclaw mcp set cove '{"command":"cove-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `cove-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
