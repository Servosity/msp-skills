# SentinelOne CLI

**Every SentinelOne v2.1 management endpoint, plus an offline SQLite store and cross-entity analytics  -  fleet health, threat triage, blast radius, drift  -  that no console view offers.**

Query and manage your whole SentinelOne fleet from the terminal: agents, threats, activities, sites, groups, exclusions, Ranger, and more. Sync to a local store for offline full-text search, then run analytics the console can't  -  `fleet-health stale` ranks decaying endpoints, `threats blast-radius` traces one hash across the fleet, `whatchanged --since 24h` diffs overnight, and `posture` rolls up a per-tenant scorecard. Ships an MCP server so an AI agent can drive all of it.

Learn more at [SentinelOne](https://twitter.com/frikkylikeme).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `sentinelone-cli` binary and the `pp-sentinelone` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install sentinelone
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install sentinelone --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install sentinelone --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install sentinelone --agent claude-code
npx -y @mvanhorn/printing-press-library install sentinelone --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/sentinelone/cmd/sentinelone-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/sentinelone-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install sentinelone --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-sentinelone --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-sentinelone --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install sentinelone --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/sentinelone-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SENTINELONE_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/sentinelone/cmd/sentinelone-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "sentinelone": {
      "command": "sentinelone-mcp",
      "env": {
        "SENTINELONE_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with a SentinelOne API token: create a Service User (Settings > Users > Service Users), generate its API token, then set `SENTINELONE_API_TOKEN`. Tokens are sent as `Authorization: ApiToken <token>` and inherit the generating user's role and scope. Point the CLI at your console with `SENTINELONE_BASE_URL=https://<your-console>.sentinelone.net/web/api/v2.1`. Run `sentinelone-cli doctor` to confirm reachability and auth.

## Quick Start

```bash
# First export SENTINELONE_BASE_URL=https://<your-console>.sentinelone.net/web/api/v2.1 and SENTINELONE_API_TOKEN, then confirm the console is reachable and the token is valid
sentinelone-cli doctor

# Pull agents, threats, activities, sites, groups into the local store
sentinelone-cli sync --full

# Rank the riskiest, most-decayed endpoints first
sentinelone-cli fleet-health stale --agent

# List active threats, narrowed to the high-gravity fields
sentinelone-cli threats get --json --select data.threatInfo.threatName,data.threatInfo.sha1,data.agentRealtimeInfo.agentComputerName

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Time-Travel & Diffing
- **`whatchanged`**  -  One answer to 'what changed across all my tenants since I logged off?'  -  new threats, newly-offline or newly-unhealthy agents, version regressions, and protection-mode flips. Needs at least 2 syncs of local history.

  _Reach for this instead of paging Get_Threats + Get_Agents and diffing by hand  -  it returns the cross-entity delta over a window that no single API call provides._

  ```bash
  sentinelone-cli whatchanged --since 24h --agent
  ```
- **`threats verdicts`**  -  Flags threats whose analyst verdict, confidence level, or incident status changed since the last sync  -  suspicious to malicious, or an auto-mitigated threat re-opened  -  so nothing flips silently. Needs at least 2 syncs of local history.

  _Use to catch silent verdict flips  -  it diffs against stored prior state the API doesn't retain._

  ```bash
  sentinelone-cli threats verdicts --changed --agent
  ```

### Threat Intelligence Joins
- **`threats recurrence`**  -  Surfaces threats whose same hash or name re-appears across endpoints  -  or returns on an endpoint after a prior mitigation  -  the signal of an unkilled root cause. Needs at least 2 syncs of local history.

  _Use when a threat keeps coming back  -  it identifies the recurring hash/endpoint pair a single threat listing can't reveal._

  ```bash
  sentinelone-cli threats recurrence --by-agent --agent
  ```
- **`threats mttr`**  -  Computes mean time from threat detection to mitigation per site, and flags SLA breaches and the longest-unresolved threats. Needs at least 2 syncs of local history.

  _Use to report response performance  -  it derives detection-to-mitigation durations the API only exposes as raw timestamps._

  ```bash
  sentinelone-cli threats mttr --agent
  ```
- **`threats blast-radius`**  -  For one threat, instantly shows every endpoint it touched, which are mitigated vs still active, the affected sites/groups, and the spread timeline.

  _Use during incident response  -  it answers 'where else is this?' by joining one threat across the whole fleet and timeline._

  ```bash
  sentinelone-cli threats blast-radius 3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a --agent
  ```
- **`threats triage`**  -  One ranked, cross-site worklist of every open threat  -  scored by confidence × severity × age  -  so the morning triage order needs zero console scope flips.

  _Reach for this first each morning  -  it returns the cross-site triage order no single threats query computes._

  ```bash
  sentinelone-cli threats triage --agent
  ```
- **`agents dossier`**  -  Everything about one endpoint on one card: agent state, full threat history, recent activities, and site/group membership  -  the IR handoff view.

  _Use when drilling into one box during incident response  -  it joins the endpoint's whole story in one call._

  ```bash
  sentinelone-cli agents dossier "FINANCE-LT-042" --agent
  ```

### Fleet Health & Coverage
- **`fleet-health stale`**  -  Ranks endpoints by a composite decay score  -  last-seen age, last-scan age, out-of-date agent version, and reduced or disabled protection  -  so the riskiest agents triage first.

  _Reach for this to answer 'which endpoints are rotting?'  -  a ranked health score the console never computes._

  ```bash
  sentinelone-cli fleet-health stale --agent
  ```
- **`coverage gaps`**  -  Lists endpoints in detect-only mode, with self-protection off, or with Ranger/firewall/device-control disabled  -  the 'are we actually protecting everyone?' compliance view per tenant.

  _Use for compliance/QBR prep  -  it finds unprotected endpoints by site that require joining policy state to site membership._

  ```bash
  sentinelone-cli coverage gaps --agent
  ```
- **`versions rollout`**  -  Shows agent-version distribution per site over time and flags sites stuck on EOL/old versions or stalled mid-upgrade-wave. Needs at least 2 syncs of local history.

  _Reach for this during an upgrade campaign  -  it tracks rollout progress per site, which requires snapshot history the API doesn't keep._

  ```bash
  sentinelone-cli versions rollout --agent
  ```
- **`ranger exposure`**  -  Surfaces unmanaged/rogue endpoints on each subnet by cross-referencing Ranger-discovered devices against managed agents, ranked by managed-peer density.

  _Reach for this to find blind spots  -  it joins network discovery to managed agents to surface rogue devices a single listing can't._

  ```bash
  sentinelone-cli ranger exposure --agent
  ```
- **`fleet-health summary`**  -  At-a-glance fleet counts  -  online/offline/decommissioned, infected, out-of-date, under-protected  -  across all sites in one call.

  _Use for the weekly fleet sweep  -  one command replaces the per-client CSV-export-and-pivot ritual._

  ```bash
  sentinelone-cli fleet-health summary --agent
  ```

### Reporting & Rollups
- **`posture`**  -  A one-page per-tenant rollup  -  agent health %, coverage %, open-threat count, oldest unresolved, version compliance  -  for the morning MSSP review or a client QBR.

  _Reach for this for a client-ready summary  -  it composes health, coverage, and threat metrics into one scorecard the API never returns._

  ```bash
  sentinelone-cli posture --agent
  ```
- **`exclusions audit`**  -  Flags risky exclusions  -  never matched by any threat, wildcard paths, and entries older than a threshold  -  the 'are we hiding real threats?' review.

  _Use for periodic security hygiene  -  it cross-references exclusions to threat history the API never correlates._

  ```bash
  sentinelone-cli exclusions audit --agent
  ```
- **`sites risk`**  -  Ranks sites/clients against each other by composite risk  -  open-threat density, coverage gaps, stale agents, MTTR  -  so you know which tenant to call first.

  _Reach for this for the portfolio view  -  it ranks every client by risk in one command._

  ```bash
  sentinelone-cli sites risk --agent
  ```

## Recipes


### Triage overnight

```bash
sentinelone-cli whatchanged --since 24h --agent
```

Cross-entity delta of new threats, lost/unhealthy agents, and mode flips since yesterday.

### Morning triage worklist

```bash
sentinelone-cli threats triage --agent
```

Every open threat across all sites, ranked by confidence × severity × age  -  the cross-site triage order the console can't show.

### Narrow a noisy threat feed

```bash
sentinelone-cli threats get --agent --select data.threatInfo.threatName,data.threatInfo.sha1,data.agentRealtimeInfo.agentComputerName
```

Pair --agent with --select dotted paths to pull only the high-gravity fields from deeply-nested threat objects.

### Find decaying endpoints

```bash
sentinelone-cli fleet-health stale --agent
```

Composite decay rank across last-seen, last-scan, version, and protection state.

### Trace a threat across the fleet

```bash
sentinelone-cli threats blast-radius 3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a --agent
```

Every endpoint a hash touched, mitigated vs active, with a spread timeline.

### Client QBR scorecard

```bash
sentinelone-cli posture --agent
```

Per-site health %, coverage %, open threats, and version compliance in one rollup.

## Usage

Run `sentinelone-cli --help` for the full command reference and flag list.

## Commands

### accounts

accounts operations

- **`sentinelone-cli accounts create`** - Create a new Account. This command requires Global permissions and an MSSP deployment. Consult with your SE before you run this command. An Account is a logical segment with permissions to configure features for specific Sites. Multiple Accounts can be useful for deployments with multiple Sites for third-parties (such as MSSP). Each Account has one or more SKUs, that you assign to Sites. If an Account has the Complete SKU, and you create a new Site in the Account, it will automatically have the Complete SKU. Best practice: Run "name-available" first, to make sure the name is unique in your deployment.
- **`sentinelone-cli accounts get`** - Get the Accounts, and their data, that match the filter. This command gives the Account IDs, which other commands require. <br>Accounts are created by a Global User or by SentinelOne. Each Account contains Sites, which can inherit assets and settings. Each Account has one or more SKUs, that you assign to the Sites. To have both Core and Complete Sites in an Account, the Account must have both SKUs.
- **`sentinelone-cli accounts get-by-id`** - Get Account data from a given Account ID. To get an Account ID, run "accounts".
- **`sentinelone-cli accounts update`** - Change the data of an Account. This command requires a Global user or an Account user and Admin role. Use this command to change the name, ID, SKUs and how they are distributed among Sites and Agents, and more. (See the Body sample.) Best practice:  Consult with your SentinelOne SE.

### activities

activities operations

- **`sentinelone-cli activities get`** - Get the activities, and their data, that match the filters.
 We recommend that you set some values for the filters. The full list will be too large to be useful.
- **`sentinelone-cli activities get-activity-types`** - Get a list of activity types. This is useful to see valid values to filter activities in other commands.

### agents

agents operations

- **`sentinelone-cli agents abort-scan`** - Immediately stop a Full Disk Scan on all Agents that match the filter. See "Initiate scan" to learn more about Full Disk Scan.
- **`sentinelone-cli agents approve-uninstall`** - If a user tries to uninstall the SentinelOne Agent from an endpoint, an uninstall request is sent to the Management. You must approve the request. <BR>After you approve a request, users see a message that the request was approved. They can restart to complete the Agent uninstall.<BR>We recommend that you do not approve these requests until you understand the reason for the request, you agree with the request, and you have alternative security for the endpoint until you install the Agent again.<BR>This command will approve pending uninstall requests for all Agents that match the filter.
- **`sentinelone-cli agents broadcast-message`** - You can send a message through the Agents that users can see. <BR>This is useful for endpoints that have human users. This command is supported on Windows and macOS endpoints (not supported on Linux). The message is sent to all endpoints that match the filter. <br>Put the message in the data parameter: "data":{"message":"<your message>"} <br>The message must be 140 characters or less.
- **`sentinelone-cli agents can-run-remote-shell`** - Who can run Remote Shell? Remote Shell is a powerful way to respond remotely to events on endpoints. It lets you open full shell capabilities - PowerShell on Windows and Bash on macOS and Linux. To be able to run a Remote Shell session, SentinelOne users require permissions, which are set on different levels. It can be confusing to know who has permission. Use this command to see if a username you created for someone else or the API, or your own name, has permission.<BR> If a user does not have Remote Shell permission, how can you grant it? First, you need the Control SKU. Then, the user must have a role with permission to use Remote Shell: Admin, SOC, IR Team. The IT role does not have Remote Shell permission, and the user must be responsible for the Account, Site, or Group on whose policy Remote Shell is enabled.
- **`sentinelone-cli agents clear-remote-shell`** - Remote Shell is a powerful way to respond remotely to events on endpoints. It lets you open full shell capabilities - PowerShell on Windows and Bash on macOS and Linux. <BR>For best practices, a Remote Shell session can be terminated in many ways: from the UI, from Agent timeouts, from endpoint or connections issues, and so on. If a shell closes at the same time that an Agent goes offline, Remote Shell status is incorrect on the Management. <BR>Use this command to clear the "open shell" flags on the Management. <BR>The IT user role does not have permissions to run this command.
- **`sentinelone-cli agents connect-to-network`** - After you run "disconnect from network" on endpoints, analyze the issue, and mitigate threats. Use this command to reconnect to the network all endpoints that match the filter. To learn more, see "Disconnect from Network".
- **`sentinelone-cli agents count`** - Get the count of Agents that match a filter. This command is useful to run before you run other commands. You will be able to manage Agent maintenance better if you know how many Agents will get a command that takes time (such as Update Software).
- **`sentinelone-cli agents decommission`** - If a user is scheduled for time off, or a device is scheduled for maintenance, you can decommission the Agent. This removes the Agent from the Management Console. <BR>When the Agent communicates with the Management again, the Management recommissions it and returns it to the Console. Use this command to decommission the Agents that match the filter.
- **`sentinelone-cli agents disable`** - Use this command to disable Agents that match the filter. <BR>Disabled agents run with minimal footprint and do not detect or mitigate threats, but they maintain connectivity with the Management Console. <BR>If the command returns "Insufficient permissions", make sure you have permissions for the Account, Site, or Group and a role that allows Disable Agent (Admin, IR team or IT).<BR>In the body of this command, the data parameter set is mandatory.
- **`sentinelone-cli agents disable-ranger`** - Disable Ranger from the Agents that match the filter.<BR>SentinelOne Ranger gives full visibility of all devices connected to your network. Ranger scans your corporate environment to identify and manage connected devices, even those not protected by or supported by SentinelOne. When Ranger is enabled on an Agent, the Agent adds "Scanner" to its functionality. It is the starting point for the Ranger scans.<BR>Best Practice: Disable Ranger on endpoints that are performance-sensitive and on endpoints that often connect to non-corporate networks.
- **`sentinelone-cli agents disconnect-from-network`** - Use this command to isolate (quarantine) endpoints from the network, if the endpoints match the filter. <BR>The Agent can communicate with the Management, which lets you analyze and mitigate threats. Best practice: For Active threats that spread, apply "Disconnect from network" immediately. In the policy, you can set this is to be automatic. When the Agent detects a high-confidence malicious threat, it will mitigate the threat (on Protect) with the action set by the policy. Then the Agent will immediately quarantine the endpoint. To make Disconnect from network automatic in an Account policy, run the "accounts/{id} command (see "Update Account") with: "networkQuarantine":true.
- **`sentinelone-cli agents enable`** - Use this command to enable disabled Agents that match the filter. <BR>If the command returns "Insufficient permissions", make sure you have permissions for the Account, Site, or Group and a role that allows Disable Agent (Admin, IR team or IT).<BR>In the body of this command, the data parameter set is mandatory.
- **`sentinelone-cli agents enable-ranger`** - SentinelOne Ranger gives full visibility of all devices connected to your network. Ranger scans your corporate environment to identify and manage connected devices, even those not protected by or supported by SentinelOne. Use this command to enable Ranger on Agents that match the filter. The Agent adds "Scanner" to its functionality.<BR>If the given Agent cannot support Ranger, or if Ranger is already enabled, this command does nothing.<BR>Ranger requires a special license. Consult with your SentinelOne SE.
- **`sentinelone-cli agents fetch-firewall-logs`** - Get Firewall Control events in the local log file, written in clear text, for Firewall Control events of an endpoint with Firewall Control enabled. Enable the logs for Agents that match the filter. <BR>When Firewall Logging is enabled, you can choose if blocked traffic events go only to a local log on the endpoint (reportMgmt: false, reportLog: true), or also to Console > Activity (reportMgmt: true).<BR>Allowed traffic is not logged. <BR>Each Agent with Firewall Control Event Logging enabled keeps five log files, for a total of 100 MB maximum. The logs cycle older lines to maintain the size threshold. <BR>On Windows endpoints, the Firewall Control logs are in C:\ProgramData\Sentinel\logs\. Search for log files with "visible" in the filename.<BR>On macOS, run: sudo sentinelctl log.<BR>On Linux, run: sudo /opt/sentinelone/bin/sentinelctl log generate /output_path.<BR>Make sure the Group and Site of the Agent has Firewall Control enabled. Firewall Control requires a Control SKU.
- **`sentinelone-cli agents fetch-firewall-rules`** - Firewall Control is disabled at the Global level. When it is first enabled, all Sites and Groups inherit the Firewall Control policy from the Global policy. Agents have Firewall Control disabled, until they connect to a Site or Group with an enabled Firewall Control policy. <BR>After Agents get Firewall Control, if you add or change a Firewall rule, you can use this command to make sure all Agents fetch the rules, (though Agents usually update their policies every few seconds). Use the filter parameter to set which Agents will fetch the rules, if you do not want all of them to attempt it.<BR>Firewall Control requires a Control SKU.
- **`sentinelone-cli agents fetch-logs`** - Get the Agent and Endpoint logs from Agents that match the filter. <BR>The Agent logs are encrypted and only Support can read them. <BR>The Endpoint logs, for operations on the computers, laptops, or servers that have the Agent installed, are readable. The Endpoint logs are available for Windows endpoints only and require Agent version 3.6 or later. After you run this command, download the fetched logs. You can download the logs from the Console GUI or collect them. <BR>On Windows: C:\ProgramData\Sentinel\logs.<BR>On macOS: Run sudo sentinelctl logreport and get the log files on the desktop.<BR>On Linux: Run sudo /opt/sentinelone/bin/sentinelctl log generate.
- **`sentinelone-cli agents get`** - Get the Agents, and their data, that match the filter. This command gives the Agent ID, which you can use in other commands. <BR>To save the list and data to a CSV file, use "export/agents".
- **`sentinelone-cli agents get-application`** - Get the installed applications for a specific Agent. <BR>To get the Agent ID, run "agents".
- **`sentinelone-cli agents get-installed-apps-for`** - Application Risk Management is an EA feature. Contact your partner or SentinelOne SE to learn how to join the EA program.<BR> If you have this feature, you can use this command to have all Agents update the data of the applications that are installed on the endpoint. Change the filter parameter values to send this command to matching Agents only. The updated data of installed applications shows on the Console.<BR>Some filter fields are required. <BR>Best practice: Enter all fields in the body. Click in the Body sample to get a copy of the fields in the body form.
- **`sentinelone-cli agents get-passphrase`** - Show the passphrase for the Agents that match the filter. This is an important command. You need the passphrase for most SentinelCtl commands and for different API commands.
- **`sentinelone-cli agents initiate-scan`** - Use this command to run a Full Disk Scan on Agents that match the filter. <BR>Full Disk Scan finds dormant suspicious activity, threats, and compliance violations, that are then mitigated according to the policy. It scans the local file system.<BR>Full Disk Scan does not inspect drives that require user credentials (such as network drives) or external drives. <BR>Full Disk Scan does not work on hashes. It does not check each file against the blacklist. <BR>If the Static AI determines a file is suspicious, the Agent calculates its hash and sees if the hash is in the blacklist. If a file is executed, all aspects of the process are inspected, including hash-based analysis and blacklist checks. Full Disk Scan can run when the endpoint is offline, but when it is connected to the Management, it can use the most updated Cloud data to improve detection.
- **`sentinelone-cli agents mark-as-uptodate`** - The value of the Agent version as "up-to-date" is a useful filter for many actions. There are scenarios where the Management does not recognize a version as latest. <BR>For example, if Agents that were sent a new version with the update-software command did not yet report to their Management. <BR>You can manually mark these Agents as up-to-date. <BR>This command is not available to users with the SOC role.
- **`sentinelone-cli agents move-between-sites`** - This command requires Account or Global level access. <BR>Agents are assigned to a Site when they are first installed with a Site Token. If you have the required access level, a role with permissions (the SOC role does not allow this action), and permission for both Sites, you can move Agents from one Site to a different Site. Agents will be moved to the best matching dynamic group, or to the Default group if no dynamic group matches.
- **`sentinelone-cli agents move-to-console`** - You can move Agents between Management Consoles. This command moves Agents to a target Console, Account, and Site, given the Console URL and Site token. <BR>You must have Global permissions for the source Console and access to the Site token of the target Site. <BR>Resolve all threats on the Agents to move before you run this command. <BR>If the Agents have local configurations, the configurations are maintained. <BR>If the new Management has different blacklists, exclusions, and other assets, these are applied the next time the Agent communicates with the Management. <BR>This command works on these Agent versions: Windows 3.0 and later, macOS 3.0 and later, Linux 3.4 and later. <BR>An Agent tries to connect to the new Management Console for 3 minutes. If the Agent cannot connect (has unresolved threats or other requirements are not met), it stays in the original Management Console. <BR>To get the Site token, run the "sites" command (see Sites list) and take the "registrationToken" value.
- **`sentinelone-cli agents processes`** - [OBSOLETE] Returns empty array. To get processes of an Agent, see Applications.
- **`sentinelone-cli agents randomize-uuid`** - IMPORTANT: This action will assign a new UUID to Agents that match the filter. <BR>Run it only when instructed to do so by SentinelOne Support. <BR>If you clone the Agent on a VM or VDI without the /VDI switch, you might need to run this command. It is best to ask for Support assistance. Historical threat and Deep Visibility data will be kept in the Management, but that data will be disassociated from the Agent.
- **`sentinelone-cli agents reject-uninstall`** - Reject uninstall requests for all Agents that match the filter. To learn more about Uninstall Requests, see "Approve Uninstall".
- **`sentinelone-cli agents reset-local-config`** - SentinelCtl is the CLI for Agents. It runs commands directly on one Agent at a time. You can use this command to clear the SentinelCtl changes from all Agents that match the filter. Specific SentinelCtl settings are not cleared: <BR>On Windows: proxy address and Management token.<BR>On macOC: Management server address and server site key.
- **`sentinelone-cli agents restart`** - Use this command to restart endpoints that have an Agent installed and that fit the filter. We recommend that you use the "broadcast" command to send a message to users of endpoints before you restart their computers.
- **`sentinelone-cli agents set-external-id`** - You can add a Customer Identifier (a string) to identify each endpoint or to tag sets of endpoints. The string shows in the Endpoint Details of the Management Console. For example, you can tag endpoints based on their state, installed applications, or endpoint status. The identifier is set on all Agents that match the filter.
- **`sentinelone-cli agents set-persistent-configuration-overrides`** - This command requires Global permissions or Support.<BR>The configuration of an Agent can be changed in different ways, such as through  Policy settings, Policy Override, SentinelCtl, and changes to the LocalConfig.json file. <BR>For Windows, Policy Override overwrites policy settings, and local changes (to the file and from this command) overwrite Policy Override from the Console or with policy updates from the API. <BR>For macOS, the Policy Override has the highest priority. If you run this command and then update a Group policy that affects both Windows and macOS endpoints, the settings of this command are applied to the Windows endpoints. But the macOS endpoints will apply the settings of the policy, for settings that are duplicated in both the policy and this command.<BR>When you use this command, enter the filter values to set which Agents get the change. Then use the data parameter to set the actual changes. Get the JSON settings for data from the Agent Configuration or see the Knowledge Base: https://support.sentinelone.com/hc/en-us/articles/360022158673-sentinelctl
- **`sentinelone-cli agents shutdown`** - You can shut down endpoints remotely for performance, maintenance, or security. <BR>This command shuts down all endpoints that match the filter. Best Practice:  If an endpoint is infected, we recommend the "disconnect" command and not the "shutdown" command. The disconnect command secures the environment from infection while you analyze the cause and best response.<BR>If the endpoint is offline, the shutdown command is not available.
- **`sentinelone-cli agents start-remote-profiling`** - Use this command to start remote profiling on Agents that match the filter. <BR>Remote profiling lets you collect runtime diagnostic information for Agents on containers. <BR>If the command returns "Insufficient permissions", make sure you have permissions for the Account, Site, or Group and a role that allows Start Remote Profiling (Admin or IT).
- **`sentinelone-cli agents start-remote-shell`** - Remote shell is an opened websocket between the browser and the Agent, with a proprietary communication protocol that requires an unreasonable effort to run from the API. We recommend that you not use this call.<BR><BR> If you do want to use this API, you must have permission through your user role (not IT or Viewer), specific Remote Shell permissions, 2FA enabled on the username with a valid code in the twoFaCode parameter, valid code in the twoFaCode parameter, and permissions for the Account, Site, or Group on whose policy Remote Shell is enabled. To make sure you have permission to start Remote Shell, use the "can-start-remote-shell" command. Best practice: Use the UUID filter to run Remote Shell on a specific endpoint. To get the UUID, run the "agents" command. <BR>In the body of this command, the data parameter set is mandatory. <BR>Remote Shell requires a Control SKU.
- **`sentinelone-cli agents stop-remote-profiling`** - Use this command to stop remote profiling on Agents that match the filter. <BR>If the command returns "Insufficient permissions", make sure you have permissions for the Account, Site, or Group and a role that allows Stop Remote Profiling (Admin or IT).
- **`sentinelone-cli agents terminate-remote-shell`** - Remote Shell is a powerful, full shell for Windows, macOS, and Linux. It is best practice to terminate Remote Shell sessions when they are not in use. A Remote Shell session terminates when the user closes the session, the session times out, or the session is idle longer than the idle-timeout. <BR>Use this command terminate a session immediately.
- **`sentinelone-cli agents uninstall`** - Use this command to uninstall Agents that match the filter. For Windows and macOS, make sure that all remnants of the Agent are removed: reboot the endpoints after uninstall. Use the "restart" command.
- **`sentinelone-cli agents update-software`** - Use this command to update the Agent version on endpoints that have the Agent installed and that match the filter. For a cloud-based Management, SentinelOne updates your Management Console with the latest Agent versions. For On-Prem environments, or if you need a package that is not in your Management Console, request files from SentinelOne Support. <BR>IMPORTANT: These parameters are required:<br>packageType - example: "packageType": "AgentAndRanger",osType - example: "osType": "windows",fileName - example: "fileName": "SentinelInstaller-x86_windows_32bit_v4_6_12_241.exe"<BR>Best Practice:  Upgrade your SentinelOne Agents by group or OS. Note about macOS endpoints: It is important that you upgrade the Agent before the endpoint operating system is upgraded to a version that the Agent does not support. More best practices: read the Release Notes, review the system requirements, and if you decide to not upgrade Agents yet, review the Agent Lifecycle. Make sure your deployment is in the supportable bounds.

### application-inventory

application-inventory operations

- **`sentinelone-cli application-inventory`** - [DEPRECATED] Retrieve application inventory grouped by Name, Publisher.

### application-inventory-counts

application-inventory-counts operations

- **`sentinelone-cli application-inventory-counts`** - [DEPRECATED] Application inventory counters.

### applications

applications operations


### cloud-detection

cloud-detection operations

- **`sentinelone-cli cloud-detection activate-rules`** - Activate Custom Detection Rules based on a filter.
- **`sentinelone-cli cloud-detection create-rule`** - Create a Custom Detection Rule for a scope specified by ID. To get the ID, run "accounts", "sites", "groups", or set "tenant" to "true" for Global.
- **`sentinelone-cli cloud-detection delete-rules`** - Deletes Custom Detection Rules that match a filter.
- **`sentinelone-cli cloud-detection disable-rules`** - Disable Custom Detection Rules based on a filter.
- **`sentinelone-cli cloud-detection get-alerts`** - Get a list of alerts for a given scope
- **`sentinelone-cli cloud-detection get-rules`** - Get a list of Custom Detection Rules for a given scope. <br>Note:  You can create and see rules only for your highest available scope. For example, if your username has an access level of scope Account, you cannot see rules created for the Global scope or rules created for a specific Site.
- **`sentinelone-cli cloud-detection update-alert-analyst-verdict`** - Change the verdict of an alert
- **`sentinelone-cli cloud-detection update-rule`** - Change a Custom Detection rule. <br>This command requires the rule ID. (See Get Rules).
- **`sentinelone-cli cloud-detection updated-threat-incident`** - Update the incident details of an alert.

### config-override

config-override operations

- **`sentinelone-cli config-override create`** - Override the configuration of Agents that match the filter. Best practice:  Run "support-actions/config" to get the complete syntax. This command requires a Global user or Support.
- **`sentinelone-cli config-override delete`** - Delete overrides value. To get the required IDs, run "config-override".
- **`sentinelone-cli config-override delete-configoverride`** - Delete an override value. To get the required ID, run "config-override".
- **`sentinelone-cli config-override get`** - There are different ways to override the configuration of an Agent, and the priority of changes depends on the endpoint OS and the version of the installed Agent. Use this command to see the configuration values that are changed for each Agent that matches the filter.
- **`sentinelone-cli config-override update`** - Use this command to change the value of one configuration value. To get the required ID, run "config-override".

### device-control

device-control operations

- **`sentinelone-cli device-control copy-rules`** - You can copy a set of Device Control rules to use in other Accounts, Sites, or Groups. Copy the rules from a source Group, Site, or Account to target Groups, Sites, or Accounts. <br>Define the rules to copy with the filters. To get the values for devices, run "unscoped". To get Account IDs, run "accounts". To get Site IDs, run "sites". <br>Device Control requires Control SKU. Linux Agents do not support Device Control.
- **`sentinelone-cli device-control create-rule`** - Use this command to create a new Device Control rule. These rules allow or block devices, based on device identifiers. Rules apply to a scope: Global (tenant), Account, Site, or Group. To learn details of the fields, see https://support.sentinelone.com/hc/en-us/articles/360023338494. <br>Recommended: Before you begin, see Device Control Known Limitations: https://support.sentinelone.com/hc/en-us/articles/360021104114.<br>Device Control requires Control SKU. Linux Agents do not support Device Control.
- **`sentinelone-cli device-control delete-rules`** - Delete Device Control rules that match the filter.
- **`sentinelone-cli device-control enable-disable-rules`** - It is best practice to disable a rule rather than delete it. Use this command to change the status of a rule between Enabled and Disabled. <br>Note: On Windows, if a USB device is already connected to an endpoint, new rules and rule changes do not affect it. USB rules will apply the next time the device connects to the endpoint. For Windows Bluetooth rules, the device and endpoint must be paired after the SentinelOne Agent that supports Bluetooth is installed or upgraded. If the endpoint and device were already paired before the Agent supported bluetooth, reboot the endpoint to activate the rule, or re-pair the endpoint and device.<br>On macOS, changes apply to devices that are already connected to an endpoint.
- **`sentinelone-cli device-control export-rules`** - Export Device Control rules to a CSV file.
- **`sentinelone-cli device-control get-configuration`** - Get Device Control configuration for a given scope. You can enter a Group ID, Site ID, Account ID, or "tenant = true". If you select tenant, the response shows your Global Device Control configuration for all Windows and macOS endpoints.
Device Control requires Control SKU. It is not supported on Linux.
- **`sentinelone-cli device-control get-device-rules`** - Get the Device Control rules of a specified Account, Site, Group or Global (tenant) that match the filter.
- **`sentinelone-cli device-control get-events`** - Get the data of Device Control events on Windows and macOS endpoints with Device Control-enabled Agents that match the filter.
Device Control requires Control SKU. Linux Agents do not support Device Control.
- **`sentinelone-cli device-control import-rules`** - Import Device Control rules from a CSV file. In the file field, enter the pathname of the output file of an Export Rules execution - "device-control/export".
- **`sentinelone-cli device-control move-rules`** - You can move a set of Device Control rules to other Accounts, Sites, or Groups. This command removes the rule from the source and copies to the targets. 
Define the rules to copy with the filters. To get the values for devices, run "unscoped". To get Account IDs, run "accounts". To get Site IDs, run "sites".
Device Control requires Control SKU. Linux Agents do not support Device Control.
- **`sentinelone-cli device-control reorder-rules`** - When an external device connects to an endpoint, the SentinelOne Agent looks at the rules based on their order in the Device Control policy, from the top to the bottom. When the Agent finds a rule that matches the device identifiers of a connected device, that rule is applied. The Agent does not continue to the lower rules in the list.
Use this command to change the order of rules for a specific scope. 
Device Control requires Control SKU. Linux Agents do not support Device Control.
- **`sentinelone-cli device-control update-configuration`** - Use this command to change the Device Control configuration. Enter a Group ID, Site ID, Account ID, or "tenant = true". If you select only tenant, and the other scopes are empty, the change is applied to the Global policy.
Device Control requires Control SKU. It is not supported on Linux.
- **`sentinelone-cli device-control update-device-rule`** - Change the Device Control rule that matches the filter. To learn more about the fields, see https://support.sentinelone.com/hc/en-us/articles/360023338494.

### dv

dv operations

- **`sentinelone-cli dv cancel-running-query`** - Stop a Deep Visibility Query by queryId. The body is {"queryID":"string_ID"}. Get the ID of the query from "init-query". See "Create Query and get QueryId".<br> Deep Visibility requires Complete SKU.
- **`sentinelone-cli dv create-query-and-get-query-id`** - Start a Deep Visibility Query and get the queryId. You can use the queryId for other commands, such as Get Events and Get Query Status. For complete query syntax, see Query Syntax in the Knowledge Base (support.sentinelone.com) or the Console Help. SentinelOne Deep Visibility extends the ActiveEDR capabilities, with full visibility into endpoint data and threat hunting.  Its kernel-based monitoring searches across endpoints for all indicators of compromise (IOC). <br>Rate limit: 1 call per minute for each different user token. <br>Deep Visibility requires Complete SKU.
- **`sentinelone-cli dv download-source-process-file`** - Download the source process file associated with a Deep Visibility event.
- **`sentinelone-cli dv get-events`** - Get all Deep Visibility events from a queryId. You can use this command to send a sub-query, a new query to run on these events. Get the ID from "init-query". See "Create Query and get QueryId". <br>For complete documentation, see Query Syntax in the Knowledge Base (support.sentinelone.com) or the Console Help.
- **`sentinelone-cli dv get-events-by-type`** - Get Deep Visibility results from the query that matches the given event type. Valid values for Event Type:<br> Process Exit<br> Process Modification<br> Process Creation<br> Duplicate Process Handle<br> Duplicate Thread Handle<br> Open Remote Process Handle<br> Remote Thread Creation<br> Remote Process Termination<br> Command Script<br> IP Connect<br> IP Listen<br> File Modification<br> File Creation<br> File Scan<br> File Deletion<br> File Rename<br> Pre Execution Detection<br> Login<br> Logout<br> GET<br> OPTIONS<br> POST<br> PUT<br> DELETE<br> CONNECT<br> HEAD<br> DNS Resolved<br> DNS Unresolved<br> Task Register<br> Task Update<br> Task Start<br> Task Trigger<br> Task Delete<br> Registry Key Create<br> Registry Key Rename<br> Registry Key Delete<br> Registry Key Export<br> Registry Key Security Changed<br> Registry Key Import<br> Registry Value Modified<br> Registry Value Create<br> Registry Value Delete<br> Behavioral Indicators<br> Module Load
- **`sentinelone-cli dv get-process-state`** - Get details of all Deep Visibility processes from a queryId.To get the ID from "init-query". See "Create Query and get QueryId".
- **`sentinelone-cli dv get-query-status`** - Get that status of a Deep Visibility Query. When the status is FINISHED, you can get the results with the queryId in "Get Events".<br>Deep Visibility requires Complete SKU.<br>Rate limit: 1 call per second for each different user token.

### exclusions

exclusions operations

- **`sentinelone-cli exclusions create`** - Create Exclusions to make your Agents suppress alerts and mitigation for items that you consider to be benign or which you require for interoperability.<br>IMPORTANT! Every Exclusion is a possible security hole. Do not create Exclusions unless you are sure this hash, path, certificate signer, file type, or browser is always benign.<br>Of course, if you can make the Exclusion by its hash or path, that is much more secure than excluding all detections of a specific signer, file type, or browser. We do not recommend the last types for Exclusions on production endpoints. These Exclusions might be helpful in a lab or pentester group. When you create an Exclusion, make sure you set the filter to the smallest possible scope. For example, if you can exclude security for this item on a group, do not enter values for siteIds or accountIds.<br>We recommend that you read "Not Recommended Exclusions: https://support.sentinelone.com/hc/en-us/articles/360007532894<br> and Best Practices for Exclusions: https://support.sentinelone.com/hc/en-us/articles/360008709014
- **`sentinelone-cli exclusions delete`** - Every Exclusion opens a possible security hole. If you decide that an Exclusion (or multiple Exclusions) is not required, use this command to delete it. To get the ID of the Exclusion to delete, run the "exclusions" command.
- **`sentinelone-cli exclusions get`** - Get a list of all the Exclusions that match the filter. <br>Note: To see items from the  Global Exclusion scope, make sure "tenant" is "true" and no other scope ID is given.
- **`sentinelone-cli exclusions update`** - Change the properties of an Exclusion through the data fields. To get the original data, run "exclusions" with a filter to give the item you want.
- **`sentinelone-cli exclusions validate-item`** - Check if an exclusion is on the list of SentinelOne items that are "Not Allowed" or "Not Recommended". This API returns one of the following statuses:<br> * Not Recommended: This item is not recommended by SentinelOne because it decreases security. For example, If you accidentally exclude a path that is too broad, malware can enter your environment.<br>* Not Allowed: This exclusion can harm the product and lead to unexpected functionality. From version North Pole SP3 you are prevented from creating Not Allowed exclusions.* None: This item is not on the list of SentinelOne items that are "Not Allowed" or "Not Recommended".

### filters

filters operations

- **`sentinelone-cli filters delete`** - Delete a saved filter.
- **`sentinelone-cli filters delete-deep-visibility`** - Delete a saved Deep Visibility query.
- **`sentinelone-cli filters get`** - Get the list of saved filters. See Save Filter. The response includes the ID of the filter, which you can use in other commands.
- **`sentinelone-cli filters get-deep-visibility`** - Get saved Deep Visibility queries with full data. See Save Deep Visibility Filters.The response includes the ID of the filter, which you can use in other commands.
- **`sentinelone-cli filters save`** - Save a new filter to get a list of matching endpoints. When you save a filter, you can run actions on the Agents as a set of objects or create a dynamic group (automatically adds new Agents that match the filter and drops Agents if they change to not match).
For example, you can save a filter with {"data":{"filterFields":{"infected":true}}} to run kill and quarantine commands on all the Agents at once, or to create a group that holds currently infected endpoints. Best Practice: Set a scope for the new Saved Filter. Run "accounts", "sites", or "groups" to get the IDs for the scope.
- **`sentinelone-cli filters save-deep-visibility`** - Save a Deep Visibility query with data as a filter, to get notifications of specific events sent to named recipients on a given frequency. The recipients must be Console users with permissions on the scope of the query. Notifications are sent through email: you must have an SMTP server configured in the SentinelOne solution (/settings/smtp see Set SMTP Settings).
Deep Visibility requires a Complete SKU.
- **`sentinelone-cli filters update`** - Update an existing filter
- **`sentinelone-cli filters update-deep-visibility`** - Change a saved Deep Visibility filter. To get the ID and fields to change, run Get Deep Visibility Filters.

### firewall-control

firewall-control operations

- **`sentinelone-cli firewall-control add-rule-tags`** - Create a Firewall Rule tag. <br>Create tags to represent Firewall policies - a set of rules in a specific order. After you create the tag, add rules to it.<br>Notes:<br>* Tags apply to a scope and cannot be linked to rules from different scopes.<br>* Tags must be 2 to 256 characters.
- **`sentinelone-cli firewall-control copy-rules`** - Copy a set of rules to other scopes. <br>In the filter of the body, enter the properties to define the source. In the data field of the body, define the targets by ID. To get a scope ID, run 'accounts', 'sites', or 'groups'.
- **`sentinelone-cli firewall-control create-firewall-rule`** - Create a Firewall Control rule for a scope specified by ID (run "accounts", "sites", "groups", or set "tenant" to "true") and specific OS, to allow or block network traffic to matching endpoints.<br>You can create one clean-up rule, with the Action of Allow or Block and with no other parameters defined explicitly. Make this the default rule at the end of your rule list. Traffic that does not match other rules first will match this rule. If you do not have a clean-up rule to match all traffic, the default Firewall Control behavior is to allow traffic that is not explicitly blocked.<br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control create-firewall-rule-by-category`** - Create a Firewall Control rule for a scope specified by ID (run "accounts", "sites", "groups", or set "tenant" to "true") and specific OS, to allow or block network traffic to matching endpoints.<br>You can create one clean-up rule, with the Action of Allow or Block and with no other parameters defined explicitly. Make this the default rule at the end of your rule list. Traffic that does not match other rules first will match this rule. If you do not have a clean-up rule to match all traffic, the default Firewall Control behavior is to allow traffic that is not explicitly blocked.<br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control delete-rules`** - Delete Firewall Control rules that match the filter.
- **`sentinelone-cli firewall-control delete-rules-by-category`** - Delete Firewall Control rules that match the filter.
- **`sentinelone-cli firewall-control enable-disable-rules`** - Change the status of a set of Firewall Control rules that match the filter to "Enabled" or "Disabled". In one request, you can set one status or the other.
- **`sentinelone-cli firewall-control export-rules`** - Export Firewall Control rules that match the filter to a JSON file from a scope specified by ID (run "accounts", "sites", "groups", or leave the scope empty and set "tenant" to "true") and import them to another scope (with the "import" command. <br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control get-configuration`** - Get the Firewall Control configuration for a given scope. <br>To get the ID of a scope, run "accounts", "sites", or "groups". To get all scopes in your deployment, leave the filtersempty and set "tenant" to "true". The response shows if Firewall Control is enabled for the scope, if Location Awareness is enabled, the higher scope from which this scope inherited the configuration, and whether a lower scope inherits this configuration.<br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control get-firewall-rules`** - Get the Firewall Control rules for a scope specified by ID (run "accounts", "sites, "groups", or set "tenant" to "true") that match the filter. <br>The response will be quite long because it includes all the rule properties, thus at least one of these filters is required: action, status, osType, name, or scope ID.
- **`sentinelone-cli firewall-control get-firewall-rules-by-category`** - Get the Firewall Control rules for a scope specified by ID (run "accounts", "sites, "groups", or set "tenant" to "true") that match the filter. <br>The response will be quite long because it includes all the rule properties, thus at least one of these filters is required: action, status, osType, name, or scope ID.
- **`sentinelone-cli firewall-control get-protocols`** - Get a list of protocols that can be used in Firewall Control rules.
- **`sentinelone-cli firewall-control get-tag-firewall-rules`** - Get all Firewall rules linked to tag, regardless of inheritance mode. <br>To get the ID of a tag, run the firewall-control API (see Get Firewall Rules) and see tagIDs in the response.
- **`sentinelone-cli firewall-control import-rules`** - Import Firewall Control rules from an exported JSON file to scopes specified by ID (run "accounts", "sites", "groups", or leave the scope empty and set "tenant" to "true").<br>Firewall Control requires Control SKU, in the target and in the source.
- **`sentinelone-cli firewall-control move-rules`** - Remove Firewall Rules, defined with the ID of the rules (run 'firewall-control'), from scopes specified by ID (run 'accounts', 'sites', or 'groups') and add the rules to the scope IDs in the data field.<br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control remove-rule-tags`** - Remove firewall tags from rules matching the filter.<br>Tags represent Firewall policies - a set of rules in a specific order. When you remove a rule with a tag, all scopes that subscribe to the tag get the change.
- **`sentinelone-cli firewall-control reorder-rules`** - Change the order of rules for a scope  specified by ID (run "accounts", "sites", or "groups"). <br>The Agent looks at the rules based on their order in the Firewall Control policy, from the top to the bottom. First it goes through the Group rules, then the Site rules, then the Account rules, then the Global rules. When the Agent finds a rule that matches the parameters of the traffic, that rule is applied. The Agent does not continue to the lower rules in the list. Thus, the scope and the order of the rules is important.<br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control set-location`** - Set location attributes for a Location Aware Firewall Control rule. These rules are applied by Agents only if the network parameters of the endpoint match the properties of the location definition. To get a Location ID, run "locations". <br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control update-configuration`** - Change the Firewall Control configuration for a given scope.  <br>To get the ID of a scope, run "accounts", "sites", or "groups". To change the Global configuration, leave the filtersempty and set "tenant" to "true". In the Body, you can set if Firewall Control is enabled for the scope, if Location Awareness is enabled, the higher scope from which this scope inherits the configuration ("Global" or a scope ID), whether the lower scopes inherit this configuration, and whether blocked actions are reported.<br>Firewall Control requires Control SKU.
- **`sentinelone-cli firewall-control update-firewall-rule-by-category`** - Change a Firewall Control rule. <br>This command requires the rule ID, which you can get from "firewall-control" (see Get Firewall Rules) or "firewall-control/unscoped" (see Get Unscoped Rules).

### groups

groups operations

- **`sentinelone-cli groups create`** - Create a new group. You must create the Group in a Site (run "sites" to get the Site ID) for which you have permissions. If you create a dynamic Group, you must have the ID of a filter saved in the Site (run "filters?siteIds=<id from sites>").
- **`sentinelone-cli groups delete`** - Delete a Group given by the required Group ID (run "groups"). If there are Agents in the Group, and the Group is dynamic, the next dynamic Groups will collect matching Agents, and unmatched Agents will go to the Default Group. If this is a static Group with Agents, all the Agents will go to the Default Group. (Agents always go to matching dynamic Groups. If a static Group holds Agents, there are no matching dynamic Groups.)
- **`sentinelone-cli groups get`** - Get data of groups that match the filter. Best practice: use as narrow a filter as you can. The data can be quite long for many groups. The response returns the ID of each group, which you can use in other commands.
- **`sentinelone-cli groups get-by-id`** - Get data of a given Group. To get a Group ID, run "groups". This command responds with the ID of the Site of the Group, Group name, type (dynamic or static), and similar data. Your username must permissions for the Site.
- **`sentinelone-cli groups update`** - Change properties of a Group specified by its ID (run "groups"). The body of the request holds all the properties of a Group. You must have access permissions on the Site. Note: iocAttributes refers to Deep Visibility. If you do not have a Complete SKU, you can remove this set.
- **`sentinelone-cli groups update-ranks`** - An Agent can belong to only one Group. If the Agent matches multiple Dynamic Groups, it goes to the Group with the highest rank. The "rank" parameter has a minimum of "1". The lower the integer, the higher priority it has to collect Agents. Make sure the IDs of the groups in this command are for Dynamic groups.

### hashes

hashes operations


### installed-applications

installed-applications operations

- **`sentinelone-cli installed-applications get`** - Get the applications, and their data (such as risk level), installed on endpoints with Application Risk-enabled Agents that match the filter. SentinelOne Application Risk lets you monitor applications installed on endpoints. Applications not updated with the latest patches are vulnerable to exploits. With SentinelOne Application Risk you can see all applications to be patched, on all endpoints or on a specific endpoint. The Agent takes a snapshot of the endpoint application data and checks for vulnerabilities in the SentinelOne Cloud. When the Agent detects a change to the application data, it sends a diff to the Management.<br>Application Risk requires Complete SKU. This feature is in EA. To join the EA program, contact your SentinelOne Sales Rep.
- **`sentinelone-cli installed-applications get-cves`** - Get known CVEs for applications that are installed on endpoints with Application Risk-enabled Agents. <br>Application Risk requires Complete SKU. This feature is in EA. To join the EA program, contact your SentinelOne Sales Rep.

### last-activity-as-syslog

last-activity-as-syslog operations

- **`sentinelone-cli last-activity-as-syslog`** - Get the Syslog message that corresponds to the last activity that matches the filter. <br>If Syslog messages that you expected to see are not in the response, make sure you selected "Syslog" for the activity type. <br>To see your Syslog settings, run: "settings/notifications". <br>To change the settings, run: "settings/notifications" with the changes in the body of the request.

### locations

locations operations

- **`sentinelone-cli locations create`** - Create a location that defines parameters of Agents in a scope filter. Parameters include: <br>* ipAddresses - The Agent compares the endpoint active IPv4 or IPv6 addresses to the IP addresses, ranges, and CIDRs defined for the location. <br>* dnsServers - The Agent compares the configured DNS servers of the endpoint to the DNS servers defined for the location.<br>* dnsLookup - The Agent resolves the FQDN of the endpoint to IPv4 or IPv6 addresses and compares them to the addresses configured in the location setting.<br>* networkInterfaces - The Agent determines if the endpoint is connected to the network over a wireless connection. If one connected interface is wireless, the endpoint is considered wireless.<br>* serverConnectivity - The Agent reports if it is connected to its Management.<br>* registryKeys - The Agent compares the endpoint registry keys in HKEY_LOCAL_MACHINE\SOFTWARE with the registry key of the location definition. <br>When you set a location parameter, also set the operator to ALL, NONE, or at least 1. <br>The serverConnectivity parameter takes "enabled" (true or false) and "value" (connected or disconnected). <br>The networkInterfaces parameter takes "enabled" (true or false) and "value" (wired or wireless).
- **`sentinelone-cli locations delete`** - Delete location definitions of a given location. To get location IDs, run "locations".
- **`sentinelone-cli locations get`** - Get the locations of Agents in a given scope that match the filter.  Agent locations are based on endpoint network parameters (IP, DNS, NIC, Registry Key, or SentinelOne connection set for all true, at least one true, or none true and applied to a Site, Account, or Global). Agents detect their location settings and apply Firewall Control rules that have Location Aware parameters that match the Agent location. Agents can be in multiple locations at the same time. If an Agent that supports Locations does not detect that it is in a defined location, it uses the Firewall rules assigned to the Fallback location. <br>Use this command with a filter for "hasFirewallRules" to find Locations that do not have matching Firewall Control rules. The response to this request includes the ID of the location, which you can use in other commands.<br>Firewall Control and Location Awareness require Control SKU.
- **`sentinelone-cli locations update`** - Change the parameter values of a location definition. See Create Location.

### ranger

ranger operations

- **`sentinelone-cli ranger add-cred-details`** - Add cred details to a cred group.
- **`sentinelone-cli ranger add-new-deploy-command-for-device-from-agent-from-task-infra`** - Creates a new agent deploy command for devices. Used for communication between API service and Task Infra service
- **`sentinelone-cli ranger change-device-review`** - Change the review state of one device.
- **`sentinelone-cli ranger change-device-review-in-bulk`** - Change the review state of more than one device.
- **`sentinelone-cli ranger change-device-tags`** - Change the device tags.
- **`sentinelone-cli ranger create-cred-group`** - Create a new Cred Group.
- **`sentinelone-cli ranger delete-cred-group`** - Delete cred group value.
- **`sentinelone-cli ranger delete-cred-group-detail`** - Delete cred group detail value.
- **`sentinelone-cli ranger export-data`** - Export Ranger data to csv. You can set filters to get only relevant data. The response sends the csv data as text.
- **`sentinelone-cli ranger get-cred-group-details`** - Get the data for each row in the Cred Groups details table.
- **`sentinelone-cli ranger get-cred-groups`** - Get the data for each row in the Cred Groups table.
- **`sentinelone-cli ranger get-gateways`** - Get the gateways in your deployment that match the filter from a Ranger scan. 
Ranger requires a Ranger license.
- **`sentinelone-cli ranger get-settings`** - Ranger gives full visibility of all devices connected to your network. Ranger scans your corporate environment to identify and manage connected devices, even those not protected by or supported by SentinelOne. Ranger identifies devices as:<br>* Secured - End-user computer or laptop, or server, with a SentinelOne Agent.<br>* Unsecured - Endpoint of supported hardware and OS, without an Agent.<br>* Unsupported - Hardware or software that are not compatible with the SentinelOne Agent.<br>* Unknown - Ranger cannot determine if the device is Unsecured or Unsupported.<br>When you install Windows Agents with Ranger, the Agents can become scanners. Selected scanners from networks that you enable for scanning find connected devices with passive and active scan techniques. The scanners send the collected data to Ranger on the Management. Ranger then runs fingerprinting to identify and classify unique devices and to update the Device Inventory Table in the Management Console. With port scanning, it is important that you understand the legal and ethical considerations and that you document a Ranger plan and implementation. See https://support.sentinelone.com/hc/en-us/articles/360041484913 > Legal Considerations and Proper Implementation.<br>Requirements:  Ranger license, Cloud-based Management (not supported for On-Prem), Global user or Account user with scope access to the Account with a Ranger license.<br>Use this command to get the Ranger Settings for the Account of the given ID (run "accounts" to get an Account ID). The Response shows if Ranger is enabled on the Account, the protocols and ports of the scans, and more:<br>* minAgentsInNetworkToScan - To help you determine which networks are corporate, Ranger looks at the number of secured endpoints (Agents) in a network. If there are not enough Agents in a network - set by this parameter value - Ranger considers the network to be non-corporate and will not scan it.<br>* scanOnlyLocalSubnets - If false, Ranger scans remote subnets that do not have online Ranger scanners. This will create network traffic through the corporate firewall (and between different corporate locations), which can impact network performance.<br>* usePeriodicSnapshots - A complete scan includes scanner port scanning and Ranger AI analysis of the scanner data to update the Device Inventory Snapshot. If this setting is true, Ranger runs a new scan on an interval. If snapshotPeriod is shorter, the data is more accurate. If longer, there is better performance.
- **`sentinelone-cli ranger get-table`** - Get the data for each row in the Ranger Device Inventory Table. Best practice: Set filters. Each row is a set of parameters that quickly fills the pagination limits.
- **`sentinelone-cli ranger update-cred-group`** - Update cred group values.
- **`sentinelone-cli ranger update-cred-group-details`** - Update cred group values.
- **`sentinelone-cli ranger update-gateway`** - Change the Ranger scan configuration for a gateway that Ranger discovered
- **`sentinelone-cli ranger update-gateways`** - Change the status of filtered gateways discovered by Ranger. You can set the archived status, whether the network behind the gateway may be scanned by Ranger, and whether Ranger will scan only local networks.
- **`sentinelone-cli ranger update-settings`** - Change the Ranger Settings. Best Practice: Get the current settings before you change them. See: Get Ranger Settings.

### rbac

rbac operations

- **`sentinelone-cli rbac create-new-role`** - Create a new role for Role-Based Access Control (RBAC).
- **`sentinelone-cli rbac delete-role`** - With the ID of a role (see Get All Roles), you can delete a role. If there are users assigned to the role, specify the ID of their new role.
- **`sentinelone-cli rbac get-all-roles`** - See roles assigned to users that match the filter, a basic description of the roles, and the number of users for each role. <br>Role-Based Access Control (RBAC) has predefined roles. (Currently, customized roles are not supported.), This command gives the ID of the role, which you can use in other commands.
- **`sentinelone-cli rbac get-specific-role-definition`** - With the ID of a role (see Get All Roles) you can see the permissions of that role. <br>The definition of a role can change in different scopes and SKUs. For example, an Admin role with the scope access of a Site does not have Ranger permissions, but an IT role with the scope access of an Account with a Ranger license does have permissions on Ranger. <br>The Response shows role permissions to see views in the WebUI and to use Console features.
- **`sentinelone-cli rbac get-template-for-new-role`** - Get the template for a new role.
- **`sentinelone-cli rbac update-role`** - With the ID of a role (see Get All Roles), you can update the permissions of users with this role.

### remote-scripts

remote-scripts operations

- **`sentinelone-cli remote-scripts get-scripts`** - Get the SentinelOne scripts from the Script Library.
- **`sentinelone-cli remote-scripts run`** - Run remote script
- **`sentinelone-cli remote-scripts upload-a-new-script`** - Upload a new script

### report-tasks

report-tasks operations

- **`sentinelone-cli report-tasks create`** - Create a task to generate a report immediately, one time in the future, or on a schedule. Best Practice: Get Report Tasks first, to have a basis for a new task.
- **`sentinelone-cli report-tasks get`** - Get the tasks that were done to generate reports and to schedule future reports. Best Practice: Use a filter. Each task includes many lines of data and can quickly fill the page limit. Use this command to get the ID of a report task to use in other commands.
- **`sentinelone-cli report-tasks update`** - Update the report task of the given ID. To get the task ID, and the data to change, run Get Report Tasks.

### reports

reports operations

- **`sentinelone-cli reports delete`** - Delete the reports that match the filter. To delete a specific report, use its ID (see Get Reports).
- **`sentinelone-cli reports delete-tasks`** - You can schedule a report to be generated on a routine. Use this command to remove a task to generate a report in the future. To get an ID to delete a specific task, see Get Report Tasks.
- **`sentinelone-cli reports download`** - When the Management generates a report, it is uploaded to the Management Console. Use this command to get the report as a PDF or HTML file. To get the ID of the report, see Get Reports.
- **`sentinelone-cli reports get`** - Get the reports that match the filter and the data of the reports. Use this command to get the ID of reports to use in other commands. Other data in the response: schedule, Insight Type, name and ID of the user who created the report, the date range, and more.
- **`sentinelone-cli reports get-insight`** - Get the Insight Report types. These reports show high-level and detailed information on the state of your endpoint security. Reports include statistics, trends, and summaries with easy to read and actionable information about your network. Use this command to see the predefined reports. This command does not give data for specific reports.

### restrictions

restrictions operations

- **`sentinelone-cli restrictions create-blacklist-item`** - Create a blacklist item for a SHA1 hash, for the scopes you enter in the filter fields. You can add the hash to multiple Groups, Sites, Accounts, and to the Global list. <br> IMPORTANT: The type must be "black_hash" - any other value will create an Exclusion rather than a Blacklist item.<br>Users with the IT role do not have permissions to run this.
- **`sentinelone-cli restrictions delete-blacklist-item`** - Agents immediately identify files on the blacklist and block them from executing. Agents identify files on the blacklist before they look at exclusions. If there is a conflict - for example, if a hash is blacklisted from the Cloud Intelligence, and you have an exclusion to run an application that requires this hash - you can delete the hash from the Blacklist. Users with the IT role do not have permissions to run this command.
- **`sentinelone-cli restrictions get-blacklist`** - Get a list of all the items in the Blacklist that match the filter. <br>Note: To see items from the Global Blacklist, make sure "tenant" is "true" and no other scope ID is given.
- **`sentinelone-cli restrictions update-blacklist-item`** - Change the properties of a Blacklist item through the data fields. To get the original data, run "restrictions" with a filter to give the item you want.
- **`sentinelone-cli restrictions validate-blacklist-item`** - Check if a hash is on the list of SentinelOne items that are "Not Allowed" or "Not Recommended". This API returns one of the following statuses:<br> * Not Recommended: This item is not recommended by SentinelOne because it decreases security. <br>* Not Allowed: This item can harm the product and lead to unexpected functionality. From version North Pole SP3 you are prevented from creating Not Allowed blacklist item. * None: This item is not on the list of SentinelOne items that are "Not Allowed" or "Not Recommended".

### rogues

rogues operations

- **`sentinelone-cli rogues export-data`** - Export Rogues data to CSV. You can set filters to get only relevant data. The response sends the CSV data as text.
- **`sentinelone-cli rogues get-settings`** - Rogues gives full visibility of all unsecured devices connected to your network. Rogues scans your corporate environment to identify and manage connected devices, even those not protected by or supported by SentinelOne. Rogues identifies devices as:<BR> * UnSecured - End-user computer or laptop, or server, without a SentinelOne Agent.<BR> When you install Windows Agents with Rogues, the Agents can become scanners. Selected scanners from networks that you enable for scanning find connected devices with passive and active scan techniques. The scanners send the collected data to Rogues on the Management. Rogues then runs fingerprinting to identify and classify unique devices and to update the Device Inventory Table in the Management Console. With port scanning, it is important that you understand the legal and ethical considerations and that you document a Rogues plan and implementation. See Legal Considerations and Proper Implementation in the Console Help.<BR> * minAgentsInNetworkToScan - To help you determine which networks are corporate, Rogues looks at the number of secured endpoints (Agents) in a network. If there are not enough Agents in a network - set by this parameter value - Rogues considers the network to be non-corporate and will not scan it.
- **`sentinelone-cli rogues get-table`** - Get the data for each row in the Rogues Device Inventory Table. <BR>Best practice: Set filters. Each row is a set of parameters that quickly fills the pagination limits.
- **`sentinelone-cli rogues update-settings`** - Change the Rogues Settings. Best Practice: Get the current settings before you change them. See: Get Rogues Settings.

### sentinelone-export

Manage sentinelone export

- **`sentinelone-cli sentinelone-export activities`** - Export the list of activities.
- **`sentinelone-cli sentinelone-export agents`** - Export Agent data to a CSV, for Agents that match the filter. This command exports only 10,000 items (each datum is an item).
- **`sentinelone-cli sentinelone-export events`** - Export threat events in CSV or JSON format.
- **`sentinelone-cli sentinelone-export list-installed-applications`** - Export the list of applications installed on endpoints with Application Risk-enabled Agents and their properties, including the the CVEs for each application that requires a patch. The CSV file is stored on the Management. Application Risk requires Complete SKU. <br>This feature is in EA. To join the EA program, contact your SentinelOne Sales Rep.
- **`sentinelone-cli sentinelone-export threat-timeline`** - Export a threat's timeline.

### sentinelonerss

sentinelonerss operations

- **`sentinelone-cli sentinelonerss`** - Get the SentinelOne RSS feed. In the SentinelOne Management Console, we show the feed contents in the Dashboard.

### settings

settings operations

- **`sentinelone-cli settings clear-pending-emails`** - Clear (discard without sending) pending email notifications for the given Sites (to get the IDs, run "sites") or Accounts ("accounts"). <br>When you set email recipients to get notifications for activities in the system, you can set too many, or in other ways cause issues that demand that the queue be cleared.
- **`sentinelone-cli settings delete-notification-recipient`** - Delete a notification recipient by ID. To get the IDs of recipients, run "recipients" (see Get Notification Recipients).
- **`sentinelone-cli settings get-ad`** - Get the Global Active Directory settings.
- **`sentinelone-cli settings get-ad-fqdns`** - Get the map of Active Directory FQDNs to user roles of the given Sites (use "sites" to get IDs) or Accounts ("accounts").
- **`sentinelone-cli settings get-microsoft`** - [DEPRECATED] Gets the Microsoft settings of the Sites or Accounts.
- **`sentinelone-cli settings get-notification`** - Get the notification settings for the given Sites (to get the IDs, run "settings") or Accounts ("accounts"). <br>The response shows every possible notification and whether it is active and if so, for email or syslog or both. It also shows the ID string for each notification, which can be used in other commands. <br>Note: Each notification also shows "sms" which is deprecated.
- **`sentinelone-cli settings get-notification-recipients`** - Get the emails that are configured to receive notifications.
- **`sentinelone-cli settings get-sms`** - [DEPRECATED] Gets the site's SMS settings.
- **`sentinelone-cli settings get-smtp`** - Get the SMTP server configuration of the given Sites (to get the IDs, run "sites") or Accounts ("accounts"). The SMTP integration is required to send notifications by email.
- **`sentinelone-cli settings get-sso`** - Get the Single Sign-On configuration for the given Sites (to get the IDs, run "sites") or Accounts ("accounts").
- **`sentinelone-cli settings get-syslog`** - Get the configuration of the syslog server integrated with the given Sites (to get the IDs, run "sites") or Accounts ("accounts").
- **`sentinelone-cli settings set-ad`** - Update the Global Active Directory settings.
- **`sentinelone-cli settings set-ad-fqdns`** - Update the Active Directory FQDNs of a Site or Account.
- **`sentinelone-cli settings set-microsoft`** - [DEPRECATED] Update Microsoft settings for the given Sites or Accounts.
- **`sentinelone-cli settings set-notification`** - Change the notifications for the given Sites (to get the IDs, run "settings") or Accounts ("accounts"). Best practice: Get the current settings (see Get Notification Settings) before you run this command.
- **`sentinelone-cli settings set-notification-recipients`** - Set the emails of recipients to get notifications.
- **`sentinelone-cli settings set-sms`** - [DEPRECATED] Set SMS settings.
- **`sentinelone-cli settings set-smtp`** - Change the SMTP server configuration for the given Sites or Accounts. Use this command to integrate a different SMTP server, which is required to send notifications by email.
- **`sentinelone-cli settings set-sso`** - Change the Single Sign-On configuration for the given Sites (to get the IDs, run "sites") or Accounts ("accounts"). <br>The Management supports SAML 2.0 and will integrate with SAML 2.0 compliant SSO providers. <br>SentinelOne Technical Support can help you with issues related to the provider we tested: Okta. To use a different ID provider, see the provider documentation and support. <br>For requirements and best practices of Okta integration, see https://support.sentinelone.com/hc/en-us/articles/360004195714.
- **`sentinelone-cli settings set-syslog`** - Change the configuration of the syslog server of the given Sites (to get the IDs, run "sites") or Accounts ("accounts"). Use this command to send notifications to a different syslog server. Best Practice: Get Syslog Settings before you run this command.
- **`sentinelone-cli settings test-ad`** - Test Active Directory settings.
- **`sentinelone-cli settings test-microsoft`** - [DEPRECATED] Test Microsoft settings.
- **`sentinelone-cli settings test-smtp`** - Test SMTP settings between the Management and the SMTP server. This integration is required if you use email notifications.
- **`sentinelone-cli settings test-sso`** - Test Single Sign-On settings.
- **`sentinelone-cli settings test-syslog`** - Test Syslog settings. The Management tests the connection to the Syslog server.

### singularity-marketplace

singularity-marketplace operations

- **`sentinelone-cli singularity-marketplace delete-marketplace-application`** - Delete application integration from your Marketplace.
- **`sentinelone-cli singularity-marketplace enable-or-disable-application`** - Use this command to enable or disable application integrations that match the filter.
- **`sentinelone-cli singularity-marketplace get-applications-catalog`** - Get the Marketplace Application Catalog.
- **`sentinelone-cli singularity-marketplace get-configuration-fields`** - Get the Catalog Application Configuration Fields.
- **`sentinelone-cli singularity-marketplace get-configuration-fields-for-catalog-application`** - Returns The configuration schema for a requested Application Catalog.
- **`sentinelone-cli singularity-marketplace get-marketplace-applications`** - Get the installed Marketplace applications for a scope specified.
- **`sentinelone-cli singularity-marketplace install-applications`** - Install application from the Application Catalog.
- **`sentinelone-cli singularity-marketplace update-application-configuration`** - Update installed application configuration.

### site-with-admin

site-with-admin operations

- **`sentinelone-cli site-with-admin`** - Create a Site and an Admin role user. This requires an Admin role with a Global scope or Account scope that has permissions over the Account to which the Site will belong. <br>You must have a license for a new Site. <br>In the body of this request, include the policy and user properties.

### sites

sites operations

- **`sentinelone-cli sites create`** - Create a Site. This requires an Admin role with a Global scope or Account scope that has permissions over the Account to which the Site will belong. <br>You must have a license for a new Site. <br>In the body of this request, include the policy.
- **`sentinelone-cli sites create-duplicate`** - [DEPRECATED] Create duplicate site.
- **`sentinelone-cli sites delete`** - Delete the Site of the given ID. To get the ID, run "sites". <br>You must have an Admin role with scope access that includes the Site.
- **`sentinelone-cli sites get`** - Get the Sites that match the filters. <br>The response includes the IDs of Sites, which you can use in other commands.
- **`sentinelone-cli sites get-by-id`** - Get the data of the Site of the ID. To get the ID, run "sites". <br>The response shows the Site expiration date, SKU, licenses (total and active), token, Account name and ID, who and when it was created and changed, and its status.
- **`sentinelone-cli sites update`** - Change the policy and properties of the Site given by ID. <br>To get the ID, run 'sites'.

### system

system operations

- **`sentinelone-cli system cache-status`** - Get an indication of the system's cache health status. <br>This command returns a positive response when the cache server is up and running. <br>This command does not require authentication. <br>Rate limit: 1 call per second for each IP address that communicates with the Console.
- **`sentinelone-cli system database-status`** - Get an indication of the system's database health status. <br>This command returns a positive response when the DB server is up and running. <br>This command does not require authentication. <br>Rate limit: 1 call per second for each IP address that communicates with the Console.
- **`sentinelone-cli system get-config`** - Get the configuration of your SentinelOne system. <br>The response shows basic information of the deployed SKUs and licenses, 2FA, and the Management URL.
- **`sentinelone-cli system info`** - Get the Console build, version, patch, and release information.
- **`sentinelone-cli system set-config`** - Change the system configuration. <br>Before you run this, see Get System Config. <br>This command requires a Global Admin user or Support.
- **`sentinelone-cli system status`** - Get an indication of the system's health status. <br>This command returns a positive response when the Management Console and API server are up and running. This command does not require authentication.<br>Rate limit: 1 call per second for each IP address that communicates with the Console.

### tags

tags operations

- **`sentinelone-cli tags create`** - Add tags to create user-defined logical groups.
- **`sentinelone-cli tags delete`** - Delete tags by given filter.
- **`sentinelone-cli tags delete-by-id`** - Delete tag by ID.
- **`sentinelone-cli tags edit`** - Edit tag
- **`sentinelone-cli tags get`** - Get tags.

### tasks-configuration

tasks-configuration operations

- **`sentinelone-cli tasks-configuration create-task`** - Create a task configuration.
- **`sentinelone-cli tasks-configuration get-child-scope-task-configuration`** - Get the task configuration of child scopes of the given scope, if the tasks are not inherited.
- **`sentinelone-cli tasks-configuration get-task-configuration`** - Get the task configuration of a scope.
- **`sentinelone-cli tasks-configuration has-child-scopes`** - From a given scope, see if there are scopes under it that have local, explicit tasks. The response returns True if a sub-scope has a local (not inherited) task configuration.

### tenant

tenant operations

- **`sentinelone-cli tenant global-policy`** - Get the Global policy. This is the default policy for your deployment. See also: Get Policy.
- **`sentinelone-cli tenant update-global-policy`** - Change the policy of your deployment. Best practice: Get the Global policy before you attempt to change it. See also:  Get Policy. 
 You must be a Global Admin user to change the Global Policy.

### tests

tests operations

- **`sentinelone-cli tests`** - Returns a metadata list of the available free-text filters

### threat-intelligence

threat-intelligence operations

- **`sentinelone-cli threat-intelligence create-io-cs`** - Add an IoC to the Threat Intelligence database. These values under data are required fields: source, externalID, type, value, and method. "Type" and "method" must be in upper case.
- **`sentinelone-cli threat-intelligence delete-io-cs`** - Delete an IoC from the Threat Intelligence database that matches a filter using the accountID and one other field.

### threats

threats operations

- **`sentinelone-cli threats add-note-to-multiple`** - Add a threat note to multiple threats.
- **`sentinelone-cli threats add-to-blacklist`** - Add threats that have a SHA1 hash and that match the filter to the Blacklist of the target scope: Global, Account, Site, or Group.<BR> Your role must have permissions to change the Blacklist - Admin, IR Team, SOC - and your user scope access must include the Agent. The target scope is the Group, Site, or Account of the Agent.
- **`sentinelone-cli threats add-to-exclusions`** - Add a threat to exclusions. The "whitening option" is required. <BR>When you create an exclusion, you override the "malicious" verdict of the Agent for a detection. This can open holes in your security deployment. Use with caution.<BR>Best practice: Use the most specific definition of the exclusion possible and the lowest mode possible.
- **`sentinelone-cli threats disable-engines`** - If your list of threats shows too many False Positives, use this command to troubleshoot the Agent Engines that return unexpected results in your deployment. Valid values:  "penetration", "dataFiles","exploits", "reputation", "executables", "preExecutionSuspicious", "preExecution", "lateralMovement", and "pup".
- **`sentinelone-cli threats export`** - Export data of threats (as seen in the Console > Incidents) that match the filter. Note: Use the filter. This command exports only 20,000 items (each datum is an item).
- **`sentinelone-cli threats export-mitigation-report`** - Export the mitigation report as a CSV file.
- **`sentinelone-cli threats fetch-file`** - Fetch a file associated with the threat that matches the filter. Your user role must have permissions to Fetch Threat File - Admin, IR Team, SOC.
- **`sentinelone-cli threats get`** - Get data of threats that match the filter. <BR>Best Practice: Use the filters. Each threat gives a number of data lines that will quickly fill the page limit.
- **`sentinelone-cli threats mitigate`** - Apply a mitigation action to a group of threats that match the filter. Valid values for mitigation: "kill", "quarantine", "remediate", "rollback-remediation", "un-quarantine","network-quarantine".<BR>Your user role must have permissions to mitigate threats - Admin, IR Team, SOC. <BR>Rollback is applied only on Windows. Remediate is applied only on macOS and Windows.
- **`sentinelone-cli threats update-analyst-verdict`** - Change the verdict of a threat, as determined by a Console user.
- **`sentinelone-cli threats update-external-ticket-id`** - Change the external ticket ID of a threat.
- **`sentinelone-cli threats updated-incident`** - Update the incident details of a threat.

### update

update operations

- **`sentinelone-cli update delete-packages`** - Delete Agent packages from your Management. Use the IDs from Get Latest Packages.
- **`sentinelone-cli update download-agent-package`** - [DEPRECATED] Download an agent package by package ID.Rate limit: 2 call per minute for each different user token
- **`sentinelone-cli update download-package`** - Download a package by site_id ("sites") and filename. <br>Rate limit: 2 call per minute for each user token. <br>Use this command to manually deploy Agent updates that cannot be deployed with the update-software command (see Agent Actions > Update Software) or through the Console.
- **`sentinelone-cli update get-latest-packages`** - Get the Agent packages that are uploaded to your Management. <br>The response shows the data of each package, including the IDs, which you can use in other commands.
- **`sentinelone-cli update latest-packages-by-os`** - [DEPRECATED] Use "Latest packages" API call instead ("GET /web/api/v2.1/update/agent/packages").
- **`sentinelone-cli update package`** - Update the metadata for an existing package.

### upload

upload operations

- **`sentinelone-cli upload agent-package`** - If you have an On-Prem Management or you are a participant in the Beta program, you can use this command to upload an Agent package to the Management. Then you can deploy the Agent to update endpoints.
- **`sentinelone-cli upload deploy-system-package`** - If you have an On-Prem Management or you are a participant in the Beta program, you can upload a Management package and then use this command to deploy the new Management. You must first upload the package (see Upload System Package).
- **`sentinelone-cli upload system-package`** - If you have an On-Prem Management or otherwise require a manual package upload, use this command to upload an Agent package or a Management package. Then you can deploy the update (see Deploy System Package).

### user

users operations

- **`sentinelone-cli user`** - Get a user by token.

### users

users operations

- **`sentinelone-cli users auth-app`** - Authenticate a user with a third-party app, such as DUO or Google Authenticator, for deployments that require Two Factor Authentication.
- **`sentinelone-cli users auth-by-sso`** - Authenticate a Single Sign-On response over SAML v2 protocol.
- **`sentinelone-cli users auth-recovery-code`** - Authenticate a user with a recovery code.
- **`sentinelone-cli users bulk-delete`** - Delete all users that match the filter.
- **`sentinelone-cli users change-password`** - Change the user password.
- **`sentinelone-cli users check-global`** - See if logged in user is a user with the Global scope of access.
- **`sentinelone-cli users check-remote-shell-permissions`** - See if the logged in user is allowed to use Remote Shell.
- **`sentinelone-cli users check-viewer`** - See if the logged in user has only viewer permissions.
- **`sentinelone-cli users create`** - Create a new user.
- **`sentinelone-cli users delete`** - Delete a user by ID.
- **`sentinelone-cli users disable-2-fa`** - Disable Two-Factor Authentication for one user. This requires the ID of the user (run "users").
- **`sentinelone-cli users email-verification`** - When a new user verifies their email, the Management gets a token. Use this command to verify the token and set a new password.
- **`sentinelone-cli users enable-2-fa`** - Enable two-factor authentication for a given user.
- **`sentinelone-cli users enable-2-fa-app`** - Enable support for the 2FA app (such as Duo or Google Authenticator) that your Console users will use to log in.
- **`sentinelone-cli users generate-api-token`** - Get the API token for the authenticated user.
- **`sentinelone-cli users generate-i-frame-token`** - Get a new iFrame token with the provided limitations.
- **`sentinelone-cli users generate-recovery-code`** - Get recovery codes for user authentication.
- **`sentinelone-cli users get`** - Get a user by ID.
- **`sentinelone-cli users list`** - Get a list of users.
- **`sentinelone-cli users login`** - Authenticate a user by username and password and return an authentication token. Rate limit: 1 call per second for each different IP address that communicate with the Console.
- **`sentinelone-cli users login-by-api-token`** - Log in to the API with a token. To learn more about temporary and 6-month tokens and how to generate them, see https://support.sentinelone.com/hc/en-us/articles/360004195934.
- **`sentinelone-cli users login-by-token`** - Log in with user token.
- **`sentinelone-cli users logout`** - Log out the authenticated user.
- **`sentinelone-cli users redirect-to-sso`** - If SSO is enabled for a deployment or scope, and a user attempts to log in with name and password, this command redirects the login to SSO.
- **`sentinelone-cli users request-2-fa-app`** - Request 2FA App response.
- **`sentinelone-cli users revoke-api-token`** - Revoke an API token.
- **`sentinelone-cli users send-verification-email`** - Send verification email to users that match the filter. Warning: Active users will be locked out of the Management Console until they verify their email. If your Management Console has Onboarding enabled, when you create a new user, the user gets an email invitation. If the user does not respond in time or loses the email, you can send it again. You can send the email invitation to multiple users. Your SMTP server must be correctly configured in Settings > SMTP for the Global scope. Changing the Global SMTP settings requires an Admin role with Global scope or Support.
- **`sentinelone-cli users sign-eula`** - Mark the End User License Agreement (EULA) as signed for user scopes.
- **`sentinelone-cli users token-details`** - Get details of the API token that matches the filter.
- **`sentinelone-cli users update`** - Change properties of the user of the given ID.
- **`sentinelone-cli users validate-verification-token`** - When a new user verifies their email, the Management gets a token.  Use this command to validate the token.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
sentinelone-cli accounts get

# JSON for scripting and agents
sentinelone-cli accounts get --json

# Filter to specific fields
sentinelone-cli accounts get --json --select id,name,status

# Dry run  -  show the request without sending
sentinelone-cli accounts get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
sentinelone-cli accounts get --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
sentinelone-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/sentinelone-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SENTINELONE_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `sentinelone-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `sentinelone-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SENTINELONE_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Token expired (SentinelOne rotates every 6 months) or wrong scope  -  regenerate the Service User's API token and re-export SENTINELONE_API_TOKEN.
- **404 / connection errors**  -  SENTINELONE_BASE_URL must include the /web/api/v2.1 suffix and your exact console host, e.g. https://usea1-partners.sentinelone.net/web/api/v2.1.
- **A history command says 'need at least 2 syncs'**  -  Drift, rollout, MTTR, and verdict-change views compare snapshots  -  run `sentinelone-cli sync` at least twice over time before using them.
- **A write command (mitigate, disconnect) reports no body sent**  -  The public spec omits request bodies for some actions  -  pipe the documented fields as JSON via --stdin (echo '{"filter":{"ids":["<id>"]}}' | sentinelone-cli threats mitigate kill --stdin) until typed flags land.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Celerium/SentinelOne-PowerShellWrapper**](https://github.com/Celerium/SentinelOne-PowerShellWrapper)  -  PowerShell
- [**Sentinel-One/purple-mcp**](https://github.com/Sentinel-One/purple-mcp)  -  Python
- [**x35029/sentinelone-sdk**](https://github.com/x35029/sentinelone-sdk)  -  Python
- [**fragtastic/sentinelone-api-python**](https://github.com/fragtastic/sentinelone-api-python)  -  Python
- [**Ltango/SentinelOne-API**](https://github.com/Ltango/SentinelOne-API)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
