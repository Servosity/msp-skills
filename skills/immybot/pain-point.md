# immybot skill - the MSP pain it closes

## The pain

ImmyBot is very good at the thing it was built for: put software on machines and keep it
there. The pain MSPs describe is the morning after. A maintenance window runs across
hundreds of endpoints overnight, a slice of it fails, and the console reports the same
error text once per machine - no grouping, no root cause, no way to tell one broken
package from forty broken agents. The r/msp and ImmyBot community threads on deployment
troubleshooting keep landing on the same three questions, and none of them is answerable in
one view:

- "Which of these failures are actually the same failure?"
- "Which of my clients are still below a version floor on this one title?"
- "Why did this machine not get the deployment when the one next to it did?"

The last one is the expensive one, because target assignments resolve through overlapping
scopes and the console shows you the result, not the reasoning. And because ImmyBot keeps
no history to compare against, "what changed since last night" is a question an MSP answers
with a spreadsheet or not at all.

## What this skill does about it

The skill mirrors every tenant into a local SQLite store and turns each of those questions
into one command:

- **`session-triage --since 24h`** - collapses a night of failed maintenance actions into
  distinct root causes, so you read the failure once instead of once per machine.
- **`version-spread "Google Chrome" --min-version 140`** - one software title ranked across
  every tenant with a real semver comparator, flagging everything below the floor.
- **`assignment-explain 4821`** - every target assignment that resolves onto one computer,
  which scope matched, and which rules are shadowed: the direct answer to "why not this
  machine?".
- **`fleet-diff --since 24h`** - computers added or removed, software versions moved,
  assignments modified, reconstructed from history the API does not retain. Record the first
  baseline with `fleet-diff --snapshot` after a sync; comparisons work from the next one on.
- **`deployment-health --only-failing`** - deployments that have never once succeeded, the
  failure class nobody notices because it never raised an alert.
- **`onboarding-stalled --older-than 3d`** - computers sitting in the onboarding queue,
  annotated with whether onboarding was ever actually attempted.

## Status

Beta. Validated against a live ImmyBot tenant by the contributing MSP, including a full
sync of 119,687 records across 65 resources; the cross-tenant commands above were run
against that real data.
