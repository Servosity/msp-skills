<!-- DRAFT — client-facing leave-behind one-pager. Not final copy. White-label per MSP. -->

# Do you know which apps can read your company email? (DRAFT)

Every app your team has "Signed in with Microsoft" or connected to Microsoft 365 was
handed a key to some of your company data — mail, files, calendars, your staff directory.
Some were approved for your **whole** organization. Some can **grant themselves more
access**. That access does not expire, and most businesses have never reviewed it.

## The 3-minute, read-only review

[YOUR MSP] runs a single read-only report against your Microsoft 365 tenant. **Nothing is
changed.** You get one page that answers:

- **Which outside apps can access our data**, and what each one can reach.
- **Which are the risky ones** — apps with broad, tenant-wide, or self-escalating access.
- **Which nobody remembers approving** — the shadow IT and the leftovers from apps you no
  longer use.

## What you'll see

> _Example (synthetic):_ **5 of 7 connected apps need review, including 1 that can grant
> itself full control.** A backup vendor with tenant-wide mail access. A migration tool
> that can rewrite any user. A time-tracker 40 of your staff approved themselves.

Each app is ranked by risk with a plain-English reason and a recommended action.

## Why it matters

Compromised app consent is one of the most common ways attackers keep access to Microsoft
365 **after** a password change and even after MFA. It is also the easiest to miss,
because there is no single screen in Microsoft's admin center that shows it. This report is
that screen.

## What happens next

1. We run the read-only review (no downtime, no changes).
2. We walk you through the findings.
3. We revoke what you don't need and set up a policy so new app approvals come to us first.

**Ready in a day. Read-only. Ask [YOUR MSP] to run your Third-Party App Consent Review.**

---
_[YOUR MSP NAME] · [contact] · [scheduling link]. Report generated with read-only Microsoft
Graph access; every finding is verifiable in your own Microsoft 365 admin center._
