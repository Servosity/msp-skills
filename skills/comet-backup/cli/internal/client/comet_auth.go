// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (not generated). Comet Server reads admin credentials from form
// fields in the POST body (Username, AuthType, Password or SessionKey), never
// from a header, and reports failures as a CometAPIResponseMessage
// {Status, Message} whose Status, not the HTTP code, says whether the call
// worked. Sources:
//   https://docs.cometbackup.com/latest/api/api-guide (curl/PowerShell examples)
//   https://github.com/CometBackup/comet-go-sdk cometsdk.go CometAPIClient.Request
//   CometAPIResponseMessage: "If the operation was successful, the status will be in the 200-299 range."
//
// Wired into the generated client.go at four call sites marked
// "comet-backup hand-fix" (see handfixes.json); a reprint drops them.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"comet-backup-pp-cli/internal/config"
)

const cometFormContentType = "application/x-www-form-urlencoded"

// cometCredentialFields are the request fields Comet treats as authentication.
// Caller-supplied values are always dropped so a --stdin body can never smuggle
// or override credentials.
var cometCredentialFields = []string{"Username", "AuthType", "Password", "SessionKey", "TOTP", "WebAuthnSign", "Token"}

// cometBodyAuthPath reports whether path is a Comet API call whose credential
// rides in the form body (cometFormAuth's scope), so no auth header is sent.
func cometBodyAuthPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/")
}

// cometSecret returns the credential cometFormAuth sends: the session key when
// set, otherwise the password.
func cometSecret(cfg *config.Config) (field, value string) {
	if cfg == nil {
		return "", ""
	}
	if strings.TrimSpace(cfg.CometBackupSessionKey) != "" {
		return "SessionKey", cfg.CometBackupSessionKey
	}
	if cfg.CometBackupPassword != "" {
		return "Password", cfg.CometBackupPassword
	}
	return "", ""
}

// cometMaskValue is what the generated client keeps in its authHeader
// variable: never sent as a header for /api/v1/ paths (cometBodyAuthPath), only used by
// the generated masking so error text, URLs and retries hide the secret.
// Dry-run gets "" so it prints no header line for a header that is not sent.
func cometMaskValue(cfg *config.Config, dryRun bool) string {
	if dryRun {
		return ""
	}
	_, v := cometSecret(cfg)
	return v
}

// cometFormAuth rewrites an outgoing Comet API body into the form encoding
// Comet expects and adds the configured admin credentials. JSON object bodies
// (the generator's encoding for parameterless calls and --stdin input) are
// converted field by field: strings pass through, any other value is sent as
// its JSON text, which is how Comet documents its "(JSON)" parameters. When
// redact is true (dry-run) the secret is replaced so it never reaches output.
func cometFormAuth(cfg *config.Config, path string, body []byte, contentType string, redact bool) ([]byte, string, error) {
	if !cometBodyAuthPath(path) {
		return body, contentType, nil
	}
	fields := url.Values{}
	switch {
	case len(bytes.TrimSpace(body)) == 0:
	case strings.HasPrefix(contentType, cometFormContentType):
		parsed, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, "", fmt.Errorf("encoding Comet request: %w", err)
		}
		fields = parsed
	default:
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			return nil, "", fmt.Errorf("encoding Comet request: body must be a JSON object of Comet parameters: %w", err)
		}
		for k, v := range obj {
			switch tv := v.(type) {
			case nil:
				continue
			case string:
				fields.Set(k, tv)
			case json.Number:
				fields.Set(k, tv.String())
			default:
				raw, err := json.Marshal(tv)
				if err != nil {
					return nil, "", fmt.Errorf("encoding Comet parameter %s: %w", k, err)
				}
				fields.Set(k, string(raw))
			}
		}
	}
	for _, k := range cometCredentialFields {
		fields.Del(k)
	}
	if cfg != nil {
		username := strings.TrimSpace(cfg.CometBackupUsername)
		secretField, secret := cometSecret(cfg)
		if username != "" && secret != "" {
			if redact {
				secret = "<redacted>"
			}
			fields.Set("Username", username)
			fields.Set("AuthType", secretField)
			fields.Set(secretField, secret)
		}
	}
	return []byte(fields.Encode()), cometFormContentType, nil
}

// cometMask hides every configured Comet secret (password and session key,
// including the one not in use) in text such as a server error message.
func (c *Client) cometMask(text string) string {
	if c == nil || c.Config == nil {
		return text
	}
	return c.maskCredentialText(text, c.Config.CometBackupPassword, c.Config.CometBackupSessionKey)
}

// MaskCometText hides the configured Comet secrets in server-provided free text
// (job log lines, server messages, API error bodies). The generated masker
// replaces the literal, query- and path-escaped forms in place, keeping the
// rest of the message. Then, as a fail-closed backstop, the text is decoded
// leniently (every valid %XX, malformed escapes left alone) to a fixed point,
// comparing each level with '+' and space treated alike; if any level still
// contains a secret, or decoding does not settle, the whole message is
// withheld instead of guessing which part is the secret.
func (c *Client) MaskCometText(text string) string {
	masked := c.cometMask(text)
	if c == nil || c.Config == nil {
		return masked
	}
	plusSpace := func(s string) string { return strings.ReplaceAll(s, "+", " ") }
	var secrets []string
	for _, secret := range []string{c.Config.CometBackupPassword, c.Config.CometBackupSessionKey} {
		if secret = strings.TrimSpace(secret); secret != "" {
			secrets = append(secrets, plusSpace(secret))
		}
	}
	if len(secrets) == 0 {
		return masked
	}
	const withheld = "[withheld: the message contained a configured Comet credential]"
	// Controls are dropped before checking, so a secret split by an escape byte
	// (p@<ESC>ss) is caught here, before any terminal scrubbing could rejoin it.
	cur := stripControls(masked)
	for i := 0; i < 32; i++ {
		norm := plusSpace(cur)
		for _, secret := range secrets {
			if strings.Contains(norm, secret) {
				return withheld
			}
		}
		next := lenientPercentDecode(cur)
		if next == cur {
			return masked
		}
		cur = next
	}
	return withheld // never settled: fail closed
}

// stripControls drops C0/C1 control characters and invalid UTF-8.
func stripControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
}

// lenientPercentDecode decodes every valid %XX escape and leaves anything
// else (a lone '%', "%zz") untouched, so unrelated text cannot stop decoding.
func lenientPercentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	unhex := func(b byte) (byte, bool) {
		switch {
		case b >= '0' && b <= '9':
			return b - '0', true
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10, true
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10, true
		}
		return 0, false
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				out = append(out, hi<<4|lo)
				i += 2
				continue
			}
		}
		out = append(out, s[i])
	}
	return string(out)
}

// cometEnvelopeError turns a Comet failure envelope delivered with a 2xx HTTP
// status into an *APIError. Only a top-level object carrying both a numeric
// Status (>= 400) and a non-empty string Message qualifies: every Comet response
// wrapper has that pair, while job records (BackupJobDetail.Status is a job
// state such as 7002) carry no Message, so they are never mistaken for failures.
func (c *Client) cometEnvelopeError(method, path string, body []byte) *APIError {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return nil
	}
	rawStatus, okStatus := env["Status"]
	rawMessage, okMessage := env["Message"]
	if !okStatus || !okMessage {
		return nil
	}
	var status int
	var message string
	if json.Unmarshal(rawStatus, &status) != nil || json.Unmarshal(rawMessage, &message) != nil {
		return nil
	}
	// Same rule as the official SDKs: comet-go-sdk fails on Status >= 400 with a
	// non-empty Message, comet-php-sdk on Status >= 400.
	if status < 400 || message == "" {
		return nil
	}
	code := status
	if code > 599 {
		// Keep the typed exit-code mapping meaningful for odd values.
		code = 500
	}
	return &APIError{
		Method:     method,
		Path:       path,
		StatusCode: code,
		Body:       "Comet returned Status " + fmt.Sprint(status) + ": " + c.MaskCometText(message) + c.cometTokenHint(status),
	}
}

// cometTokenHint names the one Comet credential this CLI cannot send: a
// personal API token (bkup_..., Comet 26.8.3+), whose login field the public
// API docs do not document. It is "" unless an authentication failure (401 or
// 403) answered a request that carried a bkup_-shaped password or session key.
func (c *Client) cometTokenHint(status int) string {
	if c == nil || (status != http.StatusUnauthorized && status != http.StatusForbidden) {
		return ""
	}
	if _, v := cometSecret(c.Config); strings.HasPrefix(strings.TrimSpace(v), "bkup_") {
		return "; personal API tokens (bkup_...) are not supported: set COMET_BACKUP_PASSWORD to the admin password, or COMET_BACKUP_SESSION_KEY to an admin session key"
	}
	return ""
}

// errCometCrossOriginRedirect stops a redirect that would replay a
// credential-bearing POST body (307/308) to another origin.
func errCometCrossOriginRedirect(next *url.URL) error {
	return fmt.Errorf("refusing redirect to %s: Comet requests carry the admin credential in the body; set COMET_BACKUP_BASE_URL to the final server URL", next.Redacted())
}
