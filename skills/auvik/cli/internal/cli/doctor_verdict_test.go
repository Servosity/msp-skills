// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pins the doctor verdict for Auvik's shipped default base URL.

package cli

import "testing"

// Auvik's default base URL is the real US1 regional API root. Treating it as a
// placeholder (the shipped doctor did) told every US1 operator who never set
// AUVIK_BASE_URL that their working install was unconfigured, and skipped the
// credential probe. Placeholder templates and RFC 2606 hosts stay refused.
func TestDoctorShippedAuvikBaseURLIsNotAPlaceholder(t *testing.T) {
	for _, base := range []string{"https://auvikapi.us1.my.auvik.com", "https://auvikapi.eu1.my.auvik.com/"} {
		if doctorBaseURLIsPlaceholder(base) {
			t.Errorf("%s is a real Auvik regional API root, not a placeholder", base)
		}
	}
	for _, base := range []string{"https://{region}.my.auvik.com", "https://api.example.com"} {
		if !doctorBaseURLIsPlaceholder(base) {
			t.Errorf("%s must be refused as a placeholder", base)
		}
	}
}
