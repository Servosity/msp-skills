// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-hands-the-kit-whole-records):
// export streams ordinary records through the credential kit's stream sinks
// (stdout, an --output file, the --deliver tee, the MCP mirror's child stdout).
// The kit checks each unit with the stream's tail as context and keeps a JSON
// document open across units; past its 16 KiB window an open document makes
// the stream lost and every later unit is withheld. A writer that hands the kit
// chunks ending mid-record (the press export's 4096-byte bufio.Writer) leaves a
// document open at every unit, so an ordinary export of a few hundred devices
// stopped after 20 KiB. Every record must arrive, and the run must succeed.

package credwire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// exportStreamKey is the held v3 key for these runs: low-entropy letters and
// dashes no record, path or message carries any 8-character piece of.
const exportStreamKey = "exportkey-mmmm-pppp-mmmm"

// exportDevices is past 64 KiB of JSON lines: four pages, and about five times
// the kit's 16 KiB stream window.
const exportDevices = 400

// exportHealth is the same for the health resource's shorter records.
const exportHealth = 1200

// exportMCPDevices fits the MCP text budget (bound.MaxBytes, 60000 bytes), so
// the whole export can come back in one tool result: three pages, and more
// than three times the kit's window.
const exportMCPDevices = 250

func exportDeviceRecord(i int) string {
	c := i % 7
	return fmt.Sprintf(`{"device_id": "d%d", "br_device_id": "br%d", "client": {"name": "C%d", "client_id": "c%d", "br_company_name": "C%d"}, "device_name": "dev%d", "br_device_name": "dev%d", "br_job_name": "job", "last_result": [], "history": []}`, i, i, c, c, c, i, i)
}

func exportHealthRecord(i int) string {
	return fmt.Sprintf(`{"client": {"name": "C%d", "client_id": "c%d"}, "history": []}`, i, i)
}

// exportPager answers the v3 device and health lists the way Backup Radar
// pages them: page_size, an offset cursor, next_cursor null on the last page.
func exportPager(w http.ResponseWriter, r *http.Request, _ seen) {
	var total int
	var rec func(int) string
	switch {
	case strings.HasSuffix(r.URL.Path, "/clients/devices"):
		total, rec = exportDevices, exportDeviceRecord
	case strings.HasSuffix(r.URL.Path, "/clients/health"):
		total, rec = exportHealth, exportHealthRecord
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"total_count":0,"next_cursor":null}`))
		return
	}
	off, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size <= 0 {
		size = 100
	}
	hi := min(off+size, total)
	items := make([]string, 0, hi-off)
	for i := off; i < hi; i++ {
		items = append(items, rec(i))
	}
	next := "null"
	if hi < total {
		next = strconv.Quote(strconv.Itoa(hi))
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data": [%s], "next_cursor": %s, "total_count": %d}`, strings.Join(items, ", "), next, total)
}

func exportWant(n int, rec func(int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = rec(i)
	}
	return out
}

// split runs the CLI with stdout and stderr kept apart, under a timeout.
func (e *env) split(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cliBin, _ := binaries(t)
	cmd := exec.Command(cliBin, args...)
	cmd.Env = e.vars
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(180 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v timed out; stderr:\n%s", args, errb.String())
	}
	return out.String(), errb.String(), cmd.ProcessState.ExitCode()
}

// sameLines fails unless got is want, one record per line, in order.
func sameLines(t *testing.T, where, got string, want []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if got == "" {
		lines = nil
	}
	complete := 0
	for complete < len(lines) && complete < len(want) && lines[complete] == want[complete] {
		complete++
	}
	if complete != len(want) || len(lines) != len(want) || !strings.HasSuffix(got, "\n") {
		tail := got
		if len(tail) > 160 {
			tail = tail[len(tail)-160:]
		}
		t.Fatalf("%s: %d bytes, %d lines, the first %d of %d records whole and in order; tail %q", where, len(got), len(lines), complete, len(want), tail)
	}
}

// sameArray fails unless got is a JSON array of exactly want.
func sameArray(t *testing.T, where, got string, want []string) {
	t.Helper()
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(got), &items); err != nil {
		t.Fatalf("%s: not one JSON array (%v): %d bytes, tail %q", where, err, len(got), got[max(0, len(got)-160):])
	}
	if len(items) != len(want) {
		t.Fatalf("%s: %d of %d records", where, len(items), len(want))
	}
	for i, it := range items {
		var g, w any
		if json.Unmarshal(it, &g) != nil || json.Unmarshal([]byte(want[i]), &w) != nil || fmt.Sprint(g) != fmt.Sprint(w) {
			t.Fatalf("%s: record %d is %s, want %s", where, i, it, want[i])
		}
	}
}

func noKitRefusal(t *testing.T, where, stderr string) {
	t.Helper()
	if strings.Contains(stderr, "credkit") || strings.Contains(stderr, "withheld") {
		t.Fatalf("%s: the kit refused ordinary records:\n%s", where, stderr)
	}
}

// TestExportStreamsEveryRecord: export of ordinary, credential-free records
// past 64 KiB, in every format and to every destination, delivers every record
// and exits 0.
func TestExportStreamsEveryRecord(t *testing.T) {
	devices := exportWant(exportDevices, exportDeviceRecord)
	health := exportWant(exportHealth, exportHealthRecord)
	for name, recs := range map[string][]string{"devices": devices, "health": health} {
		if n := len(strings.Join(recs, "\n")); n <= 64<<10 {
			t.Fatalf("the %s fixture is %d bytes; it must pass 64 KiB to prove anything", name, n)
		}
	}
	e := newEnv(t, exportPager, "BACKUP_RADAR_API_KEY="+exportStreamKey)

	t.Run("stdout jsonl", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "devices")
		noKitRefusal(t, "stdout", errOut)
		sameLines(t, "stdout", out, devices)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("stdout jsonl health", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "health")
		noKitRefusal(t, "stdout", errOut)
		sameLines(t, "stdout", out, health)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("stdout json", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "devices", "--format", "json")
		noKitRefusal(t, "stdout", errOut)
		sameArray(t, "stdout", out, devices)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("output jsonl", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "devices.jsonl")
		out, errOut, code := e.split(t, "export", "devices", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--output file", string(b), devices)
		if code != 0 {
			t.Fatalf("exit %d; stdout %q; stderr:\n%s", code, out, errOut)
		}
	})
	t.Run("output jsonl health", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "health.jsonl")
		_, errOut, code := e.split(t, "export", "health", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--output file", string(b), health)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("output json", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "devices.json")
		_, errOut, code := e.split(t, "export", "devices", "--format", "json", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameArray(t, "--output file", string(b), devices)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("deliver file", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "delivered.jsonl")
		out, errOut, code := e.split(t, "export", "devices", "--deliver", "file:"+path)
		noKitRefusal(t, "--deliver", errOut)
		sameLines(t, "--deliver stdout tee", out, devices)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("nothing delivered: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--deliver file", string(b), devices)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("mcp", func(t *testing.T) {
		want := devices[:exportMCPDevices]
		if n := len(strings.Join(want, "\n")); n <= 3*(16<<10) || n >= 60000 {
			t.Fatalf("the MCP fixture is %d bytes; it must pass three kit windows and fit the MCP budget", n)
		}
		replies, logs := e.mcp(t,
			map[string]any{"name": "export", "arguments": map[string]any{"resource": "devices", "limit": exportMCPDevices}},
			map[string]any{"name": "export", "arguments": map[string]any{"resource": "devices"}},
		)
		text, isErr := mcpText(t, replies[1])
		if isErr {
			t.Fatalf("MCP export of %d devices is an error: %.400s\nserver:\n%.2000s", exportMCPDevices, text, logs)
		}
		sameLines(t, "MCP export", strings.TrimRight(text, "\n")+"\n", want)
		// Past the MCP budget the result is the bound's explicit preview,
		// never an error and never a silent cut.
		text, isErr = mcpText(t, replies[2])
		if isErr {
			t.Fatalf("MCP export of %d devices is an error: %.400s", exportDevices, text)
		}
		var preview struct {
			Truncated bool `json:"_pp_truncated"`
		}
		if json.Unmarshal([]byte(text), &preview) != nil || !preview.Truncated {
			t.Fatalf("MCP export past the 60000-byte budget is neither whole nor the explicit truncated preview: %d bytes, head %.200q", len(text), text)
		}
	})
}

// mcpText is a tools/call reply's first content block (the command's stdout;
// stderr hints follow in their own block) and isError.
func mcpText(t *testing.T, reply string) (string, bool) {
	t.Helper()
	var msg struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(reply), &msg); err != nil {
		t.Fatalf("no tools/call reply (%v): %.400s", err, reply)
	}
	if len(msg.Result.Content) == 0 {
		return "", msg.Result.IsError
	}
	return msg.Result.Content[0].Text, msg.Result.IsError
}
