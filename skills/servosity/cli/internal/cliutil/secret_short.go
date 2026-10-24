// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "credential-header-values-masked" entry. See docs/reprint-survival.md.

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// FunnelEchoesShort is FunnelEchoes for credentials shorter than
// funnelMinSecretLen (a 6-digit MFA code), which FunnelEchoes skips. It walks
// the same fixed decoding steps breadth-first under the same total-work cap,
// keeps views down to minLen characters, and fails closed: past the cap, or
// for text over funnelMaxInput, it reports true. There is no depth limit, so
// a value wrapped in any number of encodings is still reached. secret_funnel.go
// stays byte-identical across the fleet; this file adds the short-value walk.
// extra steps (a caller's own decoders, such as full-width folding) run beside
// the funnel's on every view, under the same cap.
func FunnelEchoesShort(text string, secrets []string, minLen int, extra ...func(string) []string) bool {
	// The steps decode base64 and hex runs only when they are long enough to
	// hold an 8-character secret, so a short code's own base64 and hex forms
	// are needles too: a chain of encodings is decoded down to one of them.
	var needles []string
	for _, s := range secrets {
		if s = strings.TrimSpace(s); len(s) >= minLen {
			needles = append(needles, s)
			needles = append(needles, shortEncodings(s)...)
		}
	}
	if len(needles) == 0 || text == "" {
		return false
	}
	if len(text) > funnelMaxInput {
		return true
	}
	budget := funnelWorkPerByte*len(text) + funnelWorkBase
	steps := append(funnelSteps[:len(funnelSteps):len(funnelSteps)], extra...)
	seen := map[string]bool{text: true}
	queue := []string{text}
	for len(queue) > 0 {
		view := queue[0]
		queue = queue[1:]
		for _, n := range needles {
			if strings.Contains(view, n) {
				return true
			}
		}
		for _, step := range steps {
			budget -= len(view) + 1
			if budget < 0 {
				return true
			}
			for _, next := range step(view) {
				if len(next) < minLen || seen[next] {
					continue
				}
				budget -= len(next) + 1
				if budget < 0 {
					return true
				}
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func shortEncodings(s string) []string {
	b := []byte(s)
	return []string{
		base64.StdEncoding.EncodeToString(b), base64.RawStdEncoding.EncodeToString(b),
		base64.URLEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b),
		hex.EncodeToString(b), strings.ToUpper(hex.EncodeToString(b)),
	}
}
