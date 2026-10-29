package credkit

// kit15rv Codex round 6 (session 01a19f20, its second round), the probe as written.

import (
	"bytes"
	"encoding/json"
	utf16enc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/unicode/norm"
	"mime"
	"os"
	"path/filepath"
	"testing"
)

func TestKit15R6WideCompatibilityMIMEJoin(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	text := "=?utf-8?B?ｅｗ==?=\"あ\":0,\"a\":\"secr\",\"b\":\"et!!\"}"
	unit := utf16LE(text)
	decoded, err := utf16enc.UTF16(utf16enc.LittleEndian, utf16enc.IgnoreBOM).NewDecoder().Bytes(unit)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := new(mime.WordDecoder).DecodeHeader(norm.NFKC.String(string(decoded)))
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("CLEAN charset -> NFKC -> MIME word -> joined values: verdict=%v err=%v unit=%x", v, err, unit)
	}
}
