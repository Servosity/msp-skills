// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/credkit"
)

// Codex round 1: the archive GET was sent on the plain transport, outside
// the credential kit, so a portal-supplied link (or a redirect hop) naming
// another host with a held credential in it sent that credential there. The
// kit now judges the request and every hop without sending them, and the
// archive bytes still stream on the plain transport.

// archiveTok is held for this test only; split so no scanner reads it as a
// literal credential.
var archiveTok = "ak_R7m2X" + "9q4L1w8Z5"

type storageRec struct {
	mu   sync.Mutex
	seen []string
}

func (r *storageRec) note(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Decoded too: the query, the path, and Basic credentials from userinfo.
	_, pass, _ := req.BasicAuth()
	r.seen = append(r.seen, req.URL.String()+" "+req.URL.Path+" "+fmt.Sprint(req.URL.Query())+" "+pass)
}

func (r *storageRec) carried(s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.seen {
		if strings.Contains(x, s) {
			return true
		}
	}
	return false
}

func (r *storageRec) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = nil
}

func TestArchiveRequestsNeverCarryAHeldValueOffOrigin(t *testing.T) {
	credkit.Hold("minted:archive-test", archiveTok)
	rec := &storageRec{}
	var secondURL string
	serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.note(r)
		switch r.URL.Path {
		case "/hop":
			http.Redirect(w, r, r.URL.Query().Get("to"), http.StatusFound)
			return
		case "/hop-held":
			// The storage names the held value itself: the first URL is
			// clean, so only the kit's judgement of the hop can stop it.
			http.Redirect(w, r, secondURL+"/obj?token="+archiveTok, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte(zipBody))
	})
	// Two origins (one port each) the download policy allows; httptest's
	// TLS servers share one certificate, so the client trusts both.
	storage, second := httptest.NewTLSServer(serve), httptest.NewTLSServer(serve)
	defer storage.Close()
	defer second.Close()
	su, _ := url.Parse(storage.URL)
	tu, _ := url.Parse(second.URL)
	secondURL = second.URL
	withDownloadPolicy(t, func(u *url.URL) error {
		if u.Host == su.Host || u.Host == tu.Host {
			return nil
		}
		return StrictDownloadURL(u)
	})

	f := newFakePortal(t, "tok")
	var link string
	f.getHandler = func(string, map[string][]string) string {
		b, _ := jsonString(link)
		return `{"success":true,"data":` + b + `}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Options{BaseURL: srv.URL, RatePerSec: -1, HTTPClient: storage.Client()}, "tok")
	if err != nil {
		t.Fatal(err)
	}
	// The portal session was opened through the test's identity guard; the
	// archive requests from here on are judged by the real kit.
	prev := client.OutboundGuard
	client.OutboundGuard = credkit.Transport
	defer func() { client.OutboundGuard = prev }()

	fetch := func(l string) error {
		link = l
		var buf bytes.Buffer
		_, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{})
		return err
	}
	// Controls: with no held value the link, and a hop to the second
	// origin, are fetched, so each refusal below is the kit's doing.
	if err := fetch(storage.URL + "/obj?X-Amz-Signature=abc"); err != nil {
		t.Fatalf("a clean link must be fetched: %v", err)
	}
	if err := fetch(storage.URL + "/hop?to=" + url.QueryEscape(second.URL+"/obj?X-Amz-Signature=abc")); err != nil {
		t.Fatalf("a clean cross-origin hop must be followed: %v", err)
	}
	// Presigned links as object storage mints them, percent-encoding
	// included, are fetched, and leave the next link fetched too.
	for _, l := range []string{
		storage.URL + "/bucket/job%201/part1.zip?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAEXAMPLE%2F20261105%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20261105T120000Z&X-Amz-Expires=60&X-Amz-SignedHeaders=host&X-Amz-Signature=" + strings.Repeat("9f", 32),
		storage.URL + "/container/part1.zip?sv=2021-08-06&se=2026-11-05T12%3A01%3A00Z&sr=b&sp=r&sig=" + url.QueryEscape("Zm9vYmFyYmF6+/="),
		second.URL + "/obj?Expires=1793900000&Signature=" + url.QueryEscape("aGVsbG8gd29ybGQ="),
	} {
		for i := 0; i < 2; i++ {
			if err := fetch(l); err != nil {
				t.Fatalf("a presigned link must be fetched: %v", err)
			}
		}
	}
	if err := fetch(storage.URL + "/obj?X-Amz-Signature=abc"); err != nil {
		t.Fatalf("a clean link after presigned ones must be fetched: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(archiveTok))
	var pct strings.Builder
	for i := 0; i < len(archiveTok); i++ {
		pct.WriteString("%" + strings.ToUpper(hexByte(archiveTok[i])))
	}
	// The hop whose first URL is clean runs first: once the kit has refused
	// a userinfo or percent-encoded credential it may withhold more, which
	// would refuse that clean first URL and hide whether the hop is judged.
	for _, tc := range []struct{ name, link string }{
		{"redirect hop the storage names", storage.URL + "/hop-held"},
		{"query", storage.URL + "/obj?token=" + archiveTok},
		{"path", storage.URL + "/obj/" + archiveTok},
		{"userinfo", strings.Replace(storage.URL, "https://", "https://u:"+archiveTok+"@", 1) + "/obj"},
		{"percent-encoded", storage.URL + "/obj?token=" + pct.String()},
		{"base64", storage.URL + "/obj?t=" + url.QueryEscape(b64)},
		{"fragment", storage.URL + "/obj#" + archiveTok},
		{"redirect hop named in the link", storage.URL + "/hop?to=" + url.QueryEscape(second.URL+"/obj?token="+archiveTok)},
	} {
		rec.reset()
		err := fetch(tc.link)
		if err == nil {
			t.Errorf("%s: a link carrying a held value was fetched", tc.name)
		} else if strings.Contains(err.Error(), archiveTok) || strings.Contains(err.Error(), b64) {
			t.Errorf("%s: the refusal names the value: %v", tc.name, err)
		}
		if rec.carried(archiveTok) || rec.carried(b64) || rec.carried(url.QueryEscape(b64)) {
			t.Errorf("%s: storage received the held value", tc.name)
		}
	}
}

func hexByte(b byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[b>>4], digits[b&15]})
}

func jsonString(s string) (string, error) {
	var b strings.Builder
	b.WriteByte('"')
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`))
	b.WriteByte('"')
	return b.String(), nil
}
