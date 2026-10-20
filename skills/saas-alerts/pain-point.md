# saas-alerts skill - the MSP pain it closes

## The pain

SaaS Alerts watches Microsoft 365 and Google Workspace activity for every customer, and
its Respond module can act on a compromised account automatically or after a technician
approves it. The approval queue is on a clock. The Respond API documents that a manual
approval "will auto expire when the rule triggers again or 12 hours whichever is
earlier", and the SaaS Alerts help center
([Analyzing a Respond trigger](https://help.saasalerts.kaseya.com/help/Content/How-To/analyzing-a-respond-trigger.htm))
says a failed remediation is reminded every 15 minutes and automatically ignored after
the third reminder. With dozens of customers, the question every morning is not "what
is in the queue" but "which decision lapses first, and what would approving it do".

The volume does not help. MSP reviewers of SaaS Alerts on
[G2](https://www.g2.com/products/saas-alerts/reviews) describe a flood of notifications
until the configuration is tuned, and false positives you learn to filter out over time;
Kaseya's own [Tech Jam webinar](https://webinar.kaseya.com/webinar-saas-alerts/tech-jam-getting-past-the-basics-and-level-up-your-saas-alerts)
spends a session on noise reduction. Tuning well means seeing which alerts repeat across
customers, and which source is hitting more than one of them, not one tenant at a time.

Scripting it has its own trap. Respond wants a short-lived access token minted from the
partner key with `POST /respondApi/api/v1/auth`, and a call that carries only the key
fails with `Only secure origins are allowed`. In the r/msp thread
["Kaseya Saas Alerts API"](https://www.reddit.com/r/msp/comments/1p9dbc1/kaseya_saas_alerts_api/) an MSP on the EU instance got that error on every call;
another replied they had been "banging my head against this same issue for weeks",
suspecting CORS, with support tickets unanswered. The answer the thread found: POST to
the auth endpoint first, then use the access token.

## What this skill does about it

- `saas-alerts-cli rule-triggers queue --expiring-within 2h` - the Respond triggers that
  expire or auto-ignore soonest, with the actions an approval would fire, so the decision
  that would be lost first is made first.
- `saas-alerts-cli rule-triggers decide --decision reject --rule <rule-id>` - one
  decision applied to every matching trigger, previewed first, sent only with `--confirm`
  and re-read afterwards to verify it took.
- `saas-alerts-cli triage --since 24h` - what fired overnight across every customer,
  critical first, grouped by customer, account and event type.
- `saas-alerts-cli cross-tenant --by ip --since 24h` - one IP, network or country hitting
  two or more customers, minus each customer's own whitelist.
- `saas-alerts-cli contain "jane.doe@example.com"` - block sign-in, expire sessions and
  reset the password for one Microsoft 365 account, previewed until you confirm.

## Status

Beta. Validated against the SaaS Alerts API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
