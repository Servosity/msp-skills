// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

// The three credential classes the first publish pass stopped on, each from
// the failing input its review named, driven through the built binaries wired
// to the shared credential kit (handfixes.json credkit-wiring).

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The review's value, as the configured API key: a credential a declared
// source holds. As free text alone it is outside contract section 1.
const bbRound2 = "Round2" + "Password-9c71b4"

func round2Env(t *testing.T) *bbEnv {
	t.Helper()
	e := newEnv(t, echoServer(t), false)
	e.env = append(e.env, "PRTG_API_KEY="+bbRound2)
	return e
}

// Class mcp-protocol-frame-registration: an unknown tool's rejection repeats
// the tool name, and handler middleware never saw the password argument.
func TestKitUnknownToolNeverEchoesADeclaredArgument(t *testing.T) {
	e := newEnv(t, echoServer(t), false)
	out := e.mcpCalls(t, []map[string]any{{
		"name": bbRound2, "arguments": map[string]any{"password": bbRound2},
	}})
	if strings.Contains(out, bbRound2) {
		t.Fatalf("the rejection repeated the password argument:\n%.800s", out)
	}
	if !strings.Contains(out, `"id":1`) {
		t.Fatalf("no answer to the call, the frame must be answered, not dropped:\n%.800s", out)
	}
}

// Class sqlite-persistence-late-discovery: a credential in an earlier argument
// than the one that reveals it. Every bound value is one unit of one statement.
func TestKitTeachNeverStoresAHeldCredential(t *testing.T) {
	e := round2Env(t)
	e.run(t, "teach", "--query", "sensor uptime", "--resource-type", "sensors", "--resource", "2322",
		"--venue", bbRound2, "--notes", `{"password":"`+bbRound2+`"}`, "--no-validate")
	_ = filepath.WalkDir(e.home, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(data), bbRound2) {
			t.Errorf("%s holds the credential", p)
		}
		return nil
	})
}

// Class outbound-sink-embedded-json-credential: the feedback POST carries the
// credential inside a JSON-encoded string to a host it is not allowed.
func TestKitFeedbackNeverSendsAHeldCredentialToItsEndpoint(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, string(b))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	e := round2Env(t)
	e.env = append(e.env, "PRTG_FEEDBACK_ENDPOINT="+srv.URL+"/feedback")
	// Honest feedback arrives, so the endpoint below is really reachable.
	e.run(t, "feedback", "ordinary-feedback-text", "--send")
	mu.Lock()
	reached := strings.Contains(strings.Join(got, ""), "ordinary-feedback-text")
	mu.Unlock()
	if !reached {
		t.Fatal("honest feedback never reached the endpoint")
	}
	text, _ := json.Marshal(map[string]string{"password": bbRound2})
	out := e.run(t, "feedback", string(text), "--send")
	mu.Lock()
	defer mu.Unlock()
	for _, b := range got {
		if strings.Contains(b, bbRound2) || strings.Contains(b, "9c71b4") {
			t.Errorf("the feedback endpoint received the credential: %s", b)
		}
	}
	if strings.Contains(out, bbRound2) {
		t.Errorf("the command printed the credential:\n%.800s", out)
	}
}
