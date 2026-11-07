// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"keeper-pp-cli/internal/client"
)

// commanderSession models Service Mode's one global managed-company context
// (commands/msp.py current_mc_id): switch-to-msp answers "Already MSP" when
// no company is selected, switch-to-mc refuses with "Already switched to
// Managed Company id=<id>" while one is, and a report reads whichever
// company is selected (msp-info answers one row naming it).
type commanderSession struct {
	mu        sync.Mutex
	current   string
	sent      []string
	failBack  bool                   // switch-to-msp out of a company fails
	switched  map[string]chan string // closed when switch-to-mc <id> succeeds
	reportsIn []string
	multiline bool // answer switch-to-mc with a multi-line log, as a list
}

func (s *commanderSession) set(mc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = mc
}

func (s *commanderSession) switchedTo(mc string) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switched == nil {
		s.switched = map[string]chan string{}
	}
	if s.switched[mc] == nil {
		s.switched[mc] = make(chan string)
	}
	return s.switched[mc]
}

func (s *commanderSession) Post(_ context.Context, path string, body any) (json.RawMessage, int, error) {
	cmd := body.(map[string]string)["command"]
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, cmd)
	fail := func(msg string) (json.RawMessage, int, error) {
		b := fmt.Sprintf(`{"status":"error","error":%q}`, "Unexpected error: "+msg)
		return nil, 500, &client.APIError{Method: "POST", Path: path, StatusCode: 500, Body: b}
	}
	f := strings.Fields(cmd)
	switch f[0] {
	case "switch-to-msp":
		if s.current == "" {
			return fail("switch-to-msp: Already MSP")
		}
		if s.failBack {
			return fail("boom")
		}
		s.current = ""
		return json.RawMessage(`{"status":"success","command":"switch-to-msp","message":"Switched back to MSP","data":null}`), 200, nil
	case "switch-to-mc":
		if s.current != "" {
			return fail("switch-to-mc: Already switched to Managed Company id=" + s.current)
		}
		s.current = f[1]
		if ch := s.switched[f[1]]; ch != nil {
			close(ch)
			s.switched[f[1]] = nil
		}
		if s.multiline {
			return json.RawMessage(fmt.Sprintf(`{"status":"success","command":"switch-to-mc","message":["Syncing...","Switched to MC 'Co %s'"],"data":null}`, f[1])), 200, nil
		}
		return json.RawMessage(fmt.Sprintf(`{"status":"success","command":"switch-to-mc","message":"Switched to MC 'Co %s'","data":null}`, f[1])), 200, nil
	case "msp-info":
		in := s.current
		if in == "" {
			in = "MSP"
		}
		s.reportsIn = append(s.reportsIn, in)
		return json.RawMessage(fmt.Sprintf(`{"status":"success","command":"msp-info","data":[{"company_id":1,"company_name":%q}]}`, in)), 200, nil
	}
	return fail("unscripted")
}

func readName(ctx context.Context, s *commanderSession) (string, error) {
	rows, err := Run(ctx, s, Companies())
	if err != nil {
		return "", err
	}
	return rows[0]["company_name"].(string), nil
}

// The managed-company guard: reset first, switch in, confirm, read, check the
// session is still in the company, switch back.
func TestWithCompanySwitchesInChecksAndBack(t *testing.T) {
	s := &commanderSession{}
	var got string
	err := WithCompany(context.Background(), s, "4421", nil, func() error {
		var err error
		got, err = readName(context.Background(), s)
		return err
	})
	if err != nil || got != "4421" {
		t.Fatalf("err %v read in %q", err, got)
	}
	want := []string{"switch-to-msp", "switch-to-mc 4421", "msp-info --format=json --verbose", "switch-to-mc 4421", "switch-to-msp"}
	if strings.Join(s.sent, "|") != strings.Join(want, "|") {
		t.Fatalf("sent %v", s.sent)
	}
	if s.current != "" {
		t.Fatalf("session left in %s", s.current)
	}
}

// A session left in a managed company is returned to the MSP before an
// MSP-level read, and the caller is told.
func TestStraySessionIsReturnedBeforeAnMSPRead(t *testing.T) {
	s := &commanderSession{current: "5310"}
	var warned, got string
	err := WithCompany(context.Background(), s, "", func(m string) { warned = m }, func() error {
		var err error
		got, err = readName(context.Background(), s)
		return err
	})
	if err != nil || got != "MSP" || warned == "" {
		t.Fatalf("err %v read in %q warned %q", err, got, warned)
	}
}

// Codex r1 class cross-tenant-context-race: two reads of different managed
// companies at once never interleave their switches. Read A waits inside its
// transaction for B to switch to company 20; with the session lock B cannot,
// so A reads company 10 and B then reads company 20.
func TestConcurrentCompanyReadsNeverInterleave(t *testing.T) {
	s := &commanderSession{}
	bSwitched := s.switchedTo("20")
	aInRead := make(chan struct{})
	var mu sync.Mutex
	got, errs := map[string]string{}, map[string]error{}
	var wg sync.WaitGroup
	run := func(mc string, wait bool) {
		defer wg.Done()
		err := WithCompany(context.Background(), s, mc, nil, func() error {
			if wait {
				close(aInRead)
				select {
				case <-bSwitched:
				case <-time.After(300 * time.Millisecond):
				}
			}
			name, err := readName(context.Background(), s)
			mu.Lock()
			got[mc] = name
			mu.Unlock()
			return err
		})
		mu.Lock()
		errs[mc] = err
		mu.Unlock()
	}
	wg.Add(1)
	go run("10", true)
	<-aInRead
	wg.Add(1)
	go run("20", false)
	wg.Wait()
	for _, mc := range []string{"10", "20"} {
		if errs[mc] != nil || got[mc] != mc {
			t.Errorf("read of %s: err %v, read company %q (sent %v)", mc, errs[mc], got[mc], s.sent)
		}
	}
}

// Another client that moves the shared session during a read (not this CLI:
// no lock is shared with it) is caught by the post-read check, and the result
// is discarded.
func TestExternalSwitchDuringAReadDiscardsIt(t *testing.T) {
	for name, tc := range map[string]struct {
		mc, during string
	}{
		"company read, other client switched to another company": {"4421", "5310"},
		"company read, other client returned the session to MSP": {"4421", ""},
		"MSP read, other client switched into a company":         {"", "5310"},
	} {
		s := &commanderSession{}
		err := WithCompany(context.Background(), s, tc.mc, nil, func() error {
			s.set(tc.during)
			return nil
		})
		if !errors.Is(err, ErrContextChanged) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestSwitchBackFailureIsReported(t *testing.T) {
	s := &commanderSession{}
	err := WithCompany(context.Background(), s, "4421", nil, func() error {
		s.mu.Lock()
		s.failBack = true
		s.mu.Unlock()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "could not return the session") {
		t.Fatalf("got %v", err)
	}
}

// Service Mode returns a multi-line log message as a list of lines
// (service/util/parse_keeper_response.py _format_multiline_message), so a
// switch confirmation can arrive as ["Syncing...", "Switched to MC '...'"].
func TestMultiLineSwitchConfirmationIsRead(t *testing.T) {
	s := &commanderSession{multiline: true}
	var got string
	err := WithCompany(context.Background(), s, "4421", nil, func() error {
		var err error
		got, err = readName(context.Background(), s)
		return err
	})
	if err != nil || got != "4421" {
		t.Fatalf("err %v read in %q", err, got)
	}
}
