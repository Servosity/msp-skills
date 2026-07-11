# zammad skill - the MSP pain it closes

## The pain

Support leads live in the "who's drowning, what's aging, who's about to leave"
questions - and their help desk can't answer any of them in one place. On r/msp
the recurring complaint about ticketing tools is the same: the dashboards tell you
what's *in* a queue but never roll up *across* tickets - per-agent load, which
customers keep reopening, which accounts are quietly heading for the exit. So the
weekly ritual is exporting tickets to a spreadsheet and rebuilding those answers by
hand, while the two signals that actually predict churn - a customer who sounds fed
up, and feature/pricing/compliance asks buried in article threads - never surface
until the account is already gone.

## What this skill does about it

- `zammad-cli agent-load` - who is overloaded right now, open/pending/backlog per agent, in one call.
- `zammad-cli overdue --days 3` - every ticket open too long, priority-weighted so the worst rise first.
- `zammad-cli escalate` - active tickets whose inbound customer messages read as upset, with the matched text shown.
- `zammad-cli churn-risk` - accounts trending toward churn, scored from backlog pressure, overdue work, and negative sentiment, with the reasons listed.
- `zammad-cli feedback-scan --bucket pricing` - what customers keep asking for, bucketed into feature / pricing / compliance / bug with source tickets.

## Status

Beta. Validated read-only against a live production Zammad tenant (20,000+ tickets)
and adversarially reviewed for analytics correctness. The closed-loop receipt (a
named MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
