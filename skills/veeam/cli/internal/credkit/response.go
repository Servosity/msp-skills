// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 1 for a response: every Set-Cookie value and every declared
// response field is held before the program can read it. With fields
// declared, a body is read whole and as its headers declare it first; a
// coding the kit cannot read, or a body too large to read, makes every
// value unknown (fail closed). Trailers are held when the body ends.

import (
	"bufio"
	"bytes"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// holdResponse holds every cookie value in the headers and trailers, bound
// to the origin that set it as well as its declared origins, and every
// declared response field in the headers, the body (as written and as the
// headers declare it, acquire.go) and the trailers, before the program
// sees them.
func holdResponse(resp *http.Response, o origin) {
	setter := []string{o.scheme + "://" + o.host + ":" + o.port}
	if o.port == "" {
		setter = nil
	}
	reg.Lock()
	holdCookiesLocked(resp.Header, setter)
	fields := reg.decl.ResponseFields
	reg.Unlock()
	if resp.Body == nil || resp.Body == http.NoBody {
		// No body to read to its end: the trailers are held now.
		holdResponseHeader(resp.Header, fields)
		(&trailerReader{resp: resp, fields: fields, setter: setter}).hold()
		return
	}
	var body io.Reader = resp.Body
	t := &trailerReader{resp: resp, fields: fields, setter: setter}
	if len(fields) > 0 {
		holdResponseHeader(resp.Header, fields)
		body = fieldBody(resp, fields)
		// The body was read to its end here, so the trailers are already
		// there to read: held now, and again at the end the program reads
		// (kit15rv round 5, response-trailer-acquisition-late).
		t.holdTrailers()
	}
	t.r = body
	resp.Body = readCloser{t, resp.Body}
}

// fieldBody is resp's body with its declared fields held before the
// program reads any of it: read whole and as its headers declare it first.
func fieldBody(resp *http.Response, fields []string) io.Reader {
	h := resp.Header
	if resp.Uncompressed {
		h = h.Clone()
		h.Del("Content-Encoding")
	}
	// Blunt (kit15rv, response-field-context-loss, a class of several
	// rounds): every body is read whole, as its headers declare it, and
	// its fields held before the program reads any of it, since a field's
	// value may stand earlier in the body than the field (an echo). A body
	// past maxEncodedBody, or one the kit cannot read whole, makes every
	// value unknown (fail closed). Responses that carry declared fields
	// (token and session responses) are small documents.
	cts := headerValues(h, "Content-Type")
	ct := ""
	if len(cts) > 0 {
		ct = cts[0]
	}
	mt, _, _ := mime.ParseMediaType(ct)
	br := bufio.NewReader(resp.Body)
	b, err := io.ReadAll(io.LimitReader(br, int64(maxEncodedBody)+1))
	rs, ok := readings(b, h)
	n := len(declaredCodings(h))
	for i, r := range rs {
		// The bytes as written and each partly undone step are coded, not
		// the declared media type: they are read leniently (kit15rv: read
		// as JSON, a gzip body's own bytes were a grammar error that made
		// every ordinary coded response unknown).
		mode := modeFor(mt)
		if i < n {
			mode = lenient
		}
		scanAll(bytes.NewReader(r), fields, mode)
	}
	if len(b) > maxEncodedBody || !ok {
		markUnknown()
		return io.MultiReader(bytes.NewReader(b), errReader{err}, br)
	}
	// Deliver what you read (loop ruling 8d): the program reads the body
	// with its codings undone as the kit undid them, and no coding header
	// or length is left to read it again, or another way.
	if len(declaredCodings(h)) > 0 {
		d, _, _, exact := undoCodings(b, h)
		for _, k := range []string{"Content-Encoding", "Transfer-Encoding", "Content-Transfer-Encoding", "Content-Length"} {
			for _, name := range headerNames(resp.Header, k) {
				delete(resp.Header, name)
			}
		}
		resp.ContentLength, resp.Uncompressed = -1, true
		if err == nil && !exact {
			err = io.ErrUnexpectedEOF // a coding that stopped part way, or was not its format
		}
		return io.MultiReader(bytes.NewReader(d), errReader{err}, br)
	}
	return io.MultiReader(bytes.NewReader(b), errReader{err}, br)
}

// holdResponseHeader holds the declared fields among response headers or
// trailers: a header named for one, as written and with MIME words decoded,
// and one in a header's URL (a redirect's query or fragment).
func holdResponseHeader(h http.Header, fields []string) {
	if len(fields) == 0 {
		return
	}
	for name, vs := range h {
		if containsFold(fields, strings.ToLower(name)) {
			for _, v := range vs {
				Hold("response:"+strings.ToLower(name), headerReadings(v)...)
			}
		}
		for _, v := range vs {
			for _, r := range headerReadings(v) {
				scanAll(strings.NewReader(strings.NewReplacer("?", "&", "#", "&").Replace(r)), fields, lenient)
			}
		}
	}
}

// trailerReader holds the response's trailers (its cookies and declared
// fields) when its body ends, before the program sees the end (and so the
// trailers).
type trailerReader struct {
	r      io.Reader
	resp   *http.Response
	fields []string
	setter []string
	done   bool
}

func (t *trailerReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if err == io.EOF && !t.done {
		t.hold()
	}
	return n, err
}

// hold holds the trailers' cookies and declared fields.
func (t *trailerReader) hold() {
	t.done = true
	t.holdTrailers()
}

func (t *trailerReader) holdTrailers() {
	reg.Lock()
	holdCookiesLocked(t.resp.Trailer, t.setter)
	reg.Unlock()
	holdResponseHeader(t.resp.Trailer, t.fields)
}

type readCloser struct {
	io.Reader
	io.Closer
}

// errReader replays a read error after the buffered bytes.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	return 0, io.EOF
}

// scanAll holds the declared fields of everything r yields and returns its
// length.
func scanAll(r io.Reader, fields []string, mode scanMode) int64 {
	s := newFieldScan(fields, mode)
	buf := make([]byte, 32<<10)
	var n int64
	for {
		k, err := r.Read(buf)
		s.feed(buf[:k])
		n += int64(k)
		if err != nil {
			s.finish()
			return n
		}
	}
}

// scanMode is how a fieldScan reads a body: as JSON (declared by its
// Content-Type), as JSON when it opens with { or [ (no Content-Type), or
// leniently (form encoding and "key: value" lines, and JSON-like text).
type scanMode int

const (
	sniffJSON scanMode = iota
	asJSON
	lenient
)

// modeFor is the scan mode a Content-Type media type declares.
func modeFor(mt string) scanMode {
	switch {
	case mt == "":
		return sniffJSON
	case mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "/x-ndjson") || mt == "text/json":
		return asJSON
	}
	return lenient
}

// JSON grammar positions (fieldScan.exp).
const (
	expValue = iota // a value (a top-level value, an array element, after ':')
	expKey          // a key, or the object's end
	expColon        // the ':' after a key
	expNext         // ',' or the container's end
)

// fieldScan finds the declared fields of a body as it streams. A value
// under a declared key (a scalar, or every scalar inside an object or
// array) is a field value. JSON is read by its grammar: whitespace between
// any two tokens, keys of any length compared whole (unescaped), single
// quotes allowed; a body that breaks the grammar makes every value unknown
// (fail closed) and is read leniently from there. Other bodies are read
// leniently: form encoding, "key: value" lines, and JSON-like text in which
// a line break inside a container is whitespace.
type fieldScan struct {
	names    []string
	mode     scanMode
	json     bool   // reading by the JSON grammar
	started  bool   // a byte other than whitespace or a byte-order mark was read
	exp      int    // the JSON grammar position
	objs     []bool // the open containers, true for an object
	match    int    // len(objs) outside the container a declared key opened; -1 when none
	matchKey string
	keyNext  bool   // the next token is a key
	afterKey bool   // a key was just read
	sep      bool   // a key and its ':' or '=' were just read
	key      string // the last key read
	keyCap   int    // a key longer than this matches no declared name
	tok      byte   // the token being read: 0 none, a quote, or 'b' bare
	n        int    // its length
	isKey    bool
	keep     bool // it is a field value: its bytes wait in text
	esc      bool
	text     []byte
	out      []byte
	bom      int    // bytes of a leading UTF-8 byte-order mark matched; -1 once none can follow
	pend     []byte // those bytes, read as content if the mark does not complete
}

func newFieldScan(names []string, mode scanMode) *fieldScan {
	s := &fieldScan{names: names, mode: mode, match: -1, keyNext: true, keyCap: 64}
	for _, n := range names {
		// A name's every character escaped (\uXXXX, a surrogate pair of
		// them above U+FFFF), and its quotes.
		s.keyCap = max(s.keyCap, 12*utf8.RuneCountInString(n)+2)
	}
	return s
}

// feed scans b and returns the bytes now safe to release; the slice is
// reused by the next call.
func (s *fieldScan) feed(b []byte) []byte {
	s.out = s.out[:0]
	for _, c := range b {
		s.byte(c)
	}
	return s.out
}

// finish ends the body: a value still open is held and released.
func (s *fieldScan) finish() []byte {
	s.out = s.out[:0]
	s.unBOM()
	if s.tok != 0 {
		s.end()
	}
	return s.out
}

// unBOM reads the bytes of a byte-order mark that did not complete as
// content (kit15rv: a key starting with U+FB00 began with 0xEF, read as a
// mark byte, and lost it).
func (s *fieldScan) unBOM() {
	p := s.pend
	s.bom, s.pend = -1, nil
	for _, c := range p {
		s.byte(c)
	}
}

func (s *fieldScan) emit(c byte) {
	s.n++
	if s.keep {
		s.text = append(s.text, c)
		return
	}
	if s.isKey && len(s.text) <= s.keyCap {
		s.text = append(s.text, c)
	}
	s.out = append(s.out, c)
}

// fail ends JSON reading at a grammar error: every value is unknown.
func (s *fieldScan) fail() {
	s.json = false
	markUnknown()
}

func (s *fieldScan) byte(c byte) {
	if s.bom >= 0 {
		// Only EF BB BF as the body's first bytes is a byte-order mark.
		if c == "\xef\xbb\xbf"[s.bom] {
			s.pend = append(s.pend, c)
			if s.bom++; s.bom == 3 {
				s.out = append(s.out, s.pend...)
				s.bom, s.pend = -1, nil
			}
			return
		}
		s.unBOM()
	}
	if !s.started {
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			s.out = append(s.out, c) // whitespace
			return
		}
		s.started = true
		s.json = s.mode == asJSON || s.mode == sniffJSON && (c == '{' || c == '[')
	}
	switch s.tok {
	case '"', '\'':
		s.emit(c)
		switch {
		case s.esc:
			s.esc = false
		case c == '\\' && s.tok == '"':
			s.esc = true
		case c == s.tok && s.n > 1:
			s.end()
		}
		return
	case 'b':
		stop := "\r\n,&}]"
		if s.json {
			stop = " \t\r\n,:{}[]\"'"
		} else if s.isKey {
			stop = " \t\r\n:=,&{}[]\"'"
		}
		if strings.IndexByte(stop, c) < 0 {
			s.emit(c)
			return
		}
		s.end()
	}
	if s.json {
		if !s.jsonByte(c) {
			s.fail()
			s.lenientByte(c)
		}
		return
	}
	s.lenientByte(c)
}

// jsonByte reads one byte between tokens by the JSON grammar and reports
// whether the grammar allows it there.
func (s *fieldScan) jsonByte(c byte) bool {
	inObj := len(s.objs) > 0 && s.objs[len(s.objs)-1]
	switch c {
	case ' ', '\t', '\r', '\n':
	case ':':
		if !inObj || s.exp != expColon {
			return false
		}
		s.exp, s.sep, s.afterKey = expValue, true, false
	case ',':
		if len(s.objs) == 0 || s.exp != expNext {
			return false
		}
		s.exp, s.sep = expValue, false
		if inObj {
			s.exp = expKey
		}
	case '{', '[':
		if s.exp != expValue {
			return false
		}
		if s.sep && s.match < 0 && containsFold(s.names, s.key) {
			s.match, s.matchKey = len(s.objs), s.key
		}
		s.objs = append(s.objs, c == '{')
		s.exp, s.sep = expValue, false
		if c == '{' {
			s.exp = expKey
		}
	case '}', ']':
		closes := len(s.objs) > 0 && s.objs[len(s.objs)-1] == (c == '}')
		if !closes || c == '}' && s.exp != expKey && s.exp != expNext || c == ']' && s.exp != expValue && s.exp != expNext {
			return false
		}
		s.objs = s.objs[:len(s.objs)-1]
		if s.match >= 0 && len(s.objs) <= s.match {
			s.match = -1
		}
		s.exp, s.sep = s.next(), false
	default:
		if s.exp != expValue && s.exp != expKey {
			return false
		}
		s.keyNext = s.exp == expKey
		s.start(c)
		return true
	}
	s.out = append(s.out, c)
	return true
}

// next is the grammar position after a value: ',' or the end inside a
// container; at the top level another value (JSON lines).
func (s *fieldScan) next() int {
	if len(s.objs) == 0 {
		return expValue
	}
	return expNext
}

func (s *fieldScan) lenientByte(c byte) {
	switch c {
	case ' ', '\t', '\r':
	case ':', '=':
		if s.afterKey {
			s.sep, s.keyNext = true, false
		}
		s.afterKey = false
	case '{', '[':
		if s.sep && s.match < 0 && containsFold(s.names, s.key) {
			s.match, s.matchKey = len(s.objs), s.key
		}
		s.objs = append(s.objs, c == '{')
		s.keyNext, s.sep, s.afterKey = c == '{', false, false
	case '}', ']':
		if len(s.objs) > 0 {
			s.objs = s.objs[:len(s.objs)-1]
		}
		if s.match >= 0 && len(s.objs) <= s.match {
			s.match = -1
		}
		s.keyNext, s.sep, s.afterKey = false, false, false
	case ',':
		s.keyNext, s.sep, s.afterKey = len(s.objs) == 0 || s.objs[len(s.objs)-1], false, false
	case '\n':
		if len(s.objs) > 0 {
			break // whitespace inside a container
		}
		s.keyNext, s.sep, s.afterKey = true, false, false
	case '&':
		if len(s.objs) == 0 {
			s.keyNext, s.sep = true, false
		}
		s.afterKey = false
	default:
		s.start(c)
		return
	}
	s.out = append(s.out, c)
}

func (s *fieldScan) start(c byte) {
	s.isKey = s.keyNext
	s.keep = !s.isKey && ((s.sep && containsFold(s.names, s.key)) || (s.match >= 0 && len(s.objs) > s.match))
	s.tok, s.n, s.esc, s.text = 'b', 0, false, s.text[:0]
	if c == '"' || c == '\'' {
		s.tok = c
	}
	s.afterKey = false
	s.emit(c)
}

// end closes the token: a key is remembered (one longer than any declared
// name can be written matches none), a field value held and then released.
func (s *fieldScan) end() {
	tok, text := s.tok, s.text
	s.tok = 0
	if s.json {
		s.exp = s.next()
		if s.isKey {
			s.exp = expColon
		}
	}
	if !s.isKey && !s.keep {
		s.sep = false
		return
	}
	body := text
	if tok != 'b' {
		body = bytes.TrimSuffix(text[1:], []byte{tok})
	}
	value := strings.TrimSpace(string(body))
	switch {
	case tok == '"':
		value = string(unescape(body, true))
	case tok == 'b':
		if u, err := url.QueryUnescape(value); err == nil {
			value = u
		}
	}
	if s.isKey {
		if len(text) > s.keyCap {
			value = "" // longer than any declared name, written any way
		}
		s.key, s.afterKey, s.keyNext, s.isKey = value, true, false, false
		return
	}
	s.sep = false
	if s.keep && !(tok == 'b' && value == "null") { // a JSON null is no value
		name := s.key
		if s.match >= 0 {
			name = s.matchKey
		}
		Hold("response:"+name, string(body), strings.TrimSpace(string(body)), value)
		if c, ok := commentCut(value); ok && tok == 'b' && !s.json {
			Hold("response:"+name, c) // a lenient bare value with a trailing comment is held both ways
		}
	}
	if s.keep {
		s.out = append(s.out, text...)
		s.keep = false
	}
}
