# synology skill - the MSP pain it closes

## The pain

Synology Active Backup for Business is a common way MSPs back up small-business
PCs, servers and VMs: the backup software is free with the NAS, and the NAS sits
at the client's site. The catch shows up at scale. Every NAS is its own island,
with its own web UI, its own task list and its own notification emails.

An MSP with almost a hundred NAS units in production at its clients put it
plainly on the French NAS community forum (translated here from French): the
email notifications had become too hard to manage, nothing alerted on a backup
that had not run for a number of days, and neither Synology Active Insight nor
CMS monitored Active Backup or Hyper Backup
([nas-forum.com, Nov 2021](https://www.nas-forum.com/forum/topic/73029-monitoring-multiple-active-backup-for-business)).
Active Insight added Hyper Backup monitoring a few weeks later; it still lists
no Active Backup for Business monitoring.
The replies are the usual workarounds: SNMP (another member answered that
Synology's MIBs do not expose backup state), and an SSH script that reads Active
Backup's internal SQLite files on every NAS into a dashboard someone has to
build. Tom Lawrence's 2023 review of Active Backup for Business reached the
same conclusion from the MSP side: it "still lacks features today that make it
less ideal for use as an MSP", "mostly because of their lack of central
management"
([Lawrence Systems forum, Jan 2023](https://forums.lawrencesystems.com/t/synology-active-backup-for-business-review-2023/16543)).

Synology's own tools cover part of this. Active Insight, its cloud monitoring
service, can be delegated to an MSP and watches storage pool status, volume
capacity forecasts and Hyper Backup across hosts. ActiveProtect Manager centrally
manages NAS units running Active Backup for Business, but it needs an
ActiveProtect appliance as the management server. For an MSP with an ordinary
NAS at each client, the per-device Active Backup questions asked every morning
are still one web UI per client: which machines failed last night, which have
had no good backup in three days, and which task protects this laptop.

## What this skill does about it

- `synology-cli fleet sync` logs in to every client NAS you register (each with
  its own login, two-factor accounts included) and mirrors Active Backup tasks,
  job results, per-device runs and a Storage Manager snapshot into one local
  database.
- `synology-cli fleet status` gives one row per client NAS: backup outcomes,
  stale devices, worst storage state and how old the mirror is. A NAS that did
  not sync reads as unknown, never as OK, and the command exits 6 when any NAS
  needs attention, so an RMM script can alert on it.
- `synology-cli abb failures --since 24h` lists every task whose latest run in
  the window failed, partially succeeded or never ran, with the first error line.
- `synology-cli abb stale --older-than 72h` lists every protected PC, server
  and VM with no successful backup in three days, including ones that never had
  one.
- `synology-cli fleet health` finds degraded volumes and pools, disks with bad
  SMART status, hot disks and nearly full volumes, with days to full when the
  mirror has enough history.
- `synology-cli abb triage "Office PCs" --nas acme` explains one failed task: its newest
  failed run, that run's detail lines and the Active Backup log from that window.

## Status

Beta, awaiting live verification. Built from Synology's public DSM WebAPI
documentation and community-documented Active Backup for Business calls
(Synology publishes no Active Backup API specification), and tested against
simulated DSM responses, not a real NAS. The closed-loop
receipt (a named MSP running it against their own client NAS units at a Build
Session) is tracked separately and added here as `video.md` once it exists.
