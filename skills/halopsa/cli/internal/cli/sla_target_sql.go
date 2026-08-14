// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// Every SLA-facing local view read tickets.targetdate, which HaloPSA does not
// use. The portal's Response Target and Resolution Target come from
// respondbydate and fixbydate; targetdate is a legacy column left at the
// sentinel 1900-01-01T00:00:00. On the tenant this was found against, 1264 of
// 1265 synced tickets carry that sentinel while all 1265 carry real
// respondbydate and fixbydate values.
//
// The consequence is a silent zero rather than an error. A comparison like
//
//	datetime(targetdate) BETWEEN datetime('now') AND datetime('now','+24 hours')
//
// can never be true against 1900-01-01, so `sla breaching` and triage's
// breach_count report no SLA risk on every tenant regardless of what is
// actually due, and `sla scorecard` counts every ticket as having a target
// that nothing can satisfy, reporting 0 percent met.
//
// Verified against the portal on six tickets: each Response Target and
// Resolution Target shown in the UI matches respondbydate / fixbydate exactly
// (offset only by the tenant's UTC display conversion), while targetdate reads
// 1900-01-01 on all of them. One open ticket was 16 hours from its resolution
// target and 27 hours past its response target while `sla breaching` reported
// zero.
//
// targetdate is kept as a fallback rather than dropped: it is populated on a
// small number of tickets, so a tenant that does use it still works.

package cli

// slaResolutionTargetSQL is the ticket's resolution deadline: fixbydate, then
// targetdate, and NULL when neither carries a usable value. The 1900-01-01
// sentinel is treated as absent, so "has a target" tests must check IS NOT
// NULL rather than != ''.
//
// The expression reads an unqualified `data` column, so the surrounding query
// must expose exactly one.
const slaResolutionTargetSQL = `CASE
    WHEN COALESCE(json_extract(data,'$.fixbydate'),'') <> ''
         AND json_extract(data,'$.fixbydate') NOT LIKE '1900-01-01%'
        THEN json_extract(data,'$.fixbydate')
    WHEN COALESCE(json_extract(data,'$.targetdate'),'') <> ''
         AND json_extract(data,'$.targetdate') NOT LIKE '1900-01-01%'
        THEN json_extract(data,'$.targetdate')
END`

// slaResponseTargetSQL is the ticket's first-response deadline, resolved the
// same way. Exposed for callers that report response SLA separately; the
// breach views intentionally track the resolution target, which is what
// targetdate was standing in for.
const slaResponseTargetSQL = `CASE
    WHEN COALESCE(json_extract(data,'$.respondbydate'),'') <> ''
         AND json_extract(data,'$.respondbydate') NOT LIKE '1900-01-01%'
        THEN json_extract(data,'$.respondbydate')
END`
