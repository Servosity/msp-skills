<!--
WHITE-LABEL REPORT TEMPLATE - governance-snapshot
Fill the [BRACKETS], paste the generated findings where marked, deliver as PDF/Markdown.
Produce the findings block with:
    microsoft-graph-cli apps consent --json > audit.json
    ../report.py audit.json --org "[CLIENT NAME]" -o client-report.md
Then wrap it in this branded shell, or point report.py's output straight at the client.
This file is the *shell*; report.py generates the *body*. Keep them separate so the
body is always freshly computed and never hand-edited (hand-edited findings are how a
governance report becomes a lie).
-->

![[YOUR-MSP-LOGO]]

# Microsoft 365 Third-Party App Consent Review
### Prepared for [CLIENT NAME] by [YOUR MSP NAME]
_[DATE] · Prepared by [ANALYST NAME], [TITLE] · Confidential_

---

## Why this review exists

Every SaaS tool, plugin, and integration your team has connected to Microsoft 365 was
granted **standing access** to your tenant - often to mail, files, and directory data,
sometimes tenant-wide, sometimes able to grant itself more. That access does not expire
and rarely gets reviewed. This report inventories every third-party app consented into
[CLIENT NAME]'s tenant and ranks them by risk, so you can revoke what you do not
recognize and right-size what you keep.

The review is **read-only**. Nothing in your tenant was changed to produce it.

---

<!-- PASTE THE GENERATED FINDINGS BELOW THIS LINE -->
<!-- (the full output of: ../report.py audit.json --org "[CLIENT NAME]") -->

[[ GENERATED FINDINGS GO HERE ]]

<!-- END GENERATED FINDINGS -->

---

## What we recommend as your next step

- [ ] Revoke or confirm every app flagged **privilege-escalation** this week.
- [ ] Schedule a 30-minute review of the **admin-consented** apps with the client owner.
- [ ] Turn on the [YOUR MSP NAME] managed app-consent policy so new shadow-IT apps
      require admin review before they can access the tenant.
- [ ] Re-run this review [monthly / quarterly] - it is one read-only command.

## About [YOUR MSP NAME]

[One paragraph about your MSP, your security posture practice, and how to reach you.]
[Contact: name · email · phone · scheduling link]

---
_Generated from the read-only `microsoft-graph-cli apps consent` command. Every count is
independently verifiable in the client's Entra admin center → Enterprise applications →
Consent and permissions. [YOUR MSP NAME] retains no copy of the tenant's data; the report
is computed on demand and handed over._
