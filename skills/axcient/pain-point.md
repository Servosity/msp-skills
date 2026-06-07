# axcient skill - the MSP pain it closes

## The pain

Axcient's x360Recover Public API is built around per-entity endpoints - one call
for appliances, another for a device, another for that device's jobs - and it does
not hand you the client-to-device mapping directly. So the question every MSP asks
each morning, "whose backups failed last night," has no single fleet-wide endpoint:
you walk each appliance and device by hand, or click through the x360Recover portal
one client at a time. Worse, a job that "succeeded" can still be out of compliance -
the newest restore point is hours stale, or AutoVerify never booted the image - and
MSPs on r/msp keep describing the same trap of finding out a recovery point wasn't
there only when a restore is actually needed.

## What this skill does about it

- **`health`** - every device fleet-wide whose latest backup job failed or went
  stale, grouped by client, in one command - the morning NOC sweep without the
  per-client clicking.
- **`client-rollup`** - one row per client (devices total, failing, stale,
  RPO-breach, AutoVerify-fail) - the dashboard MSPs build by hand today.
- **`rpo`** - flags devices whose newest restore point is older than your
  recovery-point objective, because job-success and restore-point-age are different
  questions.
- **`compliance`** - exportable per-device evidence pairing restore-point age with
  AutoVerify boot-proof and an RPO verdict - QBR and audit evidence without
  screenshotting the UI device by device.
- **`billing`** - protected-system counts and storage rolled up per client into one
  table for month-end invoice reconciliation.

## Status

Beta. Validated against the Axcient x360Recover API surface (including Axcient's
public mock server); the closed-loop receipt (a named MSP running it live in their
production tenant at a Build Session) is tracked separately and added here as
`video.md` once it exists.
