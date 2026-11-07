// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-hands-the-kit-whole-records):
// export writes through the credential kit's stream sinks (stdout, an --output
// file, the --deliver tee, the MCP mirror's child stdout). The kit checks each unit
// with the stream's tail as context and keeps a JSON document open across units;
// past its 16 KiB window an open document makes the stream lost and every later
// unit is withheld. A writer that hands the kit chunks ending mid-record (the press
// export's 4096-byte bufio.Writer) leaves a document open at every unit, so an
// ordinary export of a few hundred sites stopped after 20 KiB. Every record must
// arrive, and the run must succeed. An API error still ends the run.

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

// exportSites is past 64 KiB of JSON lines over three pages, and about seven times
// the kit's 16 KiB stream window.
const exportSites = 600

// exportMCPSites fits the MCP text budget (60000 bytes), past three kit windows.
const exportMCPSites = 250

func exportRecord(i int) string {
	return fmt.Sprintf(`{"id":%d,"uid":"site-%d","name":"Site number %d","accountUid":"acct-1","description":"an ordinary site record with enough text to look real","onDemand":false,"splashtopAutoInstall":false,"devicesStatus":{"numberOfDevices":%d}}`, i, i, i, i%7)
}

// exportPager answers the account lists export (components) and get-sites (sites)
// read the way Datto pages them: page from 0, max, pageDetails.nextPageUrl null on
// the last page.
func exportPager(w http.ResponseWriter, r *http.Request, _ seen) bool {
	plural := strings.TrimPrefix(r.URL.Path, "/api/v2/account/")
	if plural != "components" && plural != "sites" {
		return false
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("max"))
	if size <= 0 {
		size = 250
	}
	lo := min(page*size, exportSites)
	hi := min(lo+size, exportSites)
	items := make([]string, 0, hi-lo)
	for i := lo; i < hi; i++ {
		items = append(items, exportRecord(i))
	}
	next := "null"
	if hi < exportSites {
		next = strconv.Quote(fmt.Sprintf("https://%s%s?page=%d&max=%d", r.Host, r.URL.Path, page+1, size))
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"pageDetails":{"count":%d,"totalCount":%d,"prevPageUrl":null,"nextPageUrl":%s},%q:[%s]}`, hi-lo, exportSites, next, plural, strings.Join(items, ","))
	return true
}

func wantRecords(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = exportRecord(i)
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

func sameRecord(a, b string) bool {
	var x, y any
	return json.Unmarshal([]byte(a), &x) == nil && json.Unmarshal([]byte(b), &y) == nil && fmt.Sprint(x) == fmt.Sprint(y)
}

// sameLines fails unless got is want, one record per line, in order.
func sameLines(t *testing.T, where, got string, want []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if got == "" {
		lines = nil
	}
	whole := 0
	for whole < len(lines) && whole < len(want) && sameRecord(lines[whole], want[whole]) {
		whole++
	}
	if whole != len(want) || len(lines) != len(want) || !strings.HasSuffix(got, "\n") {
		t.Fatalf("%s: %d bytes, %d lines, the first %d of %d records whole and in order; tail %q", where, len(got), len(lines), whole, len(want), got[max(0, len(got)-160):])
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
		if !sameRecord(string(it), want[i]) {
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

// TestExportStreamsEveryRecord: export of ordinary, credential-free records past 64
// KiB, in every format and to every destination, delivers every record and exits 0.
func TestExportStreamsEveryRecord(t *testing.T) {
	t.Parallel()
	want := wantRecords(exportSites)
	if n := len(strings.Join(want, "\n")); n <= 64<<10 {
		t.Fatalf("the fixture is %d bytes; it must pass 64 KiB to prove anything", n)
	}
	e := newEnv(t, exportPager, tokenEnv...)
	t.Run("stdout jsonl", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "account")
		noKitRefusal(t, "stdout", errOut)
		sameLines(t, "stdout", out, want)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("stdout json", func(t *testing.T) {
		out, errOut, code := e.split(t, "export", "account", "--format", "json")
		noKitRefusal(t, "stdout", errOut)
		sameArray(t, "stdout", out, want)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("output jsonl", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "sites.jsonl")
		_, errOut, code := e.split(t, "export", "account", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--output file", string(b), want)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("output json", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "sites.json")
		_, errOut, code := e.split(t, "export", "account", "--format", "json", "--output", path)
		noKitRefusal(t, "--output", errOut)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no file: %v; stderr:\n%s", err, errOut)
		}
		sameArray(t, "--output file", string(b), want)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("deliver file", func(t *testing.T) {
		path := filepath.Join(e.home, "out", "delivered.jsonl")
		out, errOut, code := e.split(t, "export", "account", "--deliver", "file:"+path)
		noKitRefusal(t, "--deliver", errOut)
		sameLines(t, "--deliver stdout tee", out, want)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("nothing delivered: %v; stderr:\n%s", err, errOut)
		}
		sameLines(t, "--deliver file", string(b), want)
		if code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, errOut)
		}
	})
	t.Run("table formats", func(t *testing.T) {
		for _, format := range []string{"--csv", "--plain"} {
			out, errOut, code := e.split(t, "account", "get-sites", "--all", format)
			noKitRefusal(t, format, errOut)
			for _, last := range []string{"Site number 0", "Site number 299", fmt.Sprintf("Site number %d", exportSites-1)} {
				if !strings.Contains(out, last) {
					t.Fatalf("%s lost %q (%d bytes); stderr:\n%s", format, last, len(out), errOut)
				}
			}
			if code != 0 {
				t.Fatalf("%s: exit %d; stderr:\n%s", format, code, errOut)
			}
		}
	})
	t.Run("mcp", func(t *testing.T) {
		wantMCP := want[:exportMCPSites]
		if n := len(strings.Join(wantMCP, "\n")); n <= 3*(16<<10) || n >= 60000 {
			t.Fatalf("the MCP fixture is %d bytes; it must pass three kit windows and fit the MCP budget", n)
		}
		replies, logs := e.mcp(t,
			map[string]any{"name": "export", "arguments": map[string]any{"resource": "account", "limit": exportMCPSites}},
			map[string]any{"name": "export", "arguments": map[string]any{"resource": "account"}},
		)
		text, isErr := mcpText(t, replies[1])
		if isErr {
			t.Fatalf("MCP export of %d sites is an error: %.400s\nserver:\n%.2000s", exportMCPSites, text, logs)
		}
		sameLines(t, "MCP export", strings.TrimRight(text, "\n")+"\n", wantMCP)
		// Past the budget the result is the bound's explicit preview, never an
		// error and never a silent cut.
		text, isErr = mcpText(t, replies[2])
		if isErr {
			t.Fatalf("MCP export of %d sites is an error: %.400s", exportSites, text)
		}
		var preview struct {
			Truncated bool `json:"_pp_truncated"`
		}
		if json.Unmarshal([]byte(text), &preview) != nil || !preview.Truncated {
			t.Fatalf("MCP export past the budget is neither whole nor the explicit truncated preview: %d bytes, head %.200q", len(text), text)
		}
	})
}

// mcpText is a tools/call reply's first content block and isError.
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

// TestExportOutputReturnsAfterAnAPIError: an API error ends an export --output run
// with a failing exit, and leaves no partial file behind.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(w http.ResponseWriter, _ *http.Request, _ seen) bool {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return true
	}, tokenEnv...)
	path := filepath.Join(e.home, "sites.jsonl")
	r := e.run(t, "", "export", "account", "--output", path)
	if r.code == 0 {
		t.Fatalf("export --output succeeded on a 401:\n%s", r.out)
	}
	if got := e.api.all(); len(got) != 1 || got[0].Path != "/api/v2/account/components" {
		t.Fatalf("want the one GET /v2/account/components, got %+v\n%s", got, r.out)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("a failed export left a file:\n%s", r.out)
	}
}
