// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), sentinelone-only: the SUCCESS-path companion of
// the shared secret-echo funnel. The fleet-canonical funnel files cover error
// text; this file covers a 2xx answer, which reaches the CLI's stdout, an MCP
// tool result and the local store. Called from three anchored lines in
// client.go (the two success returns of doInternal and the cache read).
//
// The check fails closed for the whole class "an answer repeats a held
// credential in a form the check does not see":
//   - every value is checked as received AND normalized (whitespace, control,
//     format and NUL characters and invalid UTF-8 dropped), so an echo split by
//     a CR/LF, a tab, a zero-width space or UTF-16 NULs is seen joined;
//   - a JSON answer is walked token by token (json.Decoder.Token), so every
//     key and every string or number is checked, duplicates included; an
//     answer that is not valid JSON is checked whole, as text;
//   - a binary answer is checked whole (every valid-UTF-8 stretch of it, see
//     successCheck.text), plus each of its printable runs;
//   - each value goes through the shared funnel (cliutil.FunnelHeld), then is
//     decoded again (Base32 in both alphabets, either case, padded or not, at
//     every alignment, plus every family the funnel decodes and the client.go
//     decoders it lacks) and every decoding goes back through the same check,
//     to a bounded depth, so any composition of them is reached;
//   - every view is matched, in any case, against every encoded form client.go
//     computes for a credential a client handed this path;
//   - a value over the shared funnel's size cap, or a check that runs out of
//     its work budget, withholds the whole answer. There is no weaker path.

package client

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"sentinelone-pp-cli/internal/cliutil"
)

const (
	// successFunnelMinLen: no held value shorter than this is looked for (the
	// canonical funnel's own floor, and client.go's minEncodedSecretLen), and
	// every form this check normalizes or decodes is at least as long as the
	// text it carries (UTF-16 aside, which needs a NUL per ASCII character), so
	// a shorter value cannot carry one.
	successFunnelMinLen = 8
	// successFunnelMaxValue is the canonical funnel's input cap (funnelMaxInput
	// in internal/cliutil/secret_funnel.go): it withholds longer text without
	// decoding it, so a value over this cap withholds the whole answer.
	successFunnelMaxValue = 1 << 20
	// successFunnelMaxDepth bounds how many decodings deep a value is followed
	// (each cliutil.FunnelHeld call runs its own decoding search on top).
	successFunnelMaxDepth = 4
	// The work budget of one answer, in bytes of views checked: past it the
	// answer is withheld (fail closed), as the canonical funnel does.
	successFunnelWorkPerByte = 64
	successFunnelWorkBase    = 1 << 20
)

// funnelSuccess returns body unchanged, or fails the answer closed when it
// repeats a credential this process holds in any form successEchoesHeld
// normalizes or decodes. raw is the answer as received (for a binary answer,
// body is a base64 envelope that would hide the bytes from a text check).
func (c *Client) funnelSuccess(method, path, authHeader string, body json.RawMessage, raw []byte, binary bool, status int) (json.RawMessage, int, error) {
	c.successHoldCredentials(authHeader)
	check := []byte(body)
	if binary {
		check = raw
	}
	withhold := successEchoesHeld(check, binary)
	if !withhold && binary {
		// The envelope also carries the server's Content-Type header as text.
		var meta struct {
			ContentType string `json:"content_type"`
		}
		if err := json.Unmarshal(body, &meta); err != nil {
			withhold = true
		} else {
			withhold = successEchoesHeld([]byte(meta.ContentType), false)
		}
	}
	if !withhold {
		return body, status, nil
	}
	return nil, status, fmt.Errorf("%s %s: %s", method, c.displayURL(path, authHeader), cliutil.FunnelWithheldText)
}

// successEchoesHeld reports whether an answer repeats a held credential (see
// the file comment for what is checked). It is true for any answer holding a
// single value too large to decode; a large answer made of small values is
// checked value by value and is never refused for its size alone.
func successEchoesHeld(b []byte, binary bool) bool {
	if len(b) < successFunnelMinLen {
		return false
	}
	k := newSuccessCheck(len(b))
	if binary {
		return k.binary(b)
	}
	if hit, valid := k.json(b); hit || valid {
		return hit
	}
	return k.value(string(b))
}

// successCheck is the check of one answer: the views it has explored (shared
// by every value of the answer, so a repeated value is decoded once), the
// needles of the held client credentials, and the remaining work budget.
type successCheck struct {
	needles []successNeedle
	seen    map[string]successSeen
	budget  int
}

type successSeen struct {
	depth int
	hit   bool
}

func newSuccessCheck(size int) *successCheck {
	return &successCheck{
		needles: successHeldNeedles(),
		seen:    map[string]successSeen{},
		budget:  successFunnelWorkPerByte*size + successFunnelWorkBase,
	}
}

// value checks one value of an answer. A value over the canonical funnel's cap
// is never decoded, so it reports true (withhold).
func (k *successCheck) value(s string) bool {
	if len(s) > successFunnelMaxValue {
		return true
	}
	return k.view(s, 0)
}

// view checks one view (a value, its normalized form, or a decoding of
// either) once per depth: a view seen before at the same or a shallower depth
// has its answer already.
func (k *successCheck) view(s string, depth int) bool {
	if len(s) < successFunnelMinLen {
		return false
	}
	if prior, ok := k.seen[s]; ok && (prior.hit || prior.depth <= depth) {
		return prior.hit
	}
	k.budget -= len(s) + 1
	if k.budget < 0 {
		return true
	}
	k.seen[s] = successSeen{depth: depth}
	hit := k.explore(s, depth)
	k.seen[s] = successSeen{depth: depth, hit: hit}
	return hit
}

func (k *successCheck) explore(s string, depth int) bool {
	if cliutil.FunnelWithheld(cliutil.FunnelHeld(s)) || k.matchesNeedle(s) {
		return true
	}
	if n := successNormalize(s); n != s && k.view(n, depth) {
		return true
	}
	if depth >= successFunnelMaxDepth {
		return false
	}
	for _, d := range successDecodings(s) {
		if k.text(d, depth+1) {
			return true
		}
	}
	return false
}

// text checks s whole when it is text: valid UTF-8, or text with stray invalid
// bytes (at most one byte in four), which the whole check sees joined, as a
// display shows it. Otherwise s is binary, and each of its maximal valid-UTF-8
// stretches is checked on its own: a held value is text, and so is every
// encoding of one this check or the canonical funnel decodes, so each
// occurrence lies inside one stretch. Binary is never handed over whole: a
// large run of it exhausts the canonical funnel's work budget (measured: 64
// KiB of random bytes is withheld), which would withhold every download.
func (k *successCheck) text(s string, depth int) bool {
	if utf8.ValidString(s) {
		return k.view(s, depth)
	}
	if successInvalidBytes(s)*4 <= len(s) && k.view(s, depth) {
		return true
	}
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r != utf8.RuneError || size > 1 {
			i += size
			continue
		}
		if i-start >= successFunnelMinLen && k.view(s[start:i], depth) {
			return true
		}
		i++
		start = i
	}
	return len(s)-start >= successFunnelMinLen && k.view(s[start:], depth)
}

// successInvalidBytes counts the bytes of s that are not part of valid UTF-8.
func successInvalidBytes(s string) int {
	n := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			n++
		}
		i += size
	}
	return n
}

func (k *successCheck) matchesNeedle(s string) bool {
	if len(k.needles) == 0 {
		return false
	}
	folded := strings.ToLower(s)
	for _, n := range k.needles {
		if strings.Contains(s, n.exact) || strings.Contains(folded, n.folded) {
			return true
		}
	}
	return false
}

// json walks b as a JSON token stream, checking every key and every string or
// number token (decoded, and as written when that differs), so a duplicate key
// cannot hide its first value. valid is false when b is not one or more
// complete JSON values; the caller then checks b whole, as text.
func (k *successCheck) json(b []byte) (hit, valid bool) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	depth := 0
	for {
		start := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return false, depth == 0
		}
		if err != nil {
			return false, false
		}
		switch v := tok.(type) {
		case json.Delim:
			if v == '{' || v == '[' {
				depth++
			} else {
				depth--
			}
		case json.Number:
			if k.value(string(v)) {
				return true, true
			}
		case string:
			if k.value(v) {
				return true, true
			}
			// As written: the decoder turns invalid UTF-8 into U+FFFD, which
			// would keep the halves of a value split by an invalid byte apart.
			raw := bytes.TrimLeft(b[start:dec.InputOffset()], " \t\r\n,:")
			if (bytes.IndexByte(raw, '\\') >= 0 || !utf8.Valid(raw)) && k.value(string(raw)) {
				return true, true
			}
		}
	}
}

// binary checks a binary answer whole, as one value: every text stretch of it
// (see text) as stored and normalized, which drops the control, whitespace and
// format bytes that split printable runs and the NULs of UTF-16 text; and,
// as before, each printable ASCII run, as stored and with NULs dropped. An
// answer over the canonical funnel's cap is withheld whole.
func (k *successCheck) binary(b []byte) bool {
	if len(b) > successFunnelMaxValue || k.text(string(b), 0) {
		return true
	}
	return k.printableRuns(b) || k.printableRuns(bytes.ReplaceAll(b, []byte{0}, nil))
}

func (k *successCheck) printableRuns(b []byte) bool {
	start := -1
	for i := 0; i <= len(b); i++ {
		if i < len(b) && b[i] >= 0x20 && b[i] < 0x7f {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= successFunnelMinLen && k.value(string(b[start:i])) {
			return true
		}
		start = -1
	}
	return false
}

// successNormalize drops whitespace, control (NUL, CR, LF included) and format
// characters and invalid UTF-8, so a value split by any of them is seen whole.
func successNormalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// successDecodings is every decoding of s this check follows. The canonical
// funnel decodes most of these forms itself, but its intermediate views never
// come back out of it, so a form it cannot decode (Base32, ROT13) wrapped in
// one it can (base64, percent, hex, HTML, markup) would stay hidden: each
// family is applied here too, so every composition is reached, to the depth
// bound. The run decoders (Base32 in both alphabets and either case, base64 in
// either alphabet, hex) each make their own views; the local escape decoders
// are applied together, settled (successSettle), as one view, since each
// leaves the rest of the text as it is and applying them one view each would
// multiply the views of an ordinary escape-heavy value (measured: a 4 KiB
// HTML table ran out of a 320x budget); ROT13 is its own view.
func successDecodings(s string) []string {
	var out []string
	add := func(d string) {
		if d != s && len(d) >= successFunnelMinLen {
			out = append(out, d)
		}
	}
	for _, a := range successAlphabets {
		for _, v := range a.views(s) {
			add(v)
		}
	}
	add(successSettle(s))
	add(credentialRot13(s))
	for _, v := range credentialUTF16Views(s) {
		add(v)
	}
	return out
}

// successLocalDecoders are the escape decoders: JSON and JavaScript string
// escapes, \xHH, %uXXXX, percent escapes, HTML entities, markup tags, the
// identity escape (any other backslash dropped), quoted-printable, fullwidth
// forms and overlong UTF-8 (the last four are forms client.go masks that the
// canonical funnel does not decode).
var successLocalDecoders = []func(string) (string, bool){
	credentialJSONUnescape,
	credentialDecodeSlashX,
	credentialDecodePercentU,
	func(v string) (string, bool) { return credentialPercentDecode(v, false) },
	func(v string) (string, bool) {
		if strings.IndexByte(v, '&') < 0 {
			return v, false
		}
		d := html.UnescapeString(v)
		return d, d != v
	},
	func(v string) (string, bool) {
		if strings.IndexByte(v, '<') < 0 {
			return v, false
		}
		d := successTagRE.ReplaceAllString(v, "")
		return d, d != v
	},
	func(v string) (string, bool) {
		if strings.IndexByte(v, '\\') < 0 {
			return v, false
		}
		var b strings.Builder
		for i := 0; i < len(v); i++ {
			if v[i] == '\\' && i+1 < len(v) {
				i++
			}
			b.WriteByte(v[i])
		}
		return b.String(), b.Len() != len(v)
	},
	credentialQPDecode,
	credentialFoldFullwidth,
	credentialDecodeOverlong,
}

// successSettle applies every escape decoder in turn until nothing changes
// (bounded), so text escaped in several layers is seen decoded.
func successSettle(s string) string {
	for round := 0; round < 8; round++ {
		before := s
		for _, decode := range successLocalDecoders {
			if d, ok := decode(s); ok {
				s = d
			}
		}
		if s == before {
			break
		}
	}
	return s
}

// successTagRE matches one markup tag (a value split by tags is seen joined).
var successTagRE = regexp.MustCompile(`<[^<>]*>`)

// successAlphabet is one run encoding: the value of each byte (-1: not in the
// alphabet), the bits each character carries, and how many characters make a
// whole number of bytes (the alignments a run is decoded at).
type successAlphabet struct {
	values [256]uint8
	bits   uint
	align  int
	minRun int
}

// successNoValue marks a byte outside an alphabet.
const successNoValue = 0xFF

func asciiLower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func asciiUpper(c byte) byte {
	if 'a' <= c && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func newSuccessAlphabet(chars string, bits uint, fold bool, extra map[byte]uint8) *successAlphabet {
	a := &successAlphabet{bits: bits}
	for c := range a.values {
		a.values[c] = successNoValue
	}
	var v uint8
	for i := 0; i < len(chars); i++ {
		a.values[chars[i]] = v
		if fold {
			a.values[asciiLower(chars[i])] = v
			a.values[asciiUpper(chars[i])] = v
		}
		v++
	}
	for c, v := range extra {
		a.values[c] = v
	}
	for a.align = 1; (a.align*int(bits))%8 != 0; a.align++ {
	}
	a.minRun = (successFunnelMinLen*8 + int(bits) - 1) / int(bits)
	return a
}

// successAlphabets: RFC 4648 Base32 (standard and extended-hex, either case),
// base64 (standard and URL-safe, so '+'/'-' are 62 and '/'/'_' are 63) and hex
// (either case). Padding is not in any alphabet, so it ends a run.
var successAlphabets = []*successAlphabet{
	newSuccessAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", 5, true, nil),
	newSuccessAlphabet("0123456789ABCDEFGHIJKLMNOPQRSTUV", 5, true, nil),
	newSuccessAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", 6, false, map[byte]uint8{'-': 62, '_': 63}),
	newSuccessAlphabet("0123456789abcdef", 4, true, nil),
}

// views decodes every run of the alphabet long enough to carry a held value,
// at each alignment (a run can start with text that is not part of the
// encoding, and a value can sit at any offset of a longer encoded blob): one
// view per alignment, the runs' decodings joined by a space.
func (a *successAlphabet) views(s string) []string {
	var runs []string
	start := -1
	for i := 0; i <= len(s); i++ {
		if i < len(s) && a.values[s[i]] != successNoValue {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= a.minRun {
			runs = append(runs, s[start:i])
		}
		start = -1
	}
	var out []string
	for k := 0; k < a.align && len(runs) > 0; k++ {
		var parts []string
		for _, run := range runs {
			if len(run)-k >= a.minRun {
				parts = append(parts, a.decode(run[k:]))
			}
		}
		if len(parts) > 0 {
			out = append(out, strings.Join(parts, " "))
		}
	}
	return out
}

// decode decodes a run of alphabet characters without padding, dropping a
// trailing partial byte.
func (a *successAlphabet) decode(run string) string {
	var b strings.Builder
	var acc uint32
	var bits uint
	for i := 0; i < len(run); i++ {
		acc = acc<<a.bits | uint32(a.values[run[i]])
		bits += a.bits
		if bits >= 8 {
			bits -= 8
			var out [4]byte
			binary.BigEndian.PutUint32(out[:], acc>>bits)
			b.WriteByte(out[3])
		}
		acc &= 1<<bits - 1
	}
	return b.String()
}

// successNeedle is one encoded form of a held client credential, matched as
// written or case-folded.
type successNeedle struct {
	exact, folded string
}

// successHeld is every credential value a client handed the success path, and
// its needles: the value and every encoded form client.go masks in error text
// (addEncodedCredentialMasks). The canonical funnel's own held list is not
// readable from this package, so a client registers its credentials here
// before each success check; the cache read sees those of every client that
// has already had a success answer in this process.
var successHeld = struct {
	sync.Mutex
	values  map[string]bool
	seen    map[string]bool
	needles []successNeedle
}{values: map[string]bool{}, seen: map[string]bool{}}

// successHoldCredentials registers the credentials maskCredentialText covers
// for this client, and the auth header of the request.
func (c *Client) successHoldCredentials(authHeader string) {
	values := []string{authHeader}
	if c != nil && c.Config != nil {
		c.lockTokenState()
		values = append(values, c.Config.AuthHeaderVal, c.Config.AuthHeader(), c.Config.AccessToken, c.Config.RefreshToken, c.Config.ClientSecret, c.Config.SentineloneApiToken)
		c.unlockTokenState()
	}
	successHold(values...)
}

func successHold(values ...string) {
	successHeld.Lock()
	defer successHeld.Unlock()
	needle := func(n string) {
		if len(n) < successFunnelMinLen || successHeld.seen[n] {
			return
		}
		successHeld.seen[n] = true
		successHeld.needles = append(successHeld.needles, successNeedle{exact: n, folded: strings.ToLower(n)})
	}
	for _, v := range values {
		v = strings.TrimSpace(v)
		candidates := []string{v}
		if _, rest, ok := strings.Cut(v, " "); ok {
			candidates = append(candidates, strings.TrimSpace(rest))
		}
		for _, cand := range candidates {
			if len(cand) < minEncodedSecretLen || successHeld.values[cand] {
				continue
			}
			successHeld.values[cand] = true
			needle(cand)
			addEncodedCredentialMasks(cand, needle)
		}
	}
}

// successHeldNeedles returns the needles held so far (append-only, so the
// returned slice is never changed under the caller).
func successHeldNeedles() []successNeedle {
	successHeld.Lock()
	defer successHeld.Unlock()
	return successHeld.needles[:len(successHeld.needles):len(successHeld.needles)]
}
