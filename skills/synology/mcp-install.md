# Synology MCP - install for every agent that speaks MCP

This page wires the Synology MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `synology-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint; see **Remote agents** below for what this build supports.

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `synology-cli`
and `synology-mcp` on your PATH. `synology-mcp` is what the agents talk to.

```bash
synology-mcp --help
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
    "synology": {
      "command": "synology-mcp",
      "env": {
        "SYNOLOGY_BASE_URL": "https://<nas>:5001",
        "SYNOLOGY_ACCOUNT": "<dsm-account>",
        "SYNOLOGY_PASSWORD": "<dsm-password>",
        "SYNOLOGY_DEVICE_ID": "",
        "SYNOLOGY_OTP_CODE": "",
        "SYNOLOGY_TLS_FINGERPRINT": "",
        "SYNOLOGY_SKIP_TLS_VERIFY": "",
        "SYNOLOGY_MCP_READ_ONLY": "",
        "SYNOLOGY_MCP_HTTP_TOKEN": "",
        "SYNOLOGY_NO_LOCAL_DATA": "",
        "SYNOLOGY_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Replace the three `<...>` values; leave the optional ones blank unless you need
them (`SYNOLOGY_TLS_FINGERPRINT` pins a self-signed certificate; a two-factor
account needs neither `SYNOLOGY_DEVICE_ID` nor `SYNOLOGY_OTP_CODE` once you run
`synology-cli auth login` with a fresh code in a terminal, because the server
reuses the trusted-device token it cached; any `SYNOLOGY_MCP_READ_ONLY` value
other than blank, 0, false, no or off removes every write tool). The address
must be `https://`: plain `http://`, this machine included, is refused before
the password is sent. Quit Claude Desktop completely and reopen, then ask
a question that needs the API.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Open the Command Palette > **MCP: Open User Configuration** and add the block below. A
workspace `.vscode/mcp.json` works too, but it is often committed to a repository: keep
the DSM password out of a committed file (VS Code's `inputs` with `"password": true` prompts for it instead).

```json
{
  "servers": {
    "synology": {
      "type": "stdio",
      "command": "synology-mcp",
      "env": {
        "SYNOLOGY_BASE_URL": "https://<nas>:5001",
        "SYNOLOGY_ACCOUNT": "<dsm-account>",
        "SYNOLOGY_PASSWORD": "<dsm-password>",
        "SYNOLOGY_DEVICE_ID": "",
        "SYNOLOGY_OTP_CODE": "",
        "SYNOLOGY_TLS_FINGERPRINT": "",
        "SYNOLOGY_SKIP_TLS_VERIFY": "",
        "SYNOLOGY_MCP_READ_ONLY": "",
        "SYNOLOGY_MCP_HTTP_TOKEN": "",
        "SYNOLOGY_NO_LOCAL_DATA": "",
        "SYNOLOGY_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Then open Copilot Chat and switch the mode dropdown to **Agent** - MCP tools are
invisible in Ask/Edit mode.

## Gemini CLI (Google)

Edit `~/.gemini/settings.json` (`%USERPROFILE%\.gemini\settings.json` on Windows), Gemini CLI's config, and add the same shape as
Claude Desktop:

```json
{
  "mcpServers": {
    "synology": {
      "command": "synology-mcp",
      "env": {
        "SYNOLOGY_BASE_URL": "https://<nas>:5001",
        "SYNOLOGY_ACCOUNT": "<dsm-account>",
        "SYNOLOGY_PASSWORD": "<dsm-password>",
        "SYNOLOGY_DEVICE_ID": "",
        "SYNOLOGY_OTP_CODE": "",
        "SYNOLOGY_TLS_FINGERPRINT": "",
        "SYNOLOGY_SKIP_TLS_VERIFY": "",
        "SYNOLOGY_MCP_READ_ONLY": "",
        "SYNOLOGY_MCP_HTTP_TOKEN": "",
        "SYNOLOGY_NO_LOCAL_DATA": "",
        "SYNOLOGY_USER_AGENT": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the Synology tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (host the binary over HTTPS first)

Remote agents need `synology-mcp` reachable over **HTTPS** with a bearer token. Keep
stdio (the default) for every local agent. HTTP mode serves
`http://127.0.0.1:7777/mcp` by default and refuses to start until `SYNOLOGY_MCP_HTTP_TOKEN` is
set; every request must send `Authorization: Bearer <token>`. Any address other
than loopback also needs TLS:

macOS/Linux (`<a long random value>`: at least 32 characters, for example the
output of `openssl rand -hex 32`):

```bash
SYNOLOGY_BASE_URL=https://<nas>:5001 SYNOLOGY_ACCOUNT=<dsm-account> SYNOLOGY_PASSWORD=<dsm-password> SYNOLOGY_MCP_HTTP_TOKEN=<a long random value> synology-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

Windows (PowerShell):

```powershell
$env:SYNOLOGY_BASE_URL="https://<nas>:5001"; $env:SYNOLOGY_ACCOUNT="<dsm-account>"; $env:SYNOLOGY_PASSWORD="<dsm-password>"; $env:SYNOLOGY_MCP_HTTP_TOKEN="<a long random value>"; synology-mcp --transport http --addr <host>:7777 --tls-cert <cert.pem> --tls-key <key.pem>
```

Add `SYNOLOGY_TLS_FINGERPRINT` for a NAS with a self-signed certificate, and
any optional variable from the blocks above. For a two-factor account, run
`synology-cli auth login` with a fresh `SYNOLOGY_OTP_CODE` once in a terminal on
the same machine; the server reuses the trusted-device token it cached.

The endpoint is `https://<host>:7777/mcp`: the server answers Streamable HTTP at
`/mcp` and returns 404 at the bare root. The server rejects any request without
`Authorization: Bearer <token>`, so a remote agent works only if its connector
settings can send that header; check yours first. **Treat the token like a password** -
whoever holds it can call every tool with your Synology credentials.

## ChatGPT (Developer Mode)

In ChatGPT (Pro, Plus, Team, Business, Enterprise, or Education - **not** Free):
Settings > Apps > Advanced > **Developer mode**, then create a custom connector
pointing at your HTTPS endpoint, if its authentication settings can send the bearer
token (see above).

Official OpenAI guidance (beta, plan-dependent): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

## Microsoft 365 Copilot / Copilot Studio

**Honest heads-up:** there is no local path. Microsoft 365 Copilot, Copilot Studio,
and Security Copilot all consume MCP over **remote Streamable-HTTP only** - the local
`synology-mcp` you installed is not enough on its own. You also need a **Copilot Studio
license** and a **tenant admin** to enable it. This is a build-and-host task, not a
self-serve install.

Once `synology-mcp` is hosted over HTTPS (above), the lowest-code route:

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
hermes mcp add synology -- synology-mcp

# OpenClaw
openclaw mcp set synology '{"command":"synology-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Fleet NAS units over MCP

The `env` blocks above configure the single NAS in `SYNOLOGY_BASE_URL`. `fleet_sync` and `abb_triage` reach every NAS you registered with `synology-cli fleet add`, and each of those reads its own login from the MCP server's environment: add `SYNOLOGY_<NAME>_ACCOUNT` and `SYNOLOGY_<NAME>_PASSWORD` (or the `--env-prefix` you chose) for every registered NAS to the server's `env` block; for a two-factor account, run `synology-cli fleet add <name> --check` with `SYNOLOGY_<NAME>_OTP_CODE` once in a terminal and the server reuses the cached trusted-device token. The one-click `.mcpb` has no fields for them, so a fleet setup uses the JSON config. Ask the agent to run the MCP `fleet_list` tool to see which variables each NAS expects and whether the server has them (`synology-cli fleet list` in a terminal reports that terminal's environment, not the server's). These config files hold the passwords in plain text: keep them private.

## Troubleshooting

- `synology-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
