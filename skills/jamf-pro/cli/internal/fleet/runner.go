// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: run this CLI against one or many fleet instances.

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
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"jamf-pro-pp-cli/internal/credkit"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// SelectEnv is the environment variable a child process reads to pick its instance.
const SelectEnv = jamfauth.SelectEnv

// keptEnv are JAMF_PRO_* settings that carry no host, credential, or storage
// location and may pass into an instance-pinned process (upper-cased).
var keptEnv = map[string]bool{
	"JAMF_PRO_NO_LEARN": true, "JAMF_PRO_LEARN_SESSION": true,
	"JAMF_PRO_LEARN_NO_CAPTURE": true, "JAMF_PRO_LEARN_SURFACE": true,
	// The operator's TLS trust settings (a path to a PEM bundle and a
	// certificate name, never a secret): without them a child cannot reach
	// an on-prem instance behind a private CA.
	"JAMF_PRO_CA_CERT": true, "JAMF_PRO_TLS_SERVER_NAME": true,
}

// RestrictionEnv are the restriction and opt-out settings the CLI and MCP
// server read (upper-cased). Pinning and fleet children keep every one of
// them with its value, so a pinned or child process is always at least as
// restricted as its parent: read-only MCP, learning and capture opt-outs,
// preseed and colour opt-outs, harness modes and a bound client profile.
// (Opt-ins such as JAMF_PRO_FEEDBACK_AUTO_SEND are dropped, which can only
// restrict further.)
var RestrictionEnv = map[string]bool{
	"JAMF_PRO_MCP_READ_ONLY": true, "JAMF_PRO_NO_LEARN": true, "JAMF_PRO_LEARN_NO_CAPTURE": true, "JAMF_PRO_NO_LOCAL_DATA": true,
	"PRESEED_DISABLED": true, "NO_COLOR": true, "PRINTING_PRESS_VERIFY": true, "PRINTING_PRESS_DOGFOOD": true,
	"PRINTING_PRESS_MCP_BOUND_PROFILE": true, "PRINTING_PRESS_CLIENT_PROFILE": true,
}

// childEnvAllow is the whole environment a fleet child gets besides keptEnv,
// its own secret and the variables the runner sets (upper-cased; LC_* and
// XDG_* by prefix). Everything else is dropped, whatever it is called.
var childEnvAllow = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "TZ": true, "LANG": true,
	"TMPDIR": true, "TEMP": true, "TMP": true,
	"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true,
	"USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true, "HOMEDRIVE": true, "HOMEPATH": true,
	"SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "HTTPS_PROXY": true, "HTTP_PROXY": true, "NO_PROXY": true,
}

// ChildEnv builds a fleet child's environment from an allowlist, comparing
// names case-insensitively (Windows names are): no variable reaches the
// child unless it is listed. The runner adds p's own secret entry.
func ChildEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(key)
		if childEnvAllow[up] || keptEnv[up] || RestrictionEnv[up] || strings.HasPrefix(up, "LC_") || strings.HasPrefix(up, "XDG_") {
			out = append(out, kv)
		}
	}
	return out
}

// Scrub returns env as a process pinned to an instance keeps it for itself: no
// JAMF_PRO_* variable in any casing except harmless settings, so the inherited
// JAMF_PRO_FLEET_SECRETS, which carries every instance's secret, is dropped with
// the rest; the caller puts back the instance's own entry. (A child of that
// process gets ChildEnv instead.)
func Scrub(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(key)
		if keptEnv[up] || RestrictionEnv[up] || !strings.HasPrefix(up, "JAMF_PRO_") {
			out = append(out, kv)
		}
	}
	return out
}

// Request is one read-only GET, path relative to the instance root
// (/api/... or /JSSResource/...). Query keeps repeated keys, which the Jamf
// Pro API uses for section=GENERAL&section=HARDWARE.
type Request struct {
	Path  string
	Query url.Values
}

// Encode renders a request as one "path?query" argument for the child.
func (q Request) Encode() string {
	if len(q.Query) == 0 {
		return q.Path
	}
	return q.Path + "?" + q.Query.Encode()
}

// DecodeRequest parses an Encode()d argument.
func DecodeRequest(arg string) (Request, error) {
	path, rawQuery, _ := strings.Cut(arg, "?")
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return Request{}, fmt.Errorf("bad request %q: %w", arg, err)
	}
	return Request{Path: path, Query: values}, nil
}

// Runner executes this binary once per instance. Each child selects its
// instance via JAMF_PRO_INSTANCE, so it gets that instance's server,
// credential, and isolated token cache; nothing is shared between clients.
type Runner struct {
	// Exe is the CLI binary; empty means the running executable.
	Exe string
	// Concurrency bounds parallel child processes (default 8).
	Concurrency int
	// Timeout bounds each request inside a child (0 = child default).
	Timeout time.Duration
	// Registry lets the runner drop every other instance's secret from a child.
	Registry *Registry
	// FreshToken makes the child mint a new token instead of reusing a cached
	// one, so a credential probe proves the current secret.
	FreshToken bool
}

// Fetch runs every request against one instance in a single child process
// and returns one (redacted) body per request, in order.
func (r Runner) Fetch(ctx context.Context, p Instance, reqs ...Request) ([]json.RawMessage, error) {
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
	args := []string{"fleet", "exec", "--raw"}
	// No overall deadline here: the child bounds every HTTP call with
	// --timeout and every request with a page cap, so it cannot hang.
	if r.Timeout > 0 {
		args = append(args, "--timeout", r.Timeout.String())
	}
	if r.FreshToken {
		args = append(args, "--fresh-token")
	}
	for _, q := range reqs {
		args = append(args, "--req", q.Encode())
	}
	args = append(args, "--json", "--no-input")
	cmd := exec.CommandContext(ctx, exe, args...) // #nosec G204 -- re-executes this CLI with validated args
	// The child acts as exactly this identity, handed over whole: it never
	// looks the name up again, so a registry edit after this point cannot
	// redirect it (it only refuses, see HoldInstance).
	id, err := jamfauth.ForInstance(p.Name, p.Server, p.ClientID)
	if err != nil {
		return nil, err
	}
	secret, err := SecretFor(p.Name)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(ChildEnv(os.Environ()), DirEnv+"="+base, SelectEnv+"="+p.Name, jamfauth.IdentityEnv+"="+id.Encode())
	if secret != "" {
		// The child sees its own entry and no other instance's secret.
		cmd.Env = append(cmd.Env, EntriesEnv+"="+SecretsFor(p.Name, secret))
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := credkit.RunCommand(cmd); err != nil { // Hand-fix (handfixes.json credkit-wiring): this CLI re-executing itself; the kit carries the instance secret down
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

// Each runs fn for every instance with bounded concurrency; fn gets the
// instance's index so callers write results into a pre-sized slice.
func Each(ctx context.Context, concurrency int, instances []Instance, fn func(ctx context.Context, i int, p Instance)) {
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, p := range instances {
		wg.Add(1)
		go func(i int, p Instance) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn(ctx, i, p)
		}(i, p)
	}
	wg.Wait()
}

// childError keeps the child's first meaningful stderr line, deny by
// default: anything after "HTTP <status>" (a quoted upstream body) is cut,
// except the token-error form {"error":"<code>"} for one of the six RFC 6749
// codes, and a line the redaction funnel would change is withheld whole.
func childError(stderr string, err error) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "Error: ")
		if line == "" || strings.HasPrefix(line, "warning:") || strings.HasPrefix(line, "hint:") || strings.HasPrefix(line, "{") {
			continue
		}
		// The transport's own fixed messages name the failure kind (an
		// unreachable server, a timeout, a refused redirect). Go wraps them
		// with the request URL ("Post \"https://host/.../oauth/token\": ..."),
		// which the credential check would mistake for a credential; return
		// our constant text instead of withholding it.
		for _, m := range TransportMessages {
			if strings.Contains(line, m) {
				return m
			}
		}
		if m := httpStatus.FindStringIndex(line); m != nil {
			rest := line[m[1]:]
			line = line[:m[1]]
			if code, ok := strings.CutPrefix(rest, `: {"error":"`); ok && strings.HasSuffix(code, `"}`) && jamfauth.OAuthErrorCodes[strings.TrimSuffix(code, `"}`)] {
				line += rest
			}
		}
		if RedactString(line) != line {
			return "error details withheld (they matched a credential pattern)"
		}
		return line
	}
	return err.Error()
}

// TransportMessages are the fixed messages the CLI's transport returns
// (cli/jamf_token_auth.go); none holds a server-sent value.
var TransportMessages = []string{
	"could not connect to the server", "could not resolve the server host", "request timed out", "request canceled",
	"the server's TLS certificate is not trusted", "redirects are refused", "reading the response failed",
	"the response is too large", "the decoded response is too large", "the response could not be decoded",
	"unsupported content encoding", "HTML page where an API response was expected", "response is not valid JSON",
	"token response is not valid", "token request failed",
}

var (
	httpStatus = regexp.MustCompile(`HTTP [1-5][0-9]{2}`)
)

// NamesOf lists instance names in order.
func NamesOf(instances []Instance) []string {
	out := make([]string, 0, len(instances))
	for _, p := range instances {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
