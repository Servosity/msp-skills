//go:build ignore

// corpus_ktrace runs one command and every process it starts under ptrace and
// records, at the system-call boundary, every call that can put bytes or a
// name in a file (the leak list's kernel record, ruling 14). Linux only
// (amd64, arm64); class_secret_corpus.py builds it with `go build` on this
// file and runs each corpus unit's connector process under it.
//
//	corpus_ktrace <record fd> <command> [args...]
//
// Records go to the record descriptor (a pipe the list reads), each an 8-byte
// big-endian length, the writer's 8-byte pid (its thread group), a kind byte
// and the unit:
//
//	0 start   the root process's pid, in decimal
//	2 write   "dev:ino" NUL name NUL the bytes one call put in a regular file
//	3 fail    why the run is not recorded whole (the call named is not made)
//	4 name    a name put in the file system, a symbolic link's target, or an
//	          extended attribute's name or value
//	6 image   "dev:ino" NUL name NUL the whole bytes of a shared mapping
//	5 end     the root's exit status, written after every traced process ended
//	7 exec    the program a process now runs (its /proc exe link), after each exec
//	8 args    the starter's pid, a newline, and the argv and environment an exec
//	          gave a program, newline-joined, unless the process only passed on
//	          what it was given
//
// The set of calls is closed with a failing default: a call outside the
// architecture's own table (another ABI, or a number above the newest this
// file knows), and every call that can change a file's bytes in a way no
// record here reads (io_uring, process_vm_writev, ptrace, a writable shared
// mapping of a file other than SQLite's -shm index, collapse and insert
// ranges, reflink and dedupe ioctls, ...), is a fail record and the process
// is killed before the call runs. A record that cannot be read (memory, a
// descriptor's file) is a fail record too.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	kStart = 0
	kWrite = 2
	kFail  = 3
	kName  = 4
	kEnd   = 5
	kImage = 6
	kExec  = 7
	kArgs  = 8

	maxGap  = 64<<20 + 1 // the zeros a file extension materializes, capped where the kit cannot read whole
	maxPath = 4096
	maxRead = 1 << 30
)

// The calls this record reads, by name; the numbers come from the
// architecture's table below.
const (
	cWrite = iota + 1
	cPwrite
	cWritev
	cPwritev
	cPwritev2
	cSendfile
	cSplice
	cTee
	cVmsplice
	cCopyFileRange
	cFallocate
	cTruncate
	cFtruncate
	cOpen
	cOpenat
	cOpenat2
	cCreat
	cMkdir
	cMkdirat
	cMknod
	cMknodat
	cRename
	cRenameat
	cRenameat2
	cLink
	cLinkat
	cSymlink
	cSymlinkat
	cSetxattr
	cLsetxattr
	cFsetxattr
	cSetxattrat
	cBind
	cMmap
	cMunmap
	cMsync
	cMprotect
	cExecve
	cExecveat
	cIoctl
	cPrctl
	cClone
	cClone3
	cDenied // a call no record reads: the process is stopped before it runs
)

var (
	table   map[uint64]int
	names   map[uint64]string
	nativeA uint32 // the AUDIT_ARCH of this architecture's own system-call ABI
	maxNr   uint64 // the newest call number this file knows; above it fails closed
)

func init() {
	var t map[string]uint64
	var denied map[string]uint64
	switch runtime.GOARCH {
	case "amd64":
		nativeA = 0xC000003E
		t = map[string]uint64{"write": 1, "pwrite64": 18, "writev": 20, "pwritev": 296, "pwritev2": 328,
			"sendfile": 40, "splice": 275, "tee": 276, "vmsplice": 278, "copy_file_range": 326, "fallocate": 285,
			"truncate": 76, "ftruncate": 77, "open": 2, "openat": 257, "openat2": 437, "creat": 85, "mkdir": 83,
			"mkdirat": 258, "mknod": 133, "mknodat": 259, "rename": 82, "renameat": 264, "renameat2": 316,
			"link": 86, "linkat": 265, "symlink": 88, "symlinkat": 266, "setxattr": 188, "lsetxattr": 189,
			"fsetxattr": 190, "setxattrat": 463, "bind": 49, "mmap": 9, "munmap": 11, "msync": 26,
			"mprotect": 10, "pkey_mprotect": 329,
			"execve": 59, "execveat": 322, "ioctl": 16, "prctl": 157, "clone": 56,
			"clone3": 435}
		denied = map[string]uint64{"io_uring_setup": 425, "io_uring_enter": 426, "io_uring_register": 427,
			"process_vm_writev": 311, "ptrace": 101, "seccomp": 317, "bpf": 321, "userfaultfd": 323,
			"pidfd_getfd": 438, "mq_open": 240, "msgget": 68, "shmget": 29, "add_key": 248, "request_key": 249,
			"keyctl": 250, "remap_file_pages": 216, "uselib": 134, "lsm_set_self_attr": 460,
			"io_setup": 206, "io_submit": 209}
	case "arm64":
		nativeA = 0xC00000B7
		t = map[string]uint64{"write": 64, "pwrite64": 68, "writev": 66, "pwritev": 70, "pwritev2": 287,
			"sendfile": 71, "splice": 76, "tee": 77, "vmsplice": 75, "copy_file_range": 285, "fallocate": 47,
			"truncate": 45, "ftruncate": 46, "openat": 56, "openat2": 437, "mkdirat": 34, "mknodat": 33,
			"renameat": 38, "renameat2": 276, "linkat": 37, "symlinkat": 36, "setxattr": 5, "lsetxattr": 6,
			"fsetxattr": 7, "setxattrat": 463, "bind": 200, "mmap": 222, "munmap": 215, "msync": 227,
			"mprotect": 226, "pkey_mprotect": 288, "execve": 221, "execveat": 281, "ioctl": 29, "prctl": 167, "clone": 220,
			"clone3": 435}
		denied = map[string]uint64{"io_uring_setup": 425, "io_uring_enter": 426, "io_uring_register": 427,
			"process_vm_writev": 271, "ptrace": 117, "seccomp": 277, "bpf": 280, "userfaultfd": 282,
			"pidfd_getfd": 438, "mq_open": 180, "msgget": 186, "shmget": 194, "add_key": 217, "request_key": 218,
			"keyctl": 219, "remap_file_pages": 234, "lsm_set_self_attr": 460,
			"io_setup": 0, "io_submit": 2}
	}
	// ponytail: the newest call this file knows; a kernel that adds one
	// fails it closed until the table above names it.
	maxNr = 462
	kind := map[string]int{"write": cWrite, "pwrite64": cPwrite, "writev": cWritev, "pwritev": cPwritev,
		"pwritev2": cPwritev2, "sendfile": cSendfile, "splice": cSplice, "tee": cTee, "vmsplice": cVmsplice,
		"copy_file_range": cCopyFileRange, "fallocate": cFallocate, "truncate": cTruncate, "ftruncate": cFtruncate,
		"open": cOpen, "openat": cOpenat, "openat2": cOpenat2, "creat": cCreat, "mkdir": cMkdir, "mkdirat": cMkdirat,
		"mknod": cMknod, "mknodat": cMknodat, "rename": cRename, "renameat": cRenameat, "renameat2": cRenameat2,
		"link": cLink, "linkat": cLinkat, "symlink": cSymlink, "symlinkat": cSymlinkat, "setxattr": cSetxattr,
		"lsetxattr": cLsetxattr, "fsetxattr": cFsetxattr, "setxattrat": cSetxattrat, "bind": cBind, "mmap": cMmap,
		"munmap": cMunmap, "msync": cMsync, "mprotect": cMprotect, "pkey_mprotect": cMprotect, "execve": cExecve, "execveat": cExecveat,
		"ioctl": cIoctl, "prctl": cPrctl, "clone": cClone, "clone3": cClone3}
	table, names = map[uint64]int{}, map[uint64]string{}
	for n, nr := range t {
		table[nr], names[nr] = kind[n], n
	}
	for n, nr := range denied {
		table[nr], names[nr] = cDenied, n
	}
}

// ---- the record ----------------------------------------------------------

var (
	recMu  sync.Mutex
	recFd  int
	recDev uint64
	recIno uint64
)

func emit(pid int, kind byte, unit []byte) {
	recMu.Lock()
	defer recMu.Unlock()
	var h [17]byte
	binary.BigEndian.PutUint64(h[0:], uint64(len(unit)))
	binary.BigEndian.PutUint64(h[8:], uint64(pid))
	h[16] = kind
	writeAll(append(h[:], unit...))
}

func writeAll(b []byte) {
	for len(b) > 0 {
		n, err := syscall.Write(recFd, b)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			os.Exit(98) // the record is cut: the list fails the run (no end record)
		}
		b = b[n:]
	}
}

// ---- tracees -------------------------------------------------------------

type thread struct {
	tgid  int
	nr    uint64
	args  [6]uint64
	pre   pre
	inSys bool
}

// What a call's entry saw, for its exit.
type pre struct {
	skip              bool
	fd                int
	ident             string // "dev:ino" of the destination file
	name              string
	reg               bool
	off               int64 // where the bytes go; -1 when appended
	size              int64 // the file's size before the call
	names             [][]byte
	isShm             bool
	creat             bool
	isExec            bool
	ppid              int
	execArgv, execEnv [][]byte
	shm               *os.File // the -shm file, opened through the process's descriptor
}

type mapping struct {
	ident, name string
	f           *os.File // the -shm itself: a shared mapping shows the file's bytes
	last        string   // the bytes last recorded, so an unchanged image is not repeated
}

var (
	threads  = map[int]*thread{}
	seen     = map[int]bool{}
	shms     = map[string]*mapping{} // every SQLite -shm mapped by any traced process, by file identity
	mems     = map[int]*os.File{}
	live     = map[int]bool{} // thread groups, for the timeout's kill
	liveMu   sync.Mutex
	rootPid  int
	rootCode = -1
)

func statusField(tid int, field string) (int, bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", tid))
	if err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, field) {
				if n, err := strconv.Atoi(strings.TrimSpace(l[len(field):])); err == nil {
					return n, true
				}
			}
		}
	}
	return 0, false
}

func tgidOf(tid int) int {
	if n, ok := statusField(tid, "Tgid:"); ok {
		return n
	}
	return tid
}

func th(tid int) *thread {
	t := threads[tid]
	if t == nil {
		t = &thread{tgid: tgidOf(tid)}
		threads[tid] = t
		liveMu.Lock()
		live[t.tgid] = true
		liveMu.Unlock()
	}
	return t
}

func fail(t *thread, tid int, why string) {
	emit(t.tgid, kFail, []byte(why))
	_ = syscall.Kill(t.tgid, syscall.SIGKILL)
	_ = syscall.Kill(tid, syscall.SIGKILL)
}

func mem(tgid int) (*os.File, error) {
	if f := mems[tgid]; f != nil {
		return f, nil
	}
	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", tgid))
	if err != nil {
		return nil, err
	}
	mems[tgid] = f
	return f, nil
}

func dropMem(tgid int) {
	if f := mems[tgid]; f != nil {
		f.Close()
		delete(mems, tgid)
	}
}

func readMem(tid int, addr, n uint64) ([]byte, error) {
	if n > maxRead {
		return nil, fmt.Errorf("a %d-byte read", n)
	}
	f, err := mem(tid)
	if err != nil {
		return nil, err
	}
	b := make([]byte, n)
	got, err := f.ReadAt(b, int64(addr))
	if uint64(got) != n {
		return nil, fmt.Errorf("read %d of %d bytes at %#x: %v", got, n, addr, err)
	}
	return b, nil
}

// readStr reads a NUL-terminated string, page by page so it never crosses
// into an unmapped page it does not need.
func readStr(tid int, addr uint64) ([]byte, error) {
	if addr == 0 {
		return nil, nil
	}
	var out []byte
	for len(out) <= maxPath {
		n := 4096 - addr%4096
		b, err := readMem(tid, addr, n)
		if err != nil {
			return nil, err
		}
		if i := bytes.IndexByte(b, 0); i >= 0 {
			return append(out, b[:i]...), nil
		}
		out = append(out, b...)
		addr += n
	}
	return nil, fmt.Errorf("a name longer than %d bytes", maxPath)
}

// fdFile: the identity, name, regular-ness, position, flags and size of a
// tracee's descriptor, read through /proc (the magic link to the file).
type fdFile struct {
	ident string
	name  string
	reg   bool
	pos   int64
	flags int
	size  int64
	dev   uint64
	ino   uint64
	mode  uint32
}

func fdOf(tid, fd int) (fdFile, error) {
	var st syscall.Stat_t
	p := fmt.Sprintf("/proc/%d/fd/%d", tid, fd)
	if err := syscall.Stat(p, &st); err != nil {
		return fdFile{}, err
	}
	name, _ := os.Readlink(p)
	f := fdFile{ident: fmt.Sprintf("%d:%d", st.Dev, st.Ino), name: name, reg: st.Mode&syscall.S_IFMT == syscall.S_IFREG,
		size: st.Size, dev: uint64(st.Dev), ino: st.Ino, mode: st.Mode}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/fdinfo/%d", tid, fd))
	if err != nil {
		return fdFile{}, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		k, v, _ := strings.Cut(l, ":")
		v = strings.TrimSpace(v)
		switch k {
		case "pos":
			f.pos, _ = strconv.ParseInt(v, 10, 64)
		case "flags":
			x, _ := strconv.ParseInt(v, 8, 64)
			f.flags = int(x)
		}
	}
	return f, nil
}

// given: the argv and environment a thread group's program received.
type given struct{ argv, env [][]byte }

var givens = map[int]*given{}

const maxArgs = 8 << 20 // more than execve accepts

// readVec reads a NULL-terminated array of strings (argv, envp).
func readVec(tid int, p uint64) ([][]byte, error) {
	var out [][]byte
	total := 0
	for p != 0 {
		b, err := readMem(tid, p, 8)
		if err != nil {
			return nil, err
		}
		q := binary.LittleEndian.Uint64(b)
		if q == 0 {
			break
		}
		var str []byte
		for {
			n := 4096 - q%4096
			c, err := readMem(tid, q, n)
			if err != nil {
				return nil, err
			}
			i := bytes.IndexByte(c, 0)
			if i >= 0 {
				str = append(str, c[:i]...)
				break
			}
			str = append(str, c...)
			q += n
			if len(str) > maxArgs {
				return nil, fmt.Errorf("a string longer than execve accepts")
			}
		}
		total += len(str)
		if total > maxArgs {
			return nil, fmt.Errorf("more than execve accepts")
		}
		out = append(out, str)
		p += 8
	}
	return out, nil
}

// passedOn: the environment unchanged and the arguments a tail of what the
// program was given.
func passedOn(g *given, argv, env [][]byte) bool {
	if len(argv) == 0 || len(env) != len(g.env) || len(argv)-1 > len(g.argv) {
		return false
	}
	for i := range env {
		if !bytes.Equal(env[i], g.env[i]) {
			return false
		}
	}
	tail := g.argv[len(g.argv)-(len(argv)-1):]
	for i, x := range argv[1:] {
		if !bytes.Equal(x, tail[i]) {
			return false
		}
	}
	return true
}

// noFd: the process has no such descriptor, so the call fails with EBADF
// and writes nothing. Any other error (a process that made itself not
// dumpable hides its descriptors) means the call cannot be read.
func noFd(err error) bool { return errors.Is(err, syscall.ENOENT) }

func isRec(f fdFile) bool { return f.dev == recDev && f.ino == recIno }

// ---- one call ------------------------------------------------------------

func fdArg(a uint64) int { return int(int32(a)) }

// The descriptors a write-family call writes to.
func outFds(c int, a [6]uint64) []int {
	switch c {
	case cWrite, cPwrite, cWritev, cPwritev, cPwritev2, cFtruncate, cFallocate, cVmsplice, cFsetxattr:
		return []int{fdArg(a[0])}
	case cSendfile:
		return []int{fdArg(a[0])}
	case cSplice:
		return []int{fdArg(a[2])}
	case cTee:
		return []int{fdArg(a[1])}
	case cCopyFileRange:
		return []int{fdArg(a[2])}
	}
	return nil
}

const (
	oCreat   = 0x40
	oTrunc   = 0x200
	oAppend  = 0x400
	oTmpfile = 0x410000
	oAccmode = 3
)

func gapOf(f fdFile, off int64) int64 {
	if off > f.size {
		return off - f.size
	}
	return 0
}

func zeros(t *thread, tid int, f fdFile, n int64) {
	if n <= 0 {
		return
	}
	if n > maxGap {
		n = maxGap
	}
	emit(t.tgid, kWrite, unit(f.ident, f.name, make([]byte, n)))
}

func unit(ident, name string, b []byte) []byte {
	return append([]byte(ident+"\x00"+name+"\x00"), b...)
}

// name records a path argument (the bytes the program chose).
func (t *thread) name(tid int, addr uint64) error {
	b, err := readStr(tid, addr)
	if err != nil {
		return err
	}
	t.pre.names = append(t.pre.names, b)
	return nil
}

func entry(tid int, t *thread) {
	t.pre = pre{}
	c, ok := table[t.nr]
	if !ok {
		return
	}
	a := t.args
	name := names[t.nr]
	// Every descriptor a call writes to: never the record itself.
	for _, fd := range outFds(c, a) {
		f, err := fdOf(tid, fd)
		if err != nil {
			if !noFd(err) {
				fail(t, tid, name+": its descriptor cannot be read: "+err.Error())
				return
			}
			t.pre.skip = true // a bad descriptor: the call fails with EBADF and writes nothing
			continue
		}
		if isRec(f) {
			fail(t, tid, name+" to the kernel record itself")
			return
		}
	}
	switch c {
	case cDenied:
		fail(t, tid, name+": a call no record reads")
	case cWrite, cWritev, cPwrite, cPwritev, cPwritev2, cSendfile, cSplice, cCopyFileRange, cFtruncate, cFallocate:
		var fd int
		switch c {
		case cSplice, cCopyFileRange:
			fd = fdArg(a[2])
		default:
			fd = fdArg(a[0])
		}
		f, err := fdOf(tid, fd)
		if err != nil && !noFd(err) {
			fail(t, tid, name+": its descriptor cannot be read: "+err.Error())
			return
		}
		if err != nil || !f.reg {
			t.pre.skip = true
			return
		}
		t.pre.fd, t.pre.ident, t.pre.name, t.pre.reg, t.pre.size = fd, f.ident, f.name, true, f.size
		t.pre.off = f.pos
		switch c {
		case cPwrite, cPwritev:
			t.pre.off = int64(a[3])
		case cPwritev2:
			if int64(a[3]) != -1 {
				t.pre.off = int64(a[3])
			}
			if a[5]&0x10 != 0 { // RWF_APPEND
				t.pre.off = -1
			}
		case cSplice, cCopyFileRange:
			if a[3] != 0 {
				b, err := readMem(tid, a[3], 8)
				if err != nil {
					fail(t, tid, name+": its offset cannot be read: "+err.Error())
					return
				}
				t.pre.off = int64(binary.LittleEndian.Uint64(b))
			}
		}
		if f.flags&oAppend != 0 {
			t.pre.off = -1 // O_APPEND: even pwrite appends (Linux)
		}
		switch c {
		case cFtruncate:
			zeros(t, tid, f, int64(a[1])-f.size)
		case cFallocate:
			mode := a[1]
			const keep, punch, collapse, zero, insert, unshare = 0x01, 0x02, 0x08, 0x10, 0x20, 0x40
			if mode&(collapse|insert) != 0 || mode&^(keep|punch|zero|unshare) != 0 {
				fail(t, tid, fmt.Sprintf("fallocate mode %#x: a file's bytes moved", mode))
				return
			}
			if mode&keep == 0 {
				zeros(t, tid, f, int64(a[2]+a[3])-f.size)
			}
		}
	case cTruncate:
		p, err := readStr(tid, a[0])
		if err != nil {
			fail(t, tid, "truncate: its path cannot be read: "+err.Error())
			return
		}
		path := string(p)
		if !filepath.IsAbs(path) {
			path = fmt.Sprintf("/proc/%d/cwd/%s", tid, path)
		}
		var st syscall.Stat_t
		if syscall.Stat(path, &st) == nil && st.Mode&syscall.S_IFMT == syscall.S_IFREG {
			zeros(t, tid, fdFile{ident: fmt.Sprintf("%d:%d", st.Dev, st.Ino), name: string(p), size: st.Size},
				int64(a[1])-st.Size)
		}
	case cOpen, cOpenat, cOpenat2, cCreat:
		// A name is put in the file system only by O_CREAT (creat always).
		at, flags := uint64(1), uint64(oCreat)
		switch c {
		case cOpen:
			at, flags = 0, a[1]
		case cCreat:
			at = 0
		case cOpenat:
			flags = a[2]
		case cOpenat2:
			how, err := readMem(tid, a[2], 8)
			if err != nil {
				fail(t, tid, name+": its flags cannot be read: "+err.Error())
				return
			}
			flags = binary.LittleEndian.Uint64(how)
		}
		t.pre.creat = flags&oCreat != 0 && flags&oTmpfile != oTmpfile
		if t.pre.creat {
			if err := t.name(tid, a[at]); err != nil {
				fail(t, tid, name+": its path cannot be read: "+err.Error())
			}
		}
	case cMkdir, cMknod:
		if err := t.name(tid, a[0]); err != nil {
			fail(t, tid, name+": its path cannot be read: "+err.Error())
		}
	case cMkdirat, cMknodat:
		if err := t.name(tid, a[1]); err != nil {
			fail(t, tid, name+": its path cannot be read: "+err.Error())
		}
	case cRename, cLink, cSymlink:
		if err := t.name(tid, a[0]); err != nil {
			fail(t, tid, name+": its paths cannot be read: "+err.Error())
		} else if err := t.name(tid, a[1]); err != nil {
			fail(t, tid, name+": its paths cannot be read: "+err.Error())
		}
	case cRenameat, cRenameat2, cLinkat:
		if err := t.name(tid, a[1]); err != nil {
			fail(t, tid, name+": its paths cannot be read: "+err.Error())
		} else if err := t.name(tid, a[3]); err != nil {
			fail(t, tid, name+": its paths cannot be read: "+err.Error())
		}
	case cSymlinkat:
		if err := t.name(tid, a[0]); err != nil {
			fail(t, tid, name+": its paths cannot be read: "+err.Error())
		} else if err := t.name(tid, a[2]); err != nil {
			fail(t, tid, name+": its paths cannot be read: "+err.Error())
		}
	case cSetxattr, cLsetxattr, cFsetxattr:
		const at = 1 // (path or fd, name, value, size, flags)
		if c != cFsetxattr {
			if err := t.name(tid, a[0]); err != nil {
				fail(t, tid, name+": its path cannot be read: "+err.Error())
				return
			}
		}
		if err := t.name(tid, a[at]); err != nil {
			fail(t, tid, name+": its name cannot be read: "+err.Error())
			return
		}
		v, err := readMem(tid, a[at+1], a[at+2])
		if err != nil {
			fail(t, tid, name+": its value cannot be read: "+err.Error())
			return
		}
		t.pre.names = append(t.pre.names, v)
	case cSetxattrat:
		// setxattrat(dfd, path, at_flags, name, struct xattr_args *uargs, usize)
		if err := t.name(tid, a[1]); err != nil {
			fail(t, tid, name+": its path cannot be read: "+err.Error())
			return
		}
		if err := t.name(tid, a[3]); err != nil {
			fail(t, tid, name+": its name cannot be read: "+err.Error())
			return
		}
		args, err := readMem(tid, a[4], 16) // value pointer, size, flags
		if err != nil {
			fail(t, tid, name+": its arguments cannot be read: "+err.Error())
			return
		}
		v, err := readMem(tid, binary.LittleEndian.Uint64(args[0:]), uint64(binary.LittleEndian.Uint32(args[8:])))
		if err != nil {
			fail(t, tid, name+": its value cannot be read: "+err.Error())
			return
		}
		t.pre.names = append(t.pre.names, v)
	case cBind:
		// A UNIX socket's path is a name in the file system.
		n := a[2]
		if n > 2 && n <= 110 {
			b, err := readMem(tid, a[1], n)
			if err != nil {
				fail(t, tid, "bind: its address cannot be read: "+err.Error())
				return
			}
			if binary.LittleEndian.Uint16(b) == syscall.AF_UNIX {
				p := b[2:]
				if i := bytes.IndexByte(p, 0); i >= 0 && p[0] != 0 {
					p = p[:i]
				}
				t.pre.names = append(t.pre.names, p)
			}
		}
	case cMmap:
		const mapShared, mapSharedValidate, mapAnon = 0x01, 0x03, 0x20
		flags := a[3]
		if flags&mapAnon != 0 || flags&mapSharedValidate == 0 || flags&mapSharedValidate == 0x02 {
			return // private or anonymous: never written back to a file
		}
		fd := fdArg(a[4])
		f, err := fdOf(tid, fd)
		if err != nil {
			if !noFd(err) {
				fail(t, tid, name+": its descriptor cannot be read: "+err.Error())
				return
			}
			t.pre.skip = true // EBADF: no mapping
			return
		}
		if isRec(f) {
			fail(t, tid, "mmap of the kernel record itself")
			return
		}
		if !f.reg || f.flags&oAccmode == 0 || a[2]&protWrite == 0 {
			// a device, a file opened read-only, or a mapping without write
			// access (SQLite's mmap of its database): no write reaches a file.
			// Adding write access later is mprotect's, refused below.
			return
		}
		if !strings.HasSuffix(f.name, "-shm") || !sqliteDB(f.name) {
			fail(t, tid, "mmap: a writable shared mapping of "+f.name+", a file write no record reads")
			return
		}
		sf, err := os.Open(fmt.Sprintf("/proc/%d/fd/%d", tid, fd))
		if err != nil {
			fail(t, tid, "mmap: the -shm "+f.name+" cannot be opened: "+err.Error())
			return
		}
		t.pre.isShm, t.pre.ident, t.pre.name, t.pre.shm = true, f.ident, f.name, sf
	case cMprotect:
		if a[2]&protWrite != 0 {
			if name := sharedFileIn(t.tgid, a[0], a[1]); name != "" {
				fail(t, tid, "mprotect: write access to a shared mapping of "+name+", a file write no record reads")
				return
			}
		}
	case cMunmap, cMsync:
		scanMaps(t.tgid)
	case cExecve, cExecveat:
		scanMaps(t.tgid)
		ap, ep := a[1], a[2]
		if c == cExecveat {
			ap, ep = a[2], a[3]
		}
		argv, err1 := readVec(tid, ap)
		env, err2 := readVec(tid, ep)
		if err1 != nil || err2 != nil {
			fail(t, tid, name+": its argv or environment cannot be read")
			return
		}
		t.pre.execArgv, t.pre.execEnv, t.pre.isExec = argv, env, true
		t.pre.ppid, _ = statusField(t.tgid, "PPid:")
	case cIoctl:
		f, err := fdOf(tid, fdArg(a[0]))
		if err != nil && !noFd(err) {
			fail(t, tid, name+": its descriptor cannot be read: "+err.Error())
			return
		}
		if err != nil || !f.reg {
			return
		}
		// On a regular file only requests that read: FICLONE, FICLONERANGE,
		// FIDEDUPERANGE and every other request that can move a file's bytes fail.
		req := a[1] & 0xffffffff
		switch {
		case req>>30 == 2: // _IOR: the kernel copies out, the file is read
		case req == 0x5401 || req == 0x5413 || req == 0x541B || req == 0x5421 || req == 0x5450 || req == 0x5451 ||
			req == 0x5452:
			// TCGETS, TIOCGWINSZ, FIONREAD, FIONBIO, FIONCLEX, FIOCLEX, FIOASYNC
		default:
			fail(t, tid, fmt.Sprintf("ioctl %#x on the regular file %s", a[1], f.name))
		}
	case cPrctl:
		if a[0] == 22 { // PR_SET_SECCOMP
			fail(t, tid, "prctl(PR_SET_SECCOMP): a call no record reads")
		}
	case cClone, cClone3:
		// CLONE_UNTRACED would start a process this record does not follow;
		// CLONE_PARENT would hide which process started it (kind 8's ledger).
		flags := a[0]
		if c == cClone3 {
			b, err := readMem(tid, a[0], 8)
			if err != nil {
				fail(t, tid, "clone3: its arguments cannot be read: "+err.Error())
				return
			}
			flags = binary.LittleEndian.Uint64(b)
		}
		if flags&0x00800000 != 0 {
			fail(t, tid, name+" with CLONE_UNTRACED: a process this record would not follow")
		}
		if flags&0x00008000 != 0 {
			fail(t, tid, name+" with CLONE_PARENT: a process whose starter this record would not know")
		}
	}
}

func exit(tid int, t *thread, rval int64) {
	c := table[t.nr]
	p := t.pre
	a := t.args
	for _, n := range p.names {
		emit(t.tgid, kName, n)
	}
	if p.isExec && rval == 0 {
		// A process that runs what it was given on (env to python, a shell
		// script's exec "$@") gives no process anything new; every other
		// exec is what a child receives (contract 2).
		if g := givens[t.tgid]; g == nil || !passedOn(g, p.execArgv, p.execEnv) {
			emit(t.tgid, kArgs, append([]byte(strconv.Itoa(p.ppid)+"\n"),
				bytes.Join(append(append([][]byte{}, p.execArgv...), p.execEnv...), []byte("\n"))...))
		}
		givens[t.tgid] = &given{p.execArgv, p.execEnv}
	}
	switch c {
	case cOpen, cOpenat, cOpenat2, cCreat:
		if rval < 0 {
			return
		}
		f, err := fdOf(tid, int(rval))
		if err != nil {
			fail(t, tid, "open: the descriptor it returned cannot be read: "+err.Error())
			return
		}
		if isRec(f) {
			fail(t, tid, "open of the kernel record itself")
			return
		}
		if p.creat {
			emit(t.tgid, kName, []byte(f.name))
		}
	}
	if p.skip || !p.reg || rval <= 0 {
		if c == cMmap && p.isShm && rval > 0 {
			if shms[p.ident] == nil {
				shms[p.ident] = &mapping{ident: p.ident, name: p.name, f: p.shm}
			} else {
				p.shm.Close()
			}
			scanMaps(t.tgid)
		} else if p.shm != nil {
			p.shm.Close()
		}
		return
	}
	n := uint64(rval)
	var data []byte
	var err error
	switch c {
	case cWrite, cPwrite:
		data, err = readMem(tid, a[1], n)
	case cWritev, cPwritev, cPwritev2:
		data, err = readIov(tid, a[1], a[2], n)
	case cSendfile, cSplice, cCopyFileRange:
		// The bytes came from another descriptor: read them back from the file.
		data, err = readBack(tid, p, n)
	default:
		return
	}
	if err == nil && c != cSendfile && c != cSplice && c != cCopyFileRange {
		// Another thread may change a buffer while it is written: the file
		// holds what was written, so the two must agree (else fail closed).
		var back []byte
		q := p
		if q.off < 0 {
			q.off = -1
			if f, e := fdOf(tid, p.fd); e == nil && f.ident == p.ident {
				q.off = f.size - int64(n) // an append ends at the file's end
			}
		}
		if back, err = readBack(tid, q, n); err == nil && !bytes.Equal(back, data) {
			err = fmt.Errorf("the file does not hold the bytes the process wrote (a buffer changed while it was written)")
		}
	}
	if err != nil {
		fail(t, tid, names[t.nr]+": the bytes it wrote cannot be read: "+err.Error())
		return
	}
	if p.off >= 0 {
		zeros(t, tid, fdFile{ident: p.ident, name: p.name}, gapOf(fdFile{size: p.size}, p.off))
	}
	emit(t.tgid, kWrite, unit(p.ident, p.name, data))
}

func readIov(tid int, iov, cnt, n uint64) ([]byte, error) {
	if cnt > 1024 {
		return nil, fmt.Errorf("%d iovecs", cnt)
	}
	v, err := readMem(tid, iov, cnt*16)
	if err != nil {
		return nil, err
	}
	var out []byte
	for i := uint64(0); i < cnt && uint64(len(out)) < n; i++ {
		base, ln := binary.LittleEndian.Uint64(v[i*16:]), binary.LittleEndian.Uint64(v[i*16+8:])
		if want := n - uint64(len(out)); ln > want {
			ln = want
		}
		if ln == 0 {
			continue
		}
		b, err := readMem(tid, base, ln)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	if uint64(len(out)) != n {
		return nil, fmt.Errorf("the iovecs hold %d of %d bytes", len(out), n)
	}
	return out, nil
}

func readBack(tid int, p pre, n uint64) ([]byte, error) {
	if p.off < 0 {
		return nil, fmt.Errorf("an appended copy")
	}
	var st syscall.Stat_t
	f, err := os.Open(fmt.Sprintf("/proc/%d/fd/%d", tid, p.fd))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil || fmt.Sprintf("%d:%d", st.Dev, st.Ino) != p.ident {
		return nil, fmt.Errorf("the file is not the one written")
	}
	b := make([]byte, n)
	if got, err := f.ReadAt(b, p.off); uint64(got) != n {
		return nil, fmt.Errorf("read %d of %d: %v", got, n, err)
	}
	return b, nil
}

// sqliteDB: the -shm's database (its path without -shm) starts with
// SQLite's magic.
func sqliteDB(name string) bool {
	b := make([]byte, 16)
	f, err := os.Open(strings.TrimSuffix(name, "-shm"))
	if err != nil {
		return false
	}
	defer f.Close()
	n, _ := f.ReadAt(b, 0)
	return n == 16 && string(b) == "SQLite format 3\x00"
}

const protWrite = 0x2

// sharedFileIn: the file of a shared file mapping overlapping [addr,
// addr+n) other than a recorded -shm ("" when none), from the kernel's own
// list of the process's mappings. Unreadable is treated as one.
func sharedFileIn(tgid int, addr, n uint64) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", tgid))
	if err != nil {
		return "(the process's mappings cannot be read)"
	}
	for _, ln := range strings.Split(string(b), "\n") {
		f := strings.Fields(ln)
		if len(f) < 5 || len(f[1]) < 4 || f[1][3] != 's' || f[4] == "0" {
			continue
		}
		lo, hi, ok := strings.Cut(f[0], "-")
		l, e1 := strconv.ParseUint(lo, 16, 64)
		h, e2 := strconv.ParseUint(hi, 16, 64)
		if !ok || e1 != nil || e2 != nil || !(addr < h && l < addr+n) {
			continue
		}
		name := strings.Join(f[5:], " ")
		shm := false
		for _, m := range shms {
			shm = shm || m.name == name
		}
		if !shm {
			return name
		}
	}
	return ""
}

// scanMaps records each SQLite -shm file whose bytes changed since the last
// record, whole, at every record point of any traced process: the mapping
// made, munmap, msync, exec and exit, and when the record ends. A shared
// mapping shows its file's bytes, so the file is read: any mapped range, by
// any process that inherited the mapping.
func scanMaps(tgid int) {
	for _, m := range shms {
		st, err := m.f.Stat()
		if err != nil || st.Size() > maxGap {
			emit(tgid, kFail, []byte("the -shm "+m.name+" cannot be read"))
			continue
		}
		b := make([]byte, st.Size())
		n, err := m.f.ReadAt(b, 0)
		if err != nil && err != io.EOF {
			emit(tgid, kFail, []byte("the -shm "+m.name+" cannot be read: "+err.Error()))
			continue
		}
		b = b[:n]
		if string(b) == m.last {
			continue
		}
		m.last = string(b)
		emit(tgid, kImage, unit(m.ident, m.name, b))
	}
}

// ---- the loop ------------------------------------------------------------

// struct ptrace_syscall_info, PTRACE_GET_SYSCALL_INFO (Linux 5.3).
type sysInfo struct {
	Op   uint8
	_    [3]uint8
	Arch uint32
	IP   uint64
	SP   uint64
	Nr   uint64 // entry: nr; exit: rval
	Args [6]uint64
}

func syscallInfo(tid int) (sysInfo, error) {
	var si sysInfo
	_, _, e := syscall.RawSyscall6(syscall.SYS_PTRACE, 0x420e, uintptr(tid), unsafe.Sizeof(si),
		uintptr(unsafe.Pointer(&si)), 0, 0)
	if e != 0 {
		return si, e
	}
	return si, nil
}

func eventMsg(tid int) uint64 {
	m, _ := syscall.PtraceGetEventMsg(tid)
	return uint64(m)
}

func main() {
	runtime.LockOSThread()
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: corpus_ktrace <record fd> <command> [args...]")
		os.Exit(2)
	}
	fd, err := strconv.Atoi(os.Args[1])
	if err != nil {
		os.Exit(2)
	}
	recFd = fd
	syscall.CloseOnExec(fd)
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		os.Exit(2)
	}
	recDev, recIno = uint64(st.Dev), st.Ino
	// Not dumpable: no traced process (same user, no CAP_SYS_PTRACE) can open
	// this recorder's /proc entries, its descriptors or memory.
	_, _, _ = syscall.RawSyscall(syscall.SYS_PRCTL, 4 /* PR_SET_DUMPABLE */, 0, 0)
	// No core file: a crash would write the process's memory to a file.
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
	_, _, _ = syscall.RawSyscall(syscall.SYS_PRCTL, 36 /* PR_SET_CHILD_SUBREAPER */, 1, 0)

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		emit(0, kEnd, []byte("127"))
		os.Exit(127)
	}
	rootPid = cmd.Process.Pid
	emit(rootPid, kStart, []byte(strconv.Itoa(rootPid)))

	// A timeout: every traced process is killed, the record still ends.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		liveMu.Lock()
		for p := range live {
			_ = syscall.Kill(p, syscall.SIGKILL)
		}
		liveMu.Unlock()
	}()

	var ws syscall.WaitStatus
	if _, err := syscall.Wait4(rootPid, &ws, syscall.WALL, nil); err != nil || !ws.Stopped() {
		emit(rootPid, kFail, []byte("the traced process did not stop at its start"))
		emit(rootPid, kEnd, []byte("127"))
		os.Exit(127)
	}
	const opts = syscall.PTRACE_O_TRACESYSGOOD | syscall.PTRACE_O_TRACEFORK | syscall.PTRACE_O_TRACEVFORK |
		syscall.PTRACE_O_TRACECLONE | syscall.PTRACE_O_TRACEEXEC | syscall.PTRACE_O_TRACEEXIT | 0x100000 /* EXITKILL */
	if err := syscall.PtraceSetOptions(rootPid, opts); err != nil {
		emit(rootPid, kFail, []byte("ptrace options: "+err.Error()))
		_ = syscall.Kill(rootPid, syscall.SIGKILL)
		emit(rootPid, kEnd, []byte("127"))
		os.Exit(127)
	}
	seen[rootPid] = true
	th(rootPid)
	// What the root was given is the harness's, not a child's.
	cl, err1 := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", rootPid))
	ev, err2 := os.ReadFile(fmt.Sprintf("/proc/%d/environ", rootPid))
	if err1 == nil && err2 == nil {
		givens[rootPid] = &given{nulSplit(cl), nulSplit(ev)}
	}
	_ = syscall.PtraceSyscall(rootPid, 0)

	for {
		tid, err := syscall.Wait4(-1, &ws, syscall.WALL, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			break // ECHILD: no traced process is left
		}
		if ws.Exited() || ws.Signaled() {
			if t := threads[tid]; t != nil && t.inSys && t.pre.reg && !t.pre.skip {
				emit(t.tgid, kFail, []byte(names[t.nr]+": the process ended inside the call, so its bytes are not read"))
			}
			dropMem(tid)
			delete(seen, tid)
			if t := threads[tid]; t != nil && t.tgid == tid {
				delete(givens, tid)
				liveMu.Lock()
				delete(live, tid)
				liveMu.Unlock()
			}
			delete(threads, tid)
			if tid == rootPid {
				if ws.Exited() {
					rootCode = ws.ExitStatus()
				} else {
					rootCode = 128 + int(ws.Signal())
				}
			}
			continue
		}
		if !ws.Stopped() {
			continue
		}
		t := th(tid)
		s := ws.StopSignal()
		switch {
		case !seen[tid]:
			seen[tid] = true // an auto-attached process's or thread's first stop
			if s == syscall.SIGSTOP {
				s = 0
			}
		case s == syscall.SIGTRAP|0x80:
			si, err := syscallInfo(tid)
			if err != nil {
				fail(t, tid, "the system call cannot be read: "+err.Error())
				s = 0
				break
			}
			switch si.Op {
			case 1: // entry
				t.nr, t.args, t.inSys = si.Nr, si.Args, true
				if si.Arch != nativeA {
					fail(t, tid, fmt.Sprintf("system call %d through another ABI (%#x)", si.Nr, si.Arch))
				} else if si.Nr > maxNr {
					fail(t, tid, fmt.Sprintf("system call %d, newer than this record's table", si.Nr))
				} else {
					entry(tid, t)
				}
			case 2: // exit
				if t.inSys {
					exit(tid, t, int64(si.Nr))
				}
				t.inSys = false
			default:
				fail(t, tid, fmt.Sprintf("an unknown system-call stop %d", si.Op))
			}
			s = 0
		case s == syscall.SIGTRAP && ws.TrapCause() > 0:
			switch ws.TrapCause() {
			case syscall.PTRACE_EVENT_FORK, syscall.PTRACE_EVENT_VFORK, syscall.PTRACE_EVENT_CLONE:
				_ = eventMsg(tid)
			case syscall.PTRACE_EVENT_EXEC:
				old := int(eventMsg(tid))
				if o := threads[old]; old != tid && o != nil {
					threads[tid], t = o, o // the thread that ran execve now has the group's id
					delete(threads, old)
				}
				dropMem(tid)
				dropMem(old)
				exe, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", tid))
				emit(t.tgid, kExec, []byte(exe))
				t.inSys = true // the execve exit stop follows
			case syscall.PTRACE_EVENT_EXIT:
				scanMaps(t.tgid)
			}
			s = 0
		default:
			// A signal for the process, or a group stop (no siginfo): delivered
			// as it was, a group stop continued.
			if _, err := ptraceSiginfo(tid); err != nil {
				s = 0
			}
		}
		_ = syscall.PtraceSyscall(tid, int(s))
	}
	scanMaps(rootPid)
	emit(rootPid, kEnd, []byte(strconv.Itoa(rootCode)))
	if rootCode < 0 {
		rootCode = 1
	}
	os.Exit(rootCode)
}

func nulSplit(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	return bytes.Split(bytes.TrimSuffix(b, []byte{0}), []byte{0})
}

func ptraceSiginfo(tid int) ([128]byte, error) {
	var si [128]byte
	_, _, e := syscall.RawSyscall6(syscall.SYS_PTRACE, syscall.PTRACE_GETSIGINFO, uintptr(tid), 0,
		uintptr(unsafe.Pointer(&si)), 0, 0)
	if e != 0 {
		return si, e
	}
	return si, nil
}
