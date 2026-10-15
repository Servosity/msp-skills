# Proxmox MCP - install for every agent that speaks MCP

This page wires the Proxmox MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `proxmox-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so you expose `proxmox-mcp` over HTTPS first.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `proxmox-cli`
and `proxmox-mcp` on your PATH. `proxmox-mcp` is what the agents talk to.

```bash
proxmox-mcp --help
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
    "proxmox": {
      "command": "proxmox-mcp",
      "env": {
        "PROXMOX_HOST": "<pve-host>",
        "PROXMOX_API_TOKEN": "<USER@REALM!TOKENID=SECRET>",
        "PROXMOX_TLS_FINGERPRINT": "<PVE certificate SHA-256>",
        "PROXMOX_PBS_HOST": "",
        "PROXMOX_PBS_API_TOKEN": "",
        "PROXMOX_PBS_TLS_FINGERPRINT": "",
        "PROXMOX_BASE_URL": "",
        "PROXMOX_CLUSTER": "",
        "PROXMOX_CLUSTERS_FILE": "",
        "PROXMOX_USER_AGENT": "",
        "PROXMOX_MCP_READ_ONLY": "",
        "PROXMOX_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Replace the three `<...>` values; every other value is optional and may stay empty.
Fill the three `PROXMOX_PBS_*` values (`USER@REALM!TOKENID:SECRET`, colon before
the secret) to add Proxmox Backup Server verify, datastore and sync data. For many
clients, point `PROXMOX_CLUSTERS_FILE` at your clusters.toml and add each
cluster's token variables (the names you gave `clusters add`) to this same `env`
block, because the MCP server only sees the variables listed here. Which token
roles to grant is in [governance.md](./governance.md).

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
    "proxmox": {
      "type": "stdio",
      "command": "proxmox-mcp",
      "env": {
        "PROXMOX_HOST": "<pve-host>",
        "PROXMOX_API_TOKEN": "<USER@REALM!TOKENID=SECRET>",
        "PROXMOX_TLS_FINGERPRINT": "<PVE certificate SHA-256>",
        "PROXMOX_PBS_HOST": "",
        "PROXMOX_PBS_API_TOKEN": "",
        "PROXMOX_PBS_TLS_FINGERPRINT": "",
        "PROXMOX_BASE_URL": "",
        "PROXMOX_CLUSTER": "",
        "PROXMOX_CLUSTERS_FILE": "",
        "PROXMOX_USER_AGENT": "",
        "PROXMOX_MCP_READ_ONLY": "",
        "PROXMOX_MCP_HTTP_TOKEN": "",
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
    "proxmox": {
      "command": "proxmox-mcp",
      "env": {
        "PROXMOX_HOST": "<pve-host>",
        "PROXMOX_API_TOKEN": "<USER@REALM!TOKENID=SECRET>",
        "PROXMOX_TLS_FINGERPRINT": "<PVE certificate SHA-256>",
        "PROXMOX_PBS_HOST": "",
        "PROXMOX_PBS_API_TOKEN": "",
        "PROXMOX_PBS_TLS_FINGERPRINT": "",
        "PROXMOX_BASE_URL": "",
        "PROXMOX_CLUSTER": "",
        "PROXMOX_CLUSTERS_FILE": "",
        "PROXMOX_USER_AGENT": "",
        "PROXMOX_MCP_READ_ONLY": "",
        "PROXMOX_MCP_HTTP_TOKEN": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Proxmox tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (expose the binary over HTTPS first)

All remote agents need `proxmox-mcp` reachable as a public **HTTPS** endpoint. Run it
in HTTP mode with your credentials in the environment (it serves `http://127.0.0.1:7777/mcp` by
default; press 4.32+ refuses a non-loopback bind without TLS, so do not pass `--addr :7777`):

```bash
PROXMOX_HOST=<pve-host> PROXMOX_API_TOKEN='<USER@REALM!TOKENID=SECRET>' PROXMOX_TLS_FINGERPRINT='<PVE certificate SHA-256>' PROXMOX_MCP_HTTP_TOKEN='<long random value>' proxmox-mcp --transport http
```

`PROXMOX_MCP_HTTP_TOKEN` is required in HTTP mode: the server refuses to start without it, and
every request must carry `Authorization: Bearer <PROXMOX_MCP_HTTP_TOKEN>` (requests without it
get HTTP 401). Generate a long random value, for example `openssl rand -hex 32`. Add
`PROXMOX_PBS_HOST`, `PROXMOX_PBS_API_TOKEN` and `PROXMOX_PBS_TLS_FINGERPRINT` to the same line for PBS data.

Then expose `http://localhost:7777/mcp` as a public HTTPS URL via a secure tunnel
(Cloudflare Tunnel, ngrok) or your own reverse proxy. Point connectors at the
`/mcp` path, the Streamable HTTP endpoint the clients expect. **Treat that
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
`proxmox-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `proxmox-mcp` is hosted over HTTPS (above), the lowest-code route:

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
hermes mcp add proxmox -- proxmox-mcp

# OpenClaw
openclaw mcp set proxmox '{"command":"proxmox-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `proxmox-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
