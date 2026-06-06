# hubspot skill - the MSP pain it closes

## The pain

**Monthly customer reports against HubSpot lose data the moment a status changes.** An MSP runs Sales Hub for client outreach and deal tracking. At report-time the question is "how many meetings did we have with this customer that were Scheduled in April?" - but by month-end every one of those meetings has flipped to `Completed` or `No Show`. HubSpot's UI shows current state only. The standard CRM search/filter API can only filter on current property values. The property-history endpoint (`?propertiesWithHistory=`) can answer it, but no public HubSpot CLI or MCP exposes it in a queryable shape.

This is a long-running operator request - see the [HubSpot Community thread "Enable retrieval of property history in the V3 APIs"](https://community.hubspot.com/t5/APIs-Integrations/Enable-retrieval-of-property-history-in-the-V3-APIs/m-p/496344) and recurring threads on [r/hubspot](https://www.reddit.com/r/hubspot/) about needing historical state for monthly reports. The same shape applies to: deals that were ever in stage X this quarter, contacts that were ever marketing-qualified before they unsubscribed, tickets that were ever assigned to a rep who has since left.

HubSpot announced their own [Agent CLI on 2026-05-27](https://blog.hubspot.com/marketing/introducing-the-hubspot-agent-cli) (stateless, live-API). It does not address the property-history gap either: their [public skills repo](https://github.com/HubSpot/agent-cli-skills) has zero references to `propertiesWithHistory`. The gap stays open without this skill.

## What this skill does about it

Five commands an MSP owner can run from a single AI prompt:

1. **`hubspot-cli sync --resources hubspot-meetings-crm --with-history hs_meeting_outcome,hs_meeting_title,hubspot_owner_id`** - captures property snapshots into a local `hubspot_property_history` table during sync. Sync is paged and rate-limit-aware; `--with-history` requests `propertiesWithHistory`, which HubSpot's batch-read API caps per request, and the CLI pages accordingly.
2. **`hubspot-cli meetings status-report --status scheduled --month 2026-04 --csv > april.csv`** - the customer report itself: every meeting that was Scheduled at any point in April, with current outcome breakdown. One command replaces a Python pull + a HubSpot export + a manual cross-reference.
3. **`hubspot-cli meetings ever-had --property hs_meeting_outcome --value Scheduled --from 2026-04-01 --to 2026-04-30 --json`** - the underlying query for ad-hoc investigation. Returns meeting ID + title + owner + first-hit timestamp + current outcome.
4. **`hubspot-cli pipeline-health "default" --idle-days 14 --json`** - the rest of the QBR: per-stage rollup with weighted value (Closed Lost = 0, Closed Won = 1.0), $ at risk for idle deals, oldest stuck deal per stage. Read-only against the local mirror.
5. **`hubspot-cli engagements of deal:<id> --since 90d --json`** - full cross-object activity trail (calls + emails + meetings + notes + tasks) for any deal that comes up in the customer conversation. One query replaces five paginated API calls.

## Status

Beta. Validated end-to-end against a live HubSpot tenant (sync of 50 meetings produced 2 ever-SCHEDULED rows with real title + owner_id + first-hit timestamp + current outcome). Validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions). The closed-loop receipt (a named MSP customer running it against a production HubSpot portal to produce an actual monthly report) is tracked separately and will be added here as `video.md` once it exists.
