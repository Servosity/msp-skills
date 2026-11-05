// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 1: credential files. Every file Declarations.Files names is read
// token by token, never through a strict parser, so a file that does not
// parse still yields its credentials, and TOML, JSON, YAML and .env files
// read alike. Held from each: every declared credential key (and every value
// beneath one), every value under a headers table, auth_header, and the
// password and query values inside the base URL. Every member of a duplicate
// key is held. Keys keep the case they are written in.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// cfgValue is one scalar the reader found: the keys leading to it, outermost
// first and its own key last, as written.
type cfgValue struct {
	keys  []string
	value string
	raw   string // as written, before unescaping
	stray bool   // a scalar the reader could not classify (held from a credential file whatever its key)
}

type cfgFrame struct {
	key   []string // the keys that opened it
	array bool
}

// cfgParent is a YAML key whose value is the indented block below it.
type cfgParent struct {
	indent int
	keys   []string
}

type cfgReader struct {
	b         []byte
	i         int
	table     []string
	stack     []cfgFrame
	parents   []cfgParent
	bol       int // where the current line begins
	indent    int // the current line's indentation
	lineStart bool
	listItem  bool                // a YAML "- " item: a bare word is an element, not a key
	anchors   map[string][]string // a YAML anchor's name: the keys of the node it names
	tagged    bool                // a YAML tag stands before the node value reads
	props     bool                // YAML node properties (a tag or an anchor) stand before it
	pending   bool                // a line held only node properties: the next scalar is their node's
	out       []cfgValue
}

// tomlHeader matches a [table] or [[table]] line. A line that opens with [
// but is not one (a JSON or YAML array) is read as an array.
var tomlHeader = headerMatcher{}

type headerMatcher struct{}

func (headerMatcher) Match(line []byte) bool { return tomlHeaderEnd(line) > 0 }

// tomlHeaderEnd is where the brackets of a [table] or [[table]] header that
// is the whole line end, or -1. Quotes are respected: a quoted key may carry
// any character, ']' included (kit14rv: the earlier pattern read
// [token."x]y"] as an array, and the table's credentials went unheld). Outside
// quotes the key holds no "[]{},:=".
func tomlHeaderEnd(line []byte) int {
	if len(line) == 0 || line[0] != '[' {
		return -1
	}
	i, key := 1, false
	if len(line) > 1 && line[1] == '[' {
		i = 2
	}
	for ; i < len(line); i++ {
		switch c := line[i]; {
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(line) && line[j] != c && line[j] != '\n' {
				if c == '"' && line[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(line) || line[j] != c {
				return -1
			}
			i, key = j, true
		case c == ']':
			if !key {
				return -1
			}
			end := i + 1
			if end < len(line) && line[end] == ']' {
				end++
			}
			rest := bytes.TrimLeft(line[end:], " \t")
			if len(rest) == 0 || rest[0] == '#' || len(rest) == 1 && rest[0] == '\r' {
				return end
			}
			return -1
		case c == '\n' || strings.IndexByte("[{},:=", c) >= 0:
			return -1
		default:
			key = true
		}
	}
	return -1
}

// readConfig is the token-by-token reader.
func readConfig(b []byte) []cfgValue {
	r := &cfgReader{b: b, lineStart: true, anchors: map[string][]string{}}
	r.run(nil)
	return r.out
}

// run reads tokens until the end, or until stop reports true before a token
// starts.
func (r *cfgReader) run(stop func() bool) {
	b := r.b
	for r.i < len(b) {
		c := b[r.i]
		switch {
		case c == '\n':
			r.i++
			r.bol = r.i
			r.lineStart = true
			continue
		case c == ' ' || c == '\t' || c == '\r' || c == ',' || c == ';':
			r.i++
			continue
		case c == '#' || (c == '/' && r.at(1) == '/'):
			r.skipLine()
			continue
		case r.lineStart && len(r.stack) == 0 && r.lineHead(c):
			continue
		case stop != nil && stop():
			return
		case c == '[' && r.lineStart && len(r.stack) == 0 && tomlHeader.Match(r.line()):
			r.header()
		case c == '{' || c == '[':
			r.stack = append(r.stack, cfgFrame{key: r.path(), array: c == '['})
			r.i++
		case c == '}' || c == ']':
			if len(r.stack) > 0 {
				r.stack = r.stack[:len(r.stack)-1]
			}
			r.i++
		default:
			r.item()
		}
		r.lineStart = false
	}
}

func (r *cfgReader) at(k int) byte {
	if r.i+k < len(r.b) {
		return r.b[r.i+k]
	}
	return 0
}

// line is the rest of the current line.
func (r *cfgReader) line() []byte {
	end := r.i
	for end < len(r.b) && r.b[end] != '\n' {
		end++
	}
	return r.b[r.i:end]
}

// lineHead handles the first character of a line outside any {} or []: it
// closes the YAML blocks the line's indentation ends, and reads a "- " list
// marker. It reports whether it consumed c.
func (r *cfgReader) lineHead(c byte) bool {
	if c == '#' || c == '\r' {
		return false
	}
	r.indent = r.i - r.bol
	for len(r.parents) > 0 && r.parents[len(r.parents)-1].indent >= r.indent {
		r.parents = r.parents[:len(r.parents)-1]
	}
	if c == '-' && (r.at(1) == ' ' || r.at(1) == '\t' || r.at(1) == '\n' || r.at(1) == '\r' || r.at(1) == 0) {
		r.i++
		r.skipBlanks()
		r.lineStart = false
		r.listItem = true
		return true
	}
	return false
}

func (r *cfgReader) skipLine() {
	for r.i < len(r.b) && r.b[r.i] != '\n' {
		r.i++
	}
}

func (r *cfgReader) skipBlanks() {
	for r.i < len(r.b) && (r.b[r.i] == ' ' || r.b[r.i] == '\t') {
		r.i++
	}
}

// path is the keys enclosing the reader's position.
func (r *cfgReader) path() []string {
	if len(r.stack) > 0 {
		return r.stack[len(r.stack)-1].key
	}
	if len(r.parents) > 0 {
		return r.parents[len(r.parents)-1].keys
	}
	return r.table
}

func (r *cfgReader) inArray() bool {
	return len(r.stack) > 0 && r.stack[len(r.stack)-1].array
}

// header reads a [table] or [[table]] line.
func (r *cfgReader) header() {
	end := r.i + max(0, tomlHeaderEnd(r.line()))
	h := string(r.b[r.i:end])
	r.skipLine()
	h = strings.TrimPrefix(strings.TrimPrefix(h, "["), "[")
	h = strings.TrimSuffix(strings.TrimSuffix(h, "]"), "]")
	r.table = splitKey(strings.TrimSpace(h))
	r.parents = nil
}

// quotedDottedKeyEnd is where the separator ('=' or ':') after a dotted key
// that starts at i and has a quoted part and a dot outside quotes is, or -1:
// quoted parts, bare key characters, dots and blanks, on one line.
func quotedDottedKeyEnd(b []byte, i int) int {
	quote, dot := false, false
	for ; i < len(b); i++ {
		switch c := b[i]; {
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(b) && b[j] != c && b[j] != '\n' {
				if c == '"' && b[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(b) || b[j] != c {
				return -1
			}
			i, quote = j, true
		case c == '.':
			dot = true
		case c == '=' || c == ':':
			if quote && dot {
				return i
			}
			return -1
		case c == '\n' || c == '\r' || strings.IndexByte(",{}[]#;", c) >= 0:
			return -1
		}
	}
	return -1
}

// splitKey splits a dotted key outside quotes, unquoted.
func splitKey(k string) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c == '"' || c == '\'':
			// A quoted part is unquoted as word() unquotes a quoted key
			// (kit14rv: ["to\u006ben"] stayed escaped and its table's
			// credentials went unheld).
			j := i + 1
			for j < len(k) && k[j] != c {
				if c == '"' && k[j] == '\\' {
					j++
				}
				j++
			}
			cur.WriteString(unquote(c, []byte(k[i+1:min(j, len(k))])))
			i = j
		case c == '.':
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(out, strings.TrimSpace(cur.String()))
}

// item reads a key and its value, or an array or YAML list element. A
// line the reader cannot classify (a word and no separator) is a stray:
// held from a credential file whatever its key (fail closed), but for the
// structure of the formats themselves ("export", YAML document markers and
// directives).
func (r *cfgReader) item() {
	start := r.i
	atLine := r.lineStart && len(r.stack) == 0
	listItem := r.listItem
	r.listItem = false
	pending := r.pending
	r.pending = false
	// YAML node properties (tags, verbatim tags, anchors, in any order) at
	// the start of a block item mark the node after them, as in value: the
	// text as written is held too, and they are passed over before the
	// word, so a tagged key (`!!str token: x`) stays a key. Properties with
	// nothing after them but a comment mark the scalar on a following line:
	// that is pending.
	if c := r.at(0); !listItem && !r.inArray() && (c == '!' || c == '&') {
		at := r.i
		v, raw, _ := r.word(true)
		r.add(r.path(), v, raw)
		r.i = at
		for c := r.at(0); c == '!' || c == '&'; c = r.at(0) {
			p := r.i
			for r.i < len(r.b) && !isSpaceByte(r.b[r.i]) {
				r.i++
			}
			if r.b[p] == '&' {
				r.anchors[string(r.b[p+1:r.i])] = r.path()
			}
			r.skipBlanks()
		}
		if c := r.at(0); r.i >= len(r.b) || c == '\n' || c == '\r' || c == '#' {
			r.skipLine()
			r.pending = true
			return
		}
		pending = true
	}
	// A dotted key with a quoted part ("token".value, 'a' . 'b') is one
	// key, as TOML reads it (kit14rv: word() ended at the first quoted
	// part's close, and the assignment went unread).
	if !r.inArray() {
		if end := quotedDottedKeyEnd(r.b, r.i); end > 0 {
			key := splitKey(string(r.b[r.i:end]))
			sep := r.b[end]
			r.i = end + 1
			r.value(append(append([]string{}, r.path()...), key...), sep)
			return
		}
	}
	wordAt := r.i
	word, raw, quoted := r.word(r.inArray())
	r.skipBlanks()
	if !r.inArray() && (r.at(0) == '=' || r.at(0) == ':') {
		sep := r.at(0)
		r.i++
		key := []string{word}
		if !quoted {
			key = splitKey(word)
		}
		r.value(append(append([]string{}, r.path()...), key...), sep)
		return
	}
	if listItem {
		r.i = start
		r.value(r.path(), 0)
		return
	}
	// YAML node properties in front of a flow element (`[!!str ~]`): hold
	// the scalar they mark as a value under these keys (not as a stray,
	// which drops punctuation like "~"), and the text as written too.
	if r.inArray() && !quoted && (strings.HasPrefix(word, "!") || strings.HasPrefix(word, "&")) {
		if v, ok := splitProps(word); ok {
			r.add(r.path(), word, raw)
			if v != "" {
				r.add(r.path(), v, v)
			}
			if r.i == start {
				r.i++
			}
			return
		}
	}
	// The scalar node properties marked, on their line or a following one:
	// a value, whatever it holds, under these keys, read as any scalar is
	// (its quotes end it, a comment after it is not part of it), and the
	// line as written too.
	if pending && !r.inArray() {
		if atLine {
			if rest := strings.TrimSpace(string(r.line())); rest != "" {
				r.add(r.path(), word+" "+rest, raw)
			}
		}
		r.i, r.tagged = wordAt, true // properties mark it: a null is a value too
		r.value(r.path(), 0)
		if r.i == start {
			r.i++
		}
		return
	}
	switch {
	case r.inArray():
		r.add(r.path(), word, raw)
	case atLine && !quoted && word == "export":
		return // env: "export KEY=value"
	case atLine && !quoted && (strings.HasPrefix(word, "---") || strings.HasPrefix(word, "...") || strings.HasPrefix(word, "%")):
		r.skipLine() // YAML document markers and directives
		return
	case atLine:
		rest := strings.TrimSpace(string(r.line()))
		r.skipLine()
		r.stray(append(slices.Clone(r.path()), word), rest)
		r.stray(r.path(), strings.TrimSpace(word+" "+rest))
		return
	default:
		r.stray(r.path(), word)
	}
	if r.i == start {
		r.i++ // a stray character
	}
}

// stray adds an unclassified scalar; one with no letter or digit is
// punctuation the reader passed over, not a value.
func (r *cfgReader) stray(keys []string, v string) {
	if strings.IndexFunc(v, func(c rune) bool { return c > 0x7f || c >= '0' && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'z' }) >= 0 {
		r.out = append(r.out, cfgValue{keys: keys, value: v, raw: v, stray: true})
	}
}

// splitProps splits leading YAML node properties (tags and anchors, any
// number, in any order) off a scalar read in value position (they mark the
// node after them, so the scalar is a value even when it is punctuation a
// bare word passes over, like "~"). It returns the scalar after them,
// unquoted, and whether there were any. Properties with no scalar on their
// own line return ("", true): the scalar is elsewhere.
func splitProps(raw string) (val string, ok bool) {
	s := strings.TrimSpace(raw)
	for s != "" && (s[0] == '!' || s[0] == '&') {
		ok = true
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			return "", true
		}
		s = strings.TrimSpace(s[i:])
	}
	if !ok {
		return "", false
	}
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return unquote(s[0], []byte(s[1:len(s)-1])), true
	}
	return s, true
}

func isNull(v string) bool { return v == "null" || v == "Null" || v == "NULL" || v == "~" }

// value reads the value after a key's separator (sep is ':' or '=', or 0
// for a list element).
func (r *cfgReader) value(keys []string, sep byte) {
	r.skipBlanks()
	if c := r.at(0); sep != '=' && (c == '&' || c == '!') {
		// YAML node properties (an anchor, a tag, in any order) in front
		// of the node: the node is read as if they were not there. The
		// text as written is held too: it may be a value that only looks
		// like one.
		at := r.i
		v, raw, _ := r.word(true)
		r.add(keys, v, raw)
		r.i = at
		for c := r.at(0); c == '&' || c == '!'; c = r.at(0) {
			p := r.i
			for r.i < len(r.b) && !isSpaceByte(r.b[r.i]) && (len(r.stack) == 0 || strings.IndexByte(",]}", r.b[r.i]) < 0) {
				r.i++
			}
			if r.b[p] == '&' {
				r.anchors[string(r.b[p+1:r.i])] = keys
			} else {
				r.tagged = true
			}
			r.props = true
			r.skipBlanks()
		}
		r.value(keys, sep)
		return
	}
	tagged, props := r.tagged, r.props
	r.tagged, r.props = false, false
	switch c := r.at(0); {
	case r.i >= len(r.b) || c == '\n' || c == '\r' || c == '#':
		// Node properties with their scalar on the next indented line:
		// hold that scalar under these keys (not a parent whose child is
		// then read as a stray and dropped). A mapping or a sequence there
		// is read as children, as for any parent.
		if props {
			if at, ok := r.taggedBlockScalar(); ok {
				// Read there as any scalar is read: its quotes end it, a
				// comment after it is not part of it, and a null holds no
				// value unless tagged.
				r.i, r.tagged = at, tagged
				r.value(keys, sep)
				return
			}
		}
		// A YAML parent key: its children are the more indented lines
		// that follow.
		r.parents = append(r.parents, cfgParent{indent: r.indent, keys: keys})
	case c == '{' || c == '[':
		r.stack = append(r.stack, cfgFrame{key: keys, array: c == '['})
		r.i++
	default:
		v, raw, quoted := r.word(true)
		if quoted {
			r.add(keys, v, raw)
			if strings.Contains(raw, "\n") {
				r.add(keys, foldLines(v), raw) // YAML folds a quoted scalar's line breaks
			}
			return
		}
		if m := blockScalar.FindStringSubmatch(v); m != nil {
			r.block(keys, m[1] == ">")
			return
		}
		if sep != '=' && !tagged && isNull(v) {
			return // an untagged YAML or JSON null: no value (in env, TOML and INI files, text)
		}
		// A YAML alias: the node its anchor names, under these keys.
		if v[0] == '*' {
			if path, ok := r.anchors[v[1:]]; ok {
				for _, x := range r.out[:len(r.out):len(r.out)] {
					if len(x.keys) >= len(path) && slices.Equal(x.keys[:len(path)], path) {
						r.add(append(slices.Clone(keys), x.keys[len(path):]...), x.value, x.raw)
					}
				}
			}
		}
		r.add(keys, v, raw)
		// A bare value with a trailing comment is held both ways.
		if c, ok := commentCut(v); ok {
			r.add(keys, c, raw)
		}
		// A YAML plain scalar goes on over more indented lines.
		if sep == ':' && len(r.stack) == 0 {
			if more := r.plainLines(); len(more) > 0 {
				r.add(keys, foldLines(strings.Join(append([]string{v}, more...), "\n")), raw+"\n"+strings.Join(more, "\n"))
			}
		}
	}
}

// plainLines reads the lines that continue a YAML plain scalar: each more
// indented than the key's line and not itself a key, a list item or a
// comment.
func (r *cfgReader) plainLines() []string {
	var lines []string
	at := r.i
	for {
		nl := bytes.IndexByte(r.b[at:], '\n')
		if nl < 0 {
			break
		}
		start := at + nl + 1
		end := start + bytes.IndexByte(r.b[start:], '\n')
		if end < start {
			end = len(r.b)
		}
		line := strings.TrimRight(string(r.b[start:end]), "\r")
		body := strings.TrimLeft(line, " \t")
		if body == "" {
			lines = append(lines, "")
			at = end
			continue
		}
		if len(line)-len(body) <= r.indent || body[0] == '#' || strings.HasPrefix(body, "- ") ||
			strings.Contains(body, ": ") || strings.HasSuffix(body, ":") {
			break
		}
		lines = append(lines, strings.TrimSpace(body))
		at = end
		r.i = end
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// taggedBlockScalar finds the scalar on the line after node properties
// whose key line carried no inline value (`token: !!str` then `  ~` on the
// next line): the next line, past blank and comment lines, indented deeper
// than the key's line. It returns where that scalar starts. It reports
// false, and consumes nothing, when there is none or that line is a
// mapping key or a sequence item (the properties mark a collection; its
// children are read as children).
func (r *cfgReader) taggedBlockScalar() (int, bool) {
	save := r.i
	at := r.i
	// Skip blank lines and comment lines between the tag and its scalar.
	for {
		nl := bytes.IndexByte(r.b[at:], '\n')
		if nl < 0 {
			r.i = save
			return 0, false
		}
		start := at + nl + 1
		end := start + bytes.IndexByte(r.b[start:], '\n')
		if end < start {
			end = len(r.b)
		}
		line := strings.TrimRight(string(r.b[start:end]), "\r")
		body := strings.TrimLeft(line, " \t")
		if body == "" || body[0] == '#' {
			at = end
			continue // a blank or comment line between the tag and its scalar
		}
		quoted := body[0] == '"' || body[0] == '\''
		if len(line)-len(body) <= r.indent || !quoted && (body == "-" || strings.HasPrefix(body, "- ") ||
			strings.Contains(body, ": ") || strings.HasSuffix(body, ":")) {
			r.i = save
			return 0, false
		}
		return start + len(line) - len(body), true // the scalar's first byte
	}
}

// foldLines is YAML line folding: each line trimmed, a line break a space,
// an empty line a line break.
func foldLines(v string) string {
	var b strings.Builder
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		l = strings.TrimSpace(l)
		switch {
		case i == 0:
		case l == "":
			b.WriteByte('\n')
			continue
		case lines[i-1] != "" || i == 1:
			b.WriteByte(' ')
		}
		b.WriteString(l)
	}
	return b.String()
}

// blockScalar is a YAML block scalar indicator: | or >, with its chomping
// and indentation marks.
var blockScalar = regexp.MustCompile(`^([|>])[-+0-9]*\s*(#.*)?$`)

// block reads the lines of a YAML block scalar, every line after the
// indicator indented deeper than the key's own line, and adds the value
// (lines joined by a line break, or a space when folded) and each line.
func (r *cfgReader) block(keys []string, folded bool) {
	r.skipLine()
	var lines []string
	cut := -1
	for r.i < len(r.b) {
		start := r.i + 1
		end := start
		for end < len(r.b) && r.b[end] != '\n' {
			end++
		}
		line := strings.TrimRight(string(r.b[start:end]), "\r")
		body := strings.TrimLeft(line, " \t")
		if body == "" {
			lines = append(lines, "")
			r.i = end
			continue
		}
		ind := len(line) - len(body)
		if ind <= r.indent {
			break
		}
		if cut < 0 {
			cut = ind
		}
		lines = append(lines, line[min(cut, ind):])
		r.i = end
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	sep := "\n"
	if folded {
		sep = " "
	}
	r.add(keys, strings.Join(lines, sep), strings.Join(lines, "\n"))
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			r.add(keys, t, l)
		}
	}
}

// word reads a quoted string, or a bare token. A bare value runs to the end
// of the line (or a separator inside {} and []); a bare key stops at blanks
// and punctuation.
func (r *cfgReader) word(isValue bool) (text, raw string, quoted bool) {
	b := r.b
	if q := b[r.i]; q == '"' || q == '\'' {
		if r.at(1) == q && r.at(2) == q {
			body := r.i + 3
			end := strings.Index(string(b[body:]), string([]byte{q, q, q}))
			if end < 0 {
				end = len(b) - body
			}
			r.i = min(len(b), body+end+3)
			return unquote(q, b[body:body+end]), string(b[body : body+end]), true
		}
		// A quoted string may run over lines (YAML, .env) when its close
		// ends a value; else it is unterminated and its line ends it.
		j, nl := r.i+1, -1
		for j < len(b) {
			if b[j] == q && !(q == '\'' && j+1 < len(b) && b[j+1] == '\'') {
				break
			}
			if b[j] == '\n' && nl < 0 {
				nl = j
			}
			if q == '"' && b[j] == '\\' || q == '\'' && b[j] == '\'' {
				j++ // an escape: a backslash, or YAML's '' inside single quotes
			}
			j++
		}
		if nl >= 0 && (j >= len(b) || !endsValue(b[j+1:])) {
			j = nl
		}
		j = min(j, len(b))
		body := b[r.i+1 : j]
		r.i = min(len(b), j+1)
		if j < len(b) && b[j] == '\n' {
			r.i = j // unterminated: the line ends it
		}
		return unquote(q, body), string(body), true
	}
	start := r.i
	nested := len(r.stack) > 0
	for r.i < len(b) {
		c := b[r.i]
		if c == '\n' || c == '\r' {
			break
		}
		if isValue {
			if nested && (c == ',' || c == '}' || c == ']') {
				break
			}
		} else if c == ' ' || c == '\t' || strings.IndexByte("=:,{}[]\"'#;", c) >= 0 {
			break
		}
		r.i++
	}
	t := strings.TrimSpace(string(b[start:r.i]))
	return t, t, false
}

// endsValue reports whether b, what follows a closing quote, ends the
// value: blanks, then the line's end, a comment or a separator.
func endsValue(b []byte) bool {
	b = bytes.TrimLeft(b, " \t\r")
	return len(b) == 0 || strings.IndexByte("\n#,}]", b[0]) >= 0
}

func unquote(q byte, body []byte) string {
	if q == '\'' {
		return strings.ReplaceAll(string(body), "''", "'")
	}
	return string(unescape(continued(yamlEscapes(body)), true))
}

// unescapeAny is a double-quoted span read as the config reader reads it.
func unescapeAny(span string) string { return unquote('"', []byte(span)) }

// yamlNamedEscapes are YAML's escapes that no other format the kit reads
// spells (kit15rv Codex r1: "\N" is U+0085 in YAML, and the reader held
// "N").
var yamlNamedEscapes = map[byte]string{'N': "\u0085", '_': "\u00a0", 'L': "\u2028", 'P': "\u2029"}

// yamlEscapes rewrites YAML's named escapes as \u escapes, which unescape
// (unescape keeps them); an escaped backslash is passed over whole.
func yamlEscapes(b []byte) []byte {
	if !bytes.ContainsAny(b, "N_LP") {
		return b
	}
	var out []byte
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			if r, ok := yamlNamedEscapes[b[i+1]]; ok {
				out = append(out, r...)
				i++
				continue
			}
			out = append(out, b[i], b[i+1])
			i++
			continue
		}
		out = append(out, b[i])
	}
	return out
}

// continued drops each line-ending backslash with the line break and the
// blanks after it (a TOML multi-line basic string, a shell line).
func continued(b []byte) []byte {
	if !bytes.Contains(b, []byte("\\\n")) && !bytes.Contains(b, []byte("\\\r\n")) {
		return b
	}
	var out []byte
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			if b[i+1] == '\\' {
				out = append(out, b[i], b[i+1])
				i++
				continue
			}
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\r') {
				j++
			}
			if j < len(b) && b[j] == '\n' {
				for j < len(b) && isSpaceByte(b[j]) {
					j++
				}
				i = j - 1
				continue
			}
		}
		out = append(out, b[i])
	}
	return out
}

func (r *cfgReader) add(keys []string, v, raw string) {
	r.out = append(r.out, cfgValue{keys: keys, value: v, raw: raw})
}

// configSource is the source a config value is held under, or "" when the
// value is not a credential.
func configSource(keys []string, d *Declarations) string {
	if len(keys) == 0 {
		return ""
	}
	key := keys[len(keys)-1]
	for _, k := range keys[:len(keys)-1] {
		if strings.EqualFold(k, "headers") {
			return "header:" + strings.ToLower(key)
		}
	}
	for _, k := range keys {
		if strings.EqualFold(k, "auth_header") {
			return "config:" + k
		}
		if containsFold(append(slices.Clone(d.ConfigFields), d.StoredRecords...), k) {
			return "config:" + k
		}
	}
	return ""
}

// baseURLKey reports whether keys lead to a base URL: hold when the last key
// is the base URL key's name (its secrets are held wherever it appears), and
// use when the keys are the whole declared path.
func baseURLKey(d *Declarations, keys []string) (hold, use bool) {
	want := splitKey("base_url")
	if d.BaseURL.ConfigKey != "" {
		want = splitKey(d.BaseURL.ConfigKey)
	}
	if len(keys) == 0 || !strings.EqualFold(keys[len(keys)-1], want[len(want)-1]) {
		return false, false
	}
	if len(want) == 1 {
		return true, true
	}
	return true, slices.EqualFunc(keys, want, strings.EqualFold)
}

// credentialAssignmentsLocked is contract 1's blunt rule for a file that
// does not parse (kit13rv, config-reader-drops-values, after it came back in
// eight passes): wherever a declared credential key, "headers", auth_header
// or the base URL key is followed by a separator, the value after it is read
// afresh, as if the key began the file, whatever the reader made of the text
// around it (an unclosed array or table, a quoted string that ran over
// lines, any nesting). A table header naming one of them is read afresh to
// the next header the same way. A whole comment line is not an assignment in
// any format the kit reads and is passed over; a comment after a value is not
// (the reader cannot know the line's state there). On a file that parses,
// what this reads the reader read already.
func credentialAssignmentsLocked(b []byte) []cfgValue {
	d := &reg.decl
	names := []string{"headers", "auth_header", "base_url"}
	if d.BaseURL.ConfigKey != "" {
		k := splitKey(d.BaseURL.ConfigKey)
		names = append(names, k[len(k)-1])
	}
	for _, n := range append(slices.Clone(d.ConfigFields), d.StoredRecords...) {
		k := splitKey(n)
		names = append(names, k[len(k)-1])
	}
	// The keys are found as the reader reads and compares them (kit14rv,
	// after the scan's own quoting rules disagreed with the reader's): the
	// reader's word() is run from every quote and from every start of a
	// bare word, and a candidate is where its text, or one of its dotted
	// parts for a bare word, is a name under Unicode case folding. A run
	// of key characters inside any text is a candidate too (a superset:
	// the assignment it starts is read afresh either way).
	// ponytail: word() from every quote is quadratic on a file of many
	// unclosed quotes; credential files are small.
	keyChar := func(c byte) bool {
		return c == '_' || c == '-' || '0' <= c && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'z' || c >= 0x80
	}
	stop := func(c byte) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '.' || strings.IndexByte("=:,{}[]\"'#;", c) >= 0
	}
	type cand struct {
		at   int
		name string
	}
	var cands []cand
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '"' || c == '\'' {
			if text, _, _ := (&cfgReader{b: b, i: i}).word(false); containsFold(names, text) {
				cands = append(cands, cand{i, text})
			}
			continue
		}
		if stop(c) {
			continue
		}
		if i == 0 || stop(b[i-1]) {
			text, _, _ := (&cfgReader{b: b, i: i}).word(false)
			if k := slices.IndexFunc(splitKey(text), func(k string) bool { return containsFold(names, k) }); k >= 0 {
				cands = append(cands, cand{i, splitKey(text)[k]})
			}
		}
		if keyChar(c) && (i == 0 || !keyChar(b[i-1])) {
			j := i
			for j < len(b) && keyChar(b[j]) {
				j++
			}
			if n := string(b[i:j]); containsFold(names, n) {
				cands = append(cands, cand{i, n})
				if i > 0 && (b[i-1] == '"' || b[i-1] == '\'') {
					cands = append(cands, cand{i - 1, n})
				}
			}
		}
	}
	var out []cfgValue
	seen := map[int]bool{}
	for _, cd := range cands {
		at := cd.at
		bol := bytes.LastIndexByte(b[:at], '\n') + 1
		first := bol
		for first < at && (b[first] == ' ' || b[first] == '\t') {
			first++
		}
		if c := b[first]; c == '#' || c == ';' || c == '/' && first+1 < len(b) && b[first+1] == '/' {
			continue // a whole comment line
		}
		eol := bytes.IndexByte(b[at:], '\n')
		if eol < 0 {
			eol = len(b)
		} else {
			eol += at
		}
		start, header := at, b[first] == '[' && tomlHeader.Match(b[first:eol])
		if header {
			start = first
		}
		if seen[start] {
			continue
		}
		seen[start] = true
		// Whatever is read from a declared name's position is held under
		// that name, whatever keys the reader gave it (kit14rv, blunt: a
		// header or dotted key the reader parses differently from the
		// format no longer loses the association).
		for _, v := range readAssignment(b, bol, start, header) {
			if !slices.ContainsFunc(v.keys, func(k string) bool { return strings.EqualFold(k, cd.name) }) {
				v.keys = append([]string{cd.name}, v.keys...)
			}
			out = append(out, v)
		}
	}
	return out
}

// readAssignment reads b from start, a key or a table header on the line at
// bol, as if nothing came before it: the key and its value (a whole
// container, or a YAML parent's block), or the header's table to the next
// header. Text it cannot classify is not kept.
func readAssignment(b []byte, bol, start int, header bool) []cfgValue {
	r := &cfgReader{b: b, i: start, bol: bol, lineStart: true, anchors: map[string][]string{}}
	r.lineHead(b[start])
	if header {
		r.header()
	} else {
		r.item()
	}
	r.lineStart = false
	r.run(func() bool {
		if header {
			return r.lineStart && len(r.stack) == 0 && r.b[r.i] == '[' && tomlHeader.Match(r.line())
		}
		return len(r.stack) == 0 && len(r.parents) == 0
	})
	var out []cfgValue
	for _, v := range r.out {
		if !v.stray {
			out = append(out, v)
		}
	}
	return out
}

// holdConfigLocked holds every credential of one credential file and returns
// the base URL it names, if any.
func holdConfigLocked(b []byte) (baseURL string) {
	d := &reg.decl
	for _, v := range credentialAssignmentsLocked(b) {
		if v.value == "" {
			continue
		}
		if src := configSource(v.keys, d); src != "" {
			holdLocked(src, v.value, v.raw)
			if strings.HasPrefix(src, "header:") {
				reg.headers[strings.TrimPrefix(src, "header:")] = true
			}
		}
		if hold, _ := baseURLKey(d, v.keys); hold {
			holdBaseURLLocked(v.value)
		}
	}
	if !preciseConfig(b) {
		holdEveryStringLocked(b) // loop ruling 9
	}
	for _, v := range readConfig(b) {
		if v.value == "" {
			continue
		}
		if v.stray {
			holdLocked("config:unclassified", v.value)
		}
		if src := configSource(v.keys, d); src != "" {
			holdLocked(src, v.value, v.raw)
			if strings.HasPrefix(src, "header:") {
				reg.headers[strings.TrimPrefix(src, "header:")] = true
			}
		}
		if hold, use := baseURLKey(d, v.keys); hold {
			holdBaseURLLocked(v.value)
			if use && baseURL == "" {
				baseURL = v.value
			}
		}
	}
	return baseURL
}

// userinfoRE reads up to the last '@', not the first '/', '?' or '#': a password
// written raw may hold any of them (proxmox's tests name such passwords), and
// url.Parse's own error prints the URL. A blunt reading: a port and path before an
// '@' later in the URL are held too, which only a URL carrying an '@' can cost.
var userinfoRE = regexp.MustCompile(`(?s)^[A-Za-z][A-Za-z0-9+.-]*://(.*)@`)

// holdBaseURLLocked holds the password and every query value inside a base
// URL (contract 1).
func holdBaseURLLocked(raw string) {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil {
		if u.User != nil {
			if p, ok := u.User.Password(); ok {
				holdLocked("userinfo", p)
			}
		}
		for k, vs := range formPairs(u.RawQuery) {
			holdLocked("query:"+k, vs...)
		}
	} else if _, q, ok := strings.Cut(raw, "?"); ok {
		// A URL that does not parse still gives up its query (kit14rv).
		q, _, _ = strings.Cut(q, "#")
		for k, vs := range formPairs(q) {
			holdLocked("query:"+k, vs...)
		}
	}
	// A URL that does not parse still gives up its password.
	if m := userinfoRE.FindStringSubmatch(raw); m != nil {
		if _, p, ok := strings.Cut(m[1], ":"); ok {
			holdLocked("userinfo", p)
			if u, err := url.PathUnescape(p); err == nil {
				holdLocked("userinfo", u)
			}
		}
	}
}

// gatherFile holds a credential file's values. A file that does not exist
// holds nothing; one that exists but cannot be read, or is not a regular
// file (readRegular), makes every value unknown, so every unit is withheld
// (fail closed).
func gatherFile(path string) string {
	b, err := readRegular(path)
	if err != nil {
		if _, serr := statPath(path); os.IsNotExist(err) || os.IsNotExist(serr) {
			return ""
		}
		markUnknown()
		return ""
	}
	reg.Lock()
	defer reg.Unlock()
	// A byte-order mark says how the file is written (acquire.go).
	if t, _ := asUTF8(b, ""); !bytes.Equal(t, b) {
		holdConfigLocked(b)
		b = t
	}
	return holdConfigLocked(b)
}

// commentCut is v before its first comment: a '#' after a space or a tab
// (YAML, TOML and INI read either as the whitespace before a comment).
func commentCut(v string) (string, bool) {
	for k := 1; k < len(v); k++ {
		if v[k] == '#' && (v[k-1] == ' ' || v[k-1] == '\t') {
			return strings.TrimSpace(v[:k]), true
		}
	}
	return "", false
}

// Loop ruling 9 (blunt): a credential file counts as precisely readable
// only in a canonical subset of the formats the kit reads, the one every
// reader of those formats reads the same way. Any other construct anywhere
// in the file holds every string value in it (holdEveryStringLocked), on
// top of the declared-name rule (credentialAssignmentsLocked).
var (
	cfgScalar = `(?:"[^"\\]*"|'[^']*'|[+-]?[0-9][0-9_]*(?:\.[0-9_]+)?(?:[eE][+-]?[0-9]+)?|true|false|` +
		`[0-9]{4}-[0-9]{2}-[0-9]{2}(?:[Tt][0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:[Zz]|[+-][0-9]{2}:[0-9]{2})?)?)`
	cfgUnquoted = "[^\\s\"'\\\\#;$`\\[{][^\\s\"'\\\\#;$`]*"
	// bare-key = scalar, or dotenv's KEY=value (no export: an INI reader
	// reads "export KEY" as the key, kit15rv round 2; an
	// unquoted value with no blank, quote, escape, comment or expansion).
	canonAssign = regexp.MustCompile(`^([A-Za-z0-9_-]+)[ \t]*=[ \t]*(?:` + cfgScalar + `(?:[ \t]+#.*)?|` + cfgUnquoted + `)?[ \t]*$`)
	canonHeader = regexp.MustCompile(`^\[([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*)\][ \t]*(?:#.*)?$`)
	canonBlank  = regexp.MustCompile(`^[ \t]*(?:[#;].*)?$`)
)

// preciseConfig reports whether b is in the canonical subset: every line
// blank, a comment, a [table] header of bare keys (dotted, no blanks), or bare-key = scalar
// (a basic string without escapes, a literal string, a number, a boolean,
// a date-time; or a dotenv/INI unquoted value), no key or table twice
// (compared without case); or a JSON object whose members are scalars,
// without escapes or duplicate names. Valid UTF-8 without control
// characters but tab and line ends.
func preciseConfig(b []byte) bool {
	// Only LF and CRLF end a line: a lone CR, a C1 control (NEL among them),
	// LS or PS is a line break to some standard reader, so a file holding one
	// is not read precisely.
	if !utf8.Valid(b) || bytes.ContainsFunc(b, func(r rune) bool {
		return r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r >= 0x7f && r <= 0x9f || r == 0x2028 || r == 0x2029
	}) || bytes.Count(b, []byte("\r")) != bytes.Count(b, []byte("\r\n")) {
		return false
	}
	if t := bytes.TrimSpace(b); len(t) > 0 && t[0] == '{' {
		return preciseJSON(t)
	}
	seen := map[string]bool{}
	table := ""
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case canonBlank.MatchString(line):
		case canonHeader.MatchString(line):
			table = strings.ToLower(canonHeader.FindStringSubmatch(line)[1])
			if seen["["+table] {
				return false
			}
			seen["["+table] = true
		case canonAssign.MatchString(line):
			k := table + "\x00" + strings.ToLower(canonAssign.FindStringSubmatch(line)[1])
			if seen[k] {
				return false
			}
			seen[k] = true
		default:
			return false
		}
	}
	return true
}

func preciseJSON(b []byte) bool {
	if bytes.ContainsRune(b, '\\') {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		k, err := d.Token()
		ks, ok := k.(string)
		if err != nil || !ok || seen[strings.ToLower(ks)] {
			return false
		}
		seen[strings.ToLower(ks)] = true
		v, err := d.Token()
		if _, delim := v.(json.Delim); err != nil || delim {
			return false
		}
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

// holdEveryStringLocked holds every string value of a file outside the
// canonical subset, read every way the kit can without trusting a parse:
// what the reader finds, every quoted span (as written and unescaped as
// JSON, TOML or YAML would), and on each line the text after its first
// separator, whole, without its quotes or comment, and cut into tokens.
// Numbers, booleans and text with no letter or digit are not held.
func holdEveryStringLocked(b []byte) {
	hold := func(v string) {
		v = strings.TrimSpace(v)
		if strings.IndexFunc(v, func(c rune) bool { return c > 0x7f || unicode.IsLetter(c) || unicode.IsDigit(c) }) < 0 || plainScalar(v) {
			return
		}
		holdLocked("config:imprecise", v)
	}
	for _, v := range readConfig(b) {
		hold(v.value)
		hold(v.raw)
	}
	s := string(b)
	for i := 0; i < len(s); i++ {
		q := s[i]
		if q != '"' && q != '\'' {
			continue
		}
		j := i + 1
		for j < len(s) && s[j] != q {
			if q == '"' && s[j] == '\\' {
				j++
			}
			j++
		}
		if j > len(s) {
			j = len(s)
		}
		span := s[i+1 : j]
		hold(span)
		if q == '"' {
			hold(unescapeAny(span))
		} else {
			hold(strings.ReplaceAll(span, "''", "'"))
		}
		i = j
	}
	for _, line := range strings.Split(s, "\n") {
		k := strings.IndexAny(line, "=:")
		if k < 0 {
			continue
		}
		rest := strings.TrimSpace(line[k+1:])
		hold(rest)
		hold(strings.Trim(rest, "\"',"))
		if c, ok := commentCut(rest); ok {
			hold(c)
			hold(strings.Trim(c, "\"',"))
		}
		for _, f := range strings.FieldsFunc(rest, func(r rune) bool {
			return unicode.IsSpace(r) || strings.ContainsRune(",[]{}\"'", r)
		}) {
			hold(f)
		}
	}
}

func plainScalar(v string) bool {
	switch strings.ToLower(v) {
	case "true", "false", "null", "~", "yes", "no", "on", "off":
		return true
	}
	_, err := strconv.ParseFloat(strings.ReplaceAll(v, "_", ""), 64)
	return err == nil
}

// errNoFileName: a path whose final element names no file ("", "." or
// "..", or a trailing separator).
var errNoFileName = errors.New("credkit: the path does not end in a file name")

// errTooManyLinks: more symbolic links than the OS would follow (ELOOP).
var errTooManyLinks = errors.New("credkit: too many levels of symbolic links")

// maxLinks bounds the symbolic links one path follows, as the OS does.
const maxLinks = 40

// inDir runs op on name's final element through an os.Root opened on name's
// own directory part, uncleaned, so the OS resolves it exactly as it does for
// os.OpenFile(name): the same file, never a lexically cleaned one (with b a
// symbolic link, "a/b/../c" is the link target's sibling, not a/c). A final
// element that is a symbolic link is followed here, one Root per link, unless
// nofollow; op then sees the file the path names, as the OS would open it.
func inDir(name string, nofollow bool, op func(r *os.Root, base string) error) error {
	if name == "" {
		return &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist} // as os.Open("") says
	}
	for links := 0; ; links++ {
		vol := filepath.VolumeName(name)
		i := len(name) - 1
		for i >= len(vol) && !os.IsPathSeparator(name[i]) {
			i--
		}
		prefix, base := name[:i+1], name[i+1:]
		dir := prefix
		if i < len(vol) {
			dir = vol + "."
		}
		r, err := os.OpenRoot(dir)
		if err != nil {
			return err // a missing directory part: not found, as os.Open says
		}
		if base == "" || base == "." || base == ".." {
			_ = r.Close()
			return &os.PathError{Op: "open", Path: name, Err: errNoFileName}
		}
		if fi, lerr := r.Lstat(base); lerr == nil && fi.Mode()&os.ModeSymlink != 0 && !nofollow {
			t, err := r.Readlink(base)
			_ = r.Close()
			if err != nil {
				return err
			}
			if links >= maxLinks {
				return &os.PathError{Op: "open", Path: name, Err: errTooManyLinks}
			}
			if t == "" || filepath.IsAbs(t) || filepath.VolumeName(t) != "" || os.IsPathSeparator(t[0]) {
				name = t
			} else {
				name = prefix + t
			}
			continue
		}
		err = op(r, base)
		_ = r.Close()
		return err
	}
}

// openPath is os.OpenFile(name, flag, perm) through inDir. A final symbolic
// link is not followed for O_NOFOLLOW, nor for O_CREATE|O_EXCL, which the OS
// refuses on any existing name, a link included.
func openPath(name string, flag int, perm os.FileMode) (*os.File, error) {
	var f *os.File
	excl := flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL
	err := inDir(name, excl || openNofollow != 0 && flag&openNofollow != 0, func(r *os.Root, base string) error {
		var err error
		f, err = r.OpenFile(base, flag, perm)
		return err
	})
	return f, err
}

// statPath is os.Stat(name) through inDir.
func statPath(name string) (os.FileInfo, error) {
	var fi os.FileInfo
	err := inDir(name, false, func(r *os.Root, base string) error {
		var err error
		fi, err = r.Stat(base)
		return err
	})
	return fi, err
}

// errNotRegular: a credential file that is a pipe, a device or a directory.
// The kit never reads one: the read could block before main runs, or take
// input the program itself is waiting for.
var errNotRegular = errors.New("credkit: not a regular file")

// readRegular reads path when it is a regular file, and only then. The open
// does not block (openNonblock), and the opened file must be the one Stat
// saw, so a path swapped for a pipe after the Stat is refused too.
func readRegular(path string) ([]byte, error) {
	fi, err := statPath(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, &os.PathError{Op: "read", Path: path, Err: errNotRegular}
	}
	f, err := openPath(path, os.O_RDONLY|openNonblock, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	got, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !got.Mode().IsRegular() || !os.SameFile(fi, got) {
		return nil, &os.PathError{Op: "read", Path: path, Err: errNotRegular}
	}
	return io.ReadAll(f)
}
