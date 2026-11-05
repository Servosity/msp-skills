// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

// Adversarial review 2026-11-05, class 1: every reader sees every row. The fake
// pages the way the Partner API documents: page counts from 0 and defaults to 0,
// size defaults to 10 and stops at 200, every list answers {"content":[...],
// "page":{"size","totalElements","totalPages","number"}}. The oracle is the rows
// the fake holds, the requests it saw and the exit code of the real binaries.

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type pagedReq struct {
	path   string
	page   string
	size   string
	status string
}

// pagedAPI is one TLS origin answering both the API and the token endpoint.
type pagedAPI struct {
	mu    sync.Mutex
	lists map[string][]string
	reqs  []pagedReq
	// fault answers a request instead of the documented paging when it returns true.
	fault func(path string, page, size int) (int, string, bool)
	// tokenStatus, when set, is how POST /oauth/token answers.
	tokenStatus int
	sv          *server
}

func pageBlockJSON(size, total, pages, number int) string {
	return fmt.Sprintf(`{"size":%d,"totalElements":%d,"totalPages":%d,"number":%d}`, size, total, pages, number)
}

func newPagedAPI(t *testing.T, lists map[string][]string) *pagedAPI {
	t.Helper()
	api := &pagedAPI{lists: lists}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := api.answer(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	api.sv = &server{url: srv.URL + "/v1", tokenURL: srv.URL + "/oauth/token", caFile: ca}
	return api
}

func (a *pagedAPI) answer(r *http.Request) (int, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.URL.Path == "/oauth/token" {
		if a.tokenStatus != 0 {
			return a.tokenStatus, `{"error":"access_denied","error_description":"Unauthorized"}`
		}
		return http.StatusOK, fmt.Sprintf(`{"access_token":%q,"expires_in":86400,"token_type":"Bearer"}`, bbMinted)
	}
	p := strings.TrimPrefix(r.URL.Path, "/v1")
	q := r.URL.Query()
	a.reqs = append(a.reqs, pagedReq{path: p, page: q.Get("page"), size: q.Get("size"), status: q.Get("status")})
	page, size := 0, 10
	if v, err := strconv.Atoi(q.Get("page")); err == nil {
		page = v
	}
	if v, err := strconv.ParseFloat(q.Get("size"), 64); err == nil {
		size = int(v)
	}
	if size > 200 {
		size = 200
	}
	if a.fault != nil {
		if status, body, ok := a.fault(p, page, size); ok {
			return status, body
		}
	}
	rows, known := a.lists[p]
	if !known && p != "/invoices/draftItems" && len(strings.Split(strings.Trim(p, "/"), "/")) == 2 {
		return http.StatusOK, `{}`
	}
	return http.StatusOK, pageBody(rows, page, size)
}

func pageBody(rows []string, page, size int) string {
	total := len(rows)
	pages := (total + size - 1) / size
	lo, hi := page*size, (page+1)*size
	if lo > total {
		lo = total
	}
	if hi > total {
		hi = total
	}
	return `{"content":[` + strings.Join(rows[lo:hi], ",") + `],"page":` + pageBlockJSON(size, total, pages, page) + `}`
}

func (a *pagedAPI) setFault(f func(path string, page, size int) (int, string, bool)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fault = f
}

func (a *pagedAPI) requests(path string) []pagedReq {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []pagedReq
	for _, r := range a.reqs {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func (a *pagedAPI) resetLog() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reqs = nil
}

func rowID(prefix string, n int) string {
	return fmt.Sprintf("%08s-0000-4000-8000-%012d", prefix, n)
}

func rowsOf(prefix string, n int, fields string) []string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"id":%q,"name":"%s %d"%s}`, rowID(prefix, i), prefix, i, fields)
	}
	return rows
}

const activeSubFields = `,"companyId":"c","productId":"p","quantity":2,"status":"Active","price":10.0,"partnerCost":6.0,"currencyCode":"USD","billingTerm":"Monthly"`

// runRC runs the CLI and returns its exit code, stdout and stderr.
func (e *bbEnv) runRC(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if err == nil {
			return 0, out.String(), errb.String()
		}
		if errors.As(err, &exit) {
			return exit.ExitCode(), out.String(), errb.String()
		}
		t.Fatalf("%v: %v", args, err)
	case <-time.After(180 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	return -1, "", ""
}

// storeCount reads how many rows the store holds for a resource.
func (e *bbEnv) storeCount(t *testing.T, resource string) int {
	t.Helper()
	var dbPath string
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "data") && strings.HasSuffix(d.Name(), ".db") {
			dbPath = path
		}
		return nil
	})
	if dbPath == "" {
		t.Fatal("no store was written")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM resources WHERE resource_type = ?`, resource).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var rowIDRE = regexp.MustCompile(`[0-9a-zA-Z]{8}-0000-4000-8000-\d{12}`)

func distinctIDs(text string) map[string]bool {
	seen := map[string]bool{}
	for _, id := range rowIDRE.FindAllString(text, -1) {
		seen[id] = true
	}
	return seen
}

// A sync that stored 10 rows of 437 and exited 0 is the original finding: the
// five lists whose config sent no size kept the API's default of 10.
func TestPagingSyncStoresEveryRow(t *testing.T) {
	t.Parallel()
	sub0, inv0 := rowID("s", 0), rowID("i", 0)
	api := newPagedAPI(t, map[string][]string{
		"/subscriptions":               rowsOf("s", 25, activeSubFields),
		"/invoices":                    rowsOf("i", 12, `,"total":250,"companyId":"c","currencyCode":"USD","status":"Paid"`),
		"/products":                    rowsOf("p", 230, ""),
		"/orders":                      rowsOf("o", 12, ""),
		"/companies":                   rowsOf("c", 12, ""),
		"/invoices/draftItems":         rowsOf("d", 12, ""),
		"/invoices/" + inv0 + "/items": rowsOf("l", 12, ""),
		"/subscriptions/" + sub0 + "/usage-summaries": rowsOf("u", 12, ""),
		"/companies/" + rowID("c", 0) + "/contacts":   rowsOf("k", 12, ""),
	})
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--agent")
	if rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, out, errOut)
	}
	for resource, want := range map[string]int{
		"subscriptions": 25, "invoices": 12, "products": 230, "orders": 12, "companies": 12,
		"invoices-draft-items": 12, "items": 12, "subscriptions_usage_summaries": 12, "contacts": 12,
	} {
		if got := e.storeCount(t, resource); got != want {
			t.Errorf("%s: store holds %d rows, the API holds %d", resource, got, want)
		}
	}
	reqs := api.requests("/products")
	if len(reqs) != 2 {
		t.Errorf("230 rows at the documented maximum take 2 requests, the sync made %d: %v", len(reqs), reqs)
	}
	for i, r := range reqs {
		if r.size != "200" || r.page != strconv.Itoa(i) {
			t.Errorf("request %d: page=%q size=%q, want page=%d size=200", i, r.page, r.size, i)
		}
	}
	_, mrr, _ := e.runRC(t, "mrr", "--agent", "--select", "totals.mrr")
	if !strings.Contains(mrr, "500") {
		t.Errorf("mrr over 25 rows at 2 x 10 is 500:\n%s", mrr)
	}
}

// A page that is exactly full ends the walk when the block says it is the last,
// and an ordinary small list still takes one request.
func TestPagingSyncEndsOnTheBlock(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{
		"/orders":   rowsOf("o", 200, ""),
		"/products": rowsOf("p", 3, ""),
	})
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--resources", "orders,products", "--agent")
	if rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, out, errOut)
	}
	if n := len(api.requests("/orders")); n != 1 {
		t.Errorf("exactly 200 rows are one page, the sync made %d requests", n)
	}
	if n := len(api.requests("/products")); n != 1 {
		t.Errorf("3 rows are one page, the sync made %d requests", n)
	}
	if strings.Contains(errOut, "sync_warning") {
		t.Errorf("a complete walk warned:\n%s", errOut)
	}
	if got := e.storeCount(t, "orders"); got != 200 {
		t.Errorf("store holds %d of 200", got)
	}
}

// The API may answer fewer rows than asked for. The block is the truth, not the
// row count: a short page with more pages behind it is not the end.
func TestPagingSyncFollowsAClampedSize(t *testing.T) {
	t.Parallel()
	const clamp = 20
	rows := rowsOf("p", 55, "")
	api := newPagedAPI(t, map[string][]string{"/products": rows})
	api.setFault(func(p string, page, size int) (int, string, bool) {
		if p != "/products" {
			return 0, "", false
		}
		return http.StatusOK, pageBody(rows, page, clamp), true
	})
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--resources", "products", "--agent")
	if rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, out, errOut)
	}
	if got := e.storeCount(t, "products"); got != 55 {
		t.Fatalf("store holds %d of 55", got)
	}
}

// sync --full deletes what a complete walk did not see, so a walk that is not
// provably complete must fail the resource and delete nothing.
func TestPagingSyncFullNeverPrunesFromAPartialRead(t *testing.T) {
	t.Parallel()
	rows := rowsOf("p", 5, "")
	faults := []struct {
		name  string
		fault func(p string, page, size int) (int, string, bool)
	}{
		{"second page refused", func(_ string, page, size int) (int, string, bool) {
			if page == 1 {
				return http.StatusBadRequest, `{"status":400,"message":"no"}`, true
			}
			return http.StatusOK, `{"content":[` + rows[0] + `],"page":` + pageBlockJSON(1, 5, 5, 0) + `}`, true
		}},
		{"empty page before the last", func(_ string, page, size int) (int, string, bool) {
			if page == 0 {
				return http.StatusOK, `{"content":[` + rows[0] + `],"page":` + pageBlockJSON(1, 5, 5, 0) + `}`, true
			}
			return http.StatusOK, `{"content":[],"page":` + pageBlockJSON(1, 5, 5, page) + `}`, true
		}},
		{"page parameter ignored", func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[` + rows[0] + `],"page":` + pageBlockJSON(1, 5, 5, 0) + `}`, true
		}},
		{"one page claimed, rows missing", func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[` + rows[0] + `],"page":` + pageBlockJSON(1, 5, 1, 0) + `}`, true
		}},
		{"no page block", func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[` + rows[0] + `]}`, true
		}},
		{"empty list with no page block", func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[]}`, true
		}},
	}
	for _, tc := range faults {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := newPagedAPI(t, map[string][]string{"/products": rows})
			e := newEnv(t, api.sv, false).store(t)
			if rc, out, errOut := e.runRC(t, "sync", "--resources", "products", "--agent"); rc != 0 {
				t.Fatalf("first sync rc=%d\n%s\n%s", rc, out, errOut)
			}
			api.setFault(func(p string, page, size int) (int, string, bool) {
				if p != "/products" {
					return 0, "", false
				}
				return tc.fault(p, page, size)
			})
			rc, out, errOut := e.runRC(t, "sync", "--full", "--resources", "products", "--agent")
			if rc == 0 {
				t.Errorf("a paging failure exited 0\n%s\n%s", out, errOut)
			}
			if !strings.Contains(errOut, `"errored":`) || strings.Contains(errOut, `"errored":0`) {
				t.Errorf("the resource did not count as failed:\n%s", errOut)
			}
			if got := e.storeCount(t, "products"); got != 5 {
				t.Errorf("--full deleted from a partial read: store holds %d of 5", got)
			}
		})
	}
	// The control: a complete walk still prunes, so the guard above is not the
	// prune having stopped working.
	t.Run("a complete walk still prunes", func(t *testing.T) {
		t.Parallel()
		api := newPagedAPI(t, map[string][]string{"/products": rows})
		e := newEnv(t, api.sv, false).store(t)
		if rc, out, errOut := e.runRC(t, "sync", "--resources", "products", "--agent"); rc != 0 {
			t.Fatalf("first sync rc=%d\n%s\n%s", rc, out, errOut)
		}
		api.mu.Lock()
		api.lists["/products"] = rows[:2]
		api.mu.Unlock()
		rc, out, errOut := e.runRC(t, "sync", "--full", "--resources", "products", "--agent")
		if rc != 0 {
			t.Fatalf("sync --full rc=%d\n%s\n%s", rc, out, errOut)
		}
		if got := e.storeCount(t, "products"); got != 2 {
			t.Errorf("store holds %d, the API holds 2", got)
		}
	})
}

// A walk that lost rows starts over on the next run. Resuming from the page it
// stopped on would skip what it lost and read as complete.
func TestPagingSyncFailedWalkStartsOver(t *testing.T) {
	t.Parallel()
	rows := rowsOf("p", 25, "")
	api := newPagedAPI(t, map[string][]string{"/products": rows})
	// Page 1 drops rows: the total only stops adding up on the last page.
	api.setFault(func(p string, page, size int) (int, string, bool) {
		if p != "/products" {
			return 0, "", false
		}
		const pageSize = 10
		lo, hi := page*pageSize, (page+1)*pageSize
		if hi > len(rows) {
			hi = len(rows)
		}
		if page == 1 {
			hi = lo + 2
		}
		return http.StatusOK, `{"content":[` + strings.Join(rows[lo:hi], ",") + `],"page":` + pageBlockJSON(pageSize, 25, 3, page) + `}`, true
	})
	e := newEnv(t, api.sv, false).store(t)
	if rc, out, errOut := e.runRC(t, "sync", "--resources", "products", "--agent"); rc == 0 {
		t.Fatalf("a walk that lost rows exited 0\n%s\n%s", out, errOut)
	}
	api.resetLog()
	if rc, out, errOut := e.runRC(t, "sync", "--resources", "products", "--agent"); rc == 0 {
		t.Errorf("the second run resumed past the lost rows and exited 0\n%s\n%s", out, errOut)
	}
	reqs := api.requests("/products")
	if len(reqs) == 0 || reqs[0].page != "0" {
		t.Errorf("the second run did not start over from page 0: %+v", reqs)
	}
}

// Rows on the page that broke the contract are kept: the failure is loud and
// nothing the API did send is thrown away.
func TestPagingSyncKeepsRowsItRead(t *testing.T) {
	t.Parallel()
	rows := rowsOf("p", 3, "")
	api := newPagedAPI(t, nil)
	api.setFault(func(p string, page, size int) (int, string, bool) {
		if p != "/products" {
			return 0, "", false
		}
		return http.StatusOK, `{"content":[` + strings.Join(rows, ",") + `],"page":` + pageBlockJSON(200, 5, 1, 0) + `}`, true
	})
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--resources", "products", "--agent")
	if rc == 0 {
		t.Errorf("3 rows of a reported 5 exited 0\n%s\n%s", out, errOut)
	}
	if got := e.storeCount(t, "products"); got != 3 {
		t.Errorf("store holds %d of the 3 rows read", got)
	}
}

func allCommands() [][]string {
	return [][]string{
		{"subscriptions", "find"},
		{"companies", "find"},
		{"invoices", "find-partner"},
		{"products", "find-all"},
		{"orders", "find"},
		{"invoices", "find-partner-draft-items"},
		{"companies", "contacts", "get", rowID("c", 0)},
		{"invoices", "items", "find-partner-invoice", rowID("i", 0)},
		{"subscriptions", "usage-summaries", "find-subscription", rowID("s", 0)},
	}
}

func allLists(n int) map[string][]string {
	return map[string][]string{
		"/subscriptions":       rowsOf("s", n, activeSubFields),
		"/companies":           rowsOf("c", n, ""),
		"/invoices":            rowsOf("i", n, ""),
		"/products":            rowsOf("p", n, ""),
		"/orders":              rowsOf("o", n, ""),
		"/invoices/draftItems": rowsOf("d", n, ""),
		"/companies/" + rowID("c", 0) + "/contacts":            rowsOf("k", n, ""),
		"/invoices/" + rowID("i", 0) + "/items":                rowsOf("l", n, ""),
		"/subscriptions/" + rowID("s", 0) + "/usage-summaries": rowsOf("u", n, ""),
	}
}

// --all counted an absent page as 1 and asked for page 2 next: rows 10-19 of
// every list were lost, and the walk reported itself complete.
func TestPagingAllReturnsEveryRow(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, allLists(25))
	e := newEnv(t, api.sv, false).store(t)
	for _, args := range allCommands() {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			rc, out, errOut := e.runRC(t, append(append([]string{}, args...), "--all", "--json", "--data-source", "live")...)
			if rc != 0 {
				t.Fatalf("rc=%d\n%s\n%s", rc, out, errOut)
			}
			if got := len(distinctIDs(out)); got != 25 {
				t.Errorf("%d rows of 25\n%s", got, errOut)
			}
			if !strings.Contains(errOut, `"total":25`) {
				t.Errorf("the walk did not report the true total:\n%s", errOut)
			}
		})
	}
}

// The generic walk stops at 100 pages. At the default size that is 1000 rows, so
// a longer walk must not stop there.
func TestPagingAllPassesTheGenericPageCap(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/companies": rowsOf("c", 105, "")})
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "companies", "find", "--all", "--size", "1", "--json", "--data-source", "live")
	if rc != 0 {
		t.Fatalf("rc=%d\n%.2000s\n%.2000s", rc, out, errOut)
	}
	if got := len(distinctIDs(out)); got != 105 {
		t.Errorf("%d rows of 105", got)
	}
}

// A walk that cannot prove it read every row fails instead of returning part.
func TestPagingAllFailsOnAPartialRead(t *testing.T) {
	t.Parallel()
	rows := rowsOf("c", 25, "")
	api := newPagedAPI(t, map[string][]string{"/companies": rows})
	e := newEnv(t, api.sv, false).store(t)
	for name, fault := range map[string]func(p string, page, size int) (int, string, bool){
		"empty page before the last": func(_ string, page, size int) (int, string, bool) {
			if page == 0 {
				return 0, "", false
			}
			return http.StatusOK, `{"content":[],"page":` + pageBlockJSON(size, 25, 3, page) + `}`, true
		},
		"no page block": func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[` + rows[0] + `]}`, true
		},
		"page parameter ignored": func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, pageBody(rows, 0, size), true
		},
	} {
		t.Run(name, func(t *testing.T) {
			api.setFault(func(p string, page, size int) (int, string, bool) {
				if p != "/companies" {
					return 0, "", false
				}
				return fault(p, page, size)
			})
			rc, out, errOut := e.runRC(t, "companies", "find", "--all", "--json", "--data-source", "live")
			if rc == 0 {
				t.Errorf("exit 0 on a partial read\n%.2000s\n%.2000s", out, errOut)
			}
		})
	}
}

// export began at page=1, so the first page never left the API and a list of
// ten rows or fewer exported nothing.
func TestPagingExportWritesEveryRow(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{
		"/companies":     rowsOf("c", 25, ""),
		"/subscriptions": rowsOf("s", 230, activeSubFields),
		"/invoices":      rowsOf("i", 3, ""),
	})
	e := newEnv(t, api.sv, false).store(t)
	for resource, want := range map[string]int{"companies": 25, "subscriptions": 230, "invoices": 3, "orders": 0} {
		t.Run(resource, func(t *testing.T) {
			rc, out, errOut := e.runRC(t, "export", resource, "--format", "jsonl")
			if rc != 0 {
				t.Fatalf("rc=%d\n%.2000s\n%.2000s", rc, out, errOut)
			}
			ids := distinctIDs(out)
			if len(ids) != want {
				t.Errorf("%d rows of %d", len(ids), want)
			}
			if want > 0 && !ids[rowID(map[string]string{"companies": "c", "subscriptions": "s", "invoices": "i"}[resource], 0)] {
				t.Errorf("the first row is missing")
			}
		})
	}
	rc, out, errOut := e.runRC(t, "export", "companies", "--format", "jsonl", "--limit", "5")
	if rc != 0 || len(distinctIDs(out)) != 5 {
		t.Errorf("--limit 5: rc=%d rows=%d\n%s", rc, len(distinctIDs(out)), errOut)
	}
}

func TestPagingExportFailsOnAPartialRead(t *testing.T) {
	t.Parallel()
	rows := rowsOf("c", 25, "")
	api := newPagedAPI(t, map[string][]string{"/companies": rows})
	e := newEnv(t, api.sv, false).store(t)
	for name, fault := range map[string]func(p string, page, size int) (int, string, bool){
		"empty page before the last": func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[],"page":` + pageBlockJSON(size, 25, 3, page) + `}`, true
		},
		"rows missing": func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[` + rows[0] + `],"page":` + pageBlockJSON(size, 25, 1, 0) + `}`, true
		},
		"no page block": func(_ string, page, size int) (int, string, bool) {
			return http.StatusOK, `{"content":[` + rows[0] + `]}`, true
		},
	} {
		t.Run(name, func(t *testing.T) {
			api.setFault(func(p string, page, size int) (int, string, bool) { return fault(p, page, size) })
			rc, out, errOut := e.runRC(t, "export", "companies", "--format", "jsonl")
			if rc == 0 {
				t.Errorf("exit 0 on a partial read\n%.2000s\n%.2000s", out, errOut)
			}
		})
	}
}

// mcpSession is one MCP stdio session answering one request at a time, so a
// test can feed a reply's cursor into the next call.
type mcpSession struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser
	enc   *json.Encoder
	out   *bufio.Reader
	id    int
}

func (e *bbEnv) mcpSession(t *testing.T) *mcpSession {
	t.Helper()
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &mcpSession{t: t, cmd: cmd, stdin: stdin, enc: json.NewEncoder(stdin), out: bufio.NewReaderSize(stdout, 1<<20)}
	t.Cleanup(func() {
		_ = stdin.Close()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	s.request("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}})
	_ = s.enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return s
}

func (s *mcpSession) request(method string, params any) json.RawMessage {
	s.t.Helper()
	s.id++
	if err := s.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params}); err != nil {
		s.t.Fatal(err)
	}
	for {
		line, err := s.out.ReadBytes('\n')
		if err != nil {
			s.t.Fatalf("%s: %v", method, err)
		}
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(line, &reply) == nil && reply.ID == s.id {
			return reply.Result
		}
	}
}

// call runs a tool and returns its text and whether it was a tool error.
func (s *mcpSession) call(name string, args map[string]any) (string, bool) {
	s.t.Helper()
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	raw := s.request("tools/call", map[string]any{"name": name, "arguments": args})
	if err := json.Unmarshal(raw, &result); err != nil {
		s.t.Fatalf("%s: %v\n%s", name, err, raw)
	}
	var b strings.Builder
	for _, c := range result.Content {
		b.WriteString(c.Text)
	}
	return b.String(), result.IsError
}

func nextCursorOf(t *testing.T, text string) string {
	t.Helper()
	var body struct {
		NextCursor string `json:"next_cursor"`
		Data       struct {
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("not JSON: %v\n%.500s", err, text)
	}
	if body.NextCursor != "" {
		return body.NextCursor
	}
	return body.Data.NextCursor
}

// walkCursor follows next_cursor until none is returned, adding args to the
// first call and keepArgs to every later one.
func walkCursor(t *testing.T, s *mcpSession, tool string, first, later map[string]any) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	args := first
	for i := 0; i < 100; i++ {
		text, isErr := s.call(tool, args)
		if isErr {
			t.Fatalf("%s %v: %.500s", tool, args, text)
		}
		for id := range distinctIDs(text) {
			seen[id] = true
		}
		cursor := nextCursorOf(t, text)
		if cursor == "" {
			return seen
		}
		args = map[string]any{"cursor": cursor}
		for k, v := range later {
			args[k] = v
		}
	}
	t.Fatalf("%s: the cursor never ended", tool)
	return nil
}

func mcpListTools() map[string]map[string]any {
	return map[string]map[string]any{
		"subscriptions_find":                              {},
		"companies_find":                                  {},
		"invoices_find-partner":                           {},
		"invoices_find-partner-draft-items":               {},
		"orders_find":                                     {},
		"products_find-all":                               {},
		"companies_contacts_get":                          {"companyId": rowID("c", 0)},
		"invoices_items_find-partner-invoice":             {"invoiceId": rowID("i", 0)},
		"subscriptions_usage-summaries_find-subscription": {"subscriptionId": rowID("s", 0)},
		"usage-summaries_usage-lines_find":                {"usageSummaryId": "U", "usageDate": "2026-01-01"},
	}
}

// An agent has to be able to read every row: the tools refused page and never
// returned a cursor past the first API page.
func TestPagingMCPReachesEveryRow(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/subscriptions": rowsOf("s", 437, activeSubFields), "/companies": rowsOf("c", 25, "")})
	e := newEnv(t, api.sv, false).store(t)
	s := e.mcpSession(t)

	t.Run("the cursor alone continues the walk", func(t *testing.T) {
		api.resetLog()
		if got := len(walkCursor(t, s, "subscriptions_find", map[string]any{"size": 200}, nil)); got != 437 {
			t.Errorf("%d rows of 437", got)
		}
		for _, r := range api.requests("/subscriptions") {
			if r.size != "200" {
				t.Errorf("a continuation changed the size: %+v", r)
			}
		}
	})
	t.Run("the cursor with the same size", func(t *testing.T) {
		if got := len(walkCursor(t, s, "subscriptions_find", map[string]any{"size": 200}, map[string]any{"size": 200})); got != 437 {
			t.Errorf("%d rows of 437", got)
		}
	})
	t.Run("page reaches every row", func(t *testing.T) {
		seen := map[string]bool{}
		for page := 0; page < 3; page++ {
			for id := range walkCursor(t, s, "subscriptions_find", map[string]any{"size": 200, "page": page}, nil) {
				seen[id] = true
			}
		}
		if len(seen) != 437 {
			t.Errorf("%d rows of 437", len(seen))
		}
	})
	t.Run("page reads the row it names", func(t *testing.T) {
		text, isErr := s.call("subscriptions_find", map[string]any{"size": 5, "page": 1})
		if isErr {
			t.Fatalf("%.500s", text)
		}
		ids := distinctIDs(text)
		if len(ids) != 5 || !ids[rowID("s", 5)] || !ids[rowID("s", 9)] {
			t.Errorf("page 1 of size 5 is rows 5-9, got %v", ids)
		}
	})
	t.Run("the default walk reads every row", func(t *testing.T) {
		if got := len(walkCursor(t, s, "companies_find", map[string]any{}, nil)); got != 25 {
			t.Errorf("%d rows of 25", got)
		}
	})
	t.Run("a cursor refuses a change of walk", func(t *testing.T) {
		text, _ := s.call("subscriptions_find", map[string]any{"size": 200})
		cursor := nextCursorOf(t, text)
		if cursor == "" {
			t.Fatal("no cursor")
		}
		if text, isErr := s.call("subscriptions_find", map[string]any{"cursor": cursor, "size": 5}); !isErr {
			t.Errorf("a different size on a cursor skips rows and was accepted: %.300s", text)
		}
		if text, isErr := s.call("subscriptions_find", map[string]any{"cursor": cursor, "page": 2}); !isErr {
			t.Errorf("page and cursor together were accepted: %.300s", text)
		}
		if text, isErr := s.call("subscriptions_find", map[string]any{"cursor": "1"}); !isErr {
			t.Errorf("a made-up cursor was accepted: %.300s", text)
		}
	})
	t.Run("an API that ignores page is an error", func(t *testing.T) {
		rows := rowsOf("s", 437, activeSubFields)
		api.setFault(func(p string, page, size int) (int, string, bool) {
			return http.StatusOK, pageBody(rows, 0, size), true
		})
		defer api.setFault(nil)
		if text, isErr := s.call("subscriptions_find", map[string]any{"size": 200, "page": 1}); !isErr {
			t.Errorf("page 0 answered for page 1 and no error: %.300s", text)
		}
	})
}

// A cursor pins the filters it was minted under, so a walk cannot drift onto
// another result set.
func TestPagingMCPCursorKeepsFilters(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/subscriptions": rowsOf("s", 60, activeSubFields)})
	e := newEnv(t, api.sv, false).store(t)
	s := e.mcpSession(t)
	if got := len(walkCursor(t, s, "subscriptions_find", map[string]any{"size": 20, "status": "Active"}, nil)); got != 60 {
		t.Errorf("%d rows of 60", got)
	}
	reqs := api.requests("/subscriptions")
	if len(reqs) < 3 {
		t.Fatalf("60 rows at size 20 take 3 pages, saw %d requests", len(reqs))
	}
	for _, r := range reqs {
		if r.status != "Active" || r.size != "20" {
			t.Errorf("a continuation dropped the filter or the size: %+v", r)
		}
	}
}

func TestPagingMCPSchemaNamesPageAndSize(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, nil)
	e := newEnv(t, api.sv, false).store(t)
	s := e.mcpSession(t)
	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(s.request("tools/list", map[string]any{}), &list); err != nil {
		t.Fatal(err)
	}
	found := map[string]map[string]json.RawMessage{}
	for _, tool := range list.Tools {
		found[tool.Name] = tool.InputSchema.Properties
	}
	for name, required := range mcpListTools() {
		props, ok := found[name]
		if !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		for _, arg := range []string{"page", "size", "cursor"} {
			if _, ok := props[arg]; !ok {
				t.Errorf("%s: schema has no %q", name, arg)
			}
		}
		args := map[string]any{"page": 0, "size": 5}
		for k, v := range required {
			args[k] = v
		}
		if text, isErr := s.call(name, args); isErr {
			t.Errorf("%s refused page and size: %.300s", name, text)
		}
	}
}

// usage-lines documents the same paging in its description.
func TestPagingMCPUsageLinesReachEveryRow(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/usage-summaries/U/usage-lines": rowsOf("l", 25, "")})
	e := newEnv(t, api.sv, false).store(t)
	s := e.mcpSession(t)
	got := walkCursor(t, s, "usage-summaries_usage-lines_find", map[string]any{"usageSummaryId": "U", "usageDate": "2026-01-01", "size": 7},
		map[string]any{"usageSummaryId": "U", "usageDate": "2026-01-01"})
	if len(got) != 25 {
		t.Errorf("%d rows of 25", len(got))
	}
}
