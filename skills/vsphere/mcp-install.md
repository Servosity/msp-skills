# VMware vSphere MCP - install for every agent that speaks MCP

This page wires the VMware vSphere MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `vsphere-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint; see **Remote agents** below for what this build supports.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `vsphere-cli`
and `vsphere-mcp` on your PATH. `vsphere-mcp` is what the agents talk to.

```bash
vsphere-mcp --help
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
    "vsphere": {
      "command": "vsphere-mcp",
      "env": {
        "VSPHERE_PASSWORD": "<your-vsphere_password>",
        "VSPHERE_USERNAME": "<your-vsphere_username>",
        "VSPHERE_BASE_URL": "https://<your-vcenter>",
        "VSPHERE_TLS_FINGERPRINT": "",
        "VSPHERE_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "VSPHERE_MCP_READ_ONLY": "",
        "VSPHERE_NO_LOCAL_DATA": "",
        "VSPHERE_MCP_HTTP_TOKEN": ""
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
    "vsphere": {
      "type": "stdio",
      "command": "vsphere-mcp",
      "env": {
        "VSPHERE_PASSWORD": "<your-vsphere_password>",
        "VSPHERE_USERNAME": "<your-vsphere_username>",
        "VSPHERE_BASE_URL": "https://<your-vcenter>",
        "VSPHERE_TLS_FINGERPRINT": "",
        "VSPHERE_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "VSPHERE_MCP_READ_ONLY": "",
        "VSPHERE_NO_LOCAL_DATA": "",
        "VSPHERE_MCP_HTTP_TOKEN": ""
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
    "vsphere": {
      "command": "vsphere-mcp",
      "env": {
        "VSPHERE_PASSWORD": "<your-vsphere_password>",
        "VSPHERE_USERNAME": "<your-vsphere_username>",
        "VSPHERE_BASE_URL": "https://<your-vcenter>",
        "VSPHERE_TLS_FINGERPRINT": "",
        "VSPHERE_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": "",
        "VSPHERE_MCP_READ_ONLY": "",
        "VSPHERE_NO_LOCAL_DATA": "",
        "VSPHERE_MCP_HTTP_TOKEN": ""
      }
    }
  }
}
```

Restart Gemini CLI; the VMware tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (host the binary over HTTPS first)

Remote agents (ChatGPT, Microsoft 365 Copilot / Copilot Studio, the Gemini app)
need `vsphere-mcp` reachable over **HTTPS** with a bearer token. Keep stdio (the
default) for every local agent. HTTP mode serves `http://127.0.0.1:7777/mcp` by
default and refuses to start until `VSPHERE_MCP_HTTP_TOKEN` is set to at least 32
characters; every request must send `Authorization: Bearer <token>`. Any address
other than loopback also needs TLS:

```bash
export VSPHERE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
VSPHERE_BASE_URL=<value> VSPHERE_USERNAME=<value> VSPHERE_PASSWORD=<value> vsphere-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

On Windows (PowerShell):

```powershell
$b = New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
$env:VSPHERE_MCP_HTTP_TOKEN = -join ($b | ForEach-Object { '{0:x2}' -f $_ })
$env:VSPHERE_BASE_URL="<value>"; $env:VSPHERE_USERNAME="<value>"; $env:VSPHERE_PASSWORD="<value>"; vsphere-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so a remote agent works only if its connector
settings can send that header; check yours first. **Treat the token like a
password** - whoever holds it can call every tool with your vCenter credentials.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector
pointing at your HTTPS endpoint, if its authentication settings can send the bearer
token (see above).

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio and the Gemini app

Same remote pattern: point the connector at your hosted HTTPS endpoint, with the
bearer token in its authentication settings.

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add vsphere -- vsphere-mcp

# OpenClaw
openclaw mcp set vsphere '{"command":"vsphere-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `vsphere-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
