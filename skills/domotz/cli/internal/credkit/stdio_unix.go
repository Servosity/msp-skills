// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build linux || darwin

package credkit

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openNonblock keeps an open of a credential file from blocking on a pipe
// (readRegular refuses it once open).
const openNonblock = syscall.O_NONBLOCK

// openNofollow: an open that must not follow a final symbolic link.
const openNofollow = syscall.O_NOFOLLOW

// pending is how many bytes wait unread in the pipe f (FIONREAD), or -1.
// The ioctl goes through golang.org/x/sys/unix, so the kit itself imports
// no unsafe.
func pending(f *os.File) int {
	c, err := f.SyscallConn()
	if err != nil {
		return -1
	}
	n := -1
	if c.Control(func(fd uintptr) {
		if v, err := unix.IoctlGetInt(int(fd), fionread); err == nil {
			n = v
		}
	}) != nil {
		return -1
	}
	return n
}

// redirectStd points fd at a new blocking pipe and returns the original
// stream and the pipe's read end. The pipe is created blocking on purpose:
// os.Stdout and os.Stderr were opened blocking, and a non-blocking pipe
// under them would fail a write with EAGAIN when the pump falls behind.
func redirectStd(fd int) (orig, r *os.File, err error) {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	var p [2]int
	if err := pipe(p[:]); err != nil {
		return nil, nil, err
	}
	o, err := syscall.Dup(fd)
	if err != nil {
		_ = syscall.Close(p[0])
		_ = syscall.Close(p[1])
		return nil, nil, err
	}
	syscall.CloseOnExec(o)
	if err := dup2(p[1], fd); err != nil {
		_ = syscall.Close(o)
		_ = syscall.Close(p[0])
		_ = syscall.Close(p[1])
		return nil, nil, err
	}
	_ = syscall.Close(p[1])
	return os.NewFile(uintptr(o), "original"), os.NewFile(uintptr(p[0]), "credkit"), nil
}

// releaseStd points fd at the null device, closing its pipe so the pump
// sees the end of the stream.
func releaseStd(fd int) {
	n, err := syscall.Open(os.DevNull, syscall.O_WRONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		_ = syscall.Close(fd)
		return
	}
	_ = dup2(n, fd)
	_ = syscall.Close(n)
}

// fdAppends reports whether f's descriptor was opened with O_APPEND.
func fdAppends(f *os.File) bool {
	app, _ := fdAppendFlag(f)
	return app
}

// fdFlagReadable: this platform reads a descriptor's O_APPEND, which a
// program can change after the open (fcntl F_SETFL).
const fdFlagReadable = true

// fdAppendFlag is the descriptor's O_APPEND now; known is false when it
// could not be read.
func fdAppendFlag(f *os.File) (app, known bool) {
	c, err := f.SyscallConn()
	if err != nil {
		return false, false
	}
	var fl uintptr
	var e syscall.Errno
	if c.Control(func(fd uintptr) { fl, _, e = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0) }) != nil || e != 0 {
		return false, false
	}
	return fl&syscall.O_APPEND != 0, true
}

// appendAt writes b at f's end. A descriptor that appends does it with a
// plain write, which the kernel lands at the end however far another process
// has moved it, where pwrite on macOS would overwrite what that process just
// appended. One that no longer appends (a program can clear the flag at any
// time: kit15rv round 2) writes at off, the end the kit read, by offset.
func appendAt(f *os.File, b []byte, off int64) (n int, err error) {
	if app, known := fdAppendFlag(f); app && known {
		return f.Write(b)
	}
	c, err := f.SyscallConn()
	if err != nil {
		return 0, err
	}
	if cerr := c.Write(func(fd uintptr) bool { n, err = syscall.Pwrite(int(fd), b, off); return true }); cerr != nil {
		return 0, cerr
	}
	return max(n, 0), err
}

// retargetLog: the standard logger writes to fd 2 or to nothing here, and the
// redirect does not move it.
func retargetLog() {}

// quietRuntime has nothing to do: dup2 replaces a descriptor in one step.
func quietRuntime() (resume func()) { return func() {} }
