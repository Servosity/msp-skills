# nerdio skill - the MSP pain it closes

## The pain

On r/msp, the recurring Nerdio refrain is that autoscale is the whole reason to
buy Nerdio Manager - and the hardest thing to keep honest across a fleet. MSP
owners describe the same loop: a customer's host pool gets stood up, autoscale
never gets enabled (or drifts off a baseline), and idle Azure session hosts
quietly bleed spend until the monthly invoice makes someone go looking. There is
no single, fleet-wide view of which pools in which tenants actually have scaling
on.

Two structural facts make it worse:

- **Every NMM install is per-tenant.** Nerdio Manager for MSP runs as each MSP's
  own instance, so even a basic question - "how many session hosts are running
  right now across all my clients?" - means logging into one portal at a time.
- **Every change is async.** NMM mutations (provisioning, scripted actions,
  backup, host power operations) return a job ID, and the portal makes you
  babysit the Jobs page to learn whether anything actually finished. There is no
  scriptable "wait until done."

The net effect: the questions that matter most at QBR time and month-end -
cost-control sweeps, per-customer billing reconciliation, "what's running where" -
are exactly the ones the portal answers one tenant, one click, one refresh at a
time.

## What this skill does about it

- **`nerdio-cli fleet autoscale-audit`** - every host pool across every customer
  account whose autoscale is disabled or diverges from your baseline, in one
  table. The Monday cost-control sweep as one command.
- **`nerdio-cli fleet host-estate`** - every session host across all accounts with
  its pool, account, and power state - the weekend power-sweep view.
- **`nerdio-cli fleet billing-rollup --period <start>:<end>`** - per-account
  billed, paid, unpaid, and usage totals joined to account names -
  PSA-reconciliation-ready without exporting to a spreadsheet.
- **`nerdio-cli usages drift --from <start>:<end> --to <start>:<end>`** - the
  customers whose Azure consumption grew or shrank beyond a threshold between two
  periods.
- **`nerdio-cli job wait <job_id>`** - poll any NMM async job to a terminal state
  and exit non-zero when it failed, so automation never reports success on a job
  that actually errored.

Source: recurring threads on [r/msp](https://www.reddit.com/r/msp/) about Nerdio
autoscale and Azure cost control, and the per-tenant portal workflow MSP owners
describe there.

## Status

Beta. Validated against the Nerdio Manager API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
