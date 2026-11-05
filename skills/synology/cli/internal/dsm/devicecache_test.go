// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDeviceCacheMergesAndIsPrivate(t *testing.T) {
	dir := t.TempDir()
	base := "https://nas.acme.example:5001"
	path := filepath.Join(dir, SessionFileName(base))
	// An existing session record keeps its other fields.
	if err := os.WriteFile(path, []byte(`{"origin":"`+SessionKey(base)+`","token":"sid-1","synotoken":"tok-fake-Hn6v2K","account":"admin"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveDeviceID(dir, base, "admin", "did-9"); err != nil {
		t.Fatal(err)
	}
	if got := CachedDeviceID(dir, base, "admin"); got != "did-9" {
		t.Fatalf("CachedDeviceID = %q", got)
	}
	if got := CachedDeviceID(dir, base, "someone-else"); got != "" {
		t.Fatalf("token leaked to another account: %q", got)
	}
	var rec map[string]any
	raw, _ := os.ReadFile(path)
	_ = json.Unmarshal(raw, &rec)
	if rec["token"] != "sid-1" || rec["synotoken"] != "tok-fake-Hn6v2K" {
		t.Fatalf("merge dropped fields: %s", raw)
	}
	// Windows reports 0666 for every file (ACLs, not mode bits).
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
	if CachedDeviceID(dir, "https://other:5001", "admin") != "" {
		t.Fatal("token leaked to another host")
	}
}

func TestClientReportsNewDeviceID(t *testing.T) {
	srv, _ := fakeNAS(t, false) // login answers did "dev-1"
	var got []string
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL, RatePerSec: 1000,
		Creds:      Credentials{Account: "admin", Password: "pw-fake-Lq7z3T", OTPCode: "123456"},
		OnDeviceID: func(did string) error { got = append(got, did); return nil }}
	if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "dev-1" {
		t.Fatalf("OnDeviceID calls = %v", got)
	}
	// A login that already sent the same token does not re-report it.
	got = nil
	c2 := &Client{HTTP: srv.Client(), BaseURL: srv.URL, RatePerSec: 1000,
		Creds:      Credentials{Account: "admin", Password: "pw-fake-Lq7z3T", DeviceID: "dev-1"},
		OnDeviceID: func(did string) error { got = append(got, did); return nil }}
	if _, err := c2.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("OnDeviceID re-reported: %v", got)
	}
}

func TestDeviceCacheSurvivesNullAndPerAccount(t *testing.T) {
	dir := t.TempDir()
	base := "https://nas:5001"
	if err := os.WriteFile(filepath.Join(dir, SessionFileName(base)), []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveDeviceID(dir, base, "alice", "did-a"); err != nil {
		t.Fatal(err)
	}
	if err := SaveDeviceID(dir, base, "bob", "did-b"); err != nil {
		t.Fatal(err)
	}
	if CachedDeviceID(dir, base, "alice") != "did-a" || CachedDeviceID(dir, base, "bob") != "did-b" {
		t.Fatal("two accounts on one host overwrote each other's device token")
	}
}

func TestSessionFileNamesAreUniquePerOrigin(t *testing.T) {
	names := map[string]string{}
	for _, o := range []string{"https://nas:5001", "https://nas_5001", "http://nas:5001"} {
		n := SessionFileName(o)
		if prev, dup := names[n]; dup {
			t.Fatalf("%s and %s share %s", prev, o, n)
		}
		names[n] = o
	}
	// A file recorded for another origin never yields its token.
	dir := t.TempDir()
	if err := SaveDeviceID(dir, "https://nas:5001", "admin", "did-1"); err != nil {
		t.Fatal(err)
	}
	if got := CachedDeviceIDFile(filepath.Join(dir, SessionFileName("https://nas:5001")), "https://other:5001", "admin"); got != "" {
		t.Fatalf("token returned for another origin: %q", got)
	}
}

func TestPlaceholderOrigin(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://nas.example.com:5001": true, "https://EXAMPLE.org": true, "https://box.invalid:5001": true,
		"https://nas.acme.example": true, "https://192.168.1.50:5001": false, "https://nas.acme.com:5001": false,
		"https://example-nas.net": false, "https://localhost:5001": false,
	} {
		if got := PlaceholderOrigin(raw); got != want {
			t.Errorf("PlaceholderOrigin(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestForgetSessionFilesRemovesEveryAccount(t *testing.T) {
	dir := t.TempDir()
	base := "https://nas:5001"
	_ = SaveDeviceID(dir, base, "admin", "did-a")
	_ = SaveDeviceID(dir, base, "fleet", "did-f")
	_ = SaveDeviceID(dir, "https://other:5001", "admin", "did-o")
	files, _, err := CachedSessions(base, SessionDirs{Data: dir})
	if err != nil || len(files) != 1 || files[0].NAS != "https://nas:5001" ||
		strings.Join(files[0].DeviceTokenAccounts, ",") != "admin,fleet" || files[0].SessionID {
		t.Fatalf("CachedSessions(%s) = %+v, %v", base, files, err)
	}
	removed, _, err := ForgetSessionFiles(files)
	if err != nil || len(removed) != 1 {
		t.Fatalf("ForgetSessionFiles = %+v, %v", removed, err)
	}
	if CachedDeviceID(dir, base, "admin") != "" || CachedDeviceID(dir, base, "fleet") != "" {
		t.Fatal("a trusted-device token survived forgetting its NAS's session file")
	}
	if CachedDeviceID(dir, "https://other:5001", "admin") != "did-o" {
		t.Fatal("forgetting one NAS removed another NAS's token")
	}
	// Removing again removes nothing and is not an error.
	if removed, _, err := ForgetSessionFiles(files); err != nil || len(removed) != 0 {
		t.Fatalf("second ForgetSessionFiles = %+v, %v", removed, err)
	}
}

// A session file can hold another NAS's record (a hand edit, a copied data directory).
// Saving to it must neither keep that NAS's session nor stamp this NAS's origin on it.
func TestSavingOverAnotherNASRecordKeepsNothingOfIt(t *testing.T) {
	const a, b = "https://a.example.net:5001", "https://b.example.net:5001"
	other := `{"origin":"` + SessionKey(a) + `","account":"admin-a","token":"SidOfA-` + `1Qa2Ws3Ed","synotoken":"TokOfA-` + `4Rf5Tg6Y","device_id":"DidOfA-` + `7Uj8Ik9O","device_ids":{"admin-a":"DidsOfA-` + `1Qa2Ws3E"},"cookies":[{"url":"` + a + `/","name":"id","value":"CookieOfA-` + `5Tg6Yh"}]}`
	for name, save := range map[string]func(t *testing.T, dir, path string){
		"SaveDeviceID": func(t *testing.T, dir, _ string) {
			if err := SaveDeviceID(dir, b, "admin-b", "DidOfB-2Ws3Ed4Rf"); err != nil {
				t.Fatal(err)
			}
		},
		"WriteSessionRecord": func(t *testing.T, _, path string) {
			rec := `{"origin":"` + SessionKey(b) + `","account":"admin-b","token":"SidOfB-` + `3Ed4Rf5T"}`
			if err := WriteSessionRecord(path, []byte(rec), "admin-b", "DidOfB-2Ws3Ed4Rf"); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := SessionFilePath(dir, b)
			if err := os.WriteFile(path, []byte(other), 0o600); err != nil {
				t.Fatal(err)
			}
			save(t, dir, path)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, of := range []string{"OfA", "admin-a", "a.example.net"} {
				if strings.Contains(string(data), of) {
					t.Errorf("the saved record keeps %q of another NAS:\n%s", of, data)
				}
			}
			if got := CachedDeviceID(dir, b, "admin-b"); got != "DidOfB-2Ws3Ed4Rf" {
				t.Errorf("this NAS's token = %q:\n%s", got, data)
			}
		})
	}
}

// The same NAS's other fields and other accounts' tokens stay.
func TestSavingOverOwnRecordKeepsIt(t *testing.T) {
	const b = "https://b.example.net:5001"
	dir := t.TempDir()
	path := SessionFilePath(dir, b)
	own := `{"origin":"` + SessionKey(b) + `","account":"admin-b","token":"SidOfB-` + `3Ed4Rf5T","device_ids":{"second":"DidSecond-` + `9Ol8Ik7"}}`
	if err := os.WriteFile(path, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveDeviceID(dir, b, "admin-b", "DidOfB-2Ws3Ed4Rf"); err != nil {
		t.Fatal(err)
	}
	if got := CachedDeviceID(dir, b, "second"); got != "DidSecond-9Ol8Ik7" {
		t.Errorf("the other account's token = %q", got)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "SidOfB-3Ed4Rf5T") {
		t.Errorf("the session was dropped:\n%s", data)
	}
	if err := WriteSessionRecord(path, []byte(`{"origin":"`+SessionKey(b)+`","account":"admin-b"}`), "admin-b", ""); err != nil {
		t.Fatal(err)
	}
	if got := CachedDeviceID(dir, b, "second"); got != "DidSecond-9Ol8Ik7" {
		t.Errorf("after a session write, the other account's token = %q", got)
	}
}
