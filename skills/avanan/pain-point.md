# The pain this closes

Avanan (Check Point Harmony Email and Collaboration) already has integrations for
Cortex XSOAR, Microsoft Sentinel, n8n, and two community MCP servers. Every one of
them is a plugin that lives inside another product, and every one of them is
stateless: one question, one API call, no history. There is no terminal-native
tool, nothing that works offline, and nothing that can answer a question spanning
two tenants or two days without paying for N more calls against an API that
returns HTTP 429 and publishes no quota.

## "What did we catch this morning?"

The event query endpoint takes a POST body of filters and pages through an opaque
`scrollId`. It hands back detections; it does not bucket them. So the analyst
starting a shift runs the same query they ran an hour ago, scrolls it again, and
tries to remember which rows they already dealt with. `avanan-cli triage --since 24h`
buckets the window by threat type, severity, and state, and names the sender
domains driving the volume, so the shift starts with a digest instead of a scroll.

## "Is this one campaign or forty problems?"

Forty detections land in an hour. Whether that is one phishing run against forty
recipients or forty unrelated things changes the response entirely, and nothing in
the API groups them. Working it out means exporting detections and sorting by
sender and subject in a spreadsheet. `avanan-cli campaign` clusters by sender
domain and normalized subject, shows the recipient spread, and counts how many of
each cluster are still un-remediated.

## "You quarantined a legitimate email. Prove it."

A user disputes a quarantine, or a ticket needs evidence. The detection, the state
changes, the action that was submitted, the async task's outcome, and the restore
disposition are five separate lookups across three endpoints, and the task record
ages out. `avanan-cli timeline <entity-id>` reconstructs the history of one message in order
from the local mirror, after the fact.

## "Is this domain already allowlisted?"

This is the quiet one. Avanan spreads allow and block exceptions across seven
security engines - anti-phishing, spam, anomaly, click-time protection,
anti-malware, URL reputation, and DLP - each with its own path shape, its own ID
scheme, and its own delete semantics, across nine exception tables. The published
spec does not even document the sectool families; they exist in the reference site
and in shipping XSOAR code. Nothing in the product answers "is this domain excepted
anywhere", so allowlist requests get granted a second time, get contradicted by a
block in a different engine, and never get cleaned up. `avanan-cli exceptions find`
asks all nine tables at once, and `avanan-cli exceptions audit` flags the entries
that contradict each other, the exact duplicates, and the ones that have not
matched traffic in the mirrored window.

## "Which tenant is over its seats?"

Licensed seats, add-ons, monthly usage, and detection volume live on four
different MSP endpoints, keyed by tenant. Ranking tenants by any of it means
walking the tenant list and joining by hand, every month.
`avanan-cli msp fleet` produces the joined table directly.

## "Did the quarantine actually land?"

Avanan's action endpoints are asynchronous: `POST /action/entity` returns a task
ID, and the real per-item outcome only appears once `GET /task/{id}` reaches a
terminal state. Every existing integration hands that polling back to the operator.
Worse, the action endpoints accept exactly one scope, so a multi-tenant credential
that omits `scope` gets a bare HTTP 400 that reads like a broken key - the single
most common integration footgun on this API. `avanan-cli remediate` waits on the
task and reports the real per-item result, and turns the 400 into an error that
names the scopes your credential actually covers.

## Why a local mirror

Every question above is cross-tenant, cross-time, or both, and the API is
structurally unable to answer any of them in a single call. Syncing events,
entities, exceptions, and MSP objects into a local SQLite mirror turns a
rate-limited request/response API into a queryable dataset, so the answers come
from a local join instead of a fan-out that spends quota on data you already
pulled an hour ago.

One more thing the mirror is not responsible for, but which matters as much: the
signature. The legacy Avanan handshake signs `reqId + appId + date + requestString
+ secret`, and the `requestString` term is present on every non-auth call and
absent from the published docs. A client that builds the query string differently
from the string it signed gets a 401 that looks exactly like a credential problem.
This CLI signs and sends through one code path, and implements the term the docs
omit - the most-installed community MCP server for this API ships a self-declared
"best-guess HMAC" it warns users to replace.
