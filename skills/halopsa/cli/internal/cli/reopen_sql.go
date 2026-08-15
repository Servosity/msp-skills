package cli

// Reopen detection derives from the actions table rather than a ticket-level
// marker (#218).
//
// No synced ticket carries $.reopened, so the previous probe concluded that
// reopens could not be detected. That instinct was right, and better than the
// silent zeros in #203 and #211, but the conclusion was wrong: the ticket-level
// fields record only the final close (hasbeenclosed, dateclosed and
// date_fully_closed all point at the last one), while the action trail retains
// the whole history.
//
// Two independent signals, unioned because neither is sufficient alone:
//
//  1. An explicit Re-Open outcome. HaloPSA ships dedicated action outcomes
//     ("Re-Open Ticket 1st/2nd/3rd", "Re-Open Request", "Re-Open Sales"); the
//     reporting tenant had 11 such actions across 7 tickets.
//  2. Repeated resolution. A ticket resolved more than once was reopened in
//     between; that tenant had 24 tickets with two or more Resolved actions.
//
// Signal 1 misses a reopen driven by a customer email, which is how the
// operator-confirmed example on that tenant reopened, with zero explicit
// Re-Open actions. Signal 2 misses a reopen that has not been re-resolved yet.
// Taking the greater of the two per ticket keeps both.
//
// The outcome names are matched case-insensitively with a LIKE prefix. They
// look stock on the one tenant this was measured against, but outcome names can
// be tenant-configured, which is exactly why signal 2 carries the feature on its
// own where they differ: counting repeated Resolved actions is
// configuration-independent.

// reopenCountsSQL yields (ticket_id, reopens) for every ticket showing at least
// one reopen, taking the stronger of the two signals per ticket.
//
// Both branches read the actions table only; the caller joins back to tickets.
const reopenCountsSQL = `
WITH explicit_reopens AS (
    SELECT json_extract(data,'$.ticket_id') AS ticket_id,
           COUNT(*) AS n
    FROM actions
    WHERE COALESCE(json_extract(data,'$.outcome'),'') LIKE 'Re-Open%'
    GROUP BY json_extract(data,'$.ticket_id')
),
repeat_resolutions AS (
    SELECT json_extract(data,'$.ticket_id') AS ticket_id,
           COUNT(*) - 1 AS n
    FROM actions
    WHERE COALESCE(json_extract(data,'$.outcome'),'') = 'Resolved'
    GROUP BY json_extract(data,'$.ticket_id')
    HAVING COUNT(*) > 1
)
SELECT ticket_id, MAX(n) AS reopens FROM (
    SELECT ticket_id, n FROM explicit_reopens
    UNION ALL
    SELECT ticket_id, n FROM repeat_resolutions
)
WHERE ticket_id IS NOT NULL
GROUP BY ticket_id
HAVING MAX(n) > 0`
