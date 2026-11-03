// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: exposes the built-in base_url default so doctor compares a
// configured base_url against the value this connector actually ships, from
// one source, instead of repeating the placeholder host as a literal.

package config

// ShippedBaseURL is the base_url a fresh install uses when neither
// HUDU_BASE_URL nor a config file sets one (the spec's per-tenant placeholder).
func ShippedBaseURL() string { return builtinBaseURL() }
