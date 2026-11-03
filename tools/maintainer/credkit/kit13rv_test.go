// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// kit13rv, config-reader-drops-values (loop ruling 6): a declared credential
// key followed by a value is held wherever it appears in a credential file,
// inside an unclosed array or table, after a parse error, at any nesting.
func TestKit13ConfigAssignmentHeldWherever(t *testing.T) {
	d := Declarations{ConfigFields: []string{"api_key", "token", "auth"}}
	for _, c := range []struct{ name, file, want string }{
		{"recorded: unterminated array, multi-line string", "bad = [\napi_key = \"\"\"\nLeakProbeKeyq7xyz\"\"\"\n", "LeakProbeKeyq7xyz"},
		{"smallest: unterminated array", "bad = [\napi_key = \"LeakProbeKeyq7xyz\"\n", "LeakProbeKeyq7xyz"},
		{"closed array swallows an assignment", "bad = [\napi_key = \"Swallowed7Key9x\"\n]\n", "Swallowed7Key9x"},
		{"unclosed inline table", "bad = {\napi_key = \"InlineOpen7Key9\"\n", "InlineOpen7Key9"},
		{"YAML quoted scalar swallows the next line", "note: \"start\napi_key: Yaml7Swallowed9\nend\"\n", "Yaml7Swallowed9"},
		{"headers table inside an unclosed array", "bad = [\n[headers]\nAuthorization = \"Bearer Hdr7Unclosed9\"\n", "Bearer Hdr7Unclosed9"},
		{"credential table inside an unclosed array", "bad = [\n[auth]\nsecret = \"AuthTable7Open9\"\n", "AuthTable7Open9"},
		{"YAML parent inside an unclosed flow", "bad: [\nauth:\n  secret: Nested7Yaml9Tok\n", "Nested7Yaml9Tok"},
		{"dotted headers key in an unclosed array", "bad = [\nheaders.X-Api-Key = \"Dotted7Hdr9Key\"\n", "Dotted7Hdr9Key"},
		{"escaped key in an unclosed array", "bad = [\n\"api\\u005fkey\" = \"Escaped7Key9x\"\n", "Escaped7Key9x"},
		{"key in another case in an unclosed array", "bad = [\nAPI_KEY = \"Upper7Key9x\"\n", "Upper7Key9x"},
		{"key spelled with a folding letter (Kelvin sign)", "bad = [\nto\u212aen = \"Kelvin7Key9x\"\n", "Kelvin7Key9x"},
		{"single-quoted key in an unclosed array", "bad = [\n'api_key' = \"Single7Key9x\"\n", "Single7Key9x"},
		{"JSON member after a broken value", "{\"bad\": [1, 2,\n\"token\": \"Json7Broken9Tok\"}\n", "Json7Broken9Tok"},
	} {
		gatherString(t, d, "c.toml", c.file)
		if !isHeld(c.want) {
			t.Errorf("%s: %q not held; held %q", c.name, c.want, heldValues())
		}
	}
}

// The blunt rule never fires on an ordinary credential file: on well-formed
// files, commented template lines included, it holds nothing the reader did
// not already hold.
func TestKit13ConfigBluntRuleQuietOnOrdinaryFiles(t *testing.T) {
	d := Declarations{ConfigFields: []string{"api_key", "token", "client_secret", "password"}, StoredRecords: []string{"refresh_token"}}
	files := map[string]string{
		"toml": "# Copy to ~/.config/x/config.toml\n# api_key = \"your-api-key\"\n# token = true\nbase_url = \"https://api.example.com\"\n" +
			"api_key = \"Ord7Toml9Key\"\ntimeout = 30\nverbose = false\n\n[headers]\nX-Tenant = \"Tenant7Hdr9\"\n\n" +
			"[profiles.default]\nregion = \"us-east-1\"\nauth_method = \"api_key\"\n",
		"yaml": "# token: example\nbase_url: https://api.example.com\ntoken: Ord7Yaml9Tok\nretries: 3\nscopes:\n  - read\n  - write\noauth:\n  client_secret: Ord7Yaml9Sec\n  type: token\n",
		"json": "{\"base_url\": \"https://api.example.com\", \"password\": \"Ord7Json9Pw\", \"ids\": [1, 2, 3], \"mode\": \"token\", \"nested\": {\"refresh_token\": \"Ord7Json9Ref\"}}\n",
		"env":  "# API_KEY=changeme\nAPI_KEY=Ord7Env9Key\nexport TOKEN=Ord7Env9Tok\nLOG_LEVEL=info\n",
	}
	for name, f := range files {
		gatherString(t, d, "c."+name, f)
		reg.Lock()
		blunt := credentialAssignmentsLocked([]byte(f))
		reg.Unlock()
		var main []string
		for _, v := range readConfig([]byte(f)) {
			main = append(main, v.value)
		}
		for _, v := range blunt {
			if !slices.Contains(main, v.value) {
				t.Errorf("%s: the blunt rule added %q, which the reader did not hold", name, v.value)
			}
		}
		if !preciseConfig([]byte(f)) {
			continue // loop ruling 9: a file outside the canonical subset holds every string value, comments' included
		}
		for _, v := range heldValues() {
			if strings.Contains(v, "your-api-key") || v == "true" || v == "changeme" || v == "example" {
				t.Errorf("%s: a commented template value %q was held", name, v)
			}
		}
	}
	// And the corpus stays clean with what an ordinary file holds.
	gatherString(t, d, "c.toml", files["toml"])
	m, _ := currentMatcher()
	for name, unit := range ordinaryCorpus(t) {
		if v := m.check(unit); v != clean {
			t.Errorf("%s: verdict %d after an ordinary credential file, want clean", name, v)
		}
	}
}

// kit13rv Codex r1, mcp-error-bypasses-guard: a body read that ends in an
// error (Content-Length one past the bytes sent, the writing side closed)
// still returns the bytes read; their arguments are held before the 400
// goes out, so its reason phrase cannot carry the credential.
func TestKit13MCPBodyReadErrorHoldsWhatWasRead(t *testing.T) {
	resetForTest(Declarations{MCPArguments: []string{"token"}})
	srv := httptest.NewServer(MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	defer srv.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{"token":"BAD REQUEST"}}}`
	c, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body)+1, body)
	c.(*net.TCPConn).CloseWrite()
	got, _ := io.ReadAll(c)
	if !isHeld("BAD REQUEST") {
		t.Errorf("the argument of a body read that ended in an error was not held; held %q", heldValues())
	}
	if strings.Contains(strings.ToLower(string(got)), "bad request") {
		t.Errorf("the response carries the held argument: %q", got)
	}
}

// kit13rv Codex r1, decoder-length-pruning: a final text view of 16 KiB or
// more is read by the UTF-16, UTF-32, gzip and zlib decoders, as a smaller
// one is (the scheduler never decides which decoders read a part).
func TestKit13BigFinalViewReadByEveryFamily(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:probe", "secret!!")
	unit := []byte("{\"pad\":\"" + strings.Repeat(" ", 16384) + "\",\"a\":\"s\\u0000e\\u0000c\\u0000r\\u0000\",\"b\":\"e\\u0000t\\u0000!\\u0000!\\u0000\"}")
	m, _ := currentMatcher()
	if v := m.check(unit); v == clean {
		t.Errorf("a joined final view over 16 KiB carrying UTF-16LE of the held value read clean")
	}
	small := []byte("{\"pad\":\"x\",\"a\":\"s\\u0000e\\u0000c\\u0000r\\u0000\",\"b\":\"e\\u0000t\\u0000!\\u0000!\\u0000\"}")
	if v := m.check(small); v == clean {
		t.Errorf("the same document without the padding read clean")
	}
}

// The same class on the outbound transport: a request body whose read ends
// in an error has what it read held before the error returns.
func TestKit13RequestBodyReadErrorHoldsWhatWasRead(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	body := io.MultiReader(strings.NewReader("password="+"Partial7Body9x"), iotestErr{})
	req, _ := http.NewRequest("POST", "https://api.example.com/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := Transport(http.DefaultTransport).RoundTrip(req); err == nil {
		t.Fatal("the request was sent")
	}
	if !isHeld("Partial7Body9x") {
		t.Errorf("the field read before the body error was not held; held %q", heldValues())
	}
}

type iotestErr struct{}

func (iotestErr) Read([]byte) (int, error) { return 0, fmt.Errorf("body reader failed") }
