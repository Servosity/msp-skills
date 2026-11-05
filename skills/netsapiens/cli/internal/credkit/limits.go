// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Every limit on a check, in one table (contract 3, Budget). A path that
// reaches a budget limit spends the budget: the unit is withheld exactly as
// for a match. TestKit3LimitsWithhold feeds each its limit plus one and
// expects the unit withheld, and TestKit3StopsGoThroughLimits fails on a
// length compared with a constant anywhere in the matcher's files unless the
// constant is a limit here or the comparison is marked an exact shortcut.
//
// A reading bound is not a budget limit: it is a rule, the same for every
// unit, for which decoders read some views. Without one the closure of views
// has no end on ordinary output, so every unit would spend the budget
// (.review/kit3-prep.md has the proof and the measurements).

// Budget limits. Variables, so a test can lower them.
var (
	maxDepth       = 32       // decodings deep
	budgetPerByte  = 256      // decoded bytes per unit byte,
	budgetBase     = 24 << 20 // plus this (first adopters: ordinary connection names spent up to 17 MiB),
	deflPerByte    = 64       // broken or retried fixed-Huffman output per unit byte,
	deflBase       = 32 << 20 // plus this (the same units drew up to 28 MiB, one in 6,000 far more),
	deflMax        = 48 << 20 // at most this
	maxUnit        = 64 << 20 // one unit
	maxEncodedBody = 64 << 20 // a response body read whole to hold its declared fields
	maxDeclared    = 64 << 20 // a request body the kit reads to hold the values in it
	streamTailMin  = 1 << 10  // a stream's context: at least this,
	streamTailPer  = 16       // or this many times the longest held value,
	streamTailMax  = 16 << 10 // at most this
)

// maxRawNest is how many headerless raw deflate streams deep a view can
// be: as deep as maxDepth, since each stream adds a level. A constant: it
// sizes the closure's buckets.
const maxRawNest = 32

// limitNames is the table: TestKit3LimitsWithhold has a case for each.
var limitNames = []string{
	"maxDepth", "budget", "deflBudget", "maxUnit", "maxRawNest",
	"maxEncodedBody", "maxDeclared", "streamTail",
}

// Reading bounds. A byte view is read by every decoder except ROT13 (see
// rot13TextOnly). A text view is read by the text decoders (base-N, hex,
// percent, quoted-printable, character references, string escapes, MIME
// words, character-code lists, NFKC, ROT13, joins), not by the byte
// decoders that read binary (the compressed streams, UTF-16 and UTF-32,
// the charset readings). Ten bounds, each named by its constant below.
const (
	// tinyView: a decoding shorter than this many bytes is matched, after
	// NFKC and case folding as every view is, and not decoded further.
	// Base-N at every alignment of every run of printable bytes in decoded
	// binary yields many times its input in decodings of one to three
	// bytes, at every level.
	tinyView = 4
)

// rawSuffix: the bytes after a headerless raw deflate stream's end are
// read where they are, in the view that holds the stream (by every decoder,
// at every offset and every span), not as a view of their own. Ordinary
// text holds a complete fixed-Huffman stream at thousands of offsets, and
// each suffix is nearly the whole view. After a gzip or zlib stream they
// are a view of their own.
const rawSuffix = true

// brokenText: the bytes a broken headerless raw deflate stream yields are a
// final view: matched, read as a text view and by the byte decoders that
// find their input by a header or a pattern (gzip, zlib, UTF-16, UTF-32),
// and what those decode is matched and not decoded further. Raw deflate at
// every offset and the charset readings do not read it. A 150 KB JSON
// document yields 800 KB of them from its own offsets; most are
// back-references repeating a few bytes, and decoders read a repeat as a
// repeat one character shorter, one level at a time. Read by every decoder
// (measured), the same document exhausts the budget.
const brokenText = true

// brokenWindow and brokenSuffix (first adopters, 2026-11-03) are an exact
// shortcut, not a reading bound. Fixed-Huffman symbols resynchronize, so the
// attempts at many offsets of a stretch of binary all end in the same bytes:
// one unit's 11 MB of final views from 20 KB was 100 KB repeated. Of a broken
// stream's output that ends in a stretch another output added to the same view
// also ends in, only the first brokenWindow bytes of the stretch are added:
// nearly every decoder of a final view reads a window of at most that many
// bytes, and each window past them is in the other output. The readings that
// are not of such a window keep their input whole: a compressed stream that
// starts before the cut and a JSON document open across it read the whole
// output (decode.go acrossCut), and a MIME encoded word, a numeric character
// reference or a window stretched by filler leave the output uncut
// (deflateprobe.go cutAllowed).
// The cut's end only adds a place a short value can end, so it can only
// withhold more.
const (
	brokenWindow = 512
	brokenSuffix = true
)

// textViews: a charset reading, the ROT13 view, and the NFKC view of
// either, are text views (decode.go textOut). A charset reading of a charset reading is a new view at every
// level for ordinary text (EBCDIC reads ASCII letters as Latin-1
// characters, whose UTF-8 it reads as more), and the ROT13 view of ordinary
// text is a full copy of it.
const textViews = true

// joinFinal: a JSON document's joined string values are a final view, read
// as brokenText says. Joined, the values of an ordinary
// document run together at every field boundary (a date into a word into
// an address), so the join is new runs from end to end; decoded in turn at
// every level, it alone costs more than the rest of the check.
const joinFinal = true

// foldView: the case-folded view is compared (every view is compared case
// folded, so this is the comparison of the view it folds) and not decoded
// further. The decoders that read case (base64, base85, the stream
// decoders) read the view as written.
const foldView = true

// rot13TextOnly (owner amendment, 2026-10-30): ROT13 reads a text view
// only, never a byte view or a final view. decText applies it to a whole
// text part (decText's whole), the shipped "ROT13 of whole text only"
// measured in kit3rv; applied to every view it ran a 92-byte JSON unit to
// closure depth 33 and exhausted the budget on ordinary output. ROT13 of
// a byte view is out of scope: a leak only through it is not a leak.
const rot13TextOnly = true

// rot13WholeText (owner order, 2026-11-02): ROT13 reads a text view the
// kit reads whole (decode.go, decText's whole). The runs cut from a
// charset reading or from a decoder's text output are compared and read by
// the other text decoders, not rotated. Rotating them too (built in kit8rv)
// ran the 7 KB environment and the 150 KB JSON with a non-ASCII credential
// to the depth limit even at 16 times the budget, and doubled the JSON
// check (196 to 418 ms). A credential reachable only through ROT13 of such
// a run is out of scope.
const rot13WholeText = true

// ascii85Span (owner amendment, 2026-10-30): an Ascii85 span may hold tab,
// carriage return and line feed but no space and no other control byte (a
// span ends at one), though Go's encoding/ascii85 ignores space and every
// control byte. The three in alphabets[3].ignore (decode.go) are the span.
// A span across a space turns every line of prose into one run and a span
// across other control bytes runs the 150 KB JSON check out of deflate
// budget (both measured in kit3rv). A leak only through a wider span is
// out of scope.
const ascii85Span = true

// runDecoderSpans (owner amendment, 2026-10-31): a run decoder (base64,
// base32, hex, base85, character-code lists) decodes further, at every
// alignment, the maximal run it accepts. A shorter span inside that run is
// read as the stretch of the run's output it decodes to (compared there,
// with a short value counted anywhere: decode.go runOut), not decoded again
// as a view of its own, so a later decoder that reads greedily (a string
// escape's octal digits, a character reference's name, a code point) sees
// the run's own continuation, not every possible end. Every (start, end)
// span decoded as a view is quadratic in the run and multiplies at every
// level: built in kit4rv, it ran the 150 KB JSON check out of budget. A
// credential reachable only through an end this leaves out is out of scope.
const runDecoderSpans = true
