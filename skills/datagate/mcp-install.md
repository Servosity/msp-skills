# DataGate MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

This page wires the DataGate MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `datagate-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

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

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `datagate-mcp` in HTTP mode. Do
not do that with this build (see the security note at the top of this page), and
do not put this server behind a tunnel or reverse proxy. Use a local agent
above, or the Claude Code / Codex Skill, until the fix release brings these
steps back.

In this build, HTTP mode serves `http://127.0.0.1:7777/mcp` by default, refuses
to start without `DATAGATE_MCP_HTTP_TOKEN`, and requires every caller to send
that token as `Authorization: Bearer`. It refuses a non-loopback address such as
`--addr :7777`, which this page used to show, unless you also pass `--tls-cert`
and `--tls-key`.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# DATAGATE_API_KEY=<value> DATAGATE_CLIENT_ID=<value> DATAGATE_MCP_HTTP_TOKEN=<value> datagate-mcp --transport http
```

---

## Troubleshooting

- `datagate-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.
- `doctor` reports auth not configured: confirm both `DATAGATE_API_KEY` and
  `DATAGATE_CLIENT_ID` are set - DataGate requires both together, not just one.

For the full CLI command reference, see [guide.md](./guide.md).
