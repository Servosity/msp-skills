# threecx skill - the MSP pain it closes

## The pain

MSPs and VoIP resellers run one 3CX phone system per client, and every tool 3CX
ships, from the admin console to its MCP preview, looks at one PBX at a time. The
questions an MSP actually asks are about all of them: which client's SIP trunk is
down, who is still on an old build, whose licence or maintenance lapses this month,
whose backups stopped. Today that means logging into each console, or finding out
when the client calls.

Resellers build their own answer: in the 3CX forum thread
[Centralized monitoring & management for your 3CX instances](https://www.3cx.com/community/threads/centralized-monitoring-management-for-your-3cx-instances-and-more.135446/)
(October 2025), a 3CX partner describes an app that checks every instance each
minute and opens tickets on problems, and older threads such as
[Monitoring trunk status](https://www.3cx.com/community/threads/monitoring-trunk-status-3cx.56799/)
ask how to watch trunks at all.

## What this skill does about it

- `threecx-cli fleet health --agent`: one row per client PBX, pass, warn, fail or
  unknown, with the reason for each (version, licence, backup, trunks, disk,
  services, capacity). A missing fact is reported as unknown, never as healthy.
- `threecx-cli fleet trunks --offline --agent`: every trunk that is offline,
  misconfigured or unknown on any client, with certificate days left.
- `threecx-cli fleet licences --within 60d --agent`: the renewals pipeline, soonest
  first, with edition and seat headroom.
- `threecx-cli fleet backups --problems --agent`: stale, unscheduled or shrinking
  backups across the fleet.
- `threecx-cli fleet changes --since 24h --agent`: what moved since yesterday's run.

## Status

Beta. Validated against the 3CX API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
