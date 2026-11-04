// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"errors"
	"log"
	"os"
	"runtime"
	"syscall"
	"time"
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
// at a new pipe: the standard handle, os.Stdout or os.Stderr, and the original
// handle number itself, which is closed and taken over by a duplicate of the
// pipe, so a stream captured before Start writes into the pipe too (the standard
// logger follows in retargetLog). It returns the original stream, on a duplicate handle,
// and the pipe's read end. If the number cannot be taken over, the error
// makes Start exit before anything is written.
// openNonblock: opening a file does not block on Windows.
const openNonblock = 0

// openNofollow: Windows opens take no O_NOFOLLOW.
const openNofollow = 0

// The DWORD values SetStdHandle takes for STD_OUTPUT_HANDLE (-11) and
// STD_ERROR_HANDLE (-12): constant expressions, checked by the compiler.
const (
	stdOutputHandle = uint32(1<<32 + syscall.STD_OUTPUT_HANDLE)
	stdErrorHandle  = uint32(1<<32 + syscall.STD_ERROR_HANDLE)
)

func redirectStd(fd int) (orig, r *os.File, err error) {
	if err := setStdHandle.Find(); err != nil {
		return nil, nil, err
	}
	std, cur := stdOutputHandle, os.Stdout
	if fd == 2 {
		std, cur = stdErrorHandle, os.Stderr
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	fail := func(e error) (*os.File, *os.File, error) {
		_ = r.Close()
		_ = w.Close()
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
		_ = syscall.CloseHandle(h)
		if err := squat(dup, syscall.Handle(w.Fd()), h); err != nil {
			_ = syscall.CloseHandle(o)
			return fail(err)
		}
		taken[h], squatted[fd] = o, h
	}
	if ok, _, e := setStdHandle.Call(uintptr(std), w.Fd()); ok == 0 {
		return fail(e)
	}
	replaced = append(replaced, cur)
	if fd == 1 {
		os.Stdout = w
	} else {
		os.Stderr = w
	}
	return os.NewFile(uintptr(o), "original"), r, nil
}

// quietRuntime keeps the runtime from making handles while Start takes over the
// standard handles' numbers, and returns the function that lets it go on. Any
// handle made in that moment can land on a number just closed and stay there for
// good, which ends the program (redirectStd): the handles of a thread the runtime
// is starting (Thread, IRTimer, WaitCompletionPacket, Event), and the I/O
// completion port its first timer creates. With one P no new thread is started
// for the work at hand. The sleep is the first timer, made here, and it gives a
// thread already on its way time to finish. On a real connector on native Windows
// this took refused starts from 118 of 34615 to 0 of 33537.
//
// Not with GODEBUG set at all: more than one setting turns async preemption off,
// and changing GOMAXPROCS stops the world, which a goroutine in a tight loop would
// then never let finish. So there Start only sleeps, and a rare start is refused
// rather than hanging.
func quietRuntime() (resume func()) {
	if os.Getenv("GODEBUG") != "" {
		time.Sleep(time.Millisecond)
		return func() {}
	}
	procs := runtime.GOMAXPROCS(1)
	time.Sleep(time.Millisecond)
	return func() { runtime.GOMAXPROCS(procs) }
}

// retargetLog points the standard logger at the new stderr. Start calls it once
// the pumps are checking output: a logger a package's init set up can be blocked
// in a write to a full pipe, holding the lock SetOutput needs, until a pump
// drains that pipe, and a pump holds what it reads until Start lets it check.
func retargetLog() { log.SetOutput(os.Stderr) }

// pending is unknown on Windows: a synchronous pipe serializes its I/O, so
// asking (PeekNamedPipe) would wait behind the read it asks about.
func pending(*os.File) int { return -1 }

// squat duplicates src until the duplicate's handle number is want, which
// was just closed, then closes the misses. The number usually comes straight
// back, but a thread that moved to another processor since the close draws
// other free numbers first, so the walk has to be able to outlast them: on
// native Windows it took up to about one try per number below want, and
// squatTries allows for that. A number another thread now holds never comes
// back, and the walk ends in the error, which stops the program.
func squat(dup func(syscall.Handle) (syscall.Handle, error), src, want syscall.Handle) error {
	var misses []syscall.Handle
	defer func() {
		for _, m := range misses {
			_ = syscall.CloseHandle(m)
		}
	}()
	for range squatTries(want) {
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

// squatTries bounds the walk: more than every number below want (handle
// numbers are multiples of 4), up to a limit that keeps a walk to a fraction of
// a second and at a modest number of handles.
func squatTries(want syscall.Handle) int {
	return min(4096+int(want/4), 1<<16)
}

// releaseStd closes the pipe's write ends (the new stream and the handle
// number taken over) so the pump sees the end of the stream.
func releaseStd(fd int) {
	if h, ok := squatted[fd]; ok {
		_ = syscall.CloseHandle(h)
		delete(squatted, fd)
	}
	if fd == 1 {
		_ = os.Stdout.Close()
	} else {
		_ = os.Stderr.Close()
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
