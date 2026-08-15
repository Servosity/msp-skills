# The pain point

## What breaks

Auvik holds the richest picture of a client's network that an MSP owns: every
device, interface, configuration revision, lifecycle date, and alert. The API is
read-only, well documented, and per-client. That last word is the problem.

Every question an owner actually asks spans clients, or spans time, and Auvik
answers neither. "What is going end-of-life across the book" is a per-client
export, done forty times and pasted into a spreadsheet. "What changed since last
week" has no answer at all, because the API reports additions and modifications
and stays silent on removals.

The billing question is the one that reaches the owner directly. Auvik bills per
billable device, and the count driving the invoice is a number on a screen. When
it moves, nothing attached to that number says which devices moved it. The practical advice ends where the tooling does: export it and count by hand.

## Why the obvious fixes do not work

**Use the API.** It is a faithful mirror of the UI's shape: one client, right
now. There is no cross-client aggregate endpoint, so "across the book" means
fanning out over every client and joining the results yourself.

**Use `filter[modifiedAfter]`.** It surfaces devices that were added or changed.
A decommissioned device does not emit an event - it simply stops appearing in
list responses. Removals are undetectable from the API alone, no matter how
often you poll it.

**Export to a spreadsheet.** This works, once. It is stale the next morning, it
carries no history to diff against, and it is the task nobody does in the week
they should have.

**Ask for a report.** Auvik's reporting is built for the network operator
looking at one client, not for the owner looking at the portfolio.

## What is actually reducible

The reading is irreducible - the data has to come from Auvik. Everything after
it is not:

- Holding a prior view of the fleet, so a removal is visible at all
- Joining lifecycle dates and warranty expiry into one urgency bucket
- Putting the billed device list next to the synced inventory and naming the delta
- Joining discovery status to the credential probe that explains it
- Resolving alert volume from opaque device ids into names, types, and clients

Each of those is a join Auvik has the data for and does not perform.

## What good looks like

An owner asks "what is past end-of-support across every client" and gets device
rows, bucketed by urgency, in one command against a local mirror - no export, no
spreadsheet, no per-client loop. The week before a QBR, the refresh list writes
itself. And when a client's billable count jumps, the answer to "which devices?"
takes one command rather than a support ticket.
