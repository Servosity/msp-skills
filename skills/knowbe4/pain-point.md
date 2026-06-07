# knowbe4 skill - the MSP pain it closes

## The pain

On r/msp the recurring KnowBe4 complaint is reporting, not the product itself:
the KMSAT console answers one tenant, one phishing test, and one risk chart at a
time, so any cross-client or cross-test question turns into a CSV export and a
pivot table. Threads asking how to pull "repeat offenders across all clients" or
"a single phish-prone trend for a QBR" get the same answer - export everything and
build it in a spreadsheet. The hardest gap is correlation: phishing results and
training completion live in separate reports, so the most useful list a vCISO owns
- people who clicked a phish and never finished training - does not exist in the
portal at all. At quarterly-review time that means hours of manual joining per
client just to find the short list of humans who actually need attention.

## What this skill does about it

Sync once, then ask. The skill mirrors your KnowBe4 reporting data into local
SQLite and answers the questions the console can't:

- `knowbe4-cli repeat-clickers --min-clicks 2 --since 90d` - the riskiest 5% of
  humans, counted across every phishing test, not one at a time.
- `knowbe4-cli untrained-clickers --since 180d` - the anti-join the portal never
  runs: clicked a phish, no passed training.
- `knowbe4-cli risk-drift --window 90d --worsened --top 20` - who is deteriorating
  this quarter, ranked, instead of a wall of risk numbers.
- `knowbe4-cli coverage-gaps` - active users your program silently misses: zero
  phishing or zero training coverage.
- `knowbe4-cli qbr --since 90d` - the whole quarterly review (risk trend,
  phish-prone trend, training completion, top-risk humans) in one command.

## Status

Beta. Validated against the KnowBe4 API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
