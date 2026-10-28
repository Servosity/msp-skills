package cli

// Shared run-and-wait plumbing for `scripts run-wait` and `tasks run-wait`.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"kaseya-vsa-pp-cli/internal/client"
)

// Typed exit codes for the run-wait commands (declared in pp:typed-exit-codes).
const (
	exitRunFailed  = 6 // the script/task ran and failed, stopped, or exited non-zero
	exitRunTimeout = 8 // still running when --max-wait elapsed; re-run with --attach
)

func runFailedErr(err error) error  { return &cliError{code: exitRunFailed, err: err} }
func runTimeoutErr(err error) error { return &cliError{code: exitRunTimeout, err: err} }

// executionDone reports whether an execution record is finished. VSA 10 sets
// EndTime (the doc's field table spells it Endtime) when an execution ends;
// Successful / Failed / Stopped are also terminal states.
func executionDone(rec map[string]any) bool {
	for _, k := range []string{"EndTime", "Endtime"} {
		if s, ok := rec[k].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	switch strings.ToLower(fmt.Sprint(rec["State"])) {
	case "successful", "failed", "stopped":
		return true
	}
	return false
}

// pollExecution GETs path until executionDone or ctx expires. It returns the
// last record seen, the number of polls, and whether it timed out.
func pollExecution(ctx context.Context, c *client.Client, path string, interval time.Duration) (map[string]any, int, bool, error) {
	var last map[string]any
	polls := 0
	for {
		polls++
		data, err := vsaGet(ctx, c, path, nil)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return last, polls, true, nil
			}
			return last, polls, false, classifyAPIErrorOnly(err)
		}
		rec := map[string]any{}
		if err := json.Unmarshal(data, &rec); err != nil {
			return last, polls, false, apiErr(fmt.Errorf("decode execution %s: %w", path, err))
		}
		last = rec
		if executionDone(rec) {
			return rec, polls, false, nil
		}
		select {
		case <-ctx.Done():
			return last, polls, true, nil
		case <-time.After(interval):
		}
	}
}

// exitCodeValue decodes an ExitCode the API sends as a number or as a string
// (the doc table says string, the doc example shows a number).
func exitCodeValue(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil
	case json.Number:
		n, err := x.Int64()
		return int(n), err == nil
	}
	return 0, false
}

// runPreview is the --dry-run output of a run-wait command: the request it
// would send. Device names are shown as given; they resolve at run time.
type runPreview struct {
	DryRun  bool           `json:"dry_run"`
	Action  string         `json:"action"`
	Request map[string]any `json:"request"`
	Then    string         `json:"then"`
}

func writeRunPreview(w io.Writer, flags *rootFlags, action, method, path string, body any, then string) error {
	p := runPreview{DryRun: true, Action: action, Request: map[string]any{"method": method, "path": path}, Then: then}
	if body != nil {
		p.Request["body"] = body
	}
	if flags != nil && flags.asJSON {
		return json.NewEncoder(w).Encode(p)
	}
	raw, _ := json.Marshal(body)
	_, err := fmt.Fprintf(w, "dry-run: would %s %s %s\nthen: %s\n", method, path, raw, then)
	return err
}

// waitContext bounds the whole run-and-wait by --max-wait (not the root
// --timeout, which is the per-request budget and is often far shorter).
func waitContext(parent context.Context, maxWait time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, maxWait)
}
