# connectwise-manage skill - the MSP pain it closes

## The pain

Three pains, each documented in the community, each structural to how ConnectWise
PSA (Manage) presents its own data:

**1. Unlogged time is direct revenue leakage.** MSP consultancy Bering McKinley,
writing on ConnectWise Manage time entry, prices the cost plainly: "lost billable
hours, lower profitability, bad decision-making caused by incomplete data, and
unhappy clients" - techs close tickets without entering time, and nobody notices
until the invoice run. Third-party reviews of Manage (Rallied) call time entry the
most consistently reported pain point in the MSP community. The structural gap:
Manage has no single view that joins closed tickets to their logged hours, so the
leak is invisible until billing reconciliation.

**2. Cross-entity questions mean exports.** Native reporting is widely panned -
Support Adventure's ConnectWise Manage overview notes the built-in reports
"don't have the appropriate visual touch to be useful for client facing reporting"
and steers MSPs to paid BI add-ons; Pivotal Crew's "Why ConnectWise Reports Fail
MSPs" traces unreliable numbers back to "technicians logging time inaccurately,
late, or not at all" and "misconfigured agreements." Answering "how burned is this
client's block-hours agreement" or "what does this client's full footprint look
like" takes multiple portal screens - a complaint as old as the official
ConnectWise product forum, where users report needing a click and a wait for every
field ("You can't see more than one phone number on a contact unless you click and
look through the options").

**3. The API's conditions syntax punishes you silently.** String values must be
double-quoted, dates bracketed, OR-sets parenthesized. Get it wrong and you get a
400 - or worse, silently empty results. Integration vendors maintain dedicated KB
articles for "ConnectWise API Error 400: Condition Syntax Error" (Mobius), and
developer-community GitHub issues ask simply "How to use conditions?"
(ConnectPyse #13). Every live-API wrapper inherits this trap on every call.

## What this skill does about it

- `connectwise-manage-cli unbilled --since 7d` - tickets you touched or closed
  this week with zero (or under-threshold) hours logged. The billing-leak detector;
  run it before every invoice cutoff.
- `connectwise-manage-cli agreement-burn --period 30d` - hours consumed vs
  allotment per agreement with an over-limit flag. The unprofitable-client early
  warning.
- `connectwise-manage-cli account AcmeCorp` - contacts, active agreements,
  deployed configurations, open-ticket count, and last activity in one card. The
  five-screen client picture without the five screens.
- `connectwise-manage-cli stale --days 5` / `connectwise-manage-cli workload` -
  the dispatcher's daily pass: what's rotting, and who has bandwidth.
- `connectwise-manage-cli condition build --field board/name --op = --value "Help Desk"` -
  a validated conditions expression, correct the first time, for when you do need a
  live filtered query.

The first four run against a local SQLite mirror (`sync` first), so they're
instant, offline, and free of rate limits and conditions-syntax traps.

## Sources

- Bering McKinley, "ConnectWise Manage time entry: getting my techs to enter time"
  (updated 2025) - beringmckinley.com/blog/connectwise-manage-techs-time-entry
- Rallied, ConnectWise Manage review (quoting r/msp time-entry threads) -
  rallied.ai/blog/connectwise-manage-review
- Support Adventure, "Connectwise Manage overview for MSPs" (2020) -
  supportadventure.com/overview-of-connectwise-manage-for-msps
- Pivotal Crew, "Why ConnectWise Reports Fail MSPs" (2026) -
  pivotalcrew.com/connectwise-reporting-tools-what-they-do-and-why-they-fail
- ConnectWise product community, "I have several problems with the 'functionality'
  of ConnectWise's suite of software" (~2019) - product.connectwise.com thread 18730
- GitHub: joshuamsmith/ConnectPyse issue #13 "How to use conditions?" (2017);
  Mobius KB "ConnectWise API Error 400: Condition Syntax Error"

## Status

Beta. Validated against the ConnectWise Manage API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
