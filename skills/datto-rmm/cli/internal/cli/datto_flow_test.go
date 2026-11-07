// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"datto-rmm-pp-cli/internal/store"
)

// End-to-end through the real root command against a local Datto double:
// datto-rmm handfixes token-minted-per-client, dry-run-never-mints,
// sync-follows-next-page-url and doctor-datto-root-not-placeholder.

type dattoDouble struct {
	srv     *httptest.Server
	mu      sync.Mutex
	mints   int
	bearers []string
	pages   []string
	devices int
	next    func(host string, page int) string
}

func newDattoDouble(t *testing.T, access string, devices int) *dattoDouble {
	t.Helper()
	d := &dattoDouble{devices: devices}
	d.next = func(host string, page int) string {
		return fmt.Sprintf("http://%s/api/v2/account/devices?max=250&page=%d", host, page)
	}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/oauth/token" && r.Method == http.MethodPost {
			_ = r.ParseForm()
			if r.PostForm.Get("grant_type") != "password" || r.PostForm.Get("username") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			d.mints++
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":359999}`, access)
			return
		}
		d.bearers = append(d.bearers, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v2/account":
			_, _ = w.Write([]byte(`{"id":1,"uid":"acct-1","name":"Double MSP"}`))
		case "/api/v2/account/devices":
			// The server caps a page at 2 whatever max asks for, pages from 0,
			// and links the next page; a short page is never the end.
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			d.pages = append(d.pages, r.URL.RawQuery)
			var rows []string
			for i := page * 2; i < (page+1)*2 && i < d.devices; i++ {
				rows = append(rows, fmt.Sprintf(`{"id":%d,"uid":"dev-%d","hostname":"HOST-%d"}`, 1000+i, i, i))
			}
			next := "null"
			if (page+1)*2 < d.devices {
				next = strconv.Quote(d.next(r.Host, page+1))
			}
			_, _ = fmt.Fprintf(w, `{"pageDetails":{"count":%d,"totalCount":%d,"prevPageUrl":null,"nextPageUrl":%s},"devices":[%s]}`,
				len(rows), d.devices, next, strings.Join(rows, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func runDatto(t *testing.T, env map[string]string, args ...string) (string, string, error) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	for _, k := range []string{"DATTO_RMM_TOKEN", "DATTO_RMM_API_KEY", "DATTO_RMM_API_SECRET_KEY", "DATTO_RMM_PLATFORM",
		"DATTO_RMM_API_URL", "DATTO_RMM_CONFIG", "DATTO_RMM_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	var out, errb bytes.Buffer
	root := newRootCmd(&rootFlags{})
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func TestKeyAndSecretMintOnTheFirstAPICall(t *testing.T) {
	d := newDattoDouble(t, "flowMintedToken-5d2a", 0)
	stdout, stderr, err := runDatto(t, map[string]string{
		"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_API_KEY": "flowKey-1a2b3c", "DATTO_RMM_API_SECRET_KEY": "flowSecret-4d5e6f",
	}, "account", "get-user", "--json")
	if err != nil {
		t.Fatalf("account get-user with only a key and secret: %v\n%s", err, stderr)
	}
	if d.mints != 1 || len(d.bearers) == 0 || d.bearers[0] != "Bearer flowMintedToken-5d2a" {
		t.Fatalf("want one mint and the minted bearer on the API call; mints %d bearers %v", d.mints, d.bearers)
	}
	if !strings.Contains(stdout, "Double MSP") {
		t.Fatalf("the API answer did not reach stdout:\n%s", stdout)
	}
	for _, s := range []string{"flowMintedToken-5d2a", "flowSecret-4d5e6f", "flowKey-1a2b3c"} {
		if strings.Contains(stdout+stderr, s) {
			t.Fatalf("output repeats a credential %q", s)
		}
	}
}

func TestDryRunNeverMints(t *testing.T) {
	d := newDattoDouble(t, "dryRunToken-0000", 0)
	env := map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_API_KEY": "dryKey-1111", "DATTO_RMM_API_SECRET_KEY": "drySecret-2222"}
	if _, stderr, err := runDatto(t, env, "account", "get-user", "--dry-run"); err != nil {
		t.Fatalf("dry-run: %v\n%s", err, stderr)
	}
	if _, stderr, err := runDatto(t, env, "auth", "login", "--dry-run"); err != nil {
		t.Fatalf("auth login --dry-run: %v\n%s", err, stderr)
	}
	if d.mints != 0 || len(d.bearers) != 0 {
		t.Fatalf("--dry-run sent %d mints and %d API requests", d.mints, len(d.bearers))
	}
}

func TestSyncWalksEveryLinkedPage(t *testing.T) {
	d := newDattoDouble(t, "unused", 5)
	// A next link naming another host moves only the page number: every
	// request still goes to the configured host.
	d.next = func(_ string, page int) string {
		return fmt.Sprintf("https://elsewhere.example/api/v2/account/devices?max=250&page=%d", page)
	}
	db := filepath.Join(t.TempDir(), "sync.db")
	_, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_TOKEN": "syncToken-7e7e"},
		"sync", "--resources", "account-devices", "--full", "--json", "--db", db)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n int
	if err := s.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = 'account-devices'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("stored %d of 5 devices across pages; requests %v", n, d.pages)
	}
	if len(d.pages) != 3 || !strings.Contains(d.pages[0], "max=250") || !strings.Contains(d.pages[2], "page=2") {
		t.Fatalf("want three page requests (no page, page=1, page=2) asking max=250, got %v", d.pages)
	}
}

func TestDoctorProbesTheMerlotDefault(t *testing.T) {
	if doctorBaseURLIsShippedPlaceholder("https://merlot-api.centrastage.net/api") {
		t.Fatal("doctor reads Datto's merlot platform API, a real host, as the shipped placeholder")
	}
	if doctorShippedBaseURL != "" {
		t.Fatalf("doctorShippedBaseURL = %q, want empty", doctorShippedBaseURL)
	}
}

func TestListAllWalksEveryLinkedPage(t *testing.T) {
	d := newDattoDouble(t, "unused", 5)
	stdout, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_TOKEN": "listToken-7a7a"},
		"account", "get-user-devices", "--all", "--json", "--data-source", "live")
	if err != nil {
		t.Fatalf("list --all: %v\n%s", err, stderr)
	}
	for i := 0; i < 5; i++ {
		if !strings.Contains(stdout, fmt.Sprintf("HOST-%d", i)) {
			t.Fatalf("--all lost HOST-%d; requests %v\n%s", i, d.pages, stdout)
		}
	}
	if len(d.pages) != 3 || strings.Contains(d.pages[0], "page=") || !strings.Contains(d.pages[1], "page=1") {
		t.Fatalf("want page 0 (no page param), then the linked pages 1 and 2; got %v", d.pages)
	}
}

func TestKeyPairReadsAsConfigured(t *testing.T) {
	d := newDattoDouble(t, "unused", 0)
	env := map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_API_KEY": "statusKey-1111", "DATTO_RMM_API_SECRET_KEY": "statusSecret-2222"}
	stdout, stderr, err := runDatto(t, env, "auth", "status", "--json")
	if err != nil || !strings.Contains(stdout, `"authenticated": true`) {
		t.Fatalf("auth status with the key pair alone: err %v\n%s\n%s", err, stdout, stderr)
	}
	if d.mints != 0 {
		t.Fatalf("auth status minted %d token(s); it only reads the configuration", d.mints)
	}
}

func TestDoctorVerifiesTheKeyPair(t *testing.T) {
	d := newDattoDouble(t, "doctorToken-3c3c", 0)
	stdout, stderr, _ := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": d.srv.URL + "/api", "DATTO_RMM_API_KEY": "doctorKey-1111", "DATTO_RMM_API_SECRET_KEY": "doctorSecret-2222"},
		"doctor", "--json")
	if !strings.Contains(stdout, `"credentials": "valid"`) {
		t.Fatalf("doctor with the key pair alone did not verify it (mints %d, bearers %v):\n%s\n%s", d.mints, d.bearers, stdout, stderr)
	}
}

var dattoPublicClientForTest = "Basic " + base64.StdEncoding.EncodeToString([]byte("public-client:public"))
