# fortinet skill - the MSP pain it closes

## The pain

FortiGate is a common firewall at the small businesses MSPs look after, and
its SSL-VPN has become the part of the estate that gets attacked
first. In February 2024 Fortinet disclosed CVE-2024-21762, an out-of-bounds
write in the SSL-VPN daemon that an unauthenticated attacker could reach with
crafted HTTP requests, and said it was "potentially being exploited in the
wild" ([Rapid7 analysis](https://www.rapid7.com/blog/post/2024/02/12/etr-critical-fortinet-fortios-cve-2024-21762-exploited)).
Fortinet's advice was to install the fixed release for each branch (7.4.3,
7.2.7, 7.0.14, 6.4.15, 6.2.16) or disable SSL-VPN until you could. The
MSP-facing advisory from SmarterMSP put it the same way: "we strongly recommend
you upgrade to the latest available version of your current release branch"
([SmarterMSP, 13 Feb 2024](https://smartermsp.com/cybersecurity-threat-advisory-critical-vulnerability-in-fortios/)).

That advice assumes you can answer three questions across every client the
same morning: which FortiGates run SSL-VPN at all, which of those listen on a
WAN interface open to any source, and which are below the fixed build for their
own branch. FortiOS answers each question one box at a time, in its GUI or its
REST API. Without a FortiManager, the answer is a login per client.

The same shape repeats for the work between advisories. FortiGuard and FortiCare
expiry dates live on each device, so the renewal queue is a spreadsheet. Admin
accounts without trusted hosts, super_admin without two-factor, and HTTP or
Telnet left on a WAN interface are each a separate screen. A firewall policy
that is any-any accept, or has not matched a packet in 90 days, is invisible
until someone goes looking.

## What this skill does about it

- `fortinet-cli audit sslvpn` grades every FortiGate and VDOM: FAIL when SSL-VPN
  is enabled on a WAN-role interface with source address any (or address objects
  and groups that together cover every address), WARN when it is on WAN but
  restricted, UNKNOWN when the device did not establish the interface's exposure
  or a named source address could not be resolved.
- `fortinet-cli audit firmware --min-version 7.2.10,7.4.5` takes one floor per
  branch and flags every box below it, plus how many patches each is behind the
  newest image offered in its branch.
- `fortinet-cli audit licenses --within 60d` turns per-device FortiGuard and
  FortiCare expiry into one renewal queue, soonest first.
- `fortinet-cli fleet audit --new` runs all four checks across the fleet and
  shows only the findings the previous run did not have; `--fail-on fail` makes
  it exit 6 for a scheduled job.
- `fortinet-cli policies hygiene` lists any-any accept, unused, logging-disabled
  and disabled policies per VDOM.

Every one of these reads the FortiGates live, read-only. A box the CLI could not
read is reported UNKNOWN, never PASS, so a silent gap cannot look like a clean
bill of health.

## Status

Beta, awaiting live verification. Built from a community-authored description
of FortiOS 7.4 REST endpoints (derived from Fortinet's public Ansible
collection, not an official Fortinet specification) and tested against simulated FortiGate responses, not a real
FortiGate. The closed-loop receipt (a named MSP running it against their own FortiGates at a
Build Session) is tracked separately and added here as `video.md` once it exists.
