---
layout: default
title: "The MSP AI Question Book - 200+ plain-English questions your AI can answer"
description: "606 plain-English questions your AI can answer from the PSA, RMM, backup, security, and billing tools you already run - aggregated from all 51 free MSP Skills connectors. No code required, runs locally."
permalink: /question-book/
---

# The MSP AI Question Book

606 plain-English questions MSP owners ask their AI every day - aggregated from the 51 free, open source connectors on this site and grouped by tool category. Every question maps to a real, gate-verified command your AI agent runs against your PSA, RMM, backup, security, or billing stack - locally, without your data leaving your network. Find your tools, steal the questions.

Each connector installs in about 60 seconds and works with Claude, ChatGPT, Copilot, and any MCP-capable agent. New to the term? [What an MCP server is →](/what-is-an-mcp-server/)

## Analytics

### MSPbots

Free MSPbots MCP server: [install and ask →](/skills/mspbots/)

- Is our ticket backlog up or down versus last week?
- Pull the open tickets updated since June 1
- Export the whole tickets dataset to CSV for the QBR deck
- Is our open-ticket backlog up or down versus last week?
- What changed in open tickets since the last snapshot - rows added, removed, or edited?
- What columns does this dataset have, and what types?
- Export the entire dataset to CSV for the QBR deck
- Capture today's KPI snapshot (schedule it and history accrues)
- Stop pasting 19-digit IDs - name the dataset once
- Are my API key and resource bindings working?

## Backup/DR

### Acronis Cyber Protect Cloud

Free Acronis Cyber Protect Cloud MCP server: [install and ask →](/skills/acronis/)

- Whose backups failed last night?
- Which backup agents have gone offline across all my customers?
- Where am I billing for protection that isn't running?
- Whose backups succeeded, failed, or went stale across every customer last night?
- Show me each failed or missed backup in the last 24 hours, newest first
- Which customers have gone too long without a good backup (SLA breach)?
- Which backup agents have silently gone offline across all tenants?
- Did every customer's agents update after the rollout?
- Where am I billing for protection that isn't actually running?
- Which usage has no matching SKU, and which paid SKUs have zero usage?
- What grew or shrank in usage between two months?
- Give me the full picture on one customer before the call
- Which enabled customers are missing users, agents, or offering items?

### Afi

Free Afi MCP server: [install and ask →](/skills/afi/)

- Which resources across the whole fleet have no backup protection?
- Show me protected resources whose newest backup is older than 48 hours.
- Where am I over- or under-provisioned on Afi licenses?
- Which resources have no backup protection at all?
- Which protected resources have a stale backup (silent failures)?
- Is the whole fleet green this morning, or who failed?
- What is one tenant's full backup posture for a QBR or ticket?
- Am I over- or under-licensed on Afi seats?
- Who is jane.doe@example.com in Afi, across Multi-Geo tenants?
- Safely back up then release a departing employee's mailbox?

### Axcient x360Recover

Free Axcient x360Recover MCP server: [install and ask →](/skills/axcient/)

- Whose backups failed or went stale last night?
- Give me backup-compliance evidence for this client - restore-point age, AutoVerify, and an RPO pass/fail
- What does each client consume this month for invoice reconciliation?
- Whose backups failed or went stale across every client last night?
- Give me one row per client: devices total, failing, stale, RPO-breach, and AutoVerify-fail counts
- Which devices are past their recovery-point objective, grouped by client?
- Only show the devices actually breaching RPO at the cloud tier
- Produce backup-compliance evidence for one client (restore-point age + AutoVerify + RPO verdict)
- Only the compliance rows that fail RPO or AutoVerify
- What does each client consume for invoice reconciliation this month?
- Which devices does each appliance protect, and what state are those backups in?
- Find everything matching a client or device name across synced data
- Refresh the local mirror, then run the morning sweep

### Cove Data Protection

Free Cove Data Protection MCP server: [install and ask →](/skills/cove/)

- Which devices failed their last backup across all my customers since yesterday?
- Give me the month-end billing usage for every device as a CSV.
- Which devices and customers grew their backup storage the most this week?
- Which devices failed their last backup since yesterday?
- Which devices have had no successful backup in 3 days?
- What is the fleet-wide health rollup, broken down per customer?
- Which devices and customers grew their storage fastest this week?
- What is the month-end billing usage per device, with codes decoded?
- Which device SKUs or seat counts changed since last month?
- Which backup statuses flipped since my last snapshot?

### Datto BCDR

Free Datto BCDR MCP server: [install and ask →](/skills/datto-bcdr/)

- Which protected machines failed their last backup screenshot verification, across every client?
- Which agents haven't taken a local snapshot or synced offsite recently, fleet-wide?
- Which of my clients are most at risk right now in Datto?
- Which protected machines failed their last backup screenshot verification?
- Which agents are behind on local snapshots or offsite sync?
- What percentage of my fleet is actually recoverable right now?
- Which clients are most at risk across backups, alerts, and storage?
- Show me every open alert across the whole fleet, grouped by client.
- Which appliance runs out of local or offsite storage first?
- Which protected machines are paused, archived, or on an appliance that went dark?
- Which machines are running an outdated backup agent?
- Give me a one-page backup health report for a single client before the QBR.

### Servosity

Free Servosity MCP server: [install and ask →](/skills/servosity/)

- Where is my attention needed today, worst first?
- Build the backup section of Acme's QBR as a PDF
- Which installed agents still aren't pulling backups?
- Where is my attention needed today, ranked worst-first?
- What got worse since yesterday, and what recovered?
- Which clients have backups stale for 7+ days?
- Draft the follow-up email for every client with a stale backup
- Quarter-end: every client's QBR backup report in one pass
- Watch every client's restore queue during a DR event
- Does my Servosity bill match what I invoice my clients?

### SkyKick

Free SkyKick MCP server: [install and ask →](/skills/skykick/)

- Which of my SkyKick customers has a backup gap right now?
- Show me every mailbox that hasn't been snapshotted in the last two days.
- What's discovered but not protected across all my tenants?
- Which customers have a protection gap right now?
- Whose mailboxes haven't been snapshotted in 48 hours?
- What's discovered but not actually being backed up?
- Which tenants fall below our retention floor?
- Where is autodiscover off, so new mailboxes silently never enroll?
- What protection changed since my last review?
- What open alerts exist across the whole fleet, worst first?
- How does backup posture roll up by partner?

## Billing

### AppDirect

Free AppDirect MCP server: [install and ask →](/skills/appdirect/)

- Which AppDirect payments failed in the last 7 days, across every company?
- Reconcile AppDirect billing for the last 30 days - what's active but unbilled, overdue, or failed?
- Show me everything on AppDirect company <companyId>.
- Which payments failed or stalled in the last week, across every company?
- What's active-but-unbilled, overdue, or failed before month-close?
- What changed in subscriptions this week - new, ended, or suspended?
- Show one customer's full picture - users, subscriptions, invoices, opportunities.
- What does my assisted-sales pipeline look like by status?
- Which open opportunities have gone stale?
- Find any company, subscription, invoice, or opportunity by keyword.
- Pull the whole marketplace into a local mirror for offline analysis.

### Gradient MSP

Free Gradient MSP MCP server: [install and ask →](/skills/gradient/)

- Push tonight's usage counts from this file in one shot
- Show me which accounts' usage changed since my last push
- Send this alert and tell me when the PSA ticket actually exists
- Push a whole file of usage counts and rebuild billing exactly once?
- Which accounts' usage changed between my last two pushes?
- Send an alert and confirm the PSA ticket was actually created?
- Which of my dispatched alerts never became tickets?
- Which accounts are unmapped or missing a vendor SKU?
- Is my integration ready to flip to active?
- Add a single ad-hoc unit count for one account and service?
- Are my credentials valid and what's my integration status?

### Pax8

Free Pax8 MCP server: [install and ask →](/skills/pax8/)

- Where is my Pax8 billing leaking this month?
- What is my recurring revenue and margin right now, by product?
- Which customers are about to blow past their usual usage?
- Where is my billing leaking - invoiced for a cancelled product, or active but never billed?
- Can I catch that leakage before the next invoice finalizes?
- What is my MRR and margin right now, broken down by product?
- Which usage summaries are running hot before they hit the invoice?
- What changed in my book of business this week - new, cancelled, resized subscriptions?
- Which customers cost the most across every invoice?
- Everything about one customer - subscriptions, contacts, invoices, usage - in one view?
- Which products can I resell that match a vendor or keyword?

### QuickBooks Online

Free QuickBooks Online MCP server: [install and ask →](/skills/quickbooks/)

- Who owes us money, and how overdue are they?
- Which overdue invoices should I chase first?
- What does our cash position look like over the next month?
- Who owes us money, bucketed 0-30 / 31-60 / 61-90 / 90+?
- What do we owe vendors, and when is it due?
- Where does our cash stand across accounts, AR, and AP?
- What net cash movement is scheduled over the next 4 weeks?
- What is our DSO, and who are the slowest payers?
- Are the books clean enough to close this month?
- Which payments came in but were never applied to an invoice?
- Which customers are duplicated in our list?
- Who slipped an aging bucket since our last check?

### Sherweb

Free Sherweb MCP server: [install and ask →](/skills/sherweb/)

- What is my net margin per customer this month?
- Which subscriptions am I paying for but not billing back?
- What will bumping this customer to 25 seats cost before I commit it?
- What is my net margin per customer this month - receivable minus payable?
- Whose margin is sliding month over month before an account goes negative?
- Which active subscriptions am I paying Sherweb for but not billing the customer?
- Where am I absorbing metered usage I never billed back?
- Which subscriptions have more seats paid than seats actually used?
- What changed on my payable charges since the last sync - new, vanished, or repriced?
- What subscriptions were added, cancelled, or resized across my whole book this month?
- How many total seats of each product do I carry across every customer?
- What will a seat change cost before I actually submit the amendment?

### Xero

Free Xero MCP server: [install and ask →](/skills/xero/)

- Who owes us money, and how overdue is each one?
- Which authorised invoices are still owed with no payment applied?
- Do the books tie out before I sign off the close?
- Who owes us, and how overdue is each invoice?
- What do we owe suppliers, bucketed by age?
- Which contacts carry the most receivable risk?
- Which authorised invoices are still owed with no applied payment?
- Which bank transactions are unreconciled, and what might they match?
- Do the GL control accounts tie to outstanding invoices at close?
- What posted to a single account, as a running balance?
- What changed in the organisation since last week?
- Give me one state-of-the-org summary in a single call.
- Find every synced record matching a keyword.

## CRM

### HubSpot

Free HubSpot MCP server: [install and ask →](/skills/hubspot/)

- Which of my open deals have gone cold with no activity in three weeks?
- What's my pipeline health right now - count, dollars, and what's at risk per stage?
- How are open deals spread across my reps by stage and dollar value?
- Which open deals have gone cold with no engagement in the last three weeks?
- Which of my contacts haven't been touched in a month?
- What's my pipeline health right now - per-stage count, dollars, and what's at risk?
- How is the open-deal load spread across my reps?
- Who should I call today, ranked by stale-days, deal size, and stage?
- Which are my top deals by composite score (signal, amount, stage, recency)?
- What's the full activity trail for a specific deal - every call, email, meeting, note, and task?
- Which meetings were ever Scheduled in a given month, even after they flipped to Completed or No Show?

### Pipedrive

Free Pipedrive MCP server: [install and ask →](/skills/pipedrive/)

- Which open deals has nobody touched in two weeks, worst dollar value first?
- What's my weighted forecast for this quarter by pipeline?
- Give me the full picture on Jane Smith before my call.
- What's my weighted forecast for this quarter, and what's expected to close?
- Which deals are stuck in a stage longer than that stage usually takes?
- Which open deals have no next activity scheduled?
- Rank my reps by won value over the last 90 days.
- What changed since yesterday, and who do I need to call today?
- Which deals did we lose in the last six months, with reasons, for a re-engagement push?
- Find likely-duplicate organizations so I can clean up the CRM.
- Search every synced deal, person, and organization for a name.
- Pull my whole pipeline into a local copy for offline, zero-API-call analysis.

### Salesbuildr

Free Salesbuildr MCP server: [install and ask →](/skills/salesbuildr/)

- Which quotes have been sitting sent or approved for two weeks, and how much revenue is at risk?
- Show me every quote line priced under 20% markup.
- Which companies, contacts, and products are missing the external ID my PSA sync needs?
- Which sent or approved quotes are aging, and how much is at risk?
- Which quote line items are priced below my markup floor?
- How does my quote pipeline convert stage by stage, in count and dollars?
- Where have per-company pricing-book prices drifted from the master catalog?
- What's my win rate by owner, stage, or category?
- What's my probability-weighted recurring-revenue forecast on the open pipeline?
- Which catalog products have I never quoted to a given company?
- Which records are missing the external ID my PSA sync depends on?

## Documentation

### Hudu

Free Hudu MCP server: [install and ask →](/skills/hudu/)

- Which clients have the worst documentation completeness right now?
- Show me vault passwords that haven't been rotated in 180 days, grouped by company
- What expires in the next 30 days across all my clients?
- Which clients have the worst documentation completeness?
- What's expiring in the next 30 days across every client?
- Which vault passwords are overdue for rotation?
- Which knowledge-base articles are stale and probably out of date?
- Which assets have drifted from their layout's current schema?
- Give me one worst-first hygiene scorecard across every company.
- Find everything matching a keyword across all synced docs.
- Resolve this Hudu link to its asset, company, and relations.
- Which PSA/RMM records don't map to a live Hudu asset?
- Scaffold a new client's docs from our house template.

### IT Glue

Free IT Glue MCP server: [install and ask →](/skills/itglue/)

- Which client does this Fortinet firewall belong to?
- Which credentials haven't been rotated in over a year, by client?
- Which clients are under-documented right now?
- Which client owns this device, serial number, or contact?
- Which clients are under-documented, thinnest first?
- Which credentials haven't been rotated in a year, grouped by client?
- What changed across every client since a given date?
- Which contacts are duplicated across or within clients?
- Everything we know about one client, in a single offline read?
- Which records are orphaned after a client was offboarded?
- List every organization in the account.

### Liongard

Free Liongard MCP server: [install and ask →](/skills/liongard/)

- What changed across all my clients in the last 7 days?
- Which Liongard collectors have gone stale?
- Give me the MFA-enabled count for every system as a CSV.
- What changed across all my clients in the last 24 hours?
- Which collectors (launchpoints) have gone stale?
- Which agents are offline right now, and whose environment do they serve?
- Give me one health scorecard for the whole estate.
- Show me one client's complete picture in a single command.
- Which inspections failed or errored across the estate?
- Pull one metric across every system, CSV-ready for a report.
- Which systems breach a threshold, like patch age over 30 days?
- Where are my monitoring gaps (systems with no launchpoint, environments with no systems)?
- Which environments are still missing a given inspector?
- What is the full change history for one system?
- Search everything I've synced for a term.

### PandaDoc

Free PandaDoc MCP server: [install and ask →](/skills/pandadoc/)

- Which proposals are stalled?
- How much money is tied up in open quotes right now?
- Which clients have gone cold?
- Which documents were sent but never completed?
- How much money is tied up in open quotes?
- What does my whole document funnel look like right now?
- How long has each document sat in its current status?
- Which clients haven't signed anything in a month?
- Who should I follow up with today?
- Which recipients actually open and sign vs. let documents sit?
- Which templates actually close?
- Which sent documents have no auto-reminder set?
- What changed in the last day?

## Incident Response

### PagerDuty

Free PagerDuty MCP server: [install and ask →](/skills/pagerduty/)

- What's our mean time to acknowledge and resolve by service over the last 30 days?
- Which services have a broken escalation chain or a single point of failure?
- Who's on call for the Payments service right now, and when's the handoff?
- What's on fire right now, ranked by SLA risk?
- Who's on call for a service right now, and when's the handoff?
- What's our MTTA and MTTR by service this month?
- Which services have a broken escalation chain or single point of failure?
- Where does a schedule have nobody on call over the next two weeks?
- Which responders carry the most pages and off-hours load?
- Which services are the noisiest?
- What changed right before this incident broke?
- What's the full timeline of this incident?
- Which open incidents are quietly rotting with no recent activity?

### Rootly

Free Rootly MCP server: [install and ask →](/skills/rootly/)

- What's our mean time to acknowledge and resolve by service over the last quarter?
- Where does an on-call schedule have an unstaffed gap in the next two weeks?
- Who's on call right now across every service, escalation tier included?
- Who's on call right now across every service and schedule?
- What past incidents are most similar to this one?
- What actually fixed this service the last time it broke?
- What's our MTTA and MTTR by service this quarter?
- Where does an on-call schedule have an unstaffed gap?
- Is it safe to deploy this service right now?
- Give me one screen for this active incident.
- Which incidents are breaching or about to breach SLA?
- Which open action items are overdue, grouped by owner?
- Draft a paste-ready post-mortem skeleton for this incident.

## Monitoring

### Better Stack

Free Better Stack MCP server: [install and ask →](/skills/betterstack/)

- Which monitors would page nobody if they went down right now?
- What was our MTTA and MTTR over the last 30 days, by monitor?
- What's down right now and is anyone actually paged?
- Which monitors would page nobody if they failed?
- What's our MTTA and MTTR over the last 30 days, by monitor?
- Which monitors are the noisiest over the last week?
- Is anyone actually on call right now, or is there a gap?
- Which heartbeats are most at risk of a silent miss?
- Are any status pages green while a monitor has an open incident?
- How healthy is each client group right now?
- Give me one health board for the whole account.
- Which open incidents are oldest and still unacknowledged?

## Network Monitoring

### Domotz

Free Domotz MCP server: [install and ask →](/skills/domotz/)

- Which client sites have devices down right now?
- Give me one asset inventory across every site
- Show me new devices that appeared anywhere in the last 24 hours
- Is anything on fire across all my sites?
- Which Collectors (sites) are offline or degraded right now?
- What devices are offline across every client?
- What new devices appeared on any network in the last day?
- Where are IP conflicts across the fleet?
- Which devices can't be fully monitored (auth or SNMP gaps)?
- How many devices of each vendor do we manage?
- Which Collectors have gone quiet (stale sync)?
- Find a hostname or IP anywhere in the synced fleet

## PSA

### Autotask PSA

Free Autotask PSA MCP server: [install and ask →](/skills/autotask/)

- Which approved time haven't we invoiced yet?
- What's gone stale on the service desk in the last week?
- Give me the full picture on company 1234 before my 2 o'clock
- Which approved time entries haven't been invoiced yet?
- How stale is the service desk right now, bucketed by age?
- Which open tickets has nobody touched in a week?
- Which unassigned tickets should the dispatcher pick up first?
- Who's overloaded before I assign the next ticket?
- How burned are our block-hour contracts, and when do they run out?
- Everything we know about one company - tickets, contacts, contracts, config items, opportunities?
- What's the month-end billing picture - unbilled time, contract burn, money on the table?
- What's the label-to-ID map for the ticket status picklist?

### ConnectWise PSA (Manage)

Free ConnectWise PSA (Manage) MCP server: [install and ask →](/skills/connectwise-manage/)

- Which tickets did we close this week with no time logged?
- What's gone stale on the Help Desk board in the last three days?
- Give me the full picture on Acme Corp before my 2 o'clock
- Which tickets did we touch this week that have zero time logged against them?
- Which clients are about to blow through their block-hours agreement?
- What does the Help Desk board look like right now - age, owner, priority?
- Which open tickets has nobody touched in five days?
- Who has bandwidth for the next ticket?
- Which tickets are sitting unassigned on the board?
- Everything we know about one client - contacts, agreements, configurations, open tickets?
- Write me a valid conditions filter for open Help Desk tickets

### HaloPSA

Free HaloPSA MCP server: [install and ask →](/skills/halopsa/)

- What's about to breach SLA in the next 24 hours, sorted by time-to-breach?
- Give me the whole story for this client on one screen - tickets, contracts, hours left, assets.
- Who's overloaded right now - open tickets, hours logged, oldest open ticket per agent?
- What's about to breach SLA in the next 24 hours?
- What's the dispatcher view across all agents and teams?
- Who's overloaded right now?
- What's the whole story for this client, on one screen?
- How much contract time is left, and who's tracking over their bank?
- Which clients have stale tickets aging out that I should close?
- What changed in Halo since this morning while I was in a meeting?
- Which KB article should the tech link to for this ticket?

### Kaseya BMS

Free Kaseya BMS MCP server: [install and ask →](/skills/kaseya-bms/)

- Which queues are underwater this morning, and how many tickets are going stale?
- How many approved billable hours are sitting unbilled per client right now?
- Who's overloaded and who can take the next ticket?
- Which queues are underwater and what's going stale before standup?
- Which open tickets haven't been touched in a week, oldest first?
- How much of each contract have we burned this quarter?
- What approved billable time is ready to invoice, by account?
- What's the open sales pipeline by stage, and which deals have slipped?
- Find every ticket mentioning a phrase across the whole tenant
- Sync the tenant into a local mirror for instant offline queries

### SuperOps

Free SuperOps MCP server: [install and ask →](/skills/superops/)

- Who's about to breach SLA, and on whose queue?
- Give me the full picture on Acme Corp before my 2 o'clock
- Which endpoints are missing a critical patch and actively alerting?
- Who's about to breach SLA, grouped by technician?
- Which clients have alerts still sitting unresolved?
- Which open tickets has nobody touched in a week?
- Everything about one client - sites, users, contracts, tickets, assets, open invoices?
- Where is billable time concentrated before this month's invoicing?
- Give my triage agent one ticket with its worklogs, client, and SLA in a single read
- Search every synced ticket, asset, and client for "disk full"

### Syncro

Free Syncro MCP server: [install and ask →](/skills/syncro/)

- Which customers have logged labor we never invoiced?
- Bucket our unpaid invoices into aging tiers
- Give me a full snapshot for customer 12345
- Which customers have logged time we never invoiced?
- Which closed tickets had billable time that was never invoiced?
- How is our unpaid AR aging (0-30/30-60/60-90/90+)?
- What is our revenue per labor hour by customer this quarter?
- Which open tickets are going stale with no recent activity?
- Which assets are missing the most critical patches?
- Which customers generate the most RMM alert noise?
- Which RMM alerts never became a ticket?
- Give me one cross-entity card for a single customer.

## RMM

### Action1

Free Action1 MCP server: [install and ask →](/skills/action1/)

- Which endpoints across all my clients are missing the most patches?
- Rank our CVEs by how many endpoints they hit, known-exploited ones first
- Which agents have gone dark across the whole fleet in the last two weeks?
- Which endpoints across all clients are missing the most patches?
- Which CVEs hit the most machines, weighted by severity and known-exploited status?
- Which agents have stopped checking in across the fleet?
- What is the patch-and-vulnerability posture for each client organization?
- Which endpoints are waiting on a reboot to finish a patch cycle?
- Rank every endpoint by overall risk (missing updates, open CVEs, reboot, staleness)
- What software is installed across the whole fleet, deduped by version?
- What changed since the last sync - what got remediated, what is newly missing?
- List the managed endpoints in one client organization
- What updates are available or missing for one organization?

### Atera

Free Atera MCP server: [install and ask →](/skills/atera/)

- Which Atera agents have gone dark in the last 30 days?
- Which open Atera tickets are about to breach SLA?
- Which customers have managed agents but no active contract?
- Which agents have gone offline or stopped checking in?
- Which open tickets are closest to breaching SLA?
- Who is overloaded on the service desk right now?
- What contracts expire in the next 60 days?
- What's my full book of business by customer and contract mix?
- Which machines generate the most alerts over a week?
- What's the patch-compliance picture across the fleet?
- Which machines are running an end-of-life OS?
- What changed across agents, tickets, and alerts in the last 24 hours?

### Datto RMM

Free Datto RMM MCP server: [install and ask →](/skills/datto-rmm/)

- Which endpoints across all my clients have stopped checking in?
- Show me every endpoint where antivirus is missing or not running
- Which devices have hardware warranties expiring in the next 60 days?
- Which devices haven't checked in for 30 days, across every client?
- Where is antivirus missing, disabled, or not running?
- Which endpoints are most behind on patches right now?
- Which devices have warranties expiring in the next 60 days?
- Which devices are generating the most alert noise this week?
- Give me a one-page health scorecard for a client before the QBR.
- How many copies of an app are installed fleet-wide, and which versions?
- Which devices are running an out-of-date RMM agent?

### Level

Free Level MCP server: [install and ask →](/skills/levelio/)

- Which Level devices are most at risk right now?
- Which Level devices have gone dark in the last 30 days?
- Give me a per-client posture scorecard for my Level fleet
- Which devices are most at risk across alerts, patches, score, and staleness?
- Which devices have gone dark and stopped checking in?
- What is my fleet-wide patch exposure, by category?
- How is my fleet broken down by OS, platform, or group?
- Where are my active critical fires, clustered by group?
- Give me a per-client posture scorecard for QBRs.
- Which devices are below my security-score threshold?
- Which devices are waiting on a reboot to finish patching?
- Which monitors fire most often across the fleet?
- What changed since yesterday - new alerts, updates, device activity?

### N-able N-central

Free N-able N-central MCP server: [install and ask →](/skills/n-central/)

- Where is EXCHANGE01?
- What's red right now, worst first, by customer?
- Is our N-central API access healthy, and when does it expire?
- What's red right now, grouped by customer and ranked by severity?
- Where is EXCHANGE01 - server, service org, customer, site?
- Find anything named acme across every server we run
- Which devices are missing the Backup Plan custom property, by customer?
- Which devices have no maintenance window before the June 15 patch wave?
- Is the JWT healthy, and when does the API user's password kill it?
- Hardware and software inventory for one device
- Every device, exported for the QBR or your documentation tool

### Nerdio Manager

Free Nerdio Manager MCP server: [install and ask →](/skills/nerdio/)

- Which host pools have autoscale disabled or drifting across all my customers?
- Give me a per-customer billing and unpaid-balance rollup for May
- Run script 42 on these three accounts and tell me when all of them are done
- Which host pools have autoscale off or drifting across every customer?
- What is running right now across all accounts, and where?
- What did each customer get billed this period, and who is unpaid?
- Which customers' Azure usage spiked month-over-month?
- List every customer account I manage
- Show the host pools for one account
- Which Intune devices does this account have?
- Did that backup or provisioning job actually finish?
- Run one scripted action across many accounts and wait for all of them
- Search everything I have synced, offline

### NinjaOne

Free NinjaOne MCP server: [install and ask →](/skills/ninjaone/)

- Which clients are below 95% patch compliance right now?
- Which endpoints across the whole fleet have no backup?
- Every device carrying this threat, fleet-wide
- Which organizations are below 95% patch compliance?
- Which endpoints across the fleet have no backup at all?
- Which devices is a given threat on, fleet-wide?
- Which devices have antivirus definitions older than a week?
- What is each organization's overall health score, and why?
- Which devices have not checked in for two weeks?
- Which devices are running an end-of-life operating system?
- Where is a software title sprawling across too many versions?
- Did patch compliance get better or worse since last week?
- Search every synced device, organization, and alert for a string?

### Tactical RMM

Free Tactical RMM MCP server: [install and ask →](/skills/tactical-rmm/)

- Which agents across all my clients have gone dark in the last week?
- Where do I stand on pending patches and reboots across every client?
- Run whoami on every online Windows agent - show me the cohort before it executes
- What's the overall health of my fleet right now?
- Which agents need attention first?
- Which agents have gone dark or stopped checking in?
- Where are patches and reboots pending across every client?
- What changed across the fleet in the last few hours?
- Which endpoints have no checks configured (monitoring gaps)?
- What's each client's posture in a single row?
- Which checks are failing on the most agents?
- Which agents have a given software package installed?
- Summarize alerts by severity over the last day
- Which agents have a named Windows service stopped?

## Security

### Abnormal Security

Free Abnormal Security MCP server: [install and ask →](/skills/abnormal/)

- What new email threats hit us in the last 24 hours that nobody has remediated yet?
- Pull last quarter's attacks-stopped and impersonation numbers for the client report
- Give me the full account-takeover risk picture for jane@acme.com
- What new, unremediated email threats need attention right now?
- Pull a client-ready security report for the quarter
- What is the account-takeover risk picture for this employee?
- Is this vendor showing email-compromise signs?
- Remediate a threat and block until it actually completes
- List the latest Abnormal cases
- How many attacks did we stop this week?
- Find threats from a spoofed sender

### Blumira

Free Blumira MCP server: [install and ask →](/skills/blumira/)

- Show me the highest-priority open findings across every client account, ranked into one queue
- Which detection rules fell out of coverage versus our basis ruleset, across all accounts?
- Which domain controllers went stale or unprotected across every client?
- What are the worst open findings across all my client accounts right now?
- What changed since my last sync, new, resolved, or status-changed findings?
- What's my mean-time-to-resolve per account over the last month?
- Which open findings are about to breach my age-based SLA?
- Which detection rules are missing or disabled versus our basis ruleset?
- Which domain controllers are stale or unprotected across every account?
- Which findings were resolved and then re-fired?
- Which detections keep firing over and over across accounts?
- Give me one per-account rollup of open findings, age, and agent health?
- Which findings mention this IOC, hostname, or user in their evidence?
- Pull every account's Blumira data into a local mirror for offline questions?
- Give me a flat finding-to-owner-to-status table to reconcile against my PSA?
- Which analyst is carrying the most open findings, and how old are they?

### CIPP

Free CIPP MCP server: [install and ask →](/skills/cipp/)

- Show me MFA registration across every tenant
- Which assigned M365 licenses are going unused across all my clients?
- Offboard this CSV of departures across their tenants without tripping rate limits
- Which tenants still have users without MFA registered?
- How does Conditional Access coverage compare across all tenants?
- Where am I paying for M365 licenses nobody uses?
- Which licensed accounts haven't signed in for 90 days, across every client?
- Which tenants drifted off our security baseline since the last check?
- Pull one read across every client tenant at once and keep it locally
- Offboard a batch of departures from a CSV with 429 backoff and resume
- Are my CIPP credentials and connectivity healthy?

### CrowdStrike Falcon

Free CrowdStrike Falcon MCP server: [install and ask →](/skills/crowdstrike/)

- Give me one severity-sorted alert queue across every tenant
- Rank the critical vulnerabilities across every CrowdStrike tenant
- Which hosts haven't reported a sensor heartbeat in two weeks, across all tenants?
- What should I triage first across all my client tenants right now?
- Rank the critical vulnerabilities across every tenant?
- Which hosts haven't reported a sensor heartbeat lately?
- Give me one posture scorecard per tenant for the QBR deck?
- Which tenants are under-protected versus my prevention-policy baseline?
- Which single fix clears the most hosts and tenants?
- Which tenants got worse since the last sync?
- Map every child CID, CID group, and role grant across my MSSP?
- Search every synced host, alert, vuln, and policy across all tenants?
- Pull every child tenant's Falcon data into a local mirror for offline queries?

### Huntress

Free Huntress MCP server: [install and ask →](/skills/huntress/)

- Show me every open Huntress incident across all my clients, oldest first.
- Am I paying for more Huntress seats than I have agents deployed?
- Which Huntress agents haven't called home in a week?
- Which incidents are oldest across every client org?
- Where are my posture gaps - stale callbacks, disabled Defender or firewall?
- Has this IP or file hash touched any of my clients?
- Am I billed for more seats than I have agents deployed?
- Which agents went dark in the last week?
- What is my mean time-to-resolve per client?
- What changed across the fleet since my last shift?
- Give me a QBR scorecard for one client.

### KnowBe4

Free KnowBe4 MCP server: [install and ask →](/skills/knowbe4/)

- Which users clicked the bait in two or more phishing tests in the last 90 days?
- Rank the users whose risk score worsened the most this quarter
- Who clicked a phish but has no passed training to show for it?
- Who clicked the bait in more than one phishing test?
- Whose risk score is getting worse this quarter?
- Who clicked a phish but never passed training?
- Which active users have zero training or zero phishing coverage?
- Is training actually working for the Finance group?
- Who are my highest-risk users, with the why behind the score?
- Which departments are driving our risk up?
- Assemble the full client quarterly review in one command
- Who never reports a simulated phish?
- Is my synced data fresh enough to trust a clicker hunt?

### Microsoft Graph

Free Microsoft Graph MCP server: [install and ask →](/skills/microsoft-graph/)

- Which M365 licenses are we paying for but not using?
- Who holds global admin or other privileged roles right now?
- What security alerts are new and still open since yesterday?
- Which SKUs are we paying for but not fully using, ranked by wasted seats?
- Which disabled or guest accounts still hold a paid license?
- Who exactly is consuming one specific SKU before I reclaim seats?
- Who holds a privileged directory role right now, and which holders are guest or disabled?
- What open security alerts are new since yesterday, by severity and source?
- Which Intune devices are non-compliant, unencrypted, or stale this month?
- Which groups are ownerless, empty, or guest-heavy across the tenant?
- Where does this tenant stand overall - users, license waste, admins, alerts, device drift?

### Proofpoint TAP

Free Proofpoint TAP MCP server: [install and ask →](/skills/proofpoint/)

- What malicious clicks and messages got through in the last 12 hours?
- Who is both Very Attacked and a top clicker right now?
- Give me the full incident brief for this threatId
- What malicious clicks and messages got through overnight?
- Who is both Very Attacked and a top clicker?
- Give me the full incident brief for a threatId
- What indicators should I block from this threat?
- Show me every event that touched one user
- Who are my Very Attacked People this month?
- Which permitted clicks and delivered threats still need a response?
- What threats are inside this campaign?
- Decode this urldefense-rewritten link to its real target
- Is my synced threat data fresh enough to trust an offline query?

### RocketCyber

Free RocketCyber MCP server: [install and ask →](/skills/rocketcyber/)

- What broke across all my RocketCyber clients in the last 24 hours?
- Which devices are most at risk right now?
- Which suppression rules are stale and could be masking alerts?
- What broke across all my clients overnight?
- Which devices went dark this week?
- How fast is my SOC actually resolving incidents?
- Which machines are riskiest in Defender right now?
- Is this client's Microsoft 365 posture improving?
- Which suppression rules are stale and may hide detections?
- What detection events fired, by verdict?

### runZero

Free runZero MCP server: [install and ask →](/skills/runzero/)

- Which of our assets are most exposed right now?
- What appeared or disappeared on our attack surface since last week?
- Which of our assets are affected by CVE-2024-3094?
- Only the internet-facing ones?
- What changed on our attack surface since last week?
- What newly became exposed or vulnerable since the last sync?
- Which assets are affected by a given CVE?
- Where are risky services concentrated in a subnet?
- Which TLS certificates are expiring soon or using weak crypto?
- Which assets are stale, end-of-life, or unowned?
- How many assets run a given software product, by version?
- Scan a subnet on a site and wait for the result?

### SentinelOne

Free SentinelOne MCP server: [install and ask →](/skills/sentinelone/)

- Give me the ranked triage worklist of every open threat across all my client sites
- Show me every endpoint this hash touched and which are still active, not mitigated
- Which endpoints are decaying worst first - offline, out-of-date, infected, or under-protected?
- What should I triage first across all my client sites right now?
- Where did this malicious file spread, and which endpoints are still active?
- Which endpoints are decaying - offline, out-of-date, infected, or under-protected?
- Which clients have protection gaps (detect-only, Ranger off, firewall off)?
- What changed across the whole fleet since yesterday?
- Which threats keep coming back after we mitigated them?
- Are we hitting our mitigation SLA, and where are the breaches?
- Rank my clients by risk so I know which tenant to call first?
- Give me one posture scorecard per client for the QBR deck?
- Pull every site's SentinelOne data into a local mirror for offline queries?

### ThreatLocker

Free ThreatLocker MCP server: [install and ask →](/skills/threatlocker/)

- Show me every pending approval across all my clients, grouped so duplicate files collapse into one row
- Approve this file everywhere it's pending, but show me the plan before you do
- Which clients are about to lose audit evidence, and pull it before they do
- What application approvals are pending across all my clients right now?
- Approve this file hash everywhere it's pending, but show me the plan first?
- Which clients are about to lose audit evidence to the 31-day retention cliff?
- Export every client's audit log before it ages off?
- What security-relevant changes (protection off, policy edits, maintenance) happened across all tenants this week?
- Which ThreatLocker agents are offline or stale across every client?
- Where does this binary live across my whole book, approved or pending?
- Pull every tenant's ThreatLocker data into a local mirror for offline queries?

---

Want a question answered live? **[Build Sessions](https://compoundingteams.com/build-sessions)** are free every Thursday - bring one of these questions and your own tenant.
