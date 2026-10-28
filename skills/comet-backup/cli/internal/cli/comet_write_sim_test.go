// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Crash-and-retry simulation for the commands that write into live customer
// data in bulk: rerun-failed (one run-backup dispatch per failed scheduled
// backup) and import (one create per JSONL record). Each seed drives the real
// command code (RootCmd, in process) against a fake Comet Server whose every
// request draws one behavior: answer, refuse with a 5xx, refuse with a 4xx,
// drop the connection mid-response, answer after the client timeout, answer
// 200 with an empty body, or answer 200 with a Comet failure envelope. A seed
// runs two commands in parallel, then one that crashes (its context is
// cancelled at a random request), then a rerun, then a final run against a
// server that no longer faults. The fake server is the oracle: it records
// what it actually did, so a pass does not depend on what the command says.
//
// Invariants (comet-backup handfix: comet-write-crash-retry-sim):
//
//	rerun-failed
//	  R1 a run-backup for a (tenant, device, rule) never reaches the server
//	     while the server already shows that backup as running (or the job is
//	     not a re-runnable failure), across parallel runs, a crash and a rerun;
//	  R2 a run that exits 0 without --allow-partial-failure left every backup
//	     that was re-runnable when it started running on the server, and every
//	     row it reports as dispatched names a job the server started for it;
//	  R3 a rerun against a healthy server exits 0 and leaves nothing to re-run.
//	import
//	  I1 no record is created twice on the server across parallel runs, a
//	     crash and a rerun with --idempotent;
//	  I2 a run that exits 0 left every input record on the server;
//	  I3 a rerun with --idempotent against a healthy server exits 0 and every
//	     record exists exactly once.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/cliutil/testenv"
	"comet-backup-pp-cli/internal/comet"
	"comet-backup-pp-cli/internal/config"
)

const (
	simPassword = "simAdminPass-6d2b9f41"
	// simChaosTimeout is --timeout for the faulting runs: long enough for a
	// loopback answer, short enough that a late answer costs little. A healthy
	// request that misses it is one more fault, which every invariant holds under.
	simChaosTimeout = "50ms"
	simCalmTimeout  = "30s"
)

type simFault int

const (
	simOK simFault = iota
	simRefuse5xx
	simRefuse4xx
	simDrop
	simLate
	simEmpty
	simEnvelope
	simCrash // not drawn: the armed crash request
)

func (f simFault) String() string {
	return [...]string{"ok", "refuse-5xx", "refuse-4xx", "drop", "late", "empty-200", "envelope-200", "crash"}[f]
}

// simSeedCount is 200 seeds (25 under -short); COMET_WRITE_SIM_SEEDS sets
// another count for a longer soak.
func simSeedCount() int {
	if v, err := strconv.Atoi(os.Getenv("COMET_WRITE_SIM_SEEDS")); err == nil && v > 0 {
		return v
	}
	if testing.Short() {
		return 25
	}
	return 200
}

// simIsolate sandboxes HOME and every config/data/state/cache lookup, points
// the CLI at no real profile, and silences the commands' direct stderr writes
// (retry notices, warnings) for the duration of the test.
func simIsolate(t *testing.T) {
	t.Helper()
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	// The seeds model a server that shows a started job at once, and they reuse
	// names, so nothing waits out a hold from an earlier seed. comet_intent_test.go
	// proves the hold itself.
	oldHold := rerunIntentHold
	rerunIntentHold = 0
	t.Cleanup(func() { rerunIntentHold = oldHold })
	t.Setenv("COMET_BACKUP_HOME", home)
	t.Setenv("COMET_BACKUP_BASE_URL", "")
	t.Setenv("COMET_BACKUP_SERVER", "")
	t.Setenv("COMET_BACKUP_USERNAME", "admin")
	t.Setenv("COMET_BACKUP_PASSWORD", simPassword)
	t.Setenv("COMET_BACKUP_SESSION_KEY", "")
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.DogfoodEnvVar, "")
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stderr = old
		_ = devnull.Close()
	})
}

// simRun executes one CLI invocation in process and returns its exit code
// and stdout.
func simRun(ctx context.Context, args ...string) (int, string) {
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		return 0, out.String()
	}
	return ExitCode(err), out.String() + "\n" + errb.String() + "\nerror: " + err.Error()
}

// ---- the fake Comet Server -----------------------------------------------------

type simJob struct {
	GUID, User, Device, Source, Dest, Rule string
	Status                                 int64
	Start                                  int64
	Custom                                 bool // a manual or custom run: no rule id recorded
}

type simTenant struct {
	name    string
	devices []string
	online  map[string]string    // device -> connection id ("" = offline)
	sources map[string]string    // source -> owner device
	vaults  []string             // destinations
	rules   map[string][2]string // rule -> source, destination
}

type simComet struct {
	mu         sync.Mutex
	rng        *rand.Rand
	readFault  float64
	writeFault float64
	faultFree  bool
	crashAt    int
	reqN       int
	crash      context.CancelFunc
	trace      []string
	violations []string

	// rerun-failed model
	version string
	tenants map[string]*simTenant
	conns   map[string][2]string // connection id -> tenant, device
	jobs    []simJob
	clock   int64
	seq     int

	// import model
	users       map[string]bool
	userCreates map[string]int
	vaults      map[string]int // TargetUser\x00StorageProvider -> vaults created
}

// simStartClock: most seeds run a server whose jobs are hours old. Every fourth
// has a clock a few seconds ahead of this machine's, so a job it starts now has a
// start time this machine still calls the future (the SDK's date-range read only
// returns jobs that started or ended in the range).
func simStartClock(seed int64) int64 {
	if seed%4 == 0 {
		return time.Now().Unix() + 5
	}
	return time.Now().Unix() - 10*3600
}

func newSimComet(seed int64) *simComet {
	rng := rand.New(rand.NewSource(seed))
	return &simComet{
		rng:         rng,
		readFault:   0.08 + 0.12*rng.Float64(),
		writeFault:  0.15 + 0.45*rng.Float64(),
		tenants:     map[string]*simTenant{},
		conns:       map[string][2]string{},
		clock:       simStartClock(seed),
		users:       map[string]bool{},
		userCreates: map[string]int{},
		vaults:      map[string]int{},
	}
}

func (s *simComet) logf(format string, args ...any) {
	s.trace = append(s.trace, fmt.Sprintf(format, args...))
}

func (s *simComet) violate(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.violations = append(s.violations, msg)
	s.trace = append(s.trace, "VIOLATION "+msg)
}

func (s *simComet) ruleIDRecorded() bool { return comet.VersionAtLeast(s.version, 24, 6, 6) }

// draw picks this request's behavior. Callers hold s.mu.
func (s *simComet) draw(write bool) simFault {
	s.reqN++
	if s.crashAt > 0 && s.reqN == s.crashAt {
		return simCrash
	}
	if s.faultFree {
		return simOK
	}
	p, weights := s.readFault, simReadWeights
	if write {
		p, weights = s.writeFault, simWriteWeights
	}
	if s.rng.Float64() >= p {
		return simOK
	}
	total := 0
	for _, w := range weights {
		total += w
	}
	n := s.rng.Intn(total)
	for k, w := range weights {
		if n < w {
			return simFault(k + 1)
		}
		n -= w
	}
	return simOK
}

// Relative weights of refuse-5xx, refuse-4xx, drop, late, empty-200 and
// envelope-200. A read that is refused with a 5xx, dropped or answered late
// costs the run its whole --timeout (the client retries reads until the
// deadline), so reads lean on the faults that cost nothing; writes draw all
// six alike.
var (
	simReadWeights  = []int{1, 3, 1, 1, 2, 3}
	simWriteWeights = []int{1, 1, 1, 1, 1, 1}
)

// appliedUnder reports whether a write that met fault f still took effect on
// the server: a proxy 5xx, a dropped reply or an empty reply may follow work
// the server already did; a 4xx or a failure envelope did not.
func (s *simComet) appliedUnder(f simFault) bool {
	switch f {
	case simOK, simLate:
		return true
	case simRefuse5xx, simEmpty, simCrash:
		return s.rng.Intn(2) == 0
	case simDrop:
		return s.rng.Intn(10) < 7
	}
	return false
}

func (s *simComet) envelopeBody() string {
	switch s.rng.Intn(3) {
	case 0:
		return `{"Status":500,"Message":"Internal server error"}`
	case 1:
		return `{"Status":400,"Message":""}`
	}
	return `{"Status":403}`
}

func (s *simComet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/")
	write := path == "dispatcher/run-backup" || path == "add-user" || path == "request-storage-vault"
	s.mu.Lock()
	if r.PostForm.Get("Username") != "admin" || r.PostForm.Get("Password") != simPassword {
		s.violate("%s arrived without the admin credential in the form body", path)
	}
	fault := s.draw(write)
	var body string
	if write {
		body = s.applyWrite(path, r.PostForm, fault)
	} else {
		body = s.readBody(path, r.PostForm)
	}
	s.logf("#%d %s -> %s", s.reqN, path, fault)
	crash := s.crash
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch fault {
	case simOK:
		_, _ = w.Write([]byte(body))
	case simRefuse5xx:
		w.WriteHeader([]int{500, 502, 503}[s.pick(3)])
		_, _ = w.Write([]byte(`upstream unavailable`))
	case simRefuse4xx:
		w.WriteHeader([]int{400, 404, 409, 409}[s.pick(4)])
		_, _ = w.Write([]byte(`{"Status":400,"Message":"Bad request"}`))
	case simDrop:
		s.hangUp(w, s.pick(2) == 0)
	case simLate:
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		_, _ = w.Write([]byte(body))
	case simEmpty:
		w.WriteHeader(http.StatusOK)
	case simEnvelope:
		s.mu.Lock()
		env := s.envelopeBody()
		s.mu.Unlock()
		_, _ = w.Write([]byte(env))
	case simCrash:
		if crash != nil {
			crash()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		s.hangUp(w, false)
	}
}

func (s *simComet) pick(n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rng.Intn(n)
}

// hangUp drops the connection, optionally after a status line, headers and
// part of the body.
func (s *simComet) hangUp(w http.ResponseWriter, partial bool) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	if partial {
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 400\r\n\r\n{\"Status\":200,\"Mes")
		_ = buf.Flush()
	}
	_ = conn.Close()
}

// readBody answers a read from the current state. Callers hold s.mu.
func (s *simComet) readBody(path string, form map[string][]string) string {
	get := func(k string) string {
		if v := form[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	switch path {
	case "meta/version":
		return fmt.Sprintf(`{"Version":%q}`, s.version)
	case "get-jobs-for-date-range":
		var from, to int64
		_, _ = fmt.Sscan(get("Start"), &from)
		_, _ = fmt.Sscan(get("End"), &to)
		out := make([]map[string]any, 0, len(s.jobs))
		for _, j := range s.jobs {
			if j.Start < from || j.Start > to {
				continue
			}
			row := map[string]any{"GUID": j.GUID, "Username": j.User, "DeviceID": j.Device, "SourceGUID": j.Source,
				"DestinationGUID": j.Dest, "Classification": 4001, "Status": j.Status, "StartTime": j.Start}
			if j.Status < 6000 || j.Status >= 7000 {
				row["EndTime"] = j.Start + 30
			}
			if s.ruleIDRecorded() && !j.Custom {
				row["BackupRuleGUID"] = j.Rule
			}
			out = append(out, row)
		}
		b, _ := json.Marshal(out)
		return string(b)
	case "list-users-full":
		out := map[string]any{}
		for name, tn := range s.tenants {
			devices, sources, vaults, rules := map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}
			for _, d := range tn.devices {
				devices[d] = map[string]any{"FriendlyName": strings.ToUpper(name + "-" + d)}
			}
			for src, owner := range tn.sources {
				sources[src] = map[string]any{"Description": "Files " + src, "OwnerDevice": owner}
			}
			for _, v := range tn.vaults {
				vaults[v] = map[string]any{"Description": "Vault " + v}
			}
			for id, sd := range tn.rules {
				rules[id] = map[string]any{"Source": sd[0], "Destination": sd[1]}
			}
			out[name] = map[string]any{"Username": name, "Devices": devices, "Sources": sources, "Destinations": vaults, "BackupRules": rules}
		}
		b, _ := json.Marshal(out)
		return string(b)
	case "dispatcher/list-active":
		out := map[string]any{}
		for id, td := range s.conns {
			out[id] = map[string]any{"Username": td[0], "DeviceID": td[1]}
		}
		b, _ := json.Marshal(out)
		return string(b)
	case "list-users":
		names := make([]string, 0, len(s.users))
		for u := range s.users {
			names = append(names, u)
		}
		sort.Strings(names)
		b, _ := json.Marshal(names)
		return string(b)
	}
	s.violate("unexpected read %s", path)
	return `{}`
}

// applyWrite performs a write the way the server would under fault f and
// returns the success body. Callers hold s.mu.
func (s *simComet) applyWrite(path string, form map[string][]string, f simFault) string {
	get := func(k string) string {
		if v := form[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	switch path {
	case "dispatcher/run-backup":
		td, ok := s.conns[get("TargetID")]
		if !ok {
			s.violate("run-backup for unknown or offline connection %q", get("TargetID"))
			return `{"Status":200,"Message":"OK","JobID":"none"}`
		}
		tn := s.tenants[td[0]]
		rule := get("BackupRule")
		sd, ok := tn.rules[rule]
		if !ok {
			s.violate("run-backup for unknown rule %s/%s", td[0], rule)
			return `{"Status":200,"Message":"OK","JobID":"none"}`
		}
		if owner := tn.sources[sd[0]]; owner != td[1] {
			s.violate("run-backup of %s/%s sent to device %s, but the protected item belongs to %s", td[0], rule, td[1], owner)
		}
		if why := s.notRerunnable(td[0], rule); why != "" {
			s.violate("run-backup for %s/%s/%s reached the server while %s", td[0], td[1], rule, why)
			return fmt.Sprintf(`{"Status":200,"Message":"OK","JobID":"dup-%d"}`, s.reqN)
		}
		if !s.appliedUnder(f) {
			return `{"Status":200,"Message":"OK","JobID":"unapplied"}`
		}
		s.seq++
		s.clock += 60
		j := simJob{GUID: fmt.Sprintf("job-new-%d", s.seq), User: td[0], Device: td[1], Source: sd[0], Dest: sd[1], Rule: rule, Status: 6001, Start: s.clock}
		s.jobs = append(s.jobs, j)
		s.logf("  started %s for %s/%s", j.GUID, td[0], rule)
		return fmt.Sprintf(`{"Status":200,"Message":"OK","JobID":%q}`, j.GUID)
	case "add-user":
		u := get("TargetUser")
		if s.users[u] {
			if f == simOK {
				return `{"Status":400,"Message":"User already exists"}`
			}
			return `{"Status":200,"Message":"OK"}`
		}
		if !s.appliedUnder(f) {
			return `{"Status":200,"Message":"OK"}`
		}
		s.users[u] = true
		s.userCreates[u]++
		s.logf("  created user %s", u)
		return `{"Status":200,"Message":"OK"}`
	case "request-storage-vault":
		if !s.appliedUnder(f) {
			return `{"Status":200,"Message":"OK","DestinationID":"unapplied"}`
		}
		k := get("TargetUser") + "\x00" + get("StorageProvider")
		s.vaults[k]++
		s.seq++
		s.logf("  created vault %d for %q", s.vaults[k], k)
		return fmt.Sprintf(`{"Status":200,"Message":"OK","DestinationID":"vault-new-%d"}`, s.seq)
	}
	s.violate("unexpected write %s", path)
	return `{}`
}

// newest is the job Comet reports last for one protected item and vault.
func (s *simComet) newest(user, source, dest string) (simJob, bool) {
	var best simJob
	found := false
	for _, j := range s.jobs {
		if j.User == user && j.Source == source && j.Dest == dest && (!found || j.Start > best.Start) {
			best, found = j, true
		}
	}
	return best, found
}

// notRerunnable is "" when the rule's newest job is a failure rerun-failed
// may re-run on a device that is online now, else why not.
func (s *simComet) notRerunnable(user, rule string) string {
	tn := s.tenants[user]
	sd := tn.rules[rule]
	j, ok := s.newest(user, sd[0], sd[1])
	switch {
	case !ok:
		return "the rule has no job"
	case comet.Family(j.Status) != "failed":
		return fmt.Sprintf("job %s is %s", j.GUID, comet.StatusName(j.Status))
	case j.Status == 7003 || j.Status == 7005 || j.Status == 7006:
		return fmt.Sprintf("job %s is %s (never re-run)", j.GUID, comet.StatusName(j.Status))
	case j.Custom && s.ruleIDRecorded():
		return fmt.Sprintf("job %s is a custom run", j.GUID)
	case tn.online[j.Device] == "":
		return "the device is offline"
	}
	return ""
}

func (s *simComet) rerunnable() []string {
	var out []string
	for name, tn := range s.tenants {
		for rule := range tn.rules {
			if s.notRerunnable(name, rule) == "" {
				out = append(out, name+"/"+rule)
			}
		}
	}
	sort.Strings(out)
	return out
}

// buildRerunScenario lays out tenants, devices, rules, live connections and
// one job per rule, with at least one re-runnable failure.
func (s *simComet) buildRerunScenario() {
	s.version = []string{"24.9.1", "26.8.7", "24.3.0"}[s.rng.Intn(3)]
	statuses := []int64{7002, 7002, 7000, 7004, 7007, 7003, 7005, 7006, 5000, 6001}
	for ti := 1; ti <= 1+s.rng.Intn(3); ti++ {
		tn := &simTenant{name: fmt.Sprintf("tenant-%d", ti), online: map[string]string{}, sources: map[string]string{}, rules: map[string][2]string{}}
		for v := 1; v <= 1+s.rng.Intn(2); v++ {
			tn.vaults = append(tn.vaults, fmt.Sprintf("vault-%d-%d", ti, v))
		}
		for di := 1; di <= 1+s.rng.Intn(3); di++ {
			d := fmt.Sprintf("dev-%d", di)
			tn.devices = append(tn.devices, d)
			if s.rng.Intn(4) > 0 {
				id := fmt.Sprintf("conn-%d-%d", ti, di)
				tn.online[d] = id
				s.conns[id] = [2]string{tn.name, d}
			}
			for si := 1; si <= 1+s.rng.Intn(2); si++ {
				src := fmt.Sprintf("src-%d-%d-%d", ti, di, si)
				tn.sources[src] = d
				for vi, v := range tn.vaults {
					if vi > 0 && s.rng.Intn(2) == 0 {
						continue
					}
					rule := fmt.Sprintf("rule-%d-%d-%d-%d", ti, di, si, vi)
					tn.rules[rule] = [2]string{src, v}
					s.clock += 60
					s.seq++
					s.jobs = append(s.jobs, simJob{GUID: fmt.Sprintf("job-%d", s.seq), User: tn.name, Device: d, Source: src, Dest: v,
						Rule: rule, Status: statuses[s.rng.Intn(len(statuses))], Start: s.clock, Custom: s.rng.Intn(5) == 0})
				}
			}
		}
		s.tenants[tn.name] = tn
	}
	if len(s.rerunnable()) == 0 {
		// Guarantee work: fail the newest job of a rule on an online device.
		for i := range s.jobs {
			j := &s.jobs[i]
			if s.tenants[j.User].online[j.Device] != "" {
				j.Status, j.Custom = 7002, false
				break
			}
		}
	}
	if len(s.rerunnable()) == 0 {
		// Every device is offline: bring the first one online.
		j := s.jobs[0]
		id := "conn-forced"
		s.tenants[j.User].online[j.Device] = id
		s.conns[id] = [2]string{j.User, j.Device}
		s.jobs[0].Status, s.jobs[0].Custom = 7002, false
	}
}

// refail lets some running backups fail again: a new logical action that a
// later run may legitimately re-run once more.
func (s *simComet) refail() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tn := range s.tenants {
		for rule, sd := range tn.rules {
			j, ok := s.newest(tn.name, sd[0], sd[1])
			if ok && comet.Family(j.Status) == "running" && s.rng.Intn(3) == 0 {
				s.clock += 60
				s.seq++
				s.jobs = append(s.jobs, simJob{GUID: fmt.Sprintf("job-%d", s.seq), User: tn.name, Device: tn.sources[sd[0]],
					Source: sd[0], Dest: sd[1], Rule: rule, Status: 7002, Start: s.clock})
				s.logf("  %s/%s failed again as job-%d", tn.name, rule, s.seq)
			}
		}
	}
}

func (s *simComet) failf(t *testing.T, seed int64, format string, args ...any) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Errorf("seed %d: %s\ntrace:\n  %s", seed, fmt.Sprintf(format, args...), strings.Join(s.trace, "\n  "))
}

// ---- rerun-failed ---------------------------------------------------------------

type simRerunOut struct {
	Failed int `json:"failed"`
	Plan   []struct {
		Tenant   string `json:"tenant"`
		RuleID   string `json:"rule_id"`
		Action   string `json:"action"`
		NewJobID string `json:"new_job_id"`
	} `json:"plan"`
}

func decodeRerunOut(out string) (simRerunOut, bool) {
	var v simRerunOut
	dec := json.NewDecoder(strings.NewReader(out))
	if err := dec.Decode(&v); err != nil {
		return v, false
	}
	return v, v.Plan != nil
}

func rerunArgs(timeout string, allowPartial bool) []string {
	args := []string{"rerun-failed", "--window", "24h", "--yes", "--json", "--timeout", timeout, "--rate-limit", "0"}
	if allowPartial {
		args = append(args, "--allow-partial-failure")
	}
	return args
}

// checkRerunExit applies R2 to one finished run that started when the
// re-runnable set was before.
func (s *simComet) checkRerunExit(t *testing.T, seed int64, name string, exit int, out string, before []string, allowPartial bool) {
	t.Helper()
	s.mu.Lock()
	s.logf("%s exited %d", name, exit)
	view, decoded := decodeRerunOut(out)
	var problems []string
	if decoded {
		for _, r := range view.Plan {
			if r.Action != "dispatched" {
				continue
			}
			found := false
			for _, j := range s.jobs {
				if j.GUID == r.NewJobID && j.User == r.Tenant && j.Rule == r.RuleID {
					found = true
				}
			}
			if !found {
				problems = append(problems, fmt.Sprintf("%s reports %s/%s dispatched as %q, a job the server never started for it", name, r.Tenant, r.RuleID, r.NewJobID))
			}
		}
	}
	if exit == 0 && !allowPartial {
		if !decoded {
			problems = append(problems, fmt.Sprintf("%s exited 0 without a plan on stdout: %.400s", name, out))
		} else if view.Failed != 0 {
			problems = append(problems, fmt.Sprintf("%s exited 0 reporting %d failed dispatches", name, view.Failed))
		}
		for _, p := range before {
			tenant, rule, _ := strings.Cut(p, "/")
			sd := s.tenants[tenant].rules[rule]
			if j, ok := s.newest(tenant, sd[0], sd[1]); !ok || comet.Family(j.Status) != "running" {
				problems = append(problems, fmt.Sprintf("%s exited 0 but %s is not running on the server (newest %s %s)", name, p, j.GUID, comet.StatusName(j.Status)))
			}
		}
	}
	s.mu.Unlock()
	for _, p := range problems {
		s.failf(t, seed, "R2: %s", p)
	}
}

func runRerunSeed(t *testing.T, seed int64) {
	s := newSimComet(seed)
	s.buildRerunScenario()
	srv := httptest.NewUnstartedServer(s)
	conns := newSimConns()
	srv.Config.ConnState = conns.track
	srv.Start()
	defer srv.Close()
	if err := os.Setenv("COMET_BACKUP_BASE_URL", srv.URL); err != nil {
		t.Fatal(err)
	}

	// Phase 1: two runs in parallel.
	s.mu.Lock()
	before := s.rerunnable()
	s.logf("phase parallel: version %s, re-runnable %v", s.version, before)
	s.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		allow := s.pick(4) == 0
		wg.Add(1)
		go func(name string, allow bool) {
			defer wg.Done()
			exit, out := simRun(context.Background(), rerunArgs(simChaosTimeout, allow)...)
			s.checkRerunExit(t, seed, name, exit, out, before, allow)
		}(fmt.Sprintf("parallel-%d", i), allow)
	}
	wg.Wait()
	conns.settle(t)
	s.refail()

	// Phase 2: a run that crashes at a random request.
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	before = s.rerunnable()
	s.crash = cancel
	s.crashAt = s.reqN + 1 + s.rng.Intn(4+2*len(before))
	s.logf("phase crash at request #%d, re-runnable %v", s.crashAt, before)
	s.mu.Unlock()
	allow := s.pick(4) == 0
	exit, out := simRun(ctx, rerunArgs(simChaosTimeout, allow)...)
	cancel()
	conns.settle(t)
	s.mu.Lock()
	crashed := s.reqN >= s.crashAt
	s.crashAt, s.crash = 0, nil
	s.mu.Unlock()
	if crashed && exit == 0 && !allow {
		s.failf(t, seed, "R2: the crashed run exited 0")
	}
	if !crashed || allow {
		s.checkRerunExit(t, seed, "crash", exit, out, before, allow)
	}

	// Phase 3: the rerun after the crash.
	s.mu.Lock()
	before = s.rerunnable()
	s.logf("phase rerun, re-runnable %v", before)
	s.mu.Unlock()
	allow = s.pick(4) == 0
	exit, out = simRun(context.Background(), rerunArgs(simChaosTimeout, allow)...)
	conns.settle(t)
	s.checkRerunExit(t, seed, "rerun", exit, out, before, allow)

	// Phase 4: the server stops faulting; one more rerun must finish the job.
	s.mu.Lock()
	s.faultFree = true
	before = s.rerunnable()
	s.logf("phase converge, re-runnable %v", before)
	s.mu.Unlock()
	exit, out = simRun(context.Background(), rerunArgs(simCalmTimeout, false)...)
	s.checkRerunExit(t, seed, "converge", exit, out, before, false)
	s.mu.Lock()
	left := s.rerunnable()
	s.mu.Unlock()
	if exit != 0 || len(left) != 0 {
		s.failf(t, seed, "R3: rerun against a healthy server exited %d leaving %v re-runnable: %.600s", exit, left, out)
	}
	s.mu.Lock()
	violations := append([]string(nil), s.violations...)
	s.mu.Unlock()
	if len(violations) > 0 {
		s.failf(t, seed, "R1: %s", strings.Join(violations, "; "))
	}
}

// simConns counts the fake server's connections that are mid-request. A run
// that gave up on a reply (the lost or late reply the seeds inject) can leave a
// request the server has not handled yet. In production the dispatch-intent
// hold (30 minutes) refuses that rule while the straggler lands; the seeds set
// the hold to 0, so settle lets the straggler land before the next phase reads
// the job list. Without it, slow hosts (linux/amd64 under emulation) report a
// second dispatch that the hold prevents in real use.
type simConns struct {
	mu    sync.Mutex
	state map[net.Conn]simConn
}

type simConn struct{ st http.ConnState }

// simNewConnGrace is how long a connection that has sent nothing yet counts as
// a request about to land. The client's transport also opens connections it
// never uses; those stay new until they close, and are not waited for.
// ponytail: a fixed grace, not a proof; the straggler's bytes are already in
// the socket buffer when its run exits, so the server reads them well within it.
const simNewConnGrace = 500 * time.Millisecond

func newSimConns() *simConns { return &simConns{state: map[net.Conn]simConn{}} }

func (c *simConns) track(conn net.Conn, st http.ConnState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st == http.StateClosed || st == http.StateHijacked {
		delete(c.state, conn)
		return
	}
	c.state[conn] = simConn{st: st}
}

// busy counts connections mid-request: reading or handling one, or new
// (nothing read yet) and seen within the grace of start.
func (c *simConns) busy(start time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, sc := range c.state {
		if sc.st == http.StateActive || sc.st == http.StateNew && time.Since(start) < simNewConnGrace {
			n++
		}
	}
	return n
}

// settle waits until no connection is mid-request. A new connection gets the
// grace from the moment the phase ended, however long ago it was opened.
func (c *simConns) settle(t *testing.T) {
	t.Helper()
	start := time.Now()
	deadline := start.Add(10 * time.Second)
	for c.busy(start) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the fake server still has %d connection(s) mid-request after 10s", c.busy(start))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRerunFailedWriteSimulation: rerun-failed never dispatches a backup
// twice and never exits 0 with a dispatch unconfirmed (R1-R3 above).
func TestRerunFailedWriteSimulation(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	simIsolate(t)
	start := time.Now()
	for seed := int64(1); seed <= int64(simSeedCount()); seed++ {
		runRerunSeed(t, seed)
		if t.Failed() {
			return
		}
	}
	t.Logf("%d seeds in %s", simSeedCount(), time.Since(start).Round(time.Millisecond))
}

// ---- import ---------------------------------------------------------------------

// simImportModel is the fake server's model of one importable resource:
// how a JSONL record names the server record it creates.
type simImportModel struct {
	path   string
	record func(i int) map[string]string
	key    func(rec map[string]string) string
}

var simImportModels = map[string]simImportModel{
	"/api/v1/admin/add-user": {
		path: "add-user",
		record: func(i int) map[string]string {
			return map[string]string{"TargetUser": fmt.Sprintf("user-%d", i), "TargetPassword": "Pw-" + fmt.Sprint(i)}
		},
		key: func(rec map[string]string) string { return rec["TargetUser"] },
	},
	"/api/v1/admin/request-storage-vault": {
		path: "request-storage-vault",
		record: func(i int) map[string]string {
			return map[string]string{"TargetUser": "acme", "StorageProvider": fmt.Sprintf("prov-%d", i)}
		},
		key: func(rec map[string]string) string { return rec["TargetUser"] + "\x00" + rec["StorageProvider"] },
	},
}

// created is how many server records exist for key. Callers hold s.mu.
func (s *simComet) created(m simImportModel, key string) int {
	if m.path == "add-user" {
		if s.users[key] {
			return 1
		}
		return 0
	}
	return s.vaults[key]
}

func (s *simComet) createCount(m simImportModel, key string) int {
	if m.path == "add-user" {
		return s.userCreates[key]
	}
	return s.vaults[key]
}

func importArgs(resource, input, timeout string, idempotent bool) []string {
	args := []string{"import", resource, "--input", input, "--json", "--timeout", timeout, "--rate-limit", "0"}
	if idempotent {
		args = append(args, "--idempotent")
	}
	return args
}

func runImportSeed(t *testing.T, seed int64, resource string, m simImportModel, dir string) {
	s := newSimComet(seed)
	srv := httptest.NewServer(s)
	defer srv.Close()
	if err := os.Setenv("COMET_BACKUP_BASE_URL", srv.URL); err != nil {
		t.Fatal(err)
	}
	n := 2 + s.rng.Intn(6)
	keys := make([]string, 0, n)
	input := filepath.Join(dir, fmt.Sprintf("seed-%d-%s.jsonl", seed, resource))
	var lines []string
	for i := 1; i <= n; i++ {
		rec := m.record(i)
		keys = append(keys, m.key(rec))
		b, _ := json.Marshal(rec)
		lines = append(lines, string(b))
		if m.path == "add-user" && s.rng.Intn(5) == 0 {
			s.users[rec["TargetUser"]] = true // already on the server before the import
		}
	}
	if err := os.WriteFile(input, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkCreatedOnce := func(stage string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, k := range keys {
			if c := s.createCount(m, k); c > 1 {
				s.violate("I1 (%s): %q was created %d times", stage, k, c)
			}
		}
	}
	checkExit := func(name string, exit int, out string) {
		s.mu.Lock()
		s.logf("%s exited %d", name, exit)
		var missing []string
		if exit == 0 {
			for _, k := range keys {
				if s.created(m, k) == 0 {
					missing = append(missing, k)
				}
			}
		}
		s.mu.Unlock()
		if len(missing) > 0 {
			s.failf(t, seed, "I2: %s exited 0 but %q were never created: %.400s", name, missing, out)
		}
		checkCreatedOnce(name)
	}

	// Phase 1: two runs in parallel.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		idem := s.pick(2) == 0
		wg.Add(1)
		go func(name string, idem bool) {
			defer wg.Done()
			exit, out := simRun(context.Background(), importArgs(resource, input, simChaosTimeout, idem)...)
			checkExit(name, exit, out)
		}(fmt.Sprintf("parallel-%d", i), idem)
	}
	wg.Wait()

	// Phase 2: a run that crashes at a random request.
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.crash = cancel
	s.crashAt = s.reqN + 1 + s.rng.Intn(n+1)
	s.logf("phase crash at request #%d", s.crashAt)
	s.mu.Unlock()
	exit, out := simRun(ctx, importArgs(resource, input, simChaosTimeout, true)...)
	cancel()
	s.mu.Lock()
	crashed := s.reqN >= s.crashAt
	s.crashAt, s.crash = 0, nil
	s.mu.Unlock()
	if crashed && exit == 0 {
		s.failf(t, seed, "I2: the crashed import exited 0")
	}
	checkExit("crash", exit, out)

	// Phase 3: the rerun after the crash.
	exit, out = simRun(context.Background(), importArgs(resource, input, simChaosTimeout, true)...)
	checkExit("rerun", exit, out)

	// Phase 4: the server stops faulting; one more rerun must finish the job.
	s.mu.Lock()
	s.faultFree = true
	s.mu.Unlock()
	exit, out = simRun(context.Background(), importArgs(resource, input, simCalmTimeout, true)...)
	checkExit("converge", exit, out)
	s.mu.Lock()
	var wrong []string
	for _, k := range keys {
		if c := s.created(m, k); c != 1 {
			wrong = append(wrong, fmt.Sprintf("%q x%d", k, c))
		}
	}
	violations := append([]string(nil), s.violations...)
	s.mu.Unlock()
	if exit != 0 || len(wrong) > 0 {
		s.failf(t, seed, "I3: --idempotent rerun against a healthy server exited %d; records not present exactly once: %v: %.600s", exit, wrong, out)
	}
	if len(violations) > 0 {
		s.failf(t, seed, "%s", strings.Join(violations, "; "))
	}
}

// TestImportWriteSimulation: every resource import can write is simulated,
// and none is created twice or reported done while missing (I1-I3 above).
// A resource without a model here fails the test: add the model (how its
// records are keyed and whether the server refuses duplicates) or remove the
// resource from import.
func TestImportWriteSimulation(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	simIsolate(t)
	dir := t.TempDir()
	resources := make([]string, 0, len(resourceWritePaths))
	for r := range resourceWritePaths {
		resources = append(resources, r)
	}
	sort.Strings(resources)
	if len(resources) == 0 {
		t.Fatal("import writes no resource; this simulation no longer exercises it")
	}
	start := time.Now()
	for _, resource := range resources {
		t.Run(resource, func(t *testing.T) {
			m, ok := simImportModels[resourceWritePaths[resource]]
			if !ok {
				t.Fatalf("import %s writes %s, which this simulation does not model", resource, resourceWritePaths[resource])
			}
			for seed := int64(1); seed <= int64(simSeedCount()); seed++ {
				runImportSeed(t, seed, resource, m, dir)
				if t.Failed() {
					return
				}
			}
		})
	}
	t.Logf("%d resources x %d seeds in %s", len(resources), simSeedCount(), time.Since(start).Round(time.Millisecond))
}

// TestImportStorageIsNotImportable: request-storage-vault adds a new Storage
// Vault on every call and names nothing the server could refuse as a
// duplicate, so no crash-and-retry rule can make a bulk import of it act at
// most once (the simulation above failed I1 on it). import refuses the
// storage resource before reading input or sending anything; the MCP import
// tool runs this same command (cometblackbox TestImportStorageAbsentFromCLIAndMCP).
func TestImportStorageIsNotImportable(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	simIsolate(t)
	for r, p := range resourceWritePaths {
		if strings.Contains(p, "request-storage-vault") {
			t.Fatalf("import %s writes %s again", r, p)
		}
	}
	s := newSimComet(1)
	s.faultFree = true
	srv := httptest.NewServer(s)
	defer srv.Close()
	t.Setenv("COMET_BACKUP_BASE_URL", srv.URL)
	input := filepath.Join(t.TempDir(), "vaults.jsonl")
	if err := os.WriteFile(input, []byte(`{"TargetUser":"acme","StorageProvider":"prov-1"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"storage", "storage-vaults", "request-vault"} {
		exit, out := simRun(context.Background(), "import", resource, "--input", input, "--yes", "--json", "--idempotent")
		if exit != 2 || !strings.Contains(out, "unknown writable resource") {
			t.Errorf("import %s: exit %d, want the usage refusal (2): %.300s", resource, exit, out)
		}
	}
	// Control: the same input imports through a resource import does write.
	exit, out := simRun(context.Background(), "import", "users", "--input", input, "--json")
	s.mu.Lock()
	defer s.mu.Unlock()
	if exit == 2 || s.reqN != 1 {
		t.Errorf("control: import users sent %d request(s), exit %d: %.300s", s.reqN, exit, out)
	}
	if s.vaults["acme\x00prov-1"] != 0 {
		t.Errorf("a Storage Vault was created")
	}
}

// TestCometWritesNeverReplayed: one write call reaches the server at most
// once whatever the failure, which is what makes each single-request
// generated write act at most once per invocation. The client replays only
// what it may (client.go canRetryAmbiguousFailure); an Idempotency-Key or
// X-Idempotency-Key header in the config re-opened replay for every POST,
// even one marked not replay-safe, and Comet ignores those headers, so every
// Comet client drops them (comet-backup handfix: no-idempotency-replay). The
// control proves the header does cause a replay when the hook is skipped.
func TestCometWritesNeverReplayed(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	simIsolate(t)
	var mu sync.Mutex
	hits := 0
	fault := simRefuse5xx
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		f := fault
		mu.Unlock()
		if f == simDrop {
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
				}
			}
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	newC := func(header string, hooks bool) *client.Client {
		cfg := &config.Config{BaseURL: srv.URL, CometBackupUsername: "admin", CometBackupPassword: simPassword}
		if header != "" {
			cfg.Headers = map[string]string{header: "comet-sim-key"}
		}
		c := client.New(cfg, 2*time.Second, 0)
		if hooks {
			if err := ApplyClientHooks(c); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	send := func(c *client.Client, generated bool) int {
		mu.Lock()
		hits = 0
		mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		fields := url.Values{"TargetID": {"conn-1"}, "BackupRule": {"rule-1"}}
		if generated { // devices run-backup and every generated write
			_, _, _ = c.PostFormWithParamsAndHeaders(ctx, pathRunBackup, nil, fields, map[string]string{client.ReplaySafeHeader: "false"})
		} else { // rerun-failed
			_, _, _ = c.PostFormWithParams(ctx, pathRunBackup, nil, fields)
		}
		mu.Lock()
		defer mu.Unlock()
		return hits
	}
	for _, f := range []simFault{simRefuse5xx, simDrop} {
		mu.Lock()
		fault = f
		mu.Unlock()
		for _, header := range []string{"", "Idempotency-Key", "x-idempotency-key"} {
			for _, generated := range []bool{false, true} {
				if n := send(newC(header, true), generated); n != 1 {
					t.Errorf("%s with header %q (generated=%v): the write reached the server %d times", f, header, generated, n)
				}
			}
		}
	}
	// Control: without the hook the configured key makes the client replay.
	mu.Lock()
	fault = simRefuse5xx
	mu.Unlock()
	if n := send(newC("Idempotency-Key", false), true); n < 2 {
		t.Fatalf("control: an unhooked client with Idempotency-Key sent the write %d time(s); this test no longer exercises the replay", n)
	}
}

// Codex r1 + adversarial r1 (publish): two rerun-failed --yes runs reaching one
// Comet Server under different names (host vs host:443, 127.0.0.1 vs
// localhost) or from different COMET_BACKUP_HOME / XDG state dirs took
// different locks and dispatched the same backup twice. Every run on this
// host for this user takes the one lock.
func TestRerunDispatchLockIgnoresServerNameAndHome(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	simIsolate(t)
	holding := make(chan struct{})
	release := make(chan struct{})
	first := client.New(&config.Config{BaseURL: "https://backup.example.com"}, time.Second, 0)
	go func() {
		_ = withRerunDispatchLock(context.Background(), first, func() error {
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	defer close(release)
	for _, alias := range []string{"https://backup.example.com:443", "https://BACKUP.example.com/", "http://127.0.0.1:8060", "http://localhost:8060"} {
		t.Setenv("COMET_BACKUP_HOME", t.TempDir())
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		other := client.New(&config.Config{BaseURL: alias}, time.Second, 0)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		ran := false
		err := withRerunDispatchLock(ctx, other, func() error { ran = true; return nil })
		cancel()
		if ran || err == nil {
			t.Errorf("%s: a second run dispatched while the first held the lock (err=%v)", alias, err)
		}
	}
}
