---
layout: default
title: "HaloPSA + Servosity in ChatGPT (Plus / Pro / Team / Enterprise)"
description: "Use HaloPSA and Servosity in ChatGPT via MCP Developer Mode. Step-by-step setup with the mcp-remote bridge for local stdio servers. Free MSP Skills, paid ChatGPT tier required (Plus/Pro/Team/Business/Enterprise/Education)."
permalink: /integrations/chatgpt/
faqs:
  - q: "Does ChatGPT support HaloPSA?"
    a: "Yes, via MSP Skills' free HaloPSA MCP server connected through ChatGPT Developer Mode. ChatGPT requires a paid plan (Plus, Pro, Team, Business, Enterprise, or Education - Free does not yet expose Developer Mode). MSP Skills' MCP server is stdio; expose it over HTTPS with the mcp-remote bridge or your own tunnel."
  - q: "Does ChatGPT support MCP?"
    a: "Yes. OpenAI shipped native MCP support for ChatGPT in September 2025 as Developer Mode beta. Available on Plus, Pro, Team, Business, Enterprise, and Education plans. ChatGPT connects only to remote (HTTPS) MCP servers, not to local stdio binaries directly."
  - q: "Can I use ChatGPT free with HaloPSA?"
    a: "Not yet - the ChatGPT Free tier does not expose Developer Mode as of 2026-05-28. Plus, Pro, Team, Business, Enterprise, and Education plans all support MCP via Developer Mode. Watch OpenAI's announcements for Free-tier MCP."
  - q: "Do I need to host my own server for ChatGPT to use MSP Skills?"
    a: "Effectively yes - ChatGPT only talks to HTTPS endpoints, and MSP Skills' MCP servers are local. You bridge them over HTTPS using mcp-remote (the simplest path), Cloudflare Tunnel, or ngrok with HTTPS. Treat the resulting URL as sensitive; never expose your MCP server bare on the internet."
---

# HaloPSA and Servosity in ChatGPT

ChatGPT (Plus / Pro / Team / Business / Enterprise / Education) added native MCP support in September 2025 as **Developer Mode** beta. This guide installs the MSP Skills HaloPSA and Servosity MCP servers in ChatGPT.

**Two caveats up front:** ChatGPT Free does not yet expose Developer Mode, and ChatGPT only connects to **remote / HTTPS** MCP servers - not to local stdio binaries directly. MSP Skills ships local binaries, so you bridge them over HTTPS using `mcp-remote` or your own tunnel. The bridge is one extra step; everything else is the same as the [Claude Desktop install](/integrations/claude-desktop/).

## What you need

- **A paid ChatGPT plan** (Plus, Pro, Team, Business, Enterprise, or Education)
- **A HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token** - or both
- **`mcp-remote`** or a secure tunnel (Cloudflare Tunnel, ngrok with HTTPS)
- A terminal for install (after that, ChatGPT chat handles the rest)

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

Install only the skills you'll use.

## Step 2 - Choose your HTTPS bridge

**Option A - `mcp-remote` bridge (simplest).** In a terminal:

```bash
npx mcp-remote halopsa-mcp --port 7777 \
  --env HALOPSA_TENANT=<tenant> \
  --env HALOPSA_CLIENT_ID=<id> \
  --env HALOPSA_CLIENT_SECRET=<secret>
```

Then expose `https://localhost:7777` to the public internet via Cloudflare Tunnel:

```bash
cloudflared tunnel --url http://localhost:7777
```

Cloudflare prints a public HTTPS URL. Treat it as sensitive.

**Option B - run `halopsa-mcp` natively in HTTP mode.** The MSP Skills MCP binaries support `--transport http`:

```bash
HALOPSA_TENANT=<tenant> \
HALOPSA_CLIENT_ID=<id> \
HALOPSA_CLIENT_SECRET=<secret> \
halopsa-mcp --transport http --addr :7777
```

Then tunnel it the same way (Cloudflare Tunnel / ngrok HTTPS).

(Repeat the equivalent setup for `servosity-mcp` on a different port.)

## Step 3 - Enable Developer Mode in ChatGPT

In ChatGPT: **Settings → Advanced → Developer Mode** (toggle on). Then go to **Connectors → Add MCP server**. Enter the public HTTPS URL from your tunnel.

## Step 4 - Ask a real question

In ChatGPT chat:

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

ChatGPT discovers the MCP tools the connector exposes and runs them.

## Cost

- **MSP Skills:** Free (Apache-2.0).
- **ChatGPT plan:** see [OpenAI's pricing](https://openai.com/chatgpt/pricing). Plus, Pro, Team, Business, Enterprise, and Education all support MCP. Free does not (as of 2026-05-28).
- **HaloPSA / Servosity access:** your existing license / partner agreement.
- **Tunneling:** Cloudflare Tunnel is free. ngrok has a free tier with HTTPS.

## Security

- The HTTPS tunnel URL is effectively a key to your MCP server. Treat it like a credential. **Don't post it publicly.**
- For team / enterprise deployments, host MSP Skills on a server inside your network and expose via Cloudflare Access (or equivalent SSO-gated tunnel) so only authenticated users hit it.
- HaloPSA + Servosity credentials are in your local environment, never transmitted to MSP Skills or to OpenAI.

## Troubleshooting

**ChatGPT can't connect to the MCP server:** confirm the tunnel URL responds when you visit it in a browser (you should get an MCP-protocol response or an error - not a connection refused). Test the underlying server with `halopsa-cli doctor` or `servosity-cli doctor` first.

**Developer Mode toggle missing:** you're on the Free tier, or your plan admin disabled it. Check your ChatGPT plan at [chatgpt.com/#pricing](https://chatgpt.com/).

**`mcp-remote: command not found`:** install Node.js (the bridge runs under npx). `brew install node` on macOS, the official Windows installer, or whichever package manager your distro uses.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions) - we'll work it live with the MSP cohort.
- **No paid ChatGPT plan?** Use [Claude Desktop](/integrations/claude-desktop/) instead - Claude Desktop is free and supports MCP natively without HTTPS bridges.

[← Back to main install](/#install-in-60-seconds)
