# Liongard MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build has no password,
> listens on every network interface, and predates the browser-rebinding guard,
> so a web page you open can reach it. The MCP tools can also make changes with
> your credentials. Use the default stdio transport. If you set up a tunnel for
> a remote agent, shut it down. A fix is on the way.

This page wires the Liongard MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `liongard-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `liongard-cli`
and `liongard-mcp` on your PATH. `liongard-mcp` is what the agents talk to.

```bash
liongard-mcp --help
```

## Two ways to authenticate

Liongard's `X-ROAR-API-KEY` header is base64 of `accessKeyId:accessKeySecret`. Supply
it **either** way, never both:

- **A - pre-encoded key.** Set `LIONGARD_API_KEY` to the base64 value and leave
  `LIONGARD_ACCESS_KEY_ID` / `LIONGARD_ACCESS_KEY_SECRET` empty.
- **B - ID + secret.** Set `LIONGARD_ACCESS_KEY_ID` and `LIONGARD_ACCESS_KEY_SECRET`
  to the two strings Liongard hands you and leave `LIONGARD_API_KEY` empty; the server
  encodes them for you.

`LIONGARD_API_KEY` **wins whenever it is non-empty** - including when it still holds a
placeholder like `<your-liongard_api_key>`. If you are using shape B, clear that field
in the config blocks below or the ID/secret pair is ignored and every call returns 401.

Every config block below lists all five variables so you can see the full shape. Fill
in the ones for your chosen shape and leave the others as empty strings.

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
    "liongard": {
      "command": "liongard-mcp",
      "env": {
        "LIONGARD_ACCESS_KEY_ID": "",
        "LIONGARD_ACCESS_KEY_SECRET": "",
        "LIONGARD_API_KEY": "<your-liongard_api_key>",
        "LIONGARD_BASE_URL": "",
        "LIONGARD_INSTANCE": "<your-liongard_instance>"
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
    "liongard": {
      "type": "stdio",
      "command": "liongard-mcp",
      "env": {
        "LIONGARD_ACCESS_KEY_ID": "",
        "LIONGARD_ACCESS_KEY_SECRET": "",
        "LIONGARD_API_KEY": "<your-liongard_api_key>",
        "LIONGARD_BASE_URL": "",
        "LIONGARD_INSTANCE": "<your-liongard_instance>"
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
    "liongard": {
      "command": "liongard-mcp",
      "env": {
        "LIONGARD_ACCESS_KEY_ID": "",
        "LIONGARD_ACCESS_KEY_SECRET": "",
        "LIONGARD_API_KEY": "<your-liongard_api_key>",
        "LIONGARD_BASE_URL": "",
        "LIONGARD_INSTANCE": "<your-liongard_instance>"
      }
    }
  }
}
```

Restart Gemini CLI; the Liongard tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `liongard-mcp` in HTTP mode. Do
not do that with this build (see the security note at the top of this page), and
do not put this server behind a tunnel or reverse proxy. Use a local agent
above, or the Claude Code / Codex Skill, until the fix release brings these
steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# A - pre-encoded key. Leave the ID/secret pair unset.
# LIONGARD_API_KEY=<value> LIONGARD_BASE_URL=<value> LIONGARD_INSTANCE=<value> liongard-mcp --transport http
```

```bash
# B - ID + secret. Leave LIONGARD_API_KEY unset, or it wins and this pair is ignored.
# LIONGARD_ACCESS_KEY_ID=<value> LIONGARD_ACCESS_KEY_SECRET=<value> LIONGARD_BASE_URL=<value> LIONGARD_INSTANCE=<value> liongard-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add liongard -- liongard-mcp

# OpenClaw
openclaw mcp set liongard '{"command":"liongard-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `liongard-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
