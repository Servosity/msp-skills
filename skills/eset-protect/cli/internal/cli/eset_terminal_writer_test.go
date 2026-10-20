// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// spoofName is the adversarial-review receipt: an OSC 52 clipboard write, a
// BEL, then CR + ESC[2K so the terminal erases the real name and shows
// HARMLESS-KIOSK. The suffix adds C1 CSI, DEL, backspace and a bidi override.
const spoofName = "DC01\u001b]52;c;ZWNobyBwd25lZA==\u0007\r\u001b[2KHARMLESS-KIOSK\u009b2J\u007f\b\u202eXX"

// terminalLeak returns the first control byte in out that a terminal would
// act on, after removing the CLI's own allowlisted style sequences.
func terminalLeak(out []byte) (string, bool) {
	clean := out
	for _, seq := range cliStyleSequences {
		clean = bytes.ReplaceAll(clean, seq, nil)
	}
	for i := 0; i < len(clean); i++ {
		b := clean[i]
		switch {
		case b == '\r' && (i+1 == len(clean) || clean[i+1] != '\n'):
			return "bare CR", true
		case b < 0x20 && b != '\t' && b != '\n' && b != '\r', b == 0x7f:
			return "C0/DEL " + string(rune('0'+b/16)) + string("0123456789abcdef"[b%16]), true
		case b == 0xC2 && i+1 < len(clean) && clean[i+1] >= 0x80 && clean[i+1] <= 0x9f:
			return "C1", true
		case b == 0xE2 && i+2 < len(clean) && isBidiControl(clean[i+1], clean[i+2]):
			return "bidi control", true
		}
	}
	return "", false
}

func TestTerminalWriterNeutralizesControls(t *testing.T) {
	in := []byte(spoofName + "\r\nnext\tline\n")
	var whole bytes.Buffer
	if _, err := (&c1EscapeWriter{w: &whole}).Write(in); err != nil {
		t.Fatal(err)
	}
	if what, leak := terminalLeak(whole.Bytes()); leak {
		t.Fatalf("%s survived the writer: %q", what, whole.Bytes())
	}
	for _, want := range []string{`DC01\u001b]52;c;ZWNobyBwd25lZA==\u0007\u000d\u001b[2KHARMLESS-KIOSK`, `\u009b2J\u007f\u0008\u202eXX`, "\r\nnext\tline\n"} {
		if !strings.Contains(whole.String(), want) {
			t.Fatalf("output %q lacks %q", whole.String(), want)
		}
	}
	// Every split point gives the same bytes: lead bytes, CR and a style
	// prefix carried across Writes.
	for cut := 0; cut <= len(in); cut++ {
		var split bytes.Buffer
		w := &c1EscapeWriter{w: &split}
		_, _ = w.Write(in[:cut])
		_, _ = w.Write(in[cut:])
		if split.String() != whole.String() {
			t.Fatalf("split at %d changed the output:\n%q\n%q", cut, split.String(), whole.String())
		}
	}
	// encoding/json output stays valid and decodes to the same value.
	var all []rune
	for r := rune(0); r < 0xa0; r++ {
		all = append(all, r)
	}
	all = append(all, 0x202a, 0x202e, 0x2066, 0x2069, 'Z', 0xfc, 0x2014)
	enc, _ := json.Marshal(map[string]string{"name": string(all)})
	var out bytes.Buffer
	if _, err := (&c1EscapeWriter{w: &out}).Write(append(enc, '\n')); err != nil {
		t.Fatal(err)
	}
	if what, leak := terminalLeak(out.Bytes()); leak {
		t.Fatalf("%s survived in JSON: %q", what, out.Bytes())
	}
	var back map[string]string
	if err := json.Unmarshal(out.Bytes(), &back); err != nil || back["name"] != string(all) {
		t.Fatalf("JSON changed through the writer: %v %q", err, out.Bytes())
	}
}

// TestTerminalWriterKeepsCLIStyles: the CLI's own colors pass only while
// colors are on, and only as the exact sequences its helpers emit.
func TestTerminalWriterKeepsCLIStyles(t *testing.T) {
	savedHF, savedNC := humanFriendly, noColor
	t.Cleanup(func() { humanFriendly, noColor = savedHF, savedNC })
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	humanFriendly, noColor = true, false
	styled := bold("H") + red("FAIL") + green("OK") + yellow("WARN")
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("colors did not turn on; the check proves nothing")
	}
	var out bytes.Buffer
	// Flush: while a secret is registered the writer holds an unfinished
	// line until its newline or Flush (secret-registry-masks-every-boundary).
	sw := &c1EscapeWriter{w: &out}
	_, _ = sw.Write([]byte(styled + "\x1b]52;c;eA==\x07\x1b[2K\x1b[8m"))
	_ = sw.Flush()
	if !strings.HasPrefix(out.String(), styled) {
		t.Fatalf("CLI styles were rewritten: %q", out.String())
	}
	if !strings.HasSuffix(out.String(), `\u001b]52;c;eA==\u0007\u001b[2K\u001b[8m`) {
		t.Fatalf("a non-CLI sequence passed while colors were on: %q", out.String())
	}
	humanFriendly = false
	out.Reset()
	sw = &c1EscapeWriter{w: &out}
	_, _ = sw.Write([]byte("\x1b[31mRED\x1b[0m"))
	_ = sw.Flush()
	if out.String() != `\u001b[31mRED\u001b[0m` {
		t.Fatalf("with colors off every ESC must be escaped: %q", out.String())
	}
}

// captureExecuteAll runs the real Execute with stdout and stderr swapped for
// pipes, so the writers Execute installs are the ones under test.
func captureExecuteAll(t *testing.T, args ...string) (stdout, stderr []byte, err error) {
	t.Helper()
	or, ow, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	er, ew, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	oldOut, oldErr, oldArgs := os.Stdout, os.Stderr, os.Args
	os.Stdout, os.Stderr = ow, ew
	os.Args = append([]string{"eset-protect-cli"}, args...)
	outC, errC := make(chan []byte), make(chan []byte)
	go func() { b, _ := io.ReadAll(or); outC <- b }()
	go func() { b, _ := io.ReadAll(er); errC <- b }()
	savedHF, savedNC := humanFriendly, noColor
	err = Execute()
	humanFriendly, noColor = savedHF, savedNC
	_ = ow.Close()
	_ = ew.Close()
	os.Stdout, os.Stderr, os.Args = oldOut, oldErr, oldArgs
	return <-outC, <-errC, err
}

// seedSpoofMirror writes a mirror whose device, customer, detection and
// incident names all carry spoofName.
func seedSpoofMirror(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spoof.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	put := func(resource, id string, v map[string]any) {
		raw, _ := json.Marshal(v)
		if err := db.Upsert(resource, id, raw); err != nil {
			t.Fatal(err)
		}
	}
	put("device-groups", "msp", map[string]any{"uuid": "msp", "displayName": "MSP", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_MSP"})
	put("device-groups", "c1", map[string]any{"uuid": "c1", "displayName": "Acme" + spoofName, "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType})
	put("device-groups", "c2", map[string]any{"uuid": "c2", "displayName": "Acme Two" + spoofName, "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType})
	for _, d := range []struct{ id, group string }{{"00000000-0000-0000-0000-000000000003", "c1"}, {"00000000-0000-0000-0000-000000000004", "c2"}} {
		put("devices", d.id, map[string]any{"uuid": d.id, "displayName": spoofName, "parentGroupUuid": d.group, "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)})
		put("detections", "det-"+d.id, map[string]any{"uuid": "det-" + d.id, "displayName": spoofName, "objectName": spoofName, "objectHashSha1": strings.Repeat("a", 40), "occurTime": now.Add(-time.Hour).Format(time.RFC3339), "severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": d.id}})
	}
	put("incidents", "inc-1", map[string]any{"uuid": "inc-1", "displayName": spoofName, "severity": "INCIDENT_SEVERITY_LEVEL_HIGH", "status": "INCIDENT_STATUS_OPEN", "createTime": now.Add(-time.Hour).Format(time.RFC3339), "deviceUuids": []string{"00000000-0000-0000-0000-000000000003"}})
	for _, r := range []string{"devices", "device-groups", "detections", "incidents"} {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestExecuteNeutralizesTenantControlBytes drives the real Execute through
// the hand-written human printers (forced with --human-friendly, with and
// without colors) and the customer-resolution error on stderr: no control
// byte from a tenant name reaches either stream, and the respond isolate
// preview shows the targeted device's whole name.
func TestExecuteNeutralizesTenantControlBytes(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no ESET call expected, got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	})
	db := seedSpoofMirror(t)
	bs := string(rune(92))
	// The hand-written printers flatten a tenant CR to a space before the
	// terminal writer runs (human-printers-flatten-tenant-lines); ESC and BEL
	// still arrive escaped.
	visible := `DC01\u001b]52;c;ZWNobyBwd25lZA==\u0007 \u001b[2KHARMLESS-KIOSK`
	for _, noColorEnv := range []string{"", "1"} {
		t.Setenv("NO_COLOR", noColorEnv)
		t.Setenv("TERM", "xterm-256color")
		for _, argv := range [][]string{
			{"fleet", "health", "--db", db},
			{"fleet", "stale", "--db", db},
			{"triage", "--since", "1w", "--db", db},
			{"detections", "spread", "--since", "1w", "--db", db},
			{"respond", "isolate", "00000000-0000-0000-0000-000000000003", "--db", db},
		} {
			out, errOut, err := captureExecuteAll(t, append(argv, "--human-friendly")...)
			for stream, b := range map[string][]byte{"stdout": out, "stderr": errOut} {
				if what, leak := terminalLeak(b); leak {
					t.Fatalf("NO_COLOR=%q %v: %s reached %s: %q", noColorEnv, argv, what, stream, b)
				}
			}
			// Tables clip long cells, but the escaped controls stay visible;
			// the respond preview, which a human approves, shows it whole.
			want := []byte(`DC01` + bs + `u001b]52`)
			if argv[0] == "respond" {
				want = []byte(visible)
			}
			if !bytes.Contains(out, want) {
				t.Fatalf("NO_COLOR=%q %v: the escaped name is not visible (err=%v): %q\nstderr: %q", noColorEnv, argv, err, out, errOut)
			}
		}
	}
	// "Acme" matches both customers: the error lists both names on stderr.
	_, errOut, err := captureExecuteAll(t, "fleet", "health", "--customer", "Acme", "--db", db)
	if err == nil {
		t.Fatal("an ambiguous customer must fail")
	}
	if what, leak := terminalLeak(errOut); leak {
		t.Fatalf("%s reached stderr in the customer-resolution error: %q", what, errOut)
	}
	if !bytes.Contains(errOut, []byte("matches 2 customers")) || !bytes.Contains(errOut, []byte(visible)) {
		t.Fatalf("customer-resolution error lost its text: %q", errOut)
	}
}

// binaryPayload holds every byte the terminal writer rewrites, inside a file
// that is not valid JSON (like a real quarantine zip).
var binaryPayload = []byte("PK\x03\x04\x1b]52;c;eA==\x07\r\x1b[2K\xc2\x9b2J\xe2\x80\xae\x7f\x00\"deviceToken\":\"keep-me\"\r\n\xff\xfe")

// TestBinaryDownloadsAreByteExact: the quarantine downloads reach stdout, a
// --deliver file and the --json envelope exactly as ESET sent them, whatever
// Content-Type ESET labels them with (its spec says application/json), and
// are never written to a terminal.
func TestBinaryDownloadsAreByteExact(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	contentType := ""
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v1/quarantined-objects:") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(binaryPayload)
	})
	for _, ct := range []string{"application/octet-stream", "application/zip", "application/json", ""} {
		contentType = ct
		for _, command := range []string{"quarantined-objects-download", "quarantined-objects-batch-download"} {
			args := []string{command}
			if command == "quarantined-objects-batch-download" {
				args = append(args, "--object-uuids", "00000000-0000-0000-0000-0000000000c1")
			}
			out, errOut, err := captureExecuteAll(t, args...)
			if err != nil {
				t.Fatalf("%s (%q): %v\n%s", command, ct, err, errOut)
			}
			if !bytes.Equal(out, binaryPayload) {
				t.Fatalf("%s (Content-Type %q): stdout is not the server's bytes:\n got %q\nwant %q", command, ct, out, binaryPayload)
			}
			file := filepath.Join(t.TempDir(), "sample.zip")
			if _, errOut, err := captureExecuteAll(t, append(args, "--deliver", "file:"+file)...); err != nil {
				t.Fatalf("%s --deliver: %v\n%s", command, err, errOut)
			}
			if got, _ := os.ReadFile(file); !bytes.Equal(got, binaryPayload) {
				t.Fatalf("%s (Content-Type %q): --deliver file is not the server's bytes: %q", command, ct, got)
			}
			out, errOut, err = captureExecuteAll(t, append(args, "--json")...)
			if err != nil {
				t.Fatalf("%s --json: %v\n%s", command, err, errOut)
			}
			var env struct {
				Data string `json:"data"`
			}
			if jerr := json.Unmarshal(out, &env); jerr != nil {
				t.Fatalf("%s --json is not one JSON object: %v %q", command, jerr, out)
			}
			if got, _ := base64.StdEncoding.DecodeString(env.Data); !bytes.Equal(got, binaryPayload) {
				t.Fatalf("%s (Content-Type %q): --json envelope does not decode to the server's bytes", command, ct)
			}
		}
	}
	// The null device (/dev/null, NUL on Windows) is a character device but
	// not a terminal: '> /dev/null' takes the download (terminal-check-is-a-real-tty).
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skip(err)
	}
	defer null.Close()
	run := func(target *os.File) error {
		cmd := RootCmd()
		cmd.SetOut(&c1EscapeWriter{w: target})
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"quarantined-objects-download"})
		return cmd.Execute()
	}
	if err := run(null); err != nil {
		t.Fatalf("a binary download to %s must be written, not refused: %v", os.DevNull, err)
	}
	// A real terminal is refused, and the advice leads with --deliver file:
	// (byte-exact everywhere) before the macOS/Linux redirect.
	orig := esetTerminalFile
	esetTerminalFile = func(f *os.File) bool { return f == null }
	defer func() { esetTerminalFile = orig }()
	err = run(null)
	if err == nil || !strings.Contains(err.Error(), "refusing to write") {
		t.Fatalf("a binary download to a terminal must be refused, got %v", err)
	}
	msg := err.Error()
	deliver, ps, redirect := strings.Index(msg, "--deliver file:<path>"), strings.Index(msg, "PowerShell 5.1"), strings.Index(msg, "> sample.zip")
	if deliver < 0 || ps < 0 || redirect < 0 || !(deliver < ps && ps < redirect) || !strings.Contains(msg, "on macOS and Linux") {
		t.Fatalf("refusal advice must lead with --deliver file:, warn about PowerShell 5.1, then give the macOS/Linux redirect: %q", msg)
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("terminal refusal exit code = %d, want 2", code)
	}
}

// TestTerminalCheckIsARealTTY: the real predicate says no to the null device
// and to a pipe, both of which the old character-device test got wrong or
// right only by accident.
func TestTerminalCheckIsARealTTY(t *testing.T) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skip(err)
	}
	defer null.Close()
	if fi, serr := null.Stat(); serr == nil && fi.Mode()&os.ModeCharDevice == 0 && runtime.GOOS != "windows" {
		t.Fatalf("%s is expected to be a character device on this platform", os.DevNull)
	}
	if isTerminal(null) || isTerminal(&c1EscapeWriter{w: null}) {
		t.Fatalf("%s is a character device, not a terminal", os.DevNull)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) {
		t.Fatal("a pipe is not a terminal")
	}
}

// TestExportWritesThroughTerminalWriter: export used os.Stdout directly, so a
// raw C1 byte in a device name reached the terminal.
func TestExportWritesThroughTerminalWriter(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"devices\":[{\"uuid\":\"00000000-0000-0000-0000-000000000001\",\"displayName\":\"PC\u009b2J\u007f01\"}],\"nextPageToken\":\"\"}")
	})
	for _, format := range []string{"jsonl", "json"} {
		out, errOut, err := captureExecuteAll(t, "export", "devices", "--format", format)
		if err != nil {
			t.Fatalf("export --format %s: %v\n%s", format, err, errOut)
		}
		if what, leak := terminalLeak(out); leak {
			t.Fatalf("export --format %s: %s reached stdout: %q", format, what, out)
		}
		if !bytes.Contains(out, []byte(`PC\u009b2J\u007f01`)) {
			t.Fatalf("export --format %s lost the name: %q", format, out)
		}
	}
}
