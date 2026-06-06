// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestQbrAllSlug(t *testing.T) {
	cases := []struct {
		name string
		id   int64
		want string
	}{
		{"Acme Co", 1, "acme-co"},
		{"  Acme & Sons, LLC. ", 2, "acme-sons-llc"},
		{"ÜberBackup", 3, "berbackup"},
		{"", 44, "44"},
		{"---", 45, "45"},
		{"already-slugged", 5, "already-slugged"},
	}
	for _, c := range cases {
		if got := qbrAllSlug(c.name, c.id); got != c.want {
			t.Errorf("qbrAllSlug(%q,%d) = %q, want %q", c.name, c.id, got, c.want)
		}
	}
}
