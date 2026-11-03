package credkit

// kit15rv Codex round 5 (fresh session 01a19f20), its probes as written but
// for the Hold schedule noted below.

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
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestKit15R5PrefetchedTrailers(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"token"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Trailer", "Set-Cookie")
		io.WriteString(w, `{"ok":1}`)
		w.Header().Set("Set-Cookie", "session=secret!!")
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := Transport(&http.Transport{}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	value := resp.Trailer.Get("Set-Cookie")
	file := filepath.Join(t.TempDir(), "trailer.log")
	err = WriteFile(file, []byte(value), 0600)
	out, _ := os.ReadFile(file)
	if bytes.Contains(out, []byte("secret!!")) {
		t.Fatalf("trailer exposed before holding: trailer=%q held=%v unknown=%v sink=%q err=%v", value, isHeld("secret!!"), isUnknown(), out, err)
	}
}

func TestKit15R5LongNumericReferenceWideJoin(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	text := "&#000000000000⑫3;\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}"
	unit := utf16LE(text)
	decoded, err := utf16enc.UTF16(utf16enc.LittleEndian, utf16enc.IgnoreBOM).NewDecoder().Bytes(unit)
	if err != nil {
		t.Fatal(err)
	}
	doc := html.UnescapeString(norm.NFKC.String(string(decoded)))
	var standard struct {
		A string
		B string
	}
	if err := json.Unmarshal([]byte(doc), &standard); err != nil || standard.A+standard.B != "secret!!" {
		t.Fatalf("standard read failed: %q %+v %v", doc, standard, err)
	}
	m, _ := currentMatcher()
	v := m.check(unit)
	file := filepath.Join(t.TempDir(), "wide.log")
	err = WriteFile(file, unit, 0600)
	out, _ := os.ReadFile(file)
	if bytes.Equal(out, unit) {
		t.Fatalf("CLEAN charset -> NFKC -> character reference -> joined values: verdict=%v err=%v unit=%x", v, err, unit)
	}
}

func TestKit15R5CustomTransportStaleInitialCheck(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:other", "unrelated-value")
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("q")
		w.WriteHeader(204)
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/?q=secret%21%21", nil)
	o, _ := originOf(req.URL)
	m, _ := originMatcher(o, true)
	m.prepare()
	m.prep = sync.Once{}
	ready, release := make(chan struct{}), make(chan struct{})
	go m.prep.Do(func() { close(ready); <-release })
	<-ready
	done := make(chan error, 1)
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) { return http.DefaultTransport.RoundTrip(r) })
	go func() {
		resp, err := Transport(rt).RoundTrip(req)
		if resp != nil {
			resp.Body.Close()
		}
		done <- err
	}()
	stack := make([]byte, 1<<16)
	until := time.Now().Add(3 * time.Second)
	for {
		n := runtime.Stack(stack, true)
		blocked := false
		for _, st := range bytes.Split(stack[:n], []byte("\n\n")) {
			if bytes.Contains(st, []byte("credkit.(*transport).RoundTrip")) && bytes.Contains(st, []byte("credkit.(*matcher).prepare")) {
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
			t.Fatal("request did not reach initial check")
		}
		runtime.Gosched()
	}
	// Hold on its own goroutine (not Codex's schedule): a Hold that waits
	// for the request in flight is correct; one that returned must not be
	// followed by the value on the wire.
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
	if returned && got == "secret!!" {
		t.Fatalf("request sent after Hold returned, using stale check: query=%q err=%v held=%v", got, err, isHeld("secret!!"))
	}
}
