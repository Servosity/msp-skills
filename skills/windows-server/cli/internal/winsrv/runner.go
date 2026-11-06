// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (fixed-scripts).

package winsrv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"windows-server-pp-cli/internal/winrm"
)

// Runner runs one registered script on one server and returns the envelope's
// data.
type Runner interface {
	Run(ctx context.Context, p Profile, script string, args map[string]any) (json.RawMessage, error)
}

// Error kinds a script reports.
const (
	KindNotFound     = "not_found"
	KindAccessDenied = "access_denied"
	KindUnavailable  = "unavailable"
	KindInvalidArg   = "invalid_argument"
	KindRoleAbsent   = "role_absent"
	KindRefused      = "refused"
	KindError        = "error"
)

// ScriptError is a structured failure reported by a script envelope.
type ScriptError struct {
	Script  string
	Server  string
	Kind    string
	Message string
}

func (e *ScriptError) Error() string {
	return fmt.Sprintf("%s on %s: %s: %s", e.Script, e.Server, e.Kind, e.Message)
}

// IsKind reports whether err is a ScriptError of kind.
func IsKind(err error, kind string) bool {
	var se *ScriptError
	return errors.As(err, &se) && se.Kind == kind
}

// ErrNotConfirmed is returned for a mutating script run without confirmation.
var ErrNotConfirmed = errors.New("refused: this action changes the server and runs only after preview and explicit confirmation")

type confirmKey struct{}

// WithConfirmedMutation marks ctx as carrying the operator's explicit
// confirmation. Only the confirm-gated action commands call it.
func WithConfirmedMutation(ctx context.Context) context.Context {
	return context.WithValue(ctx, confirmKey{}, true)
}

func confirmedMutation(ctx context.Context) bool {
	v, _ := ctx.Value(confirmKey{}).(bool)
	return v
}

// run is one validated script run: the constant command line and script
// payload, and the arguments as JSON data.
type run struct {
	script Script
	exe    string
	argv   []string
	env    map[string]string
	args   string // the JSON arguments, for fixtures
}

// prepare validates a run and returns what to send.
func prepare(ctx context.Context, name string, args map[string]any) (run, error) {
	s, ok := Lookup(name)
	if !ok {
		return run{}, fmt.Errorf("unknown script %q", name)
	}
	vargs, err := s.Validate(args)
	if err != nil {
		return run{}, err
	}
	if s.Mutating && !confirmedMutation(ctx) {
		return run{}, ErrNotConfirmed
	}
	exe, argv, text, err := CommandLine(name)
	if err != nil {
		return run{}, err
	}
	payload, err := json.Marshal(vargs)
	if err != nil {
		return run{}, err
	}
	env := map[string]string{ArgsEnv: base64.StdEncoding.EncodeToString(payload), ScriptEnv: text}
	return run{script: s, exe: exe, argv: argv, env: env, args: string(payload)}, nil
}

// WinRMRunner runs scripts over WinRM.
type WinRMRunner struct {
	AllowHTTP bool          // the CLI-only --allow-http flag
	Timeout   time.Duration // overrides each script's default budget when set
}

// Run implements Runner.
func (r WinRMRunner) Run(ctx context.Context, p Profile, name string, args map[string]any) (json.RawMessage, error) {
	rn, err := prepare(ctx, name, args)
	if err != nil {
		return nil, err
	}
	budget := rn.script.Timeout
	if r.Timeout > 0 {
		budget = r.Timeout
	}
	shown := budget // what the timeout message names: the budget, or a shorter caller deadline
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl); left < shown {
			shown = left.Round(time.Second)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	c, err := winrm.New(p.Endpoint(r.AllowHTTP), winrm.Auth{Method: p.Auth, Username: p.Username, Password: p.Password}, budget+30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("server %s: %w", p.Name, err)
	}
	res, err := c.Run(ctx, rn.exe, rn.argv, rn.env)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s on %s: no answer within %s (raise --timeout)", name, p.Name, shown)
		}
		return nil, fmt.Errorf("%s on %s: %w", name, p.Name, err)
	}
	return Decode(name, p.Name, res.Stdout, res.Stderr, res.ExitCode)
}

var envelopeRE = regexp.MustCompile(`\{\s*"ok"\s*:`)

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
}

// Decode finds the script's envelope in stdout and returns its data. Stray text before the envelope (a module writing to the
// host while loading) is ignored.
func Decode(script, server string, stdout, stderr []byte, exit int) (json.RawMessage, error) {
	locs := envelopeRE.FindAllIndex(stdout, -1)
	if len(locs) == 0 {
		msg := cleanStderr(string(stderr))
		if msg == "" {
			msg = "no output"
		}
		return nil, &ScriptError{Script: script, Server: server, Kind: KindError, Message: fmt.Sprintf("PowerShell exited %d without a result: %s", exit, msg)}
	}
	var env envelope
	tail := stdout[locs[len(locs)-1][0]:]
	dec := json.NewDecoder(bytes.NewReader(tail))
	if err := dec.Decode(&env); err != nil {
		return nil, &ScriptError{Script: script, Server: server, Kind: KindError, Message: "unreadable result: " + err.Error()}
	}
	if len(bytes.TrimSpace(tail[dec.InputOffset():])) != 0 {
		return nil, &ScriptError{Script: script, Server: server, Kind: KindError, Message: "unreadable result: output continued after the result"}
	}
	if !env.OK {
		se := &ScriptError{Script: script, Server: server, Kind: KindError, Message: "script failed without detail"}
		if env.Error != nil {
			if env.Error.Kind != "" {
				se.Kind = env.Error.Kind
			}
			se.Message = strings.TrimSpace(env.Error.Message)
		}
		return nil, se
	}
	if len(bytes.TrimSpace(env.Data)) == 0 {
		return json.RawMessage("null"), nil
	}
	return env.Data, nil
}

// cleanStderr strips PowerShell CLIXML noise and bounds the length.
func cleanStderr(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#< CLIXML") {
		var msgs []string
		rest := s
		for {
			i := strings.Index(rest, `<S S="Error">`)
			if i < 0 {
				break
			}
			rest = rest[i+len(`<S S="Error">`):]
			j := strings.Index(rest, "</S>")
			if j < 0 {
				break
			}
			msgs = append(msgs, rest[:j])
			rest = rest[j:]
		}
		s = strings.ReplaceAll(strings.Join(msgs, ""), "_x000D__x000A_", " ")
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 1000 {
		s = s[:1000] + "..."
	}
	return s
}

// FixtureRunner answers from recorded script stdout, for tests and offline
// demos. It validates arguments and the confirmation gate exactly like
// WinRMRunner and passes output through the same Decode and filter.
type FixtureRunner struct {
	mu      sync.Mutex
	Outputs map[string]string // "server/script" or "script" -> raw stdout
	Calls   []FixtureCall
}

// FixtureCall records one Run.
type FixtureCall struct {
	Server string
	Script string
	Args   string
}

// Run implements Runner.
func (f *FixtureRunner) Run(ctx context.Context, p Profile, name string, args map[string]any) (json.RawMessage, error) {
	rn, err := prepare(ctx, name, args)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.Calls = append(f.Calls, FixtureCall{Server: p.Name, Script: name, Args: rn.args})
	out, ok := f.Outputs[p.Name+"/"+name]
	if !ok {
		out, ok = f.Outputs[name]
	}
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s on %s: no fixture", name, p.Name)
	}
	return Decode(name, p.Name, []byte(out), nil, 0)
}
