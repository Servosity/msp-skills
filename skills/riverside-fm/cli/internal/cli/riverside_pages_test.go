// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// A studio with more projects, or a project with more takes, than one page holds
// must be read whole. The fake caps every page at one item, whatever limit asks.
func TestListsReadEveryPage(t *testing.T) {
	takeRoute := func(sid string) fakeRoute {
		return fakeRoute{body: `{"takes":[{"_id":"t-` + sid + `","title":"T","recordings":[{"sessionId":"` + sid + `"}]}]}`}
	}
	dir := t.TempDir()
	routes := map[string]fakeRoute{
		r1Studio: {body: `{"projects":[]}`, byOffset: map[string]string{
			"0": `{"projects":[{"_id":"p1","title":"P1","createdAt":"2026-01-01T00:00:00Z"}]}`,
			"1": `{"projects":[{"_id":"p2","title":"P2","createdAt":"2026-01-01T00:00:00Z"}]}`,
		}},
		r1Takes: {body: `{"takes":[]}`, byOffset: map[string]string{
			"0": takeRoute("sid-1").body,
			"1": takeRoute("sid-2").body,
		}},
		"/api/v4/projects/p2/takes": {body: `{"takes":[]}`, byOffset: map[string]string{"0": takeRoute("sid-3").body}},
	}
	for _, sid := range []string{"sid-1", "sid-2", "sid-3"} {
		routes["/api/v4/transcriptions/editableWithVoiceActivity/"+sid] = fakeRoute{body: r1Words}
		routes["/api/v4/take/"+sid+"/assets"] = fakeRoute{body: r1DoneTake}
		routes["/api/v4/vod/"+sid+"/h1"] = fakeRoute{ctype: "application/x-mpegurl; charset=utf-8", body: r1M3U8}
	}
	out, _, err, _ := runAgainstFake(t, routes, "bulk", "export", "--studio", "st", "--out", dir, "--json")
	if err != nil {
		t.Fatalf("bulk export: %v\n%s", err, out)
	}
	for _, sid := range []string{"sid-1", "sid-2", "sid-3"} {
		if !resumeHas(t, dir, sid) {
			t.Errorf("%s on a later page was never exported, and the run exited 0:\n%s", sid, out)
		}
	}
}

func TestRefreshAndClipsListReadEveryPage(t *testing.T) {
	routes := map[string]fakeRoute{
		"/api/v4/projects/p1/clips/exports": {body: `{"clips":[]}`, byOffset: map[string]string{
			"0": `{"clips":[{"_id":"c1"}]}`,
			"1": `{"clips":[{"_id":"c2"}]}`,
		}},
		"/api/v4/clip/c1": {body: `{}`},
		"/api/v4/clip/c2": {status: 500, body: `{"error":"boom"}`},
	}
	out, _, err, _ := runAgainstFake(t, routes, "media", "refresh", "--project", "p1", "--json")
	if err == nil || !strings.Contains(out, "c2") {
		t.Errorf("media refresh never reached the clip on page two (err=%v): %s", err, out)
	}
	routes = map[string]fakeRoute{
		"/api/v4/projects/p1/clips": {body: `{"clips":[],"total":2}`, byOffset: map[string]string{
			"0": `{"clips":[{"_id":"c1","title":"one"}],"total":2}`,
			"1": `{"clips":[{"_id":"c2","title":"two"}],"total":2}`,
		}},
		"/api/v4/clip/c1": {body: `{"_id":"c1","title":"one"}`},
		"/api/v4/clip/c2": {body: `{"_id":"c2","title":"two"}`},
	}
	out, _, err, _ = runAgainstFake(t, routes, "clips", "list", "--project", "p1", "--json")
	if err != nil || !strings.Contains(out, "c2") {
		t.Errorf("clips list never reached the clip on page two (err=%v): %s", err, out)
	}
}

// Two items whose titles share their first 80 bytes must not land on one path:
// the id and extension survive, so the second download cannot overwrite the first.
func TestLongTitleKeepsIDAndExtension(t *testing.T) {
	title := strings.Repeat("The Weekly Show \u2014 ", 8)
	dir := t.TempDir()
	a := safeJoin(dir, title+"-69fcda9fba030a19ae93a526.mp4")
	b := safeJoin(dir, title+"-69fcda9fba030a19ae93a527.mp4")
	if a == b {
		t.Fatalf("two clips share one path, so one overwrites the other: %s", a)
	}
	for _, p := range []string{a, b} {
		base := filepath.Base(p)
		if !strings.HasSuffix(base, ".mp4") {
			t.Errorf("extension lost: %q", base)
		}
		if len(base) > 80 {
			t.Errorf("%d bytes, want at most 80: %q", len(base), base)
		}
		if !utf8.ValidString(base) {
			t.Errorf("not valid UTF-8: %q", base)
		}
	}
}
