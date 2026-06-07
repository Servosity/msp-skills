# proofpoint skill - the MSP pain it closes

## The pain

On r/msp and the Proofpoint community forums the recurring TAP complaint is not
detection quality - it is the API and the dashboard around it. The Threat Insight
SIEM endpoints cap you at 1,800 requests per rolling 24 hours and a single call
cannot span more than a one-hour window, so reconstructing even a day of clicks and
messages means looping the API by hand; the campaign-ids endpoint is throttled to
just 50 requests a day. Analysts ration their own pulls. On top of that the TAP
dashboard answers one screen at a time - one threat summary, one campaign, one Very
Attacked Person - so the questions an MSP actually asks during response ("who is both
heavily attacked and clicking", "show me every event that touched this user", "what
is inside this campaign") cross endpoints the console never joins. The result is the
same pattern teams describe everywhere: export the feeds, re-pull the same windows,
and rebuild the correlation in a spreadsheet, burning quota every time.

## What this skill does about it

Backfill once, then ask. The skill loops TAP's mandatory one-hour windows for you
and mirrors clicks, messages, campaigns, VAPs, and clickers into local SQLite, then
answers the cross-endpoint questions offline:

- `proofpoint-cli backfill --since 12h` - reconstruct overnight clicks and messages
  in one command, auto-looping the API's 1-hour windows so you do not have to.
- `proofpoint-cli risk-overlap --window 30` - the people who are both Very Attacked
  and top clickers, attack index beside click count: your highest-risk humans.
- `proofpoint-cli incident "threat-abc123"` - one incident brief from a threatId:
  severity, attribution, forensic evidence, and every local event that touched it.
- `proofpoint-cli iocs --threat-id "threat-abc123" --csv` - the nested forensic tree
  flattened into a paste-ready indicator table for an EDR or blocklist import.
- `proofpoint-cli user "jane.doe@example.com"` - every click, threat message, VAP
  status, and clicker status for one person, without re-spending SIEM quota.

## Status

Beta. Validated against the Proofpoint API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
