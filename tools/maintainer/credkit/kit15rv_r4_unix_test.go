//go:build unix

// kit15rv Codex round 4, its probes as written (renamed).

package credkit

import (
	"bytes"
	"encoding/json"
	utf16enc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/unicode/norm"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestKit15R4CompatibilityExpansion(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	unit := utf16LE("&#⑫3;\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}")
	standard, e := utf16enc.UTF16(utf16enc.LittleEndian, utf16enc.IgnoreBOM).NewDecoder().Bytes(unit)
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		A string
		B string
	}
	if e = json.Unmarshal([]byte(html.UnescapeString(norm.NFKC.String(string(standard)))), &doc); e != nil || doc.A+doc.B != "secret!!" {
		t.Fatalf("standard reader: %v %+v", e, doc)
	}
	m, _ := currentMatcher()
	if m.check(unit) != clean {
		return
	}
	var out bytes.Buffer
	_, e = NewWriter(&out, SinkFile).Write(unit)
	if bytes.Equal(out.Bytes(), unit) {
		t.Fatalf("CLEAN accepted wide -> NFKC -> HTML reference -> JSON join: err=%v unit=%x", e, unit)
	}
}
func TestKit15R4CustomRoundTripperEpoch(t *testing.T) {
	resetForTest(Declarations{})
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.URL.Query().Get("q"); w.WriteHeader(204) }))
	defer srv.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return http.DefaultTransport.RoundTrip(r)
	})
	done := make(chan error, 1)
	go func() {
		resp, e := (&http.Client{Transport: Transport(rt)}).Get(srv.URL + "/?q=secret%21%21")
		if resp != nil {
			resp.Body.Close()
		}
		done <- e
	}()
	<-entered
	held := make(chan struct{})
	go func() { Hold("minted:x", "secret!!"); close(held) }()
	returned := false
	select {
	case <-held:
		returned = true
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	e := <-done
	<-held
	if returned && got == "secret!!" {
		t.Fatalf("request sent after Hold returned: err=%v query=%q", e, got)
	}
}
func TestKit15R4OversizeNoticeEpoch(t *testing.T) {
	resetForTest(Declarations{})
	startedOnce.Do(func() { close(started) })
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	defer w.Close()
	c, e := w.SyscallConn()
	if e != nil {
		t.Fatal(e)
	}
	filler := bytes.Repeat([]byte{'x'}, 512)
	filled := 0
	if e = c.Control(func(fd uintptr) {
		if e := syscall.SetNonblock(int(fd), true); e != nil {
			t.Fatal(e)
		}
		for filled < 1<<20 {
			n, e := syscall.Write(int(fd), filler)
			if e == syscall.EAGAIN {
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			filled += n
		}
		t.Fatal("pipe did not fill")
	}); e != nil {
		t.Fatal(e)
	}
	p := &pump{out: w, sink: SinkStdout}
	done := make(chan struct{})
	go func() { p.flushOver('x'); w.Close(); close(done) }()
	buf := make([]byte, 1<<16)
	deadline := time.Now().Add(3 * time.Second)
	for {
		n := runtime.Stack(buf, true)
		blocked := false
		for _, s := range bytes.Split(buf[:n], []byte("\n\n")) {
			if bytes.Contains(s, []byte("credkit.(*pump).flushOver")) && bytes.Contains(s, []byte("internal/poll")) {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			w.Close()
			<-done
			t.Fatal("notice did not reach blocked write")
		}
		runtime.Gosched()
	}
	held := make(chan struct{})
	go func() { Hold("minted:x", "Xwithheld"); close(held) }()
	returned := false
	select {
	case <-held:
		returned = true
	case <-time.After(200 * time.Millisecond):
	}
	b, e := io.ReadAll(r)
	if e != nil {
		t.Fatal(e)
	}
	<-done
	<-held
	if returned && bytes.Contains(b[filled:], []byte("withheld")) {
		t.Fatalf("oversize notice emitted held 8-character piece after Hold returned: held=%v output=%q", isHeld("Xwithheld"), b[filled:])
	}
}
