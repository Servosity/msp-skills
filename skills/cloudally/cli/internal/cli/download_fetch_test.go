// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
)

// dlJob is one export job the stub portal lists.
type dlJob struct {
	JobID     string
	Account   string
	SubSource string
	Status    string
	Snapshot  int64
	Files     []portal.DownloadFile
}

// dlStub stands up the three things a download run talks to: the API that
// mints the portal handoff, the portal's service layer, and object storage.
type dlStub struct {
	t *testing.T

	mu          sync.Mutex
	jobs        []dlJob
	bodies      map[string][]byte
	storageCode map[string]int
	storageHits map[string]int
	// createCode answers a create with this HTTP status for the given
	// account; 0 means a 200 success envelope.
	createCode map[string]int
	// onCreate runs inside the create handler, before it answers.
	onCreate func(body map[string]any)
	creates  []string
	// urlFailure, when set, is the portal's failure text for every link.
	urlFailure string
	// createBodies records every create body, in order.
	createBodies []map[string]any
	// createDelay holds each create this long before it answers.
	createDelay time.Duration
	// listCreated adds every accepted create to the job listing, the way
	// the live portal shows a new job after a while.
	listCreated bool
	// taskEntities are the mailboxes the backup task serves, each with an
	// O365MAIL and an O365CALENDAR backup.
	taskEntities []string
	// slow streams each body in eight pieces this far apart.
	slow time.Duration

	api, portal, storage *httptest.Server
}

func newDLStub(t *testing.T) *dlStub {
	t.Helper()
	s := &dlStub{
		t: t, bodies: map[string][]byte{}, storageCode: map[string]int{},
		storageHits: map[string]int{}, createCode: map[string]int{},
	}
	s.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/partner-access-token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"handoff-token"}`))
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/backup-tasks/") {
			s.mu.Lock()
			ents := []map[string]any{}
			for i, e := range s.taskEntities {
				ents = append(ents, map[string]any{
					"id": fmt.Sprintf("e%d", i), "taskId": "task-1", "entity": e, "extId": "ext-" + e, "status": "ACTIVE",
					"lastBackupDates": map[string]any{"O365MAIL": 20221231042000 + i, "O365CALENDAR": 20221231041000 + i},
				})
			}
			s.mu.Unlock()
			raw, _ := json.Marshal(map[string]any{"id": "task-1", "account": "acct-1", "type": "BACKUP", "source": "MS365",
				"status": "ACTIVE", "numOfEntities": len(ents), "entities": ents})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(s.api.Close)
	s.storage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/obj/"))
		s.mu.Lock()
		s.storageHits[id]++
		body, ok := s.bodies[id]
		code := s.storageCode[id]
		s.mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
			_, _ = w.Write([]byte("storage error"))
			return
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.mu.Lock()
		slow := s.slow
		s.mu.Unlock()
		if slow <= 0 {
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		step := (len(body) + 7) / 8
		for off := 0; off < len(body); off += step {
			_, _ = w.Write(body[off:min(off+step, len(body))])
			w.(http.Flusher).Flush()
			time.Sleep(slow)
		}
	}))
	t.Cleanup(s.storage.Close)
	s.portal = s.portalServer()
	t.Cleanup(s.portal.Close)

	su, _ := url.Parse(s.storage.URL)
	prev, prevAddr := portal.DownloadURLPolicy, portal.DownloadAddrPolicy
	portal.DownloadURLPolicy = func(u *url.URL) error {
		if u.Host == su.Host {
			return nil
		}
		return portal.StrictDownloadURL(u)
	}
	portal.DownloadAddrPolicy = func(a netip.Addr) error {
		if a.IsLoopback() {
			return nil
		}
		return portal.StrictDownloadAddr(a)
	}
	t.Cleanup(func() { portal.DownloadURLPolicy, portal.DownloadAddrPolicy = prev, prevAddr })
	return s
}

func (s *dlStub) portalServer() *httptest.Server {
	const shell = `<html><head><meta name="_csrf" content="csrf-1"/>` +
		`<meta name="_csrf_header" content="X-CSRF-TOKEN"/></head></html>`
	mux := http.NewServeMux()
	mux.HandleFunc("/newapp", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
		_, _ = w.Write([]byte(shell))
	})
	mux.HandleFunc("/application/service/", func(w http.ResponseWriter, r *http.Request) {
		op := strings.TrimPrefix(r.URL.Path, "/application/service/")
		w.Header().Set("Content-Type", "application/json")
		s.mu.Lock()
		defer s.mu.Unlock()
		switch op {
		case "getRestoreDownloadStatus":
			rows := []map[string]any{}
			for _, j := range s.jobs {
				rows = append(rows, map[string]any{
					"taskId": j.JobID, "type": "EXPORT", "status": j.Status, "account": j.Account,
					"source": "MS365", "subsource": j.SubSource, "snapshotDate": j.Snapshot,
				})
			}
			writeEnvelope(w, rows)
		case "getDownloadInfo":
			id := r.URL.Query().Get("taskId")
			files := []portal.DownloadFile{}
			for _, j := range s.jobs {
				if j.JobID == id {
					files = append(files, j.Files...)
				}
			}
			writeEnvelope(w, files)
		case "getDownloadUrl":
			if s.urlFailure != "" {
				raw, _ := json.Marshal(map[string]any{"success": false, "status": s.urlFailure})
				_, _ = w.Write(raw)
				return
			}
			id := r.URL.Query().Get("taskId")
			for _, j := range s.jobs {
				for _, f := range j.Files {
					if f.ExportDataID == id {
						writeEnvelope(w, s.storage.URL+"/obj/"+url.PathEscape(id))
						return
					}
				}
			}
			// The live portal's answer to a job id or an unknown chunk.
			_, _ = w.Write([]byte(`{"success":false,"status":"The download link has expired.","statusCode":400}`))
		case "startRestoreOrDownload":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			account, _ := body["account"].(string)
			s.creates = append(s.creates, account)
			s.createBodies = append(s.createBodies, body)
			if s.onCreate != nil {
				s.onCreate(body)
			}
			if s.createDelay > 0 {
				s.mu.Unlock()
				time.Sleep(s.createDelay)
				s.mu.Lock()
			}
			if code := s.createCode[account]; code != 0 {
				w.WriteHeader(code)
				return
			}
			if s.listCreated {
				sub, _ := body["source"].(string)
				date, _ := body["date"].(float64)
				s.jobs = append(s.jobs, dlJob{JobID: fmt.Sprintf("job-%d", len(s.jobs)+1), Account: account, SubSource: sub, Status: "INPROCESS", Snapshot: int64(date)})
			}
			writeEnvelope(w, map[string]any{"needRedirect": false})
		case "getJobsDasboard":
			writeEnvelope(w, map[string]any{"total": 0, "dashboardElements": []any{}})
		default:
			writeEnvelope(w, nil)
		}
	})
	return httptest.NewServer(mux)
}

func writeEnvelope(w io.Writer, data any) {
	raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
	_, _ = w.Write(raw)
}

// addFile registers one archive under a job, sized as the portal reports it.
func (s *dlStub) addFile(jobID, account, sub, exportID, fileName string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies[exportID] = body
	f := portal.DownloadFile{ExportDataID: exportID, FileName: fileName, Size: fmt.Sprintf("%d Bytes", len(body))}
	for i := range s.jobs {
		if s.jobs[i].JobID == jobID {
			s.jobs[i].Files = append(s.jobs[i].Files, f)
			return
		}
	}
	s.jobs = append(s.jobs, dlJob{
		JobID: jobID, Account: account, SubSource: sub, Status: portal.StatusReadyForDownload,
		Snapshot: 20221231042000, Files: []portal.DownloadFile{f},
	})
}

func (s *dlStub) hits(exportID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.storageHits[exportID]
}

func (s *dlStub) createdAccounts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.creates...)
}

// runDL runs the CLI against the stub without re-isolating, so a ledger
// written by one run is visible to the next. Call testenv.Isolate first.
func runDL(t *testing.T, s *dlStub, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("CLOUDALLY_BASE_URL", s.api.URL)
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--portal-url", s.portal.URL))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func fetchArgs(dest string, extra ...string) []string {
	return append([]string{"download", "fetch", "--account", "acct-1", "--dest", dest, "--year", "2022", "--json"}, extra...)
}

func decodeOutcomes(t *testing.T, out string) []fetchOutcome {
	t.Helper()
	var got []fetchOutcome
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("fetch output is not JSON: %v\n%s", err, out)
	}
	return got
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

const jane = "jane@contoso.com"

// TestDownloadFetchKeepsSubSourcesApartAndReRunsOnEvidence covers the
// collision finding: two sub-sources with the same file name, and files
// with no usable name, each land at their own path, and a re-run skips
// them only because their records say they are the same exports.
func TestDownloadFetchKeepsSubSourcesApartAndReRunsOnEvidence(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
	s.addFile("j-cal", jane, "O365CALENDAR", "d2", "jane.zip", []byte("CALENDAR-ARCHIVE-LONGER"))
	s.addFile("j-con", jane, "O365CONTACTS", "d3", "", []byte("CONTACTS-ONE"))
	s.addFile("j-con", jane, "O365CONTACTS", "d4", "..", []byte("CONTACTS-TWO!"))
	dest := t.TempDir()

	out, stderr, err := runDL(t, s, fetchArgs(dest)...)
	if err != nil {
		t.Fatalf("fetch: %v\n%s", err, stderr)
	}
	for _, o := range decodeOutcomes(t, out) {
		if o.Status != fetchDownloaded {
			t.Fatalf("first run: %+v", o)
		}
	}
	box := filepath.Join(dest, "2022", jane)
	want := map[string]string{
		filepath.Join(box, "O365MAIL", "jane.zip"):     "MAIL-ARCHIVE",
		filepath.Join(box, "O365CALENDAR", "jane.zip"): "CALENDAR-ARCHIVE-LONGER",
		filepath.Join(box, "O365CONTACTS", "d3.zip"):   "CONTACTS-ONE",
		filepath.Join(box, "O365CONTACTS", "d4.zip"):   "CONTACTS-TWO!",
	}
	for path, body := range want {
		if got := readFile(t, path); got != body {
			t.Fatalf("%s = %q, want %q", path, got, body)
		}
		if _, err := os.Stat(path + recordSuffix); err != nil {
			t.Fatalf("no record beside %s: %v", path, err)
		}
	}

	out, stderr, err = runDL(t, s, fetchArgs(dest)...)
	if err != nil {
		t.Fatalf("second fetch: %v\n%s", err, stderr)
	}
	for _, o := range decodeOutcomes(t, out) {
		if o.Status != fetchSkipped {
			t.Fatalf("second run should skip everything: %+v", o)
		}
	}
	for _, id := range []string{"d1", "d2", "d3", "d4"} {
		if n := s.hits(id); n != 1 {
			t.Fatalf("%s fetched %d times, want once", id, n)
		}
	}
}

// TestDownloadFetchNeedsEvidenceBeforeSkipping proves a file of the right
// name and size is not taken for the archive without its record, and is
// never overwritten: the archive goes beside it under its export id.
func TestDownloadFetchNeedsEvidenceBeforeSkipping(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
	dest := t.TempDir()
	dir := filepath.Join(dest, "2022", jane, "O365MAIL")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Same length as the archive, different bytes, no record.
	theirs := filepath.Join(dir, "jane.zip")
	if err := os.WriteFile(theirs, []byte("CUSTOMERFILE"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, stderr, err := runDL(t, s, fetchArgs(dest)...)
	if err != nil {
		t.Fatalf("fetch: %v\n%s", err, stderr)
	}
	got := decodeOutcomes(t, out)
	if len(got) != 1 || got[0].Status != fetchDownloaded {
		t.Fatalf("want one DOWNLOADED, got %+v", got)
	}
	if body := readFile(t, theirs); body != "CUSTOMERFILE" {
		t.Fatalf("the existing file was overwritten: %q", body)
	}
	alt := filepath.Join(dir, "jane.d1.zip")
	if got[0].Path != alt || readFile(t, alt) != "MAIL-ARCHIVE" {
		t.Fatalf("archive should be at %s, outcome %+v", alt, got[0])
	}

	// Once the file that pushed it aside is gone, a re-run still finds the
	// archive under its export-id name instead of fetching it again.
	if err := os.Remove(theirs); err != nil {
		t.Fatal(err)
	}
	out, stderr, err = runDL(t, s, fetchArgs(dest)...)
	if err != nil {
		t.Fatalf("second fetch: %v\n%s", err, stderr)
	}
	got = decodeOutcomes(t, out)
	if len(got) != 1 || got[0].Status != fetchSkipped || got[0].Path != alt {
		t.Fatalf("want ALREADY_PRESENT at %s, got %+v", alt, got)
	}
	if n := s.hits("d1"); n != 1 {
		t.Fatalf("d1 fetched %d times, want once", n)
	}
}

// TestDownloadFetchExitsNonZeroWhenAnArchiveFails covers the exit code: a
// run with a FAILED archive must not report success to a script.
func TestDownloadFetchExitsNonZeroWhenAnArchiveFails(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
	s.addFile("j-cal", jane, "O365CALENDAR", "d2", "cal.zip", []byte("CALENDAR"))
	s.storageCode["d2"] = http.StatusInternalServerError
	dest := t.TempDir()

	out, _, err := runDL(t, s, fetchArgs(dest)...)
	if err == nil {
		t.Fatal("want a non-zero exit when an archive FAILED")
	}
	if code := ExitCode(err); code != 6 {
		t.Fatalf("exit code = %d, want 6 (partial failure): %v", code, err)
	}
	statuses := map[string]string{}
	for _, o := range decodeOutcomes(t, out) {
		statuses[o.ExportDataID] = o.Status
	}
	if statuses["d1"] != fetchDownloaded || statuses["d2"] != fetchFailed {
		t.Fatalf("statuses = %v", statuses)
	}
}

// TestDownloadFetchFailsAWrongSizedBody proves a short error document
// served with a 200 is not published as the archive.
func TestDownloadFetchFailsAWrongSizedBody(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("x"))
	s.mu.Lock()
	s.jobs[0].Files[0].Size = "1.00 MB"
	s.bodies["d1"] = []byte(`<Error><Code>AccessDenied</Code></Error>`)
	s.mu.Unlock()
	dest := t.TempDir()

	out, _, err := runDL(t, s, fetchArgs(dest)...)
	if err == nil {
		t.Fatal("want a failure for a body that is not the reported size")
	}
	got := decodeOutcomes(t, out)
	if len(got) != 1 || got[0].Status != fetchFailed {
		t.Fatalf("want FAILED, got %+v", got)
	}
	dir := filepath.Join(dest, "2022", jane, "O365MAIL")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Fatalf("nothing should be left behind, found %s", e.Name())
	}
}

// TestDownloadFetchCreatesPrivateFiles proves archives of customer mail are
// not readable by other accounts on the machine.
func TestDownloadFetchCreatesPrivateFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits carry no access control on Windows")
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
	dest := t.TempDir()
	if _, stderr, err := runDL(t, s, fetchArgs(dest)...); err != nil {
		t.Fatalf("fetch: %v\n%s", err, stderr)
	}
	year := filepath.Join(dest, "2022")
	box := filepath.Join(year, jane)
	sub := filepath.Join(box, "O365MAIL")
	for _, d := range []string{year, box, sub} {
		fi, err := os.Lstat(d)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Fatalf("%s mode = %o, want 0700", d, perm)
		}
	}
	archive := filepath.Join(sub, "jane.zip")
	for _, f := range []string{archive, archive + recordSuffix} {
		fi, err := os.Lstat(f)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("%s mode = %o, want 0600", f, perm)
		}
	}
}

// TestDownloadFetchNeverFollowsAPlantedPartialLink plants links at the
// predictable "<archive>.partial" names, one aimed at another archive under
// --dest and one outside it. Neither target may change.
func TestDownloadFetchNeverFollowsAPlantedPartialLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs a privilege on Windows")
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
	s.addFile("j-cal", jane, "O365CALENDAR", "d2", "cal.zip", []byte("CALENDAR"))
	dest := t.TempDir()
	inside := filepath.Join(dest, "2021", "other.zip")
	outside := filepath.Join(t.TempDir(), "victim.txt")
	for _, f := range []string{inside, outside} {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("VICTIM"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mail := filepath.Join(dest, "2022", jane, "O365MAIL")
	cal := filepath.Join(dest, "2022", jane, "O365CALENDAR")
	for _, d := range []string{mail, cal} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(inside, filepath.Join(mail, "jane.zip"+partialSuffix)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cal, "cal.zip"+partialSuffix)); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runDL(t, s, fetchArgs(dest)...)
	for _, f := range []string{inside, outside} {
		if got := readFile(t, f); got != "VICTIM" {
			t.Fatalf("%s was written through a planted link: %q", f, got)
		}
	}
	if err != nil {
		t.Fatalf("fetch: %v\n%s", err, stderr)
	}
	if readFile(t, filepath.Join(mail, "jane.zip")) != "MAIL-ARCHIVE" || readFile(t, filepath.Join(cal, "cal.zip")) != "CALENDAR" {
		t.Fatal("the archives themselves should have been written")
	}
}

// TestDownloadFetchRefusesASymlinkedFolder proves a link standing in for
// one of the folders the layout creates is refused, whether it points
// elsewhere under --dest or outside it.
func TestDownloadFetchRefusesASymlinkedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs a privilege on Windows")
	}
	cases := []struct {
		name  string
		plant func(t *testing.T, dest string) string // returns the link target
	}{
		// A relative link that stays under --dest is one os.Root would
		// follow, so only the folder check stands in its way.
		{"year links inside dest", func(t *testing.T, dest string) string {
			target := filepath.Join(dest, "elsewhere")
			mustMkdir(t, target)
			mustSymlink(t, "elsewhere", filepath.Join(dest, "2022"))
			return target
		}},
		{"mailbox links inside dest", func(t *testing.T, dest string) string {
			target := filepath.Join(dest, "2022", "other-box")
			mustMkdir(t, target)
			mustSymlink(t, "other-box", filepath.Join(dest, "2022", jane))
			return target
		}},
		{"mailbox links outside dest", func(t *testing.T, dest string) string {
			target := t.TempDir()
			mustMkdir(t, filepath.Join(dest, "2022"))
			mustSymlink(t, target, filepath.Join(dest, "2022", jane))
			return target
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
			dest := t.TempDir()
			target := tc.plant(t, dest)

			out, _, err := runDL(t, s, fetchArgs(dest)...)
			if err == nil || ExitCode(err) != 6 {
				t.Fatalf("want a partial-failure exit, got %v", err)
			}
			got := decodeOutcomes(t, out)
			if len(got) != 1 || got[0].Status != fetchFailed || !strings.Contains(got[0].Detail, "not a plain folder") {
				t.Fatalf("want FAILED for the link, got %+v", got)
			}
			entries, _ := os.ReadDir(target)
			if len(entries) != 0 {
				t.Fatalf("something was written through the link into %s: %v", target, entries)
			}
			if s.hits("d1") != 0 {
				t.Fatal("the archive should not have been fetched")
			}
		})
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// TestPublishNoReplaceNeverReplaces proves the final move refuses a name
// that is already taken, in both directions.
func TestPublishNoReplaceNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.partial", "NEW")
	write("final.zip", "OLD")
	if err := publishNoReplace(root, "a.partial", "final.zip"); err == nil {
		t.Fatal("publishing over an existing file must fail")
	}
	if got := readFile(t, filepath.Join(dir, "final.zip")); got != "OLD" {
		t.Fatalf("existing file replaced: %q", got)
	}
	if err := publishNoReplace(root, "a.partial", "free.zip"); err != nil {
		t.Fatalf("publishing to a free name: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "free.zip")); got != "NEW" {
		t.Fatalf("free.zip = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a.partial")); !os.IsNotExist(err) {
		t.Fatalf("the temporary file should be gone: %v", err)
	}
}

// TestClaimAndRenameNeverReplaces covers the last-resort publish used on a
// drive with neither hard links nor a no-replace rename (exFAT on macOS):
// a name that is taken is refused, a free one is claimed and filled.
func TestClaimAndRenameNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for name, body := range map[string]string{"a.partial": "NEW", "final.zip": "OLD"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := claimAndRename(root, "a.partial", "final.zip"); err == nil {
		t.Fatal("claiming a taken name must fail")
	}
	if got := readFile(t, filepath.Join(dir, "final.zip")); got != "OLD" {
		t.Fatalf("existing file replaced: %q", got)
	}
	if err := claimAndRename(root, "a.partial", "free.zip"); err != nil {
		t.Fatalf("claiming a free name: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "free.zip")); got != "NEW" {
		t.Fatalf("free.zip = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a.partial")); !os.IsNotExist(err) {
		t.Fatalf("the temporary file should be gone: %v", err)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"jane@contoso.com", "jane@contoso.com", true},
		{"  report.zip  ", "report.zip", true},
		{"a/b\\c:d", "a_b_c_d", true},
		{"tab\there", "tab_here", true},
		{"esc\x1b]0;OWNED\x07", "esc_]0;OWNED_", true},
		{"del\x7fname", "del_name", true},
		{"c1\u0085next\u009b2J", "c1_next_2J", true},
		{"‮ppa.exe.zip", "_ppa.exe.zip", true},
		{"x‪‫‬‭.zip", "x____.zip", true},
		{"iso⁦late⁧⁨⁩.zip", "iso_late___.zip", true},
		{"mark‎‏؜.zip", "mark___.zip", true},
		{"CON", "_CON", true},
		{"con.zip", "_con.zip", true},
		{"Nul.tar.gz", "_Nul.tar.gz", true},
		{"NUL .txt", "_NUL .txt", true},
		{"prn", "_prn", true},
		{"AUX.zip", "_AUX.zip", true},
		{"COM1", "_COM1", true},
		{"com9.pst", "_com9.pst", true},
		{"LPT1.zip", "_LPT1.zip", true},
		{"lpt9", "_lpt9", true},
		{"COM¹.zip", "_COM¹.zip", true},
		{"CONSOLE.zip", "CONSOLE.zip", true},
		{"COM10.zip", "COM10.zip", true},
		{"", "", false},
		{"   ", "", false},
		{".", "", false},
		{"..", "", false},
		{"...", "", false},
		{"///", "", false},
		{"\x1b\x07", "", false},
		{"bad\xffutf8", "bad_utf8", true},
		// Shortening must not recreate a reserved or empty name.
		{"CON" + strings.Repeat(" ", 300) + "tail", "_CON", true},
		{"nul" + strings.Repeat(" ", 300) + ".zip", "_nul.zip", true},
		{"a" + strings.Repeat(".", 300) + "b", "a.b", true},
		{strings.Repeat(" ", 300) + "x", "x", true},
	}
	for _, tc := range cases {
		got, ok := sanitizeName(tc.in, maxNameBytes)
		if got != tc.want || ok != tc.ok {
			t.Errorf("sanitizeName(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if got := safeFolderName(".."); got != "_unnamed" {
		t.Errorf("safeFolderName(..) = %q", got)
	}
}

func TestSanitizeNameShortensLongNames(t *testing.T) {
	for _, in := range []string{
		strings.Repeat("a", 400) + ".zip",
		strings.Repeat("é", 300) + ".zip",
		strings.Repeat("\U0001F600", 100) + ".pst",
		strings.Repeat("b", 300),
	} {
		got, ok := sanitizeName(in, maxArchiveBytes)
		if !ok || len(got) > maxArchiveBytes || len(got) == 0 {
			t.Fatalf("len(%q...) = %d, want 1..%d", got[:min(len(got), 10)], len(got), maxArchiveBytes)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("shortened name is not valid UTF-8: %q", got)
		}
		if ext := filepath.Ext(in); len(ext) <= 16 && ext != "" && !strings.HasSuffix(got, ext) {
			t.Fatalf("shortened name lost its extension %s: %q", ext, got)
		}
		if len(got)+len(recordSuffix) > maxNameBytes {
			t.Fatalf("record name would be too long: %d", len(got)+len(recordSuffix))
		}
	}
	// A folder name is held to the full 255.
	if got := safeFolderName(strings.Repeat("m", 600)); len(got) != maxNameBytes {
		t.Fatalf("folder name length = %d", len(got))
	}
}

// TestArchiveNameNeverTakesARecordName proves a hostile file name cannot
// make an archive sit where another archive's record goes.
func TestArchiveNameNeverTakesARecordName(t *testing.T) {
	got, err := archiveName(portal.DownloadFile{ExportDataID: "d9", FileName: "jane.zip.CLOUDALLY.json"})
	if err != nil || got != "d9.zip" {
		t.Fatalf("archiveName = %q, %v", got, err)
	}
	if _, err := archiveName(portal.DownloadFile{FileName: ".."}); err == nil {
		t.Fatal("a file with no usable name and no export id must be refused")
	}
}

// TestDownloadFetchPlanOnlyWritesNothing proves --plan-only reports what
// would be fetched without creating a folder or requesting an archive.
func TestDownloadFetchPlanOnlyWritesNothing(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("MAIL-ARCHIVE"))
	dest := t.TempDir()
	out, stderr, err := runDL(t, s, fetchArgs(dest, "--plan-only")...)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	got := decodeOutcomes(t, out)
	if len(got) != 1 || got[0].Status != "WOULD_DOWNLOAD" ||
		got[0].Path != filepath.Join(dest, "2022", jane, "O365MAIL", "jane.zip") {
		t.Fatalf("plan = %+v", got)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Fatalf("a plan created %v", entries)
	}
	if s.hits("d1") != 0 {
		t.Fatal("a plan requested the archive")
	}
}
