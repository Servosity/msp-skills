// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-hands-the-kit-whole-records):
// export streams ordinary records through the credential kit's stream sinks
// (stdout, an --output file, the --deliver tee, the MCP mirror's child stdout).
// The kit checks each unit with the stream's tail as context and keeps a JSON
// document open across units; past its 16 KiB window an open document makes
// the stream lost and every later unit is withheld. A writer that hands the kit
// chunks ending mid-record (the press export's 4096-byte bufio.Writer) leaves a
// document open at every unit, so an ordinary export stopped after about 20
// KiB. Every record must arrive, and the run must succeed.

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

// exportStreamKey is the held key for these runs: low-entropy letters and
// dashes no record, path or message carries any 8-character piece of.
const exportStreamKey = "exportkey-mmmm-pppp-mmmm"

// exportInstallations is past 64 KiB of JSON lines: several pages, and about
// five times the kit's 16 KiB stream window.
const exportInstallations = 750

// exportMCPInstallations fits the MCP text budget (60000 bytes), so the whole
// export can come back in one tool result, and passes three kit windows.
const exportMCPInstallations = 550

func exportInstallationRecord(i int) string {
	return fmt.Sprintf(`{"tenant_id":"tnt-%d","org_id":"org-%d","name":"Installation %d","kind":"o365","region":"us"}`, i, i%7, i)
}

// exportPager answers the installations list the way Afi pages it: limit, an
// opaque page_token, next_page_token empty on the last page.
func exportPager(w http.ResponseWriter, r *http.Request, _ seen) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/api/v1/applications/me/installations" {
		_, _ = w.Write([]byte(`{"items":[],"next_page_token":""}`))
		return
	}
	off, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Query().Get("page_token"), "p"))
	size, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if size <= 0 {
		size = 100
	}
	hi := min(off+size, exportInstallations)
	items := make([]string, 0, hi-off)
	for i := off; i < hi; i++ {
		items = append(items, exportInstallationRecord(i))
	}
	next := ""
	if hi < exportInstallations {
		next = "p" + strconv.Itoa(hi)
	}
	_, _ = fmt.Fprintf(w, `{"items":[%s],"next_page_token":%q}`, strings.Join(items, ","), next)
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
	recs := exportWant(exportInstallations, exportInstallationRecord)
	if n := len(strings.Join(recs, "\n")); n <= 64<<10 {
		t.Fatalf("the fixture is %d bytes; it must pass 64 KiB to prove anything", n)
	}
	e := newEnv(t, exportPager, "AFI_API_KEY="+exportStreamKey)

	t.Run("stdout jsonl", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "applications")
		noKitRefusal(t, "stdout", errOut)
		sameLines(t, "stdout", out, recs)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("stdout json", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "applications", "--format", "json")
		noKitRefusal(t, "stdout", errOut)
		sameArray(t, "stdout", out, recs)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("output jsonl", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "applications.jsonl")
		out, errOut, code := e.split(t, "export", "applications", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--output file", string(b), recs)
		if code != 0 {
			t.Fatalf("exit %d; stdout %q; stderr:\n%s", code, out, errOut)
		}
	})
	t.Run("output json", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "applications.json")
		_, errOut, code := e.split(t, "export", "applications", "--format", "json", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameArray(t, "--output file", string(b), recs)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("deliver file", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "delivered.jsonl")
		out, errOut, code := e.split(t, "export", "applications", "--deliver", "file:"+path)
		noKitRefusal(t, "--deliver", errOut)
		sameLines(t, "--deliver stdout tee", out, recs)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("nothing delivered: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--deliver file", string(b), recs)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("mcp", func(t *testing.T) {
		want := recs[:exportMCPInstallations]
		if n := len(strings.Join(want, "\n")); n <= 3*(16<<10) || n >= 60000 {
			t.Fatalf("the MCP fixture is %d bytes; it must pass three kit windows and fit the MCP budget", n)
		}
		replies, logs := e.mcp(t,
			map[string]any{"name": "export", "arguments": map[string]any{"resource": "applications", "limit": exportMCPInstallations}},
		)
		text, isErr := mcpText(t, replies[1])
		if isErr {
			t.Fatalf("MCP export of %d installations is an error: %.400s\nserver:\n%.2000s", exportMCPInstallations, text, logs)
		}
		sameLines(t, "MCP export", strings.TrimRight(text, "\n")+"\n", want)
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
