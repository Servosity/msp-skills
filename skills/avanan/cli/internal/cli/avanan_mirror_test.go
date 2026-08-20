// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"avanan-pp-cli/internal/avananmirror"
)

// TestParseMirrorResources guards the flag-vocabulary/store-key split.
//
// Store keys are namespaced (avanan_events) so they cannot collide with the
// generated syncer's tables; the flag vocabulary is the short domain word.
// Comparing user input against the namespaced constant made every value
// invalid AND every default lookup miss — turning `mirror` into a silent no-op
// and leaving every offline command permanently empty. These cases fail loudly
// if the two are ever conflated again.
func TestParseMirrorResources(t *testing.T) {
	tests := []struct {
		name    string
		csv     string
		want    []string
		wantErr bool
	}{
		{
			name: "empty selects everything",
			csv:  "",
			want: []string{avananmirror.ResourceEvents, avananmirror.ResourceEntities, avananmirror.ResourceExceptions},
		},
		{
			name: "single resource",
			csv:  "events",
			want: []string{avananmirror.ResourceEvents},
		},
		{
			name: "multiple resources",
			csv:  "events,exceptions",
			want: []string{avananmirror.ResourceEvents, avananmirror.ResourceExceptions},
		},
		{
			name: "whitespace and case are tolerated",
			csv:  " Events , ENTITIES ",
			want: []string{avananmirror.ResourceEvents, avananmirror.ResourceEntities},
		},
		{
			name: "empty segments are ignored",
			csv:  "events,,",
			want: []string{avananmirror.ResourceEvents},
		},
		{
			name:    "unknown resource is rejected",
			csv:     "nonsense",
			wantErr: true,
		},
		{
			// The store key must NOT be accepted as user input; if it is, the
			// two vocabularies have been conflated again.
			name:    "store key is not a valid flag value",
			csv:     "avanan_events",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMirrorResources(tt.csv)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseMirrorResources(%q) = %v, want error", tt.csv, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMirrorResources(%q): %v", tt.csv, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseMirrorResources(%q) selected %d resources, want %d (%v)", tt.csv, len(got), len(tt.want), got)
			}
			for _, w := range tt.want {
				if !got[w] {
					t.Errorf("parseMirrorResources(%q) did not select %q; got %v", tt.csv, w, got)
				}
			}
		})
	}
}

// TestMirrorResourceNamesCoverEveryStoreResource keeps the flag vocabulary and
// the ingester's resource set in sync, so adding a resource to one without the
// other fails here instead of silently becoming unreachable from the CLI.
func TestMirrorResourceNamesCoverEveryStoreResource(t *testing.T) {
	for _, resource := range []string{
		avananmirror.ResourceEvents,
		avananmirror.ResourceEntities,
		avananmirror.ResourceExceptions,
	} {
		found := false
		for _, mapped := range mirrorResourceNames {
			if mapped == resource {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("store resource %q has no --resources name; users cannot select it", resource)
		}
	}
}
