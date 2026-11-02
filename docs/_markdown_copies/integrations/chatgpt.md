> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/chatgpt/

# HaloPSA and Servosity in ChatGPT

ChatGPT (Plus / Pro / Team / Business / Enterprise / Education) added native MCP support in September 2025 as **Developer Mode** beta. This guide installs the MSP Skills HaloPSA and Servosity MCP servers in ChatGPT.

**Two caveats up front:** ChatGPT Free does not yet expose Developer Mode, and ChatGPT only connects to **remote / HTTPS** MCP servers - not to local stdio binaries directly. MSP Skills ships local binaries. Most of them serve Streamable HTTP themselves, so you run one with `--transport http` and put a tunnel in front; six connectors are stdio-only and need a `supergateway` bridge first. Either way it is one extra step; everything else is the same as the [Claude Desktop install](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md).

## What you need

- **A paid ChatGPT plan** (Plus, Pro, Team, Business, Enterprise, or Education)
- **A HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token** - or both
- **A secure tunnel** (Cloudflare Tunnel, ngrok with HTTPS), plus **`supergateway`** if your connector is stdio-only
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

**Option A - run `halopsa-mcp` in HTTP mode (simplest, and what most connectors support).** Most MSP Skills MCP binaries parse `--transport http` and serve Streamable HTTP themselves:

```bash
HALOPSA_TENANT=<tenant> \
HALOPSA_CLIENT_ID=<id> \
HALOPSA_CLIENT_SECRET=<secret> \
halopsa-mcp --transport http --addr :7777
```

The server answers at `http://localhost:7777/mcp`. The bare root returns 404, so the path is part of the endpoint. Expose it via Cloudflare Tunnel:

```bash
cloudflared tunnel --url http://localhost:7777
```

Cloudflare prints a public HTTPS URL - register `<that-url>/mcp` in ChatGPT. Treat the URL as sensitive.

**Option B - `supergateway` bridge, for the stdio-only connectors.** Six connectors (avanan, blumira, connectwise-automate, cork, levelio, n-central) have no HTTP mode: their MCP binary never parses `--transport`, so passing it opens no listener at all. Publish those with a bridge:

```bash
HALOPSA_TENANT=<tenant> npx -y supergateway --stdio "halopsa-mcp" --port 7777
```

That serves SSE at `http://localhost:7777/sse`. For a consumer that takes Streamable HTTP only (Microsoft 365 Copilot), add `--outputTransport streamableHttp --streamableHttpPath /mcp`. Tunnel it the same way.

Do **not** reach for `mcp-remote` here: it bridges the other direction (a remote HTTPS server down to a local stdio client) and exits with `ERR_INVALID_URL` when handed `--stdio`. <!-- install-docs:ignore -->

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

**ChatGPT can't connect to the MCP server:** check the path first - the endpoint is `/mcp` (or `/sse` behind supergateway), never the bare root. Then confirm the tunnel URL responds when you visit it in a browser (you should get an MCP-protocol response or an error - not a connection refused). Test the underlying server with `halopsa-cli doctor` or `servosity-cli doctor` first.

**Developer Mode toggle missing:** you're on the Free tier, or your plan admin disabled it. Check your ChatGPT plan at [chatgpt.com/#pricing](https://chatgpt.com/).

**`supergateway: command not found`:** install Node.js (the bridge runs under npx). `brew install node` on macOS, the official Windows installer, or whichever package manager your distro uses. You only need it for the stdio-only connectors in Option B.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions) - we'll work it live with the MSP cohort.
- **No paid ChatGPT plan?** Use [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) instead - Claude Desktop is free and supports MCP natively without HTTPS bridges.

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
