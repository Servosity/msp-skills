// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

package avananmirror

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"avanan-pp-cli/internal/cliutil"
)

// fakePoster records the bodies it is handed and replays canned responses.
type fakePoster struct {
	responses []string
	bodies    []map[string]any
	getPaths  []string
	getResp   map[string]string
	calls     int
}

func (f *fakePoster) Post(_ context.Context, _ string, body any) (json.RawMessage, int, error) {
	if b, ok := body.(map[string]any); ok {
		if rd, ok := b["requestData"].(map[string]any); ok {
			f.bodies = append(f.bodies, rd)
		}
	}
	if f.calls >= len(f.responses) {
		return nil, 0, fmt.Errorf("unexpected call %d", f.calls)
	}
	resp := f.responses[f.calls]
	f.calls++
	return json.RawMessage(resp), 200, nil
}

func (f *fakePoster) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	f.getPaths = append(f.getPaths, path)
	if resp, ok := f.getResp[path]; ok {
		return json.RawMessage(resp), nil
	}
	return json.RawMessage(`{"responseEnvelope":{"responseCode":200},"responseData":[]}`), nil
}

func TestDecodeSurfacesEnvelopeFailureOnHTTP200(t *testing.T) {
	// The API returns HTTP 200 with a failure code in the envelope. Treating
	// that as success is how a caller silently ends up with zero rows and no
	// error.
	body := `{"responseEnvelope":{"responseCode":403,"responseText":"Forbidden","additionalText":"no licence"},"responseData":[]}`
	if _, err := Decode(json.RawMessage(body)); err == nil {
		t.Fatal("Decode() returned nil error for envelope responseCode 403")
	}
}

func TestDecodeAcceptsSuccessCodes(t *testing.T) {
	for _, code := range []string{"0", "200"} {
		body := fmt.Sprintf(`{"responseEnvelope":{"responseCode":%s},"responseData":[]}`, code)
		if _, err := Decode(json.RawMessage(body)); err != nil {
			t.Errorf("Decode() with responseCode %s returned error: %v", code, err)
		}
	}
}

func TestEnvelopeRecords(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "array payload", body: `{"responseData":[{"a":1},{"a":2}]}`, want: 2},
		{name: "single object payload", body: `{"responseData":{"a":1}}`, want: 1},
		{name: "null payload", body: `{"responseData":null}`, want: 0},
		{name: "absent payload", body: `{"responseEnvelope":{"responseCode":200}}`, want: 0},
		{name: "empty array", body: `{"responseData":[]}`, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := Decode(json.RawMessage(tt.body))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			got, err := env.Records()
			if err != nil {
				t.Fatalf("Records: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("Records() = %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestRecordID(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "eventId wins", body: `{"eventId":"e1","id":"other"}`, want: "e1"},
		{name: "entityId", body: `{"entityId":"x9"}`, want: "x9"},
		{name: "plain id", body: `{"id":"abc"}`, want: "abc"},
		{name: "numeric id", body: `{"id":121775995}`, want: "121775995"},
		{name: "empty string id is ignored", body: `{"id":"","entityId":"e"}`, want: "e"},
		{name: "no identifier", body: `{"subject":"hi"}`, want: ""},
		{name: "not an object", body: `[1,2]`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recordID(json.RawMessage(tt.body)); got != tt.want {
				t.Errorf("recordID(%s) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestFlattenExceptionNormalizesEachSubSystem(t *testing.T) {
	tests := []struct {
		name          string
		src           ExceptionSource
		body          string
		wantMatch     string
		wantListSide  string
		wantCreatedBy string
	}{
		{
			name:          "anti-phishing whitelist by sender",
			src:           ExceptionSource{SubSystem: "anti-phishing-whitelist", Engine: "whitelist"},
			body:          `{"id":"1","senderEmail":"a@b.com","createdBy":"marcus"}`,
			wantMatch:     "a@b.com",
			wantListSide:  "allow",
			wantCreatedBy: "marcus",
		},
		{
			name:         "url reputation block by url",
			src:          ExceptionSource{SubSystem: "url-reputation-block", Engine: "avanan_url"},
			body:         `{"id":"2","url":"http://bad.example"}`,
			wantMatch:    "http://bad.example",
			wantListSide: "block",
		},
		{
			name:         "anti-malware by hash",
			src:          ExceptionSource{SubSystem: "anti-malware", Engine: "checkpoint2"},
			body:         `{"id":"3","hash":"d41d8cd9"}`,
			wantMatch:    "d41d8cd9",
			wantListSide: "n/a",
		},
		{
			name:         "click-time item by name",
			src:          ExceptionSource{SubSystem: "click-time-protection", Engine: "click_time_protection"},
			body:         `{"id":"4","listItemName":"partner.example"}`,
			wantMatch:    "partner.example",
			wantListSide: "n/a",
		},
		{
			name:         "unparseable payload degrades without panicking",
			src:          ExceptionSource{SubSystem: "dlp", Engine: "avanan_dlp"},
			body:         `"not-an-object"`,
			wantMatch:    "",
			wantListSide: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FlattenException(tt.src, json.RawMessage(tt.body))
			if got.MatchString != tt.wantMatch {
				t.Errorf("MatchString = %q, want %q", got.MatchString, tt.wantMatch)
			}
			if tt.wantListSide != "" && got.ListSide != tt.wantListSide {
				t.Errorf("ListSide = %q, want %q", got.ListSide, tt.wantListSide)
			}
			if tt.wantCreatedBy != "" && got.CreatedBy != tt.wantCreatedBy {
				t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, tt.wantCreatedBy)
			}
			if got.SubSystem != tt.src.SubSystem {
				t.Errorf("SubSystem = %q, want %q", got.SubSystem, tt.src.SubSystem)
			}
		})
	}
}

func TestExceptionSourcesCoverEverySubSystem(t *testing.T) {
	sources := ExceptionSources()
	if len(sources) < 9 {
		t.Fatalf("ExceptionSources() = %d entries; the API exposes seven engines across nine listing endpoints", len(sources))
	}
	seen := map[string]bool{}
	for _, s := range sources {
		if s.SubSystem == "" || s.Path == "" || s.Engine == "" {
			t.Errorf("incomplete source: %+v", s)
		}
		if seen[s.SubSystem] {
			t.Errorf("duplicate sub-system %q", s.SubSystem)
		}
		seen[s.SubSystem] = true
	}
	for _, want := range []string{"anti-malware", "dlp", "anomaly", "click-time-protection"} {
		if !seen[want] {
			t.Errorf("ExceptionSources() is missing %q", want)
		}
	}
}

func TestCloneWithScrollDoesNotMutateBase(t *testing.T) {
	base := map[string]any{"startDate": "2026-01-01T00:00:00Z"}
	out := cloneWithScroll(base, "scroll-1", 50)

	if _, leaked := base["scrollId"]; leaked {
		t.Error("cloneWithScroll mutated the base request data; paging would corrupt the next window")
	}
	if out["scrollId"] != "scroll-1" {
		t.Errorf("scrollId = %v, want scroll-1", out["scrollId"])
	}
	if out["pageSize"] != 50 {
		t.Errorf("pageSize = %v, want 50", out["pageSize"])
	}
	if out["startDate"] != base["startDate"] {
		t.Error("cloneWithScroll dropped the base fields")
	}
}

func TestOptionsPageSizeIsBounded(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{in: 0, want: MaxPageSize},
		{in: -5, want: MaxPageSize},
		{in: 50, want: 50},
		{in: MaxPageSize + 1000, want: MaxPageSize},
	}
	for _, tt := range tests {
		if got := (Options{PageSize: tt.in}).pageSize(); got != tt.want {
			t.Errorf("Options{PageSize:%d}.pageSize() = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// rateLimitedPoster fails the first exception sub-system with a typed
// rate-limit error.
type rateLimitedPoster struct{ gets int }

func (r *rateLimitedPoster) Post(_ context.Context, _ string, _ any) (json.RawMessage, int, error) {
	return nil, 0, fmt.Errorf("unused")
}

func (r *rateLimitedPoster) Get(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	r.gets++
	return nil, fmt.Errorf("listing exceptions: %w", &cliutil.RateLimitError{})
}

// TestExceptionsAbortsOnRateLimit is the guard against the worst failure mode
// this CLI could have: a throttled ingest that looks like an empty result.
// If Exceptions kept going, `exceptions find` would answer "not excepted
// anywhere" from a table it never finished filling.
func TestExceptionsAbortsOnRateLimit(t *testing.T) {
	p := &rateLimitedPoster{}
	res, problems := Exceptions(context.Background(), p, nil, Options{})

	if p.gets != 1 {
		t.Errorf("attempted %d sub-systems after a 429; want 1 (the run must abort, not degrade)", p.gets)
	}
	if len(problems) == 0 {
		t.Fatal("Exceptions() reported no problem for a rate-limited run")
	}
	if !IsRateLimited(problems[0]) {
		t.Errorf("problem %v is not detected as a rate-limit error; the caller cannot distinguish throttling from an empty tenant", problems[0])
	}
	if res.Stored != 0 {
		t.Errorf("Stored = %d, want 0", res.Stored)
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "typed rate limit", err: &cliutil.RateLimitError{}, want: true},
		{name: "wrapped rate limit", err: fmt.Errorf("ctx: %w", &cliutil.RateLimitError{}), want: true},
		{name: "ordinary error", err: fmt.Errorf("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRateLimited(tt.err); got != tt.want {
				t.Errorf("IsRateLimited(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
