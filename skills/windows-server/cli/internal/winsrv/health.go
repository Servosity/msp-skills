// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (fixed-scripts).

package winsrv

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Check statuses, worst first. n/a means the check does not apply to the
// server (the role or product is not installed) and never moves the verdict.
const (
	StatusCrit    = "crit"
	StatusUnknown = "unknown"
	StatusWarn    = "warn"
	StatusOK      = "ok"
	StatusNA      = "n/a"
)

var statusRank = map[string]int{StatusCrit: 4, StatusUnknown: 3, StatusWarn: 2, StatusOK: 1, StatusNA: 0}

// worse returns the higher-ranked of two statuses.
func worse(a, b string) string {
	if statusRank[b] > statusRank[a] {
		return b
	}
	return a
}

// Check is one health check result.
type Check struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	FailingSince string `json:"failing_since,omitempty"`
}

// Failing reports a status that needs attention.
func Failing(status string) bool {
	return status == StatusWarn || status == StatusCrit || status == StatusUnknown
}

// Verdict is the worst check status: crit, then unknown (a check that could
// not run cannot vouch for the server), then warn, then ok.
func Verdict(checks []Check) string {
	v := StatusOK
	for _, c := range checks {
		v = worse(v, c.Status)
	}
	return v
}

// Thresholds. ponytail: constants; promote to config keys when an MSP needs
// different numbers per server.
const (
	diskWarnPct      = 10.0
	diskCritPct      = 5.0
	patchWarnDays    = 35
	patchCritDays    = 65
	sigWarnDays      = 3
	sigCritDays      = 7
	clockCritSeconds = 300.0 // Kerberos tolerates five minutes of skew
	certWarnDays     = 30
	certCritDays     = 14
	checkpointDays   = 7
)

type probe struct {
	key    string // the Gathered key: the script name unless a script is read twice
	script string
	args   map[string]any
}

// healthProbes are the scripts one health run reads, in order.
var healthProbes = []probe{
	{"system", "system", nil},
	{"volumes", "volumes", nil},
	{"services", "services", nil},
	// Critical and error events are read apart, each newest first under its
	// own cap: errors can never crowd a critical event out of the read, and
	// a capped read still holds events of its level, so it cannot read as
	// "none" (round-1 review).
	{"eventlog_critical", "eventlog", map[string]any{"log": "System", "hours": 24, "levels": []int{1}, "max": 500}},
	{"eventlog_errors", "eventlog", map[string]any{"log": "System", "hours": 24, "levels": []int{2}, "max": 500}},
	{"installed_updates", "installed_updates", nil},
	{"pending_updates", "pending_updates", nil},
	{"defender", "defender", nil},
	{"time_sync", "time_sync", nil},
	{"certificates", "certificates", map[string]any{"store": "My"}},
	{"scheduled_tasks", "scheduled_tasks", map[string]any{"include_microsoft": false}},
	{"hyperv_vms", "hyperv_vms", nil},
}

// Gathered holds each probe's rows or error. Down is set when the server
// could not be reached at all.
type Gathered struct {
	Rows map[string][]Row
	Errs map[string]error
	Down error
}

// Unreachable is the result for a server no probe reached (or no credential
// was found for): every check unknown with the same reason, so a run that
// could not connect keeps the check names, failing_since and event history.
func Unreachable(err error) Gathered {
	g := Gathered{Rows: map[string][]Row{}, Errs: map[string]error{}, Down: err}
	for _, pr := range healthProbes {
		g.Errs[pr.key] = err
	}
	return g
}

// serverDown reports whether err from a server's first call means the
// server itself could not be reached, so the remaining calls are skipped.
func serverDown(err error) bool {
	var se *ScriptError
	return !errors.As(err, &se)
}

// Gather runs every health probe on one server. When the first probe cannot
// reach the server the rest are skipped and reported with the same error.
func Gather(ctx context.Context, r Runner, p Profile, now time.Time) Gathered {
	g := Gathered{Rows: map[string][]Row{}, Errs: map[string]error{}}
	for i, pr := range healthProbes {
		rows, err := Rows(ctx, r, p, pr.script, pr.args, now)
		if err != nil {
			if i == 0 && serverDown(err) {
				return Unreachable(err)
			}
			g.Errs[pr.key] = err
			continue
		}
		g.Rows[pr.key] = rows
	}
	return g
}

// Evaluate turns gathered rows into one result per check.
func Evaluate(g Gathered, now time.Time) []Check {
	checks := []Check{
		g.eval("reboot", "system", checkReboot),
		g.eval("disks", "volumes", checkDisks),
		g.eval("services", "services", func(rows []Row) (string, string) { return checkServices(rows, g.boot()) }),
		g.evalAll("system_events", []string{"eventlog_critical", "eventlog_errors"}, checkEvents),
		g.eval("patch_age", "installed_updates", func(rows []Row) (string, string) { return checkPatchAge(rows, now) }),
		g.eval("pending_updates", "pending_updates", checkPending),
		g.eval("defender", "defender", checkDefender),
		g.eval("time_sync", "time_sync", checkTime),
		g.eval("certificates", "certificates", checkCerts),
		g.eval("scheduled_tasks", "scheduled_tasks", checkTasks),
		g.eval("hyperv", "hyperv_vms", checkHyperV),
	}
	return checks
}

// boot is the uptime the system probe read in this run. The system probe runs
// first, seconds before the services probe; the delayed-start window has
// minutes of margin for that. A system probe that failed is an unknown uptime.
func (g Gathered) boot() Boot {
	if _, failed := g.Errs["system"]; failed {
		return Boot{}
	}
	return BootOf(g.Rows["system"])
}

func (g Gathered) eval(name, key string, fn func([]Row) (string, string)) Check {
	return g.evalAll(name, []string{key}, fn)
}

// evalAll judges the rows of several probes together; any probe that could
// not be read makes the check unknown (or n/a when the role is absent).
func (g Gathered) evalAll(name string, keys []string, fn func([]Row) (string, string)) Check {
	var rows []Row
	for _, key := range keys {
		if err, ok := g.Errs[key]; ok {
			var se *ScriptError
			if errors.As(err, &se) && se.Kind == KindRoleAbsent {
				return Check{Name: name, Status: StatusNA, Reason: se.Message}
			}
			return Check{Name: name, Status: StatusUnknown, Reason: "could not read: " + err.Error()}
		}
		rows = append(rows, g.Rows[key]...)
	}
	status, reason := fn(rows)
	return Check{Name: name, Status: status, Reason: reason}
}

func checkReboot(rows []Row) (string, string) {
	if len(rows) == 0 {
		return StatusUnknown, "no system information returned"
	}
	r := rows[0]
	if Bool(r["pending_reboot"]) {
		var why []string
		if list, ok := r["pending_reboot_reasons"].([]any); ok {
			for _, x := range list {
				why = append(why, Str(x))
			}
		}
		return StatusWarn, "reboot pending (" + strings.Join(why, ", ") + ")"
	}
	return StatusOK, "no reboot pending"
}

func checkDisks(rows []Row) (string, string) {
	status, worst := StatusOK, []string{}
	for _, r := range rows {
		label := Str(r["drive_letter"])
		if label == "" {
			label = Str(r["label"])
		}
		pct, _ := Num(r["percent_free"])
		s := StatusOK
		switch {
		case pct < diskCritPct || Str(r["health_status"]) == "Unhealthy":
			s = StatusCrit
		case pct < diskWarnPct || Str(r["health_status"]) == "Warning":
			s = StatusWarn
		}
		if s != StatusOK {
			worst = append(worst, fmt.Sprintf("%s %.1f%% free (%s)", label, pct, Str(r["health_status"])))
			status = worse(status, s)
		}
	}
	if len(rows) == 0 {
		return StatusUnknown, "no fixed volumes returned"
	}
	if status == StatusOK {
		return StatusOK, fmt.Sprintf("%s at least %.0f%% free", count(len(rows), "fixed volume", "fixed volumes"), diskWarnPct)
	}
	return status, strings.Join(worst, "; ")
}

// delayedStartWindow is how long after boot a delayed automatic service that
// has not been started yet (exit code 1077) reads as "not started yet"
// instead of stopped. The Service Control Manager starts Automatic (Delayed
// Start) services about 120 seconds after the last automatic service has
// started (the AutoStartDelay default), one after another at low priority, so
// on a slow boot the last one starts minutes after boot. The lab showed four
// still waiting two minutes after a start and all running a few minutes
// later. Ten minutes covers that with margin; after it, a delayed automatic
// service that never started counts as stopped again.
const delayedStartWindow = 10 * time.Minute

// Boot is how long the server had been up, as the system script read it in
// the same run. The zero value is an unknown uptime, which never hides a
// stopped service.
type Boot struct {
	UptimeSeconds float64
	Known         bool
}

// BootOf reads the uptime from the system script's row. A missing row, or an uptime that is absent, not a number or negative, is unknown.
func BootOf(rows []Row) Boot {
	if len(rows) != 1 {
		return Boot{}
	}
	s, ok := Num(rows[0]["uptime_seconds"])
	if !ok || s < 0 {
		return Boot{}
	}
	return Boot{UptimeSeconds: s, Known: true}
}

// NotStartedYet reports a delayed automatic service the Service Control
// Manager has not started since boot (exit code 1077) on a server whose
// known uptime is still inside delayedStartWindow: it is waiting for its
// delayed start, not down.
func NotStartedYet(r Row, b Boot) bool {
	if Str(r["start_mode"]) != "Auto" || Str(r["state"]) == "Running" || !Bool(r["delayed_auto_start"]) {
		return false
	}
	code, ok := Num(r["exit_code"])
	return ok && code == 1077 && b.Known && b.UptimeSeconds < delayedStartWindow.Seconds()
}

// NeedsBoot reports whether any row's verdict depends on the uptime, so a
// caller that has not read the system script knows whether to.
func NeedsBoot(rows []Row) bool {
	for _, r := range rows {
		if NotStartedYet(r, Boot{Known: true}) {
			return true
		}
	}
	return false
}

// ServiceDown reports an automatic service that is not running and is
// expected to be. A failure exit code always counts. Without one, a
// trigger-start service stops by design, and a delayed automatic service
// that has not been started yet inside the delayed-start window after boot
// is waiting. A delayed automatic service that stopped cleanly counts: the
// exit code says how it stopped, not that it should stay stopped.
func ServiceDown(r Row, b Boot) bool {
	if Str(r["start_mode"]) != "Auto" || Str(r["state"]) == "Running" {
		return false
	}
	if serviceFailed(r) {
		return true
	}
	if Bool(r["trigger_start"]) {
		return false
	}
	return !NotStartedYet(r, b)
}

// serviceFailed reports a stop with a Win32 failure code. 1077 means no start
// was attempted since boot.
func serviceFailed(r Row) bool {
	code, _ := Num(r["exit_code"])
	return code != 0 && code != 1077
}

func checkServices(rows []Row, b Boot) (string, string) {
	var down, waiting []string
	status := StatusOK
	for _, r := range rows {
		if NotStartedYet(r, b) {
			waiting = append(waiting, Str(r["name"]))
			continue
		}
		if !ServiceDown(r, b) {
			continue
		}
		down = append(down, Str(r["name"]))
		s := StatusWarn
		if serviceFailed(r) {
			s = StatusCrit
		}
		status = worse(status, s)
	}
	var parts []string
	if len(down) > 0 {
		sort.Strings(down)
		parts = append(parts, fmt.Sprintf("%s stopped: %s", count(len(down), "automatic service", "automatic services"), strings.Join(down, ", ")))
	}
	if len(waiting) > 0 {
		sort.Strings(waiting)
		up := time.Duration(b.UptimeSeconds) * time.Second
		parts = append(parts, fmt.Sprintf("%s not started yet, %s after boot (they count as stopped once the server has been up %s): %s",
			count(len(waiting), "delayed automatic service", "delayed automatic services"), up, delayedStartWindow, strings.Join(waiting, ", ")))
	}
	if len(parts) == 0 {
		return StatusOK, "every automatic service that should run is running"
	}
	return status, strings.Join(parts, "; ")
}

func checkEvents(rows []Row) (string, string) {
	crit, errs := map[string]int{}, map[string]int{}
	for _, r := range rows {
		key := fmt.Sprintf("%s %s", Str(r["provider"]), Str(r["event_id"]))
		if lv, _ := Num(r["level"]); lv == 1 {
			crit[key]++
		} else {
			errs[key]++
		}
	}
	if len(crit) > 0 {
		return StatusCrit, "critical System events in 24h: " + topKeys(crit)
	}
	if len(errs) > 0 {
		return StatusWarn, "System errors in 24h: " + topKeys(errs)
	}
	return StatusOK, "no critical or error System events in 24h"
}

func topKeys(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > 5 {
		keys = keys[:5]
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s (x%d)", k, m[k])
	}
	return strings.Join(parts, ", ")
}

func checkPatchAge(rows []Row, now time.Time) (string, string) {
	var newest time.Time
	for _, r := range rows {
		if t, ok := Time(r["installed_on"]); ok && t.After(newest) {
			newest = t
		}
	}
	if newest.IsZero() {
		return StatusUnknown, "no installed update reports an install date"
	}
	days := int(now.Sub(newest).Hours() / 24)
	reason := fmt.Sprintf("newest update installed %s ago (%s)", count(days, "day", "days"), newest.Format("2006-01-02"))
	switch {
	case days > patchCritDays:
		return StatusCrit, reason
	case days > patchWarnDays:
		return StatusWarn, reason
	}
	return StatusOK, reason
}

func checkPending(rows []Row) (string, string) {
	if len(rows) == 0 {
		return StatusOK, "no pending software updates"
	}
	critical := 0
	for _, r := range rows {
		if Str(r["severity"]) == "Critical" {
			critical++
		}
	}
	if critical > 0 {
		return StatusCrit, fmt.Sprintf("%s, %d rated Critical", count(len(rows), "pending update", "pending updates"), critical)
	}
	return StatusWarn, count(len(rows), "pending update", "pending updates")
}

func checkDefender(rows []Row) (string, string) {
	if len(rows) == 0 {
		return StatusUnknown, "no Defender status returned"
	}
	r := rows[0]
	mode := Str(r["running_mode"])
	if strings.Contains(strings.ToLower(mode), "passive") {
		return StatusNA, "Defender is in " + mode + " (another antivirus is primary)"
	}
	if !Bool(r["antivirus_enabled"]) || !Bool(r["real_time_protection_enabled"]) {
		return StatusCrit, "antivirus or real-time protection is off"
	}
	age, ok := Num(r["signature_age_days"])
	if !ok {
		return StatusUnknown, "signature age not reported"
	}
	reason := fmt.Sprintf("signatures %s old", count(int(age), "day", "days"))
	switch {
	case age > sigCritDays:
		return StatusCrit, reason
	case age > sigWarnDays:
		return StatusWarn, reason
	}
	return StatusOK, reason
}

func checkTime(rows []Row) (string, string) {
	if len(rows) == 0 {
		return StatusUnknown, "no time service status returned"
	}
	r := rows[0]
	if Str(r["service_state"]) != "Running" {
		return StatusCrit, "the Windows Time service is not running"
	}
	src := Str(r["source"])
	if r["synchronized"] == nil && src == "" {
		return StatusUnknown, "w32tm output was not in a recognized (English) format"
	}
	reason := "source " + src
	if off, ok := Num(r["phase_offset_seconds"]); ok {
		reason += fmt.Sprintf(", offset %.3fs", off)
		if off > clockCritSeconds || off < -clockCritSeconds {
			return StatusCrit, reason + " (over five minutes breaks Kerberos)"
		}
	}
	if r["synchronized"] == false {
		return StatusWarn, reason + ", not synchronized"
	}
	if strings.Contains(src, "Local CMOS Clock") || strings.Contains(src, "Free-running") {
		return StatusWarn, reason + " (no external time source)"
	}
	return StatusOK, reason
}

// checkCerts looks at every certificate with a private key (the ones a
// service can be using). Each counts by its own expiry: a binding (IIS,
// HTTP.sys, RDP) names a thumbprint, not a subject, so a renewal does not
// prove the old certificate is unused. The reason says when a newer
// certificate with the same subject exists.
func checkCerts(rows []Row) (string, string) {
	var keyed []Row
	newest := map[string]string{}
	for _, r := range rows {
		if !Bool(r["has_private_key"]) {
			continue
		}
		keyed = append(keyed, r)
		if subj := Str(r["subject"]); Str(r["not_after"]) > newest[subj] {
			newest[subj] = Str(r["not_after"])
		}
	}
	status, notes := StatusOK, []string{}
	for _, r := range keyed {
		days, ok := r["days_left"].(int)
		if !ok {
			continue
		}
		subj := Str(r["subject"])
		if tp := Str(r["thumbprint"]); tp != "" {
			subj += " (" + tp + ")"
		}
		if Str(r["not_after"]) < newest[Str(r["subject"])] {
			subj += ", renewed: remove it once no binding uses it"
		}
		s := StatusOK
		switch {
		case days < certCritDays:
			s = StatusCrit
		case days < certWarnDays:
			s = StatusWarn
		}
		if s != StatusOK {
			notes = append(notes, fmt.Sprintf("%s in %s", subj, count(days, "day", "days")))
			status = worse(status, s)
		}
	}
	if status == StatusOK {
		return StatusOK, fmt.Sprintf("%s with a private key, none expiring within %d days", count(len(keyed), "certificate", "certificates"), certWarnDays)
	}
	sort.Strings(notes)
	return status, "expiring: " + strings.Join(notes, "; ")
}

func checkTasks(rows []Row) (string, string) {
	var failed []string
	for _, r := range rows {
		if TaskFailed(r) {
			failed = append(failed, fmt.Sprintf("%s%s (%s)", Str(r["task_path"]), Str(r["task_name"]), Str(r["last_result_hex"])))
		}
	}
	if len(failed) == 0 {
		return StatusOK, fmt.Sprintf("%s, none failing", count(len(rows), "non-Microsoft task", "non-Microsoft tasks"))
	}
	sort.Strings(failed)
	return StatusWarn, "failed: " + strings.Join(failed, ", ")
}

func checkHyperV(rows []Row) (string, string) {
	status, notes := StatusOK, []string{}
	for _, r := range rows {
		name := Str(r["name"])
		switch Str(r["replication_health"]) {
		case "Critical":
			status = worse(status, StatusCrit)
			notes = append(notes, name+" replication critical")
		case "Warning":
			status = worse(status, StatusWarn)
			notes = append(notes, name+" replication warning")
		}
		if e := Str(r["checkpoint_error"]); e != "" {
			status = worse(status, StatusWarn)
			notes = append(notes, name+" checkpoints unreadable: "+e)
		}
		if d, ok := Num(r["oldest_checkpoint_days"]); ok && d > checkpointDays {
			status = worse(status, StatusWarn)
			notes = append(notes, fmt.Sprintf("%s checkpoint %s old", name, count(int(d), "day", "days")))
		}
	}
	if status == StatusOK {
		return StatusOK, fmt.Sprintf("%s, no replication or checkpoint issues", count(len(rows), "VM", "VMs"))
	}
	return status, strings.Join(notes, "; ")
}

// count reads "1 day" or "3 days".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
