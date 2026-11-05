# blackpoint skill - the MSP pain it closes

## The pain

Blackpoint Cyber's CompassOne is a managed detection and response platform sold through MSPs, and it is built tenant by tenant. Every detection, asset, vulnerability, scan and report call in the CompassOne API names one customer tenant (the `x-tenant-id` header), and the portal views follow the same shape. Reviews of Blackpoint for MSPs describe the operator's side of that: [Flamingo's Blackpoint Cyber review](https://www.flamingo.run/blog/blackpoint-cyber-review) notes reviewers find the partner portal "less polished than the detection engine behind it, with a learning curve on reporting and day-to-day navigation."

For an MSP with 30 or 80 customers, the questions that matter most span tenants: which open detection has waited longest anywhere, which endpoints stopped reporting, which customers have no recent scan or nobody to escalate to, and which exploited CVE to patch first. Answering them means opening every tenant in turn.

## What this skill does about it

- `blackpoint-cli fleet-sync` - pulls every tenant's detections, devices, vulnerabilities, scans, dark-web exposure flags, posture, contact groups and Cloud Response settings into one local mirror (passwords are never stored), and records each fetch that failed so no reader mistakes missing data for a clean result.
- `blackpoint-cli triage --older-than 24h` - one queue of open detections across every client, oldest first.
- `blackpoint-cli stale-agents --days 14` - devices whose agent went quiet, across every tenant.
- `blackpoint-cli gaps --scan-days 30` - customers with no recent completed scan, no or empty escalation contact group, or no posture rating.
- `blackpoint-cli cve-spread --exploited` - exploited CVEs ranked by how many customers and assets they affect.
- `blackpoint-cli client-report "Acme Dental" --since 30d` - one customer's reporting-window numbers from six live CompassOne endpoints in a single call.

## Status

Beta. Validated against the Blackpoint Cyber API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
