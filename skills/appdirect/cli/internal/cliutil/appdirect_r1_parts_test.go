// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/base64"
	"strings"
	"testing"
)

// derived-credential-parts-held (Codex r1 4.34.2 class holder incomplete for
// decoded authentication parts): a credential-named response member holding
// "Basic <base64 user:password>" withholds the decoded password wherever else
// the output repeats it, at any length.
func TestDecodedBasicPasswordIsHeldAtAnyLength(t *testing.T) {
	body := `{"authorization":"Basic ` + base64.StdEncoding.EncodeToString([]byte("u:pQ7zW1")) + `","name":"pQ7zW1"}`
	HoldCredentialFields([]byte(body))
	if got := WithholdText(body); strings.Contains(got, "pQ7zW1") {
		t.Errorf("the decoded Basic password printed: %s", got)
	}
	if got := string(WithholdBytes([]byte("name: pQ7zW1\n"))); strings.Contains(got, "pQ7zW1") {
		t.Errorf("the decoded Basic password printed on another line: %s", got)
	}
}

// Every held value's base64 id:secret halves are held at any length, whatever
// source held it (an APPDIRECT_CLIENT_ID of short keys, an argument, a header).
func TestHeldPairHalvesAreHeldAtAnyLength(t *testing.T) {
	FunnelHold(base64.StdEncoding.EncodeToString([]byte("Vk3q:Rt5zQ")))
	for _, text := range []string{`{"vendor":"Vk3q"}`, `{"partner":"Rt5zQ"}`, "partner Rt5zQ here"} {
		if got := WithholdText(text); strings.Contains(got, "Vk3q") || strings.Contains(got, "Rt5zQ") {
			t.Errorf("%s: a half of the held pair printed: %s", text, got)
		}
	}
	if HeldEchoes("an unrelated line about Vk and Rt") {
		t.Error("unrelated text must pass")
	}
}
