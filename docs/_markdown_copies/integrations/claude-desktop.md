> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/claude-desktop/

# HaloPSA and Servosity in Claude Desktop

Claude Desktop is the **most common MSP-owner choice** for MSP Skills - a Mac/Windows app with a chat window, no terminal after install. This guide installs the HaloPSA and Servosity MSP Skills as MCP servers in Claude Desktop in about 60 seconds.

Claude Desktop is Anthropic's original MCP host - first-class native MCP support, no Plus / Pro tier required (Claude Desktop itself is free; you pay only for Claude usage). [modelcontextprotocol.io/quickstart/user](https://modelcontextprotocol.io/quickstart/user)

## What you need

- **Claude Desktop** installed (from [claude.ai](https://claude.ai) → Download)
- A **HaloPSA tenant** with API credentials (Configuration → Integrations → Halo PSA API) **or** a **Servosity partner API token** (from the partner portal) - or both
- A terminal once for install (after that, it's all GUI)

## Step 1 - Install MSP Skills binaries

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

Install only the ones you'll use. Each installer drops the CLI and the MCP server on your PATH.

> **Claude Desktop bundle:** each connector also ships a one-click `.mcpb` (Claude Desktop > Settings > Extensions) that launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `halopsa-cli` / `servosity-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the installer above first (or set `HALOPSA_CLI_PATH` / `SERVOSITY_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

## Step 2 - Edit Claude Desktop's config

Open Claude Desktop → **Settings → Developer → Edit Config**. (This opens the JSON in your default editor.) Add the `mcpServers` block:

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "<your-tenant>",
        "HALOPSA_CLIENT_ID": "<your-client-id>",
        "HALOPSA_CLIENT_SECRET": "<your-client-secret>",
        "HALOPSA_DOMAIN": "halopsa.com"
      }
    },
    "servosity": {
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "<your-partner-token>"
      }
    }
  }
}
```

Save the file.

Note: HaloPSA instances default to `tenant.halopsa.com`. If that is your URL,
leave `HALOPSA_DOMAIN` out entirely. If you are on a custom domain such as
`psa.company.com`, split it: `psa` is your `HALOPSA_TENANT` and `company.com` is
your `HALOPSA_DOMAIN`. Setting the whole URL as the domain produces a TLS
certificate mismatch whose visible symptom is a mass sync failure rather than an
authentication error.

## Step 3 - Restart Claude Desktop

**Quit completely** (not just close - File → Quit on Mac, or right-click → Quit on Windows) and reopen. An MCP indicator appears bottom-right of the chat input once the servers connect.

## Step 4 - Ask a real question

In Claude Desktop chat:

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*
- *"Use halopsa: build a client card for Acme Corp."*

Claude reads the available MCP tools and runs the right commands.

## Troubleshooting

**MCP indicator never appears:** JSON syntax error in `claude_desktop_config.json`. Validate with `jq .` or any JSON linter. Common gotchas: trailing commas, unescaped backslashes on Windows paths.

**"halopsa-mcp: command not found":** the installer didn't put the binary on your PATH. The installer prints the line to add (something like `export PATH="$HOME/.local/bin:$PATH"`). Add it to `~/.zshrc` or `~/.bashrc` (macOS/Linux) or your Windows PATH, restart your terminal, then quit and reopen Claude Desktop.

**Authentication errors:** confirm you can run `halopsa-cli doctor` or `servosity-cli doctor` in a terminal first. If the CLI works, the MCP server will too with the same credentials.

## Want shell-driven install instead of JSON editing?

If you've moved (or want to move) onto **[Claude Cowork](https://msp-skills.compoundingteams.com/integrations/cowork.md)** (Anthropic's desktop agent with shell execution, GA Mar 2026), you can skip the config-file editing entirely - paste one prompt and Cowork runs the installer for you.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions) - we'll work it live with the MSP cohort.
- **Learn the command surface.** Each skill has a deep `SKILL.md` Claude reads on demand - you don't need to memorize commands.
- **Want a different platform?** [Request a Skill →](https://msp-skills.compoundingteams.com/requesting-a-skill.md) for the PSA, RMM, backup, or security tool you actually use.

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
