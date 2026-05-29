# Which AI agent? Install MSP Skills in any of them.

MSP Skills works with **any AI tool that speaks MCP** (Model Context Protocol). This page tells you which AI tools speak MCP today, how to install the HaloPSA and Servosity skills in each one, and the gotchas to watch for.

Pick the row for your agent and follow that section. If you're not sure what you have, jump to ["I don't know what I have"](#i-dont-know-what-i-have-yet) at the bottom.

> **All paths assume you already ran the installer** for at least one skill - see the [main README](../README.md#install-in-60-seconds) for the one-liners. The installer drops `halopsa-cli` + `halopsa-mcp` (and / or `servosity-cli` + `servosity-mcp`) into your user `bin` path. After that, this page is purely about pointing your AI agent at those binaries.

## Quick lookup

**The 5 MSP-owner primaries - lead here:**

| AI agent | Supports MCP? | Config file or panel | Verified |
| --- | --- | --- | --- |
| [Claude Desktop](#claude-desktop-anthropic) | Yes | `claude_desktop_config.json` | 2026-05-28 |
| [ChatGPT (Plus / Pro+)](#chatgpt-openai-plus-pro-team-business-enterprise-education) | Yes¹ | Settings → Connectors | 2026-05-28 |
| [Claude Code](#claude-code-anthropic-cli) | Yes | `claude mcp add ...` | 2026-05-28 |
| [Codex CLI](#codex-cli-openai) | Yes | `~/.codex/config.toml` | 2026-05-28 |
| [Claude Cowork](#claude-cowork-anthropic-desktop-agent) | Yes | paste-prompt + Settings > Connectors | 2026-05-29 |

**Long-tail and developer IDEs:**

| AI agent | Supports MCP? | Config file or panel | Verified |
| --- | --- | --- | --- |
| [Cursor](#cursor) | Yes | `.cursor/mcp.json` | 2026-05-28 |
| [Windsurf](#windsurf) | Yes | Cascade → MCP Servers | 2026-05-28 |
| [Cline](#cline-vs-code) | Yes | `Ctrl+Shift+M` / `⌘+Shift+M` | 2026-05-28 |
| [Continue.dev](#continuedev-vs-code--jetbrains) | Yes (Agent mode) | `config.json` | 2026-05-28 |
| [Gemini CLI](#gemini-cli-google) | Yes | `~/.gemini/config.json` | 2026-05-28 |
| [GitHub Copilot in VS Code](#github-copilot-in-vs-code) | Yes (Agent mode) | `mcp.json` (key: `servers`) | 2026-05-28 |
| [Zed](#zed) | Partial² | `context_servers` in `settings.json` | 2026-05-28 |
| [JetBrains AI Assistant / Junie](#jetbrains-ai-assistant--junie) | _Verify_ | _verify_ | _pending_ |

**Other agents:**

| AI agent | Status | Install path | Verified |
| --- | --- | --- | --- |
| [Hermes](#hermes-nous-research) | Yes (via MCP) | `hermes skills install Servosity/msp-skills/skills/<name>` | 2026-05-29 (paper - install not yet end-to-end tested) |
| [OpenClaw](#openclaw) | Yes (GA) | `openclaw skills install git:Servosity/msp-skills/skills/<name>@main` | 2026-05-29 (frontmatter spec match; subdir install path needs final dogfood) |

**Or install across all 51+ supported agents at once:**

| Tool | What it installs | Install path | Verified |
| --- | --- | --- | --- |
| [`npx skills`](#install-across-all-your-agents-at-once) | SKILL.md symlinks into every supported agent dir | `npx skills add Servosity/msp-skills` | 2026-05-29 (the [agentskills.io](https://agentskills.io) spec entry point; binary install is a separate step) |

¹ ChatGPT requires a paid plan (Plus, Pro, Team, Business, Enterprise, or Education). Free tier does not yet expose Developer Mode. ChatGPT connects only to **remote** MCP servers; local stdio binaries like MSP Skills need an HTTPS bridge (e.g. `mcp-remote`).
² Zed supports MCP Tools and Prompts today, not the full spec. Most MSP Skills functionality works; some advanced features may not.

---

## Claude Desktop (Anthropic)

The Mac / Windows app with a chat window. First-class MCP host - the original.

**Config file:**
- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

**Setup** (Settings → Developer → Edit Config), add the block for HaloPSA and / or Servosity:

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

**Restart Claude Desktop fully** (quit and reopen). An MCP indicator appears in the bottom-right of the chat input once the server connects. Source: [modelcontextprotocol.io/quickstart/user](https://modelcontextprotocol.io/quickstart/user).

---

## Claude Code (Anthropic CLI)

Skill-capable CLI. Reads `SKILL.md` directly and drives `halopsa-cli` / `servosity-cli`. You don't actually need MCP for Claude Code - just install the skill normally - but you can register the MCP server too if you prefer that interface.

**Skill path (recommended):** the install script symlinks the skill into `~/.claude/skills/`. Restart Claude Code; invoke with `use halopsa` or `use servosity`.

**MCP path (alternative):**

```bash
claude mcp add halopsa -- halopsa-mcp
claude mcp add servosity -- servosity-mcp
claude mcp list   # confirm status
```

For HTTP / remote: `claude mcp add --transport http <name> <url>`. Source: [code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp).

---

## Claude Cowork (Anthropic desktop agent)

Anthropic's desktop agent, GA'd March 2026. Sits between Claude Desktop (chat UI, no shell) and Claude Code (terminal-native): Cowork runs shell on your behalf when you ask it to, and exposes a Settings > Customize > Connectors UI for MCP servers. Either path works for MSP Skills.

**Skill path (recommended) - paste this into Cowork:**

> Install the HaloPSA Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex`. Then authenticate with `halopsa-cli auth login` and run `halopsa-cli --help` to explore.

(Replace `halopsa` with `servosity` for the Servosity skill.)

Cowork will detect your shell, run the installer, walk authentication, and confirm. No JSON editing, no Connector configuration, no terminal commands you type yourself.

**MCP path (alternative):** Settings > Customize > Connectors > **+** > paste an MCP server URL. This is the canonical Cowork path for remote/HTTPS MCP servers (the Composio integration pattern). MSP Skills' binaries are local stdio, so use the Skill path above for them.

Source: [Composio - HubSpot + Claude Cowork docs](https://composio.dev/toolkits/hubspot/framework/claude-cowork) shows the canonical Cowork install patterns.

---

## Codex CLI (OpenAI)

Skill-capable CLI like Claude Code and Cowork. Reads the same `SKILL.md`. Also supports MCP.

**Skill path (recommended):** the install script registers the skill with Codex; invoke `use halopsa` or `use servosity`.

**MCP path (alternative):** edit `~/.codex/config.toml` (global) or `.codex/config.toml` (project):

```toml
[mcp_servers.halopsa]
command = "halopsa-mcp"
env = { HALOPSA_TENANT = "<your-tenant>", HALOPSA_CLIENT_ID = "<id>", HALOPSA_CLIENT_SECRET = "<secret>" }

[mcp_servers.servosity]
command = "servosity-mcp"
env = { SERVOSITY_MSP_TOKEN = "<token>" }
```

Codex supports stdio + Streamable HTTP, and runs concurrent read-only tools when advertised. Source: [developers.openai.com/codex/mcp](https://developers.openai.com/codex/mcp).

---

## ChatGPT (OpenAI) - Plus, Pro, Team, Business, Enterprise, Education

**Free tier does not yet expose Developer Mode**; you need a paid plan. ChatGPT speaks MCP through "Developer Mode" connectors (beta as of Sept 2025).

**Important transport caveat:** ChatGPT connects only to **remote** MCP servers (HTTPS). MSP Skills binaries are local (stdio). To use them with ChatGPT, expose them over HTTPS:

**Option 1 - `mcp-remote` bridge (simplest):**

```bash
# In one terminal, expose halopsa-mcp over HTTPS via the bridge
mcp-remote halopsa-mcp --port 7777
# (Repeat for servosity-mcp on a different port)
```

Then in ChatGPT: Settings → Advanced → **enable Developer Mode** → Connectors tab → Add MCP server → URL = `https://<your-tunnel>:7777`. If your tunnel is local-only (e.g. ngrok), Developer Mode will refuse without HTTPS. Use a TLS tunnel (Cloudflare Tunnel, ngrok with HTTPS, your own reverse proxy).

**Option 2 - run `halopsa-mcp` / `servosity-mcp` in HTTP mode** (no bridge):

```bash
HALOPSA_TENANT=... halopsa-mcp --transport http --addr :7777
```

Then expose `https://<your-public-host>:7777` to ChatGPT via Developer Mode. Always behind a secure tunnel; never expose your MCP server bare on the internet.

Sources: [InfoQ ChatGPT MCP](https://www.infoq.com/news/2025/10/chat-gpt-mcp/) · [OpenAI Connectors help](https://help.openai.com/en/articles/11487775-connectors-in-chatgpt) · [Developer Mode docs](https://platform.openai.com/docs/guides/developer-mode).

---

## Cursor

AI-first code editor. Native MCP. Stdio, SSE, and Streamable HTTP.

**Config:** `.cursor/mcp.json` (project) or `~/.cursor/mcp.json` (global).

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": { "HALOPSA_TENANT": "<tenant>", "HALOPSA_CLIENT_ID": "<id>", "HALOPSA_CLIENT_SECRET": "<secret>" }
    },
    "servosity": {
      "command": "servosity-mcp",
      "env": { "SERVOSITY_MSP_TOKEN": "<token>" }
    }
  }
}
```

Or use the Settings → MCP UI to add it via the panel. Cursor supports config interpolation for env vars. Source: [cursor.com/docs/mcp](https://cursor.com/docs/mcp).

---

## Windsurf

The Cascade panel speaks MCP. Stdio, Streamable HTTP, and SSE. OAuth supported.

**Setup:** Click the **MCP Marketplace** icon in the Cascade panel, or Settings → Cascade → MCP Servers → Add. Use the same `command` / `env` block shape as Claude Desktop.

April 2026 update: OAuth fixes; Streamable HTTP replacing SSE. Source: [docs.windsurf.com/windsurf/cascade/mcp](https://docs.windsurf.com/windsurf/cascade/mcp).

---

## Cline (VS Code)

VS Code extension. Built-in MCP Marketplace.

**Setup:** `Ctrl+Shift+M` (Windows / Linux) or `⌘+Shift+M` (macOS) opens the marketplace. Or click the **MCP Servers** icon → **Configure** tab → add `halopsa` / `servosity` with the `command` + `env` block.

**Windows gotcha:** if Cline reports `spawn npx ENOENT`, wrap the command as `cmd` + `npx` per Cline's docs. MSP Skills' own binaries don't use `npx`, so this only matters if you're also installing Node-based MCP servers alongside.

Source: [docs.cline.bot/mcp/adding-and-configuring-servers](https://docs.cline.bot/mcp/adding-and-configuring-servers).

---

## Continue.dev (VS Code / JetBrains)

Open-source AI assistant. **MCP tools only work in Agent mode.** Stdio, SSE, Streamable HTTP. OAuth + env-var templating.

**Setup:** edit Continue's `config.json` (per-IDE path varies; see Continue docs) and add:

```json
{
  "mcpServers": [
    {
      "name": "halopsa",
      "command": "halopsa-mcp",
      "env": { "HALOPSA_TENANT": "<tenant>", "HALOPSA_CLIENT_ID": "<id>", "HALOPSA_CLIENT_SECRET": "<secret>" }
    },
    {
      "name": "servosity",
      "command": "servosity-mcp",
      "env": { "SERVOSITY_MSP_TOKEN": "<token>" }
    }
  ]
}
```

**Switch Continue to Agent mode** in the chat panel for tools to fire. Source: [docs.continue.dev/customize/deep-dives/mcp](https://docs.continue.dev/customize/deep-dives/mcp).

---

## Gemini CLI (Google)

Google's CLI agent. Native MCP - Gemini API + SDK + CLI all speak it as of Mar–Apr 2026.

**Setup:** edit `~/.gemini/config.json` (or the path Gemini CLI prints with `gemini config path`) and add an `mcpServers` block with the same shape as Claude Desktop. Gemini CLI's tool list will include the MCP tools after restart.

Google Cloud also offers fully-managed remote MCP servers for Google services - not relevant to MSP Skills, but worth knowing about. Source: [Google Cloud Blog - official MCP support](https://cloud.google.com/blog/products/ai-machine-learning/announcing-official-mcp-support-for-google-services).

---

## GitHub Copilot in VS Code

GA in VS Code 1.102 (July 2025). Most enterprise-ready MCP client by reputation: sandboxing, OAuth, Settings Sync, curated marketplace.

**Critical gotchas:**

1. **Config file is `mcp.json`** (not `settings.json`).
2. **Root key is `servers`** (NOT `mcpServers` like Claude). Easy to miss.
3. **Works only in Copilot Agent mode** - pick "Agent" in the Copilot Chat mode dropdown.

```json
{
  "servers": {
    "halopsa": {
      "type": "stdio",
      "command": "halopsa-mcp",
      "env": { "HALOPSA_TENANT": "<tenant>", "HALOPSA_CLIENT_ID": "<id>", "HALOPSA_CLIENT_SECRET": "<secret>" }
    },
    "servosity": {
      "type": "stdio",
      "command": "servosity-mcp",
      "env": { "SERVOSITY_MSP_TOKEN": "<token>" }
    }
  }
}
```

Or install through the Extensions view: search `@mcp` and install from the gallery. Source: [code.visualstudio.com/docs/copilot/customization/mcp-servers](https://code.visualstudio.com/docs/copilot/customization/mcp-servers).

---

## Zed

Calls them "context servers." **Partial MCP support** - Tools and Prompts work today; the full spec is in progress (Zed 1.0 ships agentic editing as first-class).

**Setup:** open the Agent Panel (`Cmd+Shift+A`) → top-right menu → install server extensions. Or edit `settings.json`:

```json
{
  "context_servers": {
    "halopsa": {
      "command": { "path": "halopsa-mcp", "env": { "HALOPSA_TENANT": "<tenant>", "HALOPSA_CLIENT_ID": "<id>", "HALOPSA_CLIENT_SECRET": "<secret>" } }
    },
    "servosity": {
      "command": { "path": "servosity-mcp", "env": { "SERVOSITY_MSP_TOKEN": "<token>" } }
    }
  }
}
```

Most MSP Skills tools will appear in Zed's tool list; some advanced features that depend on full-spec MCP may not work yet. Source: [zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp).

---

## JetBrains AI Assistant / Junie

JetBrains reportedly added MCP support in late 2025; we have not yet verified the install path against primary docs. If you use JetBrains AI Assistant or Junie and want MSP Skills support documented here, open an issue with the path you used and we'll add it.

---

## Hermes (Nous Research)

Hermes is Nous Research's autonomous research agent. It speaks MCP natively per the [Nous Research docs](https://hermes-agent.nousresearch.com/docs) and has its own SKILL.md installer for Claude-Code-compatible skills.

**MSP Skills works two ways under Hermes:**

1. **Via MCP** (preferred - same path as Claude Desktop / Cursor). Hermes loads `halopsa-mcp` and `servosity-mcp` as MCP servers via its `mcp_servers:` config. Same env vars (`HALOPSA_TENANT`/`CLIENT_ID`/`CLIENT_SECRET`, `SERVOSITY_MSP_TOKEN`).

2. **Via Skill install** (uses the upstream cli-printing-press template). From the Hermes CLI:

   ```bash
   hermes skills install servosity/msp-skills/skills/halopsa --force
   hermes skills install servosity/msp-skills/skills/servosity --force
   ```

   Inside a Hermes chat session:

   ```
   /skills install servosity/msp-skills/skills/halopsa --force
   ```

**Honest status:** the cli-printing-press alignment work for Hermes (PR #655) ships the frontmatter + README install sections, but end-to-end install of a printed CLI from a non-canonical path (i.e. `servosity/msp-skills/...` instead of `mvanhorn/printing-press-library/cli-skills/...`) has not been verified end-to-end by upstream. The MCP path (#1) is the more reliable route until Hermes confirms arbitrary GitHub-path resolution.

---

## OpenClaw

[OpenClaw](https://docs.openclaw.ai) is GA (latest stable as of May 2026). Browser-controlled AI agent with native Skill support (SKILL.md superset of Anthropic's spec) and native MCP. Public skill registry at [ClawHub](https://clawhub.ai).

**One-time setup:** install OpenClaw itself ([install docs](https://docs.openclaw.ai/install)).

```bash
# macOS / Linux / WSL2
curl -fsSL https://openclaw.ai/install.sh | bash

# Windows (PowerShell)
iwr -useb https://openclaw.ai/install.ps1 | iex
```

**Install MSP Skills' HaloPSA skill:**

```bash
openclaw skills install git:Servosity/msp-skills/skills/halopsa@main
```

**Install the Servosity skill:**

```bash
openclaw skills install git:Servosity/msp-skills/skills/servosity@main
```

OpenClaw reads each skill's `SKILL.md`, sees the `metadata.openclaw.requires.bins` block (already in our frontmatter via cli-printing-press), and installs the required CLI. Run `openclaw doctor` to confirm the install + env vars.

**Register the MCP servers too** (optional - the Skill path already gives you most of the value):

```bash
openclaw mcp set halopsa '{"command":"halopsa-mcp"}'
openclaw mcp set servosity '{"command":"servosity-mcp"}'
```

See the [OpenClaw MCP CLI docs](https://docs.openclaw.ai/cli/mcp) for the canonical command shape.

**Note on subdirectory install path.** OpenClaw's monorepo subdirectory `git:` syntax for `skills install` is documented in the ClawHub skill-format spec; if `git:Servosity/msp-skills/skills/halopsa@main` does not resolve in your build, fall back to cloning the repo locally and running `openclaw skills install ./msp-skills/skills/halopsa`. We are updating the dogfood receipt in the next release.

---

## Install across all your agents at once

If you run more than one AI agent - Claude Code, Codex, Cursor, Cline, Continue.dev, OpenCode, Windsurf, or any of 51+ others - the `npx skills` CLI installs MSP Skills' SKILL.md files across all of them in one command. It's the canonical install tool for the open [Agent Skills spec](https://agentskills.io) that MSP Skills conforms to.

```bash
npx skills add Servosity/msp-skills@latest
```

This symlinks (or copies, if you prefer) `skills/halopsa/SKILL.md` and `skills/servosity/SKILL.md` into every supported agent's skill directory at once. `@latest` pulls the most recently released tag so you don't track a moving `main`. Requires Node.js (which `npx` ships with). After it runs, follow up with the per-skill installer to drop the CLI + MCP binaries on your PATH:

```bash
# macOS / Linux / WSL
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)

# Windows PowerShell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

**Why this path is secondary.** Most MSP business owners don't run multiple AI agents and don't have Node.js installed by default. The paste-prompt path at the top of each per-skill README is the simpler primary recommendation. The `npx skills` path shines when:

- You're a senior tech / engineer running 3+ different agents
- You want spec-conformant SKILL.md registration as a one-shot operation
- You're already working in the [agentskills.io](https://agentskills.io) ecosystem

**Pin a specific version.** `@latest` follows the most recent release. To pin to an exact commit or tag instead:

```bash
npx skills add Servosity/msp-skills@<commit-sha-or-tag>
```

Each skill in this repo is tagged independently (`halopsa-v0.1.0`, `servosity-v0.1.0`, etc.) so per-skill version pinning requires the per-skill installer, not the cross-agent CLI.

---

## I don't know what I have yet

A 30-second triage:

| You typed this and got an AI prompt | You have | Install path |
| --- | --- | --- |
| `claude` in a terminal | **Claude Code** | Skill or MCP - see above |
| `codex` in a terminal | **Codex CLI** | Skill or MCP - see above |
| `gemini` in a terminal | **Gemini CLI** | MCP - see above |
| You downloaded Claude Desktop from claude.ai | **Claude Desktop** | MCP - see above |
| You're chatting with ChatGPT in a desktop app or browser | **ChatGPT** | MCP (paid plan only) - see above |
| You opened Cursor / Windsurf / Cline / VS Code with Copilot | The IDE name above | MCP - see above |
| You only chat in `claude.ai` or `chatgpt.com` and don't use any of the above | Web-only chat | No install yet - web-only surfaces don't expose MCP. Claude Desktop or one of the paid ChatGPT plans is the closest. |
| You're not sure | - | Open whatever you call up to chat. Check the menu / settings for "MCP" or "Skills." That tells you which family. |

If you're working with an MSP-internal team that set up your AI environment, ask them. If you set it up yourself and don't remember, search your applications folder for "Claude" or "Cursor" or "ChatGPT" - whichever has an app is what you have.

---

_All "Verified" dates reflect the date this document was last confirmed against primary sources. If you find a step that no longer works, please open an issue or send a PR. Last updated: 2026-05-28._
