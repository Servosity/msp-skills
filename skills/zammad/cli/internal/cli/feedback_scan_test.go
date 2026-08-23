// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"path/filepath"
	"testing"

	"zammad-pp-cli/internal/store"
)

func TestFeedbackScanCountsUniqueTicketsAcrossFullCorpus(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "feedback.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	statements := []string{
		`INSERT INTO tickets (id, data, number, title) VALUES ('1', '{}', '1001', 'Feature request for exports')`,
		`INSERT INTO tickets (id, data, number, title) VALUES ('2', '{}', '1002', 'Neutral title')`,
		`INSERT INTO tickets (id, data, number, title) VALUES ('3', '{}', '1003', 'Invoice question')`,
		`INSERT INTO tickets (id, data, number, title) VALUES ('4', '{}', '1004', 'Neutral title')`,
		`INSERT INTO tickets (id, data, number, title) VALUES ('5', '{}', '1005', 'Neutral title')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a1', '{}', 1, 2, 0, 'Please add this feature', '2026-01-01T00:00:00Z')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a2', '{}', 1, 2, 0, 'Another feature request', '2026-01-02T00:00:00Z')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a3', '{}', 2, 1, 0, 'The customer asked about pricing', '2026-01-01T00:00:00Z')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a4', '{}', 3, 2, 0, 'The cost is too much', '2026-01-01T00:00:00Z')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a5', '{}', 3, 2, 0, 'Pricing is expensive', '2026-01-02T00:00:00Z')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a6', '{}', 4, 2, 1, 'This pricing note is internal', '2026-01-01T00:00:00Z')`,
		`INSERT INTO articles (id, data, ticket_id, sender_id, internal, body, created_at) VALUES ('a7', '{}', 5, 2, 0, 'The restore is broken', '2026-01-01T00:00:00Z')`,
	}
	for _, statement := range statements {
		if _, err := db.DB().Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	out, err := buildFeedbackScanOutput(db, "", 1)
	if err != nil {
		t.Fatalf("buildFeedbackScanOutput: %v", err)
	}
	for _, tt := range []struct {
		bucket string
		want   int
	}{
		{bucket: "feature", want: 1},
		{bucket: "pricing", want: 1},
		{bucket: "compliance", want: 0},
		{bucket: "bug", want: 1},
	} {
		t.Run(tt.bucket, func(t *testing.T) {
			if got := out.Counts[tt.bucket]; got != tt.want {
				t.Fatalf("counts[%q] = %d, want %d", tt.bucket, got, tt.want)
			}
		})
	}
	rows := 0
	for _, bucketRows := range out.Buckets {
		rows += len(bucketRows)
	}
	if rows != 1 {
		t.Fatalf("limited result rows = %d, want 1 while counts cover full corpus", rows)
	}
}
