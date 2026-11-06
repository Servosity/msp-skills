# datto-saas skill - the MSP pain it closes

## The pain

Datto's own seat-management documentation names the gap: when Auto-Add is off, or a
client's seat cap has been reached, a newly discovered user is added without backups
enabled ([Exploring Seat Management features](https://saasprotection.datto.com/help/Google/Content/Managing_service_seats/01_Exploring_seat_management_features.htm)).
The SaaS Protection portal shows that one client at a time. Across a fleet of clients,
nobody sees the unprotected new hire until a restore request comes in for a mailbox that
was never backed up.

The same seat counts drive the Datto bill. A seat that is billed but archived, or Active
but not billable, is a margin leak that only shows up when someone reconciles seat
exports against the invoice by hand. And when a client leaves, the domains, subscription
ids, retention and seat list have to be collected from several screens before anyone can
cancel safely.

## What this skill does about it

- `datto-saas-cli seats gaps --since 30d` - every unprotected seat across all clients,
  newest first, with the remoteId a License plan needs.
- `datto-saas-cli seats reconcile --only-flagged` - per client, where Datto's seatsUsed
  disagrees with billable and Active seats, before you invoice.
- `datto-saas-cli backup-health --below 95` - clients ranked worst first by backup
  coverage, short backup windows and perfect-backup rate.
- `datto-saas-cli exit-inventory 123456 --data-source live` - one offboarding manifest:
  domains, subscription ids, retention, every seat, storage, last backups and a blockers
  checklist.
- `datto-saas-cli seats bulk-plan --action License --state Unprotected --customer 123456` -
  the License, Pause or Unlicense batches that would close a gap, without sending anything.

## Status

Beta. Validated against the Datto SaaS Protection API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
