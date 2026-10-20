---
name: dnsfilter
description: "Every DNSFilter API operation, plus one-command answers to 'why is this site blocked for this client' and 'which clients are behind on the roaming agent'. Trigger phrases: `why is this site blocked in DNSFilter`, `allow this domain for a client in DNSFilter`, `which DNSFilter roaming clients are out of date`, `DNSFilter unblock requests`, `use dnsfilter`, `run dnsfilter`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "DNSFilter"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - dnsfilter-cli
    install:
      - kind: script
        bins: [dnsfilter-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1
---

# DNSFilter  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `dnsfilter-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex
   ```
3. Verify: `dnsfilter-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `dnsfilter-cli` and `dnsfilter-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Manage organizations, sites, policies, allow and block lists, roaming clients and reports from the terminal or an agent. why-blocked and allow close the daily blocked-site ticket loop; agents drift and agents stale give an MSP a cross-client view of the roaming-agent fleet from a local SQLite mirror.

## When to Use This CLI

Use this CLI when an MSP technician or agent needs to work DNSFilter across many client organizations: diagnosing and fixing blocked-site tickets, working the unblock-request queue, checking roaming-client versions and check-ins across the fleet, or auditing allow and block lists. It also covers every raw DNSFilter API operation for scripting.

## Anti-triggers

Do not use this CLI for:
- Installing or upgrading the roaming client on an endpoint: use your RMM or the DNSFilter installer (this CLI can only queue an uninstall).
- Testing DNS resolution from a device: use dig or nslookup on that device.
- Distributor billing reconciliation from a non-distributor key: the distributor endpoints return authorization errors.

## Unique Capabilities

### Diagnose and fix a block
- **`why-blocked`**  -  Name the site or roaming-client policy and rule that blocks or allows a domain for a client org, plus query-log hits. Does not evaluate user, collection, LAN-subnet, Universal List or AppAware rules.

  _Reach for this first on any 'site is blocked for client X' ticket: it resolves the effective policy (including the current scheduled-policy slot) and checks the query log for the policy that really blocked it._

  ```bash
  dnsfilter-cli why-blocked example.com --org "Acme Dental" --agent
  ```
- **`allow`**  -  Add a domain to the allow list of the policies blocking it for a client (from the query log, else the site or roaming-client assignment). Previews by default; --confirm --note writes, then reads back.

  _Use after why-blocked confirms a legitimate site is blocked. --force is needed for a threat-categorized domain, --include-global to touch a global policy, and policies the key cannot edit are skipped._

  ```bash
  dnsfilter-cli allow example.com --org "Acme Dental" --note "TKT-4821"
  ```
- **`unblocks triage`**  -  Review pending end-user unblock requests grouped by domain across every client, with threat flags, age, and DNSFilter's own already-covered check. Needs unblock requests enabled on the account.

  _Use to work the unblock-request queue in one pass; act on a group with unblock-requests decision unblock-requests <id> --decision <action>._

  ```bash
  dnsfilter-cli unblocks triage --agent
  ```

### Fleet health across every client
- **`agents drift`**  -  See which client orgs have roaming clients behind the latest agent release, or on a specific bad build, with auto-update state per org. Joins the synced fleet with the vendor's live release feed.

  _Use after an agent release goes bad, or in the weekly fleet pass. Run dnsfilter-cli sync --resources organizations,networks,user-agents first; the release feed is read live so a rollback shows up immediately._

  ```bash
  dnsfilter-cli agents drift --bad-version 3.1.0 --agent
  ```
- **`agents stale`**  -  List roaming clients that stopped checking in or are unprotected or bypassed, grouped by client org with age buckets. Reads the local mirror; sync first.

  _Use for the weekly check of endpoints that are silently unfiltered. Run dnsfilter-cli sync --resources organizations,networks,user-agents first._

  ```bash
  dnsfilter-cli agents stale --older-than 7d --agent
  ```

### Policy hygiene across tenants
- **`lists where`**  -  Find every policy across every client whose allow or block list holds a domain or a parent domain, with counts of the sites and roaming clients it governs. Reads the local mirror; sync first.

  _Use to check precedent before allowing a domain, or to measure exposure when a domain makes the news. Run dnsfilter-cli sync --resources organizations,networks,policies,user-agents first; an empty mirror returns an empty result._

  ```bash
  dnsfilter-cli lists where example.com --agent
  ```
- **`policy-audit`**  -  Flag allow-listed domains now categorized as threats, unblocked security categories, and allowed unknown domains; with --baseline <policy-id>, also drift from that baseline policy.

  _Use in the recurring policy hygiene review; allow-list entries go bad as threat categorization changes._

  ```bash
  dnsfilter-cli policy-audit --org "Acme Dental" --baseline 1201 --agent
  ```

## Command Reference

**agent-local-user-bulk-deletes**  -  Manage agent local user bulk deletes

- `dnsfilter-cli agent-local-user-bulk-deletes counts`  -  Gets an agent local user bulk delete count by bulk delete id.
- `dnsfilter-cli agent-local-user-bulk-deletes create`  -  Create an agent local user bulk delete.
- `dnsfilter-cli agent-local-user-bulk-deletes show`  -  Gets an agent local user bulk delete by id.

**agent-local-users**  -  Manage agent local users

- `dnsfilter-cli agent-local-users all`  -  Gets ALL agent local users associated to an user organization with basic information.
- `dnsfilter-cli agent-local-users counts`  -  Gets counts of agent local users by policy assignment status.
- `dnsfilter-cli agent-local-users destroy`  -  Deletes a agent local user.
- `dnsfilter-cli agent-local-users index`  -  Gets the agent local users associated to an organization with basic information.
- `dnsfilter-cli agent-local-users show`  -  Gets basic information of the specified agent local user.
- `dnsfilter-cli agent-local-users update`  -  Updates an agent local user with the specified data.

**agent-local-users-csv-export**  -  Manage agent local users csv export

- `dnsfilter-cli agent-local-users-csv-export create`  -  Creates an agent local users CSV export record to track export of agent local users.
- `dnsfilter-cli agent-local-users-csv-export show`  -  Shows an agent local users CSV export.

**api-keys**  -  Manage api keys

- `dnsfilter-cli api-keys create`  -  Creates a new api key.
- `dnsfilter-cli api-keys destroy`  -  Removes the api key.
- `dnsfilter-cli api-keys index`  -  Gets the list of api keys associated with the current user.
- `dnsfilter-cli api-keys show`  -  Displays an api key.

**application-categories**  -  Manage application categories

- `dnsfilter-cli application-categories index`  -  List extant application categories.
- `dnsfilter-cli application-categories show`  -  Gets basic information of a specific application category.

**applications**  -  Get information about available applications for the allow and block lists.

- `dnsfilter-cli applications all`  -  List applications basic information, including deleted ones.
- `dnsfilter-cli applications index`  -  List extant applications basic information.
- `dnsfilter-cli applications show`  -  Gets basic information of a specific application.

**billing**  -  Manage billing

- `dnsfilter-cli billing address-show`  -  Show the billing address for the given organization.
- `dnsfilter-cli billing address-update`  -  Update the billing address for the given organization.
- `dnsfilter-cli billing create`  -  Register a newly created payment method in database.
- `dnsfilter-cli billing show`  -  List basic billing information of the user organization registered in Stripe platform.

**block-pages**  -  Manage block pages

- `dnsfilter-cli block-pages all`  -  Gets ALL block pages associated with the current user.
- `dnsfilter-cli block-pages create`  -  Creates a new block page.
- `dnsfilter-cli block-pages destroy`  -  Deletes a block page.
- `dnsfilter-cli block-pages index`  -  Gets the list of block pages associated with the current user.
- `dnsfilter-cli block-pages show`  -  Gets the specified block page.
- `dnsfilter-cli block-pages update`  -  Updates a block_page with the specified data.

**categories**  -  Manage categories

- `dnsfilter-cli categories all`  -  List all categories including internal categories.
- `dnsfilter-cli categories index`  -  List categories basic information.
- `dnsfilter-cli categories show`  -  Gets basic information of a specific category.

**collections**  -  Manage collections


**current-user**  -  Manage current user

- `dnsfilter-cli current-user user-current-show`  -  Fetch information about the currently authenticated user.
- `dnsfilter-cli current-user user-current-update`  -  Update attributes for the currently authenticated user.
- `dnsfilter-cli current-user user-organization-ui-settings-suppress-license-warning`  -  Returns the suppression date and relevant information.
- `dnsfilter-cli current-user user-ui-settings-show`  -  Returns the current user's UI settings.
- `dnsfilter-cli current-user user-ui-settings-update`  -  Updates current user's UI settings.

**cyber-sight**  -  Manage cyber sight

- `dnsfilter-cli cyber-sight csv-exports-create`  -  Creates a cybersight csv export record to track export of cybersight report info.
- `dnsfilter-cli cyber-sight csv-exports-show`  -  Shows a CyberSight csv export.

**dashboards**  -  Manage dashboards

- `dnsfilter-cli dashboards available`  -  Dashboards available to the current user.
- `dnsfilter-cli dashboards create`  -  Create a dashboard.
- `dnsfilter-cli dashboards destroy`  -  Delete a dashboard.
- `dnsfilter-cli dashboards index`  -  Dasbboards for use by the current user.
- `dnsfilter-cli dashboards link`  -  Linking a dashboard will add it to the list for the current user.
- `dnsfilter-cli dashboards make-private`  -  This will remove access to the dashboard for other users.
- `dnsfilter-cli dashboards make-public`  -  This will make it available to other users who are members in an organization the owning user is a member of.
- `dnsfilter-cli dashboards owned`  -  Dashboards owned by the current user.
- `dnsfilter-cli dashboards unlink`  -  Unlinking a dashboard will remove it from the list of dashboards for the current user.
- `dnsfilter-cli dashboards update`  -  Update a dashboard.

**dictionary**  -  Manage dictionary

- `dnsfilter-cli dictionary lookups-ai-applications`  -  Return a list of AI Applications and websites.
- `dnsfilter-cli dictionary lookups-cyber-sight-activity-types`  -  Returns all available CyberSightReports activity types.
- `dnsfilter-cli dictionary lookups-qp-methods`  -  List QP Methods
- `dnsfilter-cli dictionary lookups-vpn-settings-state-types`  -  Returns all available VPN Settings State types.

**distributors**  -  Manage distributors

- `dnsfilter-cli distributors health`  -  Allows testing API reachability and validation of the authentication header.
- `dnsfilter-cli distributors index`  -  Allows to fetch basic information about a Distributor and its parent organization.
- `dnsfilter-cli distributors msps-cancel`  -  Cancels the specified MSP parent organization and all of its sub-organizations.
- `dnsfilter-cli distributors msps-create`  -  Creates an MSP and its parent organization.
- `dnsfilter-cli distributors msps-index`  -  Allows to get the list of MSPs that matches the search criteria.
- `dnsfilter-cli distributors msps-reactivate`  -  Reactivates the specified MSP parent organization and all of its sub-organization if specified.
- `dnsfilter-cli distributors msps-show`  -  Allows to fetch basic information about an MSP and its parent organization.
- `dnsfilter-cli distributors msps-update`  -  Updates the specified MSP and its parent organization data.
- `dnsfilter-cli distributors msps-upsert-skus`  -  Adds or removes the SKUs available to an entitlements-managed MSP post-creation. Returns the updated MSP.
- `dnsfilter-cli distributors organizations-add-sku`  -  Helper endpoint to add a single SKU to the specified MSP parent organization or sub-organization instead of doing a
- `dnsfilter-cli distributors organizations-cancel`  -  Cancels the specified MSP sub-organization. Note: See _Organization Cancellation_ for important details.
- `dnsfilter-cli distributors organizations-create`  -  Creates an MSP sub-organization.
- `dnsfilter-cli distributors organizations-index`  -  Allows to get the list of organizations that matches the search criteria
- `dnsfilter-cli distributors organizations-reactivate`  -  Reactivates the specified MSP sub-organization. Note: See _Organization Cancellation_ for important details.
- `dnsfilter-cli distributors organizations-remove-sku`  -  Helper endpoint to remove a single SKU to the specified MSP parent organization or sub-organization instead of doing a
- `dnsfilter-cli distributors organizations-show`  -  Allows to fetch basic information about an MSP parent organization or sub-organization.
- `dnsfilter-cli distributors organizations-update`  -  Updates the specified MSP sub-organization data.
- `dnsfilter-cli distributors reports-covered-users-summary-by-organization`  -  Allows to fetch covered users information by organization for the given date range.
- `dnsfilter-cli distributors reports-usage-by-organization`  -  Allows to fetch user usage information per organization.
- `dnsfilter-cli distributors reports-usage-by-sku`  -  Allows to fetch user usage information per SKU.
- `dnsfilter-cli distributors skus-available`  -  Returns the list of SKUs available to the distributor's customers.
- `dnsfilter-cli distributors update`  -  Updates the specified support Distributor information.
- `dnsfilter-cli distributors users-add-membership`  -  Adds/creates a new user membership with an organization.
- `dnsfilter-cli distributors users-create`  -  Creates a new user alongside a membership with an organization.
- `dnsfilter-cli distributors users-index`  -  Allows to get the list of users that matches the search criteria.
- `dnsfilter-cli distributors users-remove-membership`  -  Removes/deletes an existing user membership with an organization.
- `dnsfilter-cli distributors users-reset-password-url`  -  Creates a new reset password URL for the specified user. The URL is valid for 432000 seconds (5 days).
- `dnsfilter-cli distributors users-send-reset-password-email`  -  Sends a reset password email to the specified user.
- `dnsfilter-cli distributors users-show`  -  Allows to fetch basic information about an user.
- `dnsfilter-cli distributors users-update-membership`  -  Updates an existing user membership with an organization.

**domains**  -  Manage domains

- `dnsfilter-cli domains bulk-lookup`  -  Gets the domains and categories associated for each FQDN of the specified list.
- `dnsfilter-cli domains suggest-threat`  -  Mark a FQDN to verify its threat categorization. Request example: `/v1/domains/suggest_threat?
- `dnsfilter-cli domains user-lookup`  -  Gets all the domains associated for a particular FQDN.

**enterprise-connections**  -  Manage enterprise connections

- `dnsfilter-cli enterprise-connections create`  -  Create an enterprise connection.
- `dnsfilter-cli enterprise-connections destroy`  -  Delete an enterprise connection.
- `dnsfilter-cli enterprise-connections index`  -  Gets information for all the enterprise connections for the given organization.
- `dnsfilter-cli enterprise-connections show`  -  Gets information for the specified enterprise connection.
- `dnsfilter-cli enterprise-connections update`  -  Update an enterprise connection.

**invoices**  -  Manage invoices

- `dnsfilter-cli invoices current`  -  Gets user estimated invoice for next month.
- `dnsfilter-cli invoices index`  -  Gets user associated invoices, most recent first.
- `dnsfilter-cli invoices show`  -  Gets basic information of the specified invoice.

**ip-addresses**  -  Manage ip addresses

- `dnsfilter-cli ip-addresses all`  -  Gets ALL user associated IP addresses basic information.
- `dnsfilter-cli ip-addresses create`  -  Creates a new IP address with the specified data.
- `dnsfilter-cli ip-addresses destroy`  -  Removes an IP address.
- `dnsfilter-cli ip-addresses index`  -  Gets user associated IP addresses basic information.
- `dnsfilter-cli ip-addresses myip`  -  Responds with the requester reported IP address.
- `dnsfilter-cli ip-addresses show`  -  Gets basic information of the specified IP address.
- `dnsfilter-cli ip-addresses update`  -  Updates an IP address with the specified data.
- `dnsfilter-cli ip-addresses verify`  -  Verification call to ensure IP address is not already in the system.

**mac-addresses**  -  Manage mac addresses

- `dnsfilter-cli mac-addresses all`  -  Gets ALL MAC addresses associated to an user with basic information.
- `dnsfilter-cli mac-addresses create`  -  Creates a new MAC address with the specified data.
- `dnsfilter-cli mac-addresses destroy`  -  Deletes a MAC address.
- `dnsfilter-cli mac-addresses index`  -  Gets the MAC addresses associated to an organization with basic information.
- `dnsfilter-cli mac-addresses show`  -  Gets basic information of the specified MAC address.
- `dnsfilter-cli mac-addresses update`  -  Updates a MAC address with the specified data.

**metrics**  -  Manage metrics

- `dnsfilter-cli metrics organization-usage`  -  Gets usage information for the given Organization over the given date range. Maximum date range is 365 days.
- `dnsfilter-cli metrics organization-usage-detailed`  -  Gets usage information for the given organization.

**networks**  -  Manage networks

- `dnsfilter-cli networks all`  -  Gets ALL networks associated to the user with its basic information.
- `dnsfilter-cli networks bulk-create`  -  Bulk create multiple networks with the specified data.
- `dnsfilter-cli networks bulk-create-status`  -  Bulk create status.
- `dnsfilter-cli networks bulk-destroy`  -  Bulk destroy multiple networks.
- `dnsfilter-cli networks bulk-destroy-status`  -  Bulk destroy status.
- `dnsfilter-cli networks bulk-update`  -  Bulk update multiple networks with the specified data.
- `dnsfilter-cli networks bulk-update-status`  -  Bulk update status
- `dnsfilter-cli networks counts`  -  Gets counts of networks for each network status.
- `dnsfilter-cli networks create`  -  Creates a new network with the specified data.
- `dnsfilter-cli networks destroy`  -  Deletes a network.
- `dnsfilter-cli networks geo`  -  Gets user's associated networks with geo information only.
- `dnsfilter-cli networks index`  -  Gets user's associated networks with basic information.
- `dnsfilter-cli networks lookup`  -  Gets basic information of a network based on its associated IP addresses.
- `dnsfilter-cli networks msp`  -  Gets MSP user associated networks with basic information.
- `dnsfilter-cli networks msp-all`  -  Gets ALL networks associated to the MSP user with its basic information.
- `dnsfilter-cli networks show`  -  Gets basic information of the specified network.
- `dnsfilter-cli networks subnets-all`  -  List site subnets with basic information.
- `dnsfilter-cli networks update`  -  Updates a network with the specified data.

**networks-csv-export**  -  Manage networks csv export

- `dnsfilter-cli networks-csv-export create`  -  Creates a networks csv export record to track export of networks.
- `dnsfilter-cli networks-csv-export show`  -  Shows a networks csv export.

**notes**  -  Manage notes

- `dnsfilter-cli notes domain-show`  -  Gets associated notes from a specific resource.
- `dnsfilter-cli notes domain-update`  -  Updates notes for a resource with the specified data.

**organizations**  -  Manage organizations

- `dnsfilter-cli organizations all`  -  Gets all organizations basic information.
- `dnsfilter-cli organizations bulk-update`  -  Updates one or more organizations with the specified data.
- `dnsfilter-cli organizations cancel`  -  Sets an organization as 'Canceled'.
- `dnsfilter-cli organizations create`  -  Creates a new organization with the specified data.
- `dnsfilter-cli organizations destroy`  -  Deletes an MSP customer.
- `dnsfilter-cli organizations index`  -  Gets user's associated organizations basic information.
- `dnsfilter-cli organizations promote-to-msp`  -  Promote an organization to a MSP.
- `dnsfilter-cli organizations settings`  -  Gets basic information of the specified organization.
- `dnsfilter-cli organizations show`  -  Gets basic information of the specified organization.
- `dnsfilter-cli organizations update`  -  Updates an organization with the specified data.

**policies**  -  Manage policies

- `dnsfilter-cli policies add-allowed-application`  -  Adds a single application to the allow list.
- `dnsfilter-cli policies add-blacklist-category`  -  Adds a single category to the blocklist.
- `dnsfilter-cli policies add-blacklist-domain`  -  Adds a single domain to the blocklist.
- `dnsfilter-cli policies add-blocked-application`  -  Adds a single application to the block list.
- `dnsfilter-cli policies add-whitelist-domain`  -  Adds a single domain to the allowlist.
- `dnsfilter-cli policies all`  -  Gets ALL user associated policies basic information.
- `dnsfilter-cli policies application`  -  Gets policies information for a specific application.
- `dnsfilter-cli policies application-update`  -  Updates a policy with the specified application data.
- `dnsfilter-cli policies bulk-actions-add-allowlist-domains`  -  Adds one or more domains to one or more policy's allow list.
- `dnsfilter-cli policies bulk-actions-add-blocklist-domains`  -  Adds one or more domains to one or more policy's block list.
- `dnsfilter-cli policies bulk-actions-remove-allowlist-domains`  -  Removes one or more domains from one or more policy's allow list.
- `dnsfilter-cli policies bulk-actions-remove-blocklist-domains`  -  Removes one or more domains from one or more policy's block list.
- `dnsfilter-cli policies create`  -  Creates a new policy with the specified data.
- `dnsfilter-cli policies destroy`  -  Deletes a policy.
- `dnsfilter-cli policies index`  -  Gets user associated policies basic information.
- `dnsfilter-cli policies remove-allowed-application`  -  Removes a single application from the allow list.
- `dnsfilter-cli policies remove-blacklist-category`  -  Removes a single category from the blocklist.
- `dnsfilter-cli policies remove-blacklist-domain`  -  Removes a single domain from the blocklist.
- `dnsfilter-cli policies remove-blocked-application`  -  Removes a single application from the block list.
- `dnsfilter-cli policies remove-whitelist-domain`  -  Removes a single domain from the allowlist.
- `dnsfilter-cli policies show`  -  Gets basic information of the specified policy.
- `dnsfilter-cli policies update`  -  Updates a policy with the specified data.

**policy-ips**  -  Manage policy ips

- `dnsfilter-cli policy-ips index`  -  Gets user associated policies IPs basic information from database.
- `dnsfilter-cli policy-ips show`  -  Gets basic information of the specified Policy IP.

**psa-integrations**  -  Manage psa integrations

- `dnsfilter-cli psa-integrations`  -  Creates a new PSA integration, if one does not exist and returns the redirect link.

**scheduled-policies**  -  Manage scheduled policies

- `dnsfilter-cli scheduled-policies all`  -  Gets ALL scheduled policies associated with the current user.
- `dnsfilter-cli scheduled-policies create`  -  Creates a new scheduled policy.
- `dnsfilter-cli scheduled-policies destroy`  -  Removes a scheduled policy from the database.
- `dnsfilter-cli scheduled-policies index`  -  Gets the list of scheduled policies associated with the current user.
- `dnsfilter-cli scheduled-policies show`  -  Gets the specified scheduled policy.
- `dnsfilter-cli scheduled-policies update`  -  Updates a scheduled policy in the database with the specified data.

**scheduled-report-previews**  -  Manage scheduled report previews

- `dnsfilter-cli scheduled-report-previews create`  -  Creates a scheduled report preview, triggering the background generation.
- `dnsfilter-cli scheduled-report-previews show`  -  Gets the specified scheduled report preview.

**scheduled-reports**  -  Manage scheduled reports

- `dnsfilter-cli scheduled-reports bulk-update`  -  Pauses or resumes several schedules in one call  -  the grid's multi-select Active toggle.
- `dnsfilter-cli scheduled-reports create`  -  Creates a scheduled report.
- `dnsfilter-cli scheduled-reports create-scheduledreports`  -  Creates a schedule with the options the drawer exposes: a name, a report type, external recipients
- `dnsfilter-cli scheduled-reports destroy`  -  Deletes the scheduled report.
- `dnsfilter-cli scheduled-reports destroy-scheduledreports`  -  Deletes the schedule and its recipients outright.
- `dnsfilter-cli scheduled-reports index`  -  Get all scheduled reports associated with current user's organizations.
- `dnsfilter-cli scheduled-reports index-scheduledreports`  -  Returns a page of scheduled reports for the schedules grid, scoped to the organizations the caller administers.
- `dnsfilter-cli scheduled-reports show`  -  Gets the specified scheduled report.
- `dnsfilter-cli scheduled-reports show-scheduledreports`  -  Everything the Schedule Report drawer renders
- `dnsfilter-cli scheduled-reports update`  -  Updates a scheduled report.
- `dnsfilter-cli scheduled-reports update-scheduledreports`  -  Also the pause/resume endpoint: the grid's Active toggle is a PATCH carrying `active` alone.

**traffic-reports**  -  Manage traffic reports

- `dnsfilter-cli traffic-reports qps`  -  Gets the total number of queries per second in a period of time for sites.
- `dnsfilter-cli traffic-reports qps-active-agents`  -  Gets the total number of queries per second in a period of time (maximum 20 minutes) for roaming clients.
- `dnsfilter-cli traffic-reports qps-active-organizations`  -  Gets the total number of queries per second in a period of time (maximum 20 minutes) for organizations.
- `dnsfilter-cli traffic-reports query-logs`  -  Gets the query raw logs in a period of time.
- `dnsfilter-cli traffic-reports top-agents`  -  Gets the top agents in a period of time.
- `dnsfilter-cli traffic-reports top-application-categories`  -  Gets the top application categories domains in a period of time.
- `dnsfilter-cli traffic-reports top-categories`  -  Gets the top categories domains in a period of time.
- `dnsfilter-cli traffic-reports top-collections`  -  Gets the top collections in a period of time.
- `dnsfilter-cli traffic-reports top-domains`  -  Gets the top requested domains in a period of time.
- `dnsfilter-cli traffic-reports top-networks`  -  Gets the top networks in a period of time.
- `dnsfilter-cli traffic-reports top-organizations-requests`  -  Gets the top organizations by number of requests.
- `dnsfilter-cli traffic-reports top-users`  -  Gets the top users in a period of time.
- `dnsfilter-cli traffic-reports total-applications-agents-stats`  -  Gets the statistics of number of requests for roaming clients by application in a period of time.
- `dnsfilter-cli traffic-reports total-applications-collections-stats`  -  Gets the statistics of number of requests for collections by application in a period of time.
- `dnsfilter-cli traffic-reports total-applications-networks-stats`  -  Gets the statistics of number of requests for sites by application in a period of time.
- `dnsfilter-cli traffic-reports total-applications-stats`  -  Gets the statistics of number of requests by application in a period of time.
- `dnsfilter-cli traffic-reports total-applications-users-stats`  -  Gets the statistics of number of requests for users by application in a period of time.
- `dnsfilter-cli traffic-reports total-categories`  -  Gets the total number of requests by category for sites in a period of time.
- `dnsfilter-cli traffic-reports total-categories-organizations`  -  Gets the total number of requests by category for organizations in a period of time.
- `dnsfilter-cli traffic-reports total-category-stats`  -  Gets the total number of stats for a category in a period of time.
- `dnsfilter-cli traffic-reports total-client-stats`  -  Gets the organization network, users and client stats.
- `dnsfilter-cli traffic-reports total-deployments`  -  Gets the organization deployments information.
- `dnsfilter-cli traffic-reports total-domain-stats`  -  Gets the total number of stats for a domain in a period of time.
- `dnsfilter-cli traffic-reports total-domains`  -  Gets the total number of requests by domain for sites in a period of time.
- `dnsfilter-cli traffic-reports total-domains-organizations`  -  Gets the total number of requests by domain for organizations in a period of time.
- `dnsfilter-cli traffic-reports total-domains-users`  -  Gets the total number of requests by domain for users in a period of time.
- `dnsfilter-cli traffic-reports total-organizations-requests`  -  Gets the total number of requests for organizations in a period of time.
- `dnsfilter-cli traffic-reports total-organizations-stats`  -  Gets the total number of stats for organizations in a period of time.
- `dnsfilter-cli traffic-reports total-requests`  -  Gets the total number of requests for sites in a period of time.
- `dnsfilter-cli traffic-reports total-requests-agents`  -  Gets the total number of requests for roaming clients in a period of time.
- `dnsfilter-cli traffic-reports total-requests-collections`  -  Gets the total number of requests for collections in a period of time.
- `dnsfilter-cli traffic-reports total-requests-geo`  -  Gets the total number of requests for organizations in a period of time for the heatmap on overview.
- `dnsfilter-cli traffic-reports total-requests-organizations`  -  Gets the total number of requests for organizations in a period of time.
- `dnsfilter-cli traffic-reports total-requests-users`  -  Gets the total number of requests for users in a period of time.
- `dnsfilter-cli traffic-reports total-roaming-clients`  -  Gets the roaming clients information.
- `dnsfilter-cli traffic-reports total-threats`  -  Gets the total number of threats for sites in a period of time.
- `dnsfilter-cli traffic-reports total-threats-organizations`  -  Gets the total number of threats for organizations in a period of time.
- `dnsfilter-cli traffic-reports total-threats-users`  -  Gets the total number of threats for users in a period of time.

**trials**  -  Manage trials

- `dnsfilter-cli trials`  -  Create trial.

**unblock-requests**  -  Admin queue for DNS unblock requests. Feeds the two dashboard grids: Active Requests
(awaiting a decision) and Request History (resolved). Gated behind the :enable_unblock_requests feature flag.

- `dnsfilter-cli unblock-requests counts`  -  Returns the Active and History tallies for the tab badges, scoped identically to index.
- `dnsfilter-cli unblock-requests index`  -  Returns a page of unblock requests for the selected tab
- `dnsfilter-cli unblock-requests pending-count`  -  Sidebar notification badge count.
- `dnsfilter-cli unblock-requests show`  -  Everything the Approve/Deny drawer renders for a single unblock request: request context

**unblock-requests-csv-export**  -  Async CSV export for the two Unblock Request grids. One endpoint serves both tabs via the
`tab` param. create snapshots the exact filtered / searched / sorted view (as an ordered uuid set,
capped at a server-side row cap) and enqueues the CSV build; show polls until the presigned
URL (or an error) is ready. It reuses the grid's query pipeline verbatim through
the list endpoint's filter logic, so the export always matches what the admin sees, and is gated behind the
:enable_unblock_requests feature flag exactly like the grid.

- `dnsfilter-cli unblock-requests-csv-export create`  -  Snapshots the currently filtered / searched / sorted view for the selected tab (capped at a server-side row cap rows
- `dnsfilter-cli unblock-requests-csv-export show`  -  Returns the tracking record with the presigned `url` once the build finished (or `error` if it failed)

**user-agent-bulk-deletes**  -  Manage user agent bulk deletes

- `dnsfilter-cli user-agent-bulk-deletes counts`  -  Gets a user agent bulk delete counts by id.
- `dnsfilter-cli user-agent-bulk-deletes create`  -  Create a user agent bulk delete.
- `dnsfilter-cli user-agent-bulk-deletes show`  -  Gets a user agent bulk delete by id.

**user-agent-bulk-updates**  -  Manage user agent bulk updates

- `dnsfilter-cli user-agent-bulk-updates counts`  -  Gets a user agent bulk delete counts by id.
- `dnsfilter-cli user-agent-bulk-updates create`  -  Create a user agent bulk update.
- `dnsfilter-cli user-agent-bulk-updates has-mixed`  -  Gets the value of whether each of the attributes of the elected user agents are mixed.
- `dnsfilter-cli user-agent-bulk-updates show`  -  Gets a user agent bulk update by id.

**user-agent-cleanups**  -  Manage user agent cleanups

- `dnsfilter-cli user-agent-cleanups create`  -  Creates a user agent cleanup to track bulk deletion of user agents.
- `dnsfilter-cli user-agent-cleanups show`  -  Gets the specific user agent cleanup.
- `dnsfilter-cli user-agent-cleanups update`  -  Updates a user agent cleanup.

**user-agent-csv-exports**  -  Manage user agent csv exports

- `dnsfilter-cli user-agent-csv-exports create`  -  Creates a user agent csv export record to track export of user agents.
- `dnsfilter-cli user-agent-csv-exports show`  -  Gets the specific user agent csv export.

**user-agent-releases**  -  Manage user agent releases

- `dnsfilter-cli user-agent-releases index`  -  Gets a list of latest user agent releases for each unique combination of agent_type, architecture, release_channels
- `dnsfilter-cli user-agent-releases relay`  -  Gets a list of latest relay releases for each unique combination of architecture, release_channels, and white label.

**user-agents**  -  Manage user agents

- `dnsfilter-cli user-agents all`  -  Gets a list of ALL user agents with basic information.
- `dnsfilter-cli user-agents counts`  -  Gets counts of user agents for each status.
- `dnsfilter-cli user-agents csv`  -  Gets CSV of organization's user agents.
- `dnsfilter-cli user-agents dequeue-uninstall`  -  Removes user agent from uninstall queue.
- `dnsfilter-cli user-agents destroy`  -  Soft deletes an user agent.
- `dnsfilter-cli user-agents index`  -  Gets a list of user agents with basic information.
- `dnsfilter-cli user-agents show`  -  Gets the specified user agent with basic information.
- `dnsfilter-cli user-agents tags`  -  Gets ALL tags used by user agents on a network or organization.
- `dnsfilter-cli user-agents uninstall-pin`  -  Gets Uninstall pin for organization's user agents.
- `dnsfilter-cli user-agents update`  -  Updates an user agent with the specified data.

**users**  -  Manage users

- `dnsfilter-cli users all`  -  Gets ALL users basic information.
- `dnsfilter-cli users change-password`  -  Changes the password for the currently authenticated user.
- `dnsfilter-cli users index`  -  Gets users basic information.
- `dnsfilter-cli users show`  -  Gets basic information of the specified user.


**agents**


- `dnsfilter-cli agents drift`  -  See which client orgs have roaming clients behind the latest agent release, or on a specific bad build, with auto-update state per org. Joins the synced fleet with the vendor's live release feed.

- `dnsfilter-cli agents stale`  -  List roaming clients that stopped checking in or are unprotected or bypassed, grouped by client org with age buckets. Reads the local mirror; sync first.


**allow**


- `dnsfilter-cli allow`  -  Add a domain to the allow list of the policies that blocked it for a client (from the query log, else the site or roaming-client assignment). Previews by default; writes only with --confirm --note, then reads each policy back.


**lists**


- `dnsfilter-cli lists where`  -  Find every policy across every client whose allow or block list contains a domain or one of its parent domains, with counts of the sites and roaming clients it governs. Reads the local mirror; sync first.


**policy-audit**


- `dnsfilter-cli policy-audit`  -  Flag allow-listed domains now categorized as threats, unblocked security categories, and allowed unknown domains; with --baseline <policy-id>, also drift from that baseline policy.


**unblocks**


- `dnsfilter-cli unblocks triage`  -  Review pending end-user unblock requests grouped by domain across every client, with threat flags, age, and DNSFilter's own already-covered check. Needs unblock requests enabled on the account.


**why-blocked**


- `dnsfilter-cli why-blocked`  -  Name the site or roaming-client policy and the rule that blocks (or allows) a domain for one client org, with recent query-log hits showing the policy that actually applied. User, collection and LAN-subnet assignments, Universal Lists and AppAware rules are not evaluated.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
dnsfilter-cli which "roaming clients behind the latest release"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Roaming client versions as compact JSON

```bash
dnsfilter-cli user-agents all --all --agent --select attributes.hostname,attributes.agent_version,attributes.last_sync
```

Narrow the large roaming-client payload to hostname, version and last check-in across every page before handing it to an agent.

### Precedent check before allowing a domain

```bash
dnsfilter-cli lists where example.com --agent
```

After a sync, shows every client policy that already allows or blocks the domain or a parent domain.

### Weekly silent-endpoint check

```bash
dnsfilter-cli agents stale --older-than 7d --agent
```

After a sync, roaming clients that have not checked in for a week or are unprotected, grouped by client.

### Work the unblock queue

```bash
dnsfilter-cli unblocks triage --agent
```

Pending unblock requests grouped by domain with threat flags and DNSFilter's already-covered check.

### Policy hygiene against a baseline

```bash
dnsfilter-cli policy-audit --org "Acme Dental" --baseline 1201 --agent
```

Threat-categorized allow-list entries, unblocked security categories, unknown-domain settings, and drift from policy 1201.

## Auth Setup

DNSFilter uses an API key sent as the raw value of the Authorization header (no Bearer prefix). Create one in the DNSFilter dashboard under Account Settings, API Tokens, then export DNSFILTER_API_KEY. A key created at the MSP organization sees every client organization under it. The REST API requires a paid plan.

Run `dnsfilter-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  dnsfilter-cli agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000 --agent --select count
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **No hidden prompts**  -  API writes and deletes run immediately; preview them with `--dry-run`. `allow` only writes with `--confirm`; `--yes` is only needed for local `profile delete` and cache purges
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `DNSFILTER_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `DNSFILTER_CONFIG_DIR`, `DNSFILTER_DATA_DIR`, `DNSFILTER_STATE_DIR`, `DNSFILTER_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `DNSFILTER_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `dnsfilter-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "dnsfilter": {
        "command": "dnsfilter-mcp",
        "env": {
          "DNSFILTER_HOME": "/srv/dnsfilter"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `DNSFILTER_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `DNSFILTER_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
dnsfilter-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "dnsfilter-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `dnsfilter-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `dnsfilter-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `dnsfilter-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
dnsfilter-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
dnsfilter-cli teach \
  --query "$QUERY" \
  --resource-type <type> \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
dnsfilter-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
dnsfilter-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`dnsfilter-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `DNSFILTER_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
dnsfilter-cli feedback "the --since flag is inclusive but docs say exclusive"
dnsfilter-cli feedback --stdin < notes.txt
dnsfilter-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `DNSFILTER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DNSFILTER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
dnsfilter-cli profile save briefing --json
dnsfilter-cli --profile briefing agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000
dnsfilter-cli profile list --json
dnsfilter-cli profile show briefing
dnsfilter-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `dnsfilter-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add dnsfilter-mcp --env DNSFILTER_API_KEY="$DNSFILTER_API_KEY" -- dnsfilter-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which dnsfilter-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   dnsfilter-cli why-blocked example.com --org 'Acme Dental' --agent
   ```
4. If ambiguous, drill into subcommand help: `dnsfilter-cli policies --help`.
