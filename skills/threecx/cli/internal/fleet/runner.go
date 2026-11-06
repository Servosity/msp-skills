// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: run this CLI against one or many fleet PBXes.

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
	"sort"
	"strings"
	"sync"
	"threecx-pp-cli/internal/credkit"
	"time"

	"threecx-pp-cli/internal/cliutil"
)

// SelectEnv is the environment variable a child process reads to pick its PBX.
const SelectEnv = "THREECX_PBX"

// keptEnv are THREECX_* settings that carry no host, credential, or storage
// location and may pass into a PBX-pinned process. Every other THREECX_*
// variable is dropped, so a variable added later is ignored there rather
// than leaked from one client's phone system to another. THREECX_CA_CERT is
// the operator's extra trust anchor (an MSP's internal CA for on-premises
// phone systems): it widens which certificates verify, never where a request
// goes, and every PBX-pinned process must trust what the operator trusts. The
// read-only and no-local-data switches only ever restrict what a process does.
var keptEnv = map[string]bool{
	"THREECX_NO_LEARN": true, "THREECX_LEARN_SESSION": true,
	"THREECX_LEARN_NO_CAPTURE": true, "THREECX_LEARN_SURFACE": true,
	"THREECX_CA_CERT": true, "THREECX_MCP_READ_ONLY": true, "THREECX_NO_LOCAL_DATA": true,
}

// Scrub returns env as a process pinned to p may see it: no THREECX_*
// variable except harmless settings and p's own secret, and no other
// registered PBX's secret variable.
func Scrub(env []string, reg *Registry, p PBX) []string {
	others := map[string]bool{}
	if reg != nil {
		for _, o := range reg.PBXes {
			if o.Name != p.Name {
				others[o.SecretVar] = true
			}
		}
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if key == p.SecretVar || keptEnv[key] {
			out = append(out, kv)
			continue
		}
		if strings.HasPrefix(key, "THREECX_") || others[key] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Request is one read-only XAPI GET, path relative to /xapi/v1.
type Request struct {
	Path   string
	Params map[string]string
}

// Encode renders a request as one "path?query" argument for the child.
func (q Request) Encode() string {
	if len(q.Params) == 0 {
		return q.Path
	}
	v := url.Values{}
	for k, val := range q.Params {
		v.Set(k, val)
	}
	return q.Path + "?" + v.Encode()
}

// DecodeRequest parses an Encode()d argument.
func DecodeRequest(arg string) (Request, error) {
	path, rawQuery, _ := strings.Cut(arg, "?")
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return Request{}, fmt.Errorf("bad request %q: %w", arg, err)
	}
	q := Request{Path: path, Params: map[string]string{}}
	for k, v := range values {
		if len(v) > 0 {
			q.Params[k] = v[0]
		}
	}
	return q, nil
}

// SelfExecutable locates the binary a Runner re-executes: always this
// process's own executable. It is a variable only so tests can point it at
// a fake child; nothing else sets it, and no caller can name another binary.
var SelfExecutable = os.Executable

// Runner executes this binary once per PBX. Each child selects its PBX via
// THREECX_PBX, so it gets that PBX's FQDN, credential, and isolated token
// cache; nothing is shared between client phone systems.
type Runner struct {
	// Concurrency bounds parallel child processes (default 8).
	Concurrency int
	// Timeout bounds each request inside a child (0 = child default).
	Timeout time.Duration
	// Registry lets the runner drop every other PBX's secret from a child.
	Registry *Registry
}

// Fetch runs every request against one PBX in a single child process and
// returns one (redacted) body per request, in order.
func (r Runner) Fetch(ctx context.Context, p PBX, reqs ...Request) ([]json.RawMessage, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	args, err := childArgs(r.Timeout, reqs)
	if err != nil {
		return nil, err
	}
	self, err := SelfExecutable()
	if err != nil {
		return nil, fmt.Errorf("locating CLI binary: %w", err)
	}
	base, err := BaseDir()
	if err != nil {
		return nil, err
	}
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(len(reqs))*r.Timeout+15*time.Second)
		defer cancel()
	}
	// The child is this binary (SelfExecutable), with no shell, and every
	// argument is a fixed literal or a value childArgs validated; the PBX
	// travels in the environment as a registry-validated name.
	cmd := exec.CommandContext(ctx, self, args...) // #nosec G204 -- re-executes this CLI (os.Executable) with fixed flags and validated request paths
	// The kit hands this PBX's secret to its own binary down its held-set pipe.
	cmd.Env = append(Scrub(os.Environ(), r.Registry, p), DirEnv+"="+base, SelectEnv+"="+p.Name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := credkit.RunCommand(cmd); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("timed out: %w", ctx.Err())
		}
		return nil, errors.New(childError(stderr.String(), err))
	}
	var bodies []json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &bodies); err != nil || len(bodies) != len(reqs) {
		return nil, fmt.Errorf("unexpected child output for %d request(s)", len(reqs))
	}
	// The child already redacted; redact again so nothing that reaches a
	// report depends on the child alone (idempotent).
	for i, b := range bodies {
		if bodies[i], err = RedactJSON(b); err != nil {
			return nil, fmt.Errorf("non-JSON child output: %w", err)
		}
	}
	return bodies, nil
}

// childArgs is the child's whole argument list: fixed literals, the timeout
// as a duration, and one "--req" per request whose path is a plain XAPI path
// (leading "/", no scheme, "..", query, fragment, percent, backslash,
// whitespace or control character) and whose query is url-encoded by
// Request.Encode. The child validates every path again (cleanXAPIPath and
// its deny-list) before it sends anything.
func childArgs(timeout time.Duration, reqs []Request) ([]string, error) {
	args := []string{"fleet", "exec", "--raw"}
	if timeout > 0 {
		args = append(args, "--timeout", timeout.String())
	}
	if len(reqs) == 0 {
		return nil, errors.New("no request for the child")
	}
	for _, q := range reqs {
		if !plainXAPIPath(q.Path) {
			return nil, fmt.Errorf("refusing child request path %q", q.Path)
		}
		for k := range q.Params {
			if strings.TrimSpace(k) == "" {
				return nil, fmt.Errorf("refusing an empty query parameter name for %s", q.Path)
			}
		}
		args = append(args, "--req", q.Encode())
	}
	return append(args, "--json", "--no-input"), nil
}

func plainXAPIPath(p string) bool {
	if !strings.HasPrefix(p, "/") || len(p) > 512 || strings.Contains(p, "://") || strings.Contains(p, "..") ||
		strings.ContainsAny(p, "?#%\\ ") {
		return false
	}
	for _, c := range p {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// Each runs fn for every PBX with bounded concurrency; fn gets the PBX's
// index so callers write results into a pre-sized slice.
func Each(ctx context.Context, concurrency int, pbxes []PBX, fn func(ctx context.Context, i int, p PBX)) {
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, p := range pbxes {
		wg.Add(1)
		go func(i int, p PBX) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn(ctx, i, p)
		}(i, p)
	}
	wg.Wait()
}

// childError keeps the first meaningful stderr line, sanitised and capped.
func childError(stderr string, err error) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "Error: ")
		if line != "" && !strings.HasPrefix(line, "warning:") && !strings.HasPrefix(line, "hint:") {
			return cliutil.SanitizeErrorBody(line)
		}
	}
	return err.Error()
}

// NamesOf lists PBX names in order.
func NamesOf(pbxes []PBX) []string {
	out := make([]string, 0, len(pbxes))
	for _, p := range pbxes {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
