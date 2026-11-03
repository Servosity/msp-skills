// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build linux || darwin

package credkit

import (
	"os"
	"syscall"
	"unsafe"
)

// pending is how many bytes wait unread in the pipe f (FIONREAD), or -1.
func pending(f *os.File) int {
	c, err := f.SyscallConn()
	if err != nil {
		return -1
	}
	n := int32(-1)
	if c.Control(func(fd uintptr) {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, fionread, uintptr(unsafe.Pointer(&n))); e != 0 {
			n = -1
		}
	}) != nil {
		return -1
	}
	return int(n)
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
		syscall.Close(p[0])
		syscall.Close(p[1])
		return nil, nil, err
	}
	syscall.CloseOnExec(o)
	if err := dup2(p[1], fd); err != nil {
		syscall.Close(o)
		syscall.Close(p[0])
		syscall.Close(p[1])
		return nil, nil, err
	}
	syscall.Close(p[1])
	return os.NewFile(uintptr(o), "original"), os.NewFile(uintptr(p[0]), "credkit"), nil
}

// releaseStd points fd at the null device, closing its pipe so the pump
// sees the end of the stream.
func releaseStd(fd int) {
	n, err := syscall.Open(os.DevNull, syscall.O_WRONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		syscall.Close(fd)
		return
	}
	_ = dup2(n, fd)
	syscall.Close(n)
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

// appendAt writes b at off, f's end, by offset (pwrite): where it lands
// does not depend on the descriptor's O_APPEND, which may change while the
// write runs. An appending descriptor lands it at the end, off itself.
func appendAt(f *os.File, b []byte, off int64) (n int, err error) {
	c, err := f.SyscallConn()
	if err != nil {
		return 0, err
	}
	if cerr := c.Write(func(fd uintptr) bool { n, err = syscall.Pwrite(int(fd), b, off); return true }); cerr != nil {
		return 0, cerr
	}
	return max(n, 0), err
}
