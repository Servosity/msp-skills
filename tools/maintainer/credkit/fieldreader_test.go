// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import "io"

// The response scanner's tests read a body through it as a stream; the kit
// itself reads every body with declared fields whole (fieldBody).

// fieldReader passes a body to the program through a fieldScan.
type fieldReader struct {
	src  io.Reader
	scan *fieldScan
	buf  []byte
	out  []byte
	err  error
}

func newFieldReader(r io.Reader, fields []string, mode ...scanMode) io.Reader {
	m := sniffJSON
	if len(mode) > 0 {
		m = mode[0]
	}
	return &fieldReader{src: r, scan: newFieldScan(fields, m), buf: make([]byte, 32<<10)}
}

func (f *fieldReader) Read(p []byte) (int, error) {
	for len(f.out) == 0 && f.err == nil {
		k, err := f.src.Read(f.buf)
		f.out = append(f.out, f.scan.feed(f.buf[:k])...)
		if err != nil {
			f.out = append(f.out, f.scan.finish()...)
			f.err = err
		}
	}
	n := copy(p, f.out)
	f.out = f.out[n:]
	if len(f.out) == 0 && f.err != nil {
		return n, f.err
	}
	return n, nil
}
