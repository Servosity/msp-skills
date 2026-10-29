// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/url"
	"strings"
	"sync"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// MaskCredentials masks every configured credential, in every encoding the
// client masks in its own errors, in text that hand-written commands build
// from a response (kaseya-vsa handfix: response-text-masked).
func (c *Client) MaskCredentials(text string) string {
	return c.maskCredentialText(text)
}

// kaseya-vsa handfix (output-chokepoint, review r1 P1-B): the output
// chokepoint (cliutil.HeldEchoes) consults this client's own credential
// masker, so every encoding the masker recognizes in an error is withheld
// from every other output too. Both binaries link this package, so the
// detector is always registered (TestClientMaskerFeedsTheChokepoint, in both binaries).
func init() {
	cliutil.RegisterEchoDetector(maskerEchoDetector)
}

// maskerEchoDetector builds, once per held set, the needles
// maskCredentialText builds for each value (as written, query- and
// path-escaped, the token after a scheme word, and every form
// addEncodedCredentialMasks adds) and returns a check that also runs the
// masker's own decoded-view pass (finishCredentialMask). The needle builder
// mirrors maskCredentialText's addValue and addCredential;
// TestR1BMaskCredentialTextUnchangedSinceTheChokepointMirroredIt fails when
// that body changes. Values shorter than minEncodedSecretLen are not needles
// here: the chokepoint's floor, as the funnel's.
func maskerEchoDetector(held []string) func(string) bool {
	var needles, secrets []string
	seen := map[string]struct{}{}
	addMask := func(needle string) {
		if needle == "" {
			return
		}
		if _, ok := seen[needle]; ok {
			return
		}
		seen[needle] = struct{}{}
		needles = append(needles, needle)
	}
	addValue := func(value string) {
		value = strings.TrimSpace(value)
		if len(value) < minEncodedSecretLen {
			return
		}
		addMask(value)
		if escaped := url.QueryEscape(value); escaped != value {
			addMask(escaped)
		}
		if escaped := url.PathEscape(value); escaped != value {
			addMask(escaped)
		}
		secrets = append(secrets, value)
		addEncodedCredentialMasks(value, addMask)
	}
	for _, value := range held {
		value = strings.TrimSpace(value)
		addValue(value)
		if _, token, ok := strings.Cut(value, " "); ok {
			addValue(token)
		}
	}
	if len(needles) == 0 {
		return nil
	}
	long := longCredentialSecrets(secrets)
	spans := &spanMemo{seen: map[string]bool{}}
	return func(text string) bool {
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				return true
			}
		}
		// finishCredentialMask changes text exactly when one of the spans
		// credentialRevealSpans considers reveals a secret, or else the whole
		// text does. The spans repeat across a long output (JSON keys), so
		// each distinct span is walked once per held set.
		for _, sp := range credentialMaskSpans(text) {
			if spans.reveals(text[sp[0]:sp[1]], long) {
				return true
			}
		}
		return credentialTextReveals(text, long)
	}
}

// credentialMaskSpans is the span list credentialRevealSpans walks (pinned by
// TestR1BMaskCredentialTextUnchangedSinceTheChokepointMirroredIt).
func credentialMaskSpans(text string) [][2]int {
	var spans [][2]int
	for _, sp := range credentialRuns(text, credentialIsB64, false) {
		if sp[1]-sp[0] >= minEncodedSecretLen {
			spans = append(spans, sp)
		}
	}
	spans = append(spans, credentialMIMESpans(text)...)
	for _, sp := range credentialRuns(text, credentialIsHex, false) {
		if sp[1]-sp[0] >= 16 && (sp[1]-sp[0])%2 == 0 {
			spans = append(spans, sp)
		}
	}
	return spans
}

// spanMemo remembers credentialTextReveals for short spans, bounded.
type spanMemo struct {
	mu    sync.Mutex
	seen  map[string]bool
	bytes int
}

func (m *spanMemo) reveals(span string, secrets []string) bool {
	if span == "" {
		return false
	}
	if len(span) > 1<<10 {
		return credentialTextReveals(span, secrets)
	}
	m.mu.Lock()
	r, ok := m.seen[span]
	m.mu.Unlock()
	if ok {
		return r
	}
	r = credentialTextReveals(span, secrets)
	m.mu.Lock()
	if m.bytes+len(span) > 4<<20 {
		m.seen, m.bytes = map[string]bool{}, 0
	}
	m.seen[span] = r
	m.bytes += len(span)
	m.mu.Unlock()
	return r
}
