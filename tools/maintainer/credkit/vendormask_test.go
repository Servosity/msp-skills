// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"database/sql/driver"
	"io"
	"net/http"
	"testing"
)

func TestVendorMask(t *testing.T) {
	for v, want := range map[string]bool{
		"********": true, "•••••••": true, "*": true, "########": true,
		"": false, "aaaaaaaa": false, "11111111": false, "        ": false,
		"**!*****": false, "pw******": false, "\xff\xff\xff": false, "\x00\x00\x00": false, "~~~~": true,
	} {
		if got := vendorMask(v); got != want {
			t.Errorf("vendorMask(%q) = %v, want %v", v, got, want)
		}
	}
}

// A vendor response field that carries the vendor's mask in place of the
// secret is not held; a real value in the same field is; the same mask from
// a non-response source is held as before.
func TestVendorMaskFromResponseNotHeld(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"password"}})
	body := `{"powerUser":{"enabled":true,"password":"********"},"proxy":{"password":"pxPwHd61Ys3Tc8"}}`
	resp := &http.Response{Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader([]byte(body)))}
	holdResponse(resp, origin{})
	_, _ = io.ReadAll(resp.Body)
	if isHeld("********") {
		t.Error("a vendor mask from a response was held")
	}
	if !isHeld("pxPwHd61Ys3Tc8") {
		t.Error("a real response credential was not held")
	}
	Hold("env:X", "********")
	if !isHeld("********") {
		t.Error("a mask from a non-response source was not held")
	}
}

// The mask is judged on the whole response value: a part derived from a
// value that is not a mask ("Bearer ****" gives "****") stays held.
func TestVendorMaskDerivedPartHeld(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"authorization"}})
	body := `{"authorization":"Bearer ****","echo":"****"}`
	resp := &http.Response{Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader([]byte(body)))}
	holdResponse(resp, origin{})
	_, _ = io.ReadAll(resp.Body)
	if !isHeld("****") {
		t.Error("a part of a non-mask response value was not held")
	}
}

// A value held from another source first, then returned as a mask in a
// response field, is still marked as response-held: the store refuses it.
func TestVendorMaskKeepsResponseProvenanceOfAHeldValue(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "********")
	Hold("response:password", "********")
	if !storeRefuses([]driver.NamedValue{{Ordinal: 1, Value: "********"}}) {
		t.Error("a held value seen in a response field is not refused by the store")
	}
}
