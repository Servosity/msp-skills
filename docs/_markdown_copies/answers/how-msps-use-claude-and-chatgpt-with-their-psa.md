> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/answers/how-msps-use-claude-and-chatgpt-with-their-psa/

# How do MSPs use Claude and ChatGPT with their PSA?

MSPs install a free **MCP server connector** for their PSA that runs locally on their own machine, then ask plain-English questions in Claude, ChatGPT, Codex, or Copilot. The AI runs the underlying command for them - triage the queue, build a client card, find SLA breaches, run cross-client analytics. You paste one sentence to install, enter your API credentials once, and after that you just ask questions. No code, no dashboards, no exports.

## What it looks like day to day

You ask in plain English; your AI agent runs the command under the hood.

- "Triage my queue and tell me what's breaching SLA today."
- "Give me the whole picture for Acme Corp on one screen."
- "Which clients had a backup that hasn't succeeded in 7 days?"
- "How many SLA breaches per client last quarter?"

You never type the command. The connector keeps a local copy of your data, so cross-client questions come back instantly instead of as hundreds of slow API calls.

## Which AI works with this

- **Claude Desktop** and **Claude Code** connect to the local connector directly - the most common MSP-owner starting points.
- **ChatGPT** (Plus and up) connects to remote MCP servers over HTTPS; you expose the local binary via a bridge.
- **Codex CLI** and **GitHub Copilot** (Agent mode) run the local connector too.

Full per-agent setup is on the [which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) page.

## Read this next

- Is this safe with client data? [the Trust Center →](https://msp-skills.compoundingteams.com/governance.md) · [are MCP servers safe for client data? →](https://msp-skills.compoundingteams.com/answers/are-mcp-servers-safe-for-msp-client-data.md)
- New to the term? [what is an MCP server? →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md) · [what is a Claude Code Skill? →](https://msp-skills.compoundingteams.com/answers/what-is-a-claude-code-skill.md)
- See it on a real tool: [ConnectWise PSA →](https://msp-skills.compoundingteams.com/skills/connectwise-manage.md) · [Autotask →](https://msp-skills.compoundingteams.com/skills/autotask.md) · [HaloPSA →](https://msp-skills.compoundingteams.com/skills/halopsa.md) · [NinjaOne →](https://msp-skills.compoundingteams.com/skills/ninjaone.md) · [Datto RMM →](https://msp-skills.compoundingteams.com/skills/datto-rmm.md)
