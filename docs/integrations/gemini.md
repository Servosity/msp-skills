---
layout: default
title: "HaloPSA + Servosity in Google Gemini (Gemini CLI + app)"
description: "Use HaloPSA and Servosity in Google Gemini via MCP. Gemini CLI is local and self-serve; the Gemini app is remote (HTTPS). Step-by-step for both. Free MSP Skills, runs on your machine."
permalink: /integrations/gemini/
faqs:
  - q: "Does Google Gemini support HaloPSA?"
    a: "Yes, via MSP Skills' free HaloPSA MCP server. Gemini CLI runs it locally (add an mcpServers block to ~/.gemini/settings.json). The Gemini app connects to it over HTTPS, like ChatGPT."
  - q: "Does Gemini support MCP?"
    a: "Yes. Google's Gemini API, SDK, and CLI all speak MCP as of early-mid 2026. Gemini CLI runs local stdio MCP servers directly; the Gemini app uses remote HTTPS endpoints."
  - q: "What's the difference between Gemini CLI and the Gemini app for MCP?"
    a: "Gemini CLI is local - it launches the MSP Skills binary on your machine, no hosting, just like Claude Code. The Gemini app does not launch local binaries, so you host the MCP server over HTTPS and connect that URL, like ChatGPT's Developer Mode."
---

# HaloPSA and Servosity in Google Gemini

Google's Gemini comes in two shapes that consume MCP differently. Pick the one you have.

- **Gemini CLI** - local, self-serve, no hosting. Like Claude Code. **Start here if you can.**
- **Gemini app / web** - remote-only. You host the MCP server over HTTPS, like ChatGPT.

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

Expose `http://localhost:7777` as a public **HTTPS** URL via a secure tunnel (Cloudflare Tunnel, ngrok), then connect that endpoint in the Gemini app. Treat the URL as a credential. This is the same pattern as the [ChatGPT integration](/integrations/chatgpt/) - if you want a no-hosting path on Google, use Gemini CLI above instead.

## Troubleshooting

**Gemini CLI doesn't see the tools:** check the JSON in `~/.gemini/settings.json` (trailing commas, escaped backslashes on Windows) and restart. Confirm `halopsa-cli doctor` works first.

**"command not found":** the binary isn't on your PATH - the installer prints the line to add.

## What's next

- **Try a real workflow** at a free [Build Session](https://compoundingteams.com/build-sessions).
- **Prefer a desktop chat app?** [Claude Desktop](/integrations/claude-desktop/) is the simplest local MCP host.

[← Back to main install](/#install-in-60-seconds)
