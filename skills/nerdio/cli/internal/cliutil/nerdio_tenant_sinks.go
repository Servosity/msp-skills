// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// nerdio handfix tenant-secrets-shown-never-stored, the sinks: every file this
// program writes passes WriteFile or a TenantWriter, which withhold a line
// that repeats a registered tenant secret and then hand the bytes to the
// shared credential kit (credkit.WriteFile and its writers), which checks
// them for held credentials. Echoes is the one matcher, also used by the
// last check before config.toml is written (config-file-holds-no-credential).

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"net/url"
	"os"
	"strings"

	"nerdio-pp-cli/internal/credkit"
)

// TenantWithheldNotice replaces a withheld line.
const TenantWithheldNotice = "[withheld: it repeated a tenant secret this program showed]"

// Echoes reports text that repeats any of values, at any length: as written,
// JSON-, URL- or HTML-escaped, base64, base32 or hex encoded, or inside a base64 or
// hex run of text that decodes to it.
// ponytail: forms of the whole value plus decoded runs; a value split across
// separate pieces of text is not reassembled (the kit's matcher does that
// for held credentials, and tenant secrets are not contract items).
func Echoes(text string, values []string) bool {
	views := []string{text, html.UnescapeString(text)}
	if u, err := url.QueryUnescape(text); err == nil {
		views = append(views, u)
	}
	views = append(views, decodedRuns(text)...)
	for _, v := range values {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		forms := []string{v, url.QueryEscape(v), url.PathEscape(v), hex.EncodeToString([]byte(v)), strings.ToUpper(hex.EncodeToString([]byte(v)))}
		if b, err := json.Marshal(v); err == nil {
			forms = append(forms, string(b[1:len(b)-1]))
		}
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
			forms = append(forms, enc.EncodeToString([]byte(v)))
		}
		for _, enc := range []*base32.Encoding{base32.StdEncoding, base32.HexEncoding} {
			b32 := enc.WithPadding(base32.NoPadding).EncodeToString([]byte(v)) // a prefix of the padded form
			forms = append(forms, b32, strings.ToLower(b32))
		}
		for _, view := range views {
			for _, f := range forms {
				if strings.Contains(view, f) {
					return true
				}
			}
		}
	}
	return false
}

// decodedRuns is what each base64 or hex run of text decodes to, read from
// each of its first four offsets so a run glued to other characters still
// decodes.
func decodedRuns(text string) []string {
	var out []string
	for _, run := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '-' || r == '_' || r == '=')
	}) {
		if len(run) < 4 {
			continue
		}
		if b, err := hex.DecodeString(run[:len(run)/2*2]); err == nil {
			out = append(out, string(b))
		}
		trimmed := strings.TrimRight(run, "=")
		for off := 0; off < 4 && off < len(trimmed); off++ {
			r := trimmed[off:]
			if len(r)%4 == 1 {
				r = r[:len(r)-1]
			}
			for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
				if b, err := enc.DecodeString(r); err == nil {
					out = append(out, string(b))
				}
			}
		}
	}
	return out
}

// WithholdTenantSecrets replaces every line of b that repeats a registered
// tenant secret with TenantWithheldNotice.
func WithholdTenantSecrets(b []byte) []byte {
	if !TenantSecretEchoes(string(b)) {
		return b
	}
	lines := bytes.SplitAfter(b, []byte("\n"))
	for i, l := range lines {
		if TenantSecretEchoes(string(l)) {
			nl := ""
			if bytes.HasSuffix(l, []byte("\n")) {
				nl = "\n"
			}
			lines[i] = []byte(TenantWithheldNotice + nl)
		}
	}
	out := bytes.Join(lines, nil)
	if TenantSecretEchoes(string(out)) { // split across lines: withhold it whole
		return []byte(TenantWithheldNotice + "\n")
	}
	return out
}

// WriteFile is credkit.WriteFile after WithholdTenantSecrets.
func WriteFile(name string, data []byte, perm os.FileMode) error {
	return credkit.WriteFile(name, WithholdTenantSecrets(data), perm)
}

// AppendFile is credkit.AppendFile after WithholdTenantSecrets.
func AppendFile(name string, record []byte, perm os.FileMode) error {
	return credkit.AppendFile(name, WithholdTenantSecrets(record), perm)
}

// TenantWriter passes each write to w after WithholdTenantSecrets. w is a
// kit writer (credkit.OpenFile, credkit.Create, credkit.NewWriter).
type TenantWriter struct{ W io.Writer }

func (t TenantWriter) Write(p []byte) (int, error) {
	if _, err := t.W.Write(WithholdTenantSecrets(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
