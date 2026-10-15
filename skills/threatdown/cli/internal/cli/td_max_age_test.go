// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// The fleet reports' staleness hint honors an explicit global --max-age.
func TestTDMaxAgeHonorsExplicitFlag(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want time.Duration
	}{{nil, 24 * time.Hour}, {[]string{"--max-age", "1m"}, time.Minute}, {[]string{"--max-age", "0"}, 0}} {
		var got time.Duration
		var maxAge time.Duration
		root := &cobra.Command{Use: "root"}
		root.PersistentFlags().DurationVar(&maxAge, "max-age", 30*time.Minute, "")
		child := &cobra.Command{Use: "child", RunE: func(cmd *cobra.Command, _ []string) error { got = tdMaxAge(cmd); return nil }}
		root.AddCommand(child)
		root.SetArgs(append([]string{"child"}, tc.args...))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("args %v: tdMaxAge = %s, want %s", tc.args, got, tc.want)
		}
	}
}
