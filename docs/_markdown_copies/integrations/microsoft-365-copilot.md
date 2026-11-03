> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/microsoft-365-copilot/

# HaloPSA and Servosity in Microsoft 365 Copilot / Copilot Studio: not supported yet

Microsoft 365 Copilot is the Copilot most MSPs and their clients actually have - it's bundled into the Microsoft 365 stack you already resell. So it's worth knowing exactly where MSP Skills stands with it.

**Not supported by MSP Skills yet.** Microsoft 365 Copilot, Copilot Studio, and Security Copilot consume MCP over **remote Streamable-HTTP only**. There is **no local-stdio path**: they reach an MCP server at a web address, not the `halopsa-mcp` / `servosity-mcp` binary on your laptop. Reaching them would mean running a connector in HTTP mode, and our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md#handling-credentials-safely) says not to do that until it is reprinted. Don't put a connector behind a tunnel or reverse proxy to reach Copilot either.

## What to use instead

- **Your team works in Microsoft 365 Copilot?** A [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick: it gives you that web address on day one. Check that your AI app is on the gateway's setup list (Conduit documents Microsoft 365 Copilot, [getting started](https://conduit.wyre.ai/docs/getting-started/)).
- **Want MSP Skills inside a Microsoft tool today?** [GitHub Copilot in VS Code](https://msp-skills.compoundingteams.com/integrations/github-copilot.md) runs the local binary, free, no hosting.
- **Prefer a chat app?** [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) and [the ChatGPT desktop app](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) run the same local connector.

## Note: free / consumer Copilot

The free consumer Copilot (copilot.microsoft.com, Windows Copilot) does **not** support user-supplied MCP servers. There is no MSP Skills path for it.

## What's next

- **Bring it to a Build Session.** Work your tenant + your hardest cross-client question live with the MSP cohort: [Build Sessions](https://compoundingteams.com/build-sessions).

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
