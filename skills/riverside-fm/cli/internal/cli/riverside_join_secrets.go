// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"riverside-fm-pp-cli/internal/client"
	"riverside-fm-pp-cli/internal/cliutil"
)

// Riverside's studio and take responses carry each studio's join secrets
// (producerSecret, guestSecret, shareToken). Anyone holding one can join that
// studio, and no command here needs them, so they are held and removed before
// a response reaches the program: no output, MCP result, cache file or store
// row ever has them. Installed on every client, the MCP server's included.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient != nil {
			c.HTTPClient.Transport = &joinSecretTransport{base: c.HTTPClient.Transport}
		}
		return nil
	})
}

var joinSecretFields = map[string]bool{"producersecret": true, "guestsecret": true, "sharetoken": true}

type joinSecretTransport struct{ base http.RoundTripper }

func (t *joinSecretTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.Body == nil || !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		return resp, err
	}
	body, rerr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if clean, ok := removeJoinSecrets(body); ok {
		body = clean
		resp.Header.Del("Content-Length")
		resp.ContentLength = int64(len(body))
	}
	resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), errorReader{rerr}))
	return resp, nil
}

// errorReader replays a read error after the bytes that did arrive.
type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return 0, io.EOF
}

// removeJoinSecrets returns body without any join secret field, at any depth,
// and false when it has none (or is not JSON): the bytes are then left alone.
func removeJoinSecrets(body []byte) ([]byte, bool) {
	lower := bytes.ToLower(body)
	found := false
	for name := range joinSecretFields {
		found = found || bytes.Contains(lower, []byte(name))
	}
	if !found {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // no float rounding of the ids the rest of the document carries
	var doc any
	if dec.Decode(&doc) != nil || !stripJoinSecrets(doc) {
		return nil, false
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if enc.Encode(doc) != nil {
		return nil, false
	}
	return bytes.TrimRight(out.Bytes(), "\n"), true
}

func stripJoinSecrets(v any) bool {
	removed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if joinSecretFields[strings.ToLower(k)] {
				if s, ok := child.(string); ok && s != "" {
					cliutil.Hold("response:join-secret", s)
				}
				delete(t, k)
				removed = true
				continue
			}
			removed = stripJoinSecrets(child) || removed
		}
	case []any:
		for _, child := range t {
			removed = stripJoinSecrets(child) || removed
		}
	}
	return removed
}
