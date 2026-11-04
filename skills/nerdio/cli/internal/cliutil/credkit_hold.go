// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). The shared
// credential kit (internal/credkit) holds every credential and checks every
// sink. This file is the connector's one door to its holder, so code outside
// internal/credwire names a source and never the kit's internals.

package cliutil

import (
	"strings"

	"nerdio-pp-cli/internal/credkit"
)

// Hold is credkit.Hold. Production code never reassigns it. In-process unit
// tests may, to a no-op: a held value lasts for the process and the kit
// withholds whatever carries it, so one test's fixture would withhold the next
// test's output. The built binaries' tests (internal/credwire) prove the real
// thing.
var Hold = credkit.Hold

// HoldBody is credkit.HoldBody, a variable for the same reason as Hold. A
// config or credential file read after Start is held with it before it is
// parsed, so a parse diagnostic cannot carry what the file says.
var HoldBody = credkit.HoldBody

// HoldValues holds each non-blank value under source.
func HoldValues(source credkit.Source, values ...string) {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			Hold(source, v)
		}
	}
}

// HoldAuthHeader holds an Authorization value whole and, after its scheme
// word, the credential alone.
func HoldAuthHeader(source credkit.Source, v string) {
	HoldValues(source, v)
	if _, rest, ok := strings.Cut(strings.TrimSpace(v), " "); ok {
		HoldValues(source, strings.TrimSpace(rest))
	}
}

// HoldURL is credkit.HoldURL, a variable for the same reason as Hold.
var HoldURL = credkit.HoldURL

// HoldURLCredentials holds the password and every query value of raw under
// source (credkit.HoldURL).
func HoldURLCredentials(source credkit.Source, raw string) {
	if strings.TrimSpace(raw) != "" {
		HoldURL(source, raw)
	}
}
