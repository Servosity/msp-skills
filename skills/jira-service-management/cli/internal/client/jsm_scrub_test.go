// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"testing"
)

func TestScrubDropsMediaJwtToken(t *testing.T) {
	in := []byte(`{"id":"1","objectKey":"ITAM-1","avatar":{"mediaClientConfig":{"clientId":"c","mediaJwtToken":"SENTINEL-JWT"}},"values":[{"mediaJwtToken":"SENTINEL-JWT2","n":12345678901234567890}]}`)
	out := jsmScrubIncidentalSecrets(in)
	if bytes.Contains(out, []byte("SENTINEL-JWT")) {
		t.Fatalf("mediaJwtToken survived: %s", out)
	}
	if !bytes.Contains(out, []byte(`"objectKey":"ITAM-1"`)) || !bytes.Contains(out, []byte("12345678901234567890")) {
		t.Fatalf("scrub damaged other fields: %s", out)
	}
	plain := []byte(`{"token":"keep-me"}`)
	if got := jsmScrubIncidentalSecrets(plain); !bytes.Equal(got, plain) {
		t.Fatalf("unrelated body rewritten: %s", got)
	}
}
