# kaseya-bms skill - the MSP pain it closes

## The pain

Ask any MSP running Kaseya BMS what they think of the reporting, and the answer on
r/msp and in the MSPGeek community is consistent: it's the weak spot. The questions
a service desk asks every single morning - which queues are underwater, which
tickets have gone stale, which technician is buried and which one can take the next
escalation - aren't a click away. They're an export to Excel and a pivot table.

Month-end is the other recurring complaint. Pulling approved-but-unbilled time per
client out of BMS so you can actually invoice it is a manual export-and-reconcile
chore, and the same goes for seeing how much of a prepaid contract a client has
burned before the renewal conversation. Meanwhile the BMS API caps you at 1,500
requests per hour per endpoint, so any tool that re-asks the question live every
time runs out of budget exactly when you need fleet-wide numbers at QBR time.

The result: the data is all in BMS, but the answers an owner wants are trapped
behind manual exports and a console built for one-screen-at-a-time clicking.

## What this skill does about it

It syncs the tenant into a local SQLite mirror with full-text search, then answers
the daily questions as one local join - instant, offline, and free of the rate
limit. The highest-leverage commands:

- `kaseya-bms-cli queue-health --agent` - open ticket volume by queue, priority, and
  status in one shot, with stale counts flagged before standup.
- `kaseya-bms-cli stale-tickets --days 7 --agent` - the specific open tickets nobody
  has touched in a week, oldest first, with account and assignee.
- `kaseya-bms-cli workload --agent` - open load per technician joined with hours
  logged, so you can see who's buried and who can take the next ticket.
- `kaseya-bms-cli unbilled --agent` - approved, billable, not-yet-billed time grouped
  by account, in hours - the month-end ready-to-bill review without the Excel export.
- `kaseya-bms-cli contract-burn --window-days 90 --agent` - hours consumed and percent
  of the contract period elapsed per agreement, at-risk contracts first.

## Status

Beta. Validated against the Kaseya BMS API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
