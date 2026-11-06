// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (fixed-scripts).

package winsrv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hostile are values that would change a script if they were ever
// concatenated into it.
var hostile = []string{
	"Spooler; Remove-Item C:\\ -Recurse",
	"Spooler'; whoami; '",
	"Spooler\"; whoami; \"",
	"$(whoami)",
	"`whoami`",
	"Spooler\nwhoami",
	"Spooler|whoami",
	"Spoo*",
	"..\\..\\Windows",
	"Spooler\u2019; whoami",
	"Spooler -Force",
	"@{a=1}",
	strings.Repeat("A", 81),
	"",
}

func TestEveryArgumentRefusesInjection(t *testing.T) {
	for _, name := range Names() {
		s, _ := Lookup(name)
		for _, p := range s.Params {
			for _, h := range hostile {
				if _, err := s.Validate(map[string]any{p.Name: h}); err == nil {
					t.Errorf("%s.%s accepted %q", name, p.Name, h)
				}
			}
			for _, v := range []any{1.5, -1, 1 << 40, true, []any{"x"}, map[string]any{}, nil, []int{0}, []int{5}, []int{1, 2, 3, 4, 1}} {
				if p.Kind == "bool" && v == true {
					continue
				}
				if _, err := s.Validate(map[string]any{p.Name: v}); err == nil {
					t.Errorf("%s.%s accepted %#v", name, p.Name, v)
				}
			}
		}
		if _, err := s.Validate(map[string]any{"command": "whoami"}); err == nil {
			t.Errorf("%s accepted an unknown argument", name)
		}
	}
}

func TestValidArgumentsPass(t *testing.T) {
	cases := map[string]map[string]any{
		"service_get":         {"name": "MSSQL$SQLEXPRESS"},
		"eventlog":            {"log": "Application", "hours": 720, "levels": []int{1, 2}, "max": 2000},
		"scm_events":          {"days": 30},
		"local_admin_changes": {"hours": 1},
		"scheduled_tasks":     {"include_microsoft": true},
		"certificates":        {"store": "Remote Desktop"},
		"eventlog_groups":     {"window_hours": 168, "baseline_hours": 720},
		"service_plan":        {"name": "Spooler", "action": "restart"},
	}
	for name, args := range cases {
		s, _ := Lookup(name)
		if _, err := s.Validate(args); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestEncodeIsUTF16LittleEndianBase64 pins the -EncodedCommand form,
// including a surrogate pair, so the bootstrap's script hash never moves.
func TestEncodeIsUTF16LittleEndianBase64(t *testing.T) {
	if got, want := Encode("Get-Date; A€\U0001D11E"), "RwBlAHQALQBEAGEAdABlADsAIABBAKwgNNge3Q=="; got != want {
		t.Fatalf("Encode = %q, want %q", got, want)
	}
	if Encode("") != "" {
		t.Fatal("empty script must encode to nothing")
	}
}

// TestCommandLineIsConstantAndBounded proves the command line and script
// payload are a function of the script name alone, and fit WinRS's
// 8191-character command-line limit.
func TestCommandLineIsConstantAndBounded(t *testing.T) {
	for _, name := range Names() {
		exe, argv, text, err := CommandLine(name)
		if err != nil {
			t.Fatal(err)
		}
		_, again, text2, _ := CommandLine(name)
		if strings.Join(argv, " ") != strings.Join(again, " ") || text != text2 || exe != "powershell.exe" {
			t.Fatalf("%s: command line is not constant", name)
		}
		if n := len(exe) + 1 + len(strings.Join(argv, " ")); n > 8191 {
			t.Errorf("%s: command line is %d characters (WinRS limit 8191)", name, n)
		}
		if len(text) > 32000 {
			t.Errorf("%s: script payload is %d characters (environment limit 32767)", name, len(text))
		}
		raw, _ := Assemble(name)
		for _, r := range raw {
			if r > 127 {
				t.Errorf("%s: non-ASCII character %q in the script", name, r)
				break
			}
		}
	}
}

func TestScriptsUseNoHereStringsOrInvokeExpression(t *testing.T) {
	for _, name := range Names() {
		text, _ := Assemble(name) // comments stripped: only code is checked
		src := strings.ToLower(text)
		for _, bad := range []string{"@\"", "@'", "invoke-expression", "iex ", "[scriptblock]::create", "-force", "add-type", "start-process", "invoke-command", "$executioncontext"} {
			if strings.Contains(src, bad) {
				t.Errorf("%s contains %q", name, bad)
			}
		}
	}
}

func TestMutatingScriptNeedsConfirmation(t *testing.T) {
	f := &FixtureRunner{Outputs: map[string]string{"service_action": `{"ok":true,"data":{}}`}}
	p := Profile{Name: "fs01"}
	args := map[string]any{"name": "Spooler", "action": "restart"}
	if _, err := f.Run(context.Background(), p, "service_action", args); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("want ErrNotConfirmed, got %v", err)
	}
	if len(f.Calls) != 0 {
		t.Fatal("an unconfirmed mutation reached the runner")
	}
	if _, err := f.Run(WithConfirmedMutation(context.Background()), p, "service_action", args); err != nil {
		t.Fatal(err)
	}
	for _, name := range Names() {
		s, _ := Lookup(name)
		if s.Mutating != (name == "service_action") {
			t.Errorf("%s: Mutating = %v", name, s.Mutating)
		}
	}
}

func pwsh(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"pwsh", "/opt/homebrew/bin/pwsh", "/usr/local/bin/pwsh"} {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	t.Skip("pwsh not installed")
	return ""
}

func TestScriptsParse(t *testing.T) {
	bin := pwsh(t)
	for _, name := range Names() {
		text, err := Assemble(name)
		if err != nil {
			t.Fatal(err)
		}
		check := `$e=$null;$t=$null;[void][System.Management.Automation.Language.Parser]::ParseInput($env:WSPP_TEXT,[ref]$t,[ref]$e);if($e.Count){$e|%{$_.Message};exit 1}`
		cmd := exec.Command(bin, "-NoProfile", "-NonInteractive", "-Command", check)
		cmd.Env = append(os.Environ(), "WSPP_TEXT="+text)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s does not parse: %s", name, out)
		}
	}
}

// TestPowerShellSideRefusesHostileArguments sends each hostile value straight
// to the script (bypassing Go validation) and requires the script itself to
// refuse it before any cmdlet runs.
func TestPowerShellSideRefusesHostileArguments(t *testing.T) {
	bin := pwsh(t)
	for _, name := range Names() {
		s, _ := Lookup(name)
		for _, p := range s.Params {
			if p.Kind == "bool" {
				continue
			}
			values := []string{"Spooler; whoami", "$(whoami)", "Spoo*", "Spooler -Force", "999999", "-1"}
			if p.Kind == "name" {
				values = values[:4] // digits and a dash are a valid service name
			}
			for _, h := range values {
				_, argv, script, _ := CommandLine(name)
				args := map[string]any{"name": "Spooler", "action": "restart"}
				args[p.Name] = h
				payload, _ := json.Marshal(args)
				cmd := exec.Command(bin, argv...)
				cmd.Env = append(os.Environ(), ArgsEnv+"="+base64.StdEncoding.EncodeToString(payload), ScriptEnv+"="+script)
				out, _ := cmd.Output()
				raw, err := Decode(name, "test", out, nil, 0)
				if err == nil || !IsKind(err, KindInvalidArg) {
					t.Errorf("%s.%s=%q: want invalid_argument, got %v %s", name, p.Name, h, err, raw)
				}
			}
		}
	}
}

func TestDecodeFindsLastEnvelopeAndKinds(t *testing.T) {
	raw, err := Decode("x", "s", []byte(`noise {"ok":true,"data":[1]} trailing`), nil, 0)
	if err == nil || !strings.Contains(err.Error(), "continued after") {
		t.Fatalf("trailing garbage after the envelope accepted: %s", raw)
	}
	raw, err = Decode("x", "s", []byte("WARNING: noise\r\n{\"ok\":true,\"data\":[1]}"), nil, 0)
	if err != nil || string(raw) != "[1]" {
		t.Fatalf("got %s %v", raw, err)
	}
	_, err = Decode("x", "s", []byte(`{"ok":false,"error":{"kind":"role_absent","message":"no"}}`), nil, 0)
	if !IsKind(err, KindRoleAbsent) {
		t.Fatalf("got %v", err)
	}
	_, err = Decode("x", "s", nil, []byte("#< CLIXML\r\n<Objs><S S=\"Error\">boom_x000D__x000A_</S></Objs>"), 1)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
}

func TestRowsAddsServerScopedIDsAndComputedFields(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	f := &FixtureRunner{Outputs: map[string]string{
		"certificates":    `{"ok":true,"data":[{"store":"My","thumbprint":"AB","not_after":"2026-11-07T12:00:00Z"}]}`,
		"scheduled_tasks": `{"ok":true,"data":{"task_path":"\\","task_name":"Backup","state":"Ready","last_result":2147942402,"last_run_time":"2026-10-27T00:00:00Z"}}`,
		"defender":        `{"ok":true,"data":{"quick_scan_age_days":4294967295,"signature_age_days":1}}`,
		"time_sync":       `{"ok":true,"data":{"service_state":"Running","lines":["Leap Indicator: 0(no warning)","Stratum: 4 (secondary reference - syncd by (S)NTP)","Phase Offset: -0.0012345s","Source: 169.254.169.123,0x9","Last Successful Sync Time: 28/10/2026 06:58:01"],"last_sync_utc":"2026-10-28T11:58:01.0000000Z"}}`,
		"volumes":         `{"ok":true,"data":{"value":[{"drive_letter":"C","percent_free":50}],"Count":1}}`,
	}}
	p := Profile{Name: "fs01"}
	certs, _ := Rows(context.Background(), f, p, "certificates", map[string]any{"store": "My"}, now)
	if certs[0]["id"] != "fs01/My/AB" || certs[0]["days_left"] != 10 || certs[0]["server"] != "fs01" {
		t.Fatalf("certificates: %v", certs[0])
	}
	tasks, _ := Rows(context.Background(), f, p, "scheduled_tasks", nil, now)
	if tasks[0]["last_result_hex"] != "0x80070002" || tasks[0]["last_result_ok"] != false || !TaskFailed(tasks[0]) {
		t.Fatalf("tasks: %v", tasks[0])
	}
	def, _ := Rows(context.Background(), f, p, "defender", nil, now)
	if def[0]["quick_scan_age_days"] != nil || def[0]["signature_age_days"] != 1.0 {
		t.Fatalf("defender: %v", def[0])
	}
	ts, _ := Rows(context.Background(), f, p, "time_sync", nil, now)
	if ts[0]["synchronized"] != true || ts[0]["stratum"] != 4 || ts[0]["phase_offset_seconds"] != -0.0012345 || ts[0]["last_successful_sync"] != "2026-10-28T11:58:01Z" {
		t.Fatalf("time_sync: %v", ts[0])
	}
	vols, _ := Rows(context.Background(), f, p, "volumes", nil, now)
	if len(vols) != 1 || vols[0]["id"] != "fs01/C" {
		t.Fatalf("volumes: %v", vols)
	}
}

func TestHealthVerdicts(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	g := Gathered{
		Rows: map[string][]Row{
			"system":            {{"pending_reboot": false}},
			"volumes":           {{"drive_letter": "C", "percent_free": 4.0, "health_status": "Healthy"}},
			"services":          {{"name": "Spooler", "start_mode": "Auto", "state": "Stopped", "exit_code": 1077.0}, {"name": "sppsvc", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": true, "exit_code": 0.0}, {"name": "gupdate", "start_mode": "Auto", "state": "Stopped", "trigger_start": true}},
			"eventlog_critical": {},
			"eventlog_errors":   {},
			"installed_updates": {{"installed_on": "2026-10-20T00:00:00Z"}},
			"pending_updates":   {},
			"time_sync":         {{"service_state": "Running", "source": "time.windows.com", "synchronized": true, "phase_offset_seconds": 0.1}},
			"certificates":      {{"subject": "CN=fs01", "has_private_key": true, "days_left": -100, "not_after": "2026-07-01"}, {"subject": "CN=fs01", "has_private_key": true, "days_left": 200, "not_after": "2027-05-01"}},
			"scheduled_tasks":   {},
		},
		Errs: map[string]error{
			"defender":   &ScriptError{Kind: KindRoleAbsent, Message: "not installed"},
			"hyperv_vms": &ScriptError{Kind: KindRoleAbsent, Message: "no role"},
		},
	}
	checks := Evaluate(g, now)
	got := map[string]string{}
	for _, c := range checks {
		got[c.Name] = c.Status
	}
	want := map[string]string{"reboot": "ok", "disks": "crit", "services": "warn", "system_events": "ok", "patch_age": "ok",
		"pending_updates": "ok", "defender": "n/a", "time_sync": "ok", "certificates": "crit", "scheduled_tasks": "ok", "hyperv": "n/a"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
	if Verdict(checks) != StatusCrit {
		t.Fatalf("verdict %s", Verdict(checks))
	}
	if Verdict([]Check{{Status: StatusWarn}, {Status: StatusUnknown}, {Status: StatusNA}}) != StatusUnknown {
		t.Fatal("unknown must outrank warn")
	}
	// An unreachable server: every probe skipped with the first error.
	unreachable := Gather(context.Background(), &FixtureRunner{}, Profile{Name: "x"}, now)
	if len(unreachable.Errs) != len(healthProbes) {
		t.Fatalf("errs %d", len(unreachable.Errs))
	}
}

func TestDiffReportsChangesAndSkipsUnreadSections(t *testing.T) {
	old := map[string]map[string]map[string]any{
		"local_admins": {"fs01/S-1": {"name": "CORP\\alice"}},
		"shares":       {"fs01/data": {"path": "D:\\data"}},
	}
	cur := map[string]map[string]map[string]any{
		"local_admins": {"fs01/S-1": {"name": "CORP\\alice"}, "fs01/S-2": {"name": "CORP\\mallory"}},
		"shares":       {"fs01/data": {"path": "E:\\data"}},
	}
	changes, notCompared := Diff(old, cur)
	if len(changes) != 2 || changes[0].Change != "added" || changes[0].Key != "fs01/S-2" || changes[1].Change != "changed" {
		t.Fatalf("changes %+v", changes)
	}
	if len(notCompared) != len(Sections)-2 {
		t.Fatalf("not compared %v", notCompared)
	}
}

// TestBootstrapRefusesAnyOtherScript sends the bootstrap with an altered,
// a different and a missing script payload; each must be refused unrun.
func TestBootstrapRefusesAnyOtherScript(t *testing.T) {
	bin := pwsh(t)
	_, argv, script, _ := CommandLine("system")
	_, _, other, _ := CommandLine("shares")
	text, _ := Assemble("system")
	tampered := base64.StdEncoding.EncodeToString([]byte(text + "\n[Console]::Out.Write('PWNED')"))
	for label, payload := range map[string]string{"altered": tampered, "other script": other, "missing": "", "garbage": "!!"} {
		cmd := exec.Command(bin, argv...)
		cmd.Env = append(os.Environ(), ScriptEnv+"="+payload)
		out, _ := cmd.Output()
		if strings.Contains(string(out), "PWNED") {
			t.Fatalf("%s: the altered script ran", label)
		}
		if _, err := Decode("system", "t", out, nil, 0); !IsKind(err, KindRefused) {
			t.Errorf("%s: want refused, got %v (%s)", label, err, out)
		}
	}
	// The real payload passes the check (and then fails on macOS, where
	// Get-CimInstance does not exist): anything but "refused".
	cmd := exec.Command(bin, argv...)
	cmd.Env = append(os.Environ(), ScriptEnv+"="+script)
	out, _ := cmd.Output()
	if _, err := Decode("system", "t", out, nil, 0); IsKind(err, KindRefused) {
		t.Fatalf("the genuine script was refused: %s", out)
	}
}

// TestLabFixtures runs the post-processing, health and inventory over
// outputs recorded from a Windows Server 2022 lab (testdata/lab), so the
// parsers are held to real output shapes, not only hand-written ones.
func TestLabFixtures(t *testing.T) {
	f := &FixtureRunner{Outputs: map[string]string{}}
	files, _ := filepath.Glob(filepath.Join("testdata", "lab", "*.json"))
	if len(files) < 19 {
		t.Fatalf("want 19 lab fixtures, found %d", len(files))
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Outputs[strings.TrimSuffix(filepath.Base(path), ".json")] = string(b)
	}
	now := time.Date(2026, 10, 28, 9, 10, 0, 0, time.UTC)
	p := Profile{Name: "lab"}
	ctx := context.Background()
	one := func(script string, args map[string]any) []Row {
		t.Helper()
		rows, err := Rows(ctx, f, p, script, args, now)
		if err != nil {
			t.Fatalf("%s: %v", script, err)
		}
		return rows
	}
	if r := one("time_sync", nil)[0]; r["last_successful_sync"] != "2026-10-28T09:04:02Z" || r["synchronized"] != true || r["stratum"] != 2 {
		t.Errorf("time_sync: %v", r)
	}
	if r := one("defender", nil)[0]; r["quick_scan_age_days"] != nil || r["signature_age_days"] != float64(0) {
		t.Errorf("defender never-scanned sentinel: %v", r)
	}
	if r := one("volumes", nil)[0]; r["id"] != "lab/C" || r["percent_free"] == nil {
		t.Errorf("volumes: %v", r)
	}
	for _, r := range one("certificates", map[string]any{"store": "Root"}) {
		if r["not_before"] == nil || r["days_left"] == nil {
			t.Errorf("certificate without dates: %v", r)
		}
	}
	failed := 0
	for _, r := range one("scheduled_tasks", map[string]any{"include_microsoft": true}) {
		if TaskFailed(r) {
			failed++
		}
		if !strings.HasPrefix(Str(r["last_result_hex"]), "0x") {
			t.Errorf("task without hex result: %v", r)
		}
	}
	if failed == 0 {
		t.Error("the lab has failing Microsoft tasks; none were found")
	}
	if _, err := Rows(ctx, f, p, "hyperv_vms", nil, now); !IsKind(err, KindRoleAbsent) {
		t.Errorf("hyperv_vms on a server without the role: %v", err)
	}
	checks := Evaluate(Gather(ctx, f, p, now), now)
	byName := map[string]Check{}
	for _, c := range checks {
		byName[c.Name] = c
	}
	for name, want := range map[string]string{"reboot": StatusOK, "disks": StatusOK, "services": StatusWarn, "defender": StatusOK, "time_sync": StatusOK, "hyperv": StatusNA, "certificates": StatusOK} {
		if got := byName[name].Status; got != want {
			t.Errorf("%s: %s (%s), want %s", name, got, byName[name].Reason, want)
		}
	}
	for _, c := range checks {
		if c.Status == StatusUnknown {
			t.Errorf("%s is unknown on real output: %s", c.Name, c.Reason)
		}
	}
	snap := TakeSnapshot(ctx, f, p, now)
	if len(snap.Errors) != 0 || len(snap.Sections["services"]) < 100 {
		t.Errorf("snapshot: errors %v, %d services", snap.Errors, len(snap.Sections["services"]))
	}
}

// TestTaskResultHexIsTheLow32Bits: LastTaskResult arrives as an unsigned or
// a signed 32-bit HRESULT; both read as the same 0x8007... code.
func TestTaskResultHexIsTheLow32Bits(t *testing.T) {
	for _, n := range []float64{2147942402, -2147024894} {
		task := Row{"task_path": `\`, "task_name": "Backup", "last_result": n, "last_run_time": "2026-10-27T00:00:00Z"}
		taskPost(task, time.Now())
		if task["last_result_hex"] != "0x80070002" || task["last_result_ok"] != false {
			t.Fatalf("last_result %v: hex %v ok %v", n, task["last_result_hex"], task["last_result_ok"])
		}
	}
	task := Row{"task_path": `\`, "task_name": "Backup", "last_result": float64(0x41301), "last_run_time": "2026-10-27T00:00:00Z"}
	taskPost(task, time.Now())
	if task["last_result_hex"] != "0x00041301" || task["last_result_ok"] != true {
		t.Fatalf("running task: hex %v ok %v", task["last_result_hex"], task["last_result_ok"])
	}
}

// TestUnreadableDetailIsNotOK: a task whose last result could not be read,
// or a VM whose checkpoints could not be listed, is not reported healthy.
func TestUnreadableDetailIsNotOK(t *testing.T) {
	task := Row{"task_path": `\`, "task_name": "Backup", "state": "Ready", "info_error": "access denied"}
	taskPost(task, time.Now())
	if st, why := checkTasks([]Row{task}); st != StatusWarn || !strings.Contains(why, "unreadable") {
		t.Fatalf("unreadable task info: %s %q", st, why)
	}
	vm := Row{"name": "vm1", "checkpoint_error": "access denied"}
	if st, why := checkHyperV([]Row{vm}); st != StatusWarn || !strings.Contains(why, "unreadable") {
		t.Fatalf("unreadable checkpoints: %s %q", st, why)
	}
}

// TestBasicOverHTTPIsRefusedInConfig: the password would cross in clear text.
func TestBasicOverHTTPIsRefusedInConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "[[servers]]\nname = \"fs01\"\nhost = \"fs01\"\ntransport = \"http\"\nauth = \"basic\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfiles(path); err == nil || !strings.Contains(err.Error(), "clear text") {
		t.Fatalf("basic over http accepted: %v", err)
	}
}

// TestFailureExitCodeWinsOverStartupMetadata is the round-1 review's input
// (a trigger-start service that stopped with 1067) and its class: start
// metadata explains a clean stop, never a failure code.
func TestFailureExitCodeWinsOverStartupMetadata(t *testing.T) {
	for _, r := range []Row{
		{"name": "W3SVC", "start_mode": "Auto", "state": "Stopped", "trigger_start": true, "exit_code": 1067.0},
		{"name": "MSSQL", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": true, "exit_code": 1067.0},
		{"name": "Both", "start_mode": "Auto", "state": "Stopped", "trigger_start": true, "delayed_auto_start": true, "exit_code": 1066.0},
	} {
		// A failure code counts with or without a known uptime, and inside
		// the delayed-start window too.
		for _, b := range []Boot{{}, {UptimeSeconds: 60, Known: true}} {
			if !ServiceDown(r, b) {
				t.Errorf("%s stopped with %v and is not down (boot %+v)", r["name"], r["exit_code"], b)
			}
			if s, _ := checkServices([]Row{r}, b); s != StatusCrit {
				t.Errorf("%s: services = %s, want crit (boot %+v)", r["name"], s, b)
			}
		}
	}
	for _, r := range []Row{
		{"name": "gupdate", "start_mode": "Auto", "state": "Stopped", "trigger_start": true, "exit_code": 0.0},
		{"name": "never", "start_mode": "Auto", "state": "Stopped", "trigger_start": true, "exit_code": 1077.0},
		{"name": "manual", "start_mode": "Manual", "state": "Stopped", "exit_code": 1067.0},
	} {
		if ServiceDown(r, Boot{}) {
			t.Errorf("%s stopped by design and is reported down", r["name"])
		}
	}
}

// TestCleanlyStoppedDelayedServiceCounts is round 1's input: a delayed
// automatic service with no triggers, started and then stopped normally
// (exit code 0), after the delayed-start window. Only the bounded window
// after boot excuses a delayed service; a clean stop does not.
func TestCleanlyStoppedDelayedServiceCounts(t *testing.T) {
	r := Row{"name": "Spooler", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": true, "trigger_start": false, "exit_code": 0.0}
	for _, b := range []Boot{{}, {UptimeSeconds: 3600, Known: true}, {UptimeSeconds: 60, Known: true}} {
		if !ServiceDown(r, b) {
			t.Errorf("cleanly stopped delayed service is not down (boot %+v)", b)
		}
		if s, why := checkServices([]Row{r}, b); s != StatusWarn || !strings.Contains(why, "Spooler") {
			t.Errorf("services = %s (%s), want warn naming Spooler (boot %+v)", s, why, b)
		}
	}
}

// TestEveryPrivateKeyCertificateCounts is the round-1 review's input (an
// expired certificate left behind by a renewal) and its class: a binding
// names a thumbprint, not a subject, so no certificate a service could use
// is hidden by another one.
func TestEveryPrivateKeyCertificateCounts(t *testing.T) {
	rows := []Row{
		{"subject": "CN=fs01", "thumbprint": "OLD1", "has_private_key": true, "days_left": -100, "not_after": "2026-07-01"},
		{"subject": "CN=fs01", "thumbprint": "NEW1", "has_private_key": true, "days_left": 200, "not_after": "2027-05-01"},
		{"subject": "CN=root", "thumbprint": "ROOT", "has_private_key": false, "days_left": -5, "not_after": "2026-10-23"},
	}
	s, reason := checkCerts(rows)
	if s != StatusCrit || !strings.Contains(reason, "OLD1") || strings.Contains(reason, "ROOT") {
		t.Fatalf("certificates = %s (%s), want crit naming OLD1 only", s, reason)
	}
	if s, _ := checkCerts(rows[1:]); s != StatusOK {
		t.Fatalf("a current certificate alone = %s", s)
	}
}

// cappedEventLog answers the eventlog script the way Get-WinEvent does: the
// query filter first, then -MaxEvents over the newest matches.
type cappedEventLog struct{ events []Row } // newest first

func (c cappedEventLog) Run(_ context.Context, _ Profile, name string, args map[string]any) (json.RawMessage, error) {
	if name != "eventlog" {
		return nil, &ScriptError{Kind: KindRoleAbsent, Message: "not read in this test"}
	}
	levels := map[int]bool{}
	for _, l := range args["levels"].([]int) {
		levels[l] = true
	}
	out := []Row{}
	for _, e := range c.events {
		if lv, _ := Num(e["level"]); levels[int(lv)] && len(out) < args["max"].(int) {
			out = append(out, e)
		}
	}
	return json.Marshal(out)
}

// TestCappedEventReadsNeverHideAFinding is the round-1 class for health: a
// newest-first capped read must not let 600 newer errors crowd an older
// critical event out of the System events check.
func TestCappedEventReadsNeverHideAFinding(t *testing.T) {
	var events []Row
	for i := 0; i < 600; i++ {
		events = append(events, Row{"log": "System", "record_id": 5000 - i, "level": 2.0, "provider": "Disk", "event_id": 153})
	}
	events = append(events, Row{"log": "System", "record_id": 7, "level": 1.0, "provider": "Microsoft-Windows-Kernel-Power", "event_id": 41})
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	found := false
	for _, c := range Evaluate(Gather(context.Background(), cappedEventLog{events}, Profile{Name: "fs01"}, now), now) {
		if c.Name == "system_events" {
			found = true
			if c.Status != StatusCrit || !strings.Contains(c.Reason, "Kernel-Power 41") {
				t.Fatalf("system_events = %s (%s): newer errors hid a critical event", c.Status, c.Reason)
			}
		}
	}
	if !found {
		t.Fatal("no system_events check")
	}
}

// TestAdminChangesFilterBeforeTheCap runs local_admin_changes under pwsh
// with Get-WinEvent mocked the way it behaves (the query filter, then
// -MaxEvents over the newest matches): 1000 newer changes to other groups
// must not hide an Administrators addition, and a read that hits its cap
// says so.
func TestAdminChangesFilterBeforeTheCap(t *testing.T) {
	bin := pwsh(t)
	text, err := Assemble("local_admin_changes")
	if err != nil {
		t.Fatal(err)
	}
	mock := `
function New-MockEvent([int]$id, [int64]$rec, [string]$target) {
  $xml = "<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><EventData><Data Name='TargetSid'>$target</Data><Data Name='MemberSid'>S-1-5-21-9-9-9-1105</Data><Data Name='SubjectUserSid'>S-1-5-21-9-9-9-500</Data><Data Name='SubjectDomainName'>CORP</Data><Data Name='SubjectUserName'>mallory</Data></EventData></Event>"
  $o = [pscustomobject]@{ Id = $id; RecordId = $rec; TimeCreated = (Get-Date).AddHours(-1); Xml = $xml }
  $o | Add-Member -MemberType ScriptMethod -Name ToXml -Value { $this.Xml }
  return $o
}
$script:MockEvents = New-Object System.Collections.ArrayList
for ($i = 0; $i -lt [int]$env:MOCK_OTHER; $i++) { [void]$script:MockEvents.Add((New-MockEvent 4732 (5000 - $i) 'S-1-5-32-555')) }
for ($i = 0; $i -lt [int]$env:MOCK_ADMIN; $i++) { [void]$script:MockEvents.Add((New-MockEvent 4732 (3000 - $i) 'S-1-5-32-544')) }
function Get-WinEvent {
  [CmdletBinding()] param($FilterHashtable, [string]$LogName, [string]$FilterXPath, [int]$MaxEvents)
  $ev = @($script:MockEvents)
  if ($FilterXPath.Contains("Data[@Name='TargetSid']='S-1-5-32-544'")) { $ev = @($ev | Where-Object { $_.Xml.Contains("'TargetSid'>S-1-5-32-544<") }) }
  return @($ev | Select-Object -First $MaxEvents)
}
function auditpol.exe { 'Machine Name,Policy Target,Subcategory,Subcategory GUID,Inclusion Setting,Exclusion Setting'; ''; 'FS01,System,Security Group Management,{0CCE9237-69AE-11D9-BED3-505054503030},Success,' }
function Get-LocalGroupMember { [CmdletBinding()] param([string]$SID) }
`
	path := filepath.Join(t.TempDir(), "admin_changes.ps1")
	if err := os.WriteFile(path, []byte(mock+text), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(other, admin string) (changes int, capped bool) {
		t.Helper()
		cmd := exec.Command(bin, "-NoProfile", "-NonInteractive", "-File", path)
		cmd.Env = append(os.Environ(), "WSPP_ARGS="+base64.StdEncoding.EncodeToString([]byte(`{"hours":168}`)), "MOCK_OTHER="+other, "MOCK_ADMIN="+admin)
		out, _ := cmd.Output()
		data, err := Decode("local_admin_changes", "t", out, nil, 0)
		if err != nil {
			t.Fatalf("other=%s admin=%s: %v (%s)", other, admin, err, out)
		}
		var res struct {
			Capped  bool             `json:"capped"`
			Changes []map[string]any `json:"changes"`
		}
		if err := json.Unmarshal(data, &res); err != nil {
			t.Fatal(err)
		}
		return len(res.Changes), res.Capped
	}
	if n, capped := run("1000", "1"); n != 1 || capped {
		t.Errorf("1000 newer other-group changes: %d Administrators changes (capped %v), want 1 uncapped", n, capped)
	}
	if n, capped := run("0", "1000"); n != 1000 || !capped {
		t.Errorf("1000 Administrators changes: %d (capped %v), want 1000 and capped", n, capped)
	}
}

// TestServiceStateFilterUsesOneVocabulary is the round-1 review's input (a
// state filter in the spec's words, StopPending, over Win32_Service rows that
// say "Stop Pending") and its class: the filter, the rows and the single-row
// read all use one vocabulary.
func TestServiceStateFilterUsesOneVocabulary(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	f := &FixtureRunner{Outputs: map[string]string{
		"services":    `{"ok":true,"data":[{"name":"A","state":"Stop Pending","start_mode":"Auto"},{"name":"B","state":"Start Pending","start_mode":"Manual"},{"name":"C","state":"Running","start_mode":"Auto"}]}`,
		"service_get": `{"ok":true,"data":{"name":"B","state":"Continue Pending","start_mode":"Manual"}}`,
	}}
	p := Profile{Name: "fs01"}
	for state, want := range map[string]string{"StopPending": "A", "StartPending": "B", "Running": "C"} {
		body, err := Read(context.Background(), f, p, "/services", url.Values{"state": {state}}, now)
		if err != nil {
			t.Fatal(err)
		}
		var rows []Row
		if err := json.Unmarshal(body, &rows); err != nil || len(rows) != 1 || rows[0]["name"] != want || rows[0]["state"] != state {
			t.Errorf("state=%s: %s, want only %s in the same words", state, body, want)
		}
	}
	body, err := Read(context.Background(), f, p, "/services/B", nil, now)
	if err != nil || !strings.Contains(string(body), `"state":"ContinuePending"`) {
		t.Errorf("services/B: %s (%v)", body, err)
	}
}

// labRowsAt reads one recorded lab fixture (testdata/lab) through Rows, so a
// test can change a field the way the server reported it at another moment.
func labRowsAt(t *testing.T, script string, now time.Time) []Row {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "lab", script+".json"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := Rows(context.Background(), &FixtureRunner{Outputs: map[string]string{script: string(b)}}, Profile{Name: "lab"}, script, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestDelayedStartWindow is the lab's input: about two minutes after a
// start, health flagged DPS, MSDTC, UALSVC and UsoSvc (Automatic (Delayed
// Start), exit code 1077: not started since boot) as stopped automatic
// services; minutes later they were running and only the steady-state stops
// remained. Its class: a delayed automatic service the Service Control
// Manager has not started yet is not down while the uptime read in the same
// run is inside the delayed-start window. Outside the window, for a service
// that is not delayed, for a failure exit code, and whenever the uptime is
// unknown, the stop still counts (fail toward reporting).
func TestDelayedStartWindow(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 10, 0, 0, time.UTC)
	booting := []string{"DPS", "MSDTC", "UALSVC", "UsoSvc"}
	// The lab's services as the server reported them two minutes after boot.
	labAtBoot := labRowsAt(t, "services", now)
	for _, r := range labAtBoot {
		for _, name := range booting {
			if r["name"] == name {
				if r["delayed_auto_start"] != true || r["start_mode"] != "Auto" {
					t.Fatalf("%s is not Automatic (Delayed Start) in the lab fixture: %v", name, r)
				}
				r["state"], r["exit_code"], r["process_id"] = "Stopped", 1077.0, 0.0
			}
		}
	}
	system := func(uptime any) []Row {
		r := labRowsAt(t, "system", now)
		if uptime == nil {
			delete(r[0], "uptime_seconds")
		} else {
			r[0]["uptime_seconds"] = uptime
		}
		return r
	}
	services := func(g Gathered) Check {
		t.Helper()
		for _, c := range Evaluate(g, now) {
			if c.Name == "services" {
				return c
			}
		}
		t.Fatal("no services check")
		return Check{}
	}
	gathered := func(sys []Row, sysErr error, svc []Row) Gathered {
		g := Gathered{Rows: map[string][]Row{"services": svc}, Errs: map[string]error{}}
		if sysErr != nil {
			g.Errs["system"] = sysErr
		} else if sys != nil {
			g.Rows["system"] = sys
		}
		return g
	}
	// stopped is the list of services the reason calls stopped.
	stopped := func(reason string) []string {
		_, list, ok := strings.Cut(reason, "stopped: ")
		if !ok {
			return nil
		}
		list, _, _ = strings.Cut(list, ";")
		return strings.Split(list, ", ")
	}
	names := func(list []string, want ...string) bool {
		for _, w := range want {
			found := false
			for _, n := range list {
				found = found || n == w
			}
			if !found {
				return false
			}
		}
		return true
	}
	// services down's help text says "less than 10 minutes"; change both.
	if delayedStartWindow != 10*time.Minute {
		t.Fatalf("delayedStartWindow is %s; update the services down help text with it", delayedStartWindow)
	}
	threeMin, threeHours := 180.0, 3*3600.0
	delayedNeverStarted := Row{"name": "DPS", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": true, "trigger_start": false, "exit_code": 1077.0}

	// (1) Inside the window: the four are "not started yet", not stopped.
	c := services(gathered(system(threeMin), nil, labAtBoot))
	for _, name := range booting {
		if names(stopped(c.Reason), name) {
			t.Errorf("uptime 3 min: %s is called stopped: %s", name, c.Reason)
		}
		if !strings.Contains(c.Reason, name) {
			t.Errorf("uptime 3 min: %s is hidden instead of reported as not started yet: %s", name, c.Reason)
		}
	}
	if !strings.Contains(c.Reason, "not started yet") {
		t.Errorf("uptime 3 min: the reason does not say the delayed services have not started yet: %s", c.Reason)
	}
	if !names(stopped(c.Reason), "AWSLiteAgent", "Amazon EC2Launch") || c.Status != StatusWarn {
		t.Errorf("uptime 3 min: the steady-state stops must still count: %s (%s)", c.Status, c.Reason)
	}
	if c := services(gathered(system(threeMin), nil, []Row{delayedNeverStarted})); c.Status != StatusOK || !strings.Contains(c.Reason, "DPS") {
		t.Errorf("uptime 3 min, delayed 1077 only: %s (%s), want ok naming DPS as not started yet", c.Status, c.Reason)
	}

	// (2) After the window the same stops count.
	c = services(gathered(system(threeHours), nil, labAtBoot))
	if !names(stopped(c.Reason), booting...) || c.Status != StatusWarn {
		t.Errorf("uptime 3 h: %s (%s), want warn naming %v", c.Status, c.Reason, booting)
	}
	for _, uptime := range []float64{threeHours, 600, 601} {
		if c := services(gathered(system(uptime), nil, []Row{delayedNeverStarted})); c.Status != StatusWarn || !names(stopped(c.Reason), "DPS") {
			t.Errorf("uptime %vs, delayed 1077: %s (%s), want warn", uptime, c.Status, c.Reason)
		}
	}

	// (3) Inside the window, a stop the delayed start does not explain
	// still counts: not delayed, or a failure exit code.
	for _, r := range []Row{
		{"name": "Spooler", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": false, "trigger_start": false, "exit_code": 1077.0},
		{"name": "Spooler", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": false, "trigger_start": false, "exit_code": 0.0},
	} {
		if c := services(gathered(system(threeMin), nil, []Row{r})); c.Status != StatusWarn || !names(stopped(c.Reason), "Spooler") {
			t.Errorf("uptime 3 min, not delayed, exit %v: %s (%s), want warn", r["exit_code"], c.Status, c.Reason)
		}
	}
	failed := Row{"name": "MSSQL", "start_mode": "Auto", "state": "Stopped", "delayed_auto_start": true, "trigger_start": false, "exit_code": 1067.0}
	if c := services(gathered(system(threeMin), nil, []Row{failed})); c.Status != StatusCrit {
		t.Errorf("uptime 3 min, delayed with 1067: %s (%s), want crit", c.Status, c.Reason)
	}

	// The shared predicate services down uses agrees with the check, and
	// NeedsBoot asks for the uptime only when a row could depend on it.
	inWindow, after := BootOf(system(threeMin)), BootOf(system(threeHours))
	if !inWindow.Known || inWindow.UptimeSeconds != threeMin || !after.Known {
		t.Fatalf("BootOf on the lab system row: %+v %+v", inWindow, after)
	}
	if ServiceDown(delayedNeverStarted, inWindow) || !NotStartedYet(delayedNeverStarted, inWindow) {
		t.Error("uptime 3 min: the delayed 1077 service is down by the shared predicate")
	}
	if !ServiceDown(delayedNeverStarted, after) || NotStartedYet(delayedNeverStarted, after) || !ServiceDown(delayedNeverStarted, Boot{}) {
		t.Error("uptime 3 h or unknown: the delayed 1077 service is not down by the shared predicate")
	}
	if !NeedsBoot(labAtBoot) || NeedsBoot([]Row{failed}) || NeedsBoot(labRowsAt(t, "services", now)) {
		t.Error("NeedsBoot must be true only when a delayed automatic service has not started since boot")
	}

	// (4) Unknown uptime: no system read, a failed read, a withheld row, no
	// value, a value that is not a number, a negative value.
	for label, g := range map[string]Gathered{
		"system not read":    gathered(nil, nil, []Row{delayedNeverStarted}),
		"system read failed": gathered(nil, &ScriptError{Kind: "failed", Message: "boom"}, []Row{delayedNeverStarted}),
		"system withheld":    gathered([]Row{{"withheld": "credential"}}, nil, []Row{delayedNeverStarted}),
		"no uptime":          gathered(system(nil), nil, []Row{delayedNeverStarted}),
		"uptime a string":    gathered(system("180"), nil, []Row{delayedNeverStarted}),
		"negative uptime":    gathered(system(-1.0), nil, []Row{delayedNeverStarted}),
	} {
		if c := services(g); c.Status != StatusWarn || !names(stopped(c.Reason), "DPS") {
			t.Errorf("%s: %s (%s), want warn naming DPS", label, c.Status, c.Reason)
		}
	}
}
