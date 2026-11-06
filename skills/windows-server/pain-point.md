# windows-server skill - the MSP pain it closes

## The pain

Every client has a few Windows servers that nobody watches closely until something
breaks: a file server, a line-of-business app server, a domain controller, a Hyper-V
host. The standard advice is a manual round. The
[Active Directory Pro Windows Server maintenance checklist](https://activedirectorypro.com/windows-server-maintenance-checklist/)
(Matt Roberts) has you "monitor critical server services" every day, review disk space
and services every week, and check Event Viewer's application, security and system logs
"for any critical errors", with security updates monthly and SSL certificates reviewed
twice a year. Done by hand, that is an RDP session per server, per client, and the
checks that are only twice a year (certificates) are the ones that cause the 7 a.m.
"our site is down" call.

When a server does break, the first hour goes to working out what changed: a service
that stopped (and whether it crashed or was stopped by someone), an error that has never
fired on that server before, a new local administrator, or a role that was removed.

## What this skill does about it

- `windows-server-cli health --agent`: one ok, warn, crit or unknown verdict per
  configured server across pending reboot, disks, stopped automatic services, System log
  critical and error events (24 hours), patch age, pending updates, expiring certificates,
  Defender, time sync, failed scheduled tasks and Hyper-V VMs, with the reason for every failing check and how long it has failed.
- `windows-server-cli services down --agent`: each stopped automatic service paired with
  the Service Control Manager event that explains the stop, so a down ticket starts with
  the cause, not a restart.
- `windows-server-cli eventlog new --window 24h --baseline 7d --agent`: errors and
  warnings whose source and event ID are new on that server, separating a new failure
  from the error that fires every night.
- `windows-server-cli local-admins changes --hours 168 --agent`: who added or removed
  each local Administrators member, and when.
- `windows-server-cli inventory snapshot` on a schedule, then
  `windows-server-cli inventory diff --agent`: what changed in roles, features, shares,
  local administrators, certificates, installed updates and VMs between runs.

It connects over WinRM with HTTPS and certificate verification always on, runs only a
fixed set of read scripts that ship inside the binary, and changes nothing on a server
except restarting or starting one service after a preview you confirm.

## Status

Beta. Verified against a Windows Server 2022 test server we run (every command, live);
not yet run by an MSP against a client's production servers. The closed-loop receipt (a
named MSP running it live at a Build Session) is tracked separately and added here as
`video.md` once it exists.
