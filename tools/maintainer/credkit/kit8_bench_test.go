// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The file sink's cost: 1000 small appends to one file through one
// Writer, and one 150 KB write, with the 20 bench credentials held.
//
//	go test -run '^$' -bench 'File' -benchtime 5x

import (
	"fmt"
	"path/filepath"
	"testing"
)

func BenchmarkFileAppend1000(b *testing.B) {
	benchHold(b)
	dir := b.TempDir()
	recs := make([][]byte, 1000)
	for i := range recs {
		recs[i] = fmt.Appendf(nil, "2026-11-02T10:%02d:%02dZ ticket %d updated by agent %d\n", i/60%60, i%60, 85000+i, i%17)
	}
	b.ResetTimer()
	for n := range b.N {
		w, err := Create(filepath.Join(dir, fmt.Sprint(n)))
		if err != nil {
			b.Fatal(err)
		}
		for _, r := range recs {
			if _, err := w.Write(r); err != nil {
				b.Fatal(err)
			}
		}
		w.Close()
	}
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/1000-appends")
}

func BenchmarkFileWrite150K(b *testing.B) {
	benchHold(b)
	dir := b.TempDir()
	doc := benchJSON(150 << 10)
	b.SetBytes(int64(len(doc)))
	b.ResetTimer()
	for n := range b.N {
		if err := WriteFile(filepath.Join(dir, fmt.Sprint(n)), doc, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/write")
}
