> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/gemini/

# HaloPSA and Servosity in Google Gemini

Google's Gemini comes in two shapes that consume MCP differently. Pick the one you have.

- **Gemini CLI** - local, self-serve, no hosting. Like Claude Code. **Start here if you can.**
- **Gemini app / web** - remote-only. You host the MCP server over HTTPS, like ChatGPT on the web.

## What you need

- **Gemini CLI** installed (for the local path) **or** a **Gemini app** account (for the remote path)
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

## Gemini app / web (remote)

The Gemini app doesn't launch a local binary. Host the MCP server over HTTPS first:

```bash
HALOPSA_TENANT=<tenant> HALOPSA_CLIENT_ID=<id> HALOPSA_CLIENT_SECRET=<secret> \
  halopsa-mcp --transport http --addr :7777
```

Expose `http://localhost:7777/mcp` as a public **HTTPS** URL via a secure tunnel (Cloudflare Tunnel, ngrok), then connect that endpoint in the Gemini app. The path is part of the endpoint: the server answers Streamable HTTP at `/mcp` and returns 404 at the bare root. Treat the URL as a credential. If you want a no-hosting path on Google, use Gemini CLI above instead.

## Troubleshooting

**Gemini CLI doesn't see the tools:** check the JSON in `~/.gemini/settings.json` (trailing commas, escaped backslashes on Windows) and restart. Confirm `halopsa-cli doctor` works first.

**"command not found":** the binary isn't on your PATH - the installer prints the line to add.

## What's next

- **Try a real workflow** at a free [Build Session](https://compoundingteams.com/build-sessions).
- **Prefer a desktop chat app?** [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) is the simplest local MCP host.

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
