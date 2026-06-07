package userevents

import (
	"encoding/json"
	"testing"
)

func TestBaseURLForRegion(t *testing.T) {
	cases := map[string]string{
		"":   "https://api.events.knowbe4.com",
		"us": "https://api.events.knowbe4.com",
		"US": "https://api.events.knowbe4.com",
		"eu": "https://api-eu.events.knowbe4.com",
		"ca": "https://api-ca.events.knowbe4.com",
		"uk": "https://api-uk.events.knowbe4.com",
		"de": "https://api-de.events.knowbe4.com",
	}
	for region, want := range cases {
		if got := BaseURLForRegion(region); got != want {
			t.Errorf("BaseURLForRegion(%q) = %q, want %q", region, got, want)
		}
	}
}

func TestUnwrapEnvelope(t *testing.T) {
	// {data:[...]} -> array
	if got := unwrap(json.RawMessage(`{"data":[{"id":1}],"meta":{"count":1}}`)); string(got) != `[{"id":1}]` {
		t.Errorf("unwrap list envelope = %s", got)
	}
	// {data:{...}} -> object
	if got := unwrap(json.RawMessage(`{"data":{"id":"abc"}}`)); string(got) != `{"id":"abc"}` {
		t.Errorf("unwrap object envelope = %s", got)
	}
	// bare array (no envelope) -> unchanged
	if got := unwrap(json.RawMessage(`[{"id":2}]`)); string(got) != `[{"id":2}]` {
		t.Errorf("unwrap bare array = %s", got)
	}
}

func TestNewRequiresKey(t *testing.T) {
	t.Setenv(EnvKey, "")
	if _, err := New(0); err == nil {
		t.Fatalf("New should error when %s is unset", EnvKey)
	}
	t.Setenv(EnvKey, "test-key")
	c, err := New(0)
	if err != nil {
		t.Fatalf("New with key: %v", err)
	}
	if c.BaseURL() != "https://api.events.knowbe4.com" {
		t.Errorf("default base URL = %q", c.BaseURL())
	}
}
