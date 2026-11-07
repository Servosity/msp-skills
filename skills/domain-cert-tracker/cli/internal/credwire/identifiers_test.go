// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json config-reads-held-before-parse):
// the config keys the per-read hold leaves unheld are exactly the config
// keys the kit exempts, so the two never disagree about what a credential is.

package credwire

import (
	"slices"
	"strings"
	"testing"

	"domain-cert-tracker-pp-cli/internal/config"
)

func TestIdentifiersMatchConfigNotCredentials(t *testing.T) {
	var exempt []string
	for _, id := range Declarations.Identifiers {
		if k, ok := strings.CutPrefix(id.Source, "config:"); ok {
			exempt = append(exempt, k)
		}
	}
	want := slices.Clone(config.NotCredentials)
	slices.Sort(exempt)
	slices.Sort(want)
	if !slices.Equal(exempt, want) {
		t.Fatalf("config: Identifiers %q, config.NotCredentials %q: the kit and the per-read hold disagree on which keys are credentials", exempt, want)
	}
}
