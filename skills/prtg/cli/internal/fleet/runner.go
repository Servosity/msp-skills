// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: run this CLI against one or many fleet cores.

package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/credkit"
)

// SelectEnv is the environment variable a child process reads to pick its core.
const SelectEnv = "PRTG_CORE"

// keptEnv are PRTG_* settings that carry no host, credential, or storage
// location and may pass into a core-pinned process. Every other PRTG_*
// variable is dropped, so a variable added later is ignored there rather
// than leaked from one client's core to another.
var keptEnv = map[string]bool{
	"PRTG_NO_LEARN": true, "PRTG_LEARN_SESSION": true,
	"PRTG_LEARN_NO_CAPTURE": true, "PRTG_LEARN_SURFACE": true,
	"PRTG_SKIP_TLS_VERIFY": true,
	// The operator's own switches: a selection must never turn them off.
	"PRTG_MCP_READ_ONLY": true, "PRTG_NO_LOCAL_DATA": true,
}

// Scrub returns env as a process pinned to c may see it: no PRTG_* variable
// except harmless settings (keptEnv), so the inherited PRTG_FLEET_KEYS, which
// carries every core's key, is dropped with the rest; then, when key is set,
// PRTG_FLEET_KEYS carrying c's own entry and no other. Names compare
// case-insensitively, as Windows reads them.
func Scrub(env []string, c Core, key string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if upper := strings.ToUpper(name); keptEnv[upper] || !strings.HasPrefix(upper, "PRTG_") {
			out = append(out, kv)
		}
	}
	if key != "" {
		out = append(out, KeysEnv+"="+KeysFor(c.Name, key))
	}
	return out
}

// ChildEnv marks a process started by Runner. The child only accepts
// write requests (POST) when it carries this marker, so the hidden raw mode
// is never a general write path for a person or an agent.
const ChildEnv = "PP_PRTG_FLEET_CHILD"

// Methods a Request may carry. GETALL is a GET that the child pages through
// with limit/offset until a short page (bounded), returning one merged array.
const (
	MethodGet    = "GET"
	MethodGetAll = "GETALL"
	MethodPost   = "POST"
)

// Request is one HTTP call against a core, path relative to /api/v2.
// Method is GET unless set; Body is a JSON request body for POST.
type Request struct {
	Method string
	Path   string
	Params map[string]string
	Body   json.RawMessage
}

// Encode renders a request as one "[METHOD ]path?query" argument for the child.
func (q Request) Encode() string {
	s := q.Path
	if len(q.Params) > 0 {
		v := url.Values{}
		for k, val := range q.Params {
			v.Set(k, val)
		}
		s += "?" + v.Encode()
	}
	if q.Method != "" && q.Method != MethodGet {
		s = q.Method + " " + s
	}
	return s
}

// DecodeRequest parses an Encode()d argument.
func DecodeRequest(arg string) (Request, error) {
	q := Request{Method: MethodGet, Params: map[string]string{}}
	if m, rest, ok := strings.Cut(arg, " "); ok {
		switch m {
		case MethodGet, MethodGetAll, MethodPost:
			q.Method, arg = m, rest
		default:
			return Request{}, fmt.Errorf("bad request %q: unsupported method %q", arg, m)
		}
	}
	path, rawQuery, _ := strings.Cut(arg, "?")
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return Request{}, fmt.Errorf("bad request %q: %w", arg, err)
	}
	q.Path = path
	for k, v := range values {
		if len(v) > 0 {
			q.Params[k] = v[0]
		}
	}
	return q, nil
}

// Runner executes this binary once per core. Each child selects its core via
// PRTG_CORE and sees only that core's PRTG_FLEET_KEYS entry, so it gets that
// core's server, API key, and isolated home; nothing is shared between
// client cores.
type Runner struct {
	// Exe is the CLI binary; empty means the running executable.
	Exe string
	// Timeout bounds each request inside a child (0 = child default).
	Timeout time.Duration
	// Insecure passes --insecure to every child (self-signed core certificates).
	Insecure bool
}

// Fetch runs every request against one core in a single child process and
// returns one (redacted) body per request, in order.
func (r Runner) Fetch(ctx context.Context, c Core, reqs ...Request) ([]json.RawMessage, error) {
	args := []string{"fleet", "exec", "--raw"}
	if r.Timeout > 0 {
		args = append(args, "--timeout", r.Timeout.String())
		// A paged GET may make up to MaxListPages calls.
		calls := 0
		for _, q := range reqs {
			if q.Method == MethodGetAll {
				calls += MaxListPages + 1 // plus the probe that proves the walk ended
			} else {
				calls++
			}
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(calls)*r.Timeout+15*time.Second)
		defer cancel()
	}
	var bodies []string
	for _, q := range reqs {
		args = append(args, "--req", q.Encode())
		bodies = append(bodies, string(q.Body))
	}
	var stdin []byte
	if hasPost(reqs) {
		// Request bodies travel on stdin, never on the command line; a POST
		// without a body sends "" in its slot.
		stdin, _ = json.Marshal(bodies)
	}
	stdout, err := r.Run(ctx, c, stdin, args...)
	if err != nil {
		return nil, err
	}
	var out []json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &out); err != nil || len(out) != len(reqs) {
		return nil, fmt.Errorf("unexpected child output for %d request(s)", len(reqs))
	}
	// The child already redacted; redact again so nothing that reaches a
	// report depends on the child alone (idempotent).
	for i, b := range out {
		if out[i], err = RedactJSON(b); err != nil {
			return nil, fmt.Errorf("non-JSON child output: %w", err)
		}
	}
	// A core that failed every request (unreachable, 401) is a core failure,
	// not a set of per-request gaps.
	if msg, all := AllFailed(out); all {
		return nil, errors.New(msg)
	}
	return out, nil
}

// MaxListPages caps a GETALL walk (3000 rows per page at most).
const MaxListPages = 20

// Run executes this CLI pinned to core c with args (plus --json --no-input
// and --insecure when set) and returns its stdout, also when the child fails
// (callers may read structured events from it). The child sees only the
// environment Scrub allows for c: c's own key, no other core's.
func (r Runner) Run(ctx context.Context, c Core, stdin []byte, args ...string) ([]byte, error) {
	exe := r.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, fmt.Errorf("locating CLI binary: %w", err)
		}
	}
	base, err := BaseDir()
	if err != nil {
		return nil, err
	}
	keys, err := Keys()
	if err != nil {
		return nil, err
	}
	// Runs for several cores share the caller's args; never append into its
	// backing array, or one core's --insecure could reach another's child.
	args = append([]string(nil), args...)
	// A core's own insecure_tls opt-in is applied by the child from the
	// registry (--core selection); Insecure carries only the caller's global
	// --insecure / PRTG_SKIP_TLS_VERIFY choice.
	if r.Insecure {
		args = append(args, "--insecure")
	}
	args = append(args, "--json", "--no-input")
	cmd := exec.CommandContext(ctx, exe, args...) // #nosec G204 -- re-executes this CLI with validated args
	cmd.Env = append(Scrub(os.Environ(), c, keys[c.Name]), DirEnv+"="+base, SelectEnv+"="+c.Name, ChildEnv+"=1")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := credkit.RunCommand(cmd); err != nil { // Hand-fix (handfixes.json credkit-wiring)
		if ctx.Err() != nil {
			return stdout.Bytes(), fmt.Errorf("timed out: %w", ctx.Err())
		}
		return stdout.Bytes(), errors.New(childError(stderr.String(), err))
	}
	return stdout.Bytes(), nil
}

// errorEnvelope is how the child reports one failed request without failing
// the whole batch, so a 403 on the license endpoint does not hide health.
type errorEnvelope struct {
	Err *ReqErr `json:"pp_error"`
}

// ReqErr is one failed request inside a child batch.
type ReqErr struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// ErrorBody renders a per-request failure for the child output.
func ErrorBody(status int, message string) json.RawMessage {
	b, _ := json.Marshal(errorEnvelope{Err: &ReqErr{Status: status, Message: message}})
	return b
}

// RequestError reports whether body is a per-request failure.
func RequestError(body json.RawMessage) (*ReqErr, bool) {
	var env errorEnvelope
	if len(body) == 0 || body[0] != '{' || json.Unmarshal(body, &env) != nil || env.Err == nil {
		return nil, false
	}
	return env.Err, true
}

// AllFailed reports whether every body is a per-request failure, with the
// first failure's message.
func AllFailed(bodies []json.RawMessage) (string, bool) {
	if len(bodies) == 0 {
		return "", false
	}
	first := ""
	for _, b := range bodies {
		e, ok := RequestError(b)
		if !ok {
			return "", false
		}
		if first == "" {
			first = e.Message
		}
	}
	return first, true
}

func hasPost(reqs []Request) bool {
	for _, q := range reqs {
		if q.Method == MethodPost {
			return true
		}
	}
	return false
}

// Each runs fn for every core with bounded concurrency; fn gets the core's
// index so callers write results into a pre-sized slice.
func Each(ctx context.Context, concurrency int, cores []Core, fn func(ctx context.Context, i int, c Core)) {
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, c := range cores {
		wg.Add(1)
		go func(i int, c Core) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn(ctx, i, c)
		}(i, c)
	}
	wg.Wait()
}

// childError keeps the child's "Error:" line, or else its first plain
// (non-JSON-event, non-warning) stderr line, sanitised and capped.
func childError(stderr string, err error) string {
	lines := strings.Split(stderr, "\n")
	for _, line := range lines {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Error: "); ok && rest != "" {
			return cliutil.SanitizeErrorBody(rest)
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "warning:") && !strings.HasPrefix(line, "hint:") {
			return cliutil.SanitizeErrorBody(line)
		}
	}
	return err.Error()
}
