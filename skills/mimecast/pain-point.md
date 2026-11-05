# mimecast skill - the MSP pain it closes

## The pain

"Release my email" tickets are urgent and risky at the same time. Thread's public
Mimecast runbook for MSP service desks
(https://docs.getthread.com/skill-library/vendor-runbooks/mimecast-email-gateway)
tells techs to verify the requester before releasing a held message, to treat
urgency as a caution flag, never to release a malware verdict or a sandbox or
impersonation hold on the requester's word alone, and, in its own words, "when in
doubt, don't release."

Doing that properly means reading several Mimecast Administration Console pages for
one message: message tracking to find it, the hold queue to see why it is held, the
held release log to see whether someone already released or rejected it, and the
rejection log when it never reached the hold queue at all. Each has its own search.

Phishing reports widen the problem. The same runbook says to treat an allowed click
as an incident ("an allowed click is never informational") and to pull the click
logs for sibling deliveries of the same URL. The URL Protect click log has no URL
filter in the API, so finding everyone who clicked means paging the whole log and
matching by hand.

## What this skill does about it

- `mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent` -
  where the message is now (held, rejected at the gateway, released, rejected from
  hold, delivered or bounced) and why, with policy, spam score, SPF/DKIM/DMARC and
  who released or rejected it, in one verdict.
- `mimecast-cli held triage --since 24h --group-by reason --agent` - the whole hold
  queue grouped by hold reason with a release-risk class per group, so the queue is
  worked in a few grouped decisions instead of message by message.
- `mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan` -
  the hold-reason check before a release: impersonation, malware, sandbox and policy
  holds are refused unless `--override` gives a reason, and the ticket is recorded
  in a local ledger that `held release-audit` reads back.
- `mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent` -
  every user who clicked a reported link (raw or Mimecast-rewritten), whether the
  click was blocked, and who clicked through a warning.
- `mimecast-cli threats sender @phish.example --since 3d --agent` - the blast radius
  of one phishing sender: every recipient it reached, who clicked, and which
  messages are still in the hold queue.

## Status

Beta. Validated against the Mimecast API surface and a fake Mimecast API; the
closed-loop receipt (a named MSP running it live in their production tenant at a
Build Session) is tracked separately and added here as `video.md` once it exists.
