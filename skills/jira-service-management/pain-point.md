# jira-service-management skill - the MSP pain it closes

## The pain

MSPs that run their service desk on Jira Service Management (JSM) live in three
places at once: the queues, the SLA timers, and Assets (the CMDB). None of them
answers the questions a service manager asks at 8am.

- **Internal notes reach customers by accident.** Atlassian's own public tracker
  carries the request
  ([JSDCLOUD-17210](https://jira.atlassian.com/browse/JSDCLOUD-17210), "Need clearer
  distinction between internal and external comments on issue transition
  comments"): agents "don't know if they are making an internal or external facing
  comment and may accidentally have selected the wrong one." An Atlassian Community
  thread titled "ROVO posted a comment to JSM as Public instead of Internal" shows
  the same failure with an AI agent. When the comment is about a client's breach or
  a tech's mistake, one wrong click is a client-relationship problem.
- **No single API call ranks tickets across queues by SLA.** "Which tickets breach in
  the next two hours" means listing every queue, de-duplicating tickets that sit in
  several, and reading each ticket's SLA metrics (`/request/{id}/sla`).
- **Each client is a JSM organization, kept in shape by hand.** Atlassian's guide to
  [supporting multiple clients with a single Jira site](https://support.atlassian.com/jira-service-management-cloud/docs/support-multiple-clients-with-a-single-jira-site/)
  groups each client's people into organizations. Staff who land in no
  organization, or in two, see the wrong tickets or none, and nobody notices until
  the client complains.
- **Assets only knows what someone imported.** Nothing in JSM compares the CMDB with
  the RMM or PSA inventory, so a server missing from Assets is found during an
  outage.

## What this skill does about it

- `jira-service-management-cli request note SD-123 "Escalated to network team"` -
  posts with public=false every time, and exits with an error if JSM stores the
  comment as public. An internal note stays internal.
- `jira-service-management-cli request timeline SD-123` - one chronological view
  with every comment labeled PUBLIC or INTERNAL, plus status history, approvals,
  participants and SLA state, so the agent reads the internal notes before it
  replies.
- `jira-service-management-cli breaches --within 2h` - walks every queue once,
  de-duplicates tickets, and ranks them by the least SLA time remaining, breached
  first.
- `jira-service-management-cli roster 3` - flags client staff in no organization or
  in two, empty organizations, and organizations holding the desk's customers that
  are not attached to the desk.
- `jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS` -
  diffs an Assets object type against an RMM or PSA export: missing from Assets,
  missing from the inventory, and attribute mismatches.

## Status

Beta. Validated against the Jira Service Management API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
