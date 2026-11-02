> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/answers/what-is-a-claude-code-skill/

# What is a Claude Code Skill?

A **Claude Code Skill** is a package of instructions plus a tool that an AI agent reads to do a real job - like connecting to your PSA and answering questions about it. It's the same idea most AI surfaces call an **MCP server** or **connector**, just the name Claude Code uses. Claude Code reads the Skill directly; other agents load the very same package as an MCP server. One install gives you both, so you use whichever your AI speaks.

## The one-thing-many-names map

**One thing, many names.** What this site calls an **MCP server** is what ChatGPT calls an *app* or *connector*, Claude on the web calls a *connector*, Microsoft Copilot calls a *connector*, and Claude Code calls a *Skill*. Same standard underneath: the [Model Context Protocol](https://modelcontextprotocol.io).

If your AI shows you "connectors" or "apps," that's where these go. If you use Claude Code, it's a "Skill." Underneath, it's all the **Model Context Protocol** - one open standard.

## Why an MSP cares

A plain chatbot only knows what it was trained on plus what you paste in. A Skill (or MCP server) lets your AI **call your real systems** - read a ticket, list stale backups, pull a client's contract burn - using credentials you supply, on your own machine. That's the difference between an AI that guesses and one that answers from your live data.

These particular connectors also keep a **local searchable copy** of your tool's data, so cross-client questions ("across all 47 clients...") are one instant lookup instead of hundreds of slow API calls.

## How you get one

- **Paste one sentence** into Claude Code or Codex - your agent reads the Skill and installs the binary.
- **Run a one-line installer** for your OS (bash or PowerShell).

Neither path requires code. After that you ask questions in plain English and the AI runs the command for you.

## Read this next

- The fuller glossary: [what is an MCP server? →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)
- How MSPs use it daily: [using Claude and ChatGPT with your PSA →](https://msp-skills.compoundingteams.com/answers/how-msps-use-claude-and-chatgpt-with-their-psa.md)
- Vendor AI vs this: [MCP server vs vendor built-in AI →](https://msp-skills.compoundingteams.com/answers/mcp-server-vs-vendor-built-in-ai.md)
- Is it safe? [are MCP servers safe for client data? →](https://msp-skills.compoundingteams.com/answers/are-mcp-servers-safe-for-msp-client-data.md) · [the Trust Center →](https://msp-skills.compoundingteams.com/governance.md)
- See it on a real tool: [ConnectWise PSA →](https://msp-skills.compoundingteams.com/skills/connectwise-manage.md) · [Autotask →](https://msp-skills.compoundingteams.com/skills/autotask.md) · [HaloPSA →](https://msp-skills.compoundingteams.com/skills/halopsa.md) · [NinjaOne →](https://msp-skills.compoundingteams.com/skills/ninjaone.md) · [Datto RMM →](https://msp-skills.compoundingteams.com/skills/datto-rmm.md)
