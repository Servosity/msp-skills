# The pain this closes

Cork gives an MSP a per-client cyber risk score, a compliance event feed, a
vulnerability inventory, and a cyber warranty status. All of it is per-client, and
all of it is right now. The questions an MSP actually asks are neither.

## "The score moved. Why?"

A client's risk score drops eleven points between one QBR and the next. Cork's score
history endpoint will hand back the four components (claims, compliance, coverage,
vulnerability) at each timestamp, but it never differences them, so the person
preparing the QBR is left exporting two snapshots into a spreadsheet and subtracting
by hand to find out which component moved. `cork-cli score attribute <client>`
does the subtraction and ranks the components by share of the move, which turns
"the score dropped" into "the score dropped because compliance moved, and here are
the overdue events behind it."

## "Which clients got worse this week?"

There is no cross-client aggregate endpoint in the Cork API at all. Answering
"who regressed" means an O(clients) fan-out with nothing stored to compare
yesterday against. In practice nobody does it, so regressions are found when a
client notices rather than when the data changed. `cork-cli score regressions`
answers it as one query against the local mirror.

## "What do we patch first?"

The vulnerability endpoint's server-side `sort_by` accepts only `sw_vendor` and
`sw_product`. It cannot sort by exploitability, so a technician ranking work by what
is actually being exploited has to pull the inventory and re-sort it themselves,
every time. `cork-cli vulnerabilities triage` orders by known-exploited status
first, then EPSS, then CVSS, and attaches a blast-radius device count so the top of
the list is the work that matters. When an advisory names a specific CVE, the
matching problem is worse: `cve_id` exists only nested inside a `cves[]` array and
no endpoint accepts a CVE filter at any page size, so "are we exposed to this one"
is unanswerable upstream. `cork-cli vulnerabilities exposure CVE-2023-21608`
answers it in one command.

## "Is that connector actually working?"

This is the quiet one. A Cork integration can report `connection_status: ok` while
its `last_synced_at` is days old. Both fields ship in the same payload and nothing
compares the timestamp to now, so the dashboard stays green while the data behind
it goes stale, and every downstream risk number silently drifts. Alert fatigue and
"green dashboards that lie" are a standing complaint in MSP communities for exactly
this reason. `cork-cli integrations health` flags the connector that is reporting
healthy while its data stopped moving, and names the clients it feeds.

The same class of gap shows up in device coverage: a connector reports devices, the
client record attributes devices, and the two lists are joined on
`associated_endpoints[].integration_identifier` nowhere in the product.
`cork-cli coverage gaps` diffs them, so an endpoint one tool sees and another
misses becomes visible instead of being discovered during an incident.

## "Which clients have no cyber warranty?"

Warranty state, client status, and score trend live on three different paths.
Ranking unprotected clients by how much risk they are actually carrying (the list
you want before a coverage conversation) means joining all three by hand.
`cork-cli warranties exposure` produces it directly.

## Why a local mirror

Every question above is cross-client, cross-time, or both, and the API is
structurally unable to answer any of them in a single call. Syncing Cork into a
local SQLite mirror turns the whole book of business into one queryable surface, so
the answers come from a local join instead of a fan-out that hammers an API which
publishes no rate limits.
