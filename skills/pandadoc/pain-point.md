# pandadoc skill - the MSP pain it closes

## The pain

You send proposals, SOWs, and MSAs through PandaDoc and then they go quiet. The
deal isn't dead - it's just sitting, unsigned, and nothing tells you. On r/msp the
recurring thread is some version of "how do you follow up on quotes that never get
signed?" - owners trade reminder scripts and cadences precisely because the proposal
tool surfaces one document at a time and never says *this deal stalled* or *you have
$84k in quotes nobody has touched in three weeks*. So follow-up depends on memory,
forecasting is a guess, and the QBR slide gets rebuilt by hand from a CSV export.

## What this skill does about it

- **`pandadoc-cli stalled --days 14`** - the deals quietly dying: sent but never
  completed inside your window.
- **`pandadoc-cli value`** - total open quote dollars across every in-flight
  document, the number the portal won't add up for you.
- **`pandadoc-cli followup --days 7`** - a ranked nudge worklist that joins stalled
  documents to recipient emails and days-since-sent, ready for outreach.
- **`pandadoc-cli cold-clients --days 30`** - which accounts have gone quiet, ranked
  by days since they last signed anything.
- **`pandadoc-cli forecast`** - open quote dollars bucketed into healthy, aging, and
  stalled tiers by deal age, so the pipeline number means something.

## Status

Beta. Validated against the PandaDoc API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
