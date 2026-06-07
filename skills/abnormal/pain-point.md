# abnormal skill - the MSP pain it closes

## The pain

On r/msp, the email-security threads circle the same two complaints. The first is
alert fatigue: an analyst's morning is spent inside yet another portal, scrolling the
Threat Log to figure out what's new and what's still unremediated, because the feed is
a flat list with no ranked queue. The second is the quarterly grind: turning the
Abnormal dashboard into a client-ready security report by hand - screenshotting tiles,
retyping attacks-seen and attacks-stopped numbers, chasing impersonation breakdowns -
when all the client wants is the summary. In both cases the work that matters (what's
new, what's unremediated, what to tell the client) is buried under clicking.

## What this skill does about it

- **`abnormal-cli triage --since 24h --top 20`** - a ranked queue of the newest,
  highest-severity, still-unremediated threats, so a shift starts on what actually matters
  instead of a flat list.
- **`abnormal-cli report-snapshot --since 90d --csv`** - attacks seen, attacks stopped,
  impersonation breakdowns, and trending attacks pulled into one client-ready table or CSV.
- **`abnormal-cli employee-risk "vip@acme.com"`** - one account-takeover risk picture per
  employee: profile, Genome identity analysis, 30-day login pattern, and open cases naming them.
- **`abnormal-cli vendor-risk "acme-supplies.com"`** - one vendor-email-compromise picture
  per vendor: details, recent activity, and open vendor cases.
- **`abnormal-cli remediate-watch <threat|case> <id>`** - remediate a threat or case and
  block until Abnormal reports the action reached a terminal state, with a typed exit code
  receipt so you know it finished, not just that it was submitted.

## Status

Beta. Validated against the Abnormal Security API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
