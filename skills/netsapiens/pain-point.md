# netsapiens skill - the MSP pain it closes

## The pain

An MSP that resells hosted voice on a NetSapiens platform (Crexendo, OITVOIP, PacketDial and other white-label providers) runs dozens of client domains, and the reseller portal answers one domain at a time. The questions that matter each month span every client: who is over their seat limit before invoicing, which desk phones dropped off, which numbers still point at a user who left, and where emergency caller ID or address data is missing.

E911 is where that gap costs money and liability. [IPComms' E911 terms](https://www.ipcomms.net/legal/e911/), from a SIP trunking carrier, bill an unregistered-911 fee of $250 per call placed from a number with no registered address. Kari's Law and RAY BAUM's Act put the dispatchable-location duty on whoever runs the phone system, which for a hosted-voice MSP is the MSP.

MSPs want their AI agents to reach NetSapiens: [phoneware/netsapiens-mcp](https://github.com/phoneware/netsapiens-mcp) and [MSPbots' oitvoip-mcp](https://github.com/MSPbotsAI/oitvoip-mcp) both exist. Both answer one live API call at a time, so a question across every client domain is still out of reach.

## What this skill does about it

- `netsapiens-cli fleet seats --flagged`: every client over or near a user, queue, auto attendant or conference limit, ready for the monthly true-up.
- `netsapiens-cli fleet offline`: unregistered, stale or high-latency phones, found before the client calls.
- `netsapiens-cli fleet dids --problems-only`: numbers whose routing ends at a missing user, queue, attendant or device.
- `netsapiens-cli fleet e911`: users, devices and domains with no emergency caller ID, or an emergency address that does not exist, as input to a Kari's Law / RAY BAUM's Act review (it checks the data; it does not test 911 dialing).
- `netsapiens-cli fleet changes`: users, devices, numbers and domains added, removed or re-pointed since the last check.

## Status

Beta. Validated against the NetSapiens API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
