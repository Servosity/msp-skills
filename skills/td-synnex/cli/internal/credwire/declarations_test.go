// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"slices"
	"testing"
)

// urlEnv is read inside Start, so it cannot name the Declarations literal;
// it must list exactly the URL variables the literal declares.
func TestURLEnvMatchesDeclarations(t *testing.T) {
	want := append([]string{Declarations.BaseURL.Env}, Declarations.URLEnv...)
	got := slices.Clone(urlEnv)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("urlEnv = %v, want BaseURL.Env plus URLEnv = %v", got, want)
	}
}

func TestURLValuedProfileNames(t *testing.T) {
	for name, want := range map[string]string{
		"base-url":          "profile:base-url",
		"BASE_URL":          "profile:base-url",
		"expected-base-url": "profile:url",
		"feedback-endpoint": "profile:url",
		"deliver":           "",
		"select":            "",
		"account-id":        "",
	} {
		if got := urlValuedName(name); got != want {
			t.Errorf("urlValuedName(%q) = %q, want %q", name, got, want)
		}
	}
}
