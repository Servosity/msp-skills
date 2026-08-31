---
layout: default
title: "WordPress MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Publish and manage WordPress pages, posts, media"
permalink: /skills/wordpress/
skill_name: "WordPress MCP"
image: /assets/social/wordpress/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for WordPress?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for WordPress, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the WordPress MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `WORDPRESS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `WORDPRESS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but wordpress-mcp speaks HTTP natively: run `wordpress-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my WordPress data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to install a plugin on the WordPress site?"
    a: "No. This drives the built-in WordPress REST API (the /wp-json/wp/v2 routes) that ships with WordPress 4.7 and later. You only need an Application Password for a user with the right role (in wp-admin: Users > Profile > Application Passwords), set as WORDPRESS_BASIC_AUTH."
  - q: "Can it manage more than one site?"
    a: "Yes. Point it at any site by setting that site's WORDPRESS_BASE_URL and its Application Password; named profiles and the per-site local mirror keep each site's state separate. Treat each site's Application Password as a scoped credential."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/wordpress/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your WordPress credentials once, then run wordpress-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a WordPress question in plain language; it runs wordpress-cli for you."
---

# The WordPress MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by WordPress Foundation.

**✓ Live-verified by Servosity (maintainer)** against a production tenant · 2026-08-16.

Yes - there is an MCP server for WordPress. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects WordPress to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI to "publish this landing page as a draft" or "find every page still mentioning the old pricing" and it happens from the terminal - no wp-admin clicking, no SSH for WP-CLI. This skill drives the WordPress REST API to create, update, search, and delete pages, posts, and media, and syncs the whole site into a local mirror so cross-content questions become one instant offline query.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/wordpress){: .btn}

## Instead of clicking through WordPress, just ask

**Instead of** Log into wp-admin, open the block editor, paste the HTML, wrestle the SEO fields, then click Publish - one page at a time
**just ask:** *"Publish this landing page as a draft and give me its id"*
<sub>Your agent runs: <code>wordpress-cli pages create --title "Spring Promo" --content "<h1>Spring Promo</h1>" --status draft</code></sub>

**Instead of** Hunt through the wp-admin media library to find an uploaded image's id before you can set it as a featured image
**just ask:** *"Upload this hero image and tell me its media id"*
<sub>Your agent runs: <code>wordpress-cli media upload ./hero.png --alt-text "Spring promo hero"</code></sub>

**Instead of** Open every page in wp-admin one at a time to find the ones still sitting in Draft
**just ask:** *"Which pages are still drafts?"*
<sub>Your agent runs: <code>wordpress-cli pages list --status draft</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/wordpress/wide-1200x630.png" src="/assets/video/wordpress/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/wordpress/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Publish a landing page from HTML without opening wp-admin? | `wordpress-cli pages create --title "Spring Promo" --content "<h1>Spring Promo</h1>" --status publish` |
| Which pages are still sitting in draft? | `wordpress-cli pages list --status draft` |
| Update a live page's content in place? | `wordpress-cli pages update 42 --content "<h1>Updated copy</h1>"` |
| Upload a hero image and get its media id for a featured image? | `wordpress-cli media upload ./hero.png --alt-text "Spring promo hero"` |
| Find every page or post that mentions an outdated phrase (after a `workflow archive`)? | `wordpress-cli search "summer sale"` |
| Mirror the whole site locally for offline search and backup? | `wordpress-cli workflow archive` |
| What's mirrored locally and how fresh is it? | `wordpress-cli workflow status` |
| Publish a blog post with categories and tags? | `wordpress-cli posts create --title "Patch Tuesday recap" --content "<p>This month...</p>" --status publish` |
| Which authors can I assign content to? | `wordpress-cli users list` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/wordpress/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/wordpress/guide.md).

## What makes this one different

Most WordPress integrations proxy each request straight to the live REST API - fine for editing one page, slow and noisy when you want to grep every page and post across a whole site for an old price or a dead promo link. This skill syncs WordPress into a local SQLite mirror with full-text search (`workflow archive`), so "find every page that still mentions the old pricing" becomes one instant offline query, and `workflow status` shows exactly what is mirrored and how stale it is - your AI sees the matching rows, not a dump of every page's HTML.

WordPress's own wp-admin is built for one human clicking through one screen at a time, and the official WP-CLI needs SSH access to the server's shell. This skill drives the same REST API your site already exposes over HTTPS - no server login - so an AI agent can publish, update, search, and clean up content remotely. It complements wp-admin; it doesn't replace it.

## The pain this closes

- Routine site edits - swap a promo banner, fix a phone number, push a new landing page - mean logging into wp-admin and clicking through the block editor screen by screen. The Classic Editor plugin still reports over 10 million active installs on the WordPress.org plugin directory: a standing vote against that editor for everyday content work.
- Managing content across a stack of client sites is unbillable busywork. There is no single command to publish a page, list every draft, or search all content, so it stays manual - MSPs on r/msp regularly trade scripts and gripes about website upkeep eating hours that never reach an invoice.
- The official WP-CLI is powerful but needs SSH access to the server's shell, which many managed or hosted WordPress sites do not give you - so remote, scriptable content management falls back to the browser.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/wordpress/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/wordpress/install.ps1 | iex
```

After install, authenticate once with your WordPress credentials, then verify with `wordpress-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | pages list, posts list, media list, search, users list, workflow status | Allow |
| Write (routine) | pages create, pages update, posts create, posts update, media upload, media update, categories create, tags create | Preview with --dry-run, then a reviewed write |
| Destructive / config | pages delete, posts delete, media delete, settings update | Human-in-the-loop only |

The skill reads pages, posts, media, categories, tags, users, and settings, and can create or update that content and upload media. The sharp edges are deletes (pages, posts, media) and site settings changes. The safe default for an autonomous agent is read plus previewed (`--dry-run`) writes, with a human approving anything that deletes content or changes site settings. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/wordpress/governance.md).

## Frequently asked questions

### Is there an MCP server for WordPress?

Yes - this one. A free, open source MCP server and Claude Code Skill for WordPress, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the WordPress MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `WORDPRESS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `WORDPRESS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but wordpress-mcp speaks HTTP natively: run `wordpress-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my WordPress data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to install a plugin on the WordPress site?

No. This drives the built-in WordPress REST API (the /wp-json/wp/v2 routes) that ships with WordPress 4.7 and later. You only need an Application Password for a user with the right role (in wp-admin: Users > Profile > Application Passwords), set as WORDPRESS_BASIC_AUTH.

### Can it manage more than one site?

Yes. Point it at any site by setting that site's WORDPRESS_BASE_URL and its Application Password; named profiles and the per-site local mirror keep each site's state separate. Treat each site's Application Password as a scoped credential.


## Status

Beta. Validated against the WordPress API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
