> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/gemini/

# HaloPSA and Servosity in Google Gemini

Google's Gemini comes in two shapes that consume MCP differently. Pick the one you have.

- **Gemini CLI** - local, self-serve, no hosting. Like Claude Code. **Start here if you can.**
- **Gemini app / web** - needs a remote server. Not supported by MSP Skills yet (see below).

## What you need

- **Gemini CLI** installed
- A **HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token** - or both
- A terminal once for install

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

## Gemini CLI (local - recommended)

Edit `~/.gemini/settings.json` (or the path `gemini config path` prints) and add an `mcpServers` block - the same shape as Claude Desktop:

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "<your-tenant>",
        "HALOPSA_CLIENT_ID": "<your-client-id>",
        "HALOPSA_CLIENT_SECRET": "<your-client-secret>"
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

Restart Gemini CLI. Its tool list now includes the MSP Skills tools. Ask:

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

## Gemini app / web (not supported yet)

The Gemini app doesn't launch a local binary: it reaches MCP servers at a web address, so it needs a remote server. MSP Skills does not support that yet. Our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md#handling-credentials-safely) says not to run a connector in HTTP mode until it is reprinted, so don't host one over HTTPS or put it behind a tunnel for the Gemini app. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list. For a no-hosting path on Google, use Gemini CLI above.

## Troubleshooting

**Gemini CLI doesn't see the tools:** check the JSON in `~/.gemini/settings.json` (trailing commas, escaped backslashes on Windows) and restart. Confirm `halopsa-cli doctor` works first.

**"command not found":** the binary isn't on your PATH - the installer prints the line to add.

## What's next

- **Try a real workflow** at a free [Build Session](https://compoundingteams.com/build-sessions).
- **Prefer a desktop chat app?** [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) is the simplest local MCP host.

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
