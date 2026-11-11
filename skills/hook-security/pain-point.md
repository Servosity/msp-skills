# hook-security skill - the MSP pain it closes

## The pain

Security awareness questions are short to ask and slow to answer across a book of
clients. Hook Security's own
[announcement of its API and MCP connection](https://www.hooksecurity.co/blog/hook-mcp-server)
says so: the answers are "spread across a campaign page, a report and a training
list", and an MSP wants "one question across every client, instead of opening each
one."

The quarterly review is where that adds up.
[Hook's QBR template for MSPs](https://www.hooksecurity.co/blog/msp-qbr-template-security-awareness-results-15-minutes)
asks for click rate, training completion and report rate with their trends, for every
client, every quarter, and warns that "if you're pulling these numbers manually, your
platform is doing only half the job."

## What this skill does about it

- `hook-security-cli client-risk` - every client ranked by click rate, with a fleet
  rate computed from total clicks over total sent.
- `hook-security-cli clicked-untrained --campaign cmp_88213` - the follow-up list:
  who clicked and has not finished training, with each course's status and due date.
- `hook-security-cli overdue-training --counts-only` - which clients have people
  overdue, without names or emails.
- `hook-security-cli qbr org_4521 --days 90` - one client's totals, monthly trends,
  recent campaigns and overdue training for the window.

## Status

Beta. Built from Hook's published OpenAPI document and tested against a local fake
server shaped from it. It has not yet been run against a live Hook account; the
closed-loop receipt (a named MSP running it live in their production tenant at a Build
Session) is tracked separately and added here as `video.md` once it exists.
