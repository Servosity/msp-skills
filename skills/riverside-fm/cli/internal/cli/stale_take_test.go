// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestClassifyStaleTake(t *testing.T) {
	cases := []struct {
		name, body, stage string
		pending           int
		done, dated       bool
	}{
		{"guest upload never finished", `{"take":{"createdAt":"2026-05-07T20:06:38.056Z","tracks":[{"speakerName":"Host","status":"done","clientStatus":"uploaded"},{"speakerName":"Guest","status":"recording","clientStatus":"uploading"}]}}`, "uploading", 1, false, true},
		{"uploaded, not processed", `{"take":{"createdAt":"2026-05-07T20:06:38Z","tracks":[{"speakerName":"Guest","status":"converting","clientStatus":"uploaded"}]}}`, "processing", 1, false, true},
		{"all tracks done", `{"take":{"createdAt":"2026-05-07T20:06:38Z","tracks":[{"speakerName":"Host","status":"done","clientStatus":"uploaded"}]}}`, "", 0, true, true},
		{"no tracks", `{"take":{"createdAt":"2026-05-07T20:06:38Z","tracks":[]}}`, "no-tracks", 0, false, true},
		{"undated", `{"take":{"tracks":[{"status":"done","clientStatus":"uploaded"}]}}`, "", 0, true, false},
	}
	for _, tc := range cases {
		stage, pending, created, done, err := classifyStaleTake([]byte(tc.body))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if stage != tc.stage || len(pending) != tc.pending || done != tc.done || created.IsZero() == tc.dated {
			t.Errorf("%s: got stage=%q pending=%d done=%v dated=%v", tc.name, stage, len(pending), done, !created.IsZero())
		}
	}
	if _, _, _, _, err := classifyStaleTake([]byte(`not json`)); err == nil {
		t.Error("malformed assets must be an error, not a clean take")
	}
}
