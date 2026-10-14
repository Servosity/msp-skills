// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json dry-run-redacts-secrets).

package client

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// redactDryRunBody parses a request body for the dry-run preview and replaces
// every non-empty, non-boolean value of a secret-named field (password,
// secret, private key, API key, *token but not page/sync tokens) with
// "[redacted]".
// ok is false when the body is not JSON.
func redactDryRunBody(body []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	return redactSecretFields(v), true
}

func redactSecretFields(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if secretFieldName(k) && child != nil && child != "" {
				if _, isBool := child.(bool); !isBool { // booleans such as changePasswordAtNextLogin stay visible
					t[k] = "[redacted]"
					continue
				}
			}
			t[k] = redactSecretFields(child)
		}
	case []any:
		for i := range t {
			t[i] = redactSecretFields(t[i])
		}
	}
	return v
}

func secretFieldName(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"password", "secret", "privatekey", "private_key", "apikey", "api_key"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	// Webhook signatures and keys (SAS sig=, Azure Functions code=, Google
	// key=) carried as query parameters in --deliver and watch addresses.
	if k == "sig" || k == "code" || k == "key" || strings.Contains(k, "signature") {
		return true
	}
	// Paging cursors are not credentials.
	return strings.HasSuffix(k, "token") && !strings.HasSuffix(k, "pagetoken") && !strings.HasSuffix(k, "synctoken")
}

// queryParamName matches the name= of a query parameter after ?, & or a
// JSON/HTML-escaped & (\u0026, &amp;); secretValueSpan finds its value.
var queryParamName = regexp.MustCompile(`(&amp;|\\u0026|[?&])([A-Za-z0-9_.%-]+)=`)

// redactSecretQueryParams masks the value of every secret-named query
// parameter in text (URLs inside error messages, dry-run lines, bodies).
func redactSecretQueryParams(text string) string {
	if !strings.Contains(text, "=") {
		return text
	}
	var b strings.Builder
	last := 0
	for _, m := range queryParamName.FindAllStringSubmatchIndex(text, -1) {
		if m[0] < last {
			continue
		}
		name, err := url.QueryUnescape(text[m[4]:m[5]])
		if err != nil || !secretFieldName(name) {
			continue
		}
		start, end := secretValueSpan(text, m[0], m[1])
		if start == end {
			continue
		}
		b.WriteString(text[last:start])
		b.WriteString("[redacted]")
		last = end
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

// secretValueSpan returns the span of the value that starts at i. The
// enclosing quote is the character just before the URL's scheme (looked
// up within a bounded window, so earlier text and earlier apostrophes in
// the URL do not count). A quote right after '=' is the end of an empty
// value when it is the enclosing quote, or a double quote with no
// enclosing quote, and it is followed by a separator; otherwise it opens a
// quoted value that runs to its match. An unquoted value runs to &,
// whitespace, #, ", a backslash or the enclosing quote, so an apostrophe
// inside it (O'Hara) stays masked. Hand-fix (handfixes.json
// dry-run-redacts-secrets).
func secretValueSpan(s string, urlStart, i int) (int, int) {
	enclosing := enclosingQuote(s, urlStart)
	rest := s[i:]
	for _, q := range []string{`\"`, `"`, `'`} {
		if !strings.HasPrefix(rest, q) {
			continue
		}
		after := rest[len(q):]
		closing := q == enclosing || (enclosing == "" && q != "'")
		if closing && (after == "" || strings.ContainsRune(",;:)}] \t\r\n", rune(after[0]))) {
			return i, i
		}
		start := i + len(q)
		end := start + valueEnd(after, "")
		if k := strings.Index(s[start:end+min(len(q), len(s)-end)], q); k >= 0 {
			end = start + k
		}
		return start, end
	}
	return i, i + valueEnd(rest, enclosing)
}

// enclosingQuote returns the quote (" ' or \") just before the scheme of the
// URL that contains position urlStart, or "" when the URL is not quoted.
func enclosingQuote(s string, urlStart int) string {
	lo := max(0, urlStart-2048)
	k := strings.LastIndex(s[lo:urlStart], "://")
	if k < 0 || strings.ContainsAny(s[lo+k:urlStart], " \t\r\n") {
		return ""
	}
	j := lo + k
	for j > 0 && (isSchemeByte(s[j-1])) {
		j--
	}
	switch {
	case j > 1 && s[j-1] == '"' && s[j-2] == '\\':
		return `\"`
	case j > 0 && (s[j-1] == '"' || s[j-1] == '\''):
		return string(s[j-1])
	}
	return ""
}

func isSchemeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'
}

// valueEnd is the length of an unquoted query value at the start of s.
func valueEnd(s, enclosing string) int {
	for k := 0; k < len(s); k++ {
		switch s[k] {
		case '&', ' ', '\t', '\n', '\r', '#', '"', '\\':
			return k
		case '\'':
			if enclosing == "'" {
				return k
			}
		}
	}
	return len(s)
}

// userinfoPasswordPattern splits userinfo the way net/url does: the
// authority ends at the first / ? or #, and the password runs to its last @.
var userinfoPasswordPattern = regexp.MustCompile(`(://[^/?#\s:"]*:)[^/?#\s"]*@`)

// RedactURLSecrets masks secret-named query parameters and any userinfo
// password (https://user:pass@host) in URLs inside s.
func RedactURLSecrets(s string) string {
	return userinfoPasswordPattern.ReplaceAllString(redactSecretQueryParams(s), "${1}[redacted]@")
}

// redactChannelToken masks the token Google echoes back in a watch response
// (kind api#channel), and any secret query parameter in its echoed address.
// Both are webhook secrets, and the response is printed, returned over MCP
// and written to the local store.
func redactChannelToken(body []byte) []byte {
	if !bytes.Contains(body, []byte(`api#channel`)) {
		return body
	}
	var ch map[string]json.RawMessage
	if json.Unmarshal(body, &ch) != nil {
		return body
	}
	var kind, token, address string
	_ = json.Unmarshal(ch["kind"], &kind)
	_ = json.Unmarshal(ch["token"], &token)
	_ = json.Unmarshal(ch["address"], &address)
	masked := RedactURLSecrets(address)
	if kind != "api#channel" || (token == "" && masked == address) {
		return body
	}
	if token != "" {
		ch["token"] = json.RawMessage(`"[redacted]"`)
	}
	if masked != address {
		// A webhook address can carry its own secret (?token=..., user:pass@).
		ch["address"], _ = json.Marshal(masked)
	}
	out, err := json.Marshal(ch)
	if err != nil {
		return body
	}
	return out
}
