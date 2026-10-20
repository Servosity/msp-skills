// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-tool-results-escape-controls" and
// "mcp-read-only-withholds-credentials" entries.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// installResultSafety registers the one result funnel every tool call passes
// through: typed tools, code orchestration, intents and the Cobra-tree mirror
// alike, because mcp-go applies server middleware to every registered tool at
// call time. RegisterTools installs it first so it is the outermost layer and
// also sees errors raised by the tenant gate.
func installResultSafety(s *server.MCPServer) {
	s.Use(resultSafetyMiddleware)
}

func resultSafetyMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			// A handler error would otherwise reach the client verbatim as a
			// JSON-RPC error message, outside this funnel.
			return mcplib.NewToolResultError(safeResultText(err.Error())), nil
		}
		return safeToolResult(res), nil
	}
}

// safeToolResult rewrites every text block of a result in place.
func safeToolResult(res *mcplib.CallToolResult) *mcplib.CallToolResult {
	if res == nil {
		return nil
	}
	for i, c := range res.Content {
		switch tc := c.(type) {
		case mcplib.TextContent:
			tc.Text = safeResultText(tc.Text)
			res.Content[i] = tc
		case *mcplib.TextContent:
			if tc != nil {
				tc.Text = safeResultText(tc.Text)
			}
		}
	}
	if res.StructuredContent != nil && cliutil.IsMCPReadOnlyEnv() {
		// Nothing in this server sets structured content today; a future
		// handler that does must not bypass the read-only filter.
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			var v any
			if json.Unmarshal([]byte(readOnlyCredentialFilter(string(raw))), &v) == nil {
				res.StructuredContent = v
			} else {
				res.StructuredContent = nil
			}
		} else {
			res.StructuredContent = nil
		}
	}
	return res
}

// safeResultText applies the read-only credential filter, then the terminal
// control escape.
func safeResultText(s string) string {
	return escapeTerminalControls(readOnlyCredentialFilter(s))
}

// escapeTerminalControls rewrites C0 controls (except tab and newline), a CR
// not followed by LF, DEL and C1 controls (U+0080-U+009F) as \u00XX. Tenant
// strings (organization, site, policy and agent names, notes, block-page
// text) reach tool results from API JSON, the local store and the companion
// CLI's stdout; a host that prints a result to a terminal would otherwise
// execute an OSC 52 clipboard write or a CSI sequence carried in a name.
// Inside JSON string literals the rewrite is a valid escape, so the text stays
// valid JSON and decodes to the original value.
func escapeTerminalControls(s string) string {
	if !strings.ContainsFunc(s, isTerminalControl) && !strings.Contains(s, "\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i, r := range s {
		if isTerminalControl(r) || (r == '\r' && (i+1 >= len(s) || s[i+1] != '\n')) {
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[r>>4])
			b.WriteByte("0123456789abcdef"[r&0xf])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTerminalControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// credentialWithheldValue replaces the value of a credential-named field.
const credentialWithheldValue = "[withheld: credential]"

// credentialWithheldMessage replaces a whole text block that names a
// credential field outside JSON (CSV or table headers, key=value prose) or
// carries a credential value this process already withheld.
const credentialWithheldMessage = "withheld: this result carries a credential-bearing field and " + cliutil.MCPReadOnlyEnvVar + " is set; read-only MCP never returns credentials (API key tokens, network secret keys, uninstall PINs, client secrets)"

// isCredentialKey is the store's secret-key rule (store.IsSecretKey: secret,
// password, apikey, credential, a trailing token and friends, case and
// separator insensitive) plus a trailing "pin", so a PIN field
// (uninstall_pin, pin) is withheld too.
func isCredentialKey(key string) bool {
	if store.IsSecretKey(key) {
		return true
	}
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return strings.HasSuffix(b.String(), "pin")
}

// readOnlyCredentialFilter is the read-only half of the funnel. With
// DNSFILTER_MCP_READ_ONLY on, no tool result may carry a credential: every
// JSON value in the text (the whole text, NDJSON, or a JSON body quoted inside
// an error message) has each credential-named field's value replaced at any
// depth, including JSON nested inside a string; plain text that names a
// credential field as a CSV/table header or a key=value pair is withheld
// whole; and any text carrying a value this process already withheld is
// withheld whole. Matching is by key name on the decoded JSON.
func readOnlyCredentialFilter(text string) string {
	if text == "" || !cliutil.IsMCPReadOnlyEnv() {
		return text
	}
	out, plain := redactJSONSegments(text)
	if plainTextNamesCredential(plain) || carriesWithheldValue(out) {
		return credentialWithheldMessage
	}
	return out
}

// maxJSONSegmentAttempts bounds the embedded-JSON scan on text that is not one
// JSON stream, so a result full of stray brackets cannot go quadratic.
const maxJSONSegmentAttempts = 256

// redactJSONSegments returns text with every decodable JSON object or array
// redacted, plus the concatenated non-JSON remainder for the plain-text check.
func redactJSONSegments(text string) (string, string) {
	var out, plain strings.Builder
	attempts := 0
	i := 0
	for i < len(text) {
		j := strings.IndexAny(text[i:], "{[")
		if j < 0 || attempts >= maxJSONSegmentAttempts {
			out.WriteString(text[i:])
			plain.WriteString(text[i:])
			break
		}
		start := i + j
		out.WriteString(text[i:start])
		plain.WriteString(text[i:start])
		plain.WriteByte(' ')
		attempts++
		dec := json.NewDecoder(strings.NewReader(text[start:]))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			out.WriteByte(text[start])
			plain.WriteByte(text[start])
			i = start + 1
			continue
		}
		end := start + int(dec.InputOffset())
		if cleaned, changed := redactCredentialValue(v); changed {
			out.WriteString(encodeJSONCompact(cleaned))
		} else {
			out.WriteString(text[start:end])
		}
		i = end
	}
	return out.String(), plain.String()
}

func encodeJSONCompact(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return `"` + credentialWithheldValue + `"`
	}
	return strings.TrimRight(buf.String(), "\n")
}

// redactCredentialValue replaces the value of every credential-named key at
// any depth. A boolean or null value is kept (it is a flag, not a secret).
func redactCredentialValue(value any) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range v {
			if isCredentialKey(key) {
				switch child.(type) {
				case nil, bool:
				default:
					rememberWithheld(child)
					if s, ok := child.(string); ok && s == credentialWithheldValue {
						continue
					}
					v[key] = credentialWithheldValue
					changed = true
					continue
				}
			}
			if cleaned, childChanged := redactCredentialValue(child); childChanged {
				v[key] = cleaned
				changed = true
			}
		}
		return v, changed
	case []any:
		changed := false
		for i, child := range v {
			if cleaned, childChanged := redactCredentialValue(child); childChanged {
				v[i] = cleaned
				changed = true
			}
		}
		return v, changed
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
			return v, false
		}
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		var inner any
		if dec.Decode(&inner) != nil || dec.More() {
			return v, false
		}
		if cleaned, changed := redactCredentialValue(inner); changed {
			return encodeJSONCompact(cleaned), true
		}
	}
	return value, false
}

var (
	withheldMu     sync.Mutex
	withheldValues = map[string]bool{}
)

// minWithheldValueLen keeps short values (a 4-digit PIN, "true") out of the
// backstop, where they would withhold unrelated text.
const minWithheldValueLen = 8

// maxWithheldValues caps the backstop's memory.
const maxWithheldValues = 4096

func rememberWithheld(v any) {
	var s string
	switch tv := v.(type) {
	case string:
		s = tv
	case json.Number:
		s = tv.String()
	default:
		return
	}
	s = strings.TrimSpace(s)
	if len(s) < minWithheldValueLen || s == credentialWithheldValue {
		return
	}
	withheldMu.Lock()
	defer withheldMu.Unlock()
	if len(withheldValues) < maxWithheldValues {
		withheldValues[s] = true
	}
}

func carriesWithheldValue(text string) bool {
	withheldMu.Lock()
	defer withheldMu.Unlock()
	for v := range withheldValues {
		if strings.Contains(text, v) {
			return true
		}
	}
	return false
}

// keyValueRE finds the name and separator of name=value / name: value pairs in
// plain text; the caller checks that a value follows, so the value's first
// character is never consumed (it may start the next name).
var keyValueRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_.\-]*)["']?\s*[:=]`)

// headerSplitRE splits a CSV, TSV, pipe or column-aligned table header line.
var headerSplitRE = regexp.MustCompile(`\s*[,\t|]\s*|\s{2,}`)

// plainTextNamesCredential reports whether non-JSON text names a credential
// field in a shape that can carry its value: a key=value / key: value pair, or
// a delimited header line (CSV, TSV, a pipe or column-aligned table). This
// CLI's own environment variable names (DNSFILTER_API_KEY in the auth hint
// `export DNSFILTER_API_KEY="your-token-here"`) are names, not data fields,
// and are skipped so an auth error stays readable.
func plainTextNamesCredential(plain string) bool {
	for _, m := range keyValueRE.FindAllStringSubmatchIndex(plain, -1) {
		name := plain[m[2]:m[3]]
		if strings.HasPrefix(name, "DNSFILTER_") || !isCredentialKey(name) {
			continue
		}
		if rest := strings.TrimLeft(plain[m[1]:], " \t\"'"); rest != "" && !strings.ContainsAny(rest[:1], "\r\n,;") {
			return true
		}
	}
	for _, line := range strings.Split(plain, "\n") {
		fields := headerSplitRE.Split(strings.TrimSpace(line), -1)
		if len(fields) < 2 {
			continue
		}
		for _, f := range fields {
			f = strings.Trim(f, `"' `)
			// A header cell is one identifier; prose such as "invalid token"
			// between commas is not a column name.
			if f == "" || len(f) > 64 || strings.ContainsAny(f, " \t") || strings.HasPrefix(f, "DNSFILTER_") {
				continue
			}
			if isCredentialKey(f) {
				return true
			}
		}
	}
	return false
}
