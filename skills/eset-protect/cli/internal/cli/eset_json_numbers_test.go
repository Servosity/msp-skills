// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// bigInt is 2^53+1: float64 rounds it to 9007199254740992.
const bigInt = "9007199254740993"

// TestImportExportKeepNumbersExact (handfixes.json: json-numbers-exact):
// import decoded each line into map[string]any and single-record JSON export
// decoded into any, so 2^53+1 reached the API (or the backup) as 2^53 with
// exit 0; JSONL export kept a pretty-printed record's newlines, so its output
// could not be re-imported one record per line.
func TestImportExportKeepNumbersExact(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	const id = "00000000-0000-0000-0000-000000000001"
	var mu sync.Mutex
	var posted []string
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			posted = append(posted, string(b))
			mu.Unlock()
			_, _ = io.WriteString(w, `{"policy":{"uuid":"`+id+`"}}`)
		case r.URL.Path == "/v2/policies/"+id:
			_, _ = io.WriteString(w, `{"policy":{"uuid":"`+id+`","limit":`+bigInt+`}}`)
		default:
			_, _ = io.WriteString(w, "{\"policies\":[{\n  \"uuid\": \""+id+"\",\n  \"limit\": "+bigInt+"\n}],\"nextPageToken\":\"\"}")
		}
	})

	input := filepath.Join(t.TempDir(), "in.jsonl")
	if err := os.WriteFile(input, []byte(`{"displayName":"Base","limit":`+bigInt+"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, errOut, err := captureExecuteAll(t, "import", "policies", "--input", input); err != nil {
		t.Fatalf("import: %v\n%s", err, errOut)
	}
	mu.Lock()
	got := strings.Join(posted, "\n")
	mu.Unlock()
	if !strings.Contains(got, bigInt) {
		t.Fatalf("import changed the number before sending: %s", got)
	}

	out, errOut, err := captureExecuteAll(t, "export", "policies", id, "--format", "json")
	if err != nil {
		t.Fatalf("export <id> --format json: %v\n%s", err, errOut)
	}
	if !bytes.Contains(out, []byte(bigInt)) {
		t.Fatalf("single-record JSON export changed the number: %s", out)
	}

	out, errOut, err = captureExecuteAll(t, "export", "policies", "--format", "jsonl")
	if err != nil {
		t.Fatalf("export --format jsonl: %v\n%s", err, errOut)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], bigInt) {
		t.Fatalf("JSONL export must be one exact record per line, got %d lines: %q", len(lines), out)
	}
}

// TestGeneratedWriteBodiesKeepNumbersExact (handfixes.json: json-numbers-exact):
// the generated write commands decoded --stdin bodies and JSON-valued flags
// into map[string]any / any, rounding 2^53+1 through float64 before the POST.
func TestGeneratedWriteBodiesKeepNumbersExact(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	var mu sync.Mutex
	var posted []string
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		posted = append(posted, string(b))
		mu.Unlock()
		_, _ = io.WriteString(w, `{}`)
	})
	last := func() string {
		mu.Lock()
		defer mu.Unlock()
		if len(posted) == 0 {
			return ""
		}
		return posted[len(posted)-1]
	}

	in := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(in, []byte(`{"group":{"displayName":"Acme","limit":`+bigInt+`}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdin
	os.Stdin = f
	_, errOut, err := captureExecuteAll(t, "groups", "create", "--stdin")
	os.Stdin = saved
	_ = f.Close()
	if err != nil {
		t.Fatalf("groups create --stdin: %v\n%s", err, errOut)
	}
	if got := last(); !strings.Contains(got, bigInt) {
		t.Fatalf("--stdin body changed the number before sending: %s", got)
	}

	if _, errOut, err := captureExecuteAll(t, "device-tasks", "create", "--task-action-name", "OnDemandScan", "--task-action-params", `{"limit":`+bigInt+`}`); err != nil {
		t.Fatalf("device-tasks create: %v\n%s", err, errOut)
	}
	if got := last(); !strings.Contains(got, bigInt) {
		t.Fatalf("a JSON-valued flag changed the number before sending: %s", got)
	}
}
