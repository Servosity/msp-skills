> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/governance/

TRUST CENTER · FOR THE SECURITY-NATIVE MSP

# Is it safe to give an AI agent access to my PSA?

Yes - when access is scoped, and these connectors are built to scope it. Three things make that true: every command is tiered **read / write / destructive** so an agent only does what its tier allows; the strongest control is **server-side API scoping** your own tenant enforces; and the whole supply chain is **open source with no telemetry and no servers of ours**, gated in CI. Here is each layer in plain terms.

## The command tiers: what an agent may do on its own

Every connector ships a `governance.md` that tags every command into one of three tiers. The recommended agent policy maps each tier to how much autonomy it gets:

- **Read** - listing tickets, pulling a client card, running analytics. Changes nothing. Safe to run autonomously; this is where the day-to-day value lives.
- **Write** - creating a ticket, updating a record, posting a note. The recommended rule is to **preview with `--dry-run` and require your approval** before the agent commits any mutation.
- **Destructive** - deleting records, bulk operations that can't be undone. **Human-in-the-loop:** the agent surfaces what it would do and waits for a person.

This is a recommended policy your agent reads, not a hard sandbox - which is exactly why the next layer matters more.

## The strongest control: scope the key in your own tenant

The tiers above guide the agent. **Server-side API scoping does not depend on the agent behaving at all.** You issue the API key inside your own PSA, RMM, or backup tenant, and you scope it there - read-only, a single module, one client, whatever your vendor supports. The connector can never exceed what your tenant granted the key, no matter what an agent asks it to do.

This is the control a security-native buyer should lean on hardest: enforcement lives on **your** side of the wire, in a system you already trust to gate your techs.

## The supply-chain answer, head-on

A connector is code you run on your machine, so the fair question is: can I trust it?

- **Open source, inspectable.** Every line is on GitHub under Apache-2.0. Read it, fork it, pin a commit.
- **No telemetry. No servers of ours.** Nothing phones home. There is no hosted service in the path - out of the box the binaries talk only to your vendor's API with your credentials. The paths that can send data anywhere else are ones you switch on and point at a URL of your own: `--deliver webhook:<url>`, a feedback endpoint you set, and the `--transport http` MCP listener on the connectors that have it.
- **CI gates on every change.** `gitleaks` scans for leaked secrets, **DCO sign-off** attributes every commit, and a **claims gate** proves the docs only describe commands the real binary actually has - so the install instructions can't tell you to run something that doesn't exist.
- **Hand-fixes survive regeneration.** Connectors are generated, then carry hand-fixes for live-API quirks. Those fixes are recorded in a **checked-in ledger** that CI verifies, so a regeneration can't silently drop a security or correctness fix. [How fixes outlive a reprint →](https://msp-skills.compoundingteams.com/reprint-survival.md)

## What an agent sees - and doesn't

When your AI answers a question, it sees the **result of a query**, not a raw dump of your database. A "stale backups across all clients" question returns the short list, not every backup record. Your credentials are read from your environment or your agent's config; they are never written into the repo or sent anywhere by us.

## Start read-only

The safe first move is a **read-tier triage against one client**: nothing changes, you see real output, and you decide from there. Then widen scope - and scope the API key in your tenant before you ever let an agent write.

Ready to look closer? [Browse the connectors →](https://msp-skills.compoundingteams.com/skills/) · [Why this layer at all →](https://msp-skills.compoundingteams.com/why.md) · [See what's been verified live →](https://msp-skills.compoundingteams.com/verified/)
