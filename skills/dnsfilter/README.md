# DNSFilter + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the DNSFilter
> API. Not affiliated with, endorsed by, or sponsored by DNSFilter, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/dnsfilter/">
    <img src="../../docs/assets/video/dnsfilter/animated-og.gif" alt="DNSFilter demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/dnsfilter/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every DNSFilter API operation, plus one-command answers to 'why is this site blocked for this client' and 'which clients are behind on the roaming agent'. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the DNSFilter skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `dnsfilter-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `dnsfilter-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `dnsfilter-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `dnsfilter-mcp --transport http` with `DNSFILTER_MCP_HTTP_TOKEN` set and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `dnsfilter-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download DNSFilter MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/dnsfilter-v0.1.0/dnsfilter-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every DNSFilter release on the [releases page](https://github.com/servosity/msp-skills/releases?q=dnsfilter).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install dnsfilter@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the DNSFilter Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex`. Then authenticate per the README and run `dnsfilter-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh)
```

The installer drops both `dnsfilter-cli` (the CLI) and `dnsfilter-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
dnsfilter-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update dnsfilter@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/dnsfilter --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/dnsfilter --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `dnsfilter-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the dnsfilter skill from https://github.com/servosity/msp-skills/tree/main/skills/dnsfilter. The skill defines how its required CLI (`dnsfilter-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API token in the DNSFilter dashboard (in your MSP organization, so the cross-client commands see your client orgs) and set it as `DNSFILTER_API_KEY`. Use the token itself: DNSFilter expects the bare token in the `Authorization` header, with no `Bearer` prefix.

macOS / Linux:

```bash
DNSFILTER_API_KEY=<value> dnsfilter-cli doctor
```

On Windows (PowerShell):

```powershell
$env:DNSFILTER_API_KEY = "<value>"
dnsfilter-cli doctor
```

To save the token instead of setting it each session, pipe it to `dnsfilter-cli auth set-token` (it reads stdin, so the token never lands in your shell history as an argument):

```bash
printf '%s\n' "$DNSFILTER_API_KEY" | dnsfilter-cli auth set-token
```

On Windows (PowerShell):

```powershell
$env:DNSFILTER_API_KEY | dnsfilter-cli auth set-token
```

`doctor` checks config, paths and API reachability, and sends the token to DNSFilter's `GET /v1/current_user`, so a rejected token shows up as a `Credentials: invalid` row, a token that lacks access (HTTP 403) as `WARN scope-limited`, and a wrong base URL as an `API: ERROR` row. An answer that was redirected away from the DNSFilter API never counts as proof. It exits 0 even then, so scripts should add `--fail-on error` (or `--fail-on warn`, which also fails on a 403 or a redirected probe). It also warns when a stored token starts with `Bearer ` or `Token `.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Why is this domain blocked for this client, and which policy and rule did it? | `dnsfilter-cli why-blocked "example.com" --org "Acme Dental"` |
| Allow it on the policies that actually blocked it (preview first) | `dnsfilter-cli allow "example.com" --org "Acme Dental" --note "TKT-4821"` |
| What unblock requests are waiting across every client, grouped by domain? | `dnsfilter-cli unblocks triage` |
| Which clients have roaming clients behind the latest agent release? | `dnsfilter-cli agents drift` |
| Who is still on the roaming client build that got pulled? | `dnsfilter-cli agents drift --bad-version 1.7.15 --agent-type windows` |
| Which roaming clients stopped checking in, or are unprotected or bypassed? | `dnsfilter-cli agents stale --older-than 7d` |
| Which client policies already allow or block this domain or a parent domain? | `dnsfilter-cli lists where "example.com"` |
| Which allow-listed domains are now categorized as threats, and which policies leave security categories unblocked? | `dnsfilter-cli policy-audit` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most DNSFilter integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 60 client orgs still have Windows roaming clients on the build DNSFilter just pulled" or "why is this site blocked for this one client right now".

This skill syncs DNSFilter into a **local SQLite mirror** with full-text search, so `agents drift`, `agents stale` and `lists where` answer one local query across every client org, joining roaming clients to their sites, sites to their organizations and policies. `agents drift` reads DNSFilter's release feed live on every run, so a pulled release does not linger in a stale copy. `why-blocked` and `allow` work live: they resolve the client's current policy (scheduled-policy slot included), walk DNSFilter's documented rule order, read the Query Log, and say which rule types the API does not expose, which is the chain a stateless API wrapper leaves the AI to assemble call by call.

It complements the DNSFilter dashboard rather than replacing it: policies, deployments, reporting and the Query Log stay there.

## The pain this closes

A blocked-site ticket is a chain, not a lookup: which policy the client's site or roaming client is using right now (scheduled policies switch by the quarter hour), that policy's allow list, block list and categories, and what the Query Log recorded. Then the fix has to land on the policy that actually blocked it, not a shared global one. A DNSFilter Community post, "Policy enforcement & reporting discrepancies with Roaming Client", describes a roaming client on a second site ignoring its category blocks, and reaching a URL that its reports showed as blocked.

Roaming client releases go wrong across the whole book at once. DNSFilter's status page logged the [Windows Roaming Client 1.7.15 auto-upgrade incident](https://status.dnsfilter.com/incidents/vc2qwqtx2sxz): auto-upgrade paused, the release pulled from general availability, and uninstall-and-reinstall as the workaround. Its help center documents [macOS machines losing connectivity when updating the Roaming Client from 1.x to 2.2](https://help.dnsfilter.com/hc/en-us/articles/41871975449363-Internet-connectivity-lost-when-updating-macOS-Roaming-Client-1-x-x-to-2-2-x). The first question each time is which clients are on the bad build.

- `dnsfilter-cli why-blocked "example.com" --org "Acme Dental"` - the policy and rule that decided it for that client right now, with recent Query Log hits.
- `dnsfilter-cli allow "example.com" --org "Acme Dental" --note "TKT-4821"` - the allow planned on the policies that actually blocked it; `--confirm` writes and reads back.
- `dnsfilter-cli unblocks triage` - the pending unblock queue across every client, grouped by domain, threats flagged.
- `dnsfilter-cli agents drift --bad-version 1.7.15 --agent-type windows` - every client org with roaming clients on the pulled build.
- `dnsfilter-cli agents stale --older-than 7d` - roaming clients that went silent or are unprotected or bypassed.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The DNSFilter MCP server runs locally, but it speaks HTTP natively: start it with `dnsfilter-mcp --transport http` and `DNSFILTER_MCP_HTTP_TOKEN` set (it serves `http://127.0.0.1:7777/mcp` by default and requires that token as a bearer header) and put it behind an HTTPS gateway that ChatGPT can sign in to with OAuth and that adds that bearer header (ChatGPT connectors do not send a fixed one). No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your DNSFilter credentials once.

### Is my DNSFilter data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills. Secret fields DNSFilter returns, such as each site's secret key and API key tokens, are not written to the local mirror.

### What DNSFilter API key do I need?

An API token created in the DNSFilter dashboard (see DNSFilter's API Tokens help article). Set DNSFILTER_API_KEY to the token itself: DNSFilter expects the bare token in the Authorization header, with no Bearer prefix. Create it in your MSP organization so the cross-client commands can see your client organizations; a token from a single client organization only sees that client.

### Can the AI allow a domain or change a policy by itself?

Only if you let it. allow prints the plan and writes nothing unless you add --confirm with a --note (the ticket number), refuses a domain in a security category unless you add --force, skips policies you cannot edit or whose allow list it cannot read, marks an assigned policy it cannot evaluate as review instead of writing to it, and leaves shared global policies alone unless you pass --include-global; a saved profile can never supply --confirm or --force. Generated commands such as policies update or policies add-whitelist-domain send as soon as they run, so preview them with --dry-run. Over MCP, the generic dnsfilter_execute tool can call any DNSFilter endpoint, deletes included, gated only by your AI app's approval prompt. For an agent that should only read, set DNSFILTER_MCP_READ_ONLY=1 in the MCP server's environment: write tools and allow are removed, dnsfilter_execute runs GET endpoints only, and API key reads, site secret keys and the roaming client uninstall PIN are withheld. Keep the approval prompt on.

### Does why-blocked see every rule DNSFilter applies?

No, and it says so. It evaluates the site or roaming client's current policy (including the scheduled-policy slot and a roaming client inheriting its site's policy) in DNSFilter's documented order: allow list, allow-list-only mode, block list, uncategorized domains, blocked categories. User, collection and LAN-subnet assignments, the org or MSP Universal Allow and Block Lists, and AppAware application rules are not exposed by the policy API, so they are not evaluated; when a Query Log hit names a policy outside the ones it checked, the output flags it.

### Why do agents drift, agents stale or lists where say there is nothing to report?

They read the local mirror, so run sync first: `dnsfilter-cli sync --full --resources organizations,networks,policies,scheduled-policies,user-agents`. agents drift also reads DNSFilter's release feed live on every run, so a pulled release does not linger in a stale copy.

### Has it been tested against a live DNSFilter account?

Not yet. It is built from DNSFilter's published OpenAPI spec and tested against a local mock, so it ships as Beta and Awaiting live verification. A few request shapes were inferred from the spec (some list filters sent in the request body, the bulk domain lookup requiring fqdns) and will be confirmed on the first live run; tell us what you see.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `dnsfilter-cli` why-blocked; unblocks triage; agents drift; agents stale; lists where; policy-audit; allow (without --confirm, plan only); organizations index; organizations show; policies index; policies show; user-agents index; user-agents tags; unblock-requests index; traffic-reports query-logs; domains user-lookup; search; sync | Allow |
| Filtering policy and allow lists | allow --confirm; policies add-whitelist-domain; policies remove-whitelist-domain; policies add-blacklist-domain; policies remove-blacklist-domain; policies add-blacklist-category; policies remove-blacklist-category; policies bulk-actions-add-allowlist-domains; policies update; policies permissive-mode policies-update; policies add-allowed-application; scheduled-policies update; unblock-requests decision unblock-requests; block-pages update; networks update; networks bulk-update; networks subnets network-update; user-agents update; user-agent-bulk-updates create; agent-local-users update; mac-addresses update; policies application-update; policies bulk-actions-add-blocklist-domains; policies bulk-actions-remove-blocklist-domains; policies bulk-actions-remove-allowlist-domains; mac-addresses create; mac-addresses destroy; user-agents update-settings user-agents; networks subnets network-create; networks subnets network-destroy; ip-addresses update; policies add-blocked-application; policies remove-allowed-application; policies remove-blocked-application; scheduled-policies create; scheduled-policies destroy | Preview (allow without --confirm, or --dry-run), then a human-approved write. One allow, a weaker policy, a bypass domain, or a site or roaming client moved to another policy or disabled opens filtering for everything it touches. A write not named in any tier gets this treatment until you have checked what it changes. |
| Write (routine) | networks create; organizations create; ip-addresses create; scheduled-reports create; dashboards create; notes domain-update | Preview with --dry-run, then a reviewed write. These add sites, organizations, addresses, reports and notes; none changes the filtering an existing site, device or roaming client gets. |
| Credential and access | networks index, networks show, networks all, networks msp and networks msp-all (rows carry each site's secret key); networks secret-key networks-generate; networks secret-key networks-rotate; networks secret-key networks-revoke; api-keys index; api-keys show; api-keys create; api-keys revoke api-keys; user-agents uninstall-pin; users change-password; distributors users-reset-password-url; organizations users organization-create; tail and export of networks or api-keys (rows carry secrets); psa-integrations (a GET that creates a PSA integration and returns its link); auth set-token | Human-in-the-loop only |
| Destructive and commercial | import (one create per JSONL line, so it can reach any tier above); policies destroy; networks destroy; networks bulk-destroy; organizations destroy; organizations cancel; organizations update and organizations bulk-update (SKU, seat quantity, managing MSP, auto-update); user-agents destroy; agent-local-user-bulk-deletes create; api-keys destroy; billing create; billing address-update; distributors organizations-cancel; distributors msps-update; trials | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the DNSFilter credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the DNSFilter API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-20._
