//go:build unix

package credkit

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

// kit15rv Codex round 2, each probe as Codex wrote it unless noted.

// charset-context-pruning: a UTF-16 reading whose structure is written
// through two percent-encodings.
func TestKit15R2CharsetRecursiveSubstitution(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	unit := utf16LE("%257B\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}")
	var out bytes.Buffer
	NewWriter(&out, SinkFile).Write(unit)
	if bytes.Equal(out.Bytes(), unit) {
		t.Fatalf("a UTF-16 document behind two percent-encodings passed whole: %x", unit)
	}
}

// config-reader-drops-values: an INI key that begins "export ".
func TestKit15R2CanonicalINIExportKey(t *testing.T) {
	c := "[default]\nexport TOKEN=secret!!\n"
	gatherString(t, Declarations{ConfigFields: []string{"export TOKEN"}}, "c.ini", c)
	var out bytes.Buffer
	NewWriter(&out, SinkFile).Write([]byte("secret!!"))
	if bytes.Contains(out.Bytes(), []byte("secret!!")) {
		t.Fatalf("an INI credential under an export key passed; held %q", heldValues())
	}
}

// request-query-reading-mismatch: an Opaque URL carries its own query.
func TestKit15R2OpaqueQuery(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"token"}})
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.Query().Get("token")
		w.WriteHeader(204)
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.URL.Opaque = "/?token=secret%21%21"
	if resp, err := (&http.Client{Transport: Transport(&http.Transport{})}).Do(req); err == nil {
		resp.Body.Close()
	}
	var out bytes.Buffer
	NewWriter(&out, SinkFile).Write([]byte("secret!!"))
	if received == "secret!!" && bytes.Contains(out.Bytes(), []byte("secret!!")) {
		t.Fatalf("the query sent through an Opaque URL was not read; held %v", isHeld("secret!!"))
	}
}

// file-placement-race: O_APPEND cleared between the flag read and the write.
func TestKit15R2AppendFlagClearedDuringWrite(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := NewWriter(f, SinkFile)
	if _, err = w.Write([]byte("secrXXXX")); err != nil {
		t.Fatal(err)
	}
	f.Seek(4, 0)
	placeHook = func() { setFL(t, f, 0) }
	defer func() { placeHook = nil }()
	w.Write([]byte("et!!"))
	if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("secret!!")) {
		t.Fatalf("a write landed where the kit did not check it: %q", b)
	}
}

// file-placement-race, the other direction (not Codex's probe): O_APPEND set
// between the flag read and a positioned write, which an appending
// descriptor may move to the end (Linux pwrite). The write is checked at
// the end too, so it is withheld (its notice in its place) whatever the
// platform does.
func TestKit15R2AppendFlagSetDuringWrite(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := NewWriter(f, SinkFile)
	if _, err = w.Write([]byte("XXXXsecr")); err != nil {
		t.Fatal(err)
	}
	f.Seek(0, 0)
	placeHook = func() { setFL(t, f, syscall.O_APPEND) }
	defer func() { placeHook = nil }()
	_, err = w.Write([]byte("et!!"))
	b, _ := os.ReadFile(p)
	if bytes.Contains(b, []byte("et!!")) {
		t.Fatalf("a write that an appending descriptor lands at the end was not checked there: err=%v file=%q", err, b)
	}
}

func setFL(t *testing.T, f *os.File, fl int) {
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFL, uintptr(fl)); e != 0 {
		t.Fatal(e)
	}
}

// held-set-update-race: a value held while WriteFile's second check runs.
// Codex's schedule, except that Hold runs on its own goroutine: once a Hold
// has returned, the write must not carry the value; a Hold that waits for
// the write in flight is the other correct outcome.
func TestKit15R2HeldUpdateDuringRecheck(t *testing.T) {
	resetForTest(Declarations{})
	p := filepath.Join(t.TempDir(), "f")
	ready, release := make(chan struct{}), make(chan struct{})
	fileFault = func(step string) error {
		if step == "truncate" {
			Hold("minted:other", "unrelated-value")
			m, _ := currentMatcher()
			m.prepare()
			m.prep = sync.Once{}
			go m.prep.Do(func() { close(ready); <-release })
			<-ready
		}
		return nil
	}
	defer func() { fileFault = nil }()
	done := make(chan error, 1)
	go func() { done <- WriteFile(p, []byte("secret!!"), 0600) }()
	<-ready
	buf := make([]byte, 1<<16)
	until := time.Now().Add(3 * time.Second)
	for {
		n := runtime.Stack(buf, true)
		blocked := false
		for _, st := range bytes.Split(buf[:n], []byte("\n\n")) {
			if bytes.Contains(st, []byte("credkit.WriteFile")) && bytes.Contains(st, []byte("credkit.(*matcher).prepare")) {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		if time.Now().After(until) {
			close(release)
			<-done
			t.Fatal("did not reach the second matcher check")
		}
		runtime.Gosched()
	}
	held := make(chan struct{})
	go func() { Hold("minted:x", "secret!!"); close(held) }()
	returned := false
	select {
	case <-held:
		returned = true
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	err := <-done
	<-held
	if b, _ := os.ReadFile(p); returned && bytes.Contains(b, []byte("secret!!")) {
		t.Fatalf("a value whose Hold had returned was written: err=%v file=%q", err, b)
	}
}

// kit15rv Codex round 3, charset-context-pruning: the structure of a UTF-16
// document in compatibility characters (a fullwidth brace), and, not
// Codex's probe, every structure character fullwidth.
func TestKit15R3WideNormalizationJoin(t *testing.T) {
	for _, doc := range []string{
		"｛\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}",
		"｛＂あ＂：0，＂a＂：＂secr＂，＂b＂：＂et!!＂｝",
	} {
		resetForTest(Declarations{})
		Hold("minted:x", "secret!!")
		unit := utf16LE(doc)
		var out bytes.Buffer
		NewWriter(&out, SinkFile).Write(unit)
		if bytes.Equal(out.Bytes(), unit) {
			t.Errorf("a UTF-16 document whose structure is in compatibility characters passed whole: %s", doc)
		}
	}
}

type kit15R3PausedPipe struct {
	dst              io.Writer
	entered, release chan struct{}
}

func (w *kit15R3PausedPipe) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.dst.Write(p)
}

// kit15rv Codex round 3, held-set-update-race on a stream: a value held
// while a write that was checked without it is on its way. Codex's probe,
// except that Hold runs on its own goroutine: once it has returned, the
// stream must not carry the value; a Hold that waits for the write in
// flight is the other correct outcome.
func TestKit15R3NonregularHeldBarrier(t *testing.T) {
	resetForTest(Declarations{})
	r, p, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	gate := &kit15R3PausedPipe{dst: p, entered: make(chan struct{}), release: make(chan struct{})}
	w := NewWriter(gate, SinkFile)
	done := make(chan error, 1)
	go func() { _, e := w.Write([]byte("secret!!")); p.Close(); done <- e }()
	<-gate.entered
	held := make(chan struct{})
	go func() { Hold("minted:x", "secret!!"); close(held) }()
	returned := false
	select {
	case <-held:
		returned = true
	case <-time.After(200 * time.Millisecond):
	}
	close(gate.release)
	got, _ := io.ReadAll(r)
	<-done
	<-held
	if returned && bytes.Equal(got, []byte("secret!!")) {
		t.Fatalf("a value whose Hold had returned reached the stream: %q", got)
	}
}

// The same class on an outbound request (not Codex's probe): a value held
// while the request is being dialed, after RoundTrip judged it, is not sent.
func TestKit15R3RequestHeldWhileDialing(t *testing.T) {
	resetForTest(Declarations{})
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Note")
		w.WriteHeader(204)
	}))
	defer srv.Close()
	base := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		Hold("minted:x", "Note7Dial9x") // returns before any byte is written
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("X-Note", "Note7Dial9x")
	if resp, err := (&http.Client{Transport: Transport(base)}).Do(req); err == nil {
		resp.Body.Close()
	}
	if got == "Note7Dial9x" {
		t.Fatal("a value held after the request was judged, before it was written, was sent")
	}
}

// The same class (not Codex's probe): the hex digits of a percent-encoded
// brace written fullwidth after an ASCII percent sign.
func TestKit15R3WideNormalizedEscape(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	unit := utf16LE("%７Ｂ\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}")
	var out bytes.Buffer
	NewWriter(&out, SinkFile).Write(unit)
	if bytes.Equal(out.Bytes(), unit) {
		t.Fatal("a UTF-16 document behind a percent-encoding in fullwidth digits passed whole")
	}
}
