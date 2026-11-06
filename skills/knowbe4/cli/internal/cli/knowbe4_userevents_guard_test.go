// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"knowbe4-pp-cli/internal/cliutil/testenv"
)

// knowbe4 handfix user-event-origin-pinned: the User Event API key is held
// from the environment before anything prints, and the events client sends
// only to its region's User Event API origin: a request or a redirect to any
// other host is refused before it is dialled (no network is used here).
func TestUserEventKeyStaysOnTheEventsHost(t *testing.T) {
	const key = "userEventUnitKey-6d2f9a4c1e"
	testenv.Isolate(t)
	t.Setenv("KNOWBE4_USER_EVENT_API_KEY", key)
	t.Setenv("KNOWBE4_REGION", "eu")
	c, err := newUserEventsClient(&rootFlags{timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.BaseURL(); got != "https://api-eu.events.knowbe4.com" {
		t.Fatalf("events host = %s", got)
	}
	for _, raw := range []string{"https://evil.example/events", "http://api-eu.events.knowbe4.com/events",
		"https://api.events.knowbe4.com/events", "https://eu.api.knowbe4.com/v1/users"} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		if _, err := c.HTTP.Do(req); !errors.Is(err, ErrOriginChanged) {
			t.Errorf("%s: want the pin refusal, got %v", raw, err)
		}
	}
	if c.HTTP.CheckRedirect == nil {
		t.Fatal("the events client follows redirects unchecked")
	}
	from, _ := http.NewRequest(http.MethodGet, "https://api-eu.events.knowbe4.com/events", nil)
	to, _ := http.NewRequest(http.MethodGet, "https://evil.example/collect", nil)
	if err := c.HTTP.CheckRedirect(to, []*http.Request{from}); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("a redirect off the events host: want the pin refusal, got %v", err)
	}
	dry, err := newUserEventsClient(&rootFlags{timeout: 5 * time.Second, dryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "https://api-eu.events.knowbe4.com/events", nil)
	if _, err := dry.HTTP.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Errorf("--dry-run: want nothing sent, got %v", err)
	}
}
