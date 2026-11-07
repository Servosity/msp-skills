// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pagerduty handfix oncall-entries-keyed: an on-call entry from GET /oncalls
// has no id (developer.pagerduty.com, List all of the on-calls: user,
// schedule, escalation_policy, escalation_level, start, end; start and end
// are null for a permanent on-call). The spec keys /oncalls on
// `pp_oncall_key` (x-resource-id), and this transport stamps that key on
// every entry that lacks one: a digest of the policy, level, schedule, user,
// start and end, so two shifts of one person never collapse into one store
// row and a permanent on-call is kept. Without it sync stored the entries
// under a fingerprint of their top-level scalars (level, start, end), and two
// people on call at the same level and hours overwrote each other.
// Installed beneath the origin pin by registerClientHook (no generated file
// changes); a body it cannot read as the documented envelope passes through
// unchanged.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"pagerduty-pp-cli/internal/client"
)

const oncallKeyField = "pp_oncall_key"

func init() {
	registerClientHook(func(c *client.Client) error {
		stampOncallKeys(c)
		return nil
	})
}

// stampOncallKeys puts oncallKeyTripper directly beneath the origin pin, so
// the pin stays the outermost transport (pagerduty_guard.go).
func stampOncallKeys(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	p, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		return // no pin: every request is refused before it is sent anyway
	}
	if _, done := p.next.(oncallKeyTripper); done {
		return
	}
	p.next = oncallKeyTripper{next: p.next}
	c.HTTPClient.Transport = p
}

type oncallKeyTripper struct{ next http.RoundTripper }

func (t oncallKeyTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || req.Method != http.MethodGet || resp.StatusCode != http.StatusOK ||
		!strings.HasSuffix(strings.TrimSuffix(req.URL.Path, "/"), "/oncalls") {
		return resp, err
	}
	body, rerr := client.ReadBody(resp.Body)
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	plain, derr := client.DecodeBody(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}
	out := stampOncallBody(plain)
	resp.Header.Del("Content-Encoding")
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	if resp.Header.Get("Content-Length") != "" {
		resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	}
	return resp, nil
}

// stampOncallBody returns body with pp_oncall_key on every entry of its
// top-level "oncalls" array that lacks one; any other shape comes back as is.
func stampOncallBody(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var env map[string]any
	if err := dec.Decode(&env); err != nil || dec.More() {
		return body
	}
	entries, ok := env["oncalls"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if _, has := m[oncallKeyField]; has {
			continue
		}
		m[oncallKeyField] = oncallKey(m)
		changed = true
	}
	if !changed {
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return body
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// oncallKey is the identity of one on-call entry: who is on call, for which
// policy level and schedule, over which span. Absent and null parts read as
// empty, so a permanent on-call (null start and end) still gets a key.
func oncallKey(m map[string]any) string {
	ref := func(k string) string {
		r, _ := m[k].(map[string]any)
		return scalarText(r["id"])
	}
	parts := []string{ref("escalation_policy"), scalarText(m["escalation_level"]), ref("schedule"), ref("user"),
		scalarText(m["start"]), scalarText(m["end"])}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return "oncall-" + hex.EncodeToString(sum[:12])
}

func scalarText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
