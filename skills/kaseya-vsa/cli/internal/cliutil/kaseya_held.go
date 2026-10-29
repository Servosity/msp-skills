// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// kaseya-vsa handfix (output-chokepoint, review r1 P1-B): HeldEchoes, the one
// check behind every output path (CLI streams, MCP results, --deliver,
// export, the ledgers and the store), must recognize every encoding the
// client's own masker recognizes. Review r1 found a ROT13 echo of the token
// secret that the client masked in its errors but the chokepoint printed.
//
// The fix shares the masker instead of copying its transforms: the client
// package registers a detector built from its own needle builder and its own
// decoded-view check (internal/client/kaseya_mask.go), and HeldEchoes asks
// it after the funnel's own search. Every transform the masker has, or gains
// in a reprint, is therefore in the chokepoint too.
//
// The held set only grows, so the funnel's needles and each detector are
// built once per held set and reused (the funnel used to rebuild them on
// every call, about 200 microseconds per short string); short texts that
// repeat, such as JSON keys, are answered from a bounded memo.
//
// Provenance (review r2 P1): detectors are given every held credential, and
// the encodings the chokepoint derives (holdEncodedCores) are kept apart from
// the held set, so no credential is ever mistaken for a derived needle.

import (
	"io"
	"strings"
	"sync"
)

// EchoDetector builds, for a set of held values, a check that reports
// whether a text repeats one of them in any form the detector knows.
type EchoDetector func(held []string) func(text string) bool

var echoDetectors = struct {
	sync.Mutex
	builders []EchoDetector
	gen      int
}{}

// RegisterEchoDetector adds build to the checks HeldEchoes runs beside the
// funnel. The client package registers its credential masker at init.
func RegisterEchoDetector(build EchoDetector) {
	if build == nil {
		return
	}
	echoDetectors.Lock()
	echoDetectors.builders = append(echoDetectors.builders, build)
	echoDetectors.gen++
	echoDetectors.Unlock()
}

// EchoDetectorsRegistered reports how many detectors HeldEchoes consults.
func EchoDetectorsRegistered() int {
	echoDetectors.Lock()
	defer echoDetectors.Unlock()
	return len(echoDetectors.builders)
}

const (
	heldMemoMaxText  = 4 << 10
	heldMemoMaxBytes = 4 << 20
)

// heldSnapshot is everything the chokepoint searches for, for one held set.
type heldSnapshot struct {
	n, c, gen int      // held credentials, derived needles, detectors
	needles   []string // funnelNeedles over every held credential and derived needle
	detect    []func(string) bool

	mu        sync.Mutex
	memo      map[string]bool
	memoBytes int
}

var heldCache struct {
	sync.Mutex
	snap *heldSnapshot
}

// currentHeld returns the snapshot for the held set as it is now, rebuilt
// when the credentials, the derived needles or the detectors grow (each only
// grows, so the three counts identify a snapshot). No lock is held while
// another is taken, and none while a detector is built.
func currentHeld() *heldSnapshot {
	values, cores := holdEncodedCores()
	echoDetectors.Lock()
	gen := echoDetectors.gen
	builders := append([]EchoDetector(nil), echoDetectors.builders...)
	echoDetectors.Unlock()
	heldCache.Lock()
	s := heldCache.snap
	heldCache.Unlock()
	if s != nil && s.n == len(values) && s.c == len(cores) && s.gen == gen {
		return s
	}
	s = &heldSnapshot{n: len(values), c: len(cores), gen: gen,
		needles: funnelNeedles(append(append([]string(nil), values...), cores...)), memo: map[string]bool{}}
	if base := detectorValues(values); len(base) > 0 {
		for _, build := range builders {
			if d := buildDetector(build, base); d != nil {
				s.detect = append(s.detect, d)
			}
		}
	}
	heldCache.Lock()
	if old := heldCache.snap; old == nil || old.gen < s.gen || (old.gen == s.gen && old.n+old.c < s.n+s.c) {
		heldCache.snap = s
	}
	heldCache.Unlock()
	return s
}

// detectorValues is every held credential a detector is given: all of them
// the funnel searches for at all (funnelMinSecretLen and longer), with no
// exclusion. The held set holds credentials only (holdEncodedCores).
func detectorValues(values []string) []string {
	var out []string
	for _, v := range values {
		if len(v) >= funnelMinSecretLen {
			out = append(out, v)
		}
	}
	return out
}

// buildDetector fails closed: a builder that panics yields a check that
// reports every text as an echo.
func buildDetector(build EchoDetector, held []string) (d func(string) bool) {
	defer func() {
		if recover() != nil {
			d = func(string) bool { return true }
		}
	}()
	if d = build(held); d == nil {
		return nil
	}
	return d
}

func detectorFires(d func(string) bool, text string) (fired bool) {
	defer func() {
		if recover() != nil {
			fired = true
		}
	}()
	return d(text)
}

// echoes is one window of HeldEchoes: the funnel's search, then each
// registered detector.
func (s *heldSnapshot) echoes(text string) bool {
	if funnelEchoesWith(text, s.needles) {
		return true
	}
	for _, d := range s.detect {
		if detectorFires(d, text) {
			return true
		}
	}
	return false
}

func (s *heldSnapshot) recall(text string) (result, ok bool) {
	if len(text) > heldMemoMaxText {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok = s.memo[text]
	return result, ok
}

func (s *heldSnapshot) remember(text string, result bool) {
	if len(text) > heldMemoMaxText {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memoBytes+len(text) > heldMemoMaxBytes {
		s.memo, s.memoBytes = map[string]bool{}, 0
	}
	s.memo[text] = result
	s.memoBytes += len(text)
}

// funnelEchoesWith is FunnelEchoes with its needles built once per held set:
// the same breadth-first views, steps, minimum length and work cap
// (TestFunnelEchoesWithMatchesFunnelEchoes holds the two equal).
func funnelEchoesWith(text string, needles []string) bool {
	if len(needles) == 0 || text == "" {
		return false
	}
	if len(text) > funnelMaxInput {
		return true
	}
	budget := funnelWorkPerByte*len(text) + funnelWorkBase
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
		for _, step := range funnelSteps {
			budget -= len(view) + 1
			if budget < 0 {
				return true
			}
			for _, next := range step(view) {
				if len(next) < funnelMinSecretLen || seen[next] {
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

// WithholdWriter returns a writer that passes every Write through
// WithholdBytes before w (review r1 P1-C: the learn journal, the teach logs,
// the learnings audit and feedback.jsonl). Each Write must carry whole lines,
// as a JSON encoder's Encode and one Fprintf do.
func WithholdWriter(w io.Writer) io.Writer { return withholdWriter{w: w} }

type withholdWriter struct{ w io.Writer }

func (x withholdWriter) Write(p []byte) (int, error) {
	if _, err := x.w.Write(WithholdBytes(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
