// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: widens the shared secret-echo
// funnel (secret_funnel.go, byte-identical across connectors and left
// untouched) to credentials shorter than funnelMinSecretLen, which FunnelEchoes
// never looks for, and adds a JSON-document check for server SUCCESS data.
// Hand-fix code: cove-short-credential-funnel (Codex review rounds 1 and 2,
// press 4.34.0 reprint).

package cliutil

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FunnelWithholdAll is FunnelWithhold that also withholds text repeating any
// non-empty secret shorter than the shared funnel's minimum, as written or in
// any view the shared funnel's breadth-first search reaches (funnelShortEchoes).
// Every signal FunnelWithhold has is kept: it runs first.
func FunnelWithholdAll(text string, secrets []string) string {
	if w := FunnelWithhold(text, secrets); FunnelWithheld(w) {
		return w
	}
	if !funnelShortEchoes(text, secrets) {
		return text
	}
	if funnelReadsAsAuth(text) {
		return FunnelWithheldAuthText
	}
	return FunnelWithheldText
}

// FunnelEchoesAll reports whether FunnelWithholdAll would withhold text.
func FunnelEchoesAll(text string, secrets []string) bool {
	return FunnelWithheld(FunnelWithholdAll(text, secrets))
}

// FunnelHeldAll is FunnelWithholdAll over every value FunnelHold recorded: the
// CLI and MCP error backstops use it so a short credential is withheld too.
func FunnelHeldAll(text string) string {
	return FunnelWithholdAll(text, FunnelHeldValues())
}

// FunnelHeldValues is every value FunnelHold has recorded so far.
func FunnelHeldValues() []string {
	heldSecrets.Lock()
	defer heldSecrets.Unlock()
	return append([]string(nil), heldSecrets.values...)
}

// FunnelJSONEchoes reports whether a JSON document repeats one of secrets. It
// checks the document as written (every secret, any length), then every scalar
// token FunnelJSONScan visits (member names, strings, numbers in every
// rendering funnelNumberForms makes, booleans and null), token by token so a
// large result is never refused for its size alone. A document that is not a
// complete stream of valid JSON, or is not valid UTF-8, is also checked whole.
// Any failure signal wins.
func FunnelJSONEchoes(doc []byte, secrets []string) bool {
	FunnelHold(secrets...)
	if funnelRepeatsAsWritten(string(doc), secrets) {
		return true
	}
	floor := funnelNeedleFloor(secrets)
	if floor == 0 {
		return false
	}
	seen := map[string]bool{}
	echoed := false
	complete := FunnelJSONScan(doc, func(s FunnelJSONScalar) bool {
		forms, ok := funnelScalarForms(s.Value)
		if !ok {
			echoed = true
			return true
		}
		for _, form := range forms {
			// A decoded view is never twice as long as its source, so a form
			// that short cannot reach any needle.
			if 2*len(form) < floor || seen[form] {
				continue
			}
			seen[form] = true
			if FunnelEchoesAll(form, secrets) {
				echoed = true
				return true
			}
		}
		return false
	})
	if echoed {
		return true
	}
	if !complete || !utf8.Valid(doc) {
		return FunnelEchoesAll(string(doc), secrets)
	}
	return false
}

// FunnelJSONScalar is one scalar token of a JSON text: a member name (Key) or
// a value, with the decoded names of the members it sits under, outermost
// first. Value is a string, json.Number, bool or nil.
type FunnelJSONScalar struct {
	Key     bool
	Members []string
	Value   any
}

// FunnelJSONScan is the one JSON parser the cove funnel uses: it calls fn for
// every scalar token of doc, a stream of JSON values, in order, every member
// name and every value at any depth, duplicates kept (a map decode keeps only
// the last member of a name). fn returns true to stop. complete is false when
// doc is not a complete stream of valid JSON values (the tokens before the
// fault were still visited) or when fn stopped the scan.
func FunnelJSONScan(doc []byte, fn func(FunnelJSONScalar) bool) (complete bool) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	type frame struct {
		object, wantKey bool
		name            string
	}
	var stack []frame
	members := func() []string {
		var out []string
		for _, f := range stack {
			if f.object && !f.wantKey {
				out = append(out, f.name)
			}
		}
		return out
	}
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return len(stack) == 0
		}
		if err != nil {
			return false
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{':
				stack = append(stack, frame{object: true, wantKey: true})
			case '[':
				stack = append(stack, frame{})
			default:
				if len(stack) == 0 {
					return false
				}
				stack = stack[:len(stack)-1]
				valueDone()
			}
			continue
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
			name, _ := tok.(string)
			if fn(FunnelJSONScalar{Key: true, Members: members(), Value: tok}) {
				return false
			}
			stack[n-1].name, stack[n-1].wantKey = name, false
			continue
		}
		if fn(FunnelJSONScalar{Members: members(), Value: tok}) {
			return false
		}
		valueDone()
	}
}

// FunnelTextViews is text and every view one or two of the shared funnel's
// text decodings make of it (control characters dropped, JSON and JavaScript
// escapes, identity escapes, percent escapes, HTML entities, markup tags),
// deduplicated. Callers that look for structure in text that may be escaped
// (a visa member in a body that is not JSON) search every view. Text over the
// shared funnel's input limit gets one layer only (its display is withheld
// whole anyway).
func FunnelTextViews(text string) []string {
	decoders := []func(string) string{funnelDropUnprintable, funnelJSONUnescape, funnelIdentityUnescape, funnelPercentDecode, html.UnescapeString, funnelStripTagsText}
	seen := map[string]bool{text: true}
	views := []string{text}
	layers := 2
	if len(text) > funnelMaxInput {
		layers = 1
	}
	for depth, start := 0, 0; depth < layers; depth++ {
		end := len(views)
		for _, v := range views[start:end] {
			for _, decode := range decoders {
				if d := decode(v); !seen[d] {
					seen[d] = true
					views = append(views, d)
				}
			}
		}
		start = end
	}
	return views
}

// FunnelSettle is v with every text decoding the shared funnel knows applied
// until nothing changes: the decoded form of an escaped name or value.
func FunnelSettle(v string) string {
	return funnelSettle(v, funnelNeedleChains[len(funnelNeedleChains)-1])
}

// funnelShortEchoes is the shared funnel's breadth-first view search
// (FunnelEchoes) for secrets shorter than funnelMinSecretLen: the same work
// cap, the same fail-closed input limit and the same steps (funnelSteps), plus
// a base64 and a hex step whose run minimums fit the shortest needle, so every
// stack of encodings is decoded down to the secret. The needles are each short
// secret as written, as the text steps would show it, and in each encoding
// funnelShortForms makes.
func funnelShortEchoes(text string, secrets []string) bool {
	needles := funnelShortNeedles(secrets)
	if len(needles) == 0 || text == "" {
		return false
	}
	if len(text) > funnelMaxInput {
		return true
	}
	floor := len(needles[0])
	for _, n := range needles {
		floor = min(floor, len(n))
	}
	steps := append(append([]func(string) []string(nil), funnelSteps...), funnelShortRunSteps(floor)...)
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
		for _, step := range steps {
			budget -= len(view) + 1
			if budget < 0 {
				return true
			}
			for _, next := range step(view) {
				if len(next) < floor || seen[next] {
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

// funnelShortRunSteps decode base64 and hex runs as short as an encoding of a
// floor-byte value, reusing the shared funnel's run and decode functions. A
// layer of encoding never shrinks its input, so every layer around a short
// secret is a run at least this long.
func funnelShortRunSteps(floor int) []func(string) []string {
	minB64 := (floor*4 + 2) / 3
	return []func(string) []string{
		func(view string) []string {
			var runs []string
			funnelForRuns(view, funnelIsBase64, minB64, func(run string) { runs = append(runs, run) })
			return funnelJoinedViews(runs, 4, minB64, funnelLenientBase64)
		},
		func(view string) []string {
			var runs []string
			funnelForRuns(view, funnelIsHex, 2*floor, func(run string) { runs = append(runs, run) })
			return funnelJoinedViews(runs, 2, 2*floor, funnelLenientHex)
		},
	}
}

// funnelShortNeedles is every form a short secret is looked for in: its
// encodings (funnelShortForms), its settled decodings (a secret that itself
// holds decodable text) and a space form-encoded as '+'.
func funnelShortNeedles(secrets []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if s == "" || len(s) >= funnelMinSecretLen {
			continue
		}
		for _, f := range funnelShortForms(s) {
			add(f)
		}
		for _, chain := range funnelNeedleChains {
			add(funnelSettle(s, chain))
		}
		if strings.Contains(s, " ") {
			add(strings.ReplaceAll(s, " ", "+"))
		}
	}
	return out
}

// funnelNeedleFloor is the length of the shortest needle FunnelEchoesAll looks
// for with secrets (0 when there is none).
func funnelNeedleFloor(secrets []string) int {
	floor := 0
	for _, n := range append(funnelNeedles(secrets), funnelShortNeedles(secrets)...) {
		if floor == 0 || len(n) < floor {
			floor = len(n)
		}
	}
	return floor
}

// funnelRepeatsAsWritten reports whether text carries any secret, any length,
// exactly as written.
func funnelRepeatsAsWritten(text string, secrets []string) bool {
	for _, s := range secrets {
		if s = strings.TrimSpace(s); s != "" && strings.Contains(text, s) {
			return true
		}
	}
	return false
}

// funnelScalarForms is every text a scalar JSON value can be shown as. ok is
// false when a number cannot be rendered within bounds (fail closed).
func funnelScalarForms(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		return []string{t}, true
	case json.Number:
		return funnelNumberForms(string(t))
	case bool:
		return []string{strconv.FormatBool(t)}, true
	case nil:
		return []string{"null"}, true
	}
	return []string{fmt.Sprint(v)}, true
}

// funnelMaxNumberText bounds a number's exact decimal rendering; a number
// whose exact decimal is longer is refused rather than rendered.
const funnelMaxNumberText = 4096

// funnelNumberForms is a JSON number as written, its exact decimal (no
// exponent, no leading or trailing zeros), its digits alone (a renderer that
// groups or drops the point), signed and unsigned, and every float64 rendering
// a printer would use.
func funnelNumberForms(lit string) ([]string, bool) {
	forms := []string{lit}
	body := strings.TrimPrefix(lit, "-")
	mant, expText, hasExp := strings.Cut(strings.ToLower(body), "e")
	exp := 0
	if hasExp {
		e, err := strconv.Atoi(expText)
		if err != nil || e > funnelMaxNumberText || e < -funnelMaxNumberText {
			return nil, false
		}
		exp = e
	}
	intPart, fracPart, _ := strings.Cut(mant, ".")
	digits := intPart + fracPart
	point := len(intPart) + exp
	var exact string
	switch {
	case point <= 0:
		exact = "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		exact = digits + strings.Repeat("0", point-len(digits))
	default:
		exact = digits[:point] + "." + digits[point:]
	}
	if strings.Contains(exact, ".") {
		exact = strings.TrimRight(strings.TrimRight(exact, "0"), ".")
	}
	if i, f, ok := strings.Cut(exact, "."); ok {
		exact = funnelTrimLeadingZeros(i) + "." + f
	} else {
		exact = funnelTrimLeadingZeros(exact)
	}
	if len(exact) > funnelMaxNumberText {
		return nil, false
	}
	forms = append(forms, exact, "-"+exact, funnelTrimLeadingZeros(digits))
	if f, err := strconv.ParseFloat(lit, 64); err == nil {
		for _, format := range []byte{'g', 'G', 'e', 'E', 'f'} {
			forms = append(forms, strconv.FormatFloat(f, format, -1, 64))
		}
	}
	return forms, true
}

func funnelTrimLeadingZeros(s string) string {
	if t := strings.TrimLeft(s, "0"); t != "" {
		return t
	}
	return "0"
}

// funnelShortForms is a short secret as written and in the encodings a server
// or a log line would show it in.
func funnelShortForms(s string) []string {
	b := []byte(s)
	forms := []string{
		s,
		base64.StdEncoding.EncodeToString(b),
		base64.RawStdEncoding.EncodeToString(b),
		base64.URLEncoding.EncodeToString(b),
		base64.RawURLEncoding.EncodeToString(b),
		hex.EncodeToString(b),
		strings.ToUpper(hex.EncodeToString(b)),
		url.QueryEscape(s),
		url.PathEscape(s),
		html.EscapeString(s),
	}
	if j, err := json.Marshal(s); err == nil && len(j) >= 2 {
		forms = append(forms, string(j[1:len(j)-1]))
	}
	out := forms[:0]
	seen := map[string]bool{}
	for _, f := range forms {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}
