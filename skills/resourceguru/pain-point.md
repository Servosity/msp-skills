# resourceguru skill - the MSP pain it closes

## The pain

If you schedule a team in Resource Guru, the day-level questions are the hard
ones: who is overcommitted next week, who has slack to take on new work, and how
much bookable capacity is left before you promise a deadline. Resource Guru's
booking-clash warning catches one over-allocation at a time on write, but there
is no single view of every overcommitted resource-day across the whole fleet for
a window. Verified reviewers on
[Capterra](https://www.capterra.com/p/134429/Resource-Guru/reviews/) say the
reporting and advanced analytics "feel limited" and that they end up downloading
reports "as a pivot" to get the breakdowns they want. So capacity planning turns
into a recurring spreadsheet export-and-pivot exercise instead of a question you
can just ask.

## What this skill does about it

Run `resourceguru-cli sync` once to mirror your schedule into local SQLite, then:

- `resourceguru-cli utilization --start <date> --end <date> --heatmap --agent` - booked-vs-available for every resource on every day, not a blurred range average.
- `resourceguru-cli overbooked --start <date> --end <date> --agent` - every resource-day over daily capacity, across the whole fleet, in one pass.
- `resourceguru-cli bench --start <date> --end <date> --threshold 50 --agent` - who is running under-utilized and free to staff next.
- `resourceguru-cli capacity --start <date> --end <date> --agent` - remaining bookable minutes per resource before you commit a project.
- `resourceguru-cli since 7d --agent` - what moved on the schedule since you last looked.

The API exposes the per-day booking breakdown but never aggregates it; the skill
does that math locally, so the answer comes back offline with zero extra API
calls after the sync.

## Status

Beta. Validated against the Resource Guru API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
