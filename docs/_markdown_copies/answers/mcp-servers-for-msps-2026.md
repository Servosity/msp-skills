> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/

# MCP servers for MSPs in 2026: how the options compare

> **Last checked November 2, 2026.** Disclosure: Servosity makes msp-skills and wrote this page. Product facts link to the company's own pages. Where we say we found nothing, that is a dated web and GitHub search, not a guarantee. Prices and counts change fast, so check the linked page before you buy. Spot a mistake? [Tell us on GitHub](https://github.com/Servosity/msp-skills/issues/new) and we will fix it.

An MCP server is the connector that lets an AI app like Claude, ChatGPT or Copilot use one of your tools, through the open Model Context Protocol (MCP). MSPs now have four kinds to pick from: free open-source servers you run yourself, hosted gateways that hold your API keys and give your whole team one web address, Thread's server, built around its own service desk, and servers vendors build for their own products. The real choice is which AI app you use and who holds your keys.

## The comparison at a glance

| Option | Price, as published | Where your API keys live | Coverage | What has been checked |
| --- | --- | --- | --- | --- |
| **msp-skills** (by Servosity) | Free, Apache-2.0 ([README](https://github.com/Servosity/msp-skills#readme)) | Your machine: an environment variable or your AI app's config file, or the macOS Keychain / Windows Credential Manager if you use [connect-tool](https://msp-skills.compoundingteams.com/skills/connect-tool.md) | 84 connectors ([catalog](https://github.com/Servosity/msp-skills/blob/main/catalog.json)) | 19 confirmed on a live tenant, 65 awaiting live verification ([receipts wall](https://msp-skills.compoundingteams.com/verified/)) |
| **WYRE self-run servers** | 46 of its 90 `-mcp` repositories, including [autotask-mcp](https://github.com/WYRE-AI/autotask-mcp), [connectwise-manage-mcp](https://github.com/WYRE-AI/connectwise-manage-mcp) and [halopsa-mcp](https://github.com/WYRE-AI/halopsa-mcp), are free under Apache-2.0; GitHub shows no standard license for the other 44, so their terms are unclear | Your machine or your own server, as environment variables ([autotask-mcp](https://github.com/WYRE-AI/autotask-mcp)) | 90 public `-mcp` repositories ([WYRE-AI on GitHub](https://github.com/WYRE-AI)) | The autotask-mcp README shows build and coverage badges and claims 80%+ test coverage; we did not find a list of which servers have been run against a live tenant |
| **WYRE Conduit** (hosted) | $39 per seat a month, 2 AI-agent seats free, 14-day trial ([pricing](https://conduit.wyre.ai/pricing/)) | WYRE's servers in Microsoft Azure, encrypted per record ([security](https://conduit.wyre.ai/security/)) | 155 vendor connectors ([pricing](https://conduit.wyre.ai/pricing/)) | Integrations labeled live or beta ([blog](https://conduit.wyre.ai/blog/which-of-your-msp-tools-have-an-mcp)); not SOC 2 certified, "preparing for SOC 2 Type 2" ([security](https://conduit.wyre.ai/security/)) |
| **StackJack** (hosted) | Free tier on every connector (100 calls a month); Pro $19 to $99 and Business $29 to $199 per connector a month ([about](https://stackjack.io/about/)) | StackJack's Azure Key Vault ([security](https://stackjack.io/security/)) | 114 connectors ([home](https://stackjack.io/)) | SOC 2 Type II report issued, shared under NDA ([security](https://stackjack.io/security/)) |
| **MSPbots Connector** (hosted) | "Free to use" ([MSPbots on GitHub](https://github.com/MSPbotsAIFramework/mspbots-mcp)) | MSPbots' service, "encrypted per record" ([same page](https://github.com/MSPbotsAIFramework/mspbots-mcp)) | "150+ connectors" ([same page](https://github.com/MSPbotsAIFramework/mspbots-mcp)) | MSPbots reports a completed SOC 2 Type II audit ([data security](https://mspbots.ai/data-security)) |
| **Thread MCP server** | Needs a Thread plan: AI Essentials $19 or AI Pro $34 per managed customer a month ([pricing](https://www.getthread.com/thread-pricing)); without AI Pro the server offers seven read-only tools ([guide](https://docs.getthread.com/super-magic/thread-mcp-server-developer-guide)) | No key: you sign in with your Thread account ([guide](https://docs.getthread.com/super-magic/thread-mcp-server-developer-guide)) | Thread's service desk, plus NinjaOne, ImmyBot, TimeZest, IT Glue and Hudu tools once those integrations are set up ([guide](https://docs.getthread.com/super-magic/thread-mcp-server-developer-guide)) | "Generally available to all partners" ([guide](https://docs.getthread.com/super-magic/thread-mcp-server-developer-guide)) |
| **Vendors' own servers** | Varies: Syncro includes it in every plan, Kaseya says its servers are free, Liongard's needs its own subscription (links below) | Usually your vendor sign-in; some take an API key | What that vendor's platform holds: one product for most, several for platforms such as ScalePad, N-able, Liongard and Rewst | Built by the vendor; several are still beta, preview or early access |

Local does not mean nothing leaves your machine. Whichever option you pick, your AI app reads the results the server returns ([msp-skills README](https://github.com/Servosity/msp-skills#readme)). If that app runs on a cloud model, as Claude, ChatGPT and Copilot do, those results go to the model's provider.

## When a hosted gateway is the better pick

- **Your team works in ChatGPT on the web or Microsoft 365 Copilot.** Those reach MCP servers at a web address, not programs on your laptop. A gateway hands you that address on day one; check that your AI app is on its setup list (Conduit documents Microsoft 365 Copilot, [getting started](https://conduit.wyre.ai/docs/getting-started/)). The ChatGPT desktop app is different: it can run local servers ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). With msp-skills you would host the connector yourself with TLS and a token ([NinjaOne install guide](https://github.com/Servosity/msp-skills/blob/main/skills/ninjaone/mcp-install.md)), and our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md) says not to run a connector that way until it is reprinted. 75 of our 84 install guides still carry that pause note (for example, [HaloPSA's](https://github.com/Servosity/msp-skills/blob/main/skills/halopsa/mcp-install.md)).
- **You want every tech to sign in as themselves, with one place to cut off access.** Conduit offers single sign-on and per-team tool access ([pricing](https://conduit.wyre.ai/pricing/)); StackJack gives every plan per-user sign-in and tool restrictions ([home](https://stackjack.io/)).
- **You need a team-wide log of every AI tool call.** Conduit logs and exports every call ([features](https://conduit.wyre.ai/features/)), and so does MSPbots Connector ([GitHub](https://github.com/MSPbotsAIFramework/mspbots-mcp)).
- **A security review or insurer asks for a vendor audit report.** StackJack shares its SOC 2 Type II report under NDA ([security](https://stackjack.io/security/)).
- **You do not want API keys sitting on technician laptops.** That is the core trade: a gateway moves them to one service you have to trust instead.

## When a vendor's own server is the better pick

If you need one product and its vendor ships a server, start there. It comes from the company that owns the API, and many let you sign in with the account you already have instead of handling a key. Support varies: CrowdStrike and SentinelOne each say theirs is a community project they maintain, not an official product ([CrowdStrike](https://github.com/CrowdStrike/falcon-mcp), [SentinelOne](https://github.com/Sentinel-One/purple-mcp)). The catch is coverage: each one reaches what that vendor's platform can see. Some see a lot: Liongard answers questions across the systems it inspects, and Rewst runs workflows through its integrations. Anything outside that platform needs another connector.

## When msp-skills is the better pick

- **You want it free**, with no per-seat, per-connector or per-call fee. You pay for your AI app as usual ([README](https://github.com/Servosity/msp-skills#readme)).
- **You want keys and data on your own machine**, with no Servosity service in the path ([README](https://github.com/Servosity/msp-skills#readme)).
- **You use a local AI app**: Claude Desktop, Claude Code, Codex, Cursor or GitHub Copilot in VS Code ([which agent?](https://msp-skills.compoundingteams.com/which-agent.md)).
- **You ask questions across every client.** Each connector keeps a local searchable copy of your data, so "which clients had backup failures this month?" becomes one local query, with no rate-limit hits ([README](https://github.com/Servosity/msp-skills#readme)).
- **You want to read the code.** Every connector's source is public under Apache-2.0 ([GitHub](https://github.com/Servosity/msp-skills)).

## Where msp-skills is weaker

1. **Most connectors are unproven on a real tenant.** 19 of 84 have been confirmed live; 5 of those 19 were confirmed by Servosity on its own tenants. 7 were confirmed on an earlier release than today's, and the other 12 did not record a version, so no receipt yet proves today's release ([receipts wall](https://msp-skills.compoundingteams.com/verified/)). The other 65 pass our automated build and documentation checks, which cannot tell us what a live tenant does ([CONTRIBUTING.md](https://github.com/Servosity/msp-skills/blob/main/CONTRIBUTING.md)).
2. **Web-based AI apps are hard.** ChatGPT on the web and Microsoft 365 Copilot need the setup described above.
3. **Keys sit in plain text by default.** The manual setup in our install guides puts your API secret in your AI app's config file ([HaloPSA install guide](https://github.com/Servosity/msp-skills/blob/main/skills/halopsa/mcp-install.md)). [connect-tool](https://msp-skills.compoundingteams.com/skills/connect-tool.md) can store it in the macOS Keychain or Windows Credential Manager instead, but each person still holds their own keys.
4. **No team controls.** There is no hosted dashboard, so no single sign-on, no central audit log and no one switch to revoke a tech's access. Your strongest control is scoping the API key inside your own vendor tenant ([are MCP servers safe?](https://msp-skills.compoundingteams.com/answers/are-mcp-servers-safe-for-msp-client-data.md)).
5. **The local copy is not encrypted and does not expire.** You delete its folders by hand to remove it ([SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md)).
6. **Some connectors can write without asking.** 53 connectors expose a tool that can call any endpoint the connector covers, including writes where the API has them, with no confirmation step; 17 support a read-only switch ([SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md)).
7. **Fewer connectors.** 84, against 114 for StackJack, 155 for Conduit and "150+" for MSPbots Connector.

## Vendors' own MCP servers, as of November 2, 2026

Checked on each vendor's own site or GitHub. "Your instance" means the server runs inside the copy of the product you already use.

- **CIPP**: introduced as a read-only beta in v10.5.0 ([release notes](https://github.com/KelvinTegelaar/CIPP/releases/tag/v10.5.0)); inside your own CIPP instance, Microsoft sign-in, read-only, off until you turn it on ([docs](https://docs.cipp.app/user-documentation/cipp/integrations/cipp-api#cipp-mcp)).
- **ConnectWise**: a web search on November 2 found community-built ConnectWise servers (WYRE's among them) but none from ConnectWise, and the [ConnectWise GitHub organization](https://github.com/connectwise) lists no public repositories.
- **CrowdStrike**: open-source falcon-mcp (MIT) that you run with your Falcon API keys; not yet at version 1.0 ([GitHub](https://github.com/CrowdStrike/falcon-mcp)).
- **HaloPSA**: on your Halo instance; sign in through a Halo API application, or use a static API key; needs Halo's AI module turned on ([guide](https://www.usehalo.com/guides/2798), [guide](https://www.usehalo.com/guides/2597)).
- **Hudu**: since v2.42.0 ([release notes](https://www.hudu.com/blog/release-update-2.42.0)), on your Hudu instance, Hudu sign-in, off by default ([support article](https://support.hudu.com/hc/en-us/articles/37814986523543-Hudu-MCP-Server)).
- **Huntress**: hosted by Huntress, uses your Huntress API key, read-only ([support article](https://support.huntress.io/hc/en-us/articles/50846554117395-Connecting-to-the-Huntress-MCP-Server)).
- **ImmyBot**: built into your instance; acts as you, through your sign-in or a personal access token ([docs](https://www.immy.bot/documentation/basic-instance-management/connect-an-ai-client/)).
- **Kaseya** (Autotask, Datto RMM, IT Glue): alpha, early access; Kaseya-hosted for Autotask and IT Glue, a local bridge for Datto RMM; API keys today; "The MCP servers are free" ([GitHub](https://github.com/kaseya/mcp-servers)).
- **Liongard**: connect with your Liongard sign-in or a bearer token from the admin console ([docs](https://docs.liongard.com/docs/model-context-protocol-mcp-server)); needs a Liongard MCP subscription, and queries count against a monthly allowance ([FAQ](https://docs.liongard.com/docs/liongard-mcp-faqs)).
- **Microsoft Graph**: the MCP Server for Enterprise answers read-only questions about Entra ID data in your tenant; public preview, no extra cost ([Microsoft Learn](https://learn.microsoft.com/en-us/graph/mcp-server/overview)).
- **Microsoft Sentinel**: fully hosted MCP server for security operations, with Microsoft Entra sign-in and no infrastructure to deploy ([Microsoft Learn](https://learn.microsoft.com/en-us/azure/sentinel/datalake/sentinel-mcp-overview)). Data exploration needs a Sentinel data lake and a qualifying security role; its graph tools are in preview and graph queries are billed ([data exploration](https://learn.microsoft.com/en-us/azure/sentinel/datalake/sentinel-mcp-data-exploration-tool)).
- **N-able** (N-central, N-sight): hosted by N-able, with production, preview and read-only addresses; the production address can run scripts on devices ([developer guide](https://developer.n-able.com/gql/docs/mcp)).
- **NinjaOne**: announced October 13, 2026; lets outside AI agents read NinjaOne data and take action within the access controls you already set ([press release](https://www.ninjaone.com/press/katana-ai-and-mcp/)), signing in with OAuth 2.0 ([data sheet](https://assets.ninjaone.com/wp-content/uploads/2026/10/N1-0902-NinjaOne-Katana-AI-datasheet.pdf)).
- **Pax8**: reads your Pax8 data and acts on your behalf ([developer docs](https://devx.pax8.com/docs/mcp-server)).
- **Rewst**: hosted by Rewst; creates and runs workflows ([docs](https://docs.rewst.help/rewst-documentation/documentation/rewst-agent/mcp-servers-and-rewst)).
- **ScalePad** (including Backup Radar): hosted by ScalePad; you sign in as your ScalePad user ([developer docs](https://developer.scalepad.com/docs/product-mcp)).
- **SentinelOne**: open-source Purple AI MCP (MIT) that you run; read-only ([GitHub](https://github.com/Sentinel-One/purple-mcp)).
- **SuperOps**: hosted by SuperOps; read and write for tickets and assets, read-only for everything else ([support article](https://support.superops.com/en/articles/16782631-superops-mcp-for-msps)).
- **Syncro**: generally available since August 11, 2026, hosted, included in every Syncro plan ([Syncro](https://syncrosecure.com/platform/mcp-server/)).
- **Veeam**: Veeam Intelligence MCP, which you run yourself with Docker or Node.js; needs an active license for a supported Veeam product other than Community Edition ([GitHub](https://github.com/veeam-ai/veeam-mcp-server/tree/main/servers/vi-mcp)).
- **Atera**: Atera's AI connects out to other tools' MCP servers ([support article](https://support.atera.com/hc/en-us/articles/24164010860700-AI-Center-Model-Context-Protocol-MCP-integrations)). A web search on November 2 found third-party hosted Atera servers but none from Atera.
- **Axcient**: open-source Public Gateway MCP (Apache-2.0) on Axcient's GitHub, which you run with your Axcient API key ([GitHub](https://github.com/Axcient/public-gateway-mcp)).
- **Acronis, Datto BCDR**: a web search and the vendors' GitHub organizations ([Acronis](https://github.com/acronis), [Datto](https://github.com/datto)) turned up community servers, ours among them, but none from the vendor. Kaseya's own servers cover Datto RMM, not Datto BCDR.

## About Thread's skills and MCP server

Thread is an AI service desk, not a connector library. Its MCP server lets Claude, ChatGPT, Claude Code or Cursor work in Thread itself: search tickets, manage contacts and run write actions ([guide](https://docs.getthread.com/super-magic/thread-mcp-server-developer-guide)). What each person sees follows their Thread permissions and plan: without AI Pro it is seven read-only tools, and the NinjaOne, ImmyBot, TimeZest, IT Glue and Hudu tools appear on Agentic plans once those integrations are set up ([same guide](https://docs.getthread.com/super-magic/thread-mcp-server-developer-guide)). Its public GitHub skill library is a set of prompts for Thread's own AI, each capped at 3,000 characters ([GitHub](https://github.com/Thread-Magic/super-magic-skills)). If you run Thread, its server covers Thread and those integrations; the rest of your stack needs one of the other options.

## Try an msp-skills connector

These are the 19 connectors we recommend today, because each one has been confirmed against a live tenant: [Acronis](https://msp-skills.compoundingteams.com/skills/acronis.md), [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md), [Axcient](https://msp-skills.compoundingteams.com/skills/axcient.md), [CIPP](https://msp-skills.compoundingteams.com/skills/cipp.md), [Cove](https://msp-skills.compoundingteams.com/skills/cove.md), [DataGate](https://msp-skills.compoundingteams.com/skills/datagate.md), [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md), [HaloPSA](https://msp-skills.compoundingteams.com/skills/halopsa.md), [HubSpot](https://msp-skills.compoundingteams.com/skills/hubspot.md), [Hudu](https://msp-skills.compoundingteams.com/skills/hudu.md), [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md), [ImmyBot](https://msp-skills.compoundingteams.com/skills/immybot.md), [NinjaOne](https://msp-skills.compoundingteams.com/skills/ninjaone.md), [QuickBooks](https://msp-skills.compoundingteams.com/skills/quickbooks.md), [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md), [SuperOps](https://msp-skills.compoundingteams.com/skills/superops.md), [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md), [WordPress](https://msp-skills.compoundingteams.com/skills/wordpress.md) and [Zammad](https://msp-skills.compoundingteams.com/skills/zammad.md). Servosity confirmed HaloPSA, HubSpot, QuickBooks, WordPress and Zammad on its own tenants; outside MSPs confirmed the rest ([receipts wall](https://msp-skills.compoundingteams.com/verified/)).

Each connector's page has setup steps for your AI app. Or run the installer yourself. On macOS or Linux:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

On Windows (PowerShell):

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

Swap `halopsa` for the connector you want. Before you connect, create the API key in your own tenant with just the permissions you want the AI to have. Ran one of the other 65 against your tenant? [Tell us it worked](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml) and the badge flips.

## FAQ

### Which MCP server should an MSP use in 2026?

It depends on which AI app your team uses and who you want holding your API keys. ChatGPT on the web or Microsoft 365 Copilot, or a need for team sign-in and a central audit log, points to a hosted gateway such as WYRE's Conduit, StackJack or MSPbots Connector; check that your AI app is on the gateway's setup list. One product with a vendor-built server points to that server. A local AI app such as Claude Desktop or Claude Code, no budget, and keys kept on your own machine point to msp-skills or WYRE's open-source servers.

### Is there a free MCP server for ConnectWise, Autotask or HaloPSA?

Yes. msp-skills and WYRE both publish free, open-source servers for all three that you run yourself, and StackJack has a free tier of 100 calls a month per connector. HaloPSA's own server runs on your Halo instance, and Kaseya says its early-access Autotask server is free. Of the msp-skills three, HaloPSA has been confirmed on a live tenant; ConnectWise PSA and Autotask are awaiting live verification.

### Where do my API keys live with a hosted MCP gateway?

On the gateway company's servers. Conduit encrypts each one in Microsoft Azure and says an operator with production access could in principle decrypt them ([security](https://conduit.wyre.ai/security/)). StackJack keeps them in Azure Key Vault ([security](https://stackjack.io/security/)). With msp-skills they stay on your own machine.

### Can I use msp-skills with ChatGPT?

In the ChatGPT desktop app, it should work: the app can run local MCP servers, which is how msp-skills runs ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)), but we have not yet confirmed a connector in it. ChatGPT on the web needs a remote server, and our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md) says not to run a connector that way until it is reprinted; 75 of the 84 install guides still carry that pause note. If your team works in ChatGPT on the web, a hosted gateway or your vendor's own hosted server is the better pick.

### Who makes msp-skills?

Servosity Inc., which also wrote this comparison. msp-skills is free and Apache-2.0 licensed, and anyone can contribute a connector or a fix ([CONTRIBUTING.md](https://github.com/Servosity/msp-skills/blob/main/CONTRIBUTING.md)).

## Read this next

- Is local safer? [Are MCP servers safe for MSP client data? →](https://msp-skills.compoundingteams.com/answers/are-mcp-servers-safe-for-msp-client-data.md) · [the Trust Center →](https://msp-skills.compoundingteams.com/governance.md)
- New to the term? [What is an MCP server? →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)
- Which connectors are proven: [the receipts wall →](https://msp-skills.compoundingteams.com/verified/) · [all connectors →](https://msp-skills.compoundingteams.com/skills/)
- Want to see one run against a real tenant before you choose? Bring your tool to a free Thursday Build Session. [Join a Build Session →](https://compoundingteams.com/build-sessions/?utm_source=msp-skills&utm_medium=referral&utm_campaign=answers-mcp-comparison)
