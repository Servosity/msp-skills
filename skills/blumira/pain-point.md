# blumira skill - the MSP pain it closes

## The pain

Blumira is sold as detection-and-response that an MSP can run without standing up
a full SOC, and the r/msp threads weighing it against the other MDR/SIEM options
keep landing on the same day-two catch: the product is easy to deploy, but the
ongoing work is triage, and the partner portal makes you do that triage one
account at a time. Switch the active organization, sort the open findings, note
the worst, switch to the next client, repeat. Across a book of clients there is
no single screen that joins findings, detection coverage, and agent health, so
questions like "which accounts are behind on coverage this month" or "which
domain controllers went dark this week" turn into a manual sweep an owner holds
in their head, exactly the kind of cross-tenant blind spot r/msp operators raise
about every per-tenant security portal.

## What this skill does about it

It mirrors every account into a local store and answers the cross-account
questions the portal can't compose:

- **`blumira-cli triage --status open`** - one globally-ranked open-findings queue across every client account, so analysts work the worst thing first regardless of which client it belongs to.
- **`blumira-cli coverage --against basis`** - detection rules missing or disabled versus your basis ruleset, per account, so coverage gaps surface before an auditor or attacker finds them.
- **`blumira-cli exposure --flag-dc-stale`** - domain controllers that went stale or unprotected, surfaced first, so a client's most important server is never a silent blind spot.
- **`blumira-cli velocity --by account --window 30d`** - mean-time-to-resolve and open-rate per account, so you can see which client is drowning before they churn.
- **`blumira-cli drift`** - new, resolved, and status-changed findings since your last sync, so a daily standup reads from one diff instead of thirty portals.

## Status

Beta. Validated against the Blumira API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
