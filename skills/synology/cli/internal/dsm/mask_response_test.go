// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestMaskResponseCoversEveryForm: a successful answer carrying a secret
// value the client holds (a sid echoed in a user's description, a password
// JSON-escaped or percent-encoded inside a URL) is masked in every form; the
// rest of the answer is untouched; a short secret (a 6-digit one-time code)
// is masked only as a whole string value, never inside other numbers or
// text, and a number is never rewritten.
func TestMaskResponseCoversEveryForm(t *testing.T) {
	const sid, pw, tok, otp = "SID-LEAK-7731", `p@ss/w"rd&9931`, "TOKLEAK5566", "918273"
	body := `{"users":[` +
		`{"name":"alice","description":"session SID-LEAK-7731 copied here","uid":1026},` +
		`{"name":"bob","description":"SID-LEAK-7731 escaped"},` +
		`{"name":"carol","url":"https://x/?p=` + url.QueryEscape(pw) + `&t=` + url.QueryEscape(url.QueryEscape(tok)) + `"},` +
		`{"name":"dave","note":"` + strings.ReplaceAll(strings.ReplaceAll(pw, `\`, `\\`), `"`, `\"`) + `"},` +
		`{"name":"erin","note":"sid%2dleak lower: SID%2DLEAK-7731 and sid%2DLEAK-7731"},` +
		`{"name":"frank","code":918273,"otp":"918273","ts":1791827300,"text":"call 918273 later"},` +
		`{"name":"gina","html":"p@ss/w&quot;rd&amp;9931"}` +
		`],"total":7}`
	if !json.Valid([]byte(body)) {
		t.Fatalf("fixture is not JSON: %s", body)
	}
	out := string(MaskResponse([]byte(body), sid, pw, tok, otp, "", "ab"))
	if !json.Valid([]byte(out)) {
		t.Fatalf("masked answer is not JSON: %s", out)
	}
	var decoded any
	_ = json.Unmarshal([]byte(out), &decoded)
	plain, _ := json.Marshal(decoded)
	for _, view := range []string{out, string(plain), Decoded(out), Decoded(string(plain))} {
		for _, secret := range []string{sid, pw, tok} {
			if strings.Contains(view, secret) {
				t.Fatalf("secret %q survived masking: %s", secret, view)
			}
		}
	}
	for _, keep := range []string{`"name":"alice"`, `"uid":1026`, `"ts":1791827300`, `"text":"call 918273 later"`, `"total":7`, "copied here"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("masking destroyed %s: %s", keep, out)
		}
	}
	if !strings.Contains(out, `"otp":"[redacted]"`) {
		t.Fatalf("a whole-value one-time code was not masked: %s", out)
	}
	if !strings.Contains(out, `"code":918273`) {
		t.Fatalf("masking rewrote a number (it is type-preserving: string values only): %s", out)
	}
	// Nothing to mask: the bytes come back as they were.
	if got := string(MaskResponse([]byte(`{"a": 1,  "b":"x"}`), sid)); got != `{"a": 1,  "b":"x"}` {
		t.Fatalf("an answer without secrets changed: %s", got)
	}
	// Plain text is masked as text.
	if got := string(MaskResponse([]byte("echo SID-LEAK-7731"), sid)); strings.Contains(got, sid) {
		t.Fatalf("plain text kept the sid: %s", got)
	}
}

// TestFleetClientMasksSecretValues: the fleet client (fleet sync, fleet add
// --check, abb triage, doctor) masks every successful answer for the
// password, one-time code, device token, sid and SynoToken it holds before
// the fleet layer prints or mirrors it.
func TestFleetClientMasksSecretValues(t *testing.T) {
	const sid, tok, pw = "SID-LEAK-7731-fleet", "TOKLEAK-fleet-5566", "PW-LEAK-fleet-9900"
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"` + sid + `","synotoken":"` + tok + `"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"tasks":[{"task_id":5,"task_name":"copied ` + sid + `","note":"` + url.QueryEscape(pw) + `","t":"\u0054OKLEAK-fleet-5566"}]}}`))
	}))
	defer nas.Close()
	c := &Client{HTTP: nas.Client(), BaseURL: nas.URL, Creds: Credentials{Account: "a", Password: pw}, RatePerSec: 1000}
	data, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{sid, tok, pw} {
		if strings.Contains(Decoded(string(data)), s) || strings.Contains(string(data), s) {
			t.Fatalf("the fleet client returned secret %q: %s", s, data)
		}
	}
	if !strings.Contains(string(data), `"task_id":5`) || !strings.Contains(string(data), ResponseMask) {
		t.Fatalf("masking destroyed the answer: %s", data)
	}
}

// TestMaskComparesEveryDecodingStage: a secret whose own text contains an
// escape (pw%2Fab) echoed once more encoded (pw%252Fab) decodes to the secret
// one stage in and to pw/ab at the fixed point. Comparing only the fixed
// point missed it; every stage is compared, whole-field and inside text.
func TestMaskComparesEveryDecodingStage(t *testing.T) {
	const short, long = "pw%2Fab", "Long%2FPassword-9931"
	body := `{"success":true,"data":{"note":"pw%252Fab","text":"see Long%252FPassword-9931 here","plain":"pw/ab"}}`
	out := string(MaskResponse([]byte(body), short, long))
	if strings.Contains(out, "pw%252Fab") || strings.Contains(out, "Long%252FPassword-9931") {
		t.Fatalf("an encoded echo of a held secret survived masking: %s", out)
	}
	if !strings.Contains(out, `"plain":"pw/ab"`) {
		t.Fatalf("a value that is not a secret at any stage was masked: %s", out)
	}
}
