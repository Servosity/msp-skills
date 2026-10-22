# connectsecure skill - the MSP pain it closes

## The pain

"Vulnerability scans rarely come back clean, so MSPs are often staring at
thousands of findings across dozens of customers." That is how ChannelPro framed
the problem in
[Too many vulnerabilities, not enough time](https://www.channelpronetwork.com/2026/08/02/how-msps-can-manage-vulnerability-data/)
(first published December 2025, updated August 2026). In the same piece
ConnectSecure's CEO, Peter Bellini, describes the goal: "In the next 30 days,
instead of trying to worry about these 10,000 vulnerabilities, maybe let's focus on
these 100" - which ConnectSecure approaches with AI-ready reporting in its portal.

This skill works on the same problem from the AI agent an MSP owner already uses,
and adds the questions that span every client at once - and the ones about whether
the data can be trusted at all.

### "What do we fix first, across everyone?"

ConnectSecure's per-client vulnerability report names the client but lists the
affected assets only by ID. Ranking open findings across the whole client base at
asset level - this CVE, on this machine, at this client - means matching those IDs
to the asset list yourself, usually in a spreadsheet, and redoing it every time
the data moves.

### "Does this CVE hit us?"

The morning an advisory lands, the question is which clients, which machines,
and is there a fix. Answering it takes the CVE report (details, affected
software, fixes) and the assets-by-problem report (affected machines), then a
manual grouping of machines by client.

### "Can we trust this clean report?"

A clean result on an asset that has not been discovered in weeks, was scanned
without credentials, or whose agent stopped checking in is not clean, it is
unknown. Those assets quietly make every client report look better than it is.

### "What are our suppressions hiding?"

A suppression is a decision to stop seeing a finding. Over time some of them
expire but are never lifted, some never had an end date, and some were approved
by the same technician who requested them. Each one keeps a real vulnerability
out of every report and every QBR.

## What this skill does about it

- `connectsecure-cli exposure --min-epss 0.5 --severity Critical,High` - one
  ranked list of open findings across every client, one row per finding, asset
  and client, ordered by exploit probability (EPSS), then CVSS, then whether it
  is on the CISA Known Exploited list.
- `connectsecure-cli cve-impact "CVE-2024-3400"` - which clients and assets a
  named CVE hits, the affected software, and the available fixes, with any
  assets it could not attribute to a client called out instead of dropped.
- `connectsecure-cli blind-spots` - every asset whose scan data cannot be
  trusted, grouped by client, each with a reason code (stale discovery,
  unauthenticated scan, silent agent).
- `connectsecure-cli suppression-audit` - every active suppression that expired,
  expires soon, never expires, or was unapproved or self-approved, with the
  suppressed finding's current EPSS and severity, riskiest first.
- `connectsecure-cli portfolio` and `connectsecure-cli qbr "Acme Dental"` - one
  row per client to compare the book, and one client's QBR risk pack (risk
  score, open findings by age, top fixes, remediation speed).

## Status

Beta. Validated against the ConnectSecure API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.

