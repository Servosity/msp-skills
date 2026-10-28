// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// kaseya-vsa handfix (output-chokepoint): ONE withholding function,
// WithholdText, stands in front of every output path:
//
//   - the CLI process's stdout and stderr (InstallOutputFunnel swaps both for
//     pipes and filters every line; loggers write to the swapped stderr), so
//     command output, warnings, Cobra's error line and any direct os.Stdout or
//     os.Stderr write pass it;
//   - files the CLI writes for the operator (--deliver via WithholdBytes,
//     export -o via WithholdingFile) and the local store (store package);
//   - the MCP server's tool results and errors (its result middleware) and its
//     stderr.
//
// It withholds any value the process holds (FunnelHold: the credentials,
// registered when the configuration loads, when a client is built and when
// the chokepoint is installed) in every view the shared funnel decodes (JSON
// and JavaScript escapes, percent and HTML escapes, markup, base64 and hex)
// and in the binary-to-text encodings it does not decode (ASCII85, Z85,
// base32 and base32hex), which are matched as encoded needles at every byte
// alignment. Withholding is as narrow as stays safe: a JSON string literal,
// else a whitespace token, else the whole text.

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base32"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// funnelWindow bounds one decoded search; longer texts are searched in
// overlapping windows so a large output is not withheld for its size alone.
const (
	funnelWindow  = 256 << 10
	funnelOverlap = 8 << 10
)

// expandedHeld is the chokepoint's own needles: for every held credential,
// its ASCII85, Z85 and base32 encodings at each byte alignment. They live
// here and never in the held set (FunnelHold), so the held set holds only
// credentials whatever order holds and expansions happen in (review r2 P1: a
// credential equal to another's encoding was taken for a derived needle, so
// the detectors never saw it and its own encodings were never derived).
var expandedHeld = struct {
	sync.Mutex
	upTo  int             // heldSecrets.values already expanded: FunnelHold dedups and only appends
	seen  map[string]bool // derived needles, deduplicated
	cores []string
}{seen: map[string]bool{}}

// holdEncodedCores derives the encoded needles of every held credential not
// yet expanded (the encoded run of the whole groups that lie inside the
// value, wherever it starts in a longer encoded stream) and returns the held
// credentials with every derived needle.
func holdEncodedCores() (values, cores []string) {
	heldSecrets.Lock()
	values = heldSecrets.values[:len(heldSecrets.values):len(heldSecrets.values)]
	heldSecrets.Unlock()
	expandedHeld.Lock()
	defer expandedHeld.Unlock()
	for _, v := range values[min(expandedHeld.upTo, len(values)):] {
		for _, core := range encodedCores(v) {
			if !expandedHeld.seen[core] {
				expandedHeld.seen[core] = true
				expandedHeld.cores = append(expandedHeld.cores, core)
			}
		}
	}
	expandedHeld.upTo = max(expandedHeld.upTo, len(values))
	return values, expandedHeld.cores[:len(expandedHeld.cores):len(expandedHeld.cores)]
}

var (
	b32Encodings = []*base32.Encoding{
		base32.StdEncoding.WithPadding(base32.NoPadding),
		base32.HexEncoding.WithPadding(base32.NoPadding),
	}
	z85Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"
)

func encodedCores(v string) []string {
	if len(v) < funnelMinSecretLen {
		return nil
	}
	var out []string
	for a := 0; a < 4; a++ { // 4-byte groups: ASCII85 and Z85
		core := []byte(v[min(a, len(v)):])
		core = core[:len(core)-len(core)%4]
		if len(core) < 8 {
			continue
		}
		buf := make([]byte, ascii85.MaxEncodedLen(len(core)))
		out = append(out, string(buf[:ascii85.Encode(buf, core)]), z85(core))
	}
	for a := 0; a < 5; a++ { // 5-byte groups: base32 and base32hex
		core := []byte(v[min(a, len(v)):])
		core = core[:len(core)-len(core)%5]
		if len(core) < 10 {
			continue
		}
		for _, enc := range b32Encodings {
			s := enc.EncodeToString(core)
			out = append(out, s, strings.ToLower(s))
		}
	}
	return out
}

func z85(b []byte) string {
	var sb strings.Builder
	for i := 0; i+4 <= len(b); i += 4 {
		n := uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
		var g [5]byte
		for j := 4; j >= 0; j-- {
			g[j] = z85Alphabet[n%85]
			n /= 85
		}
		sb.Write(g[:])
	}
	return sb.String()
}

// HeldEchoes reports whether text repeats a held value in any form above, or
// in any form a registered detector knows: the client registers its own
// credential masker, so every encoding the client masks in its errors is
// withheld here too (kaseya_held.go, review r1 P1-B).
func HeldEchoes(text string) bool {
	if text == "" {
		return false
	}
	h := currentHeld()
	if len(h.needles) == 0 && len(h.detect) == 0 {
		return false
	}
	if result, ok := h.recall(text); ok {
		return result
	}
	result := false
	if len(text) <= funnelWindow {
		result = h.echoes(text)
	} else {
		for start := 0; start < len(text); start += funnelWindow - funnelOverlap {
			end := min(start+funnelWindow, len(text))
			if h.echoes(text[start:end]) {
				result = true
				break
			}
			if end == len(text) {
				break
			}
		}
	}
	h.remember(text, result)
	return result
}

var (
	jsonStringLiteral = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`)
	nonSpaceRun       = regexp.MustCompile(`\S+`)
)

// WithholdText is the chokepoint: text unchanged when it repeats no held
// value; otherwise each JSON string literal, then each whitespace token, that
// repeats one is replaced by the funnel's notice, and when that is not enough
// the whole text is.
func WithholdText(text string) string {
	if !HeldEchoes(text) {
		return text
	}
	out := jsonStringLiteral.ReplaceAllStringFunc(text, func(lit string) string {
		if HeldEchoes(lit) {
			return `"` + FunnelWithheldText + `"`
		}
		return lit
	})
	if !HeldEchoes(out) {
		return out
	}
	out = nonSpaceRun.ReplaceAllStringFunc(out, func(tok string) string {
		if HeldEchoes(tok) {
			return FunnelWithheldText
		}
		return tok
	})
	if !HeldEchoes(out) {
		return out
	}
	if funnelReadsAsAuth(text) {
		return FunnelWithheldAuthText
	}
	return FunnelWithheldText
}

// WithholdBytes applies WithholdText line by line, then to the whole result.
func WithholdBytes(b []byte) []byte {
	if !HeldEchoes(string(b)) {
		return b
	}
	var out bytes.Buffer
	for _, line := range bytes.SplitAfter(b, []byte("\n")) {
		out.WriteString(withholdLine(string(line)))
	}
	if HeldEchoes(out.String()) {
		return []byte(FunnelWithheldText + "\n")
	}
	return out.Bytes()
}

func withholdLine(line string) string {
	body := strings.TrimRight(line, "\r\n")
	return WithholdText(body) + line[len(body):]
}

// lineFilter receives a stream and writes it to dst line by line through
// WithholdText. A partial line is written once the stream has been idle for
// filterIdle, so a prompt without a newline still appears.
type lineFilter struct {
	mu      sync.Mutex
	dst     io.Writer
	pending []byte
	gen     int
}

const filterIdle = 30 * time.Millisecond

var filters sync.WaitGroup

func (f *lineFilter) feed(p []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, p...)
	for {
		i := bytes.IndexByte(f.pending, '\n')
		if i < 0 {
			break
		}
		_, _ = io.WriteString(f.dst, withholdLine(string(f.pending[:i+1])))
		f.pending = f.pending[i+1:]
	}
	f.gen++
	if len(f.pending) > 0 {
		gen := f.gen
		time.AfterFunc(filterIdle, func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.gen == gen {
				f.flushLocked()
			}
		})
	}
}

func (f *lineFilter) flushLocked() {
	if len(f.pending) > 0 {
		_, _ = io.WriteString(f.dst, WithholdText(string(f.pending)))
		f.pending = nil
	}
}

// filterPipe returns the write end of a pipe whose every byte reaches dst
// through the chokepoint; closeDst also closes dst once the stream ends.
func filterPipe(dst io.Writer, closeDst bool) (*os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	f := &lineFilter{dst: dst}
	filters.Add(1)
	go func() {
		defer filters.Done()
		defer r.Close()
		buf := make([]byte, 64<<10)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				f.feed(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
		f.mu.Lock()
		f.gen++
		f.flushLocked()
		f.mu.Unlock()
		if c, ok := dst.(io.Closer); ok && closeDst {
			_ = c.Close()
		}
	}()
	return w, nil
}

// InstallOutputFunnel routes os.Stdout, os.Stderr and the standard logger
// through the chokepoint and reports whether each original stream was a
// terminal (so output format choices do not change). restore puts the
// streams back and waits until everything written, export files included,
// has been delivered.
func InstallOutputFunnel() (restore func(), outTTY, errTTY bool, err error) {
	origOut, origErr := os.Stdout, os.Stderr
	outTTY, errTTY = isCharDevice(origOut), isCharDevice(origErr)
	wOut, err := filterPipe(origOut, false)
	if err != nil {
		return func() {}, outTTY, errTTY, err
	}
	wErr, err := filterPipe(origErr, false)
	if err != nil {
		_ = wOut.Close()
		filters.Wait()
		return func() {}, outTTY, errTTY, err
	}
	os.Stdout, os.Stderr = wOut, wErr
	log.SetOutput(wErr)
	var once sync.Once
	return func() {
		once.Do(func() {
			os.Stdout, os.Stderr = origOut, origErr
			log.SetOutput(origErr)
			_ = wOut.Close()
			_ = wErr.Close()
			filters.Wait()
		})
	}, outTTY, errTTY, nil
}

// InstallStderrFunnel routes os.Stderr and the standard logger through the
// chokepoint for the life of the process (the MCP server, whose stdout is the
// protocol channel and whose results pass withholdToolResults).
func InstallStderrFunnel() error {
	w, err := filterPipe(os.Stderr, false)
	if err != nil {
		return err
	}
	os.Stderr = w
	log.SetOutput(w)
	return nil
}

// WithholdingFile wraps a file the program is about to write: the caller
// writes and closes the returned pipe; the chokepoint writes the real file and
// closes it, and InstallOutputFunnel's restore waits for that.
func WithholdingFile(f *os.File, err error) (*os.File, error) {
	if err != nil || f == nil {
		return f, err
	}
	w, perr := filterPipe(f, true)
	if perr != nil {
		_ = f.Close()
		return nil, perr
	}
	return w, nil
}

// WaitOutput blocks until every filtered stream and file has been delivered.
func WaitOutput() { filters.Wait() }

func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// TTYWriter writes to W and answers the CLI's terminal check with TTY, the
// state of the stream the chokepoint replaced.
type TTYWriter struct {
	W   io.Writer
	TTY bool
}

func (t TTYWriter) Write(p []byte) (int, error) { return t.W.Write(p) }

// TerminalOverride is read by the CLI's isTerminal.
func (t TTYWriter) TerminalOverride() bool { return t.TTY }
