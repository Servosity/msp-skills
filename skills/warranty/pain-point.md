# warranty skill - the MSP pain it closes

## The pain

"Which of this client's machines are out of warranty?" comes up at every QBR and every
hardware failure, and the answer lives in three vendor portals. Dell's own community has
asked for a bulk answer for years: "Is there an easy way to enter multiple service tags to
check the warranty status on all of my hardware?"
([Dell Community, Checking Multiple Service Tags](https://www.dell.com/community/Latitude/Checking-Multiple-Service-Tags/td-p/8194966), May 2022). One reply
had over 1,000 computers to check; the accepted answer was to request a TechDirect API key
and run scripts.

For an MSP the same question repeats per client and per vendor: Dell, Lenovo and Apple each
need their own credentials, the results end up in a spreadsheet that is stale by the next
QBR, and the devices nobody could verify quietly drop out of it.

## What this skill does about it

- `warranty-cli import devices.csv --customer "Acme Dental"` - load any RMM or asset export
  (serial, optional manufacturer, model, hostname) into the mirror under the client's name.
- `warranty-cli lookup --file devices.csv --customer "Acme Dental"` - look up every serial
  with whichever vendor credentials you have; vendors without credentials are reported, not
  called.
- `warranty-cli report --within 90d` - every device out of warranty or expiring soon, per
  client, from the mirror.
- `warranty-cli lifecycle --quarters 4 --age 4y` - coverage endings per client by quarter,
  plus devices past their age, for refresh planning.
- `warranty-cli unresolved` - every device the report cannot speak for, each with one reason
  and the fix.

## Status

Beta. Built and tested offline from each vendor's documented request and response formats
(Dell TechDirect, Lenovo eSupport, Apple Business); no live vendor account has run it yet.
The closed-loop receipt (a named MSP running it live in their production tenant at a Build
Session) is tracked separately and added here as `video.md` once it exists.
