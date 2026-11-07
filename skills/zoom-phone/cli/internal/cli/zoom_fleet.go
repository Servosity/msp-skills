// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto
// pp:client-call -- fleet reaches the API only through child zoom-phone-cli
// processes, one per saved client account.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"zoom-phone-pp-cli/internal/credkit"
)

const fleetStderrTailBytes = 2048

// Access classes for the command fleet fans out.
const (
	fleetAccessReadOnly   = "read-only"
	fleetAccessLocalWrite = "local-write"
	fleetAccessWrite      = "write"
)

// fleetExcludedTopLevel are command families fleet refuses to fan out:
// itself (recursion), accounts (profile management is local, not per
// tenant) and auth (would rewrite each account's credentials).
var fleetExcludedTopLevel = map[string]string{
	"fleet":    "fleet cannot run itself",
	"accounts": "accounts commands manage local profiles, not per-account data",
	"auth":     "auth commands would rewrite each account's credentials",
}

// fleetFrameworkReadOnly are generated top-level commands with no
// mcp:read-only annotation that only inspect state. doctor is the natural
// fleet health check (does every client's S2S app still mint a token?).
var fleetFrameworkReadOnly = map[string]bool{
	"doctor":  true,
	"version": true,
}

// fleetLocalStoreCommands are generated top-level commands whose only writes
// land in the account's own local store (inside its account home).
var fleetLocalStoreCommands = map[string]bool{
	"sync": true,
}

// fleetSingleAccountCommands may only run with --accounts naming exactly one
// account (and --allow-writes, since they are writes).
var fleetSingleAccountCommands = map[string]string{
	"offboard": "it removes one person's assignments, and that person belongs to one client account",
}

// fleetRefusedFlags choose another config, home or run profile, or a local
// file destination; in a per-account child they would break isolation or
// write outside the account. Flags a command lists in mcp:write-flags (local
// file inputs) are refused too.
var fleetRefusedFlags = map[string]string{
	"config":        "fleet sets the config file per account",
	"home":          "fleet sets the home directory per account",
	"profile":       "run profiles are not available inside per-account runs",
	"db":            "each account uses its own local store",
	"output":        "it writes a local file",
	"receipt-file":  "it writes a local file",
	"audit-dir":     "it writes local files",
	"deliver":       "it sends output to a file or URL; deliver fleet's merged output instead (zoom-phone-cli --deliver ... fleet -- ...)",
	"legacy-config": "it moves local files",
	"legacy-db":     "it moves local files",
}

type fleetChildSpec struct {
	Account string
	Path    string
	Args    []string
	Env     []string
}

type fleetChildOutcome struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

// fleetSpawn runs one child. Tests replace it to assert argv/env without
// executing a process.
var fleetSpawn = fleetExecChild

func fleetExecChild(ctx context.Context, spec fleetChildSpec) fleetChildOutcome {
	c := exec.CommandContext(ctx, spec.Path, spec.Args...) // #nosec G204 -- the CLI's own executable; args were resolved against the Cobra tree.
	c.Env = spec.Env
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := credkit.RunCommand(c) // credkit-wiring
	out := fleetChildOutcome{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			out.ExitCode = exitErr.ExitCode()
			if out.ExitCode == 0 {
				out.ExitCode = -1
			}
			return out
		}
		out.ExitCode = -1
		out.Err = err
	}
	return out
}

type fleetResult struct {
	Account   string `json:"account"`
	AccountID string `json:"account_id"`
	ExitCode  int    `json:"exit_code"`
	OK        bool   `json:"ok"`
	Result    any    `json:"result"`
	Stderr    string `json:"stderr"`
}

var fleetResultKeys = []string{"account", "account_id", "exit_code", "ok", "result", "stderr"}

type fleetPlanItem struct {
	Account   string   `json:"account"`
	AccountID string   `json:"account_id"`
	Config    string   `json:"config"`
	Args      []string `json:"args"`
}

type fleetPlan struct {
	DryRun   bool            `json:"dry_run"`
	Command  string          `json:"command"`
	ReadOnly bool            `json:"read_only"`
	Access   string          `json:"access"`
	Accounts []fleetPlanItem `json:"accounts"`
	Note     string          `json:"note,omitempty"`
}

// fleetTarget is the resolved child command and its parsed arguments.
type fleetTarget struct {
	cmd    *cobra.Command
	top    string
	access string
	parsed fleetParsedArgs
}

// fleetParsedFlag is one flag the child's own parser will see.
type fleetParsedFlag struct {
	Name  string
	Value string
}

// fleetParsedArgs is the child's argv as pflag will parse it.
type fleetParsedArgs struct {
	Flags []fleetParsedFlag
	// Terminator is the index of the "--" that really ends flag parsing (one
	// not consumed as a flag value), or -1.
	Terminator int
}

func (p fleetParsedArgs) has(name string) bool {
	for _, f := range p.Flags {
		if f.Name == name {
			return true
		}
	}
	return false
}

func fleetAnnotationTrue(cmd *cobra.Command, key string) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(cmd.Annotations[key]))
	return v == "true" || v == "1" || v == "yes"
}

// fleetCommandAccess classifies a resolved command.
func fleetCommandAccess(root, target *cobra.Command) string {
	topLevel := target.Parent() == root
	if fleetAnnotationTrue(target, "mcp:read-only") || (topLevel && fleetFrameworkReadOnly[target.Name()]) {
		return fleetAccessReadOnly
	}
	if (topLevel && fleetLocalStoreCommands[target.Name()]) || fleetAnnotationTrue(target, "mcp:local-write") {
		return fleetAccessLocalWrite
	}
	return fleetAccessWrite
}

func fleetAnnotationFlagNames(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimPrefix(strings.TrimSpace(f), "--"); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func fleetLookupFlag(target *cobra.Command, name string) *pflag.Flag {
	if f := target.Flags().Lookup(name); f != nil {
		return f
	}
	return target.InheritedFlags().Lookup(name)
}

func fleetLookupShorthand(target *cobra.Command, c byte) *pflag.Flag {
	if c >= utf8.RuneSelf {
		return nil
	}
	if f := target.Flags().ShorthandLookup(string(c)); f != nil {
		return f
	}
	return target.InheritedFlags().ShorthandLookup(string(c))
}

// fleetParseChildArgs walks args exactly as pflag will in the child, using the
// resolved command's own flag definitions: a non-boolean flag consumes the
// next argument as its value even when that argument is "--" or looks like a
// flag (`--site-id -- --config=x` sets site-id to "--" and then parses
// --config). Unknown flags and missing values are refused here, since the
// child would fail on them anyway.
func fleetParseChildArgs(target *cobra.Command, args []string) (fleetParsedArgs, error) {
	target.InitDefaultHelpFlag()
	out := fleetParsedArgs{Terminator: -1}
	path := target.CommandPath()
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) < 2 || a[0] != '-' {
			continue // positional (pflag treats "-" alone as positional too)
		}
		if a == "--" {
			out.Terminator = i
			return out, nil
		}
		if a[1] == '-' {
			body := a[2:]
			if body == "" || body[0] == '-' || body[0] == '=' {
				return out, usageErr(fmt.Errorf("fleet: bad flag syntax %q for %q", a, path))
			}
			name, value, inline := strings.Cut(body, "=")
			f := fleetLookupFlag(target, name)
			if f == nil {
				return out, usageErr(fmt.Errorf("fleet: unknown flag --%s for %q", name, path))
			}
			if !inline {
				if f.NoOptDefVal != "" {
					value = f.NoOptDefVal
				} else {
					if i+1 >= len(args) {
						return out, usageErr(fmt.Errorf("fleet: flag --%s for %q needs a value", name, path))
					}
					i++
					value = args[i]
				}
			}
			out.Flags = append(out.Flags, fleetParsedFlag{Name: f.Name, Value: value})
			continue
		}
		for shorts := a[1:]; len(shorts) > 0; {
			f := fleetLookupShorthand(target, shorts[0])
			if f == nil {
				return out, usageErr(fmt.Errorf("fleet: unknown shorthand flag %q in %s for %q", shorts[0], a, path))
			}
			var value string
			switch {
			case len(shorts) > 2 && shorts[1] == '=':
				value, shorts = shorts[2:], ""
			case f.NoOptDefVal != "":
				value, shorts = f.NoOptDefVal, shorts[1:]
			case len(shorts) > 1:
				value, shorts = shorts[1:], ""
			case i+1 < len(args):
				i++
				value, shorts = args[i], ""
			default:
				return out, usageErr(fmt.Errorf("fleet: flag -%c for %q needs a value", shorts[0], path))
			}
			out.Flags = append(out.Flags, fleetParsedFlag{Name: f.Name, Value: value})
		}
	}
	return out, nil
}

// fleetCheckChildFlags refuses parsed flags that would break per-account
// isolation or touch local files, including any flag the command (or an
// ancestor) lists in mcp:write-flags.
func fleetCheckChildFlags(target *cobra.Command, parsed fleetParsedArgs) error {
	refused := make(map[string]string, len(fleetRefusedFlags))
	for name, reason := range fleetRefusedFlags {
		refused[name] = reason
	}
	for c := target; c != nil; c = c.Parent() {
		for _, name := range fleetAnnotationFlagNames(c.Annotations["mcp:write-flags"]) {
			refused[name] = "it reads or writes a local file"
		}
	}
	for _, f := range parsed.Flags {
		if reason, bad := refused[f.Name]; bad {
			return usageErr(fmt.Errorf("fleet refuses --%s in the per-account command: %s", f.Name, reason))
		}
	}
	return nil
}

// fleetResolveTarget validates the child command against the Cobra tree.
func fleetResolveTarget(root *cobra.Command, args []string, allowWrites bool) (fleetTarget, error) {
	if len(args) == 0 {
		return fleetTarget{}, usageErr(errors.New("fleet needs a command to run, e.g. zoom-phone-cli fleet -- doctor"))
	}
	target, rest, err := root.Find(args)
	if err != nil {
		return fleetTarget{}, usageErr(fmt.Errorf("fleet: %w", err))
	}
	if target == nil || target == root {
		return fleetTarget{}, usageErr(fmt.Errorf("fleet: %q is not a zoom-phone-cli command", args[0]))
	}
	if target.HasAvailableSubCommands() && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		return fleetTarget{}, usageErr(fmt.Errorf("fleet: unknown command %q for %q", rest[0], target.CommandPath()))
	}
	top := target
	for top.Parent() != nil && top.Parent() != root {
		top = top.Parent()
	}
	if reason, excluded := fleetExcludedTopLevel[top.Name()]; excluded {
		return fleetTarget{}, usageErr(fmt.Errorf("fleet cannot run %q: %s", target.CommandPath(), reason))
	}
	// Refused flags parsed before a parse error are reported first, in argv order.
	parsed, parseErr := fleetParseChildArgs(target, args)
	if err := fleetCheckChildFlags(target, parsed); err != nil {
		return fleetTarget{}, err
	}
	if parseErr != nil {
		return fleetTarget{}, parseErr
	}
	access := fleetCommandAccess(root, target)
	if access == fleetAccessWrite && !allowWrites {
		return fleetTarget{}, usageErr(fmt.Errorf("fleet refuses %q: it writes or returns secrets (it is not marked read-only) and would run against every selected client account; pass --allow-writes to run it anyway", target.CommandPath()))
	}
	return fleetTarget{cmd: target, top: top.Name(), access: access, parsed: parsed}, nil
}

// fleetParseAccountNames splits --accounts, trimming and de-duplicating.
func fleetParseAccountNames(csv string) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, raw := range strings.Split(csv, ",") {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// fleetCheckSingleAccount enforces fleetSingleAccountCommands.
func fleetCheckSingleAccount(t fleetTarget, accountsCSV string) error {
	reason, single := fleetSingleAccountCommands[t.top]
	if !single {
		return nil
	}
	if n := len(fleetParseAccountNames(accountsCSV)); n != 1 {
		return usageErr(fmt.Errorf("fleet runs %q only with --accounts naming exactly one account (got %d): %s", t.cmd.CommandPath(), n, reason))
	}
	return nil
}

// fleetChildArgs appends --json unless the parsed command already sets --json
// or --agent. The flag goes before the real "--" terminator so it stays a
// flag.
func fleetChildArgs(args []string, parsed fleetParsedArgs) []string {
	if parsed.has("json") || parsed.has("agent") {
		return append([]string{}, args...)
	}
	out := make([]string, 0, len(args)+1)
	if parsed.Terminator < 0 || parsed.Terminator > len(args) {
		out = append(out, args...)
		return append(out, "--json")
	}
	out = append(out, args[:parsed.Terminator]...)
	out = append(out, "--json")
	return append(out, args[parsed.Terminator:]...)
}

// fleetChildEnv drops every ZOOM_PHONE_* variable outside acctEnvAllowlist
// (credentials, path, host and any future overrides), then points the child
// at one account's config file and home.
func fleetChildEnv(base []string, configPath, home string) []string {
	out := make([]string, 0, len(base)+2)
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if acctEnvKeyDropped(key) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "ZOOM_PHONE_CONFIG="+configPath, "ZOOM_PHONE_HOME="+home)
}

// fleetParseResult keeps the child's stdout as JSON when it parses, else as a
// trimmed string.
func fleetParseResult(stdout []byte) any {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		return json.RawMessage(append([]byte(nil), trimmed...))
	}
	return string(trimmed)
}

func fleetTail(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return "..." + s
}

// fleetSelectAccounts returns the accounts to run, in --accounts order when
// given, else every saved account sorted by name.
func fleetSelectAccounts(all []acctInfo, csv string) ([]acctInfo, error) {
	if strings.TrimSpace(csv) == "" {
		return all, nil
	}
	byName := make(map[string]acctInfo, len(all))
	for _, a := range all {
		byName[a.Name] = a
	}
	var missing []string
	out := make([]acctInfo, 0)
	for _, name := range fleetParseAccountNames(csv) {
		if err := acctValidateName(name); err != nil {
			return nil, usageErr(err)
		}
		a, ok := byName[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		out = append(out, a)
	}
	if len(missing) > 0 {
		saved := make([]string, 0, len(all))
		for _, a := range all {
			saved = append(saved, a.Name)
		}
		return nil, notFoundErr(fmt.Errorf("account(s) not found: %s (saved: %s)", strings.Join(missing, ", "), strings.Join(saved, ", ")))
	}
	if len(out) == 0 {
		return nil, usageErr(errors.New("--accounts named no accounts"))
	}
	return out, nil
}

// fleetRun runs childArgs once per account, at most concurrency at a time,
// and returns results in account order.
func fleetRun(ctx context.Context, exe string, baseEnv []string, accounts []acctInfo, childArgs []string, concurrency int) []fleetResult {
	results := make([]fleetResult, len(accounts))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, a := range accounts {
		wg.Add(1)
		go func(i int, a acctInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			spec := fleetChildSpec{
				Account: a.Name,
				Path:    exe,
				Args:    append([]string{}, childArgs...),
				Env:     fleetChildEnv(baseEnv, a.Path, a.home),
			}
			out := fleetSpawn(ctx, spec)
			stderr := fleetTail(out.Stderr, fleetStderrTailBytes)
			if out.Err != nil {
				if stderr != "" {
					stderr += "\n"
				}
				stderr += "fleet: could not start child: " + out.Err.Error()
			}
			results[i] = fleetResult{
				Account:   a.Name,
				AccountID: a.AccountID,
				ExitCode:  out.ExitCode,
				OK:        out.Err == nil && out.ExitCode == 0,
				Result:    fleetParseResult(out.Stdout),
				Stderr:    stderr,
			}
		}(i, a)
	}
	wg.Wait()
	return results
}

// fleetOutcomeErr maps per-account failures to the exit contract: none -> nil,
// some -> partialFailureErr (6), all -> apiErr (5).
func fleetOutcomeErr(commandPath string, results []fleetResult) error {
	failed := 0
	for _, r := range results {
		if !r.OK {
			failed++
		}
	}
	switch {
	case failed == 0:
		return nil
	case failed == len(results):
		return apiErr(fmt.Errorf("fleet: %s failed on all %d account(s); see each row's stderr", commandPath, failed))
	default:
		return partialFailureErr(fmt.Errorf("fleet: %s failed on %d of %d account(s); see each row's stderr", commandPath, failed, len(results)))
	}
}

// fleetSummarize renders one result cell for the human table.
func fleetSummarize(r fleetResult) string {
	if !r.OK {
		lines := strings.Split(strings.TrimSpace(r.Stderr), "\n")
		return fleetClip(lines[len(lines)-1], 70)
	}
	switch v := r.Result.(type) {
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(v, &decoded); err == nil {
			switch d := decoded.(type) {
			case []any:
				return fmt.Sprintf("%d items", len(d))
			case map[string]any:
				return fmt.Sprintf("object (%d keys)", len(d))
			}
		}
		return fleetClip(string(v), 70)
	case string:
		return fleetClip(strings.SplitN(v, "\n", 2)[0], 70)
	}
	return ""
}

func fleetClip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-3]) + "..."
}

// fleetVerifyAccounts refuses the whole run (configErr, exit 10) before any
// child starts when a selected account's storage tree is unsafe (a symlink or
// shared credential file could make one account act as another).
func fleetVerifyAccounts(accounts []acctInfo) error {
	var bad []string
	for _, a := range accounts {
		if err := acctVerifyStorageTree(a.Name, a.home); err != nil {
			bad = append(bad, err.Error())
		}
	}
	if len(bad) > 0 {
		return configErr(fmt.Errorf("fleet refuses to start: %s; fix the account(s) or leave them out with --accounts", strings.Join(bad, "; ")))
	}
	return nil
}
