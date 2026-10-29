---
layout: default
title: "Credential leak contract - what every connector must never print, store or send | MSP Skills"
description: "The fixed rules every MSP Skills connector and its review are judged against: which values count as held credentials, what counts as a leak (a whole value at any length, pieces of 8 or more characters, credentials sent off their declared hosts), the encodings checked, and the cases that are tracked but never block."
permalink: /credential-leak-contract/
---

# Credential leak contract

Owner-ruled 2026-10-29. This is the fixed target for every credential check and every review of a connector under
`skills/*/cli` and of the shared credential kit (`tools/maintainer/credkit`). A review judges credential findings against
this document. A finding outside it is P2 at most, unless it shows a held credential leaving through a path this document
covers. Changing a rule here needs the owner; a review cannot.

## 1. Held credentials

A held credential is every value the program uses or receives as a secret:

- each environment variable the connector declares secret (from the connector's declarations, never guessed from names);
- each credential field of the config file, including a file that does not parse (read token by token), legacy config
  paths, `[headers]` values, `auth_header`, and a password or credential query value inside `base_url`;
- credential flags and credentials read from stdin;
- credentials in outgoing requests: auth headers, cookie values, the user and password parts of Basic auth, and form or
  JSON credential fields;
- tokens minted at run time, `Set-Cookie` values, and the vendor response fields the connector declares as credentials;
- MCP tool arguments that carry a credential field.

Rules for the held set:

- A value known at startup (environment, config file, flags, stdin) is held before the first byte of any output,
  including flag-parse errors and startup warnings.
- A value acquired while running (a minted token, a `Set-Cookie` value, a declared response field, an MCP argument, a
  credential placed in an outgoing request) is held before the response, call or request that carries it reaches any
  sink.
- No exemption for length, strength or type. Numbers count.
- A value is exempt only when the connector's ledger declares it an identifier, with a reason.
- Every member of a duplicate key is held.
- Values stay held until the process exits.
- A child process gets the held set or is not started.

## 2. What counts as a leak

An emitted unit leaks when, after the decoding in section 3, it contains any of these:

- **(a) A held credential, whole.** A value of 8 or more characters counts anywhere. A shorter value counts as a whole
  token: each side is the start or end of the unit or a character that is not a letter or digit, or it is the complete
  value of a field, header, argument or line.
- **(b) A contiguous piece of a held credential 8 or more characters long,** anywhere.
- **(c) A held credential in any form in an outbound request** to an origin outside that credential's allowed origins,
  over plain http, or carried across a redirect to a different origin.

An outbound request is judged by (c) alone. A credential sent over https to one of its own allowed origins, and not
carried across a redirect to a different origin, is the program working, not a leak. Every other unit is judged by (a) and (b).

An origin is the scheme, host and port, with the scheme's default port when none is written. The host is compared after
lowercasing and IDNA conversion. An allowed origin written as `https://*.example.com` matches any subdomain of
`example.com` on port 443, and not `example.com` itself.

A character is a Unicode code point, or a byte for a value that is not valid UTF-8.

An emitted unit is one of:

- a write to stdout or stderr, checked with state kept across writes;
- one MCP message: a result, an error or a protocol frame;
- one file write, checked as the whole record;
- one SQLite statement: its text and its bound values;
- one outbound request: URL, headers and body;
- the argv and environment of one child process.

## 3. Decoders

The matcher checks the raw bytes and every decoding below, recursively, at every offset:

- base64, standard and URL-safe, padded or not, at every alignment and inside longer runs;
- base32, standard and hex alphabets, any case, padded or not, at every alignment;
- hex in any case, with or without separators;
- percent-encoding, repeated;
- quoted-printable;
- HTML and XML entities: named, decimal and hex;
- string escapes: JSON `\uXXXX` including surrogate pairs, `\xXX`, Go, TOML and C escapes, and octal;
- Ascii85, Z85 and RFC 1924 base85;
- ROT13;
- Unicode NFKC (fullwidth forms) and case folding, applied to both the held value and the unit before comparing;
- UTF-16 LE and BE, with or without a byte-order mark;
- MIME encoded words, and any charset in `golang.org/x/text/encoding`;
- gzip, zlib and raw deflate, including trailing bytes after the stream;
- character-code lists: decimal or hex code points separated by commas or spaces.

Also:

- **Joined values.** The matcher checks the concatenation of adjacent string values in one structured document, in order,
  so a credential split across fields of one document is found.
- **Numbers.** A credential made only of digits also matches its other integer spellings: leading zeros, digit grouping,
  and scientific notation.
- **Budget.** The kit sets a nesting-depth limit and a total decoded-bytes limit per unit. When a limit runs out before
  every path has been checked, the unit is withheld exactly as for a match. A decoder error on one candidate never
  makes the unit clean.

## 4. What happens on a match

- The whole unit is withheld. It is replaced by a fixed notice that names the sink and says nothing about the value.
  If the notice itself would match a held credential, the unit is dropped with no replacement.
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

## 7. Per-connector declarations

Each connector records these in its hand-fix ledger:

- its credential config fields, secret environment variables, credential headers and stored credential records;
- each identifier exempted from holding, with its reason;
- each credential's allowed origins;
- the vendor response fields that carry credentials.
