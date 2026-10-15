# proxmox skill - the MSP pain it closes

## The pain

Proxmox VE has become the hypervisor many MSPs move small clients to, usually
with a Proxmox Backup Server (PBS) at the client site, in the MSP's datacenter,
or both. That shape multiplies the places a backup can quietly go wrong, and
every one of them lives behind a separate web login.

A real example from the Proxmox support forum, ["Proxmox Backup Server - a lot of
'verify failed'"](https://forum.proxmox.com/threads/proxmox-backup-server-a-lot-of-verify-failed.165129/)
(April 2025): a provider runs a central PBS that 100 to 200 servers back up to,
plus on-site PBS units at customer locations. The weekly verify job showed a
growing number of failed snapshots; for some servers every recent backup had
failed verification, and one customer snapshot verified on the on-site PBS but
failed on the central one. The thread ran almost four weeks before it was marked
solved; part of the cause was that verify took 7 to 8 days on the biggest
datastore while garbage collection and pruning ran over the top of it.

Three things make this hard to see from the Proxmox UI:

- **A green job is not a backed-up guest.** One vzdump task backs up many
  guests, and the guest that failed or never started is named only inside the
  task log. A VM created after a job with a fixed VM list was set up is in no
  job at all.
- **A recent backup is not a restorable backup.** Verify state, encryption
  and protection live on the PBS side, not next to the VM.
- **Every client is another login.** One cluster and often one PBS per client
  means the morning check is a tour of web consoles.

There is also a trust problem. When a community member shared an AI-agent
management layer for Proxmox on the same forum ([June 2026
thread](https://forum.proxmox.com/posts/858319/)), the replies were blunt:
one administrator would not use an AI agent on their hosts at all, and another
argued that non-deterministic tools do not belong next to core
infrastructure. An agent that needs a token able to stop or delete a VM does
not get connected.

## What this skill does about it

It asks Proxmox only questions. Every request it sends is a read, so nothing it
runs can change a cluster or a backup, and the answers are joined per guest
across every client cluster:

- `posture --only problems` - every guest that is uncovered, has no restore
  point, is stale, failed verification, or whose last attempt failed
- `backup coverage --include-disks` - every guest or disk no enabled job
  includes, with the reason
- `failures --since 24h` - failed and silently incomplete backup, verify, GC,
  sync and prune tasks, one row per guest or datastore with the error line
- `verify-health` - how far a verify failure spread, and the newest
  verified-good point that is still there
- `capacity --within 30d` - datastores filling up, and the ones too full for
  garbage collection to run
- `rpo --days 30 --by-client` - per-client proof that every scheduled run
  produced a restore point, for a QBR or an audit

## Status

Beta. Validated against the Proxmox VE and Proxmox Backup Server API surface
with mocked responses; the closed-loop receipt (a named MSP running it live
against their own clusters at a Build Session) is tracked separately and added
here as `video.md` once it exists.
