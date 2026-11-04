// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contracts 4 and 5: stdout and stderr at the file-descriptor level. Start
// points the process's standard output and error at pipes before anything
// is written (dup2 on Unix, SetStdHandle on Windows), so every writer
// (fmt, log, a panic trace, a child process that inherits them) goes
// through the kit. A pump per stream reads the pipe, cuts it into units at
// pauses in the output, checks each unit with the tail of the stream as
// context, and writes it, or its notice, to the original stream. A unit is
// never cut by size: no byte of it is released before all of it is checked,
// and a unit larger than the matcher reads (maxUnit) is dropped as it
// arrives and withheld whole, with one notice, at the pause that ends it.
//
// A pause is the writer's, never the pump's: the pipe stayed empty for
// unitIdle while the reader waited on it (on Unix, FIONREAD confirms no
// byte is waiting). A reader or checker that falls behind (descheduled, or
// checking a large unit) never cuts a write, because a write in progress
// keeps the pipe full. What no pipe reports is a writer descheduled for
// unitIdle in the middle of one write after the reader drained it; that
// cut is the platform's residual, and the tail carries the first part into
// the check of the second. On Windows a synchronous pipe cannot be asked
// while a read waits on it, so there the wait alone is the pause. If any of
// that cannot be set up, the program exits before writing anything.

import (
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// ExitOutputLost is the exit status of a program that would have reported
// success when part of its standard output did not reach its reader. A
// command whose result was dropped has not succeeded.
const ExitOutputLost = 74

const (
	unitIdle = 10 * time.Millisecond // a pause this long ends a unit
	exitPoll = 50 * time.Millisecond
)

// exitWait is how long Exit waits for a stream that is not being checked
// (drainPumps). A variable, so a test can lower it.
var exitWait = 10 * time.Second

type pump struct {
	fd   int // 1 or 2
	sink string
	in   *os.File
	out  *os.File // the original stream
	tty  bool
	done chan struct{}
	st   streamCtx
	dead bool // the original stream refused a write: drain, write nothing
	// lost: bytes of this stream never reached its reader, and no notice
	// said so (the stream refused them, or a withheld unit's notice was
	// dropped too).
	lost     atomic.Bool
	writing  atomic.Bool // inside a write to the original stream
	checking atomic.Bool // inside the check of a unit
}

// pumpReadHook, when set (by a test), runs after each chunk is read and queued.
var pumpReadHook func()

var stdio struct {
	sync.Mutex
	pumps []*pump
}

// installStdio redirects fd 1 and 2 into pumps.
func installStdio() error {
	stdio.Lock()
	defer stdio.Unlock()
	if len(stdio.pumps) > 0 {
		return nil
	}
	for _, s := range []struct {
		fd   int
		sink string
		f    *os.File
	}{{1, SinkStdout, os.Stdout}, {2, SinkStderr, os.Stderr}} {
		tty := false
		if fi, err := s.f.Stat(); err == nil {
			tty = fi.Mode()&os.ModeCharDevice != 0
		}
		orig, r, err := redirectStd(s.fd)
		if err != nil {
			return err
		}
		p := &pump{fd: s.fd, sink: s.sink, in: r, out: orig, tty: tty, done: make(chan struct{})}
		stdio.pumps = append(stdio.pumps, p)
		go p.run()
	}
	return nil
}

func (p *pump) run() {
	defer close(p.done)
	// chunks carries the pipe's bytes in order, and nil where the writer
	// paused: the reader waited unitIdle on an empty pipe.
	chunks := make(chan []byte, 16)
	var mu sync.Mutex
	var since time.Time // when the read in progress began; zero between reads
	closed := false
	pause := time.AfterFunc(time.Hour, func() {
		mu.Lock()
		defer mu.Unlock()
		// Under mu the reader is still inside the read, so the pause is
		// queued before any byte that read returns.
		if !closed && !since.IsZero() && time.Since(since) >= unitIdle && pending(p.in) <= 0 {
			since = time.Time{} // one pause per wait
			chunks <- nil
		}
	})
	pause.Stop()
	go func() {
		buf := make([]byte, 64<<10)
		for {
			mu.Lock()
			since = time.Now()
			mu.Unlock()
			pause.Reset(unitIdle)
			n, err := p.in.Read(buf)
			pause.Stop()
			mu.Lock()
			since = time.Time{}
			if n > 0 {
				chunks <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				closed = true
				close(chunks)
			}
			mu.Unlock()
			if pumpReadHook != nil {
				pumpReadHook()
			}
			if err != nil {
				return
			}
		}
	}()
	var unit []byte
	over := false // the unit outgrew maxUnit: its bytes are dropped as they arrive
	var last byte
	end := func() {
		if over {
			p.flushOver(last)
		} else {
			p.flush(unit)
		}
		unit, over = nil, false
	}
	for c := range chunks {
		if c == nil {
			end()
			continue
		}
		last = c[len(c)-1]
		if over || len(unit)+len(c) > maxUnit {
			unit, over = nil, true
		} else {
			unit = append(unit, c...)
		}
	}
	end()
}

// flushOver withholds a unit too large to check: its notice, and no tail.
func (p *pump) flushOver(last byte) {
	if p.dead {
		return // lost when it went dead
	}
	<-started
	e := emitBegin() // the barrier, to the end of the write (held.go heldMu)
	n, open := noticeIn(&p.st, p.sink, []byte{last})
	p.write(n)
	emitEnd(e)
	p.st.over(n, open)
}

// write passes out on, and a unit that leaves nothing behind (out empty: its
// notice was dropped too) is lost output.
func (p *pump) write(out []byte) {
	if len(out) == 0 {
		p.lost.Store(true)
		return
	}
	p.writing.Store(true)
	_, err := p.out.Write(out)
	p.writing.Store(false)
	if err != nil {
		// ponytail: a closed reader (| head) no longer ends the program
		// by SIGPIPE; the pump drains and drops the rest, and Exit reports
		// it as the shell would have seen it: not a success.
		p.dead = true
		p.lost.Store(true)
	}
}

// flush checks one unit and writes it, or its notice.
func (p *pump) flush(unit []byte) {
	if len(unit) == 0 || p.dead {
		return
	}
	<-started        // every startup credential is held before any unit is checked
	e := emitBegin() // the barrier, to the end of the write (held.go heldMu)
	p.checking.Store(true)
	out, held := p.st.decide(unit, p.sink)
	p.checking.Store(false)
	p.write(out)
	emitEnd(e)
	p.st.done(unit, out, held)
}

// Exit ends the program with code after every byte already written to
// stdout and stderr has been checked and passed on. A program whose output
// layer is installed ends through Exit, never by returning from main or by
// os.Exit, which would drop output still in the pipes. A program about to
// report success whose stdout did not all arrive exits ExitOutputLost
// instead. A withheld unit whose notice arrived did arrive: the reader was
// told, and the program's own status stands.
func Exit(code int) {
	records.Wait()
	stdio.Lock()
	pumps := stdio.pumps
	for _, p := range pumps {
		releaseStd(p.fd)
	}
	stdio.Unlock()
	drainPumps(pumps)
	if code == 0 && mcpLost.Load() {
		code = ExitOutputLost
	}
	for _, p := range pumps {
		if p.lostOutput() && p.fd == 1 && code == 0 {
			code = ExitOutputLost
		}
	}
	os.Exit(code)
}

// drainPumps waits for the pumps to pass on what they have. It waits as long
// as a check is running, since that output is on its way and a program that
// left then would lose it, and exitWait for anything else: a reader that takes
// nothing, or a child that kept the stream open and may never close it.
func drainPumps(pumps []*pump) {
	t := time.NewTicker(exitPoll)
	defer t.Stop()
	var idle time.Duration
	for _, p := range pumps {
		for waiting := true; waiting; {
			select {
			case <-p.done:
				waiting = false
			case <-t.C:
				if slices.ContainsFunc(pumps, func(q *pump) bool { return q.checking.Load() }) {
					idle = 0
				} else if idle += exitPoll; idle >= exitWait {
					return
				}
			}
		}
	}
}

// lostOutput reports whether bytes of the stream did not reach its reader, as
// of drainPumps: a pump still inside a write has a reader that took nothing,
// and one that is only waiting has passed on what it could.
func (p *pump) lostOutput() bool { return p.lost.Load() || p.writing.Load() }

// Terminal reports whether f (os.Stdout or os.Stderr) was a terminal before
// the output layer replaced it with a pipe. Use it wherever the program
// asks whether its output is a terminal.
func Terminal(f *os.File) bool {
	stdio.Lock()
	for _, p := range stdio.pumps {
		if f == os.Stdout && p.fd == 1 || f == os.Stderr && p.fd == 2 {
			stdio.Unlock()
			return p.tty
		}
	}
	stdio.Unlock()
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// originalStdout is the stream stdout pointed at before the output layer
// was installed (MCP frames go straight to it, each checked as one MCP
// message).
func originalStdout() *os.File {
	stdio.Lock()
	defer stdio.Unlock()
	for _, p := range stdio.pumps {
		if p.fd == 1 {
			return p.out
		}
	}
	return os.Stdout
}
