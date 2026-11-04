// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fakeBase keeps these fake numbers clear of every real handle in the test
// process: squat closes its misses, and closing a real one would end the test.
const fakeBase = syscall.Handle(0x40000000)

// fakeHandles hands out numbers in rising order, as a thread that moved to
// another processor since the close sees them.
func fakeHandles() func(syscall.Handle) (syscall.Handle, error) {
	next := fakeBase
	return func(syscall.Handle) (syscall.Handle, error) {
		d := next
		next += 4
		return d, nil
	}
}

// The native Windows failure, as a thread that changed processor between the
// close and the first duplicate saw it (credkit-windows, 2026-11-04): numbers
// from 0x18c up and the wanted one 0x81c, reached on try 421 and refused after
// 256. Here the same distances, on numbers no real handle uses.
func TestSquatOutlastsTheNumbersAMovedThreadDrawsFirst(t *testing.T) {
	for _, tries := range []syscall.Handle{1, 256, 257, 421, 700, 4000} {
		if err := squat(fakeHandles(), 0, fakeBase+(tries-1)*4); err != nil {
			t.Errorf("reached on try %d: %v", tries, err)
		}
	}
}

// Every number below want can be walked, up to the limit.
func TestSquatTriesCoverEveryNumberBelowWant(t *testing.T) {
	for _, want := range []syscall.Handle{4, 0x468, 0x81c, 0xa80, 0x4000, 0x20000} {
		if n := syscall.Handle(squatTries(want)); n <= want/4 {
			t.Errorf("want %#x: %d tries cannot pass the %d numbers below it", want, n, want/4)
		}
	}
	if n := squatTries(syscall.Handle(1 << 40)); n != 1<<16 {
		t.Errorf("the limit is %d, want %d", n, 1<<16)
	}
}

// A number another thread took never comes back: the walk ends and refuses.
func TestSquatRefusesANumberThatNeverComesBack(t *testing.T) {
	tries := 0
	dup := func(syscall.Handle) (syscall.Handle, error) {
		tries++
		return fakeBase + syscall.Handle(tries*4), nil
	}
	const want = syscall.Handle(0x468)
	if err := squat(dup, 0, want); err == nil {
		t.Fatal("squat succeeded on a number that was never handed out")
	}
	if tries != squatTries(want) {
		t.Errorf("%d tries, want %d", tries, squatTries(want))
	}
}

// An error from the duplicate ends the walk and is returned.
func TestSquatReturnsADuplicateError(t *testing.T) {
	boom := errors.New("boom")
	dup := func(syscall.Handle) (syscall.Handle, error) { return 0, boom }
	if err := squat(dup, 0, 0x468); !errors.Is(err, boom) {
		t.Errorf("err %v, want %v", err, boom)
	}
}

// The walk leaves no handle behind. A closed number can be taken at once by a
// thread the runtime starts, so a few still open are not counted against it.
func TestSquatClosesItsMisses(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	p, err := syscall.GetCurrentProcess()
	if err != nil {
		t.Fatal(err)
	}
	var got []syscall.Handle
	stop := errors.New("enough")
	dup := func(h syscall.Handle) (syscall.Handle, error) {
		if len(got) == 500 {
			return 0, stop
		}
		var d syscall.Handle
		err := syscall.DuplicateHandle(p, h, p, &d, 0, false, syscall.DUPLICATE_SAME_ACCESS)
		got = append(got, d)
		return d, err
	}
	if err := squat(dup, syscall.Handle(w.Fd()), fakeBase); !errors.Is(err, stop) {
		t.Fatalf("squat: %v, want the walk to end at the duplicate's error", err)
	}
	open := 0
	for _, h := range got {
		var d syscall.Handle
		if err := syscall.DuplicateHandle(p, h, p, &d, 0, false, syscall.DUPLICATE_SAME_ACCESS); err == nil {
			_ = syscall.CloseHandle(d)
			open++
		}
	}
	if open > 16 {
		t.Errorf("%d of %d handles are still open", open, len(got))
	}
}

// The same failure on the real handle table: the second processor has 2000 free
// numbers of its own, and a number closed on the first comes back to it only
// after those, against 256 tries before the fix. Each attempt is a new process,
// since the free numbers a walk leaves make the next walk longer, as they cannot
// in a program at its first statement. An attempt can lose its number to a thread
// the runtime starts, so one success in five is the pass.
func TestSquatFindsANumberClosedOnAnotherProcessor(t *testing.T) {
	if runtime.NumCPU() < 2 {
		t.Skip("needs two processors")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for range 5 {
		cmd := exec.Command(exe, "-test.run=^$")
		cmd.Env = append(os.Environ(), childModeEnv+"="+squatMovedMode)
		last = cmd.Run()
		var ee *exec.ExitError
		if last == nil {
			return
		}
		if errors.As(last, &ee) && ee.ExitCode() == squatMovedSkip {
			t.Skip("cannot choose this thread's processor")
		}
	}
	t.Errorf("after five tries: %v", last)
}

const (
	squatMovedMode = "squat-moved"
	squatMovedSkip = 3
	quietStartMode = "quiet-start"
)

func init() {
	switch os.Getenv(childModeEnv) {
	case squatMovedMode:
		os.Exit(squatMoved())
	case quietStartMode:
		quietStart()
	}
}

// quietStart is the child of TestTakeoverRunsWithOneP. With QUIET_SPIN set, a
// goroutine that never yields is running when Start is called.
func quietStart() {
	var ready atomic.Bool
	if os.Getenv("QUIET_SPIN") != "" {
		go func() {
			for !ready.Load() {
			}
		}()
		time.Sleep(50 * time.Millisecond)
	}
	before := runtime.GOMAXPROCS(0)
	during := map[int]int{}
	redirectHook = func(fd int) { during[fd] = runtime.GOMAXPROCS(0) }
	Start(Declarations{})
	ready.Store(true)
	fmt.Printf("before=%d during1=%d during2=%d after=%d\n", before, during[1], during[2], runtime.GOMAXPROCS(0))
	Exit(0)
}

func quietStartOutput(t *testing.T, env ...string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^$")
	cmd.Env = append(append(os.Environ(), childModeEnv+"="+quietStartMode, "GOMAXPROCS=2"), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	return string(out)
}

// The runtime starts no new thread for the takeover, and has its processors back
// afterward.
func TestTakeoverRunsWithOneP(t *testing.T) {
	if got, want := quietStartOutput(t, "GODEBUG="), "before=2 during1=1 during2=1 after=2\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Stopping the world would never finish with this goroutine and no async
// preemption, so Start leaves GOMAXPROCS alone under any GODEBUG and does not hang.
func TestTakeoverDoesNotHangWithoutAsyncPreemption(t *testing.T) {
	for _, debug := range []string{"asyncpreemptoff=1", "asyncpreemptoff=01", "asyncpreemptoff=2", "gccheckmark=1"} {
		got := quietStartOutput(t, "GODEBUG="+debug, "QUIET_SPIN=1")
		if want := "before=2 during1=2 during2=2 after=2\n"; got != want {
			t.Errorf("GODEBUG=%s: got %q, want %q", debug, got, want)
		}
	}
}

// squatMoved is the child of TestSquatFindsANumberClosedOnAnotherProcessor.
func squatMoved() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setAffinity := kernel32.NewProc("SetThreadAffinityMask")
	currentThread := kernel32.NewProc("GetCurrentThread")
	runtime.LockOSThread()
	self, _, _ := currentThread.Call()
	pin := func(mask uintptr) bool {
		prev, _, _ := setAffinity.Call(self, mask)
		return prev != 0
	}
	r, w, err := os.Pipe()
	if err != nil {
		return 1
	}
	defer r.Close()
	defer w.Close()
	p, err := syscall.GetCurrentProcess()
	if err != nil {
		return 1
	}
	src := syscall.Handle(w.Fd())
	dup := func(h syscall.Handle) (syscall.Handle, error) {
		var d syscall.Handle
		err := syscall.DuplicateHandle(p, h, p, &d, 0, false, syscall.DUPLICATE_SAME_ACCESS)
		return d, err
	}
	if !pin(2) {
		return squatMovedSkip
	}
	hs := make([]syscall.Handle, 2000)
	for i := range hs {
		if hs[i], err = dup(src); err != nil {
			return 1
		}
	}
	for _, h := range hs {
		_ = syscall.CloseHandle(h)
	}
	if !pin(1) {
		return squatMovedSkip
	}
	want, err := dup(src)
	if err != nil {
		return 1
	}
	_ = syscall.CloseHandle(want)
	if !pin(2) {
		return squatMovedSkip
	}
	if squat(dup, src, want) != nil {
		return 1
	}
	return 0
}
