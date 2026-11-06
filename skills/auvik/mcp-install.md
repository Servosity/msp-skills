# Auvik MCP - install for every agent that speaks MCP

This page wires the Auvik MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, the ChatGPT desktop app,
GitHub Copilot, Gemini CLI) launch `auvik-mcp` directly on your machine - no
hosting. *Web-only* AI apps (ChatGPT on the web, Microsoft 365 Copilot / Copilot
Studio, the Gemini app) need a remote server, which msp-skills does not support
yet; see **Remote agents** below.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `auvik-cli`
and `auvik-mcp` on your PATH. `auvik-mcp` is what the agents talk to.

```bash
auvik-mcp --help
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
    "auvik": {
      "command": "auvik-mcp",
      "env": {
        "AUVIK_API_KEY": "<your-auvik_api_key>",
        "AUVIK_USERNAME": "<your-auvik_username>",
        "AUVIK_BASE_URL": "<your-auvik_base_url>",
        "AUVIK_TENANT": "<your-auvik_tenant>",
        "AUVIK_USER_AGENT": "<your-auvik_user_agent>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "AUVIK_MCP_READ_ONLY": "",
        "AUVIK_NO_LOCAL_DATA": "",
        "AUVIK_MCP_HTTP_TOKEN": ""
      }
    }
  }
}
```

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## ChatGPT desktop app (OpenAI)

The ChatGPT desktop app runs MCP servers locally (STDIO) and shares MCP
configuration with Codex on the same machine
([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). Open **Settings**, select
**MCP servers**, add a server, choose **STDIO**, and give `auvik-mcp` as the
command (its full path if the app can't find it). The server reads the same
environment variables as the Claude Desktop block above. To keep them in a file,
add the server once to Codex's `~/.codex/config.toml` (a `[mcp_servers.auvik]`
table with `command = "auvik-mcp"` and an `env` table); the desktop app reads
that file too.

ChatGPT on the web is a different case: see **Remote agents** below.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.vscode/mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "auvik": {
      "type": "stdio",
      "command": "auvik-mcp",
      "env": {
        "AUVIK_API_KEY": "<your-auvik_api_key>",
        "AUVIK_USERNAME": "<your-auvik_username>",
        "AUVIK_BASE_URL": "<your-auvik_base_url>",
        "AUVIK_TENANT": "<your-auvik_tenant>",
        "AUVIK_USER_AGENT": "<your-auvik_user_agent>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "AUVIK_MCP_READ_ONLY": "",
        "AUVIK_NO_LOCAL_DATA": "",
        "AUVIK_MCP_HTTP_TOKEN": ""
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
    "auvik": {
      "command": "auvik-mcp",
      "env": {
        "AUVIK_API_KEY": "<your-auvik_api_key>",
        "AUVIK_USERNAME": "<your-auvik_username>",
        "AUVIK_BASE_URL": "<your-auvik_base_url>",
        "AUVIK_TENANT": "<your-auvik_tenant>",
        "AUVIK_USER_AGENT": "<your-auvik_user_agent>",
        "PRINTING_PRESS_CLIENT_PROFILE": "<your-printing_press_client_profile>",
        "AUVIK_MCP_READ_ONLY": "",
        "AUVIK_NO_LOCAL_DATA": "",
        "AUVIK_MCP_HTTP_TOKEN": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Auvik tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (host the binary over HTTPS first)

**Not supported by msp-skills yet.** ChatGPT on the web, Microsoft 365 Copilot /
Copilot Studio and the Gemini app reach MCP servers at a web address, not programs
on your machine, so they would need `auvik-mcp` running in HTTP mode, hosted
by you over HTTPS. If your team works in one
of those apps, a
[hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)
is the better pick; check that your AI app is on the gateway's setup list. The
ChatGPT desktop app is different: it runs `auvik-mcp` locally (see above).

Remote agents need `auvik-mcp` reachable over **HTTPS** with a bearer token. Keep
stdio (the default) for every local agent. HTTP mode serves
`http://127.0.0.1:7777/mcp` by default and refuses to start until `AUVIK_MCP_HTTP_TOKEN` is
set; every request must send `Authorization: Bearer <token>`. Any address other
than loopback also needs TLS:

```bash
export AUVIK_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
AUVIK_API_KEY=<value> AUVIK_USERNAME=<value> AUVIK_BASE_URL=<value> AUVIK_TENANT=<value> AUVIK_USER_AGENT=<value> PRINTING_PRESS_CLIENT_PROFILE=<value> AUVIK_MCP_READ_ONLY=<value> AUVIK_NO_LOCAL_DATA=<value> auvik-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so a remote agent works only if its connector
settings can send that header; check yours first. **Treat the token like a password** -
whoever holds it can call every tool with your Auvik credentials.

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`auvik-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `auvik-mcp` is hosted over HTTPS (above), the lowest-code route:

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

Point Gemini's connector at your hosted HTTPS
endpoint, if it can send the bearer token. For a local, no-hosting path on Google, use
**Gemini CLI** (above) instead.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add auvik -- auvik-mcp

# OpenClaw
openclaw mcp set auvik '{"command":"auvik-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `auvik-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
