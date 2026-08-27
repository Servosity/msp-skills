# WordPress + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the WordPress
> API. Not affiliated with, endorsed by, or sponsored by WordPress Foundation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/wordpress/">
    <img src="../../docs/assets/video/wordpress/animated-og.gif" alt="WordPress demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/wordpress/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Publish and manage WordPress pages, posts, media Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the WordPress skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `wordpress-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `wordpress-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `wordpress-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the WordPress MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `wordpress-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download WordPress MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/wordpress-v0.1.3/wordpress-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every WordPress release on the [releases page](https://github.com/servosity/msp-skills/releases?q=wordpress).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install wordpress@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the WordPress Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.ps1 | iex`. Then authenticate per the README and run `wordpress-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.sh)
```

The installer drops both `wordpress-cli` (the CLI) and `wordpress-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
wordpress-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update wordpress@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/wordpress --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/wordpress --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `wordpress-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the wordpress skill from https://github.com/servosity/msp-skills/tree/main/skills/wordpress. The skill defines how its required CLI (`wordpress-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your WordPress portal):

```bash
WORDPRESS_BASIC_AUTH=<value> wordpress-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Publish a landing page from HTML without opening wp-admin? | `wordpress-cli pages create --title "Spring Promo" --content "<h1>Spring Promo</h1>" --status publish` |
| Which pages are still sitting in draft? | `wordpress-cli pages list --status draft` |
| Update a live page's content in place? | `wordpress-cli pages update 42 --content "<h1>Updated copy</h1>"` |
| Upload a hero image and get its media id? | `wordpress-cli media upload ./hero.png --alt-text "Spring promo hero"` |
| Mirror the whole site locally for offline search and backup? | `wordpress-cli workflow archive` |
| Find every page or post that mentions an old phrase (after a `workflow archive`)? | `wordpress-cli search "summer sale"` |
| Publish a blog post with categories and tags? | `wordpress-cli posts create --title "Patch Tuesday recap" --content "<p>This month...</p>" --status publish` |
| Which authors can I assign content to? | `wordpress-cli users list` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most WordPress integrations and MCP servers proxy each question into a live API call. That's fine for editing one record. It gets slow and noisy when you ask "find every page and post across the whole site that still mentions last year's pricing or a dead promo link."

This skill syncs WordPress into a **local SQLite mirror** with full-text search. That cross-content question becomes one instant offline query, and the AI sees the matching rows, not a dump of every page's HTML. Compound commands like `workflow archive` and `workflow status` mirror and inventory every page, post, media item, category, tag, and user in one pass - work a stateless API wrapper can't do.

## The pain this closes

Routine site edits - swap a promo banner, fix a phone number, push a new landing page - mean logging into wp-admin and clicking through the block editor screen by screen. The Classic Editor plugin still reports over 10 million active installs on the WordPress.org plugin directory: a standing vote against that editor for everyday content work. Across a stack of client sites that browser-clicking is unbillable busywork, and the official WP-CLI - the scriptable alternative - needs SSH access to the server's shell that many managed or hosted sites never give you.

This skill closes that gap from the terminal or your AI agent, over the REST API your site already exposes:

- **Publish and update content remotely** - `wordpress-cli pages create` / `wordpress-cli pages update` / `wordpress-cli posts create`, no wp-admin and no SSH.
- **Triage what is unfinished** - `wordpress-cli pages list --status draft` surfaces every page still waiting to ship.
- **Search the whole site at once** - `wordpress-cli workflow archive` mirrors the site, then `wordpress-cli search "old pricing"` finds every page or post that still mentions it.
- **Manage media by id** - `wordpress-cli media upload ./hero.png --alt-text "Spring promo hero"` returns the media id you wire into a featured image.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The WordPress MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your WordPress credentials once.

### Is my WordPress data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to install a plugin on the WordPress site?

No. This drives the built-in WordPress REST API (the `/wp-json/wp/v2` routes) that ships with WordPress 4.7 and later. You only need an Application Password for a user with the right role - create one in wp-admin under **Users > Profile > Application Passwords** - and set it as `WORDPRESS_BASIC_AUTH`.

### Can it manage more than one site?

Yes. Point it at any site by setting that site's `WORDPRESS_BASE_URL` and its Application Password. Named profiles and the per-site local mirror keep each site's state separate, so one workstation can manage a stack of client sites. Treat each site's Application Password as a scoped credential.

### Will this replace wp-admin or WP-CLI?

No - it complements them. wp-admin stays the place for theme work and plugin configuration; WP-CLI stays the server-side power tool when you have SSH. This skill adds remote, scriptable, AI-driven content management over the same REST API, with a local mirror for instant cross-site search.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `pages list`, `posts list`, `media list`, `search`, `users list`, `workflow status` | Allow |
| Write (routine) | `pages create`, `pages update`, `posts create`, `posts update`, `media upload`, `media update`, `categories create`, `tags create` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `pages delete`, `posts delete`, `media delete`, `settings update` | Human-in-the-loop only |

The strongest control is the **scope you grant the WordPress credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the WordPress API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-30._
