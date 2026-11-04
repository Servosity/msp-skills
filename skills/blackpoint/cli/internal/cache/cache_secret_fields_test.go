// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (secret-field-persistence class): Set never writes a value
// that holds, or may hold, a response secret.

package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSetSeesEveryKeyOccurrence(t *testing.T) {
	const pw = "breached-secret-123"
	dir := t.TempDir()
	s := New(dir, time.Minute)
	for i, v := range []string{
		`{"id":"e1","password":"` + pw + `"}`,
		`{"data":[{"id":"e1","password":"` + pw + `","password":null}]}`,
		`{"data":[]} {"password":"` + pw + `"}`,
		`{"data":[]}garbage"password":"` + pw + `"`,
		`{"data":[{"password":"` + pw + `"}`,
		"\xEF\xBB\xBF" + `{"password":"` + pw + `"}`,
	} {
		s.Set(strconv.Itoa(i), json.RawMessage(v))
	}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), pw) {
				t.Fatalf("cache file %s holds the response password", p)
			}
		}
		return nil
	})
	clean := json.RawMessage(`{"id":"e1","token_hint":"abc","password":null}`)
	s.Set("clean", clean)
	if got, ok := s.Get("clean"); !ok || string(got) != string(clean) {
		t.Fatalf("a clean value must still be cached: %s %v", got, ok)
	}
}
