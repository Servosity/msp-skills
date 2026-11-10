// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build unix

package credkit

import (
	"bufio"
	"bytes"
	"database/sql/driver"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// kit15rv Codex r1, decoder-binary-intermediate-pruning (the reviewer's own input).
func TestKit15R1MixedBinaryPercent(t *testing.T) {
	resetForTest(Declarations{})
	secret := "\xffsecret!"
	unit := []byte("\xffsecret%21")
	Hold("minted:x", secret)
	var out bytes.Buffer
	_, err := NewWriter(&out, SinkFile).Write(unit)
	if out.Len() > 0 && !bytes.HasPrefix(out.Bytes(), []byte("[credkit: withheld")) {
		t.Fatalf("CLEAN held=%v err=%v secret=%x unit=%x output=%x", isHeld(secret), err, []byte(secret), unit, out.Bytes())
	}
}

// kit15rv Codex r1, charset-context-pruning (the reviewer's own input).
func TestKit15R1NonmodalCharsetPercentJoin(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	var unit []byte
	for _, r := range "%7B\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}" {
		unit = append(unit, byte(r), byte(r>>8))
	}
	var out bytes.Buffer
	_, err := NewWriter(&out, SinkFile).Write(unit)
	if out.Len() > 0 && !bytes.HasPrefix(out.Bytes(), []byte("[credkit: withheld")) {
		t.Fatalf("CLEAN err=%v unit=%x output=%x", err, unit, out.Bytes())
	}
}

// kit15rv Codex r1, mcp-error-bypasses-guard (the reviewer's own input).
func TestKit15R1103Fallback(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "chunked")
	inner := MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(103); _, _ = w.Write([]byte("{}")) }))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { inner.ServeHTTP(noHijack{w}, r) }))
	defer srv.Close()
	c, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	b, _ := io.ReadAll(bufio.NewReader(c))
	if strings.Contains(string(b), "chunked") {
		t.Fatalf("held=%v wire=%q", isHeld("chunked"), b)
	}
}

// kit15rv Codex r1, response-field-context-loss (the reviewer's own input).
func TestKit15R1ResponseAcquisitionTiming(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"token"}})
	resp := &http.Response{Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader("{\"echo\":\"secret!!\","), strings.NewReader("\"token\":\"secret!!\"}")))}
	holdResponse(resp, origin{})
	var out bytes.Buffer
	_, err := io.Copy(NewWriter(&out, SinkFile), resp.Body)
	if bytes.Contains(out.Bytes(), []byte("secret!!")) {
		t.Fatalf("held=%v err=%v output=%q", isHeld("secret!!"), err, out.String())
	}
}

// kit15rv Codex r1, request-credentials-added-after-acquisition (the reviewer's own input).
func TestKit15R1ProxyCallbackAcquisition(t *testing.T) {
	resetForTest(Declarations{})
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("Authorization")
		w.WriteHeader(204)
	}))
	defer srv.Close()
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = func(r *http.Request) (*url.URL, error) {
		r.Header.Set("Authorization", "Bearer secret!!")
		return nil, nil
	}
	c := &http.Client{Transport: Transport(base)}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var out bytes.Buffer
	_, err = NewWriter(&out, SinkFile).Write([]byte("secret!!"))
	if received != "" && out.Len() > 0 {
		t.Fatalf("received=%q held=%v err=%v output=%q", received, isHeld("secret!!"), err, out.String())
	}
}

// kit15rv Codex r1, config-reader-drops-values (the reviewer's own input).
func TestKit15R1YAMLConfigEscape(t *testing.T) {
	gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.yaml", "token: \"secret\\N!!\"\n")
	var out bytes.Buffer
	_, err := NewWriter(&out, SinkFile).Write([]byte("secret\u0085!!"))
	if out.Len() > 0 && !bytes.HasPrefix(out.Bytes(), []byte("[credkit: withheld")) {
		t.Fatalf("held=%q err=%v output=%q", heldValues(), err, out.String())
	}
}

// kit15rv Codex r1, file-placement-race (the reviewer's own input).
func TestKit15R1ChangedAppendMode(t *testing.T) {
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
	if _, err = f.Seek(4, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFL, 0); e != 0 {
		t.Fatal(e)
	}
	_, err = w.Write([]byte("et!!"))
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("secret!!")) {
		t.Fatalf("err=%v file=%q", err, b)
	}
}

// kit15rv Codex r1, held-set-update-race (the reviewer's own input).
func TestKit15R1StaleHeldSnapshot(t *testing.T) {
	resetForTest(Declarations{})
	p := filepath.Join(t.TempDir(), "f")
	fileFault = func(step string) error {
		if step == "truncate" {
			Hold("minted:x", "secret!!")
		}
		return nil
	}
	defer func() { fileFault = nil }()
	err := WriteFile(p, []byte("secret!!"), 0600)
	b, _ := os.ReadFile(p)
	if bytes.Contains(b, []byte("secret!!")) {
		t.Fatalf("err=%v held=%v output=%q", err, isHeld("secret!!"), b)
	}
}

// kit15rv Codex r1, response-key-bom-pruning (the reviewer's own input).
func TestKit15R1ResponseUnicodeKey(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"\ufb00token"}})
	body := "\ufb00token=secret!!"
	resp := &http.Response{Header: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}}, Body: io.NopCloser(strings.NewReader(body))}
	holdResponse(resp, origin{})
	io.ReadAll(resp.Body)
	var out bytes.Buffer
	_, err := NewWriter(&out, SinkFile).Write([]byte("secret!!"))
	if out.Len() > 0 && !bytes.HasPrefix(out.Bytes(), []byte("[credkit: withheld")) {
		t.Fatalf("held=%q err=%v output=%q", heldValues(), err, out.String())
	}
}

type reviewTxDriver struct {
	recDriver
	ran []string
}

func (d *reviewTxDriver) Open(string) (driver.Conn, error) {
	return &reviewTxConn{recConn: recConn{&d.recDriver}, d: d}, nil
}

type reviewTxConn struct {
	recConn
	d *reviewTxDriver
}

func (c *reviewTxConn) Begin() (driver.Tx, error) {
	c.d.ran = append(c.d.ran, "begin")
	return reviewTx{c.d}, nil
}

type reviewTx struct{ d *reviewTxDriver }

func (x reviewTx) Commit() error   { x.d.ran = append(x.d.ran, "commit"); return nil }
func (x reviewTx) Rollback() error { x.d.ran = append(x.d.ran, "rollback"); return nil }
