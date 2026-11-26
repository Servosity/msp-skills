// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 4, MCP tool results (amended): a tool result the matcher would
// withhold is first redacted leaf by leaf. Each piece of its content that is
// one JSON document is rewritten, keeping member order, with leaves (a
// string, a number, a literal or an object key) replaced by Redacted:
//
//  1. every leaf that would be withheld on its own;
//  2. then, while the result as a whole would still be withheld (a match
//     the decoders assemble across neighbouring leaves, or one the result's
//     own envelope creates), the one leaf that completes the earliest match,
//     found by bisection with the whole-result check as the oracle, up to
//     maxResidualMasks leaves.
//
// The redacted result reaches the client only when the whole-result check,
// unchanged, finds it clean. Otherwise, or when a piece of the content that
// would be withheld is not one JSON document, the whole result is withheld
// as before. Redaction never releases a byte the whole-unit check refuses.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

// Redacted is the fixed text that replaces a leaf carrying a held value.
const Redacted = "***"

// maxResidualMasks bounds step 2: each mask costs about log2(leaves)+1
// whole-result checks.
const maxResidualMasks = 32

// jtok is one token of a JSON document as written, with the separator
// written before it.
type jtok struct {
	sep  byte   // ',' or ':' before the token, or 0
	b    []byte // the token, encoded
	raw  string // a string token, decoded
	kind byte   // 's' string, 'n' number, 'l' literal, 'd' delimiter
}

// jdoc is one JSON document as tokens, and the leaves step 1 and step 2
// mask.
type jdoc struct {
	toks   []jtok
	mask   []bool
	indent bool
}

func (d *jdoc) leaf(i int) bool { return d.toks[i].kind != 'd' }

// render writes the document with its masked leaves replaced.
func (d *jdoc) render() []byte {
	var buf bytes.Buffer
	red, _ := encodeString(Redacted)
	for i, t := range d.toks {
		if t.sep != 0 {
			buf.WriteByte(t.sep)
		}
		if d.mask[i] {
			buf.Write(red)
		} else {
			buf.Write(t.b)
		}
	}
	if d.indent {
		var ind bytes.Buffer
		if json.Indent(&ind, buf.Bytes(), "", "  ") == nil {
			return ind.Bytes()
		}
	}
	return buf.Bytes()
}

// maskWithheldLeaves is step 1.
func (d *jdoc) maskWithheldLeaves() {
	for i, t := range d.toks {
		switch t.kind {
		case 's':
			d.mask[i] = withheld([]byte(t.raw)) || withheld(t.b)
		case 'n', 'l':
			d.mask[i] = withheld(t.b)
		}
	}
}

// piece is one part of a tool result that redaction may rewrite.
type piece struct {
	doc *jdoc
	set func(out *mcp.CallToolResult, b []byte)
}

// redactToolResult returns a copy of res redacted as the contract says, and
// false when it cannot be made clean.
func redactToolResult(res *mcp.CallToolResult) (*mcp.CallToolResult, bool) {
	if res == nil {
		return nil, false
	}
	var pieces []piece
	for i, c := range res.Content {
		var text string
		switch tc := c.(type) {
		case mcp.TextContent:
			text = tc.Text
		case *mcp.TextContent:
			if tc == nil {
				continue
			}
			text = tc.Text
		default:
			// Images, audio, resources: not redacted. The final check on the
			// whole result withholds it if one of them carries a held value.
			continue
		}
		d, err := parseJSONDoc([]byte(text))
		if err != nil {
			if withheld([]byte(text)) {
				return nil, false
			}
			continue // clean on its own and not JSON: kept as it is
		}
		i := i
		pieces = append(pieces, piece{doc: d, set: func(out *mcp.CallToolResult, b []byte) {
			switch tc := out.Content[i].(type) {
			case mcp.TextContent:
				tc.Text = string(b)
				out.Content[i] = tc
			case *mcp.TextContent:
				cp := *tc
				cp.Text = string(b)
				out.Content[i] = &cp
			}
		}})
	}
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return nil, false
		}
		d, err := parseJSONDoc(b)
		if err != nil {
			return nil, false
		}
		pieces = append(pieces, piece{doc: d, set: func(out *mcp.CallToolResult, b []byte) {
			out.StructuredContent = json.RawMessage(b)
		}})
	}
	build := func() *mcp.CallToolResult {
		out := *res
		out.Content = append([]mcp.Content(nil), res.Content...)
		for _, p := range pieces {
			p.set(&out, p.doc.render())
		}
		return &out
	}
	clean := func() bool {
		b, err := json.Marshal(build())
		return err == nil && !withheld(b)
	}

	for _, p := range pieces {
		p.doc.maskWithheldLeaves()
	}
	if clean() {
		return build(), true
	}

	// Step 2. cands are the unmasked leaves, in order across the pieces.
	type ref struct{ p, i int }
	for n := 0; n < maxResidualMasks; n++ {
		var cands []ref
		for pi, p := range pieces {
			for i := range p.doc.toks {
				if p.doc.leaf(i) && !p.doc.mask[i] {
					cands = append(cands, ref{pi, i})
				}
			}
		}
		set := func(k int) { // mask cands[:k], unmask the rest
			for j, c := range cands {
				pieces[c.p].doc.mask[c.i] = j < k
			}
		}
		set(len(cands))
		if len(cands) == 0 || !clean() {
			return nil, false // the match is not in any leaf
		}
		lo, hi := 1, len(cands) // masking cands[:hi] is clean
		for lo < hi {
			mid := (lo + hi) / 2
			set(mid)
			if clean() {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		set(0)
		c := cands[lo-1]
		pieces[c.p].doc.mask[c.i] = true
		if clean() {
			return build(), true
		}
	}
	return nil, false
}

var errNotOneDocument = errors.New("credkit: not one JSON document")

// parseJSONDoc reads doc, one JSON document, as tokens.
func parseJSONDoc(doc []byte) (*jdoc, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	type frame struct {
		obj bool // an object: its tokens alternate key, value
		key bool // an object's next token is a key
		n   int  // members or elements read
	}
	var (
		d     = &jdoc{indent: bytes.IndexByte(bytes.TrimSpace(doc), '\n') >= 0}
		stack []frame
		done  bool // the top-level value is complete
	)
	// after records that a value (or a key) was read in the container.
	after := func() {
		if len(stack) == 0 {
			done = true
			return
		}
		f := &stack[len(stack)-1]
		if f.obj && f.key {
			f.key = false
			return
		}
		if f.obj {
			f.key = true
		}
		f.n++
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if done {
			return nil, errNotOneDocument
		}
		var t jtok
		closing := tok == json.Delim('}') || tok == json.Delim(']')
		if len(stack) > 0 && !closing {
			f := stack[len(stack)-1]
			switch {
			case f.obj && f.key && f.n > 0:
				t.sep = ','
			case f.obj && !f.key:
				t.sep = ':'
			case !f.obj && f.n > 0:
				t.sep = ','
			}
		}
		switch v := tok.(type) {
		case json.Delim:
			t.kind, t.b = 'd', []byte{byte(v)}
			d.toks = append(d.toks, t)
			if v == '{' || v == '[' {
				stack = append(stack, frame{obj: v == '{', key: v == '{'})
				continue
			}
			stack = stack[:len(stack)-1]
		case string:
			enc, err := encodeString(v)
			if err != nil {
				return nil, err
			}
			t.kind, t.b, t.raw = 's', enc, v
			d.toks = append(d.toks, t)
		case json.Number:
			t.kind, t.b = 'n', []byte(v.String())
			d.toks = append(d.toks, t)
		case bool:
			t.kind, t.b = 'l', []byte("false")
			if v {
				t.b = []byte("true")
			}
			d.toks = append(d.toks, t)
		case nil:
			t.kind, t.b = 'l', []byte("null")
			d.toks = append(d.toks, t)
		}
		after()
	}
	if !done {
		return nil, errNotOneDocument
	}
	d.mask = make([]bool, len(d.toks))
	return d, nil
}

// encodeString is s as a JSON string, without HTML escaping. It builds the
// bytes itself: a json.Encoder is an output call (the kit's exit-point
// gate), and json.Marshal would escape <, > and & in every URL kept.
func encodeString(s string) ([]byte, error) {
	const (
		hex = "0123456789abcdef"
		bs  = 0x5c // backslash
	)
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range strings.ToValidUTF8(s, "�") {
		switch {
		case r == '"' || r == bs:
			b = append(b, bs, byte(r))
		case r == '\n':
			b = append(b, bs, 'n')
		case r == '\r':
			b = append(b, bs, 'r')
		case r == '\t':
			b = append(b, bs, 't')
		case r < 0x20 || r == ' ' || r == ' ':
			b = append(b, bs, 'u', hex[r>>12&0xf], hex[r>>8&0xf], hex[r>>4&0xf], hex[r&0xf])
		default:
			b = utf8.AppendRune(b, r)
		}
	}
	return append(b, '"'), nil
}
