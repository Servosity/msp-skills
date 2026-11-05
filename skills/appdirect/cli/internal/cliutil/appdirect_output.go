// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// appdirect handfix (output-chokepoint): ONE withholding function,
// WithholdText, stands in front of every output path:
//
//   - the CLI process's stdout and stderr (InstallOutputFunnel swaps both for
//     pipes and filters every line; loggers write to the swapped stderr), so
//     command output, warnings, Cobra's error line and any direct os.Stdout or
//     os.Stderr write pass it;
//   - files the CLI writes for the operator (--deliver via WithholdBytes,
//     export -o via WithholdingFile) and the local store (store package);
//   - the MCP server's tool results and errors (its result middleware) and its
//     stderr.
//
// It withholds any value the process holds (FunnelHold: the credentials,
// registered when the configuration loads, when a client is built, when a
// token is minted and when the chokepoint is installed) in every view the shared funnel decodes (JSON
// and JavaScript escapes, percent and HTML escapes, markup, base64 and hex)
// and in the binary-to-text encodings it does not decode (the base85 family:
// Ascii85, Z85 and RFC 1924; base32 and base32hex here; uuencode, xxencode, crypt, bcrypt, Crockford,
// z-base-32 and base36/58/62 in appdirect_encodings.go), which are matched as
// encoded needles at every byte alignment, and in every per-byte numeric
// encoding (quoted-printable, \x41, 0x41, decimal, octal and binary byte
// lists, ...), decoded into views by appdirect_encodings.go. A value split across lines, table cells or adjacent JSON
// values is found in joined views of the text (whitespace removed, JSON
// string values concatenated, letters and digits only). Withholding is as
// narrow as stays safe: a JSON string literal, else a whitespace token, else
// the run of neighbouring lines, else the whole text.

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// funnelWindow bounds one decoded search; longer texts are searched in
// overlapping windows so a large output is not withheld for its size alone.
// The funnel's work cap fails closed per call, and whitespace-free JSON (an
// --agent list) needs more work per byte than spaced text: measured on a
// 108 KB compact document, 64 KB windows tripped the cap and 32 KB did not,
// so windows are 16 KB. The overlap is longer than any encoded form of a
// credential (a 45-character key \u-escaped is 270 bytes).
const (
	funnelWindow  = 16 << 10
	funnelOverlap = 4 << 10
)

var expandedHeld = struct {
	sync.Mutex
	seen    map[string]bool
	encoded map[string]bool // the encodedCores forms (appdirect_scattered.go spells only the others)
}{seen: map[string]bool{}, encoded: map[string]bool{}}

// isEncodedForm reports a held value that holdEncodedCores added as an
// encoding of another held value (not a credential or a decoded part).
func isEncodedForm(v string) bool {
	expandedHeld.Lock()
	defer expandedHeld.Unlock()
	return expandedHeld.encoded[v]
}

// holdEncodedCores adds, for every held value, its base85 (Ascii85, Z85,
// RFC 1924) and base32
// encodings at each byte alignment (the encoded run of the whole groups that
// lie inside the value, wherever it starts in a longer encoded stream). The
// forms the funnel searches for (the part after a scheme word, the halves of a
// base64 id:secret pair, decoded text) are held and encoded too: a value held
// as "Bearer x" is matched by x alone in every encoding (Codex r1 class
// secret-in-encoded-output, 4.34.2 reprint).
func holdEncodedCores() {
	heldSecrets.Lock()
	values := append([]string(nil), heldSecrets.values...)
	heldSecrets.Unlock()
	var add []string
	expandedHeld.Lock()
	for _, v := range values {
		if expandedHeld.seen[v] {
			continue
		}
		expandedHeld.seen[v] = true
		forms := append([]string{v, funnelSchemeless(v)}, funnelNeedles([]string{v})...)
		forms = append(forms, credentialParts(v)...) // derived parts at any length (Codex r1 class holder incomplete for decoded authentication parts)
		for _, form := range forms {
			if form != v && !expandedHeld.seen[form] {
				expandedHeld.seen[form] = true
				add = append(add, form)
			}
			for _, core := range encodedCores(form) {
				if !expandedHeld.seen[core] {
					expandedHeld.seen[core] = true
					expandedHeld.encoded[core] = true
					add = append(add, core)
				}
			}
		}
	}
	FunnelHold(add...) // published before the lock opens, so no check sees "expanded" ahead of the held forms (Codex r2 P2)
	expandedHeld.Unlock()
}

var (
	b32Encodings = []*base32.Encoding{
		base32.StdEncoding.WithPadding(base32.NoPadding),
		base32.HexEncoding.WithPadding(base32.NoPadding),
	}
	z85Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"
	// rfc1924Alphabet is RFC 1924's base85 (Python's base64.b85encode, git
	// binary patches, Mercurial).
	rfc1924Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"
	// base85Alphabets is the radix-85 family (4 bytes to 5 characters), by
	// alphabet rather than by name (Codex r1 appdirect class secret in encoded
	// output: RFC 1924 was missing beside Ascii85 and Z85): Ascii85 (btoa,
	// Adobe; "!" to "u"), Z85 and RFC 1924. Every held value is matched in each.
	base85Alphabets = []string{ascii85Alphabet(), z85Alphabet, rfc1924Alphabet}
)

func ascii85Alphabet() string {
	b := make([]byte, 85)
	for i := range b {
		b[i] = byte('!' + i)
	}
	return string(b)
}

// base85 encodes b in alphabet: each 4-byte group is 5 characters, and a
// final group of n bytes is padded with zeros and written as n+1 characters
// (the Ascii85 and RFC 1924 rule). No group shortcut ("z") is used.
func base85(b []byte, alphabet string) string {
	var sb strings.Builder
	for i := 0; i < len(b); i += 4 {
		var group [4]byte
		n := copy(group[:], b[i:])
		v := uint32(group[0])<<24 | uint32(group[1])<<16 | uint32(group[2])<<8 | uint32(group[3])
		var g [5]byte
		for j := 4; j >= 0; j-- {
			g[j] = alphabet[v%85]
			v /= 85
		}
		sb.Write(g[:n+1])
	}
	return sb.String()
}

func encodedCores(v string) []string {
	if len(v) < shortMinLen {
		return shortEncodings(v, false) // the long whole-value encodings of a value of any length
	}
	// A short value's whole-value encodings, then (for every value) the
	// encodings of the whole groups inside it at each alignment: a short value
	// embedded in a longer encoded stream is matched too (Codex r2 class
	// undecoded-encoding-gap, fixed after the stop rule).
	out := shortEncodings(v, false)
	// 4-byte groups: the base85 family. Each output character depends on a whole
	// group, so at each alignment the needle is the run of whole groups inside
	// the value; a value of heldMatchFloor bytes holds at least one group at
	// every alignment (Codex r1 class undecoded-encoding-gap: the floor was two
	// groups, which left 8- to 11-byte values unmatched at some alignments).
	for a := 0; a < 4; a++ {
		core := []byte(v[min(a, len(v)):])
		core = core[:len(core)-len(core)%4]
		if len(core) < 4 {
			continue
		}
		for _, alphabet := range base85Alphabets {
			out = append(out, base85(core, alphabet))
		}
	}
	for a := 0; a < 5; a++ { // 5-byte groups: base32 and base32hex
		core := []byte(v[min(a, len(v)):])
		core = core[:len(core)-len(core)%5]
		if len(core) < 10 {
			continue
		}
		for _, enc := range b32Encodings {
			s := enc.EncodeToString(core)
			out = append(out, s, strings.ToLower(s))
		}
	}
	// base32 and base32hex carry 5 bits per character, so a character is fixed
	// by the value alone when its 5 bits lie inside it. For each byte offset
	// of the value in a longer stream (0 to 4: base32 repeats every 5 bytes),
	// the needle is every such character: the whole value (offset 0, any
	// suffix, padding or none) and every embedding, at any length from
	// heldMatchFloor (at least 12 characters). The whole-group cores above stay.
	for off := 0; off < 5; off++ {
		buf := make([]byte, off+len(v)+5)
		copy(buf[off:], v)
		lo, hi := (8*off+4)/5, 8*(off+len(v))/5
		if hi-lo < 6 {
			continue // too few fixed characters to tell from chance
		}
		for _, enc := range b32Encodings {
			s := enc.EncodeToString(buf)[lo:hi]
			out = append(out, s, strings.ToLower(s))
		}
	}
	// The other fixed-alphabet and radix encodings (appdirect_encodings.go, Codex
	// r1 4.34.2 class secret-in-encoded-output).
	out = append(out, alphabetCores(v)...)
	return append(out, radixNeedles(v)...)
}

// heldMatchFloor is the shortest held value the shared funnel matched anywhere
// (its funnelMinSecretLen before the fleet credential filter); this file keeps
// its own copy so it builds against either version of the funnel.
const heldMatchFloor = 8

// shortMinLen is the length below which a held value is "very short": it is
// matched anywhere all the same (shortEchoes), with its whole-value encodings
// and every per-byte escape run of any length (byteEscapeEchoes). The other
// floors (the scattered spelling, the holder's parts) use it as their own.
const shortMinLen = 4

// shortEncodings returns the whole-value encodings of a short held value
// (base64, base64url, hex, base32, ASCII85): those of heldMatchFloor or
// more characters when long is false (they are held and matched anywhere),
// the shorter ones when long is true (they are matched as whole words).
func shortEncodings(v string, short bool) []string {
	if v == "" || len(v) >= heldMatchFloor {
		return nil
	}
	b := []byte(v)
	all := []string{
		base64.StdEncoding.EncodeToString(b), base64.RawStdEncoding.EncodeToString(b),
		base64.URLEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b),
		hex.EncodeToString(b), strings.ToUpper(hex.EncodeToString(b)),
		base32.StdEncoding.EncodeToString(b), base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b),
		base32.HexEncoding.EncodeToString(b), base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b),
	}
	for _, alphabet := range base85Alphabets {
		all = append(all, base85(b, alphabet))
	}
	var out []string
	for _, e := range all {
		if (len(e) < heldMatchFloor) == short {
			out = append(out, e)
		}
	}
	return out
}

func heldValues() []string {
	holdEncodedCores()
	heldSecrets.Lock()
	defer heldSecrets.Unlock()
	return append([]string(nil), heldSecrets.values...)
}

// HeldEchoes reports whether text repeats a held value in any form above,
// whole or split (see joinedEchoes), or spelled by its units in any order
// with anything between them (see scatteredEchoes).
func HeldEchoes(text string) bool {
	if text == "" {
		return false
	}
	return echoesForms(text, heldValues()) // every matcher (appdirect_decoded.go)
}

// shortEchoes covers the held values the shared funnel template leaves out
// (its heldMatchFloor floor). A value of any length, and each of its short
// encodings, is matched anywhere (as a substring) in the text and every view
// the funnel decodes from it, and as two adjacent parts (JSON string values,
// or words of the text or of one value); its longer encodings are held and
// matched by the funnel. Fail-closed by design, at every length (Codex r1
// appdirect class short credential in output: a value under shortMinLen was
// matched only where it stood alone, so "prefixabcsuffix" printed a held
// "abc"): a short value inside an unrelated word withholds that word, and a
// credential of a few characters withholds most of what would repeat it.
func shortEchoes(text string, held []string) bool {
	var views, leaves []string
	for _, h := range held {
		if len(h) >= heldMatchFloor {
			continue
		}
		if views == nil {
			var complete bool
			if views, complete = funnelViews(text); !complete {
				return true // the work budget ran out: fail closed, as the funnel does
			}
			leaves = jsonStringValueList(text)
		}
		for _, n := range append([]string{h}, shortEncodings(h, true)...) {
			for _, v := range views {
				if strings.Contains(v, n) {
					return true
				}
			}
		}
		if adjacentJoin(leaves, h) || adjacentJoin(words(text), h) {
			return true
		}
		for _, leaf := range leaves {
			if adjacentJoin(words(leaf), h) {
				return true
			}
		}
	}
	return false
}

// funnelViews is every view of text the shared funnel's decoders make (the
// steps FunnelEchoes walks, breadth-first under the same work budget), down
// to shortMinLen characters, so a needle below the funnel's own floor is
// searched in each of them (Codex r2 class undecoded-encoding-gap, fixed after
// the stop rule: HTML entities, tags and identity escapes were not searched).
// complete is false when the budget ran out.
func funnelViews(text string) (views []string, complete bool) {
	budget := funnelWorkPerByte*len(text) + funnelWorkBase
	seen := map[string]bool{text: true}
	queue := []string{text}
	for len(queue) > 0 {
		view := queue[0]
		queue = queue[1:]
		views = append(views, view)
		for _, step := range funnelSteps {
			if budget -= len(view) + 1; budget < 0 {
				return views, false
			}
			for _, next := range step(view) {
				if next == "" || seen[next] {
					continue
				}
				if budget -= len(next) + 1; budget < 0 {
					return views, false
				}
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return views, true
}

// words splits s on whitespace and trims quotes and table punctuation.
func words(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		if w := strings.Trim(f, "\"',:;|[]{}()"); w != "" {
			out = append(out, w)
		}
	}
	return out
}

func adjacentJoin(parts []string, h string) bool {
	for i := 0; i+1 < len(parts); i++ {
		if parts[i]+parts[i+1] == h {
			return true
		}
	}
	return false
}

func windowedEchoes(text string, held []string) bool {
	needles := cachedFunnelNeedles(held)
	if len(text) <= funnelWindow {
		return funnelEchoesNeedles(text, needles)
	}
	for start := 0; start < len(text); start += funnelWindow - funnelOverlap {
		end := min(start+funnelWindow, len(text))
		if funnelEchoesNeedles(text[start:end], needles) {
			return true
		}
		if end == len(text) {
			break
		}
	}
	return false
}

// needleCache keeps the funnel's needles for the last few held lists (keyed by
// a hash of the whole list): FunnelEchoes rebuilt them on every call, which
// dominated the output chokepoint's cost once every matcher reads every view.
var needleCache = struct {
	sync.Mutex
	order []uint64
	byKey map[uint64][]string
}{byKey: map[uint64][]string{}}

func cachedFunnelNeedles(held []string) []string {
	h := fnv.New64a()
	for _, v := range held {
		_, _ = h.Write([]byte(v))
		_, _ = h.Write([]byte{0})
	}
	key := h.Sum64()
	needleCache.Lock()
	if n, ok := needleCache.byKey[key]; ok {
		needleCache.Unlock()
		return n
	}
	needleCache.Unlock()
	n := funnelNeedles(held)
	needleCache.Lock()
	if _, ok := needleCache.byKey[key]; !ok {
		needleCache.byKey[key] = n
		needleCache.order = append(needleCache.order, key)
		if len(needleCache.order) > 8 {
			delete(needleCache.byKey, needleCache.order[0])
			needleCache.order = needleCache.order[1:]
		}
	}
	needleCache.Unlock()
	return n
}

// funnelEchoesNeedles is FunnelEchoes (secret_funnel.go) over needles already
// built: the same views, steps, input cap and work budget, failing closed the
// same way.
func funnelEchoesNeedles(text string, needles []string) bool {
	if len(needles) == 0 || text == "" {
		return false
	}
	if len(text) > funnelMaxInput {
		return true
	}
	budget := funnelWorkPerByte*len(text) + funnelWorkBase
	seen := map[string]bool{text: true}
	queue := []string{text}
	for len(queue) > 0 {
		view := queue[0]
		queue = queue[1:]
		for _, n := range needles {
			if strings.Contains(view, n) {
				return true
			}
		}
		for _, step := range funnelSteps {
			budget -= len(view) + 1
			if budget < 0 {
				return true
			}
			for _, next := range step(view) {
				if len(next) < funnelMinSecretLen || seen[next] {
					continue
				}
				budget -= len(next) + 1
				if budget < 0 {
					return true
				}
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

// joinedMinAlnum is the shortest letters-and-digits form of a held value the
// joined view matches; shorter ones would match by chance.
const joinedMinAlnum = 16

// joinedEchoes finds a held value split across lines, table cells or
// adjacent values: in the text with whitespace removed, in its JSON string
// values concatenated in document order, and in the text reduced to letters
// and digits (each held value reduced the same way).
func joinedEchoes(text string, held []string) bool {
	if j := strings.Join(strings.Fields(text), ""); j != text && windowedEchoes(j, held) {
		return true
	}
	if leaves := jsonStringValues(text); leaves != "" && windowedEchoes(leaves, held) {
		return true
	}
	at := lettersAndDigits(text)
	for _, h := range held {
		if n := lettersAndDigits(h); len(n) >= joinedMinAlnum && strings.Contains(at, n) {
			return true
		}
	}
	return false
}

// jsonStringValues concatenates every string value (not object keys) of the
// JSON values in text, from its first "{" or "[", in document order; "" when
// there is none.
func jsonStringValues(text string) string {
	return strings.Join(jsonStringValueList(text), "")
}

func jsonStringValueList(text string) []string {
	i := strings.IndexAny(text, "{[")
	if i < 0 {
		return nil
	}
	type frame struct{ object, wantKey bool }
	var stack []frame
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
	var out []string
	dec := json.NewDecoder(strings.NewReader(text[i:]))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, frame{object: true, wantKey: true})
			case '[':
				stack = append(stack, frame{})
			default:
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				valueDone()
			}
		case string:
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
				stack[n-1].wantKey = false
				continue
			}
			out = append(out, t)
			valueDone()
		default:
			valueDone()
		}
	}
	return out
}

func lettersAndDigits(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

var (
	jsonStringLiteral = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`)
	nonSpaceRun       = regexp.MustCompile(`\S+`)
)

// jsonMember is a JSON object member with a string value.
var jsonMember = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"(\s*:\s*)"((?:[^"\\\n]|\\.)*)"`)

// redactSecretMembers replaces the string value of every JSON member named
// like a credential (SecretFieldName: password, passhash, token, key,
// clientSecret, psk, secretAccessKey, key, ...) with RedactedValue, whatever its
// length or source: a request body a --dry-run preview prints, a credential a
// response carries. Each value it finds is held first, so the held-value pass
// that follows withholds it wherever else the text repeats it (a username
// equal to the password). Masks the API already sent ("****") stay.
func redactSecretMembers(text string) string {
	text = redactURLPasswords(text)
	if !strings.Contains(text, `":`) && !strings.Contains(text, `" :`) {
		return text
	}
	for _, sub := range jsonMember.FindAllStringSubmatch(text, -1) {
		var name, val string
		if json.Unmarshal([]byte(`"`+sub[1]+`"`), &name) == nil && SecretFieldName(name) &&
			json.Unmarshal([]byte(`"`+sub[3]+`"`), &val) == nil && !isMaskValue(val) {
			FunnelHold(val)
		}
	}
	text = redactNumericSecretMembers(text)
	return jsonMember.ReplaceAllStringFunc(text, func(m string) string {
		sub := jsonMember.FindStringSubmatch(m)
		var name, val string
		if json.Unmarshal([]byte(`"`+sub[1]+`"`), &name) != nil || !SecretFieldName(name) {
			return m
		}
		_ = json.Unmarshal([]byte(`"`+sub[3]+`"`), &val)
		if isMaskValue(val) {
			return m
		}
		return `"` + sub[1] + `"` + sub[2] + `"` + RedactedValue + `"`
	})
}

// jsonNumberMember is a JSON member whose value is a number.
var jsonNumberMember = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"(\s*:\s*)(-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)([\s,}\]]|$)`)

// redactNumericSecretMembers holds and redacts the number of every JSON member
// named like a credential that is not a measure (Codex r1 4.34.2 class
// holder-incomplete: {"otp":736251} was printed as written).
func redactNumericSecretMembers(text string) string {
	if !strings.Contains(text, `":`) && !strings.Contains(text, `" :`) {
		return text
	}
	return jsonNumberMember.ReplaceAllStringFunc(text, func(m string) string {
		sub := jsonNumberMember.FindStringSubmatch(m)
		var name string
		if json.Unmarshal([]byte(`"`+sub[1]+`"`), &name) != nil || !SecretFieldName(name) || numericMeasureName(name) {
			return m
		}
		holdNumber(json.Number(sub[3]))
		return `"` + sub[1] + `"` + sub[2] + `"` + RedactedValue + `"` + sub[4]
	})
}

// urlSchemeSep is how a URL's scheme separator is written as text: plain, and
// with the slashes JSON-escaped.
var urlSchemeSep = []string{"://", `:\/\/`}

// redactURLPasswords replaces the password of every URL userinfo in text
// (scheme://user:password@host) with RedactedValue, holding it first. A URL
// that carries a password is a credential wherever it comes from, so no holder
// has to have seen it first (Codex r2 class output-before-chokepoint, fixed
// after the stop rule: a refused credential store's base_url, and a base_url
// that does not parse, were printed as written). The userinfo runs to the last
// "@" before the next whitespace, quote, "?" or "#", so a raw "/" or "@" in a
// password is covered; a URL whose path holds an "@" after a port is redacted
// too (fail closed).
func redactURLPasswords(text string) string {
	if !strings.Contains(text, "://") && !strings.Contains(text, `:\/\/`) {
		return text
	}
	var b strings.Builder
	rest := text
	for {
		i, sep := -1, ""
		for _, s := range urlSchemeSep {
			if j := strings.Index(rest, s); j >= 0 && (i < 0 || j < i) {
				i, sep = j, s
			}
		}
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i+len(sep)])
		rest = rest[i+len(sep):]
		end := strings.IndexFunc(rest, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("\"'`<>?#", r) })
		if end < 0 {
			end = len(rest)
		}
		at := strings.LastIndex(rest[:end], "@")
		if at < 0 {
			continue
		}
		user, pw, ok := strings.Cut(rest[:at], ":")
		if !ok || pw == "" || isMaskValue(pw) {
			continue
		}
		FunnelHold(pw)
		b.WriteString(user + ":" + RedactedValue)
		rest = rest[at:]
	}
}

// WithholdText is the chokepoint: credential-named JSON members are redacted
// (redactSecretMembers); then the text is unchanged when it repeats no held
// value; otherwise each JSON string literal, then each whitespace token, that
// repeats one is replaced by the funnel's notice, and when that is not enough
// the whole text is.
func WithholdText(text string) string {
	text = redactSecretMembers(text)
	if !HeldEchoes(text) {
		return text
	}
	out := jsonStringLiteral.ReplaceAllStringFunc(text, func(lit string) string {
		if HeldEchoes(lit) {
			return `"` + FunnelWithheldText + `"`
		}
		return lit
	})
	if !HeldEchoes(out) {
		return out
	}
	out = nonSpaceRun.ReplaceAllStringFunc(out, func(tok string) string {
		if HeldEchoes(tok) {
			return FunnelWithheldText
		}
		return tok
	})
	if !HeldEchoes(out) {
		return out
	}
	if funnelReadsAsAuth(text) {
		return FunnelWithheldAuthText
	}
	return FunnelWithheldText
}

// WithholdBytes applies WithholdText line by line; a value split across
// neighbouring lines withholds the shortest run of up to splitWindow lines
// that repeats it, and anything still repeating one withholds the whole.
func WithholdBytes(b []byte) []byte {
	text := redactSecretMembers(string(b))
	if !HeldEchoes(text) {
		return []byte(text)
	}
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		lines[i] = withholdLine(line)
	}
	if !HeldEchoes(strings.Join(lines, "")) {
		return []byte(strings.Join(lines, ""))
	}
	for i := 0; i < len(lines); i++ {
		for j := i + 1; j < len(lines) && j < i+splitWindow; j++ {
			if !HeldEchoes(strings.Join(lines[i:j+1], "")) {
				continue
			}
			// Shrink to the shortest run ending at j that still repeats it.
			k := i
			for k+1 < j && HeldEchoes(strings.Join(lines[k+1:j+1], "")) {
				k++
			}
			for ; k <= j; k++ {
				lines[k] = withheldLine(lines[k])
			}
			i = j
			break
		}
	}
	out := strings.Join(lines, "")
	if HeldEchoes(out) {
		return []byte(FunnelWithheldText + "\n")
	}
	return []byte(out)
}

// splitWindow bounds how many neighbouring lines one split value may span
// before the whole text is withheld instead.
const splitWindow = 8

func withholdLine(line string) string {
	body := strings.TrimRight(line, "\r\n")
	return WithholdText(body) + line[len(body):]
}

func withheldLine(line string) string {
	body := strings.TrimRight(line, "\r\n")
	return FunnelWithheldText + line[len(body):]
}

// lineFilter receives a stream and writes it to dst through WithholdBytes,
// a burst at a time: output is held until the stream has been idle for
// filterIdle (or ends), so lines written together are checked together and a
// value split across them is caught. A burst over filterBurst is written up to
// its last filterKeep bytes of whole lines, which stay to be checked with what
// follows.
type lineFilter struct {
	mu      sync.Mutex
	dst     io.Writer
	pending []byte
	gen     int
	gate    <-chan struct{} // closed once the stream may deliver (see startupGate)
}

// openGate is the gate of a stream that may deliver at once (a file sink).
var openGate = func() <-chan struct{} { c := make(chan struct{}); close(c); return c }()

// startupGate keeps the process's startup streams from delivering anything
// until every startup source has been held (Codex r1 class
// output-before-chokepoint): the chokepoint is installed first, then the
// environment, the configuration and credential store and the command line
// are held, and only then is a line written while they loaded (a config
// refusal that repeats its base_url) checked and delivered. Fail closed: a
// line is never checked against a holder that is still filling.
type startupGate struct {
	ch   chan struct{}
	once sync.Once
}

func newStartupGate() *startupGate { return &startupGate{ch: make(chan struct{})} }

func (g *startupGate) open() { g.once.Do(func() { close(g.ch) }) }

// holdThenOpen runs hold (the caller's startup sources) and opens the gate,
// even when hold panics.
func (g *startupGate) holdThenOpen(hold func()) {
	defer g.open()
	if hold != nil {
		hold()
	}
}

func gateOpen(gate <-chan struct{}) bool {
	select {
	case <-gate:
		return true
	default:
		return false
	}
}

const (
	filterIdle  = 30 * time.Millisecond
	filterBurst = 1 << 20
	filterKeep  = 64 << 10
)

var filters sync.WaitGroup

func (f *lineFilter) feed(p []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, p...)
	if len(f.pending) > filterBurst && gateOpen(f.gate) {
		// ponytail: a value split across this cut is checked only with the
		// kept tail; raise filterKeep if bursts this large matter.
		if i := bytes.LastIndexByte(f.pending[:len(f.pending)-filterKeep], '\n'); i >= 0 {
			_, _ = f.dst.Write(WithholdBytes(f.pending[:i+1]))
			f.pending = append([]byte(nil), f.pending[i+1:]...)
		}
	}
	f.gen++
	if len(f.pending) > 0 {
		gen := f.gen
		time.AfterFunc(filterIdle, func() {
			<-f.gate
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.gen == gen {
				f.flushLocked()
			}
		})
	}
}

func (f *lineFilter) flushLocked() {
	if len(f.pending) > 0 {
		_, _ = f.dst.Write([]byte(DeliverChecked(string(WithholdBytes(f.pending)), func() string { return FunnelWithheldText + "\n" })))
		f.pending = nil
	}
}

// delivered is the tail of everything this process has delivered through the
// chokepoint (stdout and stderr bursts, MCP protocol messages, file sinks), so
// a held value spelled across two deliveries is caught by the second (Codex r1
// appdirect class credential split across protocol messages: two unknown-tool
// errors each repeated half of the token, and each was filtered alone). A
// stream's deliveries are checked together, and so are the streams.
var delivered = struct {
	sync.Mutex
	tail  string
	clean int // the number of held values the tail was last found clean against
}{}

// deliveredKeep bounds the tail. It is far longer than any credential in any
// encoding the funnel reads, and than funnelOverlap.
const deliveredKeep = 64 << 10

// DeliverChecked is the last step of every streaming output path: text (already
// withheld on its own) is checked again after the tail of what this process
// delivered before it. When the two together repeat a held value, the part
// that completed it is replaced by withheld() (the earlier part is already
// out). A tail that repeats a value held only since it was delivered is
// dropped first: it cannot be recalled, and checking against it would withhold
// everything after it. The delivered text joins the tail.
func DeliverChecked(text string, withheld func() string) string {
	if text == "" {
		return text
	}
	held := len(heldValues())
	delivered.Lock()
	defer delivered.Unlock()
	if delivered.tail != "" && delivered.clean != held {
		if HeldEchoes(delivered.tail) {
			delivered.tail = ""
		}
		delivered.clean = held
	}
	if delivered.tail != "" && HeldEchoes(delivered.tail+"\n"+text) {
		text = withheld()
	}
	tail := delivered.tail + "\n" + text
	if len(tail) > deliveredKeep {
		tail = tail[len(tail)-deliveredKeep:]
		delivered.clean = -1 // a cut tail is checked again before it is used
	}
	delivered.tail = tail
	return text
}

// filterPipe returns the write end of a pipe whose every byte reaches dst
// through the chokepoint; closeDst also closes dst once the stream ends.
func filterPipe(dst io.Writer, closeDst bool, gate <-chan struct{}) (*os.File, error) {
	w, _, err := filterPipeDone(dst, closeDst, gate)
	return w, err
}

// filterPipeDone is filterPipe plus a channel closed once the stream has
// ended and everything written to it has been delivered.
func filterPipeDone(dst io.Writer, closeDst bool, gate <-chan struct{}) (*os.File, <-chan struct{}, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	f := &lineFilter{dst: dst, gate: gate}
	done := make(chan struct{})
	filters.Add(1)
	go func() {
		defer filters.Done()
		defer close(done)
		defer r.Close()
		buf := make([]byte, 64<<10)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				f.feed(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
		<-f.gate
		f.mu.Lock()
		f.gen++
		f.flushLocked()
		f.mu.Unlock()
		if c, ok := dst.(io.Closer); ok && closeDst {
			_ = c.Close()
		}
	}()
	return w, done, nil
}

// startupStream is one process stream (os.Stdout or os.Stderr) during
// startup: ptr is the variable, dst the original stream, next its long-lived
// filter.
type startupStream struct {
	ptr  **os.File
	dst  *os.File
	next *os.File
}

// heldStartup points each stream at a gated filter while hold runs, so a line
// a startup source prints is checked only after hold has registered
// everything those sources carry (startupGate). It then points each stream at
// its long-lived filter and returns once the startup output has been
// delivered, so it precedes everything written later.
func heldStartup(hold func(), streams []startupStream) error {
	gate := newStartupGate()
	type started struct {
		s    startupStream
		w    *os.File
		done <-chan struct{}
	}
	var on []started
	finish := func() {
		for _, st := range on {
			*st.s.ptr = st.s.next
			if st.s.ptr == &os.Stderr {
				log.SetOutput(st.s.next)
			}
			_ = st.w.Close()
			<-st.done
		}
	}
	for _, s := range streams {
		w, done, err := filterPipeDone(s.dst, false, gate.ch)
		if err != nil {
			gate.open()
			finish()
			return err
		}
		on = append(on, started{s: s, w: w, done: done})
		*s.ptr = w
		if s.ptr == &os.Stderr {
			log.SetOutput(w)
		}
	}
	gate.holdThenOpen(func() {
		if hold != nil {
			hold()
		}
		holdEncodedCores()
	})
	finish()
	return nil
}

// InstallOutputFunnel routes os.Stdout, os.Stderr and the standard logger
// through the chokepoint, THEN runs holdStartup (every startup source the
// caller holds: environment, configuration, credential store, command line),
// and only then lets the streams deliver, so a warning a source prints while
// it loads is checked against everything that source carries. It reports
// whether each original stream was a terminal (so output format choices do
// not change). restore puts the streams back and waits until everything
// written, export files included, has been delivered.
func InstallOutputFunnel(holdStartup func()) (restore func(), outTTY, errTTY bool, err error) {
	origOut, origErr := os.Stdout, os.Stderr
	outTTY, errTTY = isCharDevice(origOut), isCharDevice(origErr)
	wOut, err := filterPipe(origOut, false, openGate)
	if err != nil {
		return func() {}, outTTY, errTTY, err
	}
	wErr, err := filterPipe(origErr, false, openGate)
	if err != nil {
		_ = wOut.Close()
		filters.Wait()
		return func() {}, outTTY, errTTY, err
	}
	if err := heldStartup(holdStartup, []startupStream{{&os.Stdout, origOut, wOut}, {&os.Stderr, origErr, wErr}}); err != nil {
		os.Stdout, os.Stderr = origOut, origErr
		log.SetOutput(origErr)
		_ = wOut.Close()
		_ = wErr.Close()
		filters.Wait()
		return func() {}, outTTY, errTTY, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			os.Stdout, os.Stderr = origOut, origErr
			log.SetOutput(origErr)
			_ = wOut.Close()
			_ = wErr.Close()
			filters.Wait()
		})
	}, outTTY, errTTY, nil
}

// InstallStderrFunnel routes os.Stderr and the standard logger through the
// chokepoint for the life of the process (the MCP server, whose stdout is the
// protocol channel and whose results pass withholdToolResults), then runs
// holdStartup and only then lets stderr deliver (see InstallOutputFunnel).
func InstallStderrFunnel(holdStartup func()) error {
	orig := os.Stderr
	w, err := filterPipe(orig, false, openGate)
	if err != nil {
		return err
	}
	return heldStartup(holdStartup, []startupStream{{&os.Stderr, orig, w}})
}

// WithholdingFile wraps a file the program is about to write: the caller
// writes and closes the returned pipe; the chokepoint writes the real file and
// closes it, and InstallOutputFunnel's restore waits for that.
func WithholdingFile(f *os.File, err error) (*os.File, error) {
	if err != nil || f == nil {
		return f, err
	}
	w, perr := filterPipe(f, true, openGate)
	if perr != nil {
		_ = f.Close()
		return nil, perr
	}
	return w, nil
}

// WaitOutput blocks until every filtered stream and file has been delivered.
func WaitOutput() { filters.Wait() }

func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// TTYWriter writes to W and answers the CLI's terminal check with TTY, the
// state of the stream the chokepoint replaced.
type TTYWriter struct {
	W   io.Writer
	TTY bool
}

func (t TTYWriter) Write(p []byte) (int, error) { return t.W.Write(p) }

// TerminalOverride is read by the CLI's isTerminal.
func (t TTYWriter) TerminalOverride() bool { return t.TTY }

// WithheldWriter writes each Write through WithholdBytes: the file sinks that
// append one record per Write (jobs ledger, feedback, learn and teach logs).
func WithheldWriter(w io.Writer) io.Writer { return withheldWriter{w} }

type withheldWriter struct{ w io.Writer }

func (h withheldWriter) Write(p []byte) (int, error) {
	out := DeliverChecked(string(WithholdBytes(p)), func() string { return FunnelWithheldText + "\n" })
	if _, err := h.w.Write([]byte(out)); err != nil {
		return 0, err
	}
	return len(p), nil
}
