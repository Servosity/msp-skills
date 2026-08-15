package cli

// Every SLA-facing local view read tickets.targetdate, which HaloPSA does not
// use. The portal's Response Target and Resolution Target come from
// respondbydate and fixbydate; targetdate is a legacy column left at the
// sentinel 1900-01-01T00:00:00. On the tenant this was reported against, 1264
// of 1265 synced tickets carry that sentinel while all 1265 carry real
// respondbydate and fixbydate values (#213, reporter @geekbrownbear).
//
// The consequence is a silent zero rather than an error. A comparison like
//
//	datetime(targetdate) BETWEEN datetime('now') AND datetime('now','+24 hours')
//
// can never be true against 1900-01-01, so `sla breaching` and triage's
// breach_count reported no SLA risk on every tenant regardless of what was
// actually due, and `sla scorecard` counted every closed ticket as carrying a
// target nothing could satisfy, reporting 0 percent met on a tenant whose real
// attainment is 97 to 99 percent.
//
// Verified against the portal on six tickets: each Response Target and
// Resolution Target shown in the UI matches respondbydate / fixbydate exactly,
// offset only by the tenant's UTC display conversion, while targetdate reads
// 1900-01-01 on all of them. One open ticket sat 16 hours from its resolution
// target and 27 hours past its response target while `sla breaching` reported
// zero.
//
// targetdate is kept as a fallback rather than dropped: a small number of
// tickets do populate it, so a tenant that relies on it still works.
//
// This is the same defect family as the blank agent_name / datecreated columns
// in #203 and #211: a generated column the live API never fills. The spec
// describes all three date fields with no description, so the generator had no
// way to tell which one HaloPSA actually writes.

// slaResolutionTargetExpr is the ticket's resolution deadline: fixbydate, then
// targetdate, and NULL when neither carries a usable value. The 1900-01-01
// sentinel is treated as absent, so "has a target" tests must check IS NOT NULL
// rather than != ''.
func slaResolutionTargetExpr(alias string) string {
	p := qualify(alias)
	return `CASE
    WHEN COALESCE(json_extract(` + p + `data,'$.fixbydate'),'') <> ''
         AND json_extract(` + p + `data,'$.fixbydate') NOT LIKE '1900-01-01%'
        THEN json_extract(` + p + `data,'$.fixbydate')
    WHEN COALESCE(json_extract(` + p + `data,'$.targetdate'),'') <> ''
         AND json_extract(` + p + `data,'$.targetdate') NOT LIKE '1900-01-01%'
        THEN json_extract(` + p + `data,'$.targetdate')
END`
}

// slaResponseTargetExpr is the ticket's first-response deadline, resolved the
// same way. Exposed for callers that report response SLA separately; the breach
// views intentionally track the resolution target, which is what targetdate was
// standing in for. Surfacing response breaches as well is a behaviour change
// rather than a bug fix, so it is deliberately left unwired.
func slaResponseTargetExpr(alias string) string {
	p := qualify(alias)
	return `CASE
    WHEN COALESCE(json_extract(` + p + `data,'$.respondbydate'),'') <> ''
         AND json_extract(` + p + `data,'$.respondbydate') NOT LIKE '1900-01-01%'
        THEN json_extract(` + p + `data,'$.respondbydate')
END`
}
