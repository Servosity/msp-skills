// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contracts 4 and 5: every file the program writes (journal, feedback,
// export, teach logs, profiles, config writes and caches). One file write is
// one unit, checked as the whole record.
//
//   - WriteFile writes a whole file. A withheld record is not written: the
//     file keeps what it held before (so a withheld write never destroys the
//     user's data) and the error is the notice.
//   - A Writer (Create, OpenFile, AppendFile, NewWriter) checks each Write
//     call as one record, with state kept across writes, and writes the
//     notice in place of a withheld record.
//
// A config write is checked like every other file write: contract 5 gives a
// credential file no exemption.
//
// The file sink fails closed by construction (kit8):
//
//  1. Every kit write to a file holds the file's path lock (the cleaned
//     absolute path) and then the file's own lock (its identity, so two
//     names of one file share it) across the open, the truncate, the check,
//     the write and the stream update.
//  2. A write lands at the offset it was checked for: WriteAt there, or an
//     append at the end it checked. The handle's position is read under the
//     locks, and a write before or past the end is checked as the whole file
//     it leaves (a gap as zero bytes).
//  3. Any error, short write, failed read-back or change the kit did not
//     make leaves the file's stream unknown (fileState.fail): every later
//     write is withheld, the notice included, until the file is truncated
//     or recreated under the locks, or found empty.
//  4. The context keeps a JSON document open in any reading, or the stream
//     is lost (window.add), and a notice is checked with that context.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// Test hooks: writeFileHook runs between WriteFile's write and its stream's
// update, openHook after OpenFile's open, placeHook after a write's offset
// is read, fileFault (when it returns an error) fails the named step.
var (
	writeFileHook, openHook, placeHook func()
	fileFault                          func(step string) error
)

// faultAt is err, or the test fault for step.
func faultAt(step string, err error) error {
	if err == nil && fileFault != nil {
		return fileFault(step)
	}
	return err
}

// errChanged: the file is not as the kit's last write left it.
var errChanged = errors.New("credkit: the file changed outside the kit; its writes are withheld until it is truncated")

// pathLocks is one lock per path a kit call writes, by the cleaned
// absolute path with symbolic links resolved.
// ponytail: one entry per path for the life of the process.
var pathLocks struct {
	sync.Mutex
	m map[string]*sync.Mutex
}

// fileKey is name cleaned and absolute, its directory's (or its own)
// symbolic links resolved.
func fileKey(name string) string {
	abs, err := filepath.Abs(name)
	if err != nil {
		return filepath.Clean(name)
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(r, filepath.Base(abs))
	}
	return abs
}

func pathLock(key string) *sync.Mutex {
	pathLocks.Lock()
	defer pathLocks.Unlock()
	if pathLocks.m == nil {
		pathLocks.m = map[string]*sync.Mutex{}
	}
	l := pathLocks.m[key]
	if l == nil {
		l = new(sync.Mutex)
		pathLocks.m[key] = l
	}
	return l
}

// fileState is one file's stream, by file (os.SameFile): every kit call
// that writes the file (AppendFile again and again, two Writers, a whole
// write and then appends, two names of it) continues it (contract 2, 3
// Streams and 5). A file written by an earlier run is not context
// (contract 6, item 5).
type fileState struct {
	mu      sync.Mutex
	fi      os.FileInfo
	ctx     window // the file's bytes before end, as far as they are context
	end     int64  // the file's size after the kit's last write to it
	unknown bool   // rule 3: every write is withheld until a truncate under the locks
}

// fileStates is every file the kit writes in this process.
// ponytail: scanned in turn; a program that writes thousands of files
// wants an inode-keyed map.
var fileStates struct {
	sync.Mutex
	list []*fileState
}

func stateOf(fi os.FileInfo) *fileState {
	fileStates.Lock()
	defer fileStates.Unlock()
	for _, fs := range fileStates.list {
		if os.SameFile(fs.fi, fi) {
			return fs
		}
	}
	fs := &fileState{fi: fi, end: fi.Size()}
	fileStates.list = append(fileStates.list, fs)
	return fs
}

// restart starts the stream over at the file's end, size: after a
// truncate, or for a file the kit first writes. It is the only way out of
// unknown.
func (fs *fileState) restart(size int64) {
	fs.ctx, fs.end, fs.unknown = window{}, size, false
}

// fail marks the stream unknown (rule 3) and returns err.
func (fs *fileState) fail(err error) (int, error) {
	fs.unknown = true
	return 0, err
}

// WriteFile is os.WriteFile with data checked as one record. The file's
// stream starts over with data (a later append continues it).
func WriteFile(name string, data []byte, perm os.FileMode) error {
	m, unknown := currentMatcher()
	if unknown {
		return withheldError{SinkFile}
	}
	v, open := m.checkOpen(data)
	if v != clean {
		return withheldError{SinkFile}
	}
	pl := pathLock(fileKey(name))
	pl.Lock()
	defer pl.Unlock()
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE, perm)
	if err != nil {
		return err // nothing opened, nothing truncated
	}
	defer f.Close()
	fs, err := lockedState(f)
	if err != nil {
		return err // not truncated: a stream it cannot name is not touched
	}
	defer fs.mu.Unlock()
	if err := faultAt("truncate", f.Truncate(0)); err != nil {
		_, err = fs.fail(err)
		return err
	}
	fs.restart(0)
	// The held set as of the write itself (kit15rv: a value held after
	// the first check, before the write, was written unchecked). A
	// withheld record leaves the file empty, as truncated.
	e := emitBegin() // to the end of the write call (held.go heldMu)
	if m2, unknown := currentMatcher(); unknown {
		emitEnd(e)
		return withheldError{SinkFile}
	} else if m2 != m {
		if v, open = m2.checkOpen(data); v != clean {
			emitEnd(e)
			return withheldError{SinkFile}
		}
	}
	n, err := f.Write(data)
	emitEnd(e)
	if err = faultAt("write", err); err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = faultAt("close", f.Close())
	}
	if err != nil {
		_, err = fs.fail(err)
		return err
	}
	if writeFileHook != nil {
		writeFileHook()
	}
	fs.end = int64(len(data))
	fs.ctx.add(data, open)
	fs.unknown = fs.ctx.lost
	return nil
}

// lockedState is f's stream, locked.
func lockedState(f *os.File) (*fileState, error) {
	fi, err := f.Stat()
	if err = faultAt("open-stat", err); err != nil {
		return nil, err
	}
	fs := stateOf(fi)
	fs.mu.Lock()
	return fs, nil
}

// AppendFile appends one record to name, creating it with perm.
func AppendFile(name string, record []byte, perm os.FileMode) error {
	w, err := OpenFile(name, os.O_WRONLY|os.O_APPEND|os.O_CREATE, perm)
	if err != nil {
		return err
	}
	if _, err := w.Write(record); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// Create is os.Create behind a Writer.
func Create(name string) (*Writer, error) {
	return OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// OpenFile is os.OpenFile behind a Writer. O_TRUNC truncates under the
// file's lock, with its stream's restart.
func OpenFile(name string, flag int, perm os.FileMode) (*Writer, error) {
	key := fileKey(name)
	pl := pathLock(key)
	pl.Lock()
	defer pl.Unlock()
	_, statErr := os.Lstat(name)
	f, err := os.OpenFile(name, flag&^os.O_TRUNC, perm)
	if err != nil {
		return nil, err // nothing opened, nothing truncated
	}
	if openHook != nil {
		openHook()
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err // not truncated
	}
	if !fi.Mode().IsRegular() {
		mu, st := streamOfFile(fi, streamID(streamGOOS, key, nil))
		return &Writer{w: f, sink: SinkFile, mu: mu, st: st}, nil // a pipe or a device: a stream
	}
	fs, err := lockedState(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	defer fs.mu.Unlock()
	if flag&os.O_TRUNC != 0 {
		if err := faultAt("truncate", f.Truncate(0)); err != nil {
			fs.fail(err)
			f.Close()
			return nil, err
		}
		fs.restart(0)
	} else if os.IsNotExist(statErr) {
		fs.restart(fi.Size()) // created by this open: a stream of its own
	}
	return &Writer{w: f, sink: SinkFile, fs: fs, key: key, f: f, appends: flag&os.O_APPEND != 0, osAppend: flag&os.O_APPEND != 0}, nil
}

// fileStreams is the stream of every *os.File the kit writes as a stream
// (a pipe, a FIFO or a device), by file: every Writer of one file, every
// AppendFile to a FIFO, continues one stream (contract 3 Streams). A Writer
// over any other io.Writer is a stream of its own: wrap such a destination
// once.
// ponytail: scanned in turn, kept for the life of the process.
var fileStreams struct {
	sync.Mutex
	list []fileStream
}

type fileStream struct {
	fi os.FileInfo
	id string
	mu *sync.Mutex
	st *streamCtx
}

// streamGOOS is runtime.GOOS; a test sets it to drive the Windows rule.
var streamGOOS = runtime.GOOS

// streamID names a pipe or a device where os.SameFile cannot: on Windows it
// has no file identity (its FileInfo has zero ids, so os.SameFile reports
// every pipe the same file). There it is known by the path it was opened
// by, else by its handle. Elsewhere it is "" and os.SameFile decides.
func streamID(goos, path string, f *os.File) string {
	switch {
	case goos != "windows":
		return ""
	case path != "":
		return "path:" + path
	}
	return fmt.Sprintf("handle:%d", f.Fd())
}

// streamOfFile is the stream of the file fi names (id from streamID).
//
// Blunt rule (Windows, class file-stream-state-reset, its fifth round): a
// pipe or a device there has no identity Go can read, so two handles of one
// pipe (DuplicateHandle, a second open by name) cannot be told from two
// pipes, and no context kept per handle or shared by all is complete. The
// kit follows one such stream: the first handle it wraps. Every other
// Windows pipe or device stream, and every one opened by path (each open a
// new connection), is unknown: every write to it is withheld (contract 3
// Budget, fail closed). Ordinary output does not use them: stdout and
// stderr go through the stdio layer, files are regular files.
func streamOfFile(fi os.FileInfo, id string) (*sync.Mutex, *streamCtx) {
	fileStreams.Lock()
	defer fileStreams.Unlock()
	if strings.HasPrefix(id, "path:") {
		return new(sync.Mutex), &streamCtx{lost: true}
	}
	for _, x := range fileStreams.list {
		if x.id == id && (id != "" || os.SameFile(x.fi, fi)) {
			return x.mu, x.st
		}
	}
	if id != "" && slices.ContainsFunc(fileStreams.list, func(x fileStream) bool { return x.id != "" }) {
		return new(sync.Mutex), &streamCtx{lost: true}
	}
	x := fileStream{fi: fi, id: id, mu: new(sync.Mutex), st: &streamCtx{}}
	fileStreams.list = append(fileStreams.list, x)
	return x.mu, x.st
}

// Writer checks each Write as one unit, with the tail of what was already
// written as context (for a file, by every kit call that writes it), and
// writes the sink's notice in place of a withheld unit (nothing at all
// when the notice itself would match). It has no ReadFrom or WriteString
// of its own to go around it.
type Writer struct {
	w        io.Writer
	sink     string
	mu       *sync.Mutex // a stream's (st), shared with every Writer of its file
	st       *streamCtx  // a stream that is not a regular file
	fs       *fileState  // a regular file's stream, shared by every kit call that writes the file
	key      string      // the file's path lock
	f        *os.File    // the regular file
	appends  bool        // every write lands at the end (O_APPEND), as of the open
	osAppend bool        // the os.File was opened for append: it refuses WriteAt
}

// NewWriter wraps w, whose units are reported as sink. An *os.File, under
// any sink, continues that file's stream.
func NewWriter(w io.Writer, sink string) *Writer {
	f, ok := w.(*os.File)
	if !ok {
		return &Writer{w: w, sink: sink, mu: new(sync.Mutex), st: &streamCtx{}}
	}
	fi, err := f.Stat()
	if err != nil {
		return &Writer{w: w, sink: sink, mu: new(sync.Mutex), st: &streamCtx{lost: true}} // a file the kit cannot identify: unknown
	}
	if !fi.Mode().IsRegular() {
		mu, st := streamOfFile(fi, streamID(streamGOOS, "", f))
		return &Writer{w: w, sink: sink, mu: mu, st: st}
	}
	key := fileKey(f.Name())
	return &Writer{w: w, sink: sink, fs: stateOf(fi), key: key, f: f, appends: appendMode(f), osAppend: osAppends(f)}
}

// appendMode reports whether every write to f lands at its end: opened
// with O_APPEND here (os.File refuses WriteAt), or by the descriptor's flag.
func appendMode(f *os.File) bool { return osAppends(f) || fdAppends(f) }

// osAppends reports whether os opened f with O_APPEND (it refuses WriteAt).
func osAppends(f *os.File) bool {
	_, err := f.WriteAt(nil, 0)
	return err != nil && strings.Contains(err.Error(), "O_APPEND")
}

// Write writes p, or the notice in its place. A write that does not land
// at the end of a regular file (a seek before the end, or past it, where
// the file system fills the gap with zero bytes) joins the bytes already
// there, so it is checked as the whole file it leaves (contract 2, 3
// Streams and 5). The notice is checked like the write it replaces, with
// the same context, and is dropped when it would match (contract 4).
func (w *Writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.fs == nil {
		return w.writeStream(p)
	}
	pl := pathLock(w.key)
	pl.Lock()
	defer pl.Unlock()
	fs := w.fs
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.unknown {
		return 0, withheldError{w.sink}
	}
	fi, err := w.f.Stat()
	if err = faultAt("stat", err); err != nil {
		return fs.fail(err)
	}
	size := fi.Size()
	if size != fs.end {
		if size != 0 {
			return fs.fail(errChanged)
		}
		fs.restart(0) // found empty: nothing in it to join
	}
	// Where a write lands is read again for each write (kit15rv: a
	// program can clear or set the descriptor's O_APPEND after the open).
	// An os.File opened for append refuses WriteAt; if its descriptor no
	// longer appends, a write lands at an offset the kit cannot pin, so the
	// stream fails closed.
	appends := w.appends
	if fdFlagReadable {
		app, known := fdAppendFlag(w.f)
		if !known || w.osAppend && !app {
			return fs.fail(errChanged)
		}
		appends = app
	}
	off := size
	if !appends {
		off, err = w.f.Seek(0, io.SeekCurrent)
		if err = faultAt("seek", err); err != nil {
			return fs.fail(err)
		}
	}
	if placeHook != nil {
		placeHook()
	}
	defer emitEnd(emitBegin()) // to the end of the write call (held.go heldMu)
	m, unknown := currentMatcher()
	if off == size {
		return w.atEnd(m, unknown, p, size, appends)
	}
	return w.placed(m, unknown, p, off, size, appends)
}

// atEnd writes p, or its notice, at the file's end, with the file's
// context.
func (w *Writer) atEnd(m *matcher, unknown bool, p []byte, size int64, appends bool) (int, error) {
	fs := w.fs
	out, open := checkWith(m, unknown, &fs.ctx, p, w.sink)
	if err := w.put(out, size, size, appends); err != nil {
		return fs.fail(err)
	}
	if len(out) == 0 {
		return 0, withheldError{w.sink} // withheld, and its notice with it: nothing landed
	}
	fs.ctx.add(out, open)
	fs.end = size + int64(len(out))
	fs.unknown = fs.ctx.lost
	return len(p), nil
}

// checkWith is what a sink with context c gets for unit p: p, its notice,
// or nothing; and whether a document is then open at the end of c.
func checkWith(m *matcher, unknown bool, c *window, p []byte, sink string) ([]byte, bool) {
	if !unknown && !c.lost && !c.runLost && (len(c.b) == 0 || m.check(p) == clean) {
		if v, open := m.checkOpen(append(slices.Clip(c.b), p...)); v == clean {
			return p, open
		}
	}
	n := noticeFor(sink, p)
	if n == nil || unknown || c.lost || c.runLost {
		return nil, c.open
	}
	if v, open := m.checkOpen(append(slices.Clip(c.b), n...)); v == clean {
		return n, open
	}
	return nil, c.open
}

// placed writes p, or its notice, at off, before or past the end of the
// file: checked as the whole file the write leaves.
// ponytail: the whole file is read for each such write; a program that
// patches a large file in place many times wants a windowed read.
func (w *Writer) placed(m *matcher, unknown bool, p []byte, off, size int64, appends bool) (int, error) {
	fs := w.fs
	b, err := readBack(w.f, size)
	if err = faultAt("read", err); err != nil {
		return fs.fail(err)
	}
	pa := &placed{b: b, off: off}
	// Blunt (kit15rv round 2, file-placement-race): a descriptor that
	// appends when the write runs lands it at the end, whatever off (Linux
	// pwrite), so the write is checked at the end too.
	end := &placed{b: b, off: size}
	out, open := []byte(nil), false
	if !unknown && m.check(p) == clean && m.check(end.with(p)) == clean {
		if v, o := m.checkOpen(pa.with(p)); v == clean {
			out, open = p, o
		}
	}
	if out == nil && !unknown {
		if n := noticeFor(w.sink, p); n != nil && m.check(end.with(n)) == clean {
			if v, o := m.checkOpen(pa.with(n)); v == clean {
				out, open = n, o
			}
		}
	}
	if err := w.put(out, off, size, appends); err != nil {
		return fs.fail(err)
	}
	if out == nil {
		return 0, withheldError{w.sink} // withheld, and its notice with it: nothing landed
	}
	after := pa.with(out)
	fs.ctx = window{}
	fs.ctx.add(after, open)
	fs.end = int64(len(after))
	fs.unknown = fs.ctx.lost
	return len(p), nil
}

// put writes out at off of a file whose size is size, and proves it
// landed there: the whole of it, and the file the size that leaves.
func (w *Writer) put(out []byte, off, size int64, appends bool) error {
	if len(out) == 0 {
		return nil
	}
	var n int
	var err error
	if appends {
		// At size by offset, not by the descriptor's flag, which the
		// program can clear while the write runs (kit15rv round 2).
		if n, err = appendAt(w.f, out, size); err == nil {
			_, err = w.f.Seek(size+int64(n), io.SeekStart)
		}
	} else if n, err = w.f.WriteAt(out, off); err == nil {
		_, err = w.f.Seek(off+int64(n), io.SeekStart) // where a write leaves the handle
	}
	if err = faultAt("write", err); err == nil && n != len(out) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	fi, err := w.f.Stat()
	if err = faultAt("verify", err); err != nil {
		return err
	}
	if fi.Size() != max(size, off+int64(len(out))) {
		return errChanged
	}
	return nil
}

// writeStream is Write for a stream that is not a regular file.
func (w *Writer) writeStream(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer emitEnd(emitBegin()) // to the end of the write call (held.go heldMu)
	if w.st.lost {
		return 0, withheldError{w.sink}
	}
	out, held := w.st.decide(p, w.sink)
	if len(out) > 0 {
		n, err := w.w.Write(out)
		if err == nil && n != len(out) {
			err = io.ErrShortWrite
		}
		if err != nil {
			w.st.lost = true // rule 3: what the sink holds is unknown
			return 0, err
		}
	}
	w.st.done(p, out, held)
	if len(out) == 0 {
		return 0, withheldError{w.sink} // withheld, and its notice with it
	}
	return len(p), nil
}

// placed is a regular file's bytes and where the next write lands in them.
type placed struct {
	b   []byte
	off int64
}

// with is the file as a write of q leaves it.
func (pa *placed) with(q []byte) []byte {
	n := int64(len(pa.b))
	after := slices.Clip(pa.b[:min(pa.off, n)])
	if pa.off > n {
		after = append(after, make([]byte, pa.off-n)...) // the gap reads as zero bytes
	}
	after = append(after, q...)
	if end := pa.off + int64(len(q)); end < n {
		after = append(after, pa.b[end:]...)
	}
	return after
}

// readBack is f's content, size bytes: read through f, or, when f was
// opened for writing only, through the same file opened again by name.
func readBack(f *os.File, size int64) ([]byte, error) {
	b := make([]byte, size)
	n, err := f.ReadAt(b, 0)
	if err == nil || err == io.EOF && int64(n) == size {
		return b[:n], nil
	}
	r, err := os.Open(f.Name())
	if err != nil {
		return nil, err
	}
	defer r.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if ri, err := r.Stat(); err != nil || !os.SameFile(fi, ri) {
		return nil, os.ErrInvalid
	}
	if n, err = r.ReadAt(b, 0); int64(n) != size {
		return nil, errChanged
	}
	return b, nil
}

// Close closes the wrapped writer when it is an io.Closer.
func (w *Writer) Close() error {
	if c, ok := w.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// window is the end of what a sink holds, as the next write's check needs
// it: the last bytes, reaching back to the start of every run still open
// and, while a JSON document is open in any reading, to its head.
type window struct {
	b       []byte
	open    bool // a document is open where b ends (the check of b found one)
	cut     bool // bytes before b are gone
	runLost bool // a run still open began before b: withhold until it ends
	lost    bool // bytes an open document needs are gone: withhold for good
}

// add moves c past out, after which a check found a document open or not
// (contract 3, Streams and Budget). A document still open keeps every byte
// back to where c was last cut, which held no open document; past
// streamTailMax its head is dropped and the stream is lost.
func (c *window) add(out []byte, open bool) {
	full := append(slices.Clip(c.b), out...)
	start := max(0, len(full)-tailLen())
	if open {
		start = 0
	}
	rs := openRunStart(full)
	runLost := rs == 0 && c.cut // the run began before the context held
	start = min(start, rs)
	if len(full)-start > streamTailMax {
		start, runLost = len(full)-streamTailMax, true // limits.go, streamTail
		c.lost = c.lost || open
	}
	c.b = append([]byte(nil), full[start:]...)
	c.open = open
	c.cut = start > 0 || c.cut && start == 0
	c.runLost = runLost
}

// streamCtx is a stream's state between writes (stdout, stderr, the MCP
// stdio session and a Writer that is not a regular file): the stream's own
// context, and once a unit was withheld, the sink's (which holds a notice
// where the stream has the unit).
type streamCtx struct {
	tail  window
	sink  window
	split bool    // a unit was withheld: sink is the sink's context
	lost  bool    // rule 3: a write failed or context an open document needs is gone; every write is withheld
	pend  [2]bool // open, after the last unit checked clean: tail then sink
	oOpen bool    // open, after what decide gave the sink
}

func (st *streamCtx) sinkCtx() *window {
	if st.split {
		return &st.sink
	}
	return &st.tail
}

// withheld is the check for one write of a stream: the write alone, the
// write after the stream's context and after the sink's; and every write
// while a run or a document the context no longer holds may continue
// (contract 3's joined values, with the budget's rule for context that no
// longer fits: withhold, as for a match).
func (st *streamCtx) withheld(p []byte) bool {
	m, unknown := currentMatcher()
	if unknown || st.lost || st.tail.runLost || st.split && (st.sink.runLost || st.sink.lost) {
		return true
	}
	if len(st.tail.b) > 0 && m.check(p) != clean {
		return true
	}
	v, open := m.checkOpen(append(slices.Clip(st.tail.b), p...))
	if v != clean {
		return true
	}
	st.pend = [2]bool{open, open}
	if st.split {
		if v, st.pend[1] = m.checkOpen(append(slices.Clip(st.sink.b), p...)); v != clean {
			return true
		}
	}
	return false
}

// decide is what the sink gets for unit p: p, its notice, or nothing.
func (st *streamCtx) decide(p []byte, sink string) ([]byte, bool) {
	if !st.withheld(p) {
		st.oOpen = st.pend[1]
		return p, false
	}
	var n []byte
	n, st.oOpen = noticeIn(st, sink, p)
	return n, true
}

// done moves the stream past unit p, whose sink got out (decide's) whole.
func (st *streamCtx) done(p, out []byte, held bool) {
	if held && !st.split {
		st.sink, st.split = st.tail, true
		st.sink.b = slices.Clone(st.sink.b)
	}
	if st.split && len(out) > 0 {
		st.sink.add(out, st.oOpen)
	}
	if held {
		st.tail.add(p, false)
		// After a withheld unit the context is cut to its longest clean
		// suffix, so the next write is judged on what it adds.
		st.tail.b = cleanSuffix(st.tail.b)
		if m, unknown := currentMatcher(); !unknown {
			_, st.tail.open = m.checkOpen(st.tail.b)
		}
	} else {
		st.tail.add(p, st.pend[0])
	}
	st.lost = st.lost || st.tail.lost || st.sink.lost
}

// over moves the stream past a unit too large to check, whose sink got n
// (open: a document is then open there).
func (st *streamCtx) over(n []byte, open bool) {
	if !st.split {
		st.sink, st.split = st.tail, true
		st.sink.b = slices.Clone(st.sink.b)
	}
	if len(n) > 0 {
		st.sink.add(n, open)
	}
	st.tail = window{}
	st.lost = st.lost || st.sink.lost
}

// runSets are the bytes a run decoder's span can hold (contract 3,
// Candidates): base64 and base32 with the line breaks they ignore, hex with
// its prefixes and separators, each base85 alphabet (Ascii85 with the
// control bytes it ignores), and character-code lists.
var runSets = func() [][256]bool {
	var b64, hex, a85, z85, r85, codes [256]bool
	for c := range 256 {
		b64[c] = alphabets[0].val[c] >= 0 || c == '=' || alphabets[0].ignore[c]
		a85[c] = alphabets[3].in(byte(c)) || alphabets[3].ignore[c]
		z85[c] = alphabets[4].val[c] >= 0
		r85[c] = alphabets[5].val[c] >= 0
		h := c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
		hex[c] = h || strings.IndexByte(" ,:-xX\\", byte(c)) >= 0
		codes[c] = h || strings.IndexByte(" ,[]xX", byte(c)) >= 0
	}
	return [][256]bool{b64, hex, a85, z85, r85, codes}
}()

// openRunStart is where the earliest run still open at the end of b
// starts: a run of one decoder's span bytes, or a stretch of binary, any
// of which the next write may continue; len(b) when none is open.
func openRunStart(b []byte) int {
	start := len(b)
	for i := range runSets {
		set := &runSets[i]
		j := len(b)
		for j > 0 && set[b[j-1]] {
			j--
		}
		start = min(start, j)
	}
	// Binary: a compressed stream can carry any bytes, printable ones
	// included, so a run of printable bytes is no proof it ended (the
	// 2026-10 zlib-across-1200-spaces leak). The open binary run reaches
	// back to the first byte of b that is not text: until a stream is
	// decompressed and shown to end (withheld's job, on the whole
	// context), any later binary byte may continue it. window.add caps
	// this at streamTailMax and sets runLost when the cap drops a byte of
	// it, so a binary stretch longer than the context withholds rather
	// than splits. Pure text (ordinary output) has no such byte and keeps
	// none.
	for j := 0; j < len(b); {
		r, n := utf8.DecodeRune(b[j:])
		if r == utf8.RuneError && n == 1 || r < 0x20 && !isSpaceByte(byte(r)) && r != 0x1b || r == 0x7f {
			start = min(start, j)
			break
		}
		j += n
	}
	return start
}

// cleanSuffix is the longest suffix of b that does not match, found by
// binary search on where it starts (nil when the held set is unknown).
func cleanSuffix(b []byte) []byte {
	m, unknown := currentMatcher()
	if unknown {
		return nil
	}
	lo, hi := 0, len(b) // b[hi:] is clean; the answer is in [lo, hi]
	for lo < hi {
		mid := (lo + hi) / 2
		if m.check(b[mid:]) == clean {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return slices.Clone(b[hi:])
}

// tailLen is the stream context: sixteen times the longest held value, so
// an encoded value split across writes is still whole in the context, from
// 1 KiB to 16 KiB.
func tailLen() int {
	m, _ := currentMatcher()
	n := streamTailMin
	if m != nil {
		n = max(n, streamTailPer*m.maxLen)
	}
	return min(n, streamTailMax)
}

// noticeFor is the notice for a withheld unit: ending in a newline when the
// unit did; nil when it would match on its own.
func noticeFor(sink string, unit []byte) []byte {
	n := []byte(Notice(sink))
	if len(unit) > 0 && unit[len(unit)-1] == '\n' {
		n = append(n, '\n')
	}
	if withheld(n) {
		return nil
	}
	return n
}

// noticeIn is the replacement for a withheld unit of a stream, and whether
// a document is then open in the sink. It is nil when the notice would
// match on its own or after what the stream and the sink hold, or when the
// context is not whole (st lost or inside a run it no longer holds): the
// unit is then dropped (contract 4).
func noticeIn(st *streamCtx, sink string, unit []byte) ([]byte, bool) {
	sc := st.sinkCtx()
	n := noticeFor(sink, unit)
	m, unknown := currentMatcher()
	if n == nil || unknown || st.lost || sc.lost || sc.runLost || st.tail.runLost {
		return nil, sc.open
	}
	v, open := m.checkOpen(append(slices.Clip(sc.b), n...))
	if v != clean || st.split && m.check(append(slices.Clip(st.tail.b), n...)) != clean {
		return nil, sc.open
	}
	return n, open
}
