// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"riverside-fm-pp-cli/internal/cliutil/testenv"
)

// Security review round 1 (reprint 4.35.1). Each test drives the real command
// against a fake Riverside over TLS, so the client, its envelope handling and
// the command's own bookkeeping are all on the path.

type fakeRoute struct {
	status int
	ctype  string
	body   string
	// byOffset answers a list route by its offset query value, the way a server
	// that caps its page size does; an offset not listed gets body.
	byOffset map[string]string
}

type fakeRiverside struct {
	routes map[string]fakeRoute
	hits   int64
}

func (f *fakeRiverside) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&f.hits, 1)
	rt, ok := f.routes[r.URL.Path]
	if !ok {
		rt = fakeRoute{status: http.StatusNotFound, body: `{"error":"not found"}`}
	}
	if rt.ctype == "" {
		rt.ctype = "application/json; charset=utf-8"
	}
	w.Header().Set("Content-Type", rt.ctype)
	if rt.status == 0 {
		rt.status = http.StatusOK
	}
	if page, ok := rt.byOffset[r.URL.Query().Get("offset")]; ok {
		rt.body = page
	}
	w.WriteHeader(rt.status)
	_, _ = w.Write([]byte(rt.body))
}

// runAgainstFake runs the CLI with args against routes and returns stdout,
// stderr, the command error and the number of requests the fake served.
func runAgainstFake(t *testing.T, routes map[string]fakeRoute, args ...string) (string, string, error, int64) {
	t.Helper()
	testenv.Isolate(t)
	fake := &fakeRiverside{routes: routes}
	srv := httptest.NewUnstartedServer(fake)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	t.Setenv("RIVERSIDE_FM_BASE_URL", srv.URL)
	t.Setenv("RIVERSIDE_FM_SKIP_TLS_VERIFY", "1")
	// No client retry back-off: a failing route fails once.
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err, atomic.LoadInt64(&fake.hits)
}

const (
	r1Studio   = "/api/v4/projects/studio/st"
	r1Takes    = "/api/v4/projects/p1/takes"
	r1Trans    = "/api/v4/transcriptions/editableWithVoiceActivity/sid-1"
	r1Assets   = "/api/v4/take/sid-1/assets"
	r1VOD      = "/api/v4/vod/sid-1/h1"
	r1Projects = `{"projects":[{"_id":"p1","title":"P","createdAt":"2026-01-01T00:00:00Z"}]}`
	r1TakeList = `{"takes":[{"_id":"t1","title":"T","recordings":[{"sessionId":"sid-1"}]}]}`
	r1Words    = `{"success":true,"data":{"speakers":[{"name":"A","sentences":[{"index":0,"words":[["hi",0,100]]}]}],"voiceActivity":{"speakers":[]}}}`
	r1DoneTake = `{"take":{"tracks":[{"archiveId":"h1","status":"done","filename":"a.wav","duration":60}]}}`
	r1M3U8     = "#EXTM3U\n#EXT-X-VERSION:3\n"
)

func r1Fail() fakeRoute {
	return fakeRoute{status: http.StatusInternalServerError, body: `{"error":"boom"}`}
}

func resumeHas(t *testing.T, outDir, sid string) bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(outDir, ".resume.json"))
	if err != nil {
		return false
	}
	var done map[string]bool
	_ = json.Unmarshal(b, &done)
	return done[sid]
}

// Class A: a failed fetch must never checkpoint a take as exported.
func TestReviewR1BulkExportDoesNotCheckpointFailedFetch(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "archive")
	_, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		r1Studio: {body: r1Projects},
		r1Takes:  {body: r1TakeList},
		r1Trans:  r1Fail(),
		r1Assets: r1Fail(),
	}, "bulk", "export", "--studio", "st", "--out", outDir, "--json")
	if resumeHas(t, outDir, "sid-1") {
		t.Fatalf("take sid-1 was checkpointed in .resume.json although its transcript and assets fetches failed; a resumed export would skip it forever")
	}
	if err == nil {
		t.Fatalf("bulk export exited 0 although every per-take fetch failed")
	}
}

// Class A: a failed write must never checkpoint a take as exported.
func TestReviewR1BulkExportDoesNotCheckpointFailedWrite(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "archive")
	// transcript.json is a directory, so writing the transcript fails.
	if err := os.MkdirAll(filepath.Join(outDir, "st", "P-p1", "T-sid-1", "transcript.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		r1Studio: {body: r1Projects},
		r1Takes:  {body: r1TakeList},
		r1Trans:  {body: r1Words},
		r1Assets: {body: r1DoneTake},
		r1VOD:    {ctype: "application/x-mpegurl; charset=utf-8", body: r1M3U8},
	}, "bulk", "export", "--studio", "st", "--out", outDir, "--json")
	if resumeHas(t, outDir, "sid-1") {
		t.Fatalf("take sid-1 was checkpointed although writing its transcript failed")
	}
	if err == nil {
		t.Fatalf("bulk export exited 0 although a file write failed")
	}
}

// Class A: a fetch error is not "nothing available", and not a silent fallback.
func TestReviewR1GrabFetchErrorIsNotNothingAvailable(t *testing.T) {
	cases := map[string]map[string]fakeRoute{
		"every tier fails": {r1Trans: r1Fail(), r1Assets: r1Fail()},
		"transcript fails": {r1Trans: r1Fail(), r1Assets: {body: r1DoneTake}},
	}
	for name, routes := range cases {
		t.Run(name, func(t *testing.T) {
			outDir := t.TempDir()
			stdout, _, err, _ := runAgainstFake(t, routes, "grab", "sid-1", "--out", outDir, "--json")
			if err == nil {
				t.Fatalf("grab exited 0 after a failed fetch; stdout=%s", stdout)
			}
			if ExitCode(err) == 13 || strings.Contains(stdout, `"tier":"none"`) {
				t.Fatalf("grab reported nothing available (exit %d) when the fetch failed; stdout=%s", ExitCode(err), stdout)
			}
		})
	}
}

// Class A: a clip whose detail fetch failed must not vanish from the refresh.
func TestReviewR1MediaRefreshReportsFailedClipFetch(t *testing.T) {
	stdout, _, _, _ := runAgainstFake(t, map[string]fakeRoute{
		"/api/v4/projects/p1/clips/exports": {body: `{"clips":[{"_id":"c1"}]}`},
		"/api/v4/clip/c1":                   r1Fail(),
	}, "media", "refresh", "--project", "p1", "--json")
	if !strings.Contains(stdout, `"c1"`) || !strings.Contains(stdout, "fetch-failed") {
		t.Fatalf("media refresh dropped clip c1 whose fetch failed instead of reporting it; stdout=%s", stdout)
	}
}

// Class A: ready must not print an authoritative empty list when it could not look.
func TestReviewR1ReadyFetchFailureIsNotEmpty(t *testing.T) {
	stdout, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		r1Studio: {body: r1Projects},
		r1Takes:  {body: r1TakeList},
		r1Assets: r1Fail(),
	}, "ready", "--studio", "st", "--json")
	if err == nil {
		t.Fatalf("ready exited 0 with an empty list although every take fetch failed; stdout=%s", stdout)
	}
}

// Class A: a clip whose detail fetch failed must not be filtered out silently.
func TestReviewR1ClipsListDetailFailureIsNotSilent(t *testing.T) {
	stdout, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		"/api/v4/projects/p1/clips": {body: `{"clips":[{"_id":"c1","title":"x","duration":1000}],"total":1}`},
		"/api/v4/clip/c1":           r1Fail(),
	}, "clips", "list", "--project", "p1", "--min-score", "50", "--json")
	if err == nil {
		t.Fatalf("clips list exited 0 after dropping clip c1 whose detail fetch failed; stdout=%s", stdout)
	}
}

// Class B: an unrecognised facet must not make wait report READY.
func TestReviewR1WaitRejectsUnknownFacet(t *testing.T) {
	for _, include := range []string{"transcripts", "", " , "} {
		t.Run(include, func(t *testing.T) {
			stdout, _, err, hits := runAgainstFake(t, map[string]fakeRoute{
				r1Trans: {body: `{"success":true,"data":{"speakers":[]}}`},
			}, "wait", "sid-1", "--include", include, "--timeout", "2s", "--interval", "1s", "--json")
			if err == nil {
				t.Fatalf("wait --include %q exited 0 (READY) after %d requests; stdout=%s", include, hits, stdout)
			}
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--include") {
				t.Fatalf("wait --include %q: want a usage error naming --include, got exit %d: %v", include, ExitCode(err), err)
			}
		})
	}
}

// Class C: a manifest served as application/x-mpegurl is written as the manifest,
// not as the client's base64 _pp_binary envelope.
func TestReviewR1BulkExportWritesRawManifest(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "archive")
	_, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		r1Studio: {body: r1Projects},
		r1Takes:  {body: r1TakeList},
		r1Trans:  {body: r1Words},
		r1Assets: {body: r1DoneTake},
		r1VOD:    {ctype: "application/x-mpegurl; charset=utf-8", body: r1M3U8},
	}, "bulk", "export", "--studio", "st", "--out", outDir, "--json")
	if err != nil {
		t.Fatalf("bulk export: %v", err)
	}
	got, rerr := os.ReadFile(filepath.Join(outDir, "st", "P-p1", "T-sid-1", "h1.m3u8"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != r1M3U8 {
		t.Fatalf("h1.m3u8 = %q, want the raw manifest %q", got, r1M3U8)
	}
}

func TestReviewR1GrabVideoTierWritesRawManifest(t *testing.T) {
	outDir := t.TempDir()
	_, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		r1Trans:  {body: `{"success":true,"data":{"speakers":[],"voiceActivity":{"speakers":[]}}}`},
		r1Assets: {body: `{"take":{"tracks":[{"archiveId":"h1","status":"processing"}]}}`},
		r1VOD:    {ctype: "application/x-mpegurl; charset=utf-8", body: r1M3U8},
	}, "grab", "sid-1", "--out", outDir, "--json")
	if err != nil {
		t.Fatalf("grab: %v", err)
	}
	got, rerr := os.ReadFile(filepath.Join(outDir, "sid-1-h1-manifest.m3u8"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != r1M3U8 {
		t.Fatalf("manifest = %q, want the raw manifest %q", got, r1M3U8)
	}
}

// Round 2 of the same class: a row that failed must read as failed in every
// output mode, the exit code included, and a checkpoint counts only once it is
// on disk.
func TestReviewR2BulkExportResumeWriteFailureIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".resume.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, _, err, _ := runAgainstFake(t, map[string]fakeRoute{
		r1Studio: {body: r1Projects},
		r1Takes:  {body: r1TakeList},
		r1Trans:  {body: r1Words},
		r1Assets: {body: r1DoneTake},
		r1VOD:    {ctype: "application/x-mpegurl; charset=utf-8", body: r1M3U8},
	}, "bulk", "export", "--studio", "st", "--out", dir, "--resume=false", "--json")
	if err == nil {
		t.Fatalf("resume-cursor write failed but the command exited 0: %s", out)
	}
	if !strings.Contains(out, `"takes_checkpointed": 0`) {
		t.Fatalf("a checkpoint that never reached disk was counted: %s", out)
	}
}

func TestReviewR2MediaRefreshTerminalShowsFailure(t *testing.T) {
	var b bytes.Buffer
	printMediaRows(&b, []mediaRow{{Kind: "clip-export", ID: "c1", Status: "fetch-failed: HTTP 500"}})
	if !strings.Contains(b.String(), "fetch-failed") {
		t.Fatalf("terminal output hid the failure: %q", b.String())
	}
}

func TestReviewR2MediaRefreshFailedRowExitsNonZero(t *testing.T) {
	routes := map[string]fakeRoute{
		"/api/v4/projects/p1/clips/exports": {body: `{"clips":[{"_id":"c1"}]}`},
		"/api/v4/clip/c1":                   {status: http.StatusInternalServerError, body: `{"error":"boom"}`},
	}
	out, _, err, _ := runAgainstFake(t, routes, "media", "refresh", "--project", "p1", "--json")
	if err == nil {
		t.Fatalf("a failed clip fetch exited 0: %s", out)
	}
	if !strings.Contains(out, "fetch-failed") {
		t.Fatalf("the failed row is gone from the output: %s", out)
	}
}

func TestReviewR2ClipsHarvestFailedRowExitsNonZero(t *testing.T) {
	// A closed local port: the download fails without touching any network.
	const deadURL = "https://127.0.0.1:1/riverside.com/x.mp4?Signature=s"
	routes := map[string]fakeRoute{
		"/api/v4/projects/p1/ai-generation-status": {body: `{"canGenerateClips":true}`},
		"/api/v4/projects/p1/clips":                {body: `{"clips":[{"_id":"c1","title":"T"}],"total":1}`},
		"/api/v4/clip/c1":                          {body: `{"data":{"url":"` + deadURL + `"}}`},
	}
	out, _, err, _ := runAgainstFake(t, routes, "clips", "harvest", "--project", "p1", "--download", "--out", t.TempDir(), "--json")
	if err == nil {
		t.Fatalf("a failed clip download exited 0: %s", out)
	}
	if !strings.Contains(out, "download-failed") {
		t.Fatalf("the failed row is gone from the output: %s", out)
	}
}

func TestRowStatusFailed(t *testing.T) {
	for status, want := range map[string]bool{
		"": false, "downloaded": false, "ready": false, "not-rendered": false, "metadata-only": false,
		"fetch-failed": true, "failed: HTTP 500": true, "download-failed: x": true, "skipped: name/id would escape --out": true, "anything new": true,
	} {
		if got := rowStatusFailed(status); got != want {
			t.Errorf("rowStatusFailed(%q) = %v, want %v", status, got, want)
		}
	}
}
