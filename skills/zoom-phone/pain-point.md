# zoom-phone skill - the MSP pain it closes

## The pain

An MSP that sells or manages Zoom Phone runs one Zoom account per client, and
every account has its own admin portal. The questions that matter cut across
pages the portal keeps apart. The most serious one is emergency calling: Zoom's
own [emergency services best practices](https://library.zoom.com/zoom-workplace/zoom-phone/expert-insights/emergency-services-best-practices/regulatory-considerations-in-the-united-states)
note that in the United States the RAY BAUM's Act requires a dispatchable
address to be sent with emergency calls, and that a default address should be
set for each user and device. Checking that by hand means opening every user,
common-area phone and site in every client tenant.

Routing breaks the same quiet way: an IVR key that points at a deleted queue, or
a queue with nobody set to receive calls, is found by a customer whose call went
nowhere. And offboarding one employee means removing them from call queues,
shared lines, pickup and monitoring groups, desk phones, numbers and the calling
plan, each on a different page. The community
[zoom-phone-mcp](https://github.com/FujitsuPolycom/zoom-phone-mcp) project
documents a further trap: the legacy phone admin scopes are not enough for some
call-handling and settings endpoints.

## What this skill does about it

- `zoom-phone-cli e911 audit` - users, common areas and sites whose emergency address is missing or not verified, sites with no emergency service location, and BYOC numbers that need a carrier update.
- `zoom-phone-cli routing audit` - dead IVR targets, call queues with nobody to answer, orphaned auto receptionists and auto receptionists with no upcoming holiday hours.
- `zoom-phone-cli routing trace +14155550123` - where one number really rings, for business and closed hours.
- `zoom-phone-cli offboard jane@example.com` - the ordered removal plan for a departing user; `--apply` runs it after a live re-check, with no further prompt in the CLI (through MCP it also needs `confirm: true`).
- `zoom-phone-cli fleet -- e911 audit` - the same audit across every client account saved with `accounts connect`, one row per account.

## Status

Beta. Verified offline against the Zoom Phone API specification and a local
test server; awaiting live verification by an MSP against a production Zoom
Phone account.
