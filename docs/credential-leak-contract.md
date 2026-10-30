---
layout: default
title: "Credential leak contract - what every connector must never print, store or send | MSP Skills"
description: "The fixed rules every MSP Skills connector and its review are judged against: which values count as held credentials, what counts as a leak (a whole value at any length, pieces of 8 or more characters, credentials sent off their declared hosts), the closed list of encodings checked, public text, and the cases that are tracked but never block."
permalink: /credential-leak-contract/
last_modified_at: 2026-10-29
---

# Credential leak contract

Owner-ruled 2026-10-29. This is the fixed target for every credential check and every review of a connector under
`skills/*/cli` and of the shared credential kit (`tools/maintainer/credkit`). A review judges credential findings against
this document. A finding outside it is P2 at most, unless it shows a held credential leaving through a sink this document
covers in a reading section 3 names. Changing a rule here needs the owner; a review cannot.

Amended 2026-10-29, owner-approved: section 3 is a closed list, the budget is an invariant with no clean exit, public
text is defined and used only for collisions (it exempts no piece), and the kit's matcher is the one implementation
(section 8).

## 1. Held credentials

A held credential is every value the program uses or receives as a secret:

- each environment variable the connector declares secret (from the connector's declarations, never guessed from names);
- each credential field of the config file, including a file that does not parse (read token by token), legacy config
  paths, `[headers]` values, `auth_header`, and a password or credential query value inside `base_url`;
- credential flags and credentials read from stdin;
- credentials in outgoing requests: auth headers, cookie values, the user and password parts of Basic auth, and form or
  JSON credential fields;
- tokens minted at run time, `Set-Cookie` values in response headers or trailers, and the vendor response fields the
  connector declares as credentials;
- MCP tool arguments that carry a credential field.

Rules for the held set:

- A value known at startup (environment, config file, flags, stdin) is held before the first byte of any output,
  including flag-parse errors and startup warnings.
- A value acquired while running (a minted token, a `Set-Cookie` value, a declared response field, an MCP argument, a
  credential placed in an outgoing request) is held before the response, call or request that carries it reaches any
  sink.
- No exemption for length, strength or type. Numbers count.
- A value is exempt only when the connector's ledger declares it an identifier, with a reason, or when it is a value
  acquired while running that occurs, as written, in public text (section 2).
- Every member of a duplicate key is held.
- Values stay held until the process exits.
- A child process gets the held set or is not started.

**Public text** is fixed text a connector emits whether or not it holds any credential. It is used only for the
collision rules of section 2 and exempts no piece. It is exactly the entries of one public-text table in the kit's
source. Nothing outside that table is public text, and a table entry that is not one of these kinds breaks this clause:

1. The fixed text of the kit's own notices: the withhold notices, the startup refusal, the record of an unheld value,
   and the replacement the kit writes for a withheld header. The names a notice fills in at run time, such as a variable
   or field name, are not public text.
2. HTTP protocol text: the version tokens `HTTP/1.0` and `HTTP/1.1`, each reason phrase that Go's `net/http.StatusText`
   returns at the Go version in the kit's `go.mod`, each status line built from a version, a code and its reason phrase,
   and the header field names in the IANA HTTP Field Name Registry.
3. The JSON-RPC 2.0 envelope: the member names `jsonrpc`, `id`, `method`, `params`, `result`, `error`, `code`, `message`
   and `data`, the member `"jsonrpc":"2.0"`, and the reserved error codes with the messages the JSON-RPC 2.0
   specification gives them.
4. The server-sent events field names `event`, `data`, `id` and `retry`.
5. The JSON literals `true`, `false` and `null`.
6. Text the operating system or the Go runtime adds on its own, which is exactly this: the variable name `SYSTEMROOT`,
   which Go's `os/exec` adds to a child process environment on Windows. Its value is machine-specific and is not public
   text.

## 2. What counts as a leak

An emitted unit leaks when, after the decoding in section 3, it contains any of these:

- **(a) A held credential, whole.** A value of 8 or more characters counts anywhere. A shorter value counts as a whole
  token: each side is the start or end of the unit or a character that is not a letter or digit, or it is the complete
  value of a field, header, argument or line.
- **(b) A contiguous piece of a held credential 8 or more characters long,** anywhere.
- **(c) An outbound request** to an origin outside a credential's allowed origins, over plain http, or carried across
  a redirect to a different origin, that contains that credential whole or a piece of it, by the tests of (a) and (b)
  after the decoding in section 3.

An outbound request is judged by (c) alone. A credential sent over https to one of its own allowed origins, and not
carried across a redirect to a different origin, is the program working, not a leak. Every other unit is judged by (a) and (b).

An origin is the scheme, host and port, with the scheme's default port when none is written. The host is compared after
lowercasing and IDNA conversion. An allowed origin written as `https://*.example.com` matches any subdomain of
`example.com` on port 443, and not `example.com` itself.

A character is a Unicode code point, or a byte for a value that is not valid UTF-8.

**Public text.** Public text is a fixed, published table, so a value that collides with it could be guessed from the
table alone, and holding it would withhold the protocol text that ordinary units carry. It is used only for these
collision rules and exempts nothing at any sink, outbound requests included:

- A value collides with public text when the matcher, applying section 2(a) with everything section 3 requires (every
  decoder and recursion, joined values and Numbers spellings), finds the value whole in one public-text entry taken
  alone as the raw bytes of a unit, so the entry's start and end are unit boundaries. A collision check that the budget
  stops counts as a collision.
- A startup credential that collides with public text: the kit refuses to start. The refusal names the source (the
  environment variable, config field or flag, or stdin), never the value.
- A value acquired while running is not held when the value itself, as written, occurs in one public-text entry: byte
  for byte, as a substring when the value has 8 or more characters, and as a whole token of the entry by the boundary
  rule of (a) when it is shorter. The kit records each such value, naming its source (such as the header, response field
  or argument), never the value. A value acquired while running that collides only through another spelling or reading
  (a Numbers spelling, a normalized, folded or decoded view) stays held, and so does a value that is already held.
- The refusal and the record are output like any other. When naming the source would match a held credential, the kit
  writes the same fixed text without the name, and when that text would still match, section 4 drops it. A refusal
  stops the program either way.
- No piece is exempt. A held value that only partly overlaps public text or protocol framing (an 8-character piece
  inside a status line, a header name or a notice) is a piece under (b), so the unit that carries it is withheld. That
  is the safe direction, and a withhold notice that would itself match is dropped under section 4. A piece shared with
  another configured value, a URL, an identifier or a test input counts too.

An emitted unit is one of:

- a write to stdout or stderr, checked with state kept across writes (section 3, Streams);
- one MCP message: a result, an error or a protocol frame, together with the HTTP status line, headers and trailers
  that carry it;
- one file write, checked as the whole record; a file written as a stream of writes is checked like stdout;
- one SQLite statement: its text and its bound values;
- one outbound request: URL, headers and body;
- the argv and environment of one child process, as the child receives them.

## 3. Decoders

This list is complete. An encoding, format, charset or spelling that this section does not name is out of scope: a unit
that carries a held credential only through one is not a leak, and a review may raise it as P2 at most. A review
finding about decoding must name the clause of this section that the matcher breaks. The matcher may read more than
this section names; a match there withholds the unit, but the absence of an unnamed reading is never a finding.

**Views.** A view is the raw bytes of a unit, or what one decoder below produces from a view. Every view, whatever its
length or content (text, binary, mixed, one character), is compared under section 2 and read by every decoder
(Recursion). A candidate that a decoder's rule does not accept gives no view.

**Candidates.** Each decoder below is of one of three kinds, and its kind fixes where its candidates start and end:

- **Run decoders** (base64, base32, hex, base85, character-code lists): a candidate is every contiguous byte span of
  the view, from every start position to every end position, that the decoder its bullet below names or defines
  accepts whole. Bytes that decoder accepts and ignores are inside the span: carriage returns and newlines for Go's
  `encoding/base64` and `encoding/base32`, and space and control characters for Go's `encoding/ascii85`. No separate
  test decides which characters a span may contain. Each candidate's output is a view of its own, so its start and end
  are view boundaries for 2(a).
- **Substitution decoders** (percent-encoding, quoted-printable, character references, string escapes, MIME encoded
  words, ROT13, normalization, case folding): the candidate is the whole view. The output is the view with every
  sequence the rule recognizes replaced by what it decodes to, and every other byte kept as written.
- **Stream decoders** (charsets and compressed streams): a candidate starts at every byte offset of the view and runs to
  its end; the output is what the bullet below defines.

The decoders:

- **Base64**, standard and URL-safe alphabets, padded or not: a span is accepted when Go's `encoding/base64` decodes it
  (`StdEncoding` and `URLEncoding` for a padded span, their `Raw` forms for an unpadded one, leftover bits ignored).
- **Base32**, standard and hex alphabets, any case, padded or not: a span is accepted when Go's `encoding/base32`
  (`StdEncoding`, `HexEncoding` and their unpadded forms) decodes it after upper-casing.
- **Hex**: hex digit pairs in any case. Each pair may carry a `0x`, `0X` or `\x` prefix, and any run of separator
  characters (space, comma, colon, hyphen) may stand between two pairs; a span is accepted when it starts and ends on
  a whole pair.
- **Percent-encoding**: `%` and two hex digits in any case. A repeated encoding is read by recursion.
- **Quoted-printable** (RFC 2045), soft line breaks included.
- **HTML and XML character references**: named, decimal and hex, as Go's `html.UnescapeString` reads them at the Go
  version in the kit's `go.mod`.
- **String escapes**: a backslash followed by one of: one of the characters `a b e f n r t v \ ' " ? /`; `x` and two hex
  digits; `u` and four hex digits, with a UTF-16 surrogate pair of two `\u` escapes read as one character; `U` and eight
  hex digits; or one to three octal digits with a value up to octal 377, read greedily as C reads them. This covers the
  JSON, Go, TOML and C escapes.
- **Base85**: Ascii85 as Go's `encoding/ascii85` decodes it (the `z` shorthand included, `<~` `~>` delimiters
  optional), Z85 (ZeroMQ RFC 32), and RFC 1924.
- **ROT13** of ASCII letters.
- **Unicode normalization and case folding**: NFKC as `golang.org/x/text/unicode/norm` implements it, and full case
  folding as `golang.org/x/text/cases` implements it, at the `golang.org/x/text` version named below. Both apply to the
  held value and to each view before comparing, and the normalized and folded views are views too.
- **Charsets**: every encoding in the `All` lists of the `golang.org/x/text/encoding` packages `charmap`, `japanese`,
  `korean`, `simplifiedchinese`, `traditionalchinese`, `unicode` and `unicode/utf32`, at the `golang.org/x/text`
  version pinned in the kit's `go.mod` (`tools/maintainer/credkit/go.mod`). These include UTF-16 and UTF-32 in both
  byte orders, with or without a byte-order mark. A charset reading is exactly what that encoding's decoder outputs over
  a view from one starting byte offset to the end. If the decoder stops with an error, its output before the error is
  the reading. A byte sequence that the decoder does not produce a rune for (it outputs nothing, or its replacement
  character U+FFFD) is not a reading of any other character.
- **MIME encoded words** (RFC 2047, B and Q encodings, adjacent words joined): decoded in the charset the word names, as
  the `htmlindex` or `ianaindex` package of the same `golang.org/x/text` version resolves the name, and as raw bytes.
- **Compressed streams**: gzip, zlib and raw deflate, tried at every byte offset of every view. Every nonempty output is
  a view: a complete stream's output, the bytes a truncated or broken stream yields before its error, and the bytes
  after a stream's end.
- **Character-code lists**: two or more tokens separated by runs of commas and spaces, optionally inside one pair of
  square brackets, read as Unicode code points twice: once as decimal, where a token is a run of ASCII digits, and once
  as hex, where a token is a run of ASCII hex digits with an optional `0x` or `0X` prefix. A token that is not of that
  form, or whose value is not a Unicode scalar value (above hex 10FFFF, or a surrogate), ends the run. No other number
  spelling is read here: no sign, decimal point, exponent or non-ASCII digit.

Also:

- **Joined values.** For each JSON document, the matcher also reads one more view: the document's string values joined
  in document order, as the parser decodes them, with member names and values of other types left out. A JSON document
  is a view, or one line of a view (JSON lines), that a conforming JSON parser (RFC 8259) reads as exactly one value;
  braces, brackets and quotes inside a string are not structure. A document written across several writes of one
  stream is one document (Streams). No other structured format is joined: TOML, YAML, CSV, form bodies and every other
  format are out of scope. No cap on the size of a document, the number of lines or the number of values applies other
  than the budget.
- **Numbers.** A held credential made only of digits also matches these spellings of the same integer: the digits with
  leading zeros added, or removed where the credential has them; a leading plus sign; the digits split into groups of
  any sizes with one separator between two groups, each separator a comma, full stop, underscore, apostrophe or space,
  mixed or not; and scientific notation (a mantissa with at most one decimal point, `e` or `E`, and an exponent with or
  without a sign) whose value is exactly that integer. The plus sign, leading zeros and groups combine. A spelling is
  judged by its own length under section 2: one shorter than 8 characters counts only as a whole token, and a separator
  between two digits does not end a token. An 8-digit piece of such a credential also matches when its digits are
  grouped this way.
- **Streams.** Stdout, stderr and a file written as a stream of writes are checked write by write, each write with the
  context of the stream that the kit still holds, so a credential split across writes is found at the first write that
  completes it whole or completes an 8-character piece of it. A JSON document still open at the end of a write is joined
  over the string values written so far, an unfinished value included. Bytes an earlier write released are not
  recalled: before the write that completes a credential or a piece, fewer than 8 of its characters in a row have
  reached the sink, the residue section 6, item 6 already tracks.
- **Recursion.** Every decoder above, normalization, case folding and charsets included, reads every view, its own
  output and joined values included, so decodings chain in any order. The raw unit is depth 0, and each decoding adds
  one level. Chains continue to the kit's depth limit; a view the limit stops spends the budget.
- **Budget.** The kit sets a depth limit and a total decoded-bytes limit per unit. The budget is an invariant: every view
  this section defines is either checked in full or the unit is withheld exactly as for a match (section 4). A view is
  checked in full when it has been compared under section 2 and read by every decoder, and each view that produces is
  checked in full in turn. Anything that ends a view before that spends the budget: the depth limit, the decoded-bytes
  limit, any other limit, cap or length gate, a decoder or parser failure that a conforming decoder for that rule
  would not have (it would have produced more output from the same input), stream context the kit no longer holds
  while a document or a split value may still continue, and any performance shortcut. An error where the input itself
  stops being valid for the rule is part of the rule, not a stop: the output before it is the view, checking that view
  in full completes the path, and an error before any output gives no view. The one exception is an exact shortcut:
  one that skips only work that cannot change the verdict for any held set and any unit, such as not reading again a
  view identical byte for byte to one already read for this unit. One input that a shortcut misses proves it is not
  exact. A candidate a decoder's rule rejects gives no view, so rejecting
  it is not a stop. No view may end clean unchecked. The leak test list counts an exhausted unit as a failure, and it
  counts as exhausted a unit its own capture cannot read in full (protocol framing or a body it does not parse).
- **Budget quality target.** This is a quality target, not a leak rule: ordinary output, the connector's normal results
  and messages, should not exhaust the budget. An exhaustion on ordinary output is a P2 finding, fixed in the kit without
  loosening the invariant above.

## 4. What happens on a match

- The whole unit is withheld. It is replaced by a fixed notice that names the sink and says nothing about the value.
  If the notice itself would match a held credential, the unit is dropped with no replacement.
- A unit whose budget is spent (section 3) is withheld exactly the same way.
- No byte of a unit reaches its sink before the whole unit has been checked. A timer, a full buffer or a stalled reader
  never releases part of a unit.
- Partial redaction is not used.
- If the output layer cannot be installed, the program exits before writing anything.

## 5. Sinks covered

- stdout and stderr, at the file-descriptor level, from the first statement of `main`;
- MCP results, errors, protocol frames and logs;
- every file the program writes: journal, feedback, export, teach logs, profiles, config writes and caches;
- SQLite;
- outbound HTTP;
- child process argv and environment.

## 6. Advisory cases (tracked, never blocking)

A review may raise these as P2, never as P1:

1. The MCP client's own JSON-RPC request id echoed back to it.
2. Scientific-notation spellings of a number, unless the held credential is made only of digits.
3. Numeric vendor fields the connector does not declare as credentials.
4. Values the ledger declares identifiers, with a reason.
5. A credential split across two separate runs of the program.
6. Pieces shorter than 8 characters, other than the whole-token rule in 2(a).
7. Ordinary output that exhausts the budget (section 3, budget quality target).

## 7. Per-connector declarations

Each connector records these in its hand-fix ledger:

- its credential config fields, secret environment variables, credential headers and stored credential records;
- each identifier exempted from holding, with its reason;
- each credential's allowed origins;
- the vendor response fields that carry credentials.

Declarations in Go source are read from the files the connector's build compiles, under the build tags and flags that
build uses. A declaration the reader cannot interpret is a failure, never an empty declaration.

## 8. One implementation

The kit's matcher is the only implementation of sections 2 and 3. The leak test list does not carry its own copy: it
calls the kit's matcher through a small command (the held set and a unit in, one verdict out: clean, match or
exhausted) and judges every case by that verdict. A matcher fix therefore lands once and reaches both. A difference
between what the kit withholds and what the list reports is a defect in the command or the list, never a second reading
of this contract. How the list chooses, names and baselines its cases, and the integrity of its own code, are outside
this contract.
