# granola skill - the MSP pain it closes

## The pain

MSP owners and vCIOs record their client calls - QBRs, discovery calls, onboarding,
project check-ins - and let a tool like Granola transcribe and summarize each one.
The recordings and AI summaries pile up, but the context stays trapped one meeting
at a time. On r/msp the recurring complaint around QBR prep and client reviews is
the same shape: "the notes are all there, I just can't get to them across meetings."
Before a quarterly review you're scrubbing back through weeks of separate calls to
remember what you promised; nobody can answer "what did we tell this client last
quarter" without opening every meeting by hand. The meeting data exists - it just
isn't queryable across calls.

## What this skill does about it

It syncs every Granola meeting into a local, full-text-searchable mirror, so the
questions that used to mean re-watching calls become one command:

- **`attendee timeline <contact>`** - every call with a client contact, oldest to
  newest, so you walk into their QBR with the whole history in front of you.
- **`attendee brief <contact> --last 5`** - the last few calls with a contact, real
  notes + AI summary inlined, as instant pre-call prep.
- **`meetings list --query "<topic>"`** - every meeting where a topic (a project, a
  commitment, a product) came up, matched against titles, notes, and transcripts.
- **`export-all --out ./calls --last 7d`** - bulk-export a week of client calls to
  markdown files for your PSA, a follow-up doc, or a downstream agent.
- **`talktime --by participant --since 30d`** - see how much of a month of client
  calls was you talking versus everyone else, without re-listening to a recording.

## Status

Beta. Validated against the Granola API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
