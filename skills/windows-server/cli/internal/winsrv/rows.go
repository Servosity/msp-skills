// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (fixed-scripts).

package winsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"windows-server-pp-cli/internal/winrm"
)

// Row is one result record. Every row carries "server" (the configured
// server name) and "id" (server-scoped, so rows from different servers never
// collide in the local store).
type Row = map[string]any

type checkDef struct {
	single bool
	key    func(Row) string
	post   func(Row, time.Time)
}

func field(name string) func(Row) string {
	return func(r Row) string { return Str(r[name]) }
}

var checks = map[string]checkDef{
	"system": {single: true},
	"volumes": {key: func(r Row) string {
		if l := Str(r["drive_letter"]); l != "" {
			return l
		}
		return Str(r["unique_id"])
	}},
	"services":          {key: field("name"), post: servicePost},
	"service_get":       {single: true, key: field("name"), post: servicePost},
	"eventlog":          {key: func(r Row) string { return Str(r["log"]) + "/" + Str(r["record_id"]) }},
	"scm_events":        {key: field("record_id")},
	"installed_updates": {key: field("hotfix_id")},
	"pending_updates":   {key: field("update_id")},
	"features":          {key: field("name")},
	"local_admins":      {key: field("sid")},
	"shares":            {key: field("name")},
	"scheduled_tasks":   {key: func(r Row) string { return Str(r["task_path"]) + Str(r["task_name"]) }, post: taskPost},
	"certificates":      {key: func(r Row) string { return Str(r["store"]) + "/" + Str(r["thumbprint"]) }, post: certPost},
	"defender":          {single: true, post: defenderPost},
	"time_sync":         {single: true, post: timePost},
	"hyperv_vms":        {key: field("vm_id")},
}

// ServiceStates is the one vocabulary for a service state: the spec's enum,
// which is also what Get-Service (and so the service actions) writes.
// Win32_Service writes the pending states with a space ("Stop Pending");
// servicePost removes it, so a filter, a comparison and the output all use
// the same words (round-1 review).
var ServiceStates = []string{"Running", "Stopped", "Paused", "StartPending", "StopPending", "ContinuePending", "PausePending", "Unknown"}

func servicePost(r Row, _ time.Time) {
	if s, ok := r["state"].(string); ok {
		r["state"] = strings.ReplaceAll(s, " ", "")
	}
}

// Rows runs a check and returns its rows with server, id and computed fields.
func Rows(ctx context.Context, r Runner, p Profile, script string, args map[string]any, now time.Time) ([]Row, error) {
	def, ok := checks[script]
	if !ok {
		return nil, fmt.Errorf("%s is not a row check", script)
	}
	raw, err := r.Run(ctx, p, script, args)
	if err != nil {
		return nil, err
	}
	var rows []Row
	if def.single {
		var one Row
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, fmt.Errorf("%s on %s: %w", script, p.Name, err)
		}
		if one != nil {
			rows = []Row{one}
		}
	} else if err := json.Unmarshal(NormalizeArray(raw), &rows); err != nil {
		return nil, fmt.Errorf("%s on %s: %w", script, p.Name, err)
	}
	for _, row := range rows {
		if def.post != nil {
			def.post(row, now)
		}
		row["server"] = p.Name
		if def.key == nil {
			row["id"] = p.Name
		} else {
			row["id"] = p.Name + "/" + def.key(row)
		}
	}
	return rows, nil
}

// NormalizeArray turns whatever PowerShell produced for a list into a JSON
// array: null -> [], one object -> [obj], and the Windows PowerShell 5.1
// {"value":[...],"Count":n} wrapper -> its value.
func NormalizeArray(raw json.RawMessage) json.RawMessage {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return json.RawMessage("[]")
	}
	if t[0] == '{' {
		var wrapper map[string]json.RawMessage
		if json.Unmarshal(t, &wrapper) == nil && len(wrapper) == 2 {
			if v, ok := wrapper["value"]; ok {
				if _, ok := wrapper["Count"]; ok {
					return v
				}
			}
		}
		return append(append([]byte{'['}, t...), ']')
	}
	return t
}

// Str renders a JSON-decoded scalar as text ("" for nil).
func Str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// Num returns a JSON-decoded number (and whether it was one).
func Num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// Bool returns a JSON-decoded boolean (false when absent).
func Bool(v any) bool {
	b, _ := v.(bool)
	return b
}

// Time parses an RFC 3339 field.
func Time(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// taskOK is the set of LastTaskResult codes that are not failures: success
// and the Task Scheduler status codes for ready, running, disabled, not yet
// run, no more runs, terminated by the user, and queued.
var taskOK = map[int64]bool{0: true, 0x41300: true, 0x41301: true, 0x41302: true, 0x41303: true, 0x41304: true, 0x41306: true, 0x41325: true}

func taskPost(r Row, _ time.Time) {
	if Str(r["info_error"]) != "" { // the last result could not be read: not vouched for
		r["last_result_hex"], r["last_result_ok"] = "unreadable", false
		return
	}
	n, ok := Num(r["last_result"])
	if !ok {
		r["last_result_hex"], r["last_result_ok"] = nil, true
		return
	}
	code := int64(n)
	r["last_result_hex"] = fmt.Sprintf("0x%08X", code&0xFFFFFFFF) // the low 32 bits: a negative HRESULT reads 0x8...
	_, ran := Time(r["last_run_time"])
	r["last_result_ok"] = taskOK[code] || !ran
}

func certPost(r Row, now time.Time) {
	if t, ok := Time(r["not_after"]); ok {
		r["days_left"] = int(math.Floor(t.Sub(now).Hours() / 24))
		r["expired"] = !t.After(now)
	}
}

// neverScanned is what Get-MpComputerStatus reports for a scan that never ran.
const neverScanned = 4294967295

func defenderPost(r Row, _ time.Time) {
	for _, k := range []string{"quick_scan_age_days", "full_scan_age_days", "signature_age_days"} {
		if n, ok := Num(r[k]); ok && (n >= neverScanned || n >= 65535 && k == "signature_age_days") {
			r[k] = nil
		}
	}
}

var leapRE = regexp.MustCompile(`^\s*(\d+)`)

// timePost parses `w32tm /query /status /verbose` (English output) into
// fields. On a non-English server the fields stay empty and health reports
// the check as unknown rather than guessing.
func timePost(r Row, _ time.Time) {
	kv := map[string]string{}
	if lines, ok := r["lines"].([]any); ok {
		for _, l := range lines {
			s := Str(l)
			if i := strings.Index(s, ":"); i > 0 {
				kv[strings.TrimSpace(s[:i])] = strings.TrimSpace(s[i+1:])
			}
		}
	}
	delete(r, "lines")
	r["source"], r["stratum"], r["leap_indicator"], r["synchronized"], r["last_successful_sync"], r["phase_offset_seconds"] = nil, nil, nil, nil, nil, nil
	if v, ok := kv["Source"]; ok {
		r["source"] = v
	}
	if m := leapRE.FindStringSubmatch(kv["Stratum"]); m != nil {
		n, _ := strconv.Atoi(m[1])
		r["stratum"] = n
	}
	if v, ok := kv["Leap Indicator"]; ok {
		r["leap_indicator"] = v
		if m := leapRE.FindStringSubmatch(v); m != nil {
			r["synchronized"] = m[1] != "3"
		}
	}
	if t, ok := Time(r["last_sync_utc"]); ok {
		r["last_successful_sync"] = t.UTC().Format(time.RFC3339)
	}
	delete(r, "last_sync_utc")
	if v := strings.TrimSuffix(kv["Phase Offset"], "s"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			r["phase_offset_seconds"] = f
		}
	}
}

// HTTPError carries the status the virtual REST surface answers with.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

type query struct {
	v       url.Values
	allowed map[string]bool
}

func newQuery(v url.Values, allowed ...string) (*query, error) {
	q := &query{v: v, allowed: map[string]bool{}}
	for _, a := range allowed {
		q.allowed[a] = true
	}
	for k, vals := range v {
		if !q.allowed[k] {
			return nil, &HTTPError{400, fmt.Sprintf("unknown parameter %q", k)}
		}
		if len(vals) != 1 {
			return nil, &HTTPError{400, fmt.Sprintf("parameter %q given more than once", k)}
		}
	}
	return q, nil
}

func (q *query) str(name, def string) string {
	if s := q.v.Get(name); s != "" {
		return s
	}
	return def
}

func (q *query) boolean(name string) (bool, bool, error) {
	s := q.v.Get(name)
	if s == "" {
		return false, false, nil
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, false, &HTTPError{400, fmt.Sprintf("%s must be true or false", name)}
	}
	return b, true, nil
}

func (q *query) integer(name string, def, min, max int) (int, bool, error) {
	s := q.v.Get(name)
	if s == "" {
		return def, false, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < min || n > max {
		return 0, false, &HTTPError{400, fmt.Sprintf("%s must be a whole number from %d to %d", name, min, max)}
	}
	return n, true, nil
}

func oneOf(name, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return &HTTPError{400, fmt.Sprintf("%s must be one of %s", name, strings.Join(allowed, ", "))}
}

// EventLevels maps the eventlog level choice to Windows event levels.
var EventLevels = map[string][]int{"critical": {1}, "error": {1, 2}, "warning": {1, 2, 3}, "information": {1, 2, 3, 4}}

// Read answers one GET on the virtual REST surface (the paths in spec.yaml)
// for server p. The result is a JSON array, or an object for single reads.
func Read(ctx context.Context, r Runner, p Profile, path string, v url.Values, now time.Time) (json.RawMessage, error) {
	simple := map[string]string{
		"/system": "system", "/volumes": "volumes", "/installed-updates": "installed_updates",
		"/pending-updates": "pending_updates", "/features": "features", "/local-admins": "local_admins",
		"/shares": "shares", "/defender": "defender", "/time-sync": "time_sync", "/hyperv-vms": "hyperv_vms",
	}
	var rows []Row
	var err error
	single := false
	switch {
	case simple[path] != "":
		if _, err := newQuery(v); err != nil {
			return nil, err
		}
		script := simple[path]
		single = checks[script].single
		rows, err = Rows(ctx, r, p, script, nil, now)
	case path == "/services":
		q, qerr := newQuery(v, "stopped_automatic", "state", "start_mode")
		if qerr != nil {
			return nil, qerr
		}
		stoppedAuto, _, berr := q.boolean("stopped_automatic")
		if berr != nil {
			return nil, berr
		}
		state, mode := q.str("state", ""), q.str("start_mode", "")
		if state != "" {
			if err := oneOf("state", state, ServiceStates...); err != nil {
				return nil, err
			}
		}
		if mode != "" {
			if err := oneOf("start_mode", mode, "Auto", "Manual", "Disabled", "Boot", "System"); err != nil {
				return nil, err
			}
		}
		rows, err = Rows(ctx, r, p, "services", nil, now)
		rows = filterRows(rows, func(row Row) bool {
			if stoppedAuto && !(Str(row["start_mode"]) == "Auto" && Str(row["state"]) != "Running") {
				return false
			}
			return (state == "" || Str(row["state"]) == state) && (mode == "" || Str(row["start_mode"]) == mode)
		})
	case strings.HasPrefix(path, "/services/") && strings.Count(path, "/") == 2:
		if _, err := newQuery(v); err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(path, "/services/")
		if !serviceNameRE.MatchString(name) {
			return nil, &HTTPError{400, "service name must be letters, digits, _ . $ - (at most 80)"}
		}
		single = true
		rows, err = Rows(ctx, r, p, "service_get", map[string]any{"name": name}, now)
	case path == "/eventlog":
		q, qerr := newQuery(v, "log", "hours", "level", "max")
		if qerr != nil {
			return nil, qerr
		}
		log, level := q.str("log", "System"), q.str("level", "warning")
		if err := oneOf("log", log, "System", "Application"); err != nil {
			return nil, err
		}
		if err := oneOf("level", level, "critical", "error", "warning", "information"); err != nil {
			return nil, err
		}
		hours, _, herr := q.integer("hours", 24, 1, 720)
		if herr != nil {
			return nil, herr
		}
		max, _, merr := q.integer("max", 200, 1, 2000)
		if merr != nil {
			return nil, merr
		}
		rows, err = Rows(ctx, r, p, "eventlog", map[string]any{"log": log, "hours": hours, "levels": EventLevels[level], "max": max}, now)
	case path == "/scheduled-tasks":
		q, qerr := newQuery(v, "failed", "include_microsoft")
		if qerr != nil {
			return nil, qerr
		}
		failed, _, ferr := q.boolean("failed")
		if ferr != nil {
			return nil, ferr
		}
		incl, _, ierr := q.boolean("include_microsoft")
		if ierr != nil {
			return nil, ierr
		}
		rows, err = Rows(ctx, r, p, "scheduled_tasks", map[string]any{"include_microsoft": incl}, now)
		if failed {
			rows = filterRows(rows, TaskFailed)
		}
	case path == "/certificates":
		q, qerr := newQuery(v, "store", "expiring_days")
		if qerr != nil {
			return nil, qerr
		}
		store := q.str("store", "My")
		if err := oneOf("store", store, "My", "WebHosting", "Remote Desktop", "CA", "Root"); err != nil {
			return nil, err
		}
		days, set, derr := q.integer("expiring_days", 0, 1, 3650)
		if derr != nil {
			return nil, derr
		}
		rows, err = Rows(ctx, r, p, "certificates", map[string]any{"store": store}, now)
		if set {
			rows = filterRows(rows, func(row Row) bool {
				n, ok := row["days_left"].(int)
				return ok && n <= days
			})
		}
	default:
		return nil, &HTTPError{404, "no such endpoint: " + path}
	}
	if err != nil {
		return nil, httpErr(err)
	}
	var body []byte
	switch {
	case single && len(rows) == 0:
		return nil, &HTTPError{404, "not found"}
	case single:
		body, err = json.Marshal(rows[0])
	default:
		if rows == nil {
			rows = []Row{}
		}
		body, err = json.Marshal(rows)
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}

// TaskFailed reports an enabled task whose last run returned a failure code.
func TaskFailed(row Row) bool {
	return !Bool(row["last_result_ok"]) && Str(row["state"]) != "Disabled"
}

func filterRows(rows []Row, keep func(Row) bool) []Row {
	out := rows[:0]
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// httpErr maps a run error to the status the virtual surface answers with.
// Nothing maps to 5xx: the generated client retries those, and a retry
// reruns the whole script on the server for an answer that will not change.
// A script failure is 422 and an unreachable server 424.
func httpErr(err error) error {
	var se *ScriptError
	if errors.As(err, &se) {
		switch se.Kind {
		case KindNotFound:
			return &HTTPError{404, se.Error()}
		case KindRoleAbsent:
			return &HTTPError{404, se.Error()}
		case KindInvalidArg:
			return &HTTPError{400, se.Error()}
		case KindAccessDenied:
			return &HTTPError{403, se.Error()}
		}
		return &HTTPError{422, se.Error()}
	}
	if errors.Is(err, winrm.ErrAuth) {
		return &HTTPError{401, err.Error()}
	}
	return &HTTPError{424, err.Error()}
}
