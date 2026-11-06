// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import "testing"

func TestParseSizeBytes(t *testing.T) {
	cases := map[string]int64{
		"0 Bytes": 0,
		"512 B":   512,
		"1 KB":    1024,
		"1 MB":    1048576,
		"1 GB":    1073741824,
	}
	for in, want := range cases {
		if got := ParseSizeBytes(in); got != want {
			t.Fatalf("ParseSizeBytes(%q) = %d, want %d", in, got, want)
		}
	}
	// The portal's unit is binary, not decimal: it called a chunk
	// "871.26 MB" and the downloaded file was 913,581,909 bytes. The
	// string is rounded to two decimals, so the byte count it yields is
	// approximate by a few hundred bytes; what matters is the scale.
	const observed = 913581909
	got := ParseSizeBytes("871.26 MB")
	if diff := got - observed; diff > 4096 || diff < -4096 {
		t.Fatalf("871.26 MB = %d, want within 4 KiB of the observed %d", got, observed)
	}
	if decimal := int64(871260000); got < decimal {
		t.Fatalf("871.26 MB = %d, which reads as decimal MB rather than MiB", got)
	}
	// The 871.26 MB case above is the live cross-check: the portal called
	// that chunk 871.26 MB and the downloaded file was 913,581,909 bytes,
	// so its "MB" is MiB.
	if ParseSizeBytes("1.79 GB") <= ParseSizeBytes("224.04 KB") {
		t.Fatal("GB must scale above KB")
	}
	// An unreadable size must be -1, never 0: treating it as empty would
	// skip a mailbox that may hold data.
	for _, bad := range []string{"", "unknown", "lots", "12 parsecs", "Not Accessible"} {
		if got := ParseSizeBytes(bad); got != -1 {
			t.Fatalf("ParseSizeBytes(%q) = %d, want -1", bad, got)
		}
	}
}

func TestTaskEntityInfoAccessible(t *testing.T) {
	if (TaskEntityInfo{Status: "Not Accessible"}).Accessible() {
		t.Fatal("Not Accessible must report inaccessible")
	}
	for _, s := range []string{"Active", "Archived", ""} {
		if !(TaskEntityInfo{Status: s}).Accessible() {
			t.Fatalf("%q should be accessible", s)
		}
	}
}
