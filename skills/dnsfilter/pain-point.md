# dnsfilter skill - the MSP pain it closes

## The pain

DNS filtering generates two kinds of tickets for an MSP, and both are chains of
lookups rather than one question.

**"Why can't I get to this site?"** The answer depends on which policy the
client's site or roaming client is using at that moment (a scheduled policy
switches by the quarter hour, and a roaming client without its own assignment
inherits its site's), that policy's allow list, block list and blocked
categories, and what the Query Log recorded. Then the fix has to land on the
policy that actually blocked it, not on a shared global policy that opens the
site for every other client. Users report the gap between what the dashboard
says and what devices do: a DNSFilter Community post titled "Policy enforcement &
reporting discrepancies with Roaming Client" describes a roaming client on a
second site ignoring its category blocks while reports showed the site as
blocked. End-user unblock requests add a queue on top, one client at a time.

**"Which machines are on the bad agent?"** Roaming client releases go wrong
across the whole book at once. DNSFilter's own status page logged the
[Windows Roaming Client 1.7.15 auto-upgrade incident](https://status.dnsfilter.com/incidents/vc2qwqtx2sxz)
(June 2022): auto-upgrade was paused, the release was pulled from general
availability, and the workaround was to uninstall and reinstall the client.
DNSFilter's help center documents another:
[internet connectivity lost when updating the macOS Roaming Client from 1.x to 2.2](https://help.dnsfilter.com/hc/en-us/articles/41871975449363-Internet-connectivity-lost-when-updating-macOS-Roaming-Client-1-x-x-to-2-2-x).
Each time, the first question is which clients, and which machines, are on the
affected build, and the dashboard answers it one organization at a time.

## What this skill does about it

- `dnsfilter-cli why-blocked "example.com" --org "Acme Dental"` - names the policy
  and the rule that blocked or allowed the domain for that client right now,
  with the recent Query Log hits, and says which rule types it could not see.
- `dnsfilter-cli allow "example.com" --org "Acme Dental" --note "TKT-4821"` -
  plans the allow on the policies that actually blocked it, refuses threat
  categories, skips shared global policies; `--confirm` writes and reads back.
- `dnsfilter-cli unblocks triage` - the pending unblock queue across every client,
  grouped by domain, with threat flags and DNSFilter's own already-covered check.
- `dnsfilter-cli agents drift --bad-version 1.7.15 --agent-type windows` - every
  client org with roaming clients on the pulled build, against the live release feed.
- `dnsfilter-cli agents stale --older-than 7d` - roaming clients that went silent
  or are unprotected or bypassed, grouped by client org.

## Status

Beta. Validated against the DNSFilter API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
