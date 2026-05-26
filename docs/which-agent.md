# Which agent do I have?

`msp-skills` works with two families of AI agents. Pick yours, follow the matching install link, and skip the rest.

## You typed `claude` in a terminal and got an interactive AI prompt

You have **Claude Code** (or Codex CLI). Both are skill-capable agents. They read `SKILL.md` files directly and drive CLIs.

Install path: [install-skill.md](./install-skill.md).

## You downloaded Claude Desktop from claude.ai

You have **Claude Desktop**. It is a Mac / Windows app with a chat window. It does not load `SKILL.md` files; it talks to MCP servers instead.

Install path: [install-mcp.md](./install-mcp.md).

## You use ChatGPT in the desktop app, not the website

You have **ChatGPT Desktop**. Same story as Claude Desktop: it talks to MCP servers, not Skills.

Install path: [install-mcp.md](./install-mcp.md).

## You only use ChatGPT or Claude on the web

Web-only chat surfaces do not yet have a way to install MCP servers or Skills. There is nothing to install here for that path. When the web surfaces add MCP, we will document it.

## You use Cursor, Cline, Aider, Continue, or another agent

These agents are evolving fast. Some load Skills, some only use MCP, some have their own format. Check your agent's docs for "MCP server" or "Skill" support and pick the matching install path here.

## Still not sure

If you are working with an MSP-internal team that set up your AI environment, ask them. If you set it up yourself and do not remember: open whatever you call up to chat with the AI and look at the menu / settings. The presence of a "skills" or "MCP" section in the settings tells you which family.
