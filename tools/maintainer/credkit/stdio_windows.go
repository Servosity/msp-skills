// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"errors"
	"log"
	"os"
	"syscall"
)

var (
	setStdHandle = syscall.NewLazyDLL("kernel32.dll").NewProc("SetStdHandle")
	// replaced keeps the os.Stdout and os.Stderr the program had before
	// Start reachable: a stream captured earlier (a package-level logger)
	// still writes through one, to a handle number that now names a pipe,
	// and its finalizer must never close that handle.
	replaced []*os.File
	// taken maps each original handle number the pipe took over to the
	// duplicate that now carries the original stream.
	taken = map[syscall.Handle]syscall.Handle{}
	// squatted is the handle number taken over for fd 1 and fd 2.
	squatted = map[int]syscall.Handle{}
)

// redirectStd points the process's standard output (fd 1) or error (fd 2)
// at a new pipe: the standard handle, os.Stdout or os.Stderr, the standard
// logger, and the original handle number itself, which is closed and taken
// over by a duplicate of the pipe, so a stream captured before Start writes
// into the pipe too. It returns the original stream, on a duplicate handle,
// and the pipe's read end. If the number cannot be taken over, the error
// makes Start exit before anything is written.
func redirectStd(fd int) (orig, r *os.File, err error) {
	if err := setStdHandle.Find(); err != nil {
		return nil, nil, err
	}
	std, cur := int32(syscall.STD_OUTPUT_HANDLE), os.Stdout
	if fd == 2 {
		std, cur = int32(syscall.STD_ERROR_HANDLE), os.Stderr
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	fail := func(e error) (*os.File, *os.File, error) {
		r.Close()
		w.Close()
		return nil, nil, e
	}
	p, err := syscall.GetCurrentProcess()
	if err != nil {
		return fail(err)
	}
	dup := func(h syscall.Handle) (syscall.Handle, error) {
		var d syscall.Handle
		err := syscall.DuplicateHandle(p, h, p, &d, 0, false, syscall.DUPLICATE_SAME_ACCESS)
		return d, err
	}
	o := syscall.InvalidHandle // no stream: nothing to write to, nothing to take over
	switch h := syscall.Handle(cur.Fd()); {
	case h == syscall.InvalidHandle || h == 0:
	case taken[h] != 0:
		// stdout and stderr were one handle, already taken over: a stream
		// captured before Start writes into stdout's pipe, which goes where
		// that handle went.
		if o, err = dup(taken[h]); err != nil {
			return fail(err)
		}
	default:
		if o, err = dup(h); err != nil {
			return fail(err)
		}
		syscall.CloseHandle(h)
		if err := squat(dup, syscall.Handle(w.Fd()), h); err != nil {
			syscall.CloseHandle(o)
			return fail(err)
		}
		taken[h], squatted[fd] = o, h
	}
	if ok, _, e := setStdHandle.Call(uintptr(uint32(std)), w.Fd()); ok == 0 {
		return fail(e)
	}
	replaced = append(replaced, cur)
	if fd == 1 {
		os.Stdout = w
	} else {
		os.Stderr = w
		log.SetOutput(w)
	}
	return os.NewFile(uintptr(o), "original"), r, nil
}

// pending is unknown on Windows: a synchronous pipe serializes its I/O, so
// asking (PeekNamedPipe) would wait behind the read it asks about.
func pending(*os.File) int { return -1 }

// squat duplicates src until the duplicate's handle number is want, which
// was just closed (Windows hands out the lowest free number), then closes
// the misses.
func squat(dup func(syscall.Handle) (syscall.Handle, error), src, want syscall.Handle) error {
	var misses []syscall.Handle
	defer func() {
		for _, m := range misses {
			syscall.CloseHandle(m)
		}
	}()
	for range 256 {
		d, err := dup(src)
		if err != nil {
			return err
		}
		if d == want {
			return nil
		}
		misses = append(misses, d)
	}
	return errors.New("credkit: the standard handle could not be taken over")
}

// releaseStd closes the pipe's write ends (the new stream and the handle
// number taken over) so the pump sees the end of the stream.
func releaseStd(fd int) {
	if h, ok := squatted[fd]; ok {
		syscall.CloseHandle(h)
		delete(squatted, fd)
	}
	if fd == 1 {
		os.Stdout.Close()
	} else {
		os.Stderr.Close()
	}
}

// fdAppends: a descriptor's own O_APPEND is not read on this platform;
// a file opened with O_APPEND through os is (appendMode).
func fdAppends(*os.File) bool { return false }

const fdFlagReadable = false

func fdAppendFlag(*os.File) (app, known bool) { return false, false }

// appendAt writes b at f's end, off: a descriptor's flags do not change
// after the open on this platform.
func appendAt(f *os.File, b []byte, _ int64) (int, error) { return f.Write(b) }
