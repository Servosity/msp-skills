# salesbuildr skill - the MSP pain it closes

## The pain

Quoting is where MSP revenue leaks quietly. The proposals go out fast; the
follow-up doesn't. On a lean team nobody owns "chase the quotes I sent three
weeks ago," so a recurring r/msp theme is sent proposals that simply age out -
real pipeline that expires because no one circled back. The money was on the
table; it just went cold.

The second leak is margin, one line at a time. A tech adds a part at cost and
forgets to apply the markup, or a recurring item gets quoted under floor. Nobody
notices on a single quote - it surfaces at the QBR, after it has already
shipped at a loss.

The third is drift. Per-company pricing books slowly diverge from the master
catalog until two clients pay different prices for the same SKU and no one
decided that on purpose. And underneath all of it, records that never got an
external identifier silently fall out of the Autotask/ConnectWise sync, so the
PSA and the quoting tool quietly disagree about who the customers even are.

Salesbuildr's portal shows you one quote, one opportunity, one pricing book at a
time. None of these leaks is visible in a single record - they only show up when
you look *across* every open quote, every line, every client. That's the view
the portal makes you assemble by hand in a spreadsheet.

## What this skill does about it

It syncs Salesbuildr into a local mirror, then answers the cross-record questions
in one command:

- **`salesbuildr-cli quote stale --days 14`** - every sent/approved quote aging
  past your cutoff, with the dollar value still at risk. The follow-up list that
  was never anyone's job.
- **`salesbuildr-cli quote thin --floor 20`** - every open line priced below your
  markup floor, caught before it ships, not at the QBR.
- **`salesbuildr-cli pricing drift`** - per-company prices that have diverged from
  the master catalog cost or price.
- **`salesbuildr-cli company whitespace "Acme Managed IT"`** - catalog products a
  client has never been quoted: the cross-sell you keep meaning to make.
- **`salesbuildr-cli reconcile-psa`** - companies, contacts, products, and
  opportunities missing the external ID your PSA sync depends on.

## Status

Beta. Validated against the Salesbuildr API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
