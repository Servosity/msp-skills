# TD SYNNEX + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the TD SYNNEX
> API. Not affiliated with, endorsed by, or sponsored by TD SYNNEX Corporation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/td-synnex/">
    <img src="../../docs/assets/video/td-synnex/animated-og.gif" alt="TD SYNNEX StreamOne ION demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/td-synnex/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every StreamOne ION reseller API, plus a local licence book for renewals, margin and cross-distributor true-up. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the TD SYNNEX skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `td-synnex-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `td-synnex-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `td-synnex-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `td-synnex-mcp --transport http` with `TD_SYNNEX_MCP_HTTP_TOKEN` set and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `td-synnex-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download TD SYNNEX MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/td-synnex-v0.1.1/td-synnex-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON. ION needs one shell step first: install the CLI (Path B below) and run `td-synnex-cli auth mint --refresh-token-stdin` once (see [Authenticate](#authenticate)); the extension reads the token it saves. (Browse every TD SYNNEX release on the [releases page](https://github.com/servosity/msp-skills/releases?q=td-synnex).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install td-synnex@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the TD SYNNEX Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex`. Then authenticate per the README and run `td-synnex-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh)
```

The installer drops both `td-synnex-cli` (the CLI) and `td-synnex-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
td-synnex-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update td-synnex@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/td-synnex --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/td-synnex --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `td-synnex-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the td-synnex skill from https://github.com/servosity/msp-skills/tree/main/skills/td-synnex. The skill defines how its required CLI (`td-synnex-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

StreamOne ION signs you in with an OAuth refresh token. In the ION portal open **Settings > Users > Edit > OAuth credentials > Request New Credentials** and copy the refresh token. Then set your ION account ID and hand the token to `auth mint` on stdin, so it never lands in shell history:

**macOS / Linux:**

```bash
export TD_SYNNEX_ACCOUNT_ID=<your ION account ID>   # also add this line to your shell profile (~/.zshrc or ~/.bashrc)
pbpaste | td-synnex-cli auth mint --refresh-token-stdin   # Linux: xclip -o -selection clipboard | td-synnex-cli auth mint --refresh-token-stdin
td-synnex-cli auth check
```

**Windows (PowerShell):**

```powershell
setx TD_SYNNEX_ACCOUNT_ID "<your ION account ID>"   # persists for new windows
$env:TD_SYNNEX_ACCOUNT_ID = "<your ION account ID>"  # this window
Get-Clipboard | td-synnex-cli auth mint --refresh-token-stdin
td-synnex-cli auth check
```

Keep `TD_SYNNEX_ACCOUNT_ID` set permanently: the local store is keyed on it, and without it every new 2-hour access token starts an empty store.

ION refresh tokens are **single-use** and last 32 days; access tokens last 2 hours. `auth mint` exchanges the token, saves the new access token and the new refresh token to a private credentials file before it prints anything, and never prints a token unless you pass `--print-access-token`. Keep it fresh from cron or Task Scheduler with `td-synnex-cli auth mint --if-expiring 15m`. `auth check` asks ION how long both tokens stay valid. Do not give the refresh token to `auth set-token`, which stores an access token.

Then pull your book into the local store and confirm the numbers against the portal:

```bash
td-synnex-cli sync --resources customers,subscriptions,orders
td-synnex-cli doctor
```

## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which subscriptions renew in the next 30 days with auto-renew switched off? | `td-synnex-cli subscriptions renewals --within 30d --auto-renew off` |
| Which customers am I selling at a loss or below list price? | `td-synnex-cli margin --by customer` |
| Which SKUs carry the thinnest margin across every customer? | `td-synnex-cli margin --by sku` |
| Do my TD SYNNEX and Pax8 seats agree for each customer and SKU? | `td-synnex-cli position trueup --with pax8-positions.csv --map company=customer_name --map sku=sku_key --map quantity=seats` |
| Am I billing every seat I pay TD SYNNEX for? | `td-synnex-cli position trueup --billed psa-agreements.csv --map billed_qty=seats` |
| Which orders are stuck on hold or failed this week? | `td-synnex-cli orders stuck --older-than 24h --since 7d` |
| What seat, renewal and status changes went through ION in the last week? | `td-synnex-cli subscriptions changes --since 7d` |
| What does one customer hold, what does it cost, and what is still open? | `td-synnex-cli customers show "Contoso"` |
| What would changing this subscription to 12 seats send to ION? | `td-synnex-cli subscriptions set "10505" "108536" "1608355" --seats 12` |

`subscriptions set` only previews the order; add `--confirm` to place it. The licence-book commands (renewals, changes, margin, position, orders stuck, customers show) read the local store, so run `sync` first. `position trueup --with` and `--billed` read export files from your disk, so they run where your agent has a shell (Claude Code, Codex, Cowork) or in your terminal; over MCP (Claude Desktop, ChatGPT, Copilot) the tool refuses file paths and only lists SKUs a customer holds in more than one TD SYNNEX subscription.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most StreamOne ION integrations are PSA billing connectors or API wrappers that make one live call per question. That's fine for one record. It dies at scale, when you're asking "which of my 60 customers have NCE terms renewing this month with auto-renew off, and what do I make on each of them".

This skill syncs TD SYNNEX into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `subscriptions renewals`, `margin` and `position trueup` join across customers, subscriptions, orders and your Pax8, Sherweb or PSA exports - work a stateless API wrapper can't do. It complements the ION portal and TD SYNNEX's PSA connectors rather than replacing them: ordering, provisioning and billing sync stay there.

## The pain this closes

Microsoft's New Commerce terms only let you cut seats or cancel in the first seven days of a term or a renewal. Miss the renewal and you carry another year of seats the client may no longer pay for. The tminus365 blog for MSPs ([How I automated subscription renewal notices for Microsoft NCE](https://tminus365.com/how-i-automated-subscription-renewal-notices-microsoft-nce/), 2022) describes building a Power Automate flow on a distributor API just to open a ticket before each renewal. Add a second distributor and the seat counts you pay for sit in two portals plus the PSA agreement you bill from.

- **Renewals before the window closes:** `td-synnex-cli subscriptions renewals --within 30d --auto-renew off` lists every term renewing soon, soonest first.
- **Customers sold at a loss:** `td-synnex-cli margin --by customer` ranks customers worst margin first and flags negative, zero and below-list-price rows.
- **Seats you pay for but never bill:** `td-synnex-cli position trueup --billed psa-agreements.csv` compares ION seats with your PSA export and flags each mismatch (`billed_mismatch`), and paid seats no PSA row bills at all (`not_billed`; trial seats are flagged `trial` instead).
- **Orders nobody noticed:** `td-synnex-cli orders stuck` lists orders still open after a day, and orders that failed.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The TD SYNNEX MCP server runs locally, but it speaks HTTP natively: start it with `td-synnex-mcp --transport http` and a `TD_SYNNEX_MCP_HTTP_TOKEN` bearer value (it refuses to start without one, and serves `http://127.0.0.1:7777/mcp` by default) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your TD SYNNEX credentials once.

### Is my TD SYNNEX data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Your ION tokens live in a private credentials file under your user account (or in `TD_SYNNEX_ACCESS_TOKEN` if you prefer); they are never bundled into this repo or transmitted anywhere by MSP Skills. Passwords ION returns inside customer, order and provisioning data (customer user passwords, Microsoft tenant admin credentials) are blanked in every response before anything is printed, cached, synced or handed to an agent. Free text (notes, descriptions, report CSV) is withheld whole when it mentions a credential word such as password, PIN, secret, token or API key anywhere, so a note or a `reports get-data-csv` result that merely mentions one (a "Duo Hardware Token" row, say) comes back as `[redacted]`; `reports get-data` returns the same report cell by cell, and name, ID and address fields use a narrower rule so customer and catalog names and SKUs survive. A password typed into a note with no credential word near it cannot be recognized.

### How does it sign in to StreamOne ION?

With an ION OAuth refresh token, which needs an ION reseller account. Request it in the portal under **Settings > Users > Edit > OAuth credentials > Request New Credentials** and pipe it to `td-synnex-cli auth mint --refresh-token-stdin`. ION refresh tokens are single-use, so `auth mint` saves the rotated token before printing anything; if that save fails it writes the new refresh token to a private rescue file so you are not locked out. See [Authenticate](#authenticate).

### Will this hit my ION API limits?

`subscriptions renewals`, `subscriptions changes`, `margin`, `position export`, `position trueup`, `orders stuck` and `customers show` answer from the local store. When the store is stale they first refresh the first page (about 100 rows) of each resource they read, so with a larger book run `td-synnex-cli sync` before whole-book answers; add `--data-source local` (or set `TD_SYNNEX_NO_AUTO_REFRESH=1`) to never call the API from them. Live calls respect the `--rate-limit` throttle.

### Can it change subscriptions or place orders?

Yes, and that is why the [safety model](#safety-model) matters. `subscriptions set` previews the UPDATE order and places it only with `--confirm`. Other writes (`carts checkout`, `customer-orders create-order` and `cancel-order`, `customers create` and `update`) send as soon as they run; preview them with `--dry-run` first. Set `TD_SYNNEX_MCP_READ_ONLY=1` to keep the MCP server to reads.

### Are the margin numbers exact?

Not yet proven. See [Known gaps](#known-gaps).

### Does this replace the ION portal?

No. Ordering, provisioning and invoicing stay in StreamOne ION. This skill answers the questions that span every customer, from your terminal or agent.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `subscriptions renewals`, `subscriptions changes`, `margin`, `position export`, `position trueup`, `orders stuck`, `customers show`, every `list` and `get`, `pricing`, `cloud-profiles`, `reports get-data`, `sync`, `auth check` | Allow |
| Write (routine) | `customers create` / `update`, `carts create` / `update`, `cart-items create` / `update`, `reports save` (they send immediately; `--dry-run` is an opt-in preview) | Preview with `--dry-run`, then a reviewed write |
| Orders and spend | `subscriptions set` (seats, renewal setting, name; places an order only with `--confirm`), `carts checkout`, `customer-orders create-order`, `cloud-providers provision-customer` / `link-customer`, `products enable` / `disable`, `programs batch-update-product` | Human-in-the-loop only |
| Destructive / credential | `subscriptions set --status suspended` or `deleted`, `subscriptions set --renewal cancel_at_renewal` (or its alias `auto-off`), `customer-orders cancel-order`, `carts delete`, `cart-items delete`, `reports delete`, `auth mint`, `auth set-token`, `auth logout` | Human-in-the-loop only, explicit confirmation |

`TD_SYNNEX_MCP_READ_ONLY=1` removes every tool that can change ION from the MCP server; it also removes `sync` and the tools that record learnings, and the tools it keeps still write local state (the synced store when a read refreshes it, the command journal and recall statistics).

The strongest control is the **ION user whose OAuth credentials you issue** - the CLI can only do what that user is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Known gaps

- **Not yet run against a live StreamOne ION tenant.** Every command was checked against TD SYNNEX's published v3 API documentation, a mock ION server and unauthenticated route probes, not real account data. Start with `td-synnex-cli doctor` and a small `td-synnex-cli sync --resources customers`, and compare the first results with the ION portal.
- **Margin assumes ION prices per subscription, not per seat.** `margin`, `subscriptions renewals`, `position export` and `customers show` use the price and cost ION reports for each subscription (for monthly billing, its newest billed month) and do not multiply them by seats. If ION reports them per seat, multi-seat totals are understated, and so is `position trueup`'s `unit_cost_by_source` (cost divided by seats), by the same factor. Compare one multi-seat subscription with the ION portal before relying on the totals. A price, cost, seat count, order quantity, order total or order age ION does not send (or sends in a form the CLI cannot read; the record is kept, except that a subscription or order whose status, trial flag or customer ID is unreadable is skipped with a warning, and `subscriptions set` refuses to order from a subscription whose IDs, owner, status, flags or dates it cannot read), or a billing cycle the CLI does not recognize (the commitment term is never used in its place), is reported as unknown (`price_known`, `cost_known`, `seats_known`, or a `*_incomplete` flag), never as 0, and is left out of totals and margin. When an annual subscription sends neither price nor cost, the trailing twelve billed months are summed, so a subscription that switched from monthly to annual billing inside that window counts both; `basis` names the months used.
- **Changes made outside ION are invisible.** `subscriptions changes` decodes ION orders, so a seat change made directly in Microsoft Partner Center does not appear.
- **Generated passwords are not shown.** `customers create` and `cloud-providers provision-customer` blank the one-time password ION returns, along with every other credential in ION responses. Set or reset that password in the ION portal or the Microsoft 365 admin center.
- **Tenant ID and domain are blank in `position export`.** ION subscriptions do not carry them, so `position trueup` matches customers on name unless your other files supply them.

## Status

Beta. Checked against TD SYNNEX's published v3 API documentation and a mock ION server, not yet against a live tenant (see [Known gaps](#known-gaps)). We validate connectors with MSPs running them live against their own production tenants in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-16._
