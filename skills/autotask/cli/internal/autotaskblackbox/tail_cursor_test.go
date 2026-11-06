// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

import (
	"bufio"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Codex r1 class incomplete-read-advances-cursor (handfix
// r1k-tail-cursor-advances-only-on-a-complete-poll), on the built binary, the
// Codex input as written: `tail tickets --interval 1s` first reads tickets 1
// to 101, each modified at 00:00. Then ticket 1 is modified at 00:02 and
// ticket 101 at 00:01; the update pass reads tickets 1 to 100, its second
// page answers HTTP 401, and every later answer is normal. The 401 poll held
// the 00:02 watermark, so every later poll asked only for tickets modified
// since 00:02 and ticket 101's update never printed. It must print, once, and
// ticket 1's update must not print twice.
func TestTailReadsAgainWhatAFailedPollDidNotRead(t *testing.T) {
	t.Parallel()
	const field = "lastTrackedModificationDateTime"
	var mu sync.Mutex
	mod := map[int]string{}
	for id := 1; id <= 101; id++ {
		mod[id] = "2026-11-04T00:00:00Z"
	}
	updated, denied := false, false
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != apiBase+"/Tickets/query" {
			http.NotFound(w, r)
			return
		}
		var q struct {
			MaxRecords int `json:"maxRecords"`
		}
		_ = json.Unmarshal(body, &q)
		conds, after := queryFilters(r, body), walkFrom(r, body)
		byTime := false
		for _, c := range conds {
			byTime = byTime || c["field"] == field
		}
		mu.Lock()
		defer mu.Unlock()
		if updated && byTime && after >= 100 && !denied {
			denied = true
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":["The request is not authorized."]}`))
			return
		}
		ids := make([]int, 0, len(mod))
		for id := range mod {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		items := []string{}
		for _, id := range ids {
			ok := len(items) < q.MaxRecords
			for _, c := range conds {
				switch v := c["value"].(type) {
				case float64: // the id walk: gte on page one, gt after
					ok = ok && (c["op"] == "gte" && float64(id) >= v || c["op"] == "gt" && float64(id) > v)
				case string: // modified since
					ok = ok && c["field"] == field && c["op"] == "gte" && mod[id] >= v
				default:
					ok = false
				}
			}
			if ok {
				items = append(items, fmt.Sprintf(`{"id":%d,"title":"T%d as of %s",%q:%q}`, id, id, mod[id], field, mod[id]))
			}
		}
		fmt.Fprintf(w, `{"items":[%s],"pageDetails":{"count":%d,"requestCount":%d,"prevPageUrl":null,"nextPageUrl":null}}`, strings.Join(items, ","), len(items), q.MaxRecords)
		if !updated && !byTime && after >= 100 { // the first poll read its last page
			updated = true
			mod[1], mod[101] = "2026-11-04T00:02:00Z", "2026-11-04T00:01:00Z"
		}
	}))
	sv := &server{url: srv.URL + apiBase, headers: map[creds]int{}}
	sv.caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(sv.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, sv, credEnv)
	count(&cliRuns, 1)
	cmd := exec.Command(bbCLI, "tail", "tickets", "--interval", "1s")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	done := make(chan struct{})
	defer close(done)
	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-done:
				return
			}
		}
	}()
	printed := map[string]int{}
	deadline := time.After(60 * time.Second)
	for printed["T101 as of 2026-11-04T00:01:00Z"] == 0 {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("tail exited before printing ticket 101's update (printed %d records):\n%s", len(printed), stderr.String())
			}
			var ev struct {
				Data struct {
					Title string `json:"title"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(line), &ev) == nil && ev.Data.Title != "" {
				printed[ev.Data.Title]++
			}
		case <-deadline:
			mu.Lock()
			wasDenied := denied
			mu.Unlock()
			t.Fatalf("after 60s tail had not printed ticket 101's 00:01 update (the 401 answered: %v; ticket 1's 00:02 update printed %d time(s)):\n%s",
				wasDenied, printed["T1 as of 2026-11-04T00:02:00Z"], stderr.String())
		}
	}
	if n := printed["T1 as of 2026-11-04T00:02:00Z"]; n != 1 {
		t.Errorf("ticket 1's 00:02 update printed %d times, want once:\n%s", n, stderr.String())
	}
	if n := printed["T101 as of 2026-11-04T00:00:00Z"]; n != 1 {
		t.Errorf("the first poll printed ticket 101 %d times, want once", n)
	}
	if !strings.Contains(stderr.String(), "poll failed") {
		t.Errorf("the 401 poll printed no warning:\n%s", stderr.String())
	}
}
