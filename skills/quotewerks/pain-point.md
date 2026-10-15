# quotewerks skill - the MSP pain it closes

## The pain

An MSP account manager who runs account management and vCIO work on QuoteWerks,
with ConnectWise PSA alongside, posted a request on the QuoteWerks support forum
for an official MCP server
([Official MCP server for QuoteWerks Web](https://tickets.quotewerks.com/support/discussions/topics/9000064305)).
The post lists four jobs that eat the day:

- **Quoting from an inbound email.** A client writes "we need 12 more laptops
  like the ones you sent in March, plus docks." Today that means opening
  QuoteWerks, hunting down the March quote to find out which laptops they were,
  and hand-keying every line on the Document Items tab.
- **Quote status.** "Where does the Acme quote stand?" means switching to
  QuoteWerks and running a Document Lookup.
- **Repricing.** When distributor cost moves, someone opens the document and
  touches each line. The quotes nobody gets to go out at a thinner margin than
  anyone intended.
- **Reporting.** Margin and win-rate analysis for QBRs means writing a script
  per question.

The same post explains why the QuoteWerks Web REST API (beta) does not already
solve it: the problem is shape, not access. It is generic table access, so an AI
handed a raw table endpoint has to guess the schema, which table holds line
items, and what the fields are called, and it guesses wrong in ways that quietly
produce a bad quote. The post also asks for the guardrails an MSP needs before
letting an assistant near quotes: read-only by default, explicit confirmation
before a write, and nothing the logged-in user could not do anyway.

## What this skill does about it

Typed commands that use the real QuoteWerks column names, writes that preview
until you confirm them, and a local mirror for the cross-document questions:

- **`quotewerks-cli requote "1042" --qty 20UES0X100=12`** - copies the March
  order into a new quote with new quantities and today's product-database
  prices, and shows the whole quote before anything is created. Add `--confirm`
  to create it.
- **`quotewerks-cli docs search --customer "Acme" --type QUOTE`** - quote status
  from the window you are already in, with QuoteValet acceptance decoded.
- **`quotewerks-cli cost-drift --threshold 2`** - every open-quote line whose
  quoted cost no longer matches your product database, with margin at the old and
  current cost. Then **`quotewerks-cli items reprice "1042" --part 20UES0X100 --from-products`**
  shows old versus new cost, price and margin for each line before it changes
  anything.
- **`quotewerks-cli docs lint "1042" --min-margin 10`** - catches lines priced
  below cost, missing costs, bad quantities and thin margins before the customer
  sees the quote.
- **`quotewerks-cli margin --by rep --since 90d`** and
  **`quotewerks-cli winrate --by rep`** - QBR answers from the local mirror, one
  command each.

Two of the guardrails the post asked for come with it. A key without Allow
write access cannot change anything (QuoteWerks answers writes with 403), and
with a write-enabled key the typed writes still only preview until `--confirm`.
The raw `tables` writes send immediately, so keep the key read-only unless
you need writes.

## Status

Beta. Built from the vendor's published API spec and documentation and tested
against a local stand-in for the API; no live QuoteWerks Web tenant has run it
yet. The closed-loop receipt (a named MSP running it live in their production
tenant at a Build Session) is tracked separately and added here as `video.md`
once it exists.
