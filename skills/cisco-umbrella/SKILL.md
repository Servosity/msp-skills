---
name: cisco-umbrella
description: "Every Cisco Umbrella v2 API call, plus MSP child-org switching, domain verdicts, list diffs, and fleet health. Trigger phrases: `is this domain blocked in umbrella`, `umbrella fleet health across customers`, `add a domain to the umbrella blocklist`, `umbrella roaming computers offline`, `use cisco umbrella`, `run cisco-umbrella-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Cisco Umbrella"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cisco-umbrella-cli
    install:
      - kind: script
        bins: [cisco-umbrella-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1
---

# Cisco Umbrella Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `cisco-umbrella-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex
   ```
3. Verify: `cisco-umbrella-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `cisco-umbrella-cli` and `cisco-umbrella-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Covers deployments, policies, reports, Investigate, admin, and the provider and managed-provider consoles from Cisco's official OpenAPI. One parent console key acts on any customer org with --org. hygiene reads the local store (sync first); verdict, destination-lists diff, allowlist audit, fleet health, and report-card call the API live.

## When to Use This CLI

Use this CLI to inspect or change Cisco Umbrella deployments, destination lists, reports, Investigate data, and MSP customer orgs from a terminal or an agent. It fits MSP fleet sweeps across child orgs, domain triage, and reconciling block or allow lists against files.

## Anti-triggers

Do not use this CLI for:
- Do not use it for Cisco Secure Access tenants on api.sse.cisco.com; that is a separate API.
- Do not use it to configure Umbrella SIG tunnels on the network device side; it only manages the Umbrella records.
- Do not use it for legacy Umbrella v1 management or enforcement API keys.

## Unique Capabilities

### Triage and policy
- **`verdict`**  -  One answer for a domain: category, risk score, security status, every destination list that blocks or allows it (including parent-domain entries), and who resolved it recently.

  _Reach for this when a ticket or IOC asks 'is this domain blocked, how risky is it, and who hit it'. Needs the Policies, Investigate, and Reports scopes; a 403 skips that section, so check lists_checked, investigate_checked, and notes before calling a domain clean._

  ```bash
  cisco-umbrella-cli verdict login.example.com --agent
  ```
- **`destination-lists diff`**  -  Reconcile a destination list against a source-of-truth file: see what to add and remove, then apply it in API-sized chunks. --apply also removes entries absent from the file unless --no-remove.

  _Use this before any bulk block/allow change so the push is a reviewed diff, not a blind overwrite. It prints the plan unless --apply is given._

  ```bash
  cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent
  ```
- **`allowlist audit`**  -  Find allow-list entries that are now categorized as malicious, entries that conflict with a block list, and allows added in a recent window.

  _Catches allow-list rot that no dashboard view surfaces. Needs the Policies and Investigate scopes; categorization_checked is false when the Investigate check was skipped._

  ```bash
  cisco-umbrella-cli allowlist audit --since 7d --agent
  ```

### MSP fleet operations
- **`fleet health`**  -  One row per managed customer: seats, an issues count, and counts of off, uninstalled, stale, and SWG-unprotected roaming computers, unhealthy virtual appliances, and down tunnels. Customers that fail are listed under fetch_failures.

  _The MSP weekly sweep in one call. Run it from an MSP, Multi-org, or provider parent console key with the admin customers scope, without --org. It mints one child-org token per customer, starting customers 3s apart; a 403 section is listed under the row's skipped field._

  ```bash
  cisco-umbrella-cli fleet health --stale-days 7 --agent
  ```
- **`hygiene`**  -  Device-level findings in one org: stale or offline roaming computers, unhealthy virtual appliances, down tunnels, and unverified networks, each tagged with a kind.

  _Use after fleet health flags a customer, or for a weekly hygiene pass. It reads the local store: sync roaming_computers, virtual_appliances, tunnels-tunnels-state, and networks first (with the same --org); an empty store returns no findings plus a note._

  ```bash
  cisco-umbrella-cli hygiene --stale-days 14 --agent
  ```
- **`report-card`**  -  A customer's security summary, top threat types, blocked categories, and deployment status for a window, compared with the prior window. --csv emits the summary rows only.

  _QBR-ready numbers with a trend in one command. From a parent console key pass --org <customerId>; without it the card covers the key's own org._

  ```bash
  cisco-umbrella-cli report-card --org 8675309 --from -30days --agent
  ```

## Command Reference

**activity**  -  Manage activity

- `cisco-umbrella-cli activity get-activities`  -  List all activities (dns/proxy/firewall/intrusion) within the timeframe.
- `cisco-umbrella-cli activity get-amp-retrospective`  -  List all AMP retrospective activity within the timeframe. **Access Scope:** Reports > Granular Events > Read-Only
- `cisco-umbrella-cli activity get-dns`  -  List all DNS entries within the timeframe. **Access Scope:** Reports > Granular Events > Read-Only
- `cisco-umbrella-cli activity get-firewall`  -  List all firewall activity within the timeframe. **Access Scope:** Reports > Granular Events > Read-Only
- `cisco-umbrella-cli activity get-intrusion`  -  List all Intrusion Prevention System (IPS) activity within the timeframe.
- `cisco-umbrella-cli activity get-ip`  -  (Deprecated) List all IP activity within the timeframe. **Access Scope:** Reports > Granular Events > Read-Only
- `cisco-umbrella-cli activity get-proxy`  -  List all proxy entries within the timeframe. **Access Scope:** Reports > Granular Events > Read-Only

**api_keys**  -  Manage api keys

- `cisco-umbrella-cli api-keys create`  -  Create an API key with certain scopes, name, description, allowed IP addresses and CIDR blocks, and expiration. Refused here: Umbrella shows the new secret only once and this program withholds it, so do this in the Umbrella dashboard.
- `cisco-umbrella-cli api-keys delete`  -  Delete an API key.
- `cisco-umbrella-cli api-keys get`  -  Get an API key.
- `cisco-umbrella-cli api-keys list`  -  List the API keys created by your organization.
- `cisco-umbrella-cli api-keys refresh`  -  Refresh the API key and secret. Refused here: Umbrella shows the new secret only once and this program withholds it, so do this in the Umbrella dashboard.
- `cisco-umbrella-cli api-keys update`  -  Update the name, description, scopes, and allowed IPs for an API key.

**api_usage**  -  Manage api usage

- `cisco-umbrella-cli api-usage get-keys`  -  Get the API key usage information, including the total number of API requests within a specific time period.
- `cisco-umbrella-cli api-usage get-requests`  -  Get the information about the API requests for the organization within a specific time period
- `cisco-umbrella-cli api-usage get-responses`  -  Get the information about the API responses for the organization within a specific time period
- `cisco-umbrella-cli api-usage get-summary`  -  Get the total number API requests

**app_discovery**  -  Manage app discovery

- `cisco-umbrella-cli app-discovery get-application`  -  Get an application by ID.
- `cisco-umbrella-cli app-discovery get-application-attributes`  -  List all attributes for the application.
- `cisco-umbrella-cli app-discovery get-application-identities`  -  List all identities for the application.
- `cisco-umbrella-cli app-discovery get-application-risk`  -  Get the risk for the application.
- `cisco-umbrella-cli app-discovery get-applications-info`  -  List the information about the applications.
- `cisco-umbrella-cli app-discovery get-protocol`  -  Get protocol by ID.
- `cisco-umbrella-cli app-discovery get-protocol-identities`  -  List identities for a specific protocol.
- `cisco-umbrella-cli app-discovery list-application-categories`  -  List all application categories.
- `cisco-umbrella-cli app-discovery list-applications`  -  List all discovered applications.
- `cisco-umbrella-cli app-discovery list-protocols`  -  List all protocols.
- `cisco-umbrella-cli app-discovery patch-applications`  -  Update the labels for the applications.
- `cisco-umbrella-cli app-discovery update-application`  -  Update the label for the application. Provide an application label in the request body.

**application_lists**  -  Manage application lists

- `cisco-umbrella-cli application-lists create`  -  Create an application list for the organization.
- `cisco-umbrella-cli application-lists delete`  -  Delete the application list in the organization.
- `cisco-umbrella-cli application-lists get`  -  Get the properties of the application list.
- `cisco-umbrella-cli application-lists get-applications-usage`  -  Get the list of application usage information for the application lists.
- `cisco-umbrella-cli application-lists list`  -  Get the application lists and the number of application lists in the organization.
- `cisco-umbrella-cli application-lists update`  -  Update the properties of the application list.

**console_config**  -  Manage console config

- `cisco-umbrella-cli console-config create-cname`  -  Add the cname information for the service providers console.
- `cisco-umbrella-cli console-config create-contact`  -  Create a contact for the service providers console.
- `cisco-umbrella-cli console-config create-logo`  -  Create a logo for the service providers console.
- `cisco-umbrella-cli console-config delete-cname`  -  Delete a cname for the service providers console.
- `cisco-umbrella-cli console-config delete-contact`  -  Delete a contact for the service providers console.
- `cisco-umbrella-cli console-config delete-logo`  -  Delete a logo for the service providers console.
- `cisco-umbrella-cli console-config get-cname`  -  Get the cname information for the service providers console.
- `cisco-umbrella-cli console-config get-cnames`  -  List the cname information for the service providers console.
- `cisco-umbrella-cli console-config get-contact`  -  Get a contact for the service providers console.
- `cisco-umbrella-cli console-config get-contacts`  -  List the contacts for the service providers console.
- `cisco-umbrella-cli console-config get-logo`  -  Get a logo for the service providers console.
- `cisco-umbrella-cli console-config get-logos`  -  Get the logos for the service providers console.
- `cisco-umbrella-cli console-config update-cname`  -  Update the cname information for the service providers console.
- `cisco-umbrella-cli console-config update-contact`  -  Update a contact for service providers console.
- `cisco-umbrella-cli console-config update-logo`  -  Update a logo for the service providers console.

**datacenters**  -  Manage datacenters

- `cisco-umbrella-cli datacenters`  -  List the information about the IPsec-enabled data centers.

**destination_lists**  -  Manage destination lists

- `cisco-umbrella-cli destination-lists add-destinations`  -  Add destinations to a destination list.
- `cisco-umbrella-cli destination-lists create`  -  Create a destination list in your organization.
- `cisco-umbrella-cli destination-lists delete`  -  Delete a destination list from your organization.
- `cisco-umbrella-cli destination-lists get`  -  Get a destination list.
- `cisco-umbrella-cli destination-lists list`  -  Get the destination lists in your organization.
- `cisco-umbrella-cli destination-lists list-destinations`  -  Get destinations in a destination list.
- `cisco-umbrella-cli destination-lists remove-destinations`  -  Remove destinations from the destination list. **Note:** Accepts a list that contains no more than 500 destination IDs.
- `cisco-umbrella-cli destination-lists update`  -  Update a destination list in your organization.

**internal_domains**  -  Manage internal domains

- `cisco-umbrella-cli internal-domains create`  -  Create an internal domain.
- `cisco-umbrella-cli internal-domains delete`  -  Delete an internal domain.
- `cisco-umbrella-cli internal-domains get`  -  Get an internal domain.
- `cisco-umbrella-cli internal-domains list`  -  List the internal domains.
- `cisco-umbrella-cli internal-domains update`  -  Update an internal domain.

**internal_networks**  -  Manage internal networks

- `cisco-umbrella-cli internal-networks create`  -  Create an internal network.
- `cisco-umbrella-cli internal-networks delete`  -  Delete an internal network.
- `cisco-umbrella-cli internal-networks get`  -  Get an internal network.
- `cisco-umbrella-cli internal-networks list`  -  List the internal networks.
- `cisco-umbrella-cli internal-networks list-policies`  -  List the policies for an internal network.
- `cisco-umbrella-cli internal-networks update`  -  Update an internal network.

**investigate**  -  Manage investigate

- `cisco-umbrella-cli investigate get-bgp-routes-asn`  -  A response to a valid ASN returns an array of hash references. Each hash reference contains two keys: `geo` and `cidr`.
- `cisco-umbrella-cli investigate get-bgp-routes-ip`  -  This endpoint provides data about ASN and IP relationships
- `cisco-umbrella-cli investigate get-domain-volume`  -  List the query volume for a domain over the last 30 days.
- `cisco-umbrella-cli investigate get-domains-categorization`  -  Look up the status, and security and content category IDs for the domain.
- `cisco-umbrella-cli investigate get-domains-risk-score`  -  The Investigate Risk Score is based on an analysis of the lexical characteristics of the domain name and patterns in
- `cisco-umbrella-cli investigate get-links-name`  -  List the domain names that are frequently requested around the same time (up to 60 seconds before or after)
- `cisco-umbrella-cli investigate get-passive-dnsdomain`  -  Get the Resource Record (RR) data for DNS responses, and categorization data, where the answer (or rdata)
- `cisco-umbrella-cli investigate get-passive-dnsip`  -  Get the Resource Record (RR) data for DNS responses, and categorization data, where the answer (or rdata)
- `cisco-umbrella-cli investigate get-passive-dnsname`  -  The Passive DNS endpoint provides historical data from the Umbrella resolvers for domains, IPs
- `cisco-umbrella-cli investigate get-passive-dnsraw`  -  Get the Resource Record (RR) data for DNS responses, and categorization data, where the answer (or rdata)
- `cisco-umbrella-cli investigate get-recommendations-name`  -  List the co-occurences for the specified domain.
- `cisco-umbrella-cli investigate get-sample-hash`  -  Gather the information from the /samples endpoint
- `cisco-umbrella-cli investigate get-sample-hash-artifacts`  -  Other samples associated with this sample. The sample data does not include a threat score.
- `cisco-umbrella-cli investigate get-sample-hash-behaviors`  -  Get the information about specific actions or unique properties of this sample
- `cisco-umbrella-cli investigate get-sample-hash-connections`  -  Information about network activity associated with this sample, such as connections to other domains or IPs.
- `cisco-umbrella-cli investigate get-samples-destination`  -  Specify a domain, IP, or URL. Use the destination to search for all samples associated with the destination.
- `cisco-umbrella-cli investigate get-search`  -  List the newly seen domains that match a regular expression pattern.
- `cisco-umbrella-cli investigate get-security-name`  -  List multiple scores or security features for a domain.
- `cisco-umbrella-cli investigate get-sub-domains`  -  Get the subdomains of a given domain. If there is no subdomain for the domain, Investigate returns an empty array.
- `cisco-umbrella-cli investigate get-timeline`  -  List the historical tagging timeline for a given IP, domain, or URL.
- `cisco-umbrella-cli investigate get-topmillion`  -  List the most seen domains in Umbrella.
- `cisco-umbrella-cli investigate get-whois`  -  Get the WHOIS information for the specified email addresses, nameservers, and domains.
- `cisco-umbrella-cli investigate get-whois-email`  -  Get the email address or addresses of the registrar for the domain or domains.
- `cisco-umbrella-cli investigate get-whois-history`  -  Get a standard WHOIS response record for a single domain with available historical WHOIS data returned in an object.
- `cisco-umbrella-cli investigate get-whois-nameserver`  -  Get WHOIS information for the nameserver.
- `cisco-umbrella-cli investigate get-whois-nameservers`  -  Get WHOIS information for the nameservers.
- `cisco-umbrella-cli investigate get-whois-search`  -  Performs a regular expression (RegEx) search on the WHOIS data (domain, nameserver, and email fields)
- `cisco-umbrella-cli investigate post-domains-categorization`  -  Provide a list of domains and look up the status, and security and content category IDs for each domain.

**managed_customers**  -  Manage managed customers

- `cisco-umbrella-cli managed-customers create`  -  Create a customer for a managed provider.
- `cisco-umbrella-cli managed-customers delete`  -  Delete an customer for the managed provider.
- `cisco-umbrella-cli managed-customers get`  -  Get a customer for the managed provider.
- `cisco-umbrella-cli managed-customers list`  -  List the customers for the managed provider.
- `cisco-umbrella-cli managed-customers update`  -  Update a customer for a managed provider.

**network_devices**  -  Manage network devices

- `cisco-umbrella-cli network-devices create`  -  Create a network device.
- `cisco-umbrella-cli network-devices delete`  -  Remove a network device.
- `cisco-umbrella-cli network-devices get`  -  Get a network device.
- `cisco-umbrella-cli network-devices list`  -  List the network devices.
- `cisco-umbrella-cli network-devices list-policies`  -  List the Umbrella policies associated with a network device.
- `cisco-umbrella-cli network-devices update`  -  Update a network device.

**networks**  -  Manage networks

- `cisco-umbrella-cli networks create`  -  Create a network. Before you can create a network, contact Support to get your IP range verified.
- `cisco-umbrella-cli networks delete`  -  Delete a network.
- `cisco-umbrella-cli networks get`  -  Get a Network.
- `cisco-umbrella-cli networks list`  -  List the Networks.
- `cisco-umbrella-cli networks list-policies`  -  List the policies for a network.
- `cisco-umbrella-cli networks update`  -  Update a Network. Before you can update the network's IP address, contact Support to get your IP range verified.

**organizations**  -  Manage organizations

- `cisco-umbrella-cli organizations`  -  Get information about your provider organizations.

**password_resets**  -  Manage password resets

- `cisco-umbrella-cli password-resets <customerId>`  -  Renew the passwords for the user accounts in the child (customer) organization of a provider organization.

**policies**  -  Manage policies

- `cisco-umbrella-cli policies delete-policy-identities`  -  Remove an identity from an Umbrella policy. Policy changes may require up to 20 minutes to take effect globally.
- `cisco-umbrella-cli policies list`  -  List the Umbrella policies. You can filter by policy type.
- `cisco-umbrella-cli policies update-policy-identities`  -  Add an identity to your policy. Policy changes may require up to 20 minutes to take effect globally.

**provider_customers**  -  Manage provider customers

- `cisco-umbrella-cli provider-customers convert-trial-customer`  -  Convert your Managed Services License Agreement (MSLA) trial to an MSLA customer.
- `cisco-umbrella-cli provider-customers create`  -  Create a customer for a provider.
- `cisco-umbrella-cli provider-customers create-customer-access-request`  -  Create the access request for the customer's organization.
- `cisco-umbrella-cli provider-customers create-customer-trial-extensions`  -  Create the extension for the customer's trial.
- `cisco-umbrella-cli provider-customers delete`  -  Delete a customer for a provider.
- `cisco-umbrella-cli provider-customers get`  -  Get a customer for the provider.
- `cisco-umbrella-cli provider-customers get-customer-access-request`  -  Get the access request details for the customer's organization.
- `cisco-umbrella-cli provider-customers get-customer-addresses`  -  Get the customer addresses for provider.
- `cisco-umbrella-cli provider-customers get-customer-deals`  -  Get the information about deals available to customer.
- `cisco-umbrella-cli provider-customers get-customer-packages`  -  Get the packages for the trial customer.
- `cisco-umbrella-cli provider-customers get-customer-subscription-details`  -  Get the subscription details for the customer's organization.
- `cisco-umbrella-cli provider-customers get-customer-trial-strength`  -  Get the strength of a customer trial for a provider.
- `cisco-umbrella-cli provider-customers list`  -  List the customers for the provider.
- `cisco-umbrella-cli provider-customers update`  -  Update a customer for a provider.
- `cisco-umbrella-cli provider-customers update-customer-access-request`  -  Update the access request for the customer's organization.
- `cisco-umbrella-cli provider-customers update-customer-deals`  -  Update Customer Deals by ID

**provider_reports**  -  Manage provider reports

- `cisco-umbrella-cli provider-reports create-customer-security-report-request`  -  Create security report request for customer of service provider.
- `cisco-umbrella-cli provider-reports get-deployment-status-by-organization`  -  List the summary counts of deployment status for the organization within the timeframe.
- `cisco-umbrella-cli provider-reports get-provider-categories`  -  List the Provider categories **Access Scope:** Reports > Customer > Read-Only
- `cisco-umbrella-cli provider-reports get-provider-category-requests-by-organization`  -  List the summary counts of all requests for each category within the timeframe.
- `cisco-umbrella-cli provider-reports get-provider-requests-by-category`  -  List the summary counts of all requests within the timeframe. **Access Scope:** Reports > Customer > Read-Only
- `cisco-umbrella-cli provider-reports get-provider-requests-by-destination`  -  List the summary counts of all requests within the timeframe. **Access Scope:** Reports > Customer > Read-Only
- `cisco-umbrella-cli provider-reports get-provider-requests-by-hour`  -  List the activity volume within the timeframe. **Access Scope:** Reports > Customer > Read-Only
- `cisco-umbrella-cli provider-reports get-provider-requests-by-organization`  -  List the summary counts of all requests within the timeframe. **Access Scope:** Reports > Customer > Read-Only
- `cisco-umbrella-cli provider-reports get-provider-requests-by-timerange`  -  List the activity volume within the timeframe. **Access Scope:** Reports > Customer > Read-Only
- `cisco-umbrella-cli provider-reports get-providers-console-data`  -  Get the data available from the provider's console.
- `cisco-umbrella-cli provider-reports get-service-provider-customer-security-report`  -  Get the security report for the customer of the service provider.

**reports**  -  Manage reports

- `cisco-umbrella-cli reports get-applications`  -  List the applications. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-bandwidth-by-hour`  -  List the bandwidth in bytes within the timeframe. Only returns proxy data.
- `cisco-umbrella-cli reports get-bandwidth-by-timerange`  -  List the bandwidth in bytes within the timeframe. Only returns proxy data.
- `cisco-umbrella-cli reports get-categories`  -  List the categories. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-deployment-status`  -  List the deployment status within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-identities`  -  List the identities. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-identity`  -  Get identity by identity ID. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-identity-distribution`  -  List the number of requests by identity types. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-identity-distribution-type`  -  List the number of requests by identity for the type of traffic. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-organization-requests-by-hour`  -  List the activity volume within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-requests-by-hour`  -  List the activity volume within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-requests-by-hour-and-category`  -  List the activity volume within the timeframe by type of category. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-requests-by-hour-and-category-and-type`  -  List the activity volume for the type of category within the timeframe.
- `cisco-umbrella-cli reports get-requests-by-timerange`  -  List the activity volume within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-requests-by-timerange-and-category`  -  List the activity volume within the timeframe by category. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-requests-by-timerange-and-category-type`  -  List the activity volume within the timeframe by category. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-requests-by-timerange-type`  -  List the activity volume within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-summaries-by-category`  -  List the summaries of requests by category. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-summaries-by-category-type`  -  List the summaries by category for the type of request. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-summaries-by-destination`  -  List the summaries by destination. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-summaries-by-destination-type`  -  List the summaries by destination for the type of traffic. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-summaries-by-rule-intrusion`  -  List the summaries by rule for the intrusion type. **Access Scope:** Reports > Summaries by rule > Read-Only
- `cisco-umbrella-cli reports get-summary`  -  Get the summary report. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-summary-by-type`  -  Get the summary of requests by the traffic type. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-threat-name`  -  Get the threat name by threat ID. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-threat-names`  -  List the threat names. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-threat-type`  -  List the threat types by threat ID. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-threat-types`  -  List the threat types. **Access Scope:** Reports > Utilities > Read-Only
- `cisco-umbrella-cli reports get-top-categories`  -  List the categories that received the greatest number of requests. Order the number of requests in descending order.
- `cisco-umbrella-cli reports get-top-categories-type`  -  List the categories for the type of traffic that received the greatest number of requests.
- `cisco-umbrella-cli reports get-top-destinations`  -  List the destinations by the number of requests made to this destination. Return the results in descending order.
- `cisco-umbrella-cli reports get-top-destinations-type`  -  List the destinations by type of destination and the number of requests made to this destination.
- `cisco-umbrella-cli reports get-top-dnsquery-types`  -  List the top types of DNS query. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-top-event-types`  -  List the top event types by the number of requests made for each type of event.
- `cisco-umbrella-cli reports get-top-files-proxy`  -  List the top files within the timeframe. Only returns proxy data. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-top-identities`  -  List the identities by the number of requests made, sorted in descending order.
- `cisco-umbrella-cli reports get-top-identities-type`  -  List the identities for the specific traffic type by the number of requests. Sort the results in descending order.
- `cisco-umbrella-cli reports get-top-ips`  -  List the top IP addresses. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-top-ips-internal`  -  List the top internal IP addresses. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-top-threat-types`  -  List the top types of threat within the timeframe. Returns both DNS and proxy data.
- `cisco-umbrella-cli reports get-top-threat-types-by-type`  -  List the top threat-types within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-top-threats`  -  Get the top threats within the timeframe. Returns both DNS and proxy data.
- `cisco-umbrella-cli reports get-top-threats-by-type`  -  Get the top threats within the timeframe. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-top-urls`  -  List the top number of URLs that are requested for a certain domain.
- `cisco-umbrella-cli reports get-total-requests`  -  Get the count of the total requests. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports get-total-requests-by-type`  -  Get the count of the total requests for the request type. **Access Scope:** Reports > Aggregations > Read-Only
- `cisco-umbrella-cli reports post-identities`  -  Get the identities information by providing a list of identity IDs in the request body.

**roaming_computers**  -  Manage roaming computers

- `cisco-umbrella-cli roaming-computers delete`  -  Delete a roaming computer in the organization.
- `cisco-umbrella-cli roaming-computers get`  -  Get a roaming computer in the organization.
- `cisco-umbrella-cli roaming-computers get-organization-info`  -  Get the OrgInfo.json properties for deploying the Cisco Secure Client on user devices in the organization.
- `cisco-umbrella-cli roaming-computers list`  -  List the roaming computers.
- `cisco-umbrella-cli roaming-computers update`  -  Update a roaming computer in the organization.

**roles**  -  Manage roles

- `cisco-umbrella-cli roles`  -  List the roles in the organization.

**s3_bucket_keys**  -  Manage s3 bucket keys

- `cisco-umbrella-cli s3-bucket-keys`  -  Rotate the Cisco-managed S3 bucket key for the organization. Refused here: Umbrella shows the new secret only once and this program withholds it, so do this in the Umbrella dashboard.

**sites**  -  Manage sites

- `cisco-umbrella-cli sites create`  -  Create a site.
- `cisco-umbrella-cli sites delete`  -  Delete a site.
- `cisco-umbrella-cli sites get`  -  Get a site in the organization.
- `cisco-umbrella-cli sites list`  -  List the sites in the organization.
- `cisco-umbrella-cli sites update`  -  Update a site.

**swg_device_settings**  -  Manage swg device settings

- `cisco-umbrella-cli swg-device-settings list`  -  List the Secure Web Gateway (SWG) override setting for devices in the organization.
- `cisco-umbrella-cli swg-device-settings remove`  -  Remove the Secure Web Gateway (SWG) override setting for the devices in the organization.
- `cisco-umbrella-cli swg-device-settings set`  -  Add a list of origin ID and the Secure Web Gateway setting for devices in the organization.

**tags**  -  Manage tags

- `cisco-umbrella-cli tags add-to-devices`  -  For the tag ID, add the tag to the roaming computers.
- `cisco-umbrella-cli tags create`  -  Add a tag to the organization.
- `cisco-umbrella-cli tags list`  -  List the tags in the organization.
- `cisco-umbrella-cli tags list-devices`  -  List the roaming computers that have a tag with the specified tag ID.
- `cisco-umbrella-cli tags remove-from-devices`  -  For the tag ID, remove the tag on the roaming computers in the organization.

**tunnels**  -  Manage tunnels

- `cisco-umbrella-cli tunnels add`  -  Add a new tunnel to the organization. Refused here unless you choose the secret yourself: Umbrella shows a generated one only once and this program withholds it, so otherwise do this in the Umbrella dashboard.
- `cisco-umbrella-cli tunnels delete`  -  Delete a tunnel in the organization.
- `cisco-umbrella-cli tunnels get`  -  Get a specific tunnel.
- `cisco-umbrella-cli tunnels get-error-events`  -  Get the recent tunnel error events.
- `cisco-umbrella-cli tunnels get-global-error-events`  -  Get the recent global error events.
- `cisco-umbrella-cli tunnels get-org-state`  -  List the tunnel state information.
- `cisco-umbrella-cli tunnels get-policies`  -  List the policies that include the network tunnel.
- `cisco-umbrella-cli tunnels get-state`  -  Get the tunnel state information.
- `cisco-umbrella-cli tunnels list`  -  List the tunnels for an organization.
- `cisco-umbrella-cli tunnels update`  -  Update the `name`, `siteOriginId`, `networkCIDRs`, and client `deviceType` properties for a tunnel.
- `cisco-umbrella-cli tunnels update-credentials`  -  Update and rotate the tunnel credentials. Refused here unless you choose the secret yourself: Umbrella shows a generated one only once and this program withholds it, so otherwise do this in the Umbrella dashboard.

**users**  -  Manage users

- `cisco-umbrella-cli users create`  -  Create an Umbrella user account with a designated role.
- `cisco-umbrella-cli users delete`  -  Delete a user account.
- `cisco-umbrella-cli users get`  -  Get a user account.
- `cisco-umbrella-cli users list`  -  List the user accounts in the organization.

**virtual_appliances**  -  Manage virtual appliances

- `cisco-umbrella-cli virtual-appliances delete`  -  Delete a virtual appliance in the organization.
- `cisco-umbrella-cli virtual-appliances get`  -  Get a virtual appliance in the organization.
- `cisco-umbrella-cli virtual-appliances list`  -  List the virtual appliances in the organization.
- `cisco-umbrella-cli virtual-appliances update`  -  Update a virtual appliance in the organization.


**allowlist**


- `cisco-umbrella-cli allowlist audit`  -  Find allow-list entries that are now categorized as malicious, entries that conflict with a block list, and allows added in a recent window.


**destination-lists**


- `cisco-umbrella-cli destination-lists diff`  -  Reconcile a destination list against a source-of-truth file: see what to add and remove, then apply it in API-sized chunks. --apply also removes entries absent from the file unless --no-remove.


**fleet**


- `cisco-umbrella-cli fleet health`  -  One row per managed customer: seats, an issues count, and counts of off, uninstalled, stale, and SWG-unprotected roaming computers, unhealthy virtual appliances, and down tunnels. Customers that fail are listed under fetch_failures.


**hygiene**


- `cisco-umbrella-cli hygiene`  -  Device-level findings in one org: stale or offline roaming computers, unhealthy virtual appliances, down tunnels, and unverified networks, each tagged with a kind.


**report-card**


- `cisco-umbrella-cli report-card`  -  A customer's security summary, top threat types, blocked categories, and deployment status for a window, compared with the prior window. --csv emits the summary rows only.


**verdict**


- `cisco-umbrella-cli verdict`  -  One answer for a domain: category, risk score, security status, every destination list that blocks or allows it (including parent-domain entries), and who resolved it recently.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cisco-umbrella-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Triage a domain from a ticket

```bash
cisco-umbrella-cli verdict login.example.com --agent
```

Category, risk score, matching destination lists, and recent resolvers in one JSON document.

### Block one domain

```bash
cisco-umbrella-cli destination-lists add-destinations 1234567 --body-json '[{"destination":"bad.example.com","comment":"phishing ticket"}]'
```

Adds one entry to a block list (list IDs come from destination-lists list); use diff --no-remove --apply for bulk adds.

### Narrow a roaming computer list

```bash
cisco-umbrella-cli roaming-computers list --agent --select name,status,lastSync
```

Keeps only the fields an agent needs from a large device list.

### Weekly MSP sweep

```bash
cisco-umbrella-cli fleet health --stale-days 7 --agent
```

One row per customer with an issues count; customers that fail land under fetch_failures. Use a parent console key and no --org.

### Reconcile a blocklist file

```bash
cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent
```

Prints the add/remove plan. Rerun with --apply to make it; --apply also removes entries absent from the file unless --no-remove.

## Auth Setup

Create an Umbrella API key in the dashboard (Admin > API Keys, or Console Settings > API Keys in an MSP or Multi-org console) and set CISCO_UMBRELLA_API_KEY and CISCO_UMBRELLA_API_SECRET. The CLI exchanges them for a one-hour bearer token at https://api.umbrella.com/auth/v2/token. To act on one customer from a parent console key, set CISCO_UMBRELLA_ORG_ID (or pass --org); the token request then carries X-Umbrella-OrgId and is scoped to that child org. Each key only reaches the scopes it was created with; a missing scope returns 403.

Run `cisco-umbrella-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  cisco-umbrella-cli api-keys list --agent --select keys,limit,message
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
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

- Use `--home <dir>` for one invocation, or set `CISCO_UMBRELLA_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CISCO_UMBRELLA_CONFIG_DIR`, `CISCO_UMBRELLA_DATA_DIR`, `CISCO_UMBRELLA_STATE_DIR`, `CISCO_UMBRELLA_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CISCO_UMBRELLA_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Any API call with environment credentials saves the key, the secret and the bearer token there (owner-only, not encrypted); later runs use them after the variables are unset, until `auth logout` deletes the file. A key named on the run itself (`CISCO_UMBRELLA_API_KEY`, or the config file the run loads) wins over a different key, bearer or `auth set-token` token saved by an earlier run, so naming another customer's key reaches that customer. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `cisco-umbrella-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "cisco-umbrella": {
        "command": "cisco-umbrella-mcp",
        "env": {
          "CISCO_UMBRELLA_HOME": "/srv/cisco-umbrella"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CISCO_UMBRELLA_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CISCO_UMBRELLA_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; API responses are not cached. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `CISCO_UMBRELLA_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `CISCO_UMBRELLA_DATA_SOURCE` sets the `--data-source` default.

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
cisco-umbrella-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope (under `--agent` it is wrapped: read `.results.found` and `.results.results`):

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
      "next_action": ["<trial command>", "cisco-umbrella-cli learnings confirm 12"] }
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
       materially more, record the divergence via `cisco-umbrella-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `cisco-umbrella-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "laptop" teach satisfying a "roaming computer" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., one alias seeded under two canonical identity types). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `cisco-umbrella-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
cisco-umbrella-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "laptop") satisfies future queries under another alias (e.g., "roaming computer", "endpoint") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
cisco-umbrella-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
cisco-umbrella-cli teach-playbook \
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
cisco-umbrella-cli playbook amend \
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

`cisco-umbrella-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CISCO_UMBRELLA_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cisco-umbrella-cli feedback "the --since flag is inclusive but docs say exclusive"
cisco-umbrella-cli feedback --stdin < notes.txt
cisco-umbrella-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CISCO_UMBRELLA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CISCO_UMBRELLA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
cisco-umbrella-cli profile save briefing --json
cisco-umbrella-cli --profile briefing api-keys list
cisco-umbrella-cli profile list --json
cisco-umbrella-cli profile show briefing
cisco-umbrella-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `cisco-umbrella-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add cisco-umbrella-mcp --env CISCO_UMBRELLA_API_KEY=<key> --env CISCO_UMBRELLA_API_SECRET=<secret> -- cisco-umbrella-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `cisco-umbrella-mcp --transport http` requires `CISCO_UMBRELLA_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), listens at `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `CISCO_UMBRELLA_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `CISCO_UMBRELLA_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which cisco-umbrella-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   cisco-umbrella-cli verdict login.example.com --agent
   ```
4. If ambiguous, drill into subcommand help, e.g. `cisco-umbrella-cli destination-lists --help`.
