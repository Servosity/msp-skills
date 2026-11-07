# keeper skill - the MSP pain it closes

## The pain

An MSP running Keeper for a book of clients answers the same questions at every QBR: who still has 2FA off, whose passwords are weak or reused, is BreachWatch on, who left and never got cleaned up, and what moved since last time.

Those facts are read tenant by tenant. A [2026 review of Keeper for MSPs](https://www.flamingo.run/blog/keeper-security-review-for-msps) describes the password view as "per tenant" and lists advanced reporting among the add-ons billed separately. [Keeper's own MSP best practices](https://docs.keeper.io/enterprise-guide/keeper-msp/best-practices) recommend change and activity reports plus alerts. None of that keeps last quarter's numbers beside this quarter's for you. The comparison ends up in a spreadsheet someone has to rebuild.

## What this skill does about it

- `keeper-cli posture report --days 90 --agent`: every client's 2FA coverage, password health, BreachWatch, inactivity and seats in one run, stored locally.
- `keeper-cli posture diff --agent`: per-company deltas and per-user changes since the previous run, for the "since last time" answer.
- `keeper-cli users risk --kind no-2fa --agent`: users with 2FA off across every client, admins outside a 2FA-enforcing role first.
- `keeper-cli users hygiene --kind invited,inactive --agent`: stale invites and inactive users across every client, for cleanup sweeps.
- `keeper-cli events list --since 0 --agent`: each posture change once, for automation.

## Status

Beta. Validated offline against a fake modeled on the open-source Commander Service Mode; awaiting a named MSP running it live against their own Service Mode host.
