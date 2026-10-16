# Bitdefender + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Bitdefender
> API. Not affiliated with, endorsed by, or sponsored by Bitdefender SRL.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/bitdefender/">
    <img src="../../docs/assets/video/bitdefender/animated-og.gif" alt="Bitdefender GravityZone demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/bitdefender/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

140 GravityZone API methods, plus fleet health, billing and incident views across child companies. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Bitdefender skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `bitdefender-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `bitdefender-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `bitdefender-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `bitdefender-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `bitdefender-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Bitdefender MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/bitdefender-v0.1.0/bitdefender-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Bitdefender release on the [releases page](https://github.com/servosity/msp-skills/releases?q=bitdefender).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install bitdefender@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Bitdefender Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex`. Then authenticate per the README and run `bitdefender-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh)
```

The installer drops both `bitdefender-cli` (the CLI) and `bitdefender-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
bitdefender-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update bitdefender@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/bitdefender --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/bitdefender --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `bitdefender-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the bitdefender skill from https://github.com/servosity/msp-skills/tree/main/skills/bitdefender. The skill defines how its required CLI (`bitdefender-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Bitdefender portal):

```bash
BITDEFENDER_BASE_URL=<value> BITDEFENDER_API_KEY=<value> bitdefender-cli doctor
```

`doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run a read command to confirm the credential actually works end-to-end.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which customers have unmanaged, outdated or unscanned endpoints? | `bitdefender-cli fleet health` |
| What changed in endpoint protection since last week? | `bitdefender-cli fleet changes --since 7d` |
| What open incidents across every client are high or critical? | `bitdefender-cli fleet incident-queue --priority high,critical` |
| Is the same malware quarantined at several customers, and is it blocklisted? | `bitdefender-cli fleet threats --window 7d` |
| Which customer is this machine in, and what is its endpoint id? | `bitdefender-cli fleet locate "LAPTOP-7"` |
| Which machines in my RMM have no Bitdefender agent? | `bitdefender-cli fleet rmm-gap --devices ninja-devices.csv` |
| What did each customer use last month, after the partner rollup? | `bitdefender-cli usage billing --month 09/2026` |
| List every child company and sub-partner with its id and parent | `bitdefender-cli companies tree` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Bitdefender GravityZone integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 60 customers have unmanaged machines or agents that went out of date since last week".

GravityZone's Public API is JSON-RPC, one method per call, and the company tree comes back one level per `getCompaniesList` call. This skill walks the whole partner tree for you and fans each question out across every child company. `fleet health` saves each run as a dated **local snapshot** (SQLite), so `fleet changes` diffs two runs, and `fleet locate` and `fleet rmm-gap` answer from the latest one without another fan-out. A company it could not read completely is listed in `fetch_failures`, never counted as clean.

Integrations such as [Liongard's GravityZone inspector](https://docs.liongard.com/docs/bitdefender-gravityzone-inspector) and [Sumo Logic's GravityZone automation integration](https://www.sumologic.com/help/docs/platform-services/automation-service/app-central/integrations/bitdefender-gravityzone/) bring GravityZone data into their own platforms. This one puts the cross-customer answers in whichever AI agent you already use. It complements GravityZone Control Center rather than replacing it: the console stays the place for policy authoring, investigations and day-to-day response.

## The pain this closes

The GravityZone Public API is JSON-RPC, and `getCompaniesList` answers one level of the company tree at a time, so every MSP script walks sub-partners and folders itself, then pages `getEndpointsList` per company. A [Bitdefender Expert Community thread](https://community.bitdefender.com/en/discussion/92970/powershell-api-get-list-of-computers) shows a hand-built PowerShell paging loop that still returned the wrong group of the hierarchy. Meanwhile the morning questions live on different Control Center pages (Network, Incidents, Quarantine, the monthly usage reports), and month-end billing means working out each partner's own share of rolled-up usage by hand.

- `bitdefender-cli fleet health` - every child company ranked by unmanaged, outdated, unscanned, policy-not-applied and isolated endpoints.
- `bitdefender-cli fleet incident-queue --sla 24h` - one open EDR/XDR incident queue across every customer, worst and oldest first, unassigned incidents flagged.
- `bitdefender-cli fleet threats` - the same quarantined file (SHA256) at two or more customers, and whether each one blocklists it.
- `bitdefender-cli usage billing --month 09/2026` - per-company usage with each partner's own share after the rollup, ready to check against PSA agreements.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Bitdefender MCP server runs locally, but it speaks HTTP natively: start it with `bitdefender-mcp --transport http` and `BITDEFENDER_MCP_HTTP_TOKEN` set (it serves `http://127.0.0.1:7777/mcp` by default and requires that token as a bearer header) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Bitdefender credentials once.

### Is my Bitdefender data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The fleet snapshot store (SQLite) sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What GravityZone API key do I need?

A Control Center API key: click the user icon in Control Center, choose **My Account** and use the **Control Center API** section, which also shows the **Access URL**. Keys are scoped per API area when you create them, with no read/write split inside an area. The fleet commands need the Companies and Network APIs, plus Licensing for `usage billing`, Incidents for `fleet incident-queue`, and Quarantine and Incidents for `fleet threats`. Set `BITDEFENDER_API_KEY`, and set `BITDEFENDER_BASE_URL` to your Access URL if it is not `https://cloud.gravityzone.bitdefender.com/api`.

### Does it need a partner account?

For the cross-customer views, yes: walking child companies needs a partner-level key, because `getCompaniesList` is a partner API. A customer-level key works, but every fleet view then covers only that one company.

### Will this hit GravityZone API rate limits?

Control Center limits each API key to a number of requests per second and answers 429 with a `Retry-After` header when you go over. The CLI paces itself at 5 requests per second by default (`--rate-limit`) and waits out a 429. `getIncidentsList` allows about 3 calls a minute, so `fleet incident-queue` makes one partner-wide call and spaces any extra pages 21 seconds apart.

### Can the AI isolate a machine or run a command on an endpoint by itself?

Only if you let it. Generated commands run as soon as they are invoked and do not prompt, so preview with `--dry-run` first: it prints the JSON-RPC method and parameters it would send, without sending. Over MCP, the generic `bitdefender_execute` tool can call any GravityZone method, including isolation, kill process, remote command execution and deletes, gated only by your AI app's approval prompt. For an agent that should only read, set `BITDEFENDER_MCP_READ_ONLY=1` in the MCP server's environment: `bitdefender_execute` then runs only GravityZone read methods, refuses every write and the reads that return or take a credential, and the `import` tool is hidden. Keep the approval prompt on.

### Why does `fleet changes` say it has nothing to compare?

It diffs saved snapshots, and each `fleet health` run saves one (unless you pass `--no-save`). Run `fleet health` at least twice, for example daily, and `fleet changes` reports endpoints added or removed, managed/unmanaged flips, newly outdated agents, policies no longer applied and isolation changes between them.

### Does it work with on-premises GravityZone?

It speaks the same JSON-RPC Public API, so set `BITDEFENDER_BASE_URL` to your Control Center's Access URL. It has not yet been verified against a live on-premises console.

### Does it replace GravityZone Control Center?

No. Control Center stays best for policy authoring, investigations and day-to-day console work. This skill answers the across-every-customer questions and runs scripted actions from your AI agent.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet health`, `fleet incident-queue`, `fleet threats`, `fleet locate`, `fleet rmm-gap`, `fleet changes`, `companies tree`, `usage billing`, `usage monthly`, `companies list`, `endpoints list`, `endpoints get`, `incidents list`, `quarantine list`, `policies list`, `patches missing`, `tasks status`, `reports list` | Allow |
| Endpoint response | `isolation isolate`, `isolation restore`, `endpoints kill-process`, `investigation run-command`, `investigation kill-process`, `investigation collect-package`, `investigation retrieve-file`, `tasks scan`, `tasks reconfigure`, `quarantine remove`, `quarantine restore`, `quarantine empty`, `live-search`, `yara`, `response-actions create` | Human-in-the-loop, explicit confirmation - these reach real customer machines |
| Protection and policy config | `policies set-modules`, `endpoints assign-policy`, `blocklist add`, `blocklist remove`, `custom-rules create`, `custom-rules update`, `custom-rules delete`, `phasr apply`, `maintenance-windows create`, `push set`, `integrations create`, `aws-ec2 configure` | Preview with `--dry-run`, then a human-approved write - an exclusion rule or a weaker policy can open a hole |
| Write (routine) | `incidents note`, `incidents set-status`, `incidents assign`, `endpoints set-label`, `endpoints move`, `groups create`, `company-folders create`, `reports create`, `packages create`, `import` | Preview with `--dry-run`, then a reviewed write |
| Credential and access | `push get` (returns the webhook Authorization value), `license get` (returns the license key), `companies get-by-user` (takes a user's password), `packages get`, `packages install-links`, `integrations get`, `accounts create`, `accounts update`, `accounts delete`, `email-verification`, `auth set-token` | Human-in-the-loop only |
| Destructive and commercial | `endpoints delete`, `endpoints move-to-company`, `companies create`, `companies suspend`, `companies delete`, `company-folders delete`, `groups delete`, `packages delete`, `license set-key`, `license add-product-key`, `license remove-product-key`, `license set-monthly-subscription` | Human-in-the-loop only, explicit confirmation |

Generated commands run as soon as they are invoked; preview with `--dry-run`. Over MCP, the generic `bitdefender_execute` tool can reach every write in the table; set `BITDEFENDER_MCP_READ_ONLY=1` for an agent that should only read.

The strongest control is the **scope you grant the Bitdefender credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Bitdefender API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-16._
