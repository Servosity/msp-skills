> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/answers/are-mcp-servers-safe-for-msp-client-data/

# Are MCP servers safe for MSP client data?

It depends on where the MCP server runs. A hosted one routes your data through someone else's cloud. **MSP Skills connectors run locally on your own machine** with your own credentials, send nothing to servers of ours, and return only query results to the AI - never a raw dump of client data. Every command is tiered read / write / destructive, and your own tenant scopes what the API key can reach. That design is what makes them safe.

## Why "where it runs" is the whole question

An MCP server is a connector that lets your AI call a real system on your behalf. The safety question is almost entirely about **location**:

- A **hosted** MCP server puts a third party between your AI and your data. Your client data flows through their infrastructure.
- A **local** MCP server, like every MSP Skills connector, is a binary on your laptop. Out of the box it talks straight to your vendor's API with your credentials. There is no middleman, no telemetry, and no servers of ours in the path. Anything that moves data elsewhere - `--deliver webhook:<url>`, a feedback endpoint you set, an HTTP MCP listener you expose - is a switch you throw and a URL you choose.

## The two layers of control

1. **Command tiers.** Reads run autonomously; writes require a `--dry-run` preview and your approval; destructive actions require a human. Each connector ships this policy in its `governance.md`.
2. **Server-side scoping.** Issue the API key inside your own PSA or backup tenant and scope it there. The connector can never exceed what your tenant granted - this control does not depend on the agent behaving.

## Read this next

- The full safety model: [the Trust Center →](https://msp-skills.compoundingteams.com/governance.md)
- The plain-language basics: [what is an MCP server? →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)
- How this compares to vendor AI: [MCP server vs vendor built-in AI →](https://msp-skills.compoundingteams.com/answers/mcp-server-vs-vendor-built-in-ai.md)
- How MSPs actually use this: [using Claude and ChatGPT with your PSA →](https://msp-skills.compoundingteams.com/answers/how-msps-use-claude-and-chatgpt-with-their-psa.md)
- See it on a real tool: [ConnectWise PSA →](https://msp-skills.compoundingteams.com/skills/connectwise-manage.md) · [Autotask →](https://msp-skills.compoundingteams.com/skills/autotask.md) · [HaloPSA →](https://msp-skills.compoundingteams.com/skills/halopsa.md) · [NinjaOne →](https://msp-skills.compoundingteams.com/skills/ninjaone.md) · [Datto RMM →](https://msp-skills.compoundingteams.com/skills/datto-rmm.md)
