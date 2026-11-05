// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the two credential
// classes the 4.34.2 review closed with the print's own layer, proved against
// the built binaries now that the shared credential kit replaces that layer.
// (The third class, incomplete-sync-never-read-as-complete, is not a credential
// class; internal/cli/backup_radar_sync_incomplete_test.go keeps it.)

package credwire

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The held v3 and v2 keys: low-entropy letters and dashes (secret scanners pass
// them), no 8-character piece shared between them (each key is bound to its own
// origin), and none of program text.
const (
	classV3Key = "vthreekey-jjjj-xxxx-jjjj"
	classV2Key = "vtwokey-zzzz-vvvv-zzzz"
)

// TestV3KeyNeverLeavesForTheV2Host: class credential-in-request-data. `backups
// list` is a v2 command, sent to https://api.backupradar.com with the v2 key.
// Typed as a search term, the v3 key would ride to that host in a query value;
// the kit's transport refuses the request before it is sent, because the v3
// key may go only to the configured v3 base URL. The control shows the same
// command does reach the v2 host (the recording proxy) with a harmless term,
// so the refusal is the kit's, not a path that never sends.
func TestV3KeyNeverLeavesForTheV2Host(t *testing.T) {
	keys := []string{"BACKUP_RADAR_API_KEY=" + classV3Key, "BACKUP_RADAR_V2_API_KEY=" + classV2Key}

	control := newEnv(t, nil, keys...)
	r := control.run(t, "", "backups", "list", "--search-string", "acme-nightly", "--data-source", "live", "--json")
	if !reachedV2Host(control) {
		t.Fatalf("control: a harmless search never reached api.backupradar.com, so this test proves nothing\nproxy saw %+v\nprinted:\n%s", control.proxy.all(), r.out)
	}

	e := newEnv(t, nil, keys...)
	r = e.run(t, "", "backups", "list", "--search-string", classV3Key, "--data-source", "live", "--json")
	if got := e.proxy.all(); len(got) != 0 {
		t.Fatalf("the v3 key left for another host: proxy saw %+v\nprinted:\n%s", got, r.out)
	}
	if got := e.api.all(); len(got) != 0 {
		t.Fatalf("a v2 command reached the v3 fake: %+v", got)
	}
	if strings.Contains(r.out, classV3Key) {
		t.Fatalf("the refusal printed the v3 key:\n%s", r.out)
	}
}

func reachedV2Host(e *env) bool {
	for _, s := range e.proxy.all() {
		if strings.HasPrefix(s.Host, "api.backupradar.com") {
			return true
		}
	}
	return false
}

// TestExportSplitKeyNeverLandsWhole: class held-value-split-across-stream-
// deliveries. `export health --output` writes page by page; the fake puts the
// first half of the held key in a record of page 1 and the second half in
// page 2, with enough string-less records between them that the halves reach
// the file in two separate writes. Read the way contract 3 reads a stream (its
// JSON string values joined in order), the file must never carry the key whole.
func TestExportSplitKeyNeverLandsWhole(t *testing.T) {
	a, b := classV3Key[:len(classV3Key)/2], classV3Key[len(classV3Key)/2:]
	filler := strings.TrimSuffix(strings.Repeat(`{"n":1},`, 600), ",")
	pages := map[string]string{
		"":   fmt.Sprintf(`{"data":[{"name":%q},%s],"next_cursor":"p2"}`, a, filler),
		"p2": fmt.Sprintf(`{"data":[{"name":%q}],"next_cursor":null}`, b),
	}
	e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pages[r.URL.Query().Get("cursor")]))
	}, "BACKUP_RADAR_API_KEY="+classV3Key)
	out := filepath.Join(e.home, "out", "health.jsonl")
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		t.Fatal(err)
	}
	r := e.run(t, "", "export", "health", "--output", out)
	if got := e.api.all(); len(got) != 2 {
		t.Fatalf("want the two pages fetched, got %d request(s)\nprinted:\n%s", len(got), r.out)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("export left no file: %v\nprinted:\n%s", err, r.out)
	}
	if strings.Contains(string(data), classV3Key) {
		t.Fatalf("the file holds the key whole:\n%s", data)
	}
	if joined := joinedStringValues(t, data); strings.Contains(joined, classV3Key) {
		t.Fatalf("the file's string values, read together, carry the key (halves %q and %q both landed)\nprinted:\n%s", a, b, r.out)
	}
	if strings.Contains(r.out, classV3Key) {
		t.Fatalf("the run printed the key:\n%s", r.out)
	}
}

// joinedStringValues is every JSON string value of a JSON-lines file, in order,
// concatenated: what a reader of the whole file sees across its records. Keys
// are not values; a line that is not JSON contributes its raw text.
func joinedStringValues(t *testing.T, data []byte) string {
	t.Helper()
	var sb strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			sb.WriteString(x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var v any
		if json.Unmarshal(sc.Bytes(), &v) != nil {
			sb.WriteString(sc.Text())
			continue
		}
		walk(v)
	}
	return sb.String()
}
