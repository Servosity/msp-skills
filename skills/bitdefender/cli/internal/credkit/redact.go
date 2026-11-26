// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 4, MCP tool results (amended): a tool result the matcher would
// withhold is first redacted member by member. Each piece of its content
// that is one JSON document is rewritten with every leaf (a string, a
// number or an object key) that would be withheld on its own replaced by
// Redacted, keeping member order. The redacted result is then checked whole
// again, exactly as before; if it still matches, or any piece of the
// content is not one JSON document, the whole result is withheld as before.
// Redaction never releases a byte the whole-unit check would refuse: it only
// lets a result that is clean after masking reach the client instead of the
// notice.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/mark3labs/mcp-go/mcp"
)

// Redacted is the fixed text that replaces a leaf carrying a held value.
const Redacted = "***"

// redactToolResult returns a copy of res with every withheld JSON leaf of
// its content replaced by Redacted, and false when some piece of the
// content that would be withheld cannot be redacted.
func redactToolResult(res *mcp.CallToolResult) (*mcp.CallToolResult, bool) {
	if res == nil {
		return nil, false
	}
	out := *res
	out.Content = make([]mcp.Content, len(res.Content))
	for i, c := range res.Content {
		switch tc := c.(type) {
		case mcp.TextContent:
			text, ok := redactText(tc.Text)
			if !ok {
				return nil, false
			}
			tc.Text = text
			out.Content[i] = tc
		case *mcp.TextContent:
			if tc == nil {
				out.Content[i] = c
				continue
			}
			text, ok := redactText(tc.Text)
			if !ok {
				return nil, false
			}
			cp := *tc
			cp.Text = text
			out.Content[i] = &cp
		default:
			// Images, audio, resources: not redacted. The final check on the
			// whole result withholds it if one of them carries a held value.
			out.Content[i] = c
		}
	}
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return nil, false
		}
		if withheld(b) {
			r, ok := redactJSON(b)
			if !ok {
				return nil, false
			}
			out.StructuredContent = json.RawMessage(r)
		}
	}
	return &out, true
}

// redactText is text unchanged when it is clean, or text redacted as one
// JSON document.
func redactText(text string) (string, bool) {
	if !withheld([]byte(text)) {
		return text, true
	}
	r, ok := redactJSON([]byte(text))
	if !ok {
		return "", false
	}
	return string(r), true
}

var errNotOneDocument = errors.New("credkit: not one JSON document")

// redactJSON rewrites doc, one JSON document, with every leaf that would be
// withheld on its own replaced by the JSON string Redacted. Member order is
// kept; a document that was indented is indented again with two spaces.
func redactJSON(doc []byte) ([]byte, bool) {
	r, err := redactJSONTokens(doc)
	if err != nil {
		return nil, false
	}
	if bytes.IndexByte(bytes.TrimSpace(doc), '\n') >= 0 {
		var ind bytes.Buffer
		if json.Indent(&ind, r, "", "  ") == nil {
			return ind.Bytes(), true
		}
	}
	return r, true
}

func redactJSONTokens(doc []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	type frame struct {
		obj bool // an object: its tokens alternate key, value
		key bool // an object's next token is a key
		n   int  // members or elements written
	}
	var (
		buf   bytes.Buffer
		stack []frame
		done  bool // the top-level value is complete
	)
	// after records that a value (or a key) was written in the container.
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
		closing := tok == json.Delim('}') || tok == json.Delim(']')
		if len(stack) > 0 && !closing {
			f := stack[len(stack)-1]
			switch {
			case f.obj && f.key && f.n > 0:
				buf.WriteByte(',')
			case f.obj && !f.key:
				buf.WriteByte(':')
			case !f.obj && f.n > 0:
				buf.WriteByte(',')
			}
		}
		switch v := tok.(type) {
		case json.Delim:
			buf.WriteByte(byte(v))
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
			if withheld([]byte(v)) || withheld(enc) {
				enc, _ = encodeString(Redacted)
			}
			buf.Write(enc)
		case json.Number:
			if withheld([]byte(v.String())) {
				enc, _ := encodeString(Redacted)
				buf.Write(enc)
			} else {
				buf.WriteString(v.String())
			}
		case bool:
			if v {
				buf.WriteString("true")
			} else {
				buf.WriteString("false")
			}
		case nil:
			buf.WriteString("null")
		}
		after()
	}
	if !done {
		return nil, errNotOneDocument
	}
	return buf.Bytes(), nil
}

// encodeString is s as a JSON string, without HTML escaping.
func encodeString(s string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
