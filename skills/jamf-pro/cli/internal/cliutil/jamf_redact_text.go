// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// jamf-pro handfix (one-rule-everywhere): the one credential rule (Redact,
// jamf_redact.go) is applied wherever data is shown or kept, not only to
// fleet output: the generated formatters (every output format), the output
// chokepoint (CLI stdout and stderr, --deliver, export -o, MCP results and
// errors) and the local store. Before this, those paths applied a subset of
// the rule (credential-named string members and held values), so a
// credential the rule finds in any other shape printed: a name/value record
// labelled as a credential, a credential-named number, an opaque document
// such as a script body, a credential written inside a string or inside an
// encoded string (a request body echoed back base64-encoded), or an XML
// element named like a credential.
//
// RedactText applies the rule to text without changing anything else: every
// JSON document in it (the whole text, a line, or a JSON value embedded in a
// line) is decoded, judged by Redact, and only the values Redact replaces are
// spliced back in place, so key order, numbers and layout stay as printed.
// XML markup is judged element by element and attribute by attribute (an
// element or attribute is a field). Every credential value it finds is held
// first, so a repeat of it elsewhere in the output is withheld too.

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// RedactShown applies the one rule to decoded data about to be shown: its
// credential values are held (HoldCredentialValues), then Redact replaces
// them in place.
func RedactShown(v any) any {
	HoldCredentialValues(v)
	return Redact(v)
}

// ShownValue applies the one rule to any value about to be encoded as JSON
// (a preview built from Go values, request bodies as []byte included): it is
// encoded, decoded as plain JSON and passed through RedactShown. A value that
// cannot be encoded is returned as is (its encoder fails the same way).
func ShownValue(v any) any {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return v
	}
	decoded, err := decodeKeepingNumbers(buf.String())
	if err != nil {
		return v
	}
	return RedactShown(decoded)
}

// DocumentCarriesCredential reports a response body that is not a JSON
// document (a script, CSV or XML download) which the string rule would
// withhold: it has no fields for the rule to redact one by one, so it is
// judged whole, as one string value with no field name.
func DocumentCarriesCredential(raw []byte) bool {
	s := string(raw)
	return scrubString(s, "", map[string]bool{}, 0) != s
}

// RedactJSONBody applies the one rule to a response body that is one JSON
// object or array; any other body comes back as it was.
func RedactJSONBody(raw []byte) []byte {
	if out, ok := redactJSONDocument(string(raw), map[string]bool{}); ok {
		return []byte(out)
	}
	return raw
}

// RedactText applies the one rule to every JSON document and every XML
// element in text (see the file comment).
func RedactText(text string) string {
	if !strings.ContainsAny(text, "{[<") {
		return text
	}
	seen := map[string]bool{}
	if out, ok := redactJSONDocument(text, seen); ok {
		return out
	}
	lines := strings.SplitAfter(text, "\n")
	changed := false
	for i, line := range lines {
		if !strings.ContainsAny(line, "{[<") {
			continue
		}
		body := strings.TrimRight(line, "\r\n")
		out, ok := redactJSONDocument(body, seen)
		if !ok {
			out = redactXML(redactFragments(body, seen), seen, 0)
		}
		if out != body {
			lines[i] = out + line[len(body):]
			changed = true
		}
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "")
}

// redactJSONDocument applies the rule to s when s (spaces aside) is one JSON
// object or array. ok is false when it is not.
func redactJSONDocument(s string, seen map[string]bool) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') || !json.Valid([]byte(t)) {
		return "", false
	}
	orig, err1 := decodeKeepingNumbers(t)
	red, err2 := decodeKeepingNumbers(t)
	if err1 != nil || err2 != nil {
		return "", false
	}
	HoldCredentialValues(orig)
	red = redactValue(red, "", seen)
	if duplicateKeys(t) {
		// A repeated key decodes to its last value only, so a splice by key
		// path could miss the first: the whole document is re-encoded from
		// the redacted decode instead (the repeat is dropped).
		lead := s[:strings.Index(s, t)]
		return lead + encodeShown(red) + s[len(lead)+len(t):], true
	}
	repl := map[string]any{}
	diffPaths(orig, red, "", repl)
	if len(repl) == 0 {
		return s, true
	}
	return splicePaths(s, repl), true
}

// redactFragments applies the rule to every JSON object or array embedded
// in a line of text (a table cell, an error message).
func redactFragments(line string, seen map[string]bool) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(line); i++ {
		if !fragmentStart(line, i) {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line[i:]))
		dec.UseNumber()
		var v any
		if dec.Decode(&v) != nil {
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
		default:
			continue
		}
		end := i + int(dec.InputOffset())
		if out, ok := redactJSONDocument(line[i:end], seen); ok && out != line[i:end] {
			b.WriteString(line[last:i])
			b.WriteString(out)
			last = end
		}
		i = end - 1
	}
	if last == 0 {
		return line
	}
	b.WriteString(line[last:])
	return b.String()
}

// fragmentStart reports a '{' or '[' that can open a JSON document: an
// object whose next token is a key or its end, an array whose next token is
// a value or its end. Other brackets in text are not tried.
func fragmentStart(s string, i int) bool {
	if s[i] != '{' && s[i] != '[' {
		return false
	}
	rest := strings.TrimLeft(s[i+1:], " \t")
	if rest == "" {
		return false
	}
	if s[i] == '{' {
		return rest[0] == '"' || rest[0] == '}'
	}
	return strings.ContainsRune(`{["]-0123456789tfn`, rune(rest[0]))
}

func decodeKeepingNumbers(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}

func encodeShown(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return strconv.Quote(Redacted)
	}
	return strings.TrimRight(buf.String(), "\n")
}

// Path elements: an object member is "\x01"+key, an array element
// "\x02"+index; a path joins them with "\x00".
func memberPath(path, key string) string { return path + "\x00\x01" + key }
func elementPath(path string, i int) string {
	return path + "\x00\x02" + strconv.Itoa(i)
}

// diffPaths records, for every value b's redaction replaced, its path and
// the replacement (the outermost replaced value only).
func diffPaths(a, b any, path string, out map[string]any) {
	switch at := a.(type) {
	case map[string]any:
		bt, ok := b.(map[string]any)
		if !ok || len(bt) != len(at) {
			out[path] = b
			return
		}
		for k, av := range at {
			bv, ok := bt[k]
			if !ok {
				out[path] = b
				return
			}
			diffPaths(av, bv, memberPath(path, k), out)
		}
	case []any:
		bt, ok := b.([]any)
		if !ok || len(bt) != len(at) {
			out[path] = b
			return
		}
		for i := range at {
			diffPaths(at[i], bt[i], elementPath(path, i), out)
		}
	default:
		if !sameScalar(a, b) {
			out[path] = b
		}
	}
}

func sameScalar(a, b any) bool {
	switch at := a.(type) {
	case string:
		bt, ok := b.(string)
		return ok && at == bt
	case json.Number:
		bt, ok := b.(json.Number)
		return ok && at == bt
	case bool:
		bt, ok := b.(bool)
		return ok && at == bt
	case nil:
		return b == nil
	}
	return false
}

// splicePaths rewrites text, one JSON document, replacing the value at each
// path in repl with its JSON encoding; every other byte stays.
func splicePaths(text string, repl map[string]any) string {
	type frame struct {
		object, wantKey bool
		key             string
		index           int
		path            string
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var stack []*frame
	var b strings.Builder
	last := 0
	childPath := func() string {
		if len(stack) == 0 {
			return ""
		}
		f := stack[len(stack)-1]
		if f.object {
			return memberPath(f.path, f.key)
		}
		return elementPath(f.path, f.index)
	}
	valueDone := func() {
		if len(stack) == 0 {
			return
		}
		f := stack[len(stack)-1]
		if f.object {
			f.wantKey = true
		} else {
			f.index++
		}
	}
	for {
		before := int(dec.InputOffset())
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if s, ok := tok.(string); ok && len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].wantKey {
			stack[len(stack)-1].key = s
			stack[len(stack)-1].wantKey = false
			continue
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
			valueDone()
			continue
		}
		p := childPath()
		start := before + len(text[before:]) - len(strings.TrimLeft(text[before:], " \t\r\n:,"))
		if v, ok := repl[p]; ok {
			if d, isDelim := tok.(json.Delim); isDelim && (d == '{' || d == '[') {
				for depth := 1; depth > 0; {
					t2, err := dec.Token()
					if err != nil {
						break
					}
					if d2, ok := t2.(json.Delim); ok {
						if d2 == '{' || d2 == '[' {
							depth++
						} else {
							depth--
						}
					}
				}
			}
			b.WriteString(text[last:start])
			b.WriteString(encodeShown(v))
			last = int(dec.InputOffset())
			valueDone()
			continue
		}
		if d, ok := tok.(json.Delim); ok {
			stack = append(stack, &frame{object: d == '{', wantKey: d == '{', path: p})
			continue
		}
		valueDone()
	}
	b.WriteString(text[last:])
	return b.String()
}
