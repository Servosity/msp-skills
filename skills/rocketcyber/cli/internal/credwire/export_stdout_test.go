// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestExportToStdoutWritesEveryRecord: with a credential held (every real
// install), `export agents` to stdout, the default sink and format, must print
// every record of every page and exit 0. The kit withholds a streamed unit that
// a pause between page fetches splits, so a 4 KB buffered writer flushing while
// the next page loads lost every record after the first split (adversarial
// review round 1: 93 of 250, exit 74, nothing on stderr). The control is the
// same export with --output.
func TestExportToStdoutWritesEveryRecord(t *testing.T) {
	const total, pageSize = 250, 100
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/agents") {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if size < 1 || size > pageSize {
			size = pageSize
		}
		pages := (total + size - 1) / size
		var data []map[string]any
		for i := (page-1)*size + 1; i <= total && i <= page*size; i++ {
			data = append(data, map[string]any{"id": fmt.Sprintf("AG%05d", i), "customerId": 2311414,
				"hostname": fmt.Sprintf("paged-agent-%d.hq-record.example", i), "connectivity": "online",
				"platform": "windows", "osVersion": "Windows 11 Pro 23H2", "ipv4Address": "192.0.2.10"})
		}
		time.Sleep(150 * time.Millisecond) // a real page fetch takes time; the writer flushes meanwhile
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"totalCount": total, "currentPage": page, "totalPages": pages, "dataCount": len(data), "data": data})
	}))
	defer srv.Close()
	home := t.TempDir()
	ca := filepath.Join(home, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	cliBin, _ := binaries(t)
	env := closedEnv(home, "ROCKETCYBER_API_TOKEN=exptok-mzqzzkkwwmwzmmqzm", "ROCKETCYBER_BASE_URL="+srv.URL+"/v3",
		"ROCKETCYBER_CA_CERT="+ca, "SSL_CERT_FILE="+ca)
	stdout, stderr, code := runSplit(t, cliBin, env, "export", "agents", "--no-cache", "--no-learn")
	lines := strings.Count(stdout, "\n")
	if code != 0 || lines != total || !strings.Contains(stdout, `"AG00250"`) {
		t.Fatalf("export to stdout: exit %d, %d of %d records\nstderr: %s\nlast: %.200s", code, lines, total, stderr, stdout[max(0, len(stdout)-200):])
	}
	out := filepath.Join(home, "agents.jsonl")
	if _, stderr, code := runSplit(t, cliBin, env, "export", "agents", "--no-cache", "--no-learn", "--output", out); code != 0 {
		t.Fatalf("control export --output: exit %d\n%s", code, stderr)
	}
	b, err := os.ReadFile(out)
	if err != nil || strings.Count(string(b), "\n") != total {
		t.Fatalf("control export --output: %v, %d records", err, strings.Count(string(b), "\n"))
	}
}
