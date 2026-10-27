// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tokenResponseFields are the OAuth token-response members that carry a live
// credential. An authority that answers a non-success status with a token
// body (or echoes one in an error) must not get it printed.
var tokenResponseFields = []string{"access_token", "refresh_token", "id_token"}

// tokenFieldText catches a token member in any textual spelling: JSON
// ("access_token":"v"), JSON escaped inside a JSON string
// (\"access_token\":\"v\"), and form encoding (access_token=v). The value
// class admits backslashes so a \u-escaped value is masked too.
var tokenFieldText = regexp.MustCompile(`(?i)((?:\\*")?(?:access_token|refresh_token|id_token)(?:\\*")?\s*[:=]\s*(?:\\*")?)[^"&,\s}]+`)

// RedactTokenResponse masks every token-response credential in a token
// endpoint's body before any of it is shown. A JSON body is redacted
// structurally and re-serialized, so no escaped spelling of a token value
// (\u006d..., or a token body nested inside a JSON-encoded error string) can
// survive; any other text goes through the field pattern.
func RedactTokenResponse(body string) string {
	var v any
	if json.Unmarshal([]byte(strings.TrimSpace(body)), &v) == nil {
		if b, err := marshalPlain(redactTokenNode(v, 0)); err == nil {
			body = b
		}
	}
	body = tokenFieldText.ReplaceAllString(body, "${1}****")
	// Hand-fix (handfixes.json token-response-redaction): a token member can
	// also arrive spelled with escapes the pattern cannot see (access\u005ftoken,
	// access%5Ftoken, &#95;). Decode every such layer; if a token member with a
	// value other than the mask is still there, withhold the whole text.
	if tokenFieldSurvives(body) {
		return TokenResponseWithheld
	}
	return body
}

// TokenResponseWithheld replaces a token endpoint answer that still carries a
// token member after redaction.
const TokenResponseWithheld = "[token endpoint response withheld: it carried a token field]"

// tokenFieldSurvives reports whether text, after up to 8 rounds of undoing
// JSON/JavaScript escapes, percent escapes and HTML entities, still holds a
// token member whose value is not the mask.
func tokenFieldSurvives(text string) bool {
	for round := 0; round < 8; round++ {
		for _, m := range tokenFieldText.FindAllStringSubmatchIndex(text, -1) {
			value := strings.TrimLeft(text[m[3]:m[1]], `\"`)
			if !strings.HasPrefix(value, "****") {
				return true
			}
		}
		next := undoTextEscapes(text)
		if next == text {
			return false
		}
		text = next
	}
	return true // still changing after 8 rounds: fail closed
}

// undoTextEscapes undoes one layer of \uXXXX, \xXX and backslash escapes,
// percent escapes and HTML entities.
func undoTextEscapes(s string) string {
	s = jsEscape.ReplaceAllStringFunc(s, func(m string) string {
		switch {
		case strings.HasPrefix(m, `\u`) || strings.HasPrefix(m, `\x`):
			if u, err := strconv.Unquote(`"` + m + `"`); err == nil {
				return u
			}
			return m
		default:
			return m[1:]
		}
	})
	// Decode every valid %XX and leave a malformed one as it is, so one stray
	// percent sign cannot shield the rest of the text from decoding.
	s = percentEscape.ReplaceAllStringFunc(s, func(m string) string {
		if raw, err := hex.DecodeString(m[1:]); err == nil {
			return string(raw)
		}
		return m
	})
	return html.UnescapeString(s)
}

var percentEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)

var jsEscape = regexp.MustCompile(`\\u[0-9a-fA-F]{4}|\\x[0-9a-fA-F]{2}|\\.`)

func isTokenField(key string) bool {
	for _, k := range tokenResponseFields {
		if strings.EqualFold(key, k) {
			return true
		}
	}
	return false
}

// redactTokenNode replaces token-member values with "****", descending into
// objects, arrays and string values that are themselves JSON documents.
func redactTokenNode(node any, depth int) any {
	switch x := node.(type) {
	case map[string]any:
		for k, child := range x {
			if _, isString := child.(string); isString && isTokenField(k) {
				x[k] = "****"
				continue
			}
			x[k] = redactTokenNode(child, depth)
		}
	case []any:
		for i := range x {
			x[i] = redactTokenNode(x[i], depth)
		}
	case string:
		if depth < 4 {
			var inner any
			if json.Unmarshal([]byte(strings.TrimSpace(x)), &inner) == nil {
				switch inner.(type) {
				case map[string]any, []any:
					if b, err := marshalPlain(redactTokenNode(inner, depth+1)); err == nil {
						return b
					}
				}
			}
		}
	}
	return node
}

// marshalPlain is json.Marshal without HTML escaping, so a vendor message
// keeps its < > & readable.
func marshalPlain(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// unknownLifetimeValidity is how long a token whose endpoint gave no usable
// expires_in (missing or zero) is trusted before a re-mint. Short, so a token
// of unknown age is not reused for long, and bounded, so an endpoint that
// never sends expires_in costs one mint per window, not one per request.
const unknownLifetimeValidity = 10 * time.Minute

// UnknownLifetimeExpiry is the expiry to record for such a token.
func UnknownLifetimeExpiry(now time.Time) time.Time {
	return now.Add(unknownLifetimeValidity)
}

// ErrExpiredOnArrival reports a token endpoint that answered with a negative
// expires_in: the token is already spent and must not be installed.
var ErrExpiredOnArrival = errors.New("token endpoint returned an already-expired token (negative expires_in)")

// TokenErrorText is the only server text a failed token request shows (hand-fix
// handfixes.json token-error-text-allowlist). It prints no vendor prose at all:
// the OAuth error code only when it is one of the fixed RFC 6749 / Azure AD
// codes, and the Azure AD error numbers (AADSTS followed by 4 to 7 digits)
// found in error_description. Everything else the token endpoint sent is
// withheld, so no wording or encoding can carry a token or a secret out.
func TokenErrorText(body []byte) string {
	var aad struct {
		Error     string `json:"error"`
		ErrorDesc string `json:"error_description"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &aad) != nil || aad.Error == "" {
		return "(response withheld: not an Azure AD error)"
	}
	code := "(code withheld)"
	if tokenErrorCodes[aad.Error] {
		code = aad.Error
	}
	var aadsts []string
	seen := map[string]bool{}
	for _, m := range aadstsCode.FindAllString(aad.ErrorDesc, -1) {
		if !seen[m] && len(aadsts) < 3 {
			seen[m] = true
			aadsts = append(aadsts, m)
		}
	}
	out := code
	if len(aadsts) > 0 {
		out += ": " + strings.Join(aadsts, ", ")
	}
	if strings.TrimSpace(aad.ErrorDesc) != "" {
		out += " (description withheld)"
	}
	return out
}

// TokenResponseError describes a token-endpoint answer that cannot be used by
// its kind only, plus the allowlisted OAuth code and AADSTS numbers when the
// body is an Azure AD error (TokenErrorText). Never body text and never a
// decoder message: json's errors quote the input ("cannot unmarshal number
// 1234... into .expires_in"), and the input can be a token. Hand-fix
// (handfixes.json token-error-text-allowlist).
func TokenResponseError(kind string, body []byte) error {
	var aad struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &aad) == nil && aad.Error != nil {
		return errors.New(kind + ": " + TokenErrorText(body))
	}
	return errors.New(kind)
}

// tokenErrorCodes are the OAuth 2.0 error codes a token endpoint may answer
// with (RFC 6749 section 5.2, plus Azure AD's interaction_required,
// temporarily_unavailable and server_error).
var tokenErrorCodes = map[string]bool{
	"invalid_request": true, "invalid_client": true, "invalid_grant": true,
	"unauthorized_client": true, "unsupported_grant_type": true, "invalid_scope": true,
	"interaction_required": true, "temporarily_unavailable": true, "server_error": true,
}

var aadstsCode = regexp.MustCompile(`\bAADSTS[0-9]{4,7}\b`)
