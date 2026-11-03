// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 1 for every acquisition path: a credential arrives in bytes
// whose headers, or byte-order mark, say how to read them. Before the kit
// holds the fields of a request or response body, an MCP request, a header,
// a trailer or a credential file, it reads them as declared, with the
// matcher's own decoders: content and transfer codings undone (gzip,
// deflate, base64, quoted-printable), the charset or byte-order mark read
// as UTF-8, each multipart part read by its own headers, and MIME encoded
// words in a header value. A declared coding or charset the kit cannot
// read fails closed.

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/transform"
)

// readings is b as written and as h declares it: its codings undone, then
// its byte-order mark or charset read as UTF-8, then, for a multipart body,
// each part's readings (and a form field "name=value" for each named one).
// ok is false when a declared coding, transfer encoding or charset is one
// the kit cannot read, or a decoding outgrows maxDeclared.
func readings(b []byte, h http.Header) (out [][]byte, ok bool) {
	out, ok = [][]byte{b}, true
	if len(b) == 0 {
		return out, true
	}
	d, steps, good, exact := undoCodings(b, h)
	out = append(out, steps...)
	if !good {
		return out, false
	}
	if !exact && len(steps) > 1 {
		// With more than one coding, a step that is not its format or
		// stops part way means the order or the bytes are not what the
		// headers say (kit14rv): not whole.
		ok = false
	}
	// A Content-Type the kit cannot parse leaves the body's reading (a
	// charset, multipart parts) unknown: not whole (kit14rv, fail closed).
	cts := headerValues(h, "Content-Type")
	ct := ""
	if len(cts) > 0 {
		ct = cts[0]
	}
	mt, params, err := mime.ParseMediaType(ct)
	if ct != "" && err != nil || len(cts) > 1 || ambiguousBodyHeaders(h) {
		ok = false // which one the reader obeys is unknown
	}
	if t, good := asUTF8(d, params["charset"]); !good {
		ok = false
	} else if !bytes.Equal(t, d) {
		out = append(out, t)
		d = t
	}
	if strings.HasPrefix(mt, "multipart/") && params["boundary"] != "" {
		parts, good := multipartReadings(d, params["boundary"])
		out, ok = append(out, parts...), ok && good
	}
	return out, ok
}

// undoCodings is b with h's codings undone, last applied first: d is the
// result, steps each step's output. ok is false for a coding the kit cannot
// undo or a decoding past maxDeclared; exact is false when a step was not
// its format or stopped part way.
func undoCodings(b []byte, h http.Header) (d []byte, steps [][]byte, ok, exact bool) {
	d, ok, exact = b, true, true
	codings := declaredCodings(h)
	for i := len(codings) - 1; i >= 0; i-- {
		var good, ex bool
		if d, good, ex = uncode(d, codings[i]); !good {
			return d, steps, false, false
		}
		exact = exact && ex
		steps = append(steps, d)
	}
	return d, steps, true, exact
}

// declaredCodings is h's transfer codings (but chunked) and content
// codings, in the order they were applied.
func declaredCodings(h http.Header) []string {
	var out []string
	// In the order they are applied: a body is content-coded first and
	// transfer-coded last (kit14rv: Transfer-Encoding came first, so it
	// was undone last), and a MIME part's transfer coding wraps it all.
	for _, k := range []string{"Content-Encoding", "Transfer-Encoding", "Content-Transfer-Encoding"} {
		for _, v := range headerValues(h, k) {
			for _, c := range strings.Split(v, ",") {
				switch c = strings.ToLower(strings.TrimSpace(c)); c {
				case "", "identity", "chunked", "7bit", "8bit", "binary":
				default:
					out = append(out, c)
				}
			}
		}
	}
	return out
}

// headerValues is every value of the header name in h, under any spelling
// of the name (kit14rv: a map entry set directly, "content-type", goes on
// the wire as it is, and h.Get missed it).
func headerValues(h http.Header, name string) []string {
	var out []string
	for k, vs := range h {
		if strings.EqualFold(k, name) {
			out = append(out, vs...)
		}
	}
	return out
}

// headerNames is every spelling of the header name in h.
func headerNames(h http.Header, name string) []string {
	var out []string
	for k := range h {
		if strings.EqualFold(k, name) {
			out = append(out, k)
		}
	}
	return out
}

// bodyHeaders are the headers that decide how a body is read.
var bodyHeaders = []string{"Content-Type", "Content-Disposition", "Transfer-Encoding", "Content-Encoding", "Content-Transfer-Encoding"}

// ambiguousBodyHeaders reports whether a header that decides how a body is
// read appears under more than one spelling of its name, or Content-Type
// or Content-Disposition carries more than one value (an empty one included): the order or the
// value a reader obeys is unknown (kit14rv, blunt: such a body is not read
// whole). Ordinary headers have one canonical key each.
func ambiguousBodyHeaders(h http.Header) bool {
	for _, name := range bodyHeaders {
		n := 0
		for k, vs := range h {
			if strings.EqualFold(k, name) {
				n++
				if (name == "Content-Type" || name == "Content-Disposition") && len(vs) > 1 {
					return true
				}
			}
		}
		if n > 1 {
			return true
		}
	}
	return false
}

// formPairs is every name=value pair of a query or form body, none dropped:
// split at '&', and again at '&' and ';', each name and value as written
// and unescaped (kit14rv: url.ParseQuery drops a pair with a ';' or a bad
// escape, and a credential went unheld).
func formPairs(s string) map[string][]string {
	out := map[string][]string{}
	add := func(k, v string) {
		out[k] = append(out[k], v)
	}
	for _, pieces := range [][]string{strings.Split(s, "&"), strings.FieldsFunc(s, func(r rune) bool { return r == '&' || r == ';' })} {
		for _, p := range pieces {
			k, v, _ := strings.Cut(p, "=")
			add(k, v)
			uk, kerr := url.QueryUnescape(k)
			uv, verr := url.QueryUnescape(v)
			if kerr != nil {
				uk = k
			}
			if verr != nil {
				uv = v
			}
			if uk != k || uv != v {
				add(uk, uv)
				add(uk, v)
				add(k, uv)
			}
		}
	}
	return out
}

// uncode undoes one coding. A stream that stops part way gives what it
// decoded; a coding the kit does not know fails.
func uncode(b []byte, coding string) (out []byte, ok, exact bool) {
	var r io.Reader
	switch coding {
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return b, true, false // labelled but not gzip: the bytes as written are read
		}
		r = zr
	case "deflate":
		// zlib by the standard, raw deflate from some writers.
		if len(b) >= 2 && b[0]&0x0f == 8 && (uint16(b[0])<<8|uint16(b[1]))%31 == 0 {
			zr, err := zlib.NewReader(bytes.NewReader(b))
			if err != nil {
				return b, true, false
			}
			r = zr
		} else {
			r = flate.NewReader(bytes.NewReader(b))
		}
	case "base64":
		return lenientBase64(bytes.Map(func(r rune) rune {
			if isSpaceByte(byte(r&0xff)) && r < 0x80 {
				return -1
			}
			return r
		}, b)), true, true
	case "quoted-printable":
		return qpDecode(b, false), true, true
	default:
		return nil, false, false
	}
	out, err := io.ReadAll(io.LimitReader(r, int64(maxDeclared)+1))
	return out, len(out) <= maxDeclared, err == nil
}

// asUTF8 reads b as UTF-8: by its byte-order mark (UTF-8, UTF-16 or UTF-32,
// which outranks any label), else by charset when one is declared. ok is
// false for a charset the kit cannot read.
func asUTF8(b []byte, charset string) ([]byte, bool) {
	switch {
	case bytes.HasPrefix(b, []byte("\xef\xbb\xbf")):
		return b[3:], true
	case bytes.HasPrefix(b, []byte("\xff\xfe\x00\x00")):
		return decodeWith(utf32.UTF32(utf32.LittleEndian, utf32.UseBOM), b), true
	case bytes.HasPrefix(b, []byte("\x00\x00\xfe\xff")):
		return decodeWith(utf32.UTF32(utf32.BigEndian, utf32.UseBOM), b), true
	case bytes.HasPrefix(b, []byte("\xff\xfe")):
		return decodeWith(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), b), true
	case bytes.HasPrefix(b, []byte("\xfe\xff")):
		return decodeWith(unicode.UTF16(unicode.BigEndian, unicode.UseBOM), b), true
	}
	if charset == "" {
		return b, true
	}
	enc := charsetByName(charset)
	if enc == nil {
		return b, false
	}
	return decodeWith(enc, b), true
}

func decodeWith(e encoding.Encoding, b []byte) []byte {
	out, _, _ := transform.Bytes(e.NewDecoder(), b)
	return out
}

// multipartReadings is each part's readings, by the part's own headers,
// and "name=value" form fields for the named ones (each reading of the
// value).
func multipartReadings(b []byte, boundary string) (out [][]byte, ok bool) {
	ok = true
	mr := multipart.NewReader(bytes.NewReader(b), boundary)
	for {
		p, err := mr.NextRawPart()
		if err != nil {
			// Only the final boundary ends a body read whole; any other
			// error (a part header past the reader's limit, a body that
			// stops inside a part) leaves bytes unread (kit14rv: fail
			// closed).
			return out, ok && err == io.EOF
		}
		// A part header the kit cannot parse hides the part's form name
		// (Part.FormName answers "" on a parse error): not whole (kit14rv).
		if ambiguousBodyHeaders(http.Header(p.Header)) {
			ok = false
		}
		for _, k := range []string{"Content-Disposition", "Content-Type"} {
			if v := p.Header.Get(k); v != "" {
				if _, _, err := mime.ParseMediaType(v); err != nil {
					ok = false
				}
			}
		}
		body, rerr := io.ReadAll(io.LimitReader(p, int64(maxDeclared)+1))
		rs, good := readings(body, http.Header(textproto.MIMEHeader(p.Header)))
		ok = ok && good && rerr == nil && len(body) <= maxDeclared
		out = append(out, rs...)
		if name := p.FormName(); name != "" {
			for _, r := range rs {
				out = append(out, []byte(url.Values{name: {string(r)}}.Encode()))
			}
		}
	}
}

// headerReadings is a header value as written and with its MIME encoded
// words decoded.
func headerReadings(v string) []string {
	if d, ok := decodeMIMEWords([]byte(v), true); ok {
		return []string{v, string(d)}
	}
	return []string{v}
}

// requestWhole reports whether the kit reads a request (an outgoing one, or
// an inbound MCP request with rawQuery "") the one unambiguous way (loop
// ruling 8, "send what you read", blunt; it ends the parser-differential
// classes of kit13rv and kit14rv by construction): every header name in
// canonical form, the body read whole (bodyWhole), and a query of canonical
// pairs (canonicalQuery). A request that is not whole is refused and
// acquisition is marked unknown.
func requestWhole(rawQuery string, body []byte, hs ...http.Header) bool {
	for _, h := range hs {
		for k := range h {
			if textproto.CanonicalMIMEHeaderKey(k) != k {
				return false
			}
		}
	}
	return canonicalQuery(rawQuery) && bodyWhole(body, hs[0])
}

// bodyWhole reports whether a request body (or one of its parts) is one the
// kit reads exactly as sent: each header that decides its reading has one
// value, it carries no content or transfer coding (connectors never
// compress what they send), no byte-order mark and no charset but UTF-8 or
// US-ASCII, and a form or multipart body re-serializes byte for byte from
// what the kit parsed, with the writer the fleet uses for it. Any other
// body (JSON, text, bytes) is read as it is.
func bodyWhole(b []byte, h http.Header) bool {
	for _, name := range bodyHeaders {
		if len(headerValues(h, name)) > 1 {
			return false
		}
	}
	if len(declaredCodings(h)) > 0 || bytes.HasPrefix(b, []byte("\xef\xbb\xbf")) || bytes.HasPrefix(b, []byte("\xff\xfe")) ||
		bytes.HasPrefix(b, []byte("\xfe\xff")) || bytes.HasPrefix(b, []byte("\x00\x00\xfe\xff")) {
		return false
	}
	ct := strings.Join(headerValues(h, "Content-Type"), "")
	if ct == "" {
		return true
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	if cs := strings.ToLower(params["charset"]); cs != "" && cs != "utf-8" && cs != "us-ascii" {
		return false
	}
	switch {
	case mt == "application/x-www-form-urlencoded":
		return canonicalForm(string(b))
	case strings.HasPrefix(mt, "multipart/"):
		return canonicalMultipart(b, params["boundary"])
	}
	return true
}

// canonicalForm reports whether a form body is url.Values-style pairs in
// the order written: each pair name=value, both QueryEscape of their
// decoding, joined by '&' alone.
func canonicalForm(s string) bool {
	if s == "" {
		return true
	}
	for _, p := range strings.Split(s, "&") {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return false
		}
		uk, kerr := url.QueryUnescape(k)
		uv, verr := url.QueryUnescape(v)
		if kerr != nil || verr != nil || url.QueryEscape(uk) != k || url.QueryEscape(uv) != v {
			return false
		}
	}
	return true
}

// canonicalQuery reports whether a URL query is pairs split on '&' alone
// (no ';'), every escape valid, and each name and value the re-escape of
// its decoding: QueryEscape, with a space as "+" or "%20" (cliutil's
// EncodeQuery), and either escaping or leaving as written the characters
// the fleet's hand-built queries use (, : / @). A pair with no '=' (a
// key-only flag, "?upload") is a name alone.
func canonicalQuery(q string) bool {
	if strings.Contains(q, ";") {
		return false
	}
	for _, p := range strings.Split(q, "&") {
		k, v, _ := strings.Cut(p, "=")
		for _, x := range []string{k, v} {
			u, err := url.QueryUnescape(x)
			if err != nil {
				return false
			}
			e := url.QueryEscape(u)
			kept := strings.NewReplacer("%2C", ",", "%3A", ":", "%2F", "/", "%40", "@").Replace(e)
			if x != e && x != kept && x != strings.ReplaceAll(e, "+", "%20") && x != strings.ReplaceAll(kept, "+", "%20") {
				return false
			}
		}
	}
	return true
}

// canonicalMultipart reports whether a multipart body is exactly what
// mime/multipart.Writer writes for the parts the kit read, with the given
// boundary: no preamble or epilogue, part headers canonical and sorted, one
// space after each colon, and each part's own body whole.
func canonicalMultipart(b []byte, boundary string) bool {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if w.SetBoundary(boundary) != nil {
		return false
	}
	mr := multipart.NewReader(bytes.NewReader(b), boundary)
	for {
		p, err := mr.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		body, err := io.ReadAll(io.LimitReader(p, int64(maxDeclared)+1))
		if err != nil || len(body) > maxDeclared || !bodyWhole(body, http.Header(p.Header)) {
			return false
		}
		pw, err := w.CreatePart(p.Header)
		if err != nil {
			return false
		}
		_, _ = pw.Write(body)
	}
	return w.Close() == nil && bytes.Equal(buf.Bytes(), b)
}
