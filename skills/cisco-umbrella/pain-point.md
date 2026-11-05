# cisco-umbrella skill - the MSP pain it closes

## The pain

"Is every client actually protected?" is the question behind every Umbrella renewal and
QBR. In an Umbrella MSP or Multi-org console the answer lives inside each child org.
Cisco describes the console this way: "All the child-orgs will be listed which makes it
easy to switch in and out between customers to make any changes at any time"
([Cisco Umbrella for Managed Services](https://blogs.cisco.com/partner/cisco-umbrella-for-managed-services),
Cisco partner blog, December 2022). Switching in is easy; doing it for every customer to
find roaming computers that are off, uninstalled or not checking in, an unhealthy virtual
appliance or a tunnel that is down, is a morning of clicking.

The same shape repeats in incident triage. "Is this domain blocked for that client, and who
hit it?" takes three places: Investigate for the category and risk score, every
destination list (including entries for a parent domain) for membership, and an activity
report for who resolved it. And a destination list kept in step with a blocklist you
maintain elsewhere is a manual diff, while an allow-list entry that Umbrella has since
categorized as malicious stays allowed until someone notices.

## What this skill does about it

- `cisco-umbrella-cli fleet health` - one row per managed customer: seats, roaming
  computers off, uninstalled, stale or without web protection, virtual appliances not
  healthy and tunnels down, from a parent console key.
- `cisco-umbrella-cli hygiene` - every deployment problem inside one customer as a typed
  finding, from the local store.
- `cisco-umbrella-cli verdict <domain>` - category, risk score, security status, the lists
  that block or allow it and who resolved it, in one answer.
- `cisco-umbrella-cli allowlist audit` - allow-list entries now categorized as malicious,
  allow and block conflicts, and recent allows.
- `cisco-umbrella-cli destination-lists diff <id> --file <blocklist>` - the plan to make a
  list match your file; `--apply` makes the changes.
- `cisco-umbrella-cli report-card --org <child id>` - a customer's security summary
  against the prior period, ready for the QBR.

Every command takes `--org <child org ID>` to act inside one customer from the parent
console key.

## Status

Beta. Validated against the Cisco Umbrella API surface (Cisco's published OpenAPI
documents for the v2 API); no live tenant has run it yet. The closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked separately
and added here as `video.md` once it exists.
