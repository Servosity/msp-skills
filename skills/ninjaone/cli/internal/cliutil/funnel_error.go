// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"net/url"
	"strings"
)

// FunnelError passes an error's text through the secret funnel at the place the
// error is made: secrets are held for the rest of the process, and when the
// text repeats one of them, or any value already held, in any encoding the
// funnel decodes, the error reads as the funnel's withheld notice. errors.Is
// and errors.As still reach err, so exit codes keep classifying it.
//
// The shared funnel looks for held values of funnelMinSecretLen (8) or more
// only. The values passed here are the credential this exchange was sent, so a
// shorter one is looked for too, case-insensitively (a hostname made of it may
// be re-cased), in the text and in every view the funnel's decoding steps reach.
// Hand-fix (class token-errors-funnelled-at-source).
func FunnelError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	FunnelHold(secrets...)
	text := err.Error()
	if msg := FunnelHeld(text); FunnelWithheld(msg) {
		return funnelledError{err: err, msg: msg}
	}
	if funnelShortEchoes(text, secrets) {
		msg := FunnelWithheldText
		if funnelReadsAsAuth(text) {
			msg = FunnelWithheldAuthText
		}
		return funnelledError{err: err, msg: msg}
	}
	return err
}

// funnelShortEchoes reports whether text carries one of secrets shorter than
// funnelMinSecretLen, as written, query- or path-escaped, in any letter case,
// in text or any view the funnel's steps reach (views of any length are kept).
// Past the funnel's work cap, or for text over funnelMaxInput, it reports true.
func funnelShortEchoes(text string, secrets []string) bool {
	var needles []string
	shortest := funnelMinSecretLen
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if s == "" || len(s) >= funnelMinSecretLen {
			continue
		}
		if len(s) < shortest {
			shortest = len(s)
		}
		for _, form := range []string{s, url.QueryEscape(s), url.PathEscape(s)} {
			needles = append(needles, strings.ToLower(form))
		}
	}
	if len(needles) == 0 || text == "" {
		return false
	}
	// The funnel's steps, plus base64 and hex views sized to the shortest value
	// (the funnel's own decode only takes runs long enough for 8 bytes).
	steps := append(append([]func(string) []string{}, funnelSteps...),
		func(view string) []string {
			minB64 := (shortest*4 + 2) / 3
			var runs []string
			funnelForRuns(view, funnelIsBase64, minB64, func(run string) { runs = append(runs, run) })
			return funnelJoinedViews(runs, 4, minB64, funnelLenientBase64)
		},
		func(view string) []string {
			var runs []string
			funnelForRuns(view, funnelIsHex, 2*shortest, func(run string) { runs = append(runs, run) })
			return funnelJoinedViews(runs, 2, 2*shortest, funnelLenientHex)
		})
	if len(text) > funnelMaxInput {
		return true
	}
	budget := funnelWorkPerByte*len(text) + funnelWorkBase
	seen := map[string]bool{text: true}
	queue := []string{text}
	for len(queue) > 0 {
		view := queue[0]
		queue = queue[1:]
		lower := strings.ToLower(view)
		for _, n := range needles {
			if strings.Contains(lower, n) {
				return true
			}
		}
		for _, step := range steps {
			budget -= len(view) + 1
			if budget < 0 {
				return true
			}
			for _, next := range step(view) {
				if next == "" || seen[next] {
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

type funnelledError struct {
	err error
	msg string
}

func (e funnelledError) Error() string { return e.msg }
func (e funnelledError) Unwrap() error { return e.err }
